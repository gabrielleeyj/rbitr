package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
	"github.com/gabrielleeyj/rbitr/internal/ticketing"
)

func TestHandleTicketingConfigGet(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "not configured returns empty",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTicketingConfig", context.Background(), "t1").
					Return(models.TicketingConfig{}, store.ErrNotFound)
			},
			expectedCode: http.StatusOK,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTicketingConfig", context.Background(), "t1").
					Return(models.TicketingConfig{
						TenantID:   "t1",
						Provider:   "jira",
						Enabled:    true,
						BaseURL:    "https://company.atlassian.net",
						SecretRef:  "env://JIRA_TOKEN",
						ProjectKey: "PROJ",
						IssueType:  "Task",
						AutoCreate: true,
					}, nil)
			},
			expectedCode: http.StatusOK,
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
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleTicketingConfigGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)

			if tc.name == "success" {
				var resp TicketingConfigResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.Equal(t, "jira", resp.Provider)
				require.True(t, resp.Enabled)
				require.True(t, resp.SecretConfigured)
				require.True(t, resp.AutoCreate)
			}
		})
	}
}

func TestHandleTicketingConfigUpdate(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:write"}), nil)
	storeMock.On("GetTicketingConfig", context.Background(), "t1").
		Return(models.TicketingConfig{}, store.ErrNotFound)
	storeMock.On("UpsertTicketingConfig", context.Background(), mock.AnythingOfType("*models.TicketingConfig")).
		Return(nil)
	storeMock.On("InsertAuditEvent", context.Background(), mock.AnythingOfType("*models.AdminAuditEvent")).
		Return(nil)

	body := `{"provider":"jira","enabled":true,"base_url":"https://co.atlassian.net","project_key":"PROJ","issue_type":"Task","auto_create":true}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPut,
		strings.NewReader(body),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.AuthorizationHeader, "Bearer admin-key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	err := deps.handleTicketingConfigUpdate(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleTicketingConfigUpdate_InvalidProvider(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:write"}), nil)

	body := `{"provider":"github","enabled":true}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPut,
		strings.NewReader(body),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.AuthorizationHeader, "Bearer admin-key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	err := deps.handleTicketingConfigUpdate(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleTicketingSecretRefSet(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:write"}), nil)
	storeMock.On("GetTicketingConfig", context.Background(), "t1").
		Return(models.TicketingConfig{TenantID: "t1"}, nil)
	storeMock.On("UpsertTicketingConfig", context.Background(), mock.AnythingOfType("*models.TicketingConfig")).
		Return(nil)
	storeMock.On("InsertAuditEvent", context.Background(), mock.AnythingOfType("*models.AdminAuditEvent")).
		Return(nil)

	body := `{"secret_ref":"env://JIRA_API_TOKEN"}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPut,
		strings.NewReader(body),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.AuthorizationHeader, "Bearer admin-key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	err := deps.handleTicketingSecretRefSet(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleTicketLinksList(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:read"}), nil)
	storeMock.On("ListTicketLinks", context.Background(), "t1", 0, 0).
		Return([]models.TicketLink{
			{TicketLinkID: "tl-1", TenantID: "t1", ExternalKey: "PROJ-1", Provider: "jira"},
		}, nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer admin-key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	err := deps.handleTicketLinksList(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var links []models.TicketLink
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &links))
	require.Len(t, links, 1)
	require.Equal(t, "PROJ-1", links[0].ExternalKey)
}

func TestHandleTicketingWebhook_Jira(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTicketLinkByExternalKey", context.Background(), "jira", "PROJ-123").
		Return(models.TicketLink{
			TicketLinkID:      "tl-1",
			TenantID:          "t1",
			ApprovalRequestID: "ar-1",
			Provider:          "jira",
			ExternalKey:       "PROJ-123",
		}, nil)
	storeMock.On("GetTicketingConfig", context.Background(), "t1").
		Return(models.TicketingConfig{}, store.ErrNotFound)
	storeMock.On("ApproveApprovalRequest", context.Background(), "t1", "ar-1", "webhook:jira", mock.Anything, mock.Anything).
		Return(nil)
	storeMock.On("UpdateTicketLinkStatus", context.Background(), "tl-1", "RESOLVED").
		Return(nil)

	jiraPayload := `{"issue":{"key":"PROJ-123","fields":{"status":{"name":"Done"}}}}`
	ctx, _, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		strings.NewReader(jiraPayload),
		testhelpers.Params{Names: []string{"provider"}, Values: []string{"jira"}},
	)

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}, TicketingService: &ticketing.Service{}}
	err := deps.handleTicketingWebhook(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "processed", resp["status"])
	require.Equal(t, "ar-1", resp["approval_request_id"])
}

func TestHandleTicketingWebhook_UnknownTicket(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTicketLinkByExternalKey", context.Background(), "jira", "UNKNOWN-1").
		Return(models.TicketLink{}, store.ErrNotFound)

	jiraPayload := `{"issue":{"key":"UNKNOWN-1","fields":{"status":{"name":"Done"}}}}`
	ctx, _, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		strings.NewReader(jiraPayload),
		testhelpers.Params{Names: []string{"provider"}, Values: []string{"jira"}},
	)

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}, TicketingService: &ticketing.Service{}}
	err := deps.handleTicketingWebhook(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestParseWebhookPayload(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		body       string
		wantKey    string
		wantStatus string
	}{
		{
			name:       "jira",
			provider:   "jira",
			body:       `{"issue":{"key":"PROJ-1","fields":{"status":{"name":"Done"}}}}`,
			wantKey:    "PROJ-1",
			wantStatus: "Done",
		},
		{
			name:       "servicenow",
			provider:   "servicenow",
			body:       `{"number":"INC001","state":"6"}`,
			wantKey:    "INC001",
			wantStatus: "resolved",
		},
		{
			name:       "linear",
			provider:   "linear",
			body:       `{"data":{"identifier":"ENG-42","state":{"name":"Done"}}}`,
			wantKey:    "ENG-42",
			wantStatus: "Done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, status := parseWebhookPayload(tt.provider, []byte(tt.body))
			require.Equal(t, tt.wantKey, key)
			require.Equal(t, tt.wantStatus, status)
		})
	}
}
