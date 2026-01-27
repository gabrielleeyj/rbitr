package admin

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleAuditExportJSON(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:read"}), nil)
	storeMock.On("ListAuditEventsExport", context.Background(), "t1", 1000, 0, "", "", "", mock.Anything, mock.Anything).
		Return([]models.AdminAuditEvent{{AuditEventID: "ae_1", TenantID: "t1"}}, nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer key")

	deps := Dependencies{Store: storeMock}
	err := deps.handleAuditExport(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "ae_1")
}

func TestHandleAuditExportCSV(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:read"}), nil)
	storeMock.On("ListAuditEventsExport", context.Background(), "t1", 1000, 0, "", "", "", mock.Anything, mock.Anything).
		Return([]models.AdminAuditEvent{{AuditEventID: "ae_1", TenantID: "t1"}}, nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer key")
	req.URL.RawQuery = "format=csv"

	deps := Dependencies{Store: storeMock}
	err := deps.handleAuditExport(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "audit_event_id")
}

func TestHandleAuditExportIncludeDetails(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:read"}), nil)
	storeMock.On("ListAuditEventsExport", context.Background(), "t1", 1000, 0, "", "", "", mock.Anything, mock.Anything).
		Return([]models.AdminAuditEvent{{AuditEventID: "ae_1", TenantID: "t1", Before: []byte(`{"ok":true}`)}}, nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer key")
	req.URL.RawQuery = "include_details=true"

	deps := Dependencies{Store: storeMock}
	err := deps.handleAuditExport(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "ok")
}

func TestWriteAuditCSVDetails(t *testing.T) {
	events := []models.AdminAuditEvent{
		{
			AuditEventID: "ae_1",
			TenantID:     "t1",
			StreamID:     "t1",
			EventHash:    "hash",
			PrevHash:     "prev",
			ActorType:    "admin_key",
			ActorID:      "admin",
			Action:       "ACTION",
			ResourceType: "RESOURCE",
			ResourceID:   "res",
			RequestID:    "req",
			IP:           "127.0.0.1",
			UserAgent:    "agent",
			Before:       []byte(`{"ok":true}`),
			After:        []byte(`{"ok":false}`),
			CreatedAt:    time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC),
		},
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	err := writeAuditCSV(writer, events, true)
	writer.Flush()
	require.NoError(t, err)
	output := buf.String()
	require.True(t, strings.Contains(output, "{\"ok\":true}"))
}
