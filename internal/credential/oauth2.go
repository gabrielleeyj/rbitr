package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultTokenCacheTTL = 300 // 5 minutes
	httpTimeout          = 30 * time.Second
	tokenExpiryBuffer    = 30 * time.Second // refresh 30s before expiry
	maxErrorBodyBytes    = 1024
)

// oauth2Token represents a cached OAuth2 access token.
type oauth2Token struct {
	accessToken string
	expiresAt   time.Time
}

// OAuth2Provider resolves credentials via the OAuth2 client_credentials grant.
type OAuth2Provider struct {
	mu     sync.RWMutex
	cache  map[string]*oauth2Token // keyed by "tokenURL|clientID"
	client *http.Client
}

// NewOAuth2Provider creates a new OAuth2 credential provider.
func NewOAuth2Provider() *OAuth2Provider {
	return &OAuth2Provider{
		cache:  make(map[string]*oauth2Token),
		client: &http.Client{Timeout: httpTimeout},
	}
}

func (p *OAuth2Provider) Resolve(ctx context.Context, _, _ string, config *Config) (ResolvedCredential, error) {
	if config == nil {
		return ResolvedCredential{}, ErrNoCredential
	}

	token, err := p.getToken(ctx, config)
	if err != nil {
		return ResolvedCredential{}, err
	}

	return ResolvedCredential{
		HeaderName:  "Authorization",
		HeaderValue: "Bearer " + token,
	}, nil
}

func (p *OAuth2Provider) HealthCheck(ctx context.Context, _, _ string, config *Config) error {
	if config == nil {
		return ErrNoCredential
	}
	_, err := p.getToken(ctx, config)
	return err
}

func (p *OAuth2Provider) getToken(ctx context.Context, config *Config) (string, error) {
	cacheKey := config.TokenURL + "|" + config.ClientID

	// Check cache first (read lock).
	p.mu.RLock()
	if tok, ok := p.cache[cacheKey]; ok && time.Now().Before(tok.expiresAt.Add(-tokenExpiryBuffer)) {
		p.mu.RUnlock()
		return tok.accessToken, nil
	}
	p.mu.RUnlock()

	// Cache miss or expired — fetch new token (write lock).
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if tok, ok := p.cache[cacheKey]; ok && time.Now().Before(tok.expiresAt.Add(-tokenExpiryBuffer)) {
		return tok.accessToken, nil
	}

	tok, err := p.fetchToken(ctx, config)
	if err != nil {
		return "", err
	}

	p.cache[cacheKey] = tok
	return tok.accessToken, nil
}

func (p *OAuth2Provider) fetchToken(ctx context.Context, config *Config) (*oauth2Token, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {config.ClientID},
		"client_secret": {config.ClientSecret},
	}
	if config.Scope != "" {
		data.Set("scope", config.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTokenRefreshFailed, err.Error())
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTokenRefreshFailed, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("%w: token endpoint returned %d: %s", ErrTokenRefreshFailed, resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("%w: invalid token response: %s", ErrTokenRefreshFailed, err.Error())
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("%w: empty access_token in response", ErrTokenRefreshFailed)
	}

	ttl := config.TokenCacheTTL
	if ttl <= 0 {
		ttl = defaultTokenCacheTTL
	}
	// Use the shorter of configured TTL and server-reported expiry.
	expiresIn := time.Duration(ttl) * time.Second
	if tokenResp.ExpiresIn > 0 {
		serverExpiry := time.Duration(tokenResp.ExpiresIn) * time.Second
		if serverExpiry < expiresIn {
			expiresIn = serverExpiry
		}
	}

	return &oauth2Token{
		accessToken: tokenResp.AccessToken,
		expiresAt:   time.Now().Add(expiresIn),
	}, nil
}
