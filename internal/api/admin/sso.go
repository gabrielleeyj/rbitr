package admin

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

const (
	ssoStateLen = 32
)

// SSOConfigRequest is the request body for updating SSO configuration.
type SSOConfigRequest struct {
	Issuer          string `json:"issuer"`
	ClientID        string `json:"client_id"`
	ClientSecretRef string `json:"client_secret_ref"`
	RedirectURI     string `json:"redirect_uri"`
	AllowedDomains  string `json:"allowed_domains"`
	DefaultScopes   string `json:"default_scopes"`
}

// SSOConfigResponse is the response for SSO configuration.
type SSOConfigResponse struct {
	Enabled         bool   `json:"enabled"`
	Issuer          string `json:"issuer"`
	ClientID        string `json:"client_id"`
	ClientSecretRef string `json:"client_secret_ref"`
	RedirectURI     string `json:"redirect_uri"`
	AllowedDomains  string `json:"allowed_domains"`
	DefaultScopes   string `json:"default_scopes"`
}

// SSOSessionResponse is returned after successful SSO authentication.
type SSOSessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

func (d *Dependencies) handleSSOConfigGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeSettingsRead); err != nil {
		return err
	}

	cfg, err := d.Store.GetSSOConfig(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load SSO config"})
	}

	return c.JSON(http.StatusOK, SSOConfigResponse{
		Enabled:         cfg.Enabled,
		Issuer:          cfg.Issuer,
		ClientID:        cfg.ClientID,
		ClientSecretRef: cfg.ClientSecretRef,
		RedirectURI:     cfg.RedirectURI,
		AllowedDomains:  cfg.AllowedDomains,
		DefaultScopes:   cfg.DefaultScopes,
	})
}

func (d *Dependencies) handleSSOEnabledUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SSO.ENABLED.UPDATE",
		"sso_enabled",
		d.Store.GetSSOEnabled,
		d.Store.SetSSOEnabled,
	)
}

func (d *Dependencies) handleSSOConfigUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload SSOConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if strings.TrimSpace(payload.Issuer) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "issuer is required"})
	}
	if strings.TrimSpace(payload.ClientID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "client_id is required"})
	}

	before, _ := d.Store.GetSSOConfig(c.Request().Context())

	if err := d.Store.SetSSOConfig(
		c.Request().Context(),
		payload.Issuer,
		payload.ClientID,
		payload.ClientSecretRef,
		payload.RedirectURI,
		payload.AllowedDomains,
		payload.DefaultScopes,
	); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update SSO config"})
	}

	if err := d.emitAuditEvent(c, adminKey, "", "SSO.CONFIG.UPDATE", "SETTINGS", "sso_config", map[string]any{
		"issuer":    before.Issuer,
		"client_id": before.ClientID,
	}, map[string]any{
		"issuer":    payload.Issuer,
		"client_id": payload.ClientID,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit SSO config update",
			"detail": err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleSSOAuthorize(c *echo.Context) error {
	if d.OIDCProvider == nil || d.AdminSessionMgr == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": "SSO not configured"})
	}

	cfg, err := d.Store.GetSSOConfig(c.Request().Context())
	if err != nil || !cfg.Enabled {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "SSO is not enabled"})
	}

	oidcCfg := ssoConfigToOIDC(&cfg)

	// Discover OIDC endpoints.
	if _, discoverErr := d.OIDCProvider.Discover(c.Request().Context(), oidcCfg.Issuer); discoverErr != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "OIDC discovery failed"})
	}

	state, err := generateState()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate state"})
	}

	authURL, err := d.OIDCProvider.AuthorizationURL(oidcCfg, state)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to build authorization URL"})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"authorize_url": authURL,
		"state":         state,
	})
}

func (d *Dependencies) handleSSOCallback(c *echo.Context) error {
	if d.OIDCProvider == nil || d.AdminSessionMgr == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": "SSO not configured"})
	}

	code := c.QueryParam("code")
	if code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing authorization code"})
	}

	cfg, err := d.Store.GetSSOConfig(c.Request().Context())
	if err != nil || !cfg.Enabled {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "SSO is not enabled"})
	}

	oidcCfg := ssoConfigToOIDC(&cfg)

	// Resolve the client secret from the secret reference.
	clientSecret, err := d.resolveClientSecret(c, cfg.ClientSecretRef)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve client secret"})
	}
	oidcCfg.ClientSecret = clientSecret

	// Exchange code for tokens.
	tokenResp, err := d.OIDCProvider.ExchangeCode(c.Request().Context(), oidcCfg, code)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "token exchange failed"})
	}

	// Validate ID token.
	userInfo, err := d.OIDCProvider.ValidateIDToken(c.Request().Context(), tokenResp.IDToken, oidcCfg)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid ID token"})
	}

	// Issue admin session.
	scopes := parseCSV(cfg.DefaultScopes)
	if len(scopes) == 0 {
		scopes = []string{"admin:read", "admin:write"}
	}

	token, claims, err := d.AdminSessionMgr.IssueSession(userInfo, scopes)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
	}

	return c.JSON(http.StatusOK, SSOSessionResponse{
		Token:     token,
		ExpiresAt: claims.ExpiresAt,
		Email:     claims.Email,
		Name:      claims.Name,
	})
}

func (d *Dependencies) handleSSOLogout(c *echo.Context) error {
	if d.AdminSessionMgr == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": "SSO not configured"})
	}

	token := auth.AdminKeyFromRequest(c.Request())
	if !auth.IsAdminSessionToken(token) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not an SSO session"})
	}

	claims, err := d.AdminSessionMgr.ValidateSession(token)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid session"})
	}

	d.AdminSessionMgr.RevokeSession(claims.SessionID)
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) resolveClientSecret(c *echo.Context, secretRef string) (string, error) {
	if d.SecretResolver != nil {
		return d.SecretResolver.Resolve(c.Request().Context(), secretRef)
	}
	// Fall back to treating the ref as a literal value (for development).
	return secretRef, nil
}

func ssoConfigToOIDC(cfg *store.SSOConfig) *auth.OIDCConfig {
	return &auth.OIDCConfig{
		Enabled:        cfg.Enabled,
		Issuer:         cfg.Issuer,
		ClientID:       cfg.ClientID,
		RedirectURI:    cfg.RedirectURI,
		AllowedDomains: parseCSV(cfg.AllowedDomains),
		DefaultScopes:  parseCSV(cfg.DefaultScopes),
	}
}

func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func generateState() (string, error) {
	var buf [ssoStateLen]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
