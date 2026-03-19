package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// OIDC errors.
var (
	ErrOIDCDiscoveryFailed = errors.New("oidc discovery failed")
	ErrOIDCTokenExchange   = errors.New("oidc token exchange failed")
	ErrOIDCTokenInvalid    = errors.New("oidc token invalid")
	ErrOIDCDomainDenied    = errors.New("email domain not allowed")
	ErrOIDCNotConfigured   = errors.New("sso not configured")
)

// OIDCDiscovery holds the endpoints from /.well-known/openid-configuration.
type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// OIDCTokenResponse is the token endpoint response.
type OIDCTokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// OIDCUserInfo holds validated user identity from the ID token.
type OIDCUserInfo struct {
	Subject string
	Email   string
	Name    string
}

// OIDCConfig holds the SSO/OIDC configuration.
type OIDCConfig struct {
	Enabled        bool
	Issuer         string
	ClientID       string
	ClientSecret   string
	RedirectURI    string
	AllowedDomains []string
	DefaultScopes  []string
}

// OIDCProvider handles OIDC discovery, token exchange, and ID token validation.
type OIDCProvider struct {
	httpClient *http.Client

	mu        sync.RWMutex
	discovery *OIDCDiscovery
	keySet    jwk.Set
	issuer    string
}

// NewOIDCProvider creates a new OIDC provider.
func NewOIDCProvider(httpClient *http.Client) *OIDCProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second} //nolint:mnd // default timeout
	}
	return &OIDCProvider{httpClient: httpClient}
}

// Discover fetches the OIDC discovery document and JWK set for the given issuer.
func (p *OIDCProvider) Discover(ctx context.Context, issuer string) (*OIDCDiscovery, error) {
	p.mu.RLock()
	if p.discovery != nil && p.issuer == issuer {
		disc := p.discovery
		p.mu.RUnlock()
		return disc, nil
	}
	p.mu.RUnlock()

	wellKnown := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCDiscoveryFailed, err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCDiscoveryFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrOIDCDiscoveryFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) //nolint:mnd // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCDiscoveryFailed, err)
	}

	var disc OIDCDiscovery
	if err := json.Unmarshal(body, &disc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCDiscoveryFailed, err)
	}

	if disc.JWKSURI == "" {
		return nil, fmt.Errorf("%w: missing jwks_uri", ErrOIDCDiscoveryFailed)
	}

	// Fetch the JWK set.
	keySet, err := jwk.Fetch(ctx, disc.JWKSURI, jwk.WithHTTPClient(p.httpClient))
	if err != nil {
		return nil, fmt.Errorf("%w: jwk fetch: %w", ErrOIDCDiscoveryFailed, err)
	}

	p.mu.Lock()
	p.discovery = &disc
	p.keySet = keySet
	p.issuer = issuer
	p.mu.Unlock()

	return &disc, nil
}

// AuthorizationURL builds the IdP authorization URL for the OIDC code flow.
func (p *OIDCProvider) AuthorizationURL(config OIDCConfig, state string) (string, error) {
	p.mu.RLock()
	disc := p.discovery
	p.mu.RUnlock()

	if disc == nil || disc.AuthorizationEndpoint == "" {
		return "", ErrOIDCNotConfigured
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {config.ClientID},
		"redirect_uri":  {config.RedirectURI},
		"scope":         {"openid email profile"},
		"state":         {state},
	}

	return disc.AuthorizationEndpoint + "?" + params.Encode(), nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *OIDCProvider) ExchangeCode(ctx context.Context, config OIDCConfig, code string) (*OIDCTokenResponse, error) {
	p.mu.RLock()
	disc := p.discovery
	p.mu.RUnlock()

	if disc == nil || disc.TokenEndpoint == "" {
		return nil, ErrOIDCNotConfigured
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {config.RedirectURI},
		"client_id":     {config.ClientID},
		"client_secret": {config.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, disc.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCTokenExchange, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCTokenExchange, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) //nolint:mnd // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCTokenExchange, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d: %s", ErrOIDCTokenExchange, resp.StatusCode, string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCTokenExchange, err)
	}

	if tokenResp.IDToken == "" {
		return nil, fmt.Errorf("%w: missing id_token", ErrOIDCTokenExchange)
	}

	return &tokenResp, nil
}

// ValidateIDToken validates an OIDC ID token and returns the user info.
func (p *OIDCProvider) ValidateIDToken(ctx context.Context, rawToken string, config OIDCConfig) (OIDCUserInfo, error) {
	p.mu.RLock()
	keySet := p.keySet
	p.mu.RUnlock()

	if keySet == nil {
		return OIDCUserInfo{}, ErrOIDCNotConfigured
	}

	token, err := jwt.Parse(
		[]byte(rawToken),
		jwt.WithKeySet(keySet),
		jwt.WithValidate(true),
		jwt.WithIssuer(config.Issuer),
		jwt.WithAudience(config.ClientID),
		jwt.WithContext(ctx),
	)
	if err != nil {
		return OIDCUserInfo{}, fmt.Errorf("%w: %w", ErrOIDCTokenInvalid, err)
	}

	sub, _ := token.Subject()
	email := claimString(token, "email")
	name := claimString(token, "name")

	if email == "" {
		return OIDCUserInfo{}, fmt.Errorf("%w: missing email claim", ErrOIDCTokenInvalid)
	}

	// Check allowed domains.
	if len(config.AllowedDomains) > 0 {
		domain := emailDomain(email)
		if !domainAllowed(domain, config.AllowedDomains) {
			return OIDCUserInfo{}, ErrOIDCDomainDenied
		}
	}

	return OIDCUserInfo{
		Subject: sub,
		Email:   email,
		Name:    name,
	}, nil
}

func claimString(token jwt.Token, key string) string {
	var val string
	if err := token.Get(key, &val); err != nil {
		return ""
	}
	return val
}

func emailDomain(email string) string {
	parts := strings.SplitN(email, "@", 2) //nolint:mnd // email: local@domain
	if len(parts) != 2 {                   //nolint:mnd // email must have exactly one @
		return ""
	}
	return strings.ToLower(parts[1])
}

func domainAllowed(domain string, allowed []string) bool {
	for _, d := range allowed {
		if strings.EqualFold(domain, d) {
			return true
		}
	}
	return false
}
