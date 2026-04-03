package azure

import (
	"bytes"
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
)

const (
	// Azure SaaS Fulfillment API v2 base URL.
	fulfillmentBaseURL = "https://marketplaceapi.microsoft.com/api/saas"

	// Azure Marketplace Metering API base URL.
	meteringBaseURL = "https://marketplaceapi.microsoft.com/api"

	// tokenEndpointTemplate is the Azure AD token endpoint.
	tokenEndpointTemplate = "https://login.microsoftonline.com/%s/oauth2/v2.0/token" //nolint:gosec // URL template, not a credential

	// marketplaceResource is the OAuth2 scope for Azure Marketplace APIs.
	marketplaceResource = "20e940b3-4c77-4b0b-9a53-9e16a1b010a7/.default"

	// apiVersion is the SaaS API version query parameter.
	apiVersion = "2018-08-31"

	// tokenRefreshBuffer is how early to refresh the token before expiry.
	tokenRefreshBuffer = 5 * time.Minute

	// httpTimeout is the default timeout for HTTP requests.
	httpTimeout = 30 * time.Second

	// maxErrorBodySize limits how much of an error response body to read.
	maxErrorBodySize = 1024
)

// tokenResponse is the Azure AD OAuth2 token response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// HTTPFulfillmentClient implements FulfillmentClient using HTTP calls to the
// Azure SaaS Fulfillment API v2 with Azure AD authentication.
type HTTPFulfillmentClient struct {
	httpClient   *http.Client
	tenantID     string
	clientID     string
	clientSecret string

	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

// NewHTTPFulfillmentClient creates a FulfillmentClient that calls the real Azure APIs.
func NewHTTPFulfillmentClient(tenantID, clientID, clientSecret string) *HTTPFulfillmentClient {
	return &HTTPFulfillmentClient{
		httpClient:   &http.Client{Timeout: httpTimeout},
		tenantID:     tenantID,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (c *HTTPFulfillmentClient) ResolveToken(ctx context.Context, token string) (*ResolvedSubscription, error) {
	reqURL := fulfillmentBaseURL + "/subscriptions/resolve?api-version=" + apiVersion

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-ms-marketplace-token", token)

	var result ResolvedSubscription
	if err := c.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("resolve token: %w", err)
	}
	return &result, nil
}

func (c *HTTPFulfillmentClient) GetSubscription(ctx context.Context, subscriptionID string) (*Subscription, error) {
	reqURL := fulfillmentBaseURL + "/subscriptions/" + url.PathEscape(subscriptionID) + "?api-version=" + apiVersion

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var result Subscription
	if err := c.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &result, nil
}

func (c *HTTPFulfillmentClient) ActivateSubscription(ctx context.Context, subscriptionID, planID string) error {
	reqURL := fulfillmentBaseURL + "/subscriptions/" + url.PathEscape(subscriptionID) + "/activate?api-version=" + apiVersion

	body := map[string]string{"planId": planID}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := c.doJSON(req, nil); err != nil {
		return fmt.Errorf("activate subscription: %w", err)
	}
	return nil
}

func (c *HTTPFulfillmentClient) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	reqURL := fulfillmentBaseURL + "/subscriptions?api-version=" + apiVersion

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var result struct {
		Subscriptions []Subscription `json:"subscriptions"`
	}
	if err := c.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return result.Subscriptions, nil
}

// doJSON executes an authenticated request and decodes the JSON response.
func (c *HTTPFulfillmentClient) doJSON(req *http.Request, result any) error {
	token, err := c.getToken(req.Context())
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// getToken returns a valid Azure AD access token, refreshing if needed.
func (c *HTTPFulfillmentClient) getToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	tokenURL := fmt.Sprintf(tokenEndpointTemplate, url.PathEscape(c.tenantID))
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"scope":         {marketplaceResource},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return "", fmt.Errorf("token HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", errors.New("empty access token in response")
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenRefreshBuffer)

	return c.accessToken, nil
}

// HTTPMeteringClient implements MeteringClient using HTTP calls to the
// Azure Marketplace Metering API.
type HTTPMeteringClient struct {
	fulfillment *HTTPFulfillmentClient
	httpClient  *http.Client
}

// NewHTTPMeteringClient creates a MeteringClient that calls the real Azure APIs.
// Shares the AAD token from the fulfillment client.
func NewHTTPMeteringClient(fulfillment *HTTPFulfillmentClient) *HTTPMeteringClient {
	return &HTTPMeteringClient{
		fulfillment: fulfillment,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}
}

func (c *HTTPMeteringClient) BatchUsageEvent(ctx context.Context, events []UsageEvent) (*BatchUsageResponse, error) {
	reqURL := meteringBaseURL + "/batchUsageEvent?api-version=" + apiVersion

	body := struct {
		Request []UsageEvent `json:"request"`
	}{Request: events}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	token, err := c.fulfillment.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch usage event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result BatchUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
