package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleApprovalsList(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectErr    bool
		validateBody func(*testing.T, []byte)
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
			expectErr:    true,
		},
		{
			name:         "forbidden",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			expectedCode: http.StatusForbidden,
			expectErr:    true,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListApprovalRequests", context.Background(), "t1", "APPROVED", 10, 0).
					Return([]models.ApprovalRequest{{
						ApprovalRequestID: "ar1",
						Status:            "APPROVED",
						ExpiresAt:         time.Now().Add(10 * time.Minute),
						RequestContext:    map[string]any{"http_method": "POST", "path": "/refund"},
					}}, nil)
			},
			expectedCode: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var payload []models.ApprovalRequest
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Len(t, payload, 1)
				require.Equal(t, "/refund", payload[0].RequestContext["path"])
			},
		},
		{
			name:     "expired auto-update",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListApprovalRequests", context.Background(), "t1", "APPROVED", 10, 0).
					Return([]models.ApprovalRequest{{ApprovalRequestID: "ar1", Status: "PENDING", ExpiresAt: time.Now().Add(-1 * time.Minute)}}, nil)
				storeMock.On("MarkApprovalExpired", context.Background(), "t1", "ar1", mock.Anything).Return(nil)
			},
			expectedCode: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var payload []models.ApprovalRequest
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Len(t, payload, 1)
				require.Equal(t, "EXPIRED", payload[0].Status)
				require.NotNil(t, payload[0].DecidedAt)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodGet,
				nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			req.URL.RawQuery = "status=APPROVED&limit=10&offset=0"
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleApprovalsList(ctx)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)

			body := rec.Body.Bytes()
			if tc.validateBody != nil {
				tc.validateBody(t, body)
			} else if tc.expectedCode == http.StatusOK {
				var payload []models.ApprovalRequest
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Len(t, payload, 1)
			}
		})
	}
}

func TestHandleApprovalApprove(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:write"}), nil)
	storeMock.On("GetApprovalRequest", context.Background(), "t1", "ar1").
		Return(models.ApprovalRequest{ApprovalRequestID: "ar1", Status: "PENDING", ExpiresAt: time.Now().Add(10 * time.Minute)}, nil).Once()
	storeMock.On("ApproveApprovalRequest", context.Background(), "t1", "ar1", "admin", "ok", mock.Anything).
		Return(nil)
	storeMock.On("GetApprovalRequest", context.Background(), "t1", "ar1").
		Return(models.ApprovalRequest{
			ApprovalRequestID: "ar1",
			Status:            "APPROVED",
			DecidedAt:         ptrTime(time.Now().UTC()),
			DecidedBy:         "admin",
			DecisionComment:   "ok",
		}, nil).Once()
	storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(ApprovalDecisionRequest{Comment: "ok"}),
		testhelpers.Params{Names: []string{"tenant_id", "approval_request_id"}, Values: []string{"t1", "ar1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	err := deps.handleApprovalApprove(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleApprovalDetailExpired(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:read"}), nil)
	storeMock.On("GetApprovalRequest", context.Background(), "t1", "ar1").
		Return(models.ApprovalRequest{ApprovalRequestID: "ar1", Status: "PENDING", ExpiresAt: time.Now().Add(-1 * time.Minute)}, nil)
	storeMock.On("MarkApprovalExpired", context.Background(), "t1", "ar1", mock.Anything).Return(nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id", "approval_request_id"}, Values: []string{"t1", "ar1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	err := deps.handleApprovalDetail(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	var payload models.ApprovalRequest
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
	require.Equal(t, "EXPIRED", payload.Status)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
