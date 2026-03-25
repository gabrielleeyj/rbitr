package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func TestMockStoreAPIExpectations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("GetTenantByKeyHash", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetTenantByKeyHash(ctx, "hash").Return(models.Tenant{TenantID: "t1", Enabled: true}, nil)
		_, _ = storeMock.GetTenantByKeyHash(ctx, "hash")
	})

	t.Run("GetAdminKeyByHash", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetAdminKeyByHash(ctx, "hash").Return(models.AdminKey{AdminKeyID: "a1"}, nil)
		_, _ = storeMock.GetAdminKeyByHash(ctx, "hash")
	})

	t.Run("ListTenants", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListTenants(ctx).Return([]models.TenantSummary{}, nil)
		_, _ = storeMock.ListTenants(ctx)
	})

	t.Run("GetTenant", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetTenant(ctx, "t1").Return(models.TenantSummary{TenantID: "t1"}, nil)
		_, _ = storeMock.GetTenant(ctx, "t1")
	})

	t.Run("GetTenantKeyHash", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetTenantKeyHash(ctx, "t1").Return("hash", nil)
		_, _ = storeMock.GetTenantKeyHash(ctx, "t1")
	})

	t.Run("GetTool", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetTool(ctx, "t1", "tool").Return(models.Tool{ToolID: "tool"}, nil)
		_, _ = storeMock.GetTool(ctx, "t1", "tool")
	})

	t.Run("ListTools", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListTools(ctx, "t1", false).Return(nil, nil)
		_, _ = storeMock.ListTools(ctx, "t1", false)
	})

	t.Run("GetPolicy", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetPolicy(ctx, "t1").Return(models.Policy{PolicyID: "p1"}, nil)
		_, _ = storeMock.GetPolicy(ctx, "t1")
	})

	t.Run("GetTenantConfig", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetTenantConfig(ctx, "t1").Return(models.TenantConfig{TenantID: "t1"}, nil)
		_, _ = storeMock.GetTenantConfig(ctx, "t1")
	})

	t.Run("ListPolicyVersions", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListPolicyVersions(ctx, "t1").Return([]models.PolicyVersion{}, nil)
		_, _ = storeMock.ListPolicyVersions(ctx, "t1")
	})

	t.Run("GetPolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetPolicyVersion(ctx, "t1", "p_v1").Return(models.PolicyVersion{PolicyVersion: "p_v1"}, nil)
		_, _ = storeMock.GetPolicyVersion(ctx, "t1", "p_v1")
	})

	t.Run("CreatePolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().CreatePolicyVersion(ctx, "t1", "p_v2", "rego", "admin", "notes").Return(nil)
		_ = storeMock.CreatePolicyVersion(ctx, "t1", "p_v2", "rego", "admin", "notes")
	})

	t.Run("PublishPolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().PublishPolicyVersion(ctx, "t1", "p_v2").Return(nil)
		_ = storeMock.PublishPolicyVersion(ctx, "t1", "p_v2")
	})

	t.Run("RollbackPolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().RollbackPolicyVersion(ctx, "t1", "p_v1").Return(nil)
		_ = storeMock.RollbackPolicyVersion(ctx, "t1", "p_v1")
	})

	t.Run("GetRiskOverride", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetRiskOverride(ctx, "t1", "TYPE").Return("HIGH", nil)
		_, _ = storeMock.GetRiskOverride(ctx, "t1", "TYPE")
	})

	t.Run("ListRiskOverrides", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListRiskOverrides(ctx, "t1").Return([]models.RiskOverride{}, nil)
		_, _ = storeMock.ListRiskOverrides(ctx, "t1")
	})

	t.Run("DeleteRiskOverride", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().DeleteRiskOverride(ctx, "t1", "TYPE").Return(nil)
		_ = storeMock.DeleteRiskOverride(ctx, "t1", "TYPE")
	})

	t.Run("InsertADR", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().InsertADR(ctx, mock.Anything).Return(nil)
		_ = storeMock.InsertADR(ctx, &models.ActionDecisionRecord{DecisionID: "d1"})
	})

	t.Run("InsertApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().InsertApprovalRequest(ctx, mock.Anything).Return(nil)
		_ = storeMock.InsertApprovalRequest(ctx, &models.ApprovalRequest{ApprovalRequestID: "ar1"})
	})

	t.Run("ListApprovalRequests", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListApprovalRequests(ctx, "t1", "PENDING", 10, 0).Return([]models.ApprovalRequest{}, nil)
		_, _ = storeMock.ListApprovalRequests(ctx, "t1", "PENDING", 10, 0)
	})

	t.Run("GetApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetApprovalRequest(ctx, "t1", "ar1").Return(models.ApprovalRequest{ApprovalRequestID: "ar1"}, nil)
		_, _ = storeMock.GetApprovalRequest(ctx, "t1", "ar1")
	})

	t.Run("ApproveApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ApproveApprovalRequest(ctx, "t1", "ar1", "admin", "ok", now).Return(nil)
		_ = storeMock.ApproveApprovalRequest(ctx, "t1", "ar1", "admin", "ok", now)
	})

	t.Run("DenyApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().DenyApprovalRequest(ctx, "t1", "ar1", "admin", "no", now).Return(nil)
		_ = storeMock.DenyApprovalRequest(ctx, "t1", "ar1", "admin", "no", now)
	})

	t.Run("RevokeApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().RevokeApprovalRequest(ctx, "t1", "ar1", "admin", "revoke", now).Return(nil)
		_ = storeMock.RevokeApprovalRequest(ctx, "t1", "ar1", "admin", "revoke", now)
	})

	t.Run("MarkApprovalExecuted", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().MarkApprovalExecuted(ctx, "t1", "ar1", "req", "dec", now).Return(nil)
		_ = storeMock.MarkApprovalExecuted(ctx, "t1", "ar1", "req", "dec", now)
	})

	t.Run("MarkApprovalExpired", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().MarkApprovalExpired(ctx, "t1", "ar1", now).Return(nil)
		_ = storeMock.MarkApprovalExpired(ctx, "t1", "ar1", now)
	})

	t.Run("ListEvidence", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListEvidence(ctx, "t1", 50).Return([]models.ActionDecisionRecord{}, nil)
		_, _ = storeMock.ListEvidence(ctx, "t1", 50)
	})

	t.Run("ListEvidenceFiltered", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListEvidenceFiltered(ctx, "t1", "ALLOW", "", "", mock.Anything, 10).
			Return([]models.ActionDecisionRecord{}, nil)
		_, _ = storeMock.ListEvidenceFiltered(ctx, "t1", "ALLOW", "", "", &now, 10)
	})

	t.Run("UpdateTenantConfig", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().UpdateTenantConfig(ctx, "t1", "Name", "key").Return(nil)
		_ = storeMock.UpdateTenantConfig(ctx, "t1", "Name", "key")
	})

	t.Run("UpdateToolConfig", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().UpdateToolConfig(ctx, "t1", "tool", "http://example", "bearer", "token").Return(nil)
		_ = storeMock.UpdateToolConfig(ctx, "t1", "tool", "http://example", "bearer", "token")
	})

	t.Run("UpdatePolicy", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().UpdatePolicy(ctx, "t1", "rego", "p_v1").Return(nil)
		_ = storeMock.UpdatePolicy(ctx, "t1", "rego", "p_v1")
	})

	t.Run("UpdateRiskOverride", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().UpdateRiskOverride(ctx, "t1", "TYPE", "HIGH").Return(nil)
		_ = storeMock.UpdateRiskOverride(ctx, "t1", "TYPE", "HIGH")
	})

	t.Run("MarkBootstrapComplete", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().MarkBootstrapComplete(ctx).Return(nil)
		_ = storeMock.MarkBootstrapComplete(ctx)
	})

	t.Run("GetBootstrapComplete", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetBootstrapComplete(ctx).Return(true, nil)
		_, _ = storeMock.GetBootstrapComplete(ctx)
	})

	t.Run("SetAdminWriteLock", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().SetAdminWriteLock(ctx, true).Return(nil)
		_ = storeMock.SetAdminWriteLock(ctx, true)
	})

	t.Run("GetAdminWriteLock", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetAdminWriteLock(ctx).Return(true, nil)
		_, _ = storeMock.GetAdminWriteLock(ctx)
	})

	t.Run("SetDefaultApprovalTTLSeconds", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().SetDefaultApprovalTTLSeconds(ctx, 900).Return(nil)
		_ = storeMock.SetDefaultApprovalTTLSeconds(ctx, 900)
	})

	t.Run("GetDefaultApprovalTTLSeconds", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().GetDefaultApprovalTTLSeconds(ctx).Return(900, nil)
		_, _ = storeMock.GetDefaultApprovalTTLSeconds(ctx)
	})

	t.Run("ListAuditEvents", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().ListAuditEvents(ctx, "t1", 10, 0, "", "", "", (*time.Time)(nil), (*time.Time)(nil)).Return([]models.AdminAuditEvent{}, nil)
		_, _ = storeMock.ListAuditEvents(ctx, "t1", 10, 0, "", "", "", nil, nil)
	})

	t.Run("InsertAuditEvent", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		storeMock.EXPECT().InsertAuditEvent(ctx, mock.Anything).Return(nil)
		_ = storeMock.InsertAuditEvent(ctx, &models.AdminAuditEvent{AuditEventID: "ae_1"})
	})
}
