package store

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func TestMockStoreAPIRunAndReturn(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("GetTenantByKeyHash", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetTenantByKeyHash(ctx, "hash")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) (models.Tenant, error) {
			return models.Tenant{TenantID: "t1", Enabled: true}, nil
		})
		_, _ = storeMock.GetTenantByKeyHash(ctx, "hash")
	})

	t.Run("GetAdminKeyByHash", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetAdminKeyByHash(ctx, "hash")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) (models.AdminKey, error) {
			return models.AdminKey{AdminKeyID: "a1"}, nil
		})
		_, _ = storeMock.GetAdminKeyByHash(ctx, "hash")
	})

	t.Run("ListTenants", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListTenants(ctx)
		call.Run(func(context.Context) {})
		call.RunAndReturn(func(context.Context) ([]models.TenantSummary, error) {
			return []models.TenantSummary{}, nil
		})
		_, _ = storeMock.ListTenants(ctx)
	})

	t.Run("GetTenant", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetTenant(ctx, "t1")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) (models.TenantSummary, error) {
			return models.TenantSummary{TenantID: "t1"}, nil
		})
		_, _ = storeMock.GetTenant(ctx, "t1")
	})

	t.Run("GetTenantKeyHash", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetTenantKeyHash(ctx, "t1")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) (string, error) {
			return "hash", nil
		})
		_, _ = storeMock.GetTenantKeyHash(ctx, "t1")
	})

	t.Run("GetTool", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetTool(ctx, "t1", "tool")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) (models.Tool, error) {
			return models.Tool{ToolID: "tool"}, nil
		})
		_, _ = storeMock.GetTool(ctx, "t1", "tool")
	})

	t.Run("ListTools", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListTools(ctx, "t1", false, false)
		call.Run(func(context.Context, string, bool, bool) {})
		call.RunAndReturn(func(context.Context, string, bool, bool) ([]models.Tool, error) {
			return []models.Tool{}, nil
		})
		_, _ = storeMock.ListTools(ctx, "t1", false, false)
	})

	t.Run("GetPolicy", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetPolicy(ctx, "t1")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) (models.Policy, error) {
			return models.Policy{PolicyID: "p1"}, nil
		})
		_, _ = storeMock.GetPolicy(ctx, "t1")
	})

	t.Run("GetTenantConfig", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetTenantConfig(ctx, "t1")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) (models.TenantConfig, error) {
			return models.TenantConfig{TenantID: "t1"}, nil
		})
		_, _ = storeMock.GetTenantConfig(ctx, "t1")
	})

	t.Run("ListPolicyVersions", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListPolicyVersions(ctx, "t1")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) ([]models.PolicyVersion, error) {
			return []models.PolicyVersion{}, nil
		})
		_, _ = storeMock.ListPolicyVersions(ctx, "t1")
	})

	t.Run("GetPolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetPolicyVersion(ctx, "t1", "p_v1")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) (models.PolicyVersion, error) {
			return models.PolicyVersion{PolicyVersion: "p_v1"}, nil
		})
		_, _ = storeMock.GetPolicyVersion(ctx, "t1", "p_v1")
	})

	t.Run("CreatePolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().CreatePolicyVersion(ctx, "t1", "p_v2", "rego", "admin", "notes")
		call.Run(func(context.Context, string, string, string, string, string) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, string) error {
			return nil
		})
		_ = storeMock.CreatePolicyVersion(ctx, "t1", "p_v2", "rego", "admin", "notes")
	})

	t.Run("PublishPolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().PublishPolicyVersion(ctx, "t1", "p_v2")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) error {
			return nil
		})
		_ = storeMock.PublishPolicyVersion(ctx, "t1", "p_v2")
	})

	t.Run("RollbackPolicyVersion", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().RollbackPolicyVersion(ctx, "t1", "p_v1")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) error {
			return nil
		})
		_ = storeMock.RollbackPolicyVersion(ctx, "t1", "p_v1")
	})

	t.Run("GetRiskOverride", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetRiskOverride(ctx, "t1", "TYPE")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) (string, error) {
			return "HIGH", nil
		})
		_, _ = storeMock.GetRiskOverride(ctx, "t1", "TYPE")
	})

	t.Run("ListRiskOverrides", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListRiskOverrides(ctx, "t1")
		call.Run(func(context.Context, string) {})
		call.RunAndReturn(func(context.Context, string) ([]models.RiskOverride, error) {
			return []models.RiskOverride{}, nil
		})
		_, _ = storeMock.ListRiskOverrides(ctx, "t1")
	})

	t.Run("DeleteRiskOverride", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().DeleteRiskOverride(ctx, "t1", "TYPE")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) error {
			return nil
		})
		_ = storeMock.DeleteRiskOverride(ctx, "t1", "TYPE")
	})

	t.Run("InsertADR", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().InsertADR(ctx, &models.ActionDecisionRecord{DecisionID: "d1"})
		call.Run(func(context.Context, *models.ActionDecisionRecord) {})
		call.RunAndReturn(func(context.Context, *models.ActionDecisionRecord) error {
			return nil
		})
		_ = storeMock.InsertADR(ctx, &models.ActionDecisionRecord{DecisionID: "d1"})
	})

	t.Run("InsertApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().InsertApprovalRequest(ctx, &models.ApprovalRequest{ApprovalRequestID: "ar1"})
		call.Run(func(context.Context, *models.ApprovalRequest) {})
		call.RunAndReturn(func(context.Context, *models.ApprovalRequest) error {
			return nil
		})
		_ = storeMock.InsertApprovalRequest(ctx, &models.ApprovalRequest{ApprovalRequestID: "ar1"})
	})

	t.Run("ListApprovalRequests", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListApprovalRequests(ctx, "t1", "PENDING", 10, 0)
		call.Run(func(context.Context, string, string, int, int) {})
		call.RunAndReturn(func(context.Context, string, string, int, int) ([]models.ApprovalRequest, error) {
			return []models.ApprovalRequest{}, nil
		})
		_, _ = storeMock.ListApprovalRequests(ctx, "t1", "PENDING", 10, 0)
	})

	t.Run("GetApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetApprovalRequest(ctx, "t1", "ar1")
		call.Run(func(context.Context, string, string) {})
		call.RunAndReturn(func(context.Context, string, string) (models.ApprovalRequest, error) {
			return models.ApprovalRequest{ApprovalRequestID: "ar1"}, nil
		})
		_, _ = storeMock.GetApprovalRequest(ctx, "t1", "ar1")
	})

	t.Run("ApproveApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ApproveApprovalRequest(ctx, "t1", "ar1", "admin", "ok", now)
		call.Run(func(context.Context, string, string, string, string, time.Time) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, time.Time) error {
			return nil
		})
		_ = storeMock.ApproveApprovalRequest(ctx, "t1", "ar1", "admin", "ok", now)
	})

	t.Run("DenyApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().DenyApprovalRequest(ctx, "t1", "ar1", "admin", "no", now)
		call.Run(func(context.Context, string, string, string, string, time.Time) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, time.Time) error {
			return nil
		})
		_ = storeMock.DenyApprovalRequest(ctx, "t1", "ar1", "admin", "no", now)
	})

	t.Run("RevokeApprovalRequest", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().RevokeApprovalRequest(ctx, "t1", "ar1", "admin", "revoke", now)
		call.Run(func(context.Context, string, string, string, string, time.Time) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, time.Time) error {
			return nil
		})
		_ = storeMock.RevokeApprovalRequest(ctx, "t1", "ar1", "admin", "revoke", now)
	})

	t.Run("MarkApprovalExecuted", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().MarkApprovalExecuted(ctx, "t1", "ar1", "req", "dec", now)
		call.Run(func(context.Context, string, string, string, string, time.Time) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, time.Time) error {
			return nil
		})
		_ = storeMock.MarkApprovalExecuted(ctx, "t1", "ar1", "req", "dec", now)
	})

	t.Run("MarkApprovalExpired", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().MarkApprovalExpired(ctx, "t1", "ar1", now)
		call.Run(func(context.Context, string, string, time.Time) {})
		call.RunAndReturn(func(context.Context, string, string, time.Time) error {
			return nil
		})
		_ = storeMock.MarkApprovalExpired(ctx, "t1", "ar1", now)
	})

	t.Run("ListEvidence", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListEvidence(ctx, "t1", 50)
		call.Run(func(context.Context, string, int) {})
		call.RunAndReturn(func(context.Context, string, int) ([]models.ActionDecisionRecord, error) {
			return []models.ActionDecisionRecord{}, nil
		})
		_, _ = storeMock.ListEvidence(ctx, "t1", 50)
	})

	t.Run("ListEvidenceFiltered", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListEvidenceFiltered(ctx, "t1", "ALLOW", "", "", &now, 10)
		call.Run(func(context.Context, string, string, string, string, *time.Time, int) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, *time.Time, int) ([]models.ActionDecisionRecord, error) {
			return []models.ActionDecisionRecord{}, nil
		})
		_, _ = storeMock.ListEvidenceFiltered(ctx, "t1", "ALLOW", "", "", &now, 10)
	})

	t.Run("UpdateTenantConfig", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().UpdateTenantConfig(ctx, "t1", "Name", "key")
		call.Run(func(context.Context, string, string, string) {})
		call.RunAndReturn(func(context.Context, string, string, string) error {
			return nil
		})
		_ = storeMock.UpdateTenantConfig(ctx, "t1", "Name", "key")
	})

	t.Run("UpdateToolConfig", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().UpdateToolConfig(ctx, "t1", "tool", "http://example", "bearer", "token")
		call.Run(func(context.Context, string, string, string, string, string) {})
		call.RunAndReturn(func(context.Context, string, string, string, string, string) error {
			return nil
		})
		_ = storeMock.UpdateToolConfig(ctx, "t1", "tool", "http://example", "bearer", "token")
	})

	t.Run("UpdatePolicy", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().UpdatePolicy(ctx, "t1", "rego", "p_v1")
		call.Run(func(context.Context, string, string, string) {})
		call.RunAndReturn(func(context.Context, string, string, string) error {
			return nil
		})
		_ = storeMock.UpdatePolicy(ctx, "t1", "rego", "p_v1")
	})

	t.Run("UpdateRiskOverride", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().UpdateRiskOverride(ctx, "t1", "TYPE", "HIGH")
		call.Run(func(context.Context, string, string, string) {})
		call.RunAndReturn(func(context.Context, string, string, string) error {
			return nil
		})
		_ = storeMock.UpdateRiskOverride(ctx, "t1", "TYPE", "HIGH")
	})

	t.Run("MarkBootstrapComplete", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().MarkBootstrapComplete(ctx)
		call.Run(func(context.Context) {})
		call.RunAndReturn(func(context.Context) error {
			return nil
		})
		_ = storeMock.MarkBootstrapComplete(ctx)
	})

	t.Run("GetBootstrapComplete", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetBootstrapComplete(ctx)
		call.Run(func(context.Context) {})
		call.RunAndReturn(func(context.Context) (bool, error) {
			return true, nil
		})
		_, _ = storeMock.GetBootstrapComplete(ctx)
	})

	t.Run("SetAdminWriteLock", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().SetAdminWriteLock(ctx, true)
		call.Run(func(context.Context, bool) {})
		call.RunAndReturn(func(context.Context, bool) error {
			return nil
		})
		_ = storeMock.SetAdminWriteLock(ctx, true)
	})

	t.Run("GetAdminWriteLock", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetAdminWriteLock(ctx)
		call.Run(func(context.Context) {})
		call.RunAndReturn(func(context.Context) (bool, error) {
			return true, nil
		})
		_, _ = storeMock.GetAdminWriteLock(ctx)
	})

	t.Run("SetDefaultApprovalTTLSeconds", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().SetDefaultApprovalTTLSeconds(ctx, 900)
		call.Run(func(context.Context, int) {})
		call.RunAndReturn(func(context.Context, int) error {
			return nil
		})
		_ = storeMock.SetDefaultApprovalTTLSeconds(ctx, 900)
	})

	t.Run("GetDefaultApprovalTTLSeconds", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().GetDefaultApprovalTTLSeconds(ctx)
		call.Run(func(context.Context) {})
		call.RunAndReturn(func(context.Context) (int, error) {
			return 900, nil
		})
		_, _ = storeMock.GetDefaultApprovalTTLSeconds(ctx)
	})

	t.Run("ListAuditEvents", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().ListAuditEvents(ctx, "t1", 10, 0, "", "", "", (*time.Time)(nil), (*time.Time)(nil))
		call.Run(func(context.Context, string, int, int, string, string, string, *time.Time, *time.Time) {})
		call.RunAndReturn(func(context.Context, string, int, int, string, string, string, *time.Time, *time.Time) ([]models.AdminAuditEvent, error) {
			return []models.AdminAuditEvent{}, nil
		})
		_, _ = storeMock.ListAuditEvents(ctx, "t1", 10, 0, "", "", "", nil, nil)
	})

	t.Run("InsertAuditEvent", func(t *testing.T) {
		storeMock := NewMockStoreAPI(t)
		call := storeMock.EXPECT().InsertAuditEvent(ctx, &models.AdminAuditEvent{AuditEventID: "ae_1"})
		call.Run(func(context.Context, *models.AdminAuditEvent) {})
		call.RunAndReturn(func(context.Context, *models.AdminAuditEvent) error {
			return nil
		})
		_ = storeMock.InsertAuditEvent(ctx, &models.AdminAuditEvent{AuditEventID: "ae_1"})
	})
}
