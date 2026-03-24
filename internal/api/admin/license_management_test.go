package admin

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/license"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

// signValidLicenseKey creates a valid signed license key for testing.
func signValidLicenseKey(t *testing.T, priv ed25519.PrivateKey, tier string) []byte {
	t.Helper()
	now := time.Now()
	exp := now.Add(365 * 24 * time.Hour)

	ent := license.DefaultsForTier(tier)

	tok, err := jwt.NewBuilder().
		Issuer("rbitr").
		Subject("Test Corp").
		IssuedAt(now).
		NotBefore(now).
		Expiration(exp).
		Build()
	require.NoError(t, err)

	require.NoError(t, tok.Set("key_version", float64(1)))
	require.NoError(t, tok.Set("tier", tier))
	require.NoError(t, tok.Set("entitlements", ent))
	require.NoError(t, tok.Set("licensee", map[string]string{
		"name":  "Test Corp",
		"email": "test@example.com",
	}))

	key, err := jwk.Import(priv)
	require.NoError(t, err)
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA(), key))
	require.NoError(t, err)
	return signed
}

// newValidatorWithoutKey creates a validator with no existing key file.
func newValidatorWithoutKey(t *testing.T) (*license.Validator, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keyPath := filepath.Join(t.TempDir(), "license.key")
	v := license.NewValidator(pub, keyPath)
	v.LoadAndValidate()
	return v, priv
}

// --- handleLicenseStatus ---

func TestHandleLicenseStatus_NilValidator(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.GET("/license", deps.handleLicenseStatus)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/license", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["valid"])
	assert.Equal(t, "free", body["tier"])
}

func TestHandleLicenseStatus_FreeTier(t *testing.T) {
	v, _ := newValidatorWithoutKey(t)
	deps := &Dependencies{LicenseValidator: v}

	e := echo.New()
	e.GET("/license", deps.handleLicenseStatus)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/license", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["valid"])
	assert.Equal(t, "free", body["tier"])
	// No licensee/expiry fields for free tier.
	assert.Nil(t, body["licensee"])
}

func TestHandleLicenseStatus_PaidTier(t *testing.T) {
	v := testLicenseValidator(t, "paid", license.Unlimited, license.Unlimited)
	deps := &Dependencies{LicenseValidator: v}

	e := echo.New()
	e.GET("/license", deps.handleLicenseStatus)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/license", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["valid"])
	assert.Equal(t, "paid", body["tier"])
	assert.Equal(t, "Test", body["licensee"])
	assert.NotNil(t, body["expires_at"])
	assert.NotNil(t, body["days_remaining"])
}

// --- handleLicenseUpload ---

func TestHandleLicenseUpload_RawBody(t *testing.T) {
	v, priv := newValidatorWithoutKey(t)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().InsertLicenseHistory(
		mock.Anything, "paid", 1, "Test Corp", "test@example.com",
		mock.AnythingOfType("time.Time"), mock.AnythingOfType("string"),
	).Return(nil)

	deps := &Dependencies{LicenseValidator: v, Store: storeMock}

	tokenBytes := signValidLicenseKey(t, priv, "paid")

	e := echo.New()
	e.POST("/license", deps.handleLicenseUpload)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/license", bytes.NewReader(tokenBytes))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["valid"])
	assert.Equal(t, "paid", body["tier"])
	assert.Equal(t, "Test Corp", body["licensee"])
	assert.NotEmpty(t, body["fingerprint"])

	// Verify key was written to disk.
	written, err := os.ReadFile(v.KeyPath())
	require.NoError(t, err)
	assert.Equal(t, tokenBytes, written)

	// Verify validator was reloaded.
	assert.True(t, v.Info().Valid)
	assert.Equal(t, "paid", v.Info().Tier)
}

func TestHandleLicenseUpload_MultipartForm(t *testing.T) {
	v, priv := newValidatorWithoutKey(t)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().InsertLicenseHistory(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything,
	).Return(nil)

	deps := &Dependencies{LicenseValidator: v, Store: storeMock}

	tokenBytes := signValidLicenseKey(t, priv, "paid")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "license.key")
	require.NoError(t, err)
	_, err = part.Write(tokenBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	e := echo.New()
	e.POST("/license", deps.handleLicenseUpload)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/license", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["valid"])
}

func TestHandleLicenseUpload_InvalidKey(t *testing.T) {
	v, _ := newValidatorWithoutKey(t)
	deps := &Dependencies{LicenseValidator: v}

	e := echo.New()
	e.POST("/license", deps.handleLicenseUpload)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/license", bytes.NewReader([]byte("garbage-data")))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "INVALID_LICENSE_KEY", body["error"])
}

func TestHandleLicenseUpload_EmptyBody(t *testing.T) {
	v, _ := newValidatorWithoutKey(t)
	deps := &Dependencies{LicenseValidator: v}

	e := echo.New()
	e.POST("/license", deps.handleLicenseUpload)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/license", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleLicenseUpload_NilValidator(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.POST("/license", deps.handleLicenseUpload)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/license", bytes.NewReader([]byte("data")))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// --- handleLicenseRemove ---

func TestHandleLicenseRemove_WithExistingKey(t *testing.T) {
	v, priv := newValidatorWithoutKey(t)

	// Write a valid key first.
	tokenBytes := signValidLicenseKey(t, priv, "paid")
	require.NoError(t, os.WriteFile(v.KeyPath(), tokenBytes, 0o600))
	v.LoadAndValidate()
	require.True(t, v.Info().Valid)

	deps := &Dependencies{LicenseValidator: v}

	e := echo.New()
	e.DELETE("/license", deps.handleLicenseRemove)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/license", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["valid"])
	assert.Equal(t, "free", body["tier"])

	// Key file should be gone.
	_, err := os.Stat(v.KeyPath())
	assert.True(t, os.IsNotExist(err))

	// Validator should revert to free tier.
	assert.False(t, v.Info().Valid)
	assert.Equal(t, "free", v.Info().Tier)
}

func TestHandleLicenseRemove_NoExistingKey(t *testing.T) {
	v, _ := newValidatorWithoutKey(t)
	deps := &Dependencies{LicenseValidator: v}

	e := echo.New()
	e.DELETE("/license", deps.handleLicenseRemove)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/license", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Should succeed even if no key exists.
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleLicenseRemove_NilValidator(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.DELETE("/license", deps.handleLicenseRemove)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/license", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// --- atomicWriteFile ---

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.key")

	data := []byte("test-data")
	require.NoError(t, atomicWriteFile(path, data))

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, written)
}

// --- daysRemaining ---

func TestDaysRemaining(t *testing.T) {
	assert.Equal(t, 0, daysRemaining(time.Now().Add(-24*time.Hour)))
	// Use a large enough offset that hour-boundary rounding doesn't matter.
	days := daysRemaining(time.Now().Add(30*24*time.Hour + 12*time.Hour))
	assert.True(t, days >= 30 && days <= 31, "expected ~30 days, got %d", days)
}

// --- readLicenseBody ---

func TestReadLicenseBody_OversizeRejected(t *testing.T) {
	e := echo.New()
	bigBody := make([]byte, maxLicenseKeySize+1) //nolint:makezero // test needs zero-filled oversize buffer
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	_, err := readLicenseBody(c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum size")
}
