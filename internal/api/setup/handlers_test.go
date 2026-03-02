package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type stubService struct {
	statusResult     StatusResponse
	statusErr        error
	initializeResult InitializeResponse
	initializeErr    error
	lastInitRequest  InitializeRequest
}

func (s *stubService) Status(context.Context) (StatusResponse, error) {
	return s.statusResult, s.statusErr
}

func (s *stubService) Initialize(_ context.Context, req *InitializeRequest) (InitializeResponse, error) {
	if req != nil {
		s.lastInitRequest = *req
	}
	return s.initializeResult, s.initializeErr
}

func TestHandleStatusSuccess(t *testing.T) {
	e := echo.New()
	service := &stubService{
		statusResult: StatusResponse{
			SetupRequired: true,
			SchemaReady:   true,
		},
	}
	deps := &Dependencies{Service: service}
	RegisterRoutes(e, deps)

	req := httptest.NewRequest(http.MethodGet, "/setup/status", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload StatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.True(t, payload.SetupRequired)
	require.True(t, payload.SchemaReady)
}

func TestHandleInitializeValidationAndErrors(t *testing.T) {
	e := echo.New()
	service := &stubService{}
	deps := &Dependencies{Service: service}
	RegisterRoutes(e, deps)

	req := httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	service.initializeErr = ErrInvalidRequest
	body := bytes.NewBufferString(`{"tenant_name":"Acme"}`)
	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", body)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	service.initializeErr = ErrSchemaNotReady
	body = bytes.NewBufferString(`{"tenant_name":"Acme"}`)
	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", body)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusPreconditionFailed, rec.Code)

	service.initializeErr = ErrSetupComplete
	body = bytes.NewBufferString(`{"tenant_name":"Acme"}`)
	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", body)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)

	service.initializeErr = errors.New("boom")
	body = bytes.NewBufferString(`{"tenant_name":"Acme"}`)
	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", body)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleInitializeSuccess(t *testing.T) {
	e := echo.New()
	service := &stubService{
		initializeResult: InitializeResponse{
			BootstrapComplete: true,
			TenantID:          "t_abc12345",
			TenantName:        "Acme",
			TenantKey:         "rbtr_live_abc",
			AdminKey:          "rbtr_admin_abc",
		},
	}
	deps := &Dependencies{Service: service}
	RegisterRoutes(e, deps)

	body := bytes.NewBufferString(`{"tenant_name":"Acme","tenant_id":"t_abc12345"}`)
	req := httptest.NewRequest(http.MethodPost, "/setup/initialize", body)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "Acme", service.lastInitRequest.TenantName)

	var payload InitializeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.True(t, payload.BootstrapComplete)
	require.Equal(t, "t_abc12345", payload.TenantID)
}

func TestHandleInitializeAccessControl(t *testing.T) {
	e := echo.New()
	service := &stubService{
		initializeResult: InitializeResponse{
			BootstrapComplete: true,
			TenantID:          "t_abc12345",
			TenantName:        "Acme",
		},
	}
	deps := &Dependencies{
		Service: service,
		AccessPolicy: AccessPolicy{
			TokenRequired: true,
			Token:         "setup-secret",
		},
	}
	RegisterRoutes(e, deps)

	body := []byte(`{"tenant_name":"Acme","tenant_id":"t_abc12345"}`)

	req := httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	req.Header.Set(idempotencyHeader, "idem-1")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer setup-secret")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer setup-secret")
	req.Header.Set(idempotencyHeader, "idem-1")
	req.Header.Set("X-Request-Id", "req-1")
	req.Header.Set("X-Forwarded-For", "10.1.2.3")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "idem-1", service.lastInitRequest.IdempotencyKey)
	require.Equal(t, "req-1", service.lastInitRequest.RequestID)
	require.Equal(t, "10.1.2.3", service.lastInitRequest.ClientIP)
	require.NotEmpty(t, service.lastInitRequest.SetupTokenFingerprint)
}

func TestHandleInitializeCIDRRestriction(t *testing.T) {
	e := echo.New()
	service := &stubService{
		initializeResult: InitializeResponse{
			BootstrapComplete: true,
			TenantID:          "t_abc12345",
			TenantName:        "Acme",
		},
	}
	deps := &Dependencies{
		Service: service,
		AccessPolicy: AccessPolicy{
			AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		},
	}
	RegisterRoutes(e, deps)

	req := httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewBufferString(`{"tenant_name":"Acme"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
