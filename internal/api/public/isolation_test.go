package public

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestCrossTenantIsolation(t *testing.T) {
	cases := []struct {
		name         string
		authTenant   string
		requestPath  string
		pathTenant   string
		expectedCode int
		expectedBody string
	}{
		{
			name:         "tenant A cannot access tenant B evidence",
			authTenant:   "t_a",
			pathTenant:   "t_b",
			expectedCode: http.StatusForbidden,
			expectedBody: "tenant mismatch",
		},
		{
			name:         "tenant A accesses own evidence",
			authTenant:   "t_a",
			pathTenant:   "t_a",
			expectedCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: tc.authTenant, Name: "Tenant A", Enabled: true}, nil)
			if tc.authTenant == tc.pathTenant {
				mockStore.On("ListEvidence", mock.Anything, tc.pathTenant, 50).
					Return([]models.ActionDecisionRecord{}, nil)
			}

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodGet, nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{tc.pathTenant}},
			)
			req.Header.Set(auth.TenantKeyHeader, "some_key")
			req.Header.Set(auth.AgentIDHeader, "agent1")

			err := deps.handleEvidence(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectedCode, rec.Code)

			if tc.expectedBody != "" {
				var body map[string]string
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				require.Contains(t, body["error"], tc.expectedBody)
			}
		})
	}
}

func TestCrossTenantMCPIsolation(t *testing.T) {
	cases := []struct {
		name           string
		authTenant     string
		pathTenant     string
		expectedStatus int
	}{
		{
			name:           "mismatch returns error",
			authTenant:     "t_a",
			pathTenant:     "t_b",
			expectedStatus: http.StatusOK, // JSON-RPC errors return 200 with error body
		},
		{
			name:           "match proceeds",
			authTenant:     "t_a",
			pathTenant:     "t_a",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: tc.authTenant, Name: "Test", Enabled: true}, nil)
			if tc.authTenant == tc.pathTenant {
				mockStore.On("ListTools", mock.Anything, tc.pathTenant).
					Return([]models.Tool{}, nil)
			}

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				bytes.NewReader([]byte(reqBody)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{tc.pathTenant}},
			)
			req.Header.Set(auth.TenantKeyHeader, "some_key")
			req.Header.Set(auth.AgentIDHeader, "agent1")

			err := deps.handleMCP(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatus, rec.Code)

			if tc.authTenant != tc.pathTenant {
				var resp map[string]any
				json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NotNil(t, resp["error"], "should have error for tenant mismatch")
			}
		})
	}
}
