package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/policy/compiler"
	"github.com/gabrielleeyj/rbitr/internal/policy/coverage"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

// PolicyStructuredCreateRequest saves a policy authored through the structured
// rule builder. The gateway compiles it to Rego on save; OPA remains the engine.
type PolicyStructuredCreateRequest struct {
	PolicyVersion string                    `json:"policy_version"`
	Notes         string                    `json:"notes"`
	Structured    compiler.StructuredPolicy `json:"structured"`
	Publish       bool                      `json:"publish"`
}

// PolicyStructuredResponse returns a stored structured policy, or signals that
// the version was authored as raw Rego and should be edited in advanced mode.
type PolicyStructuredResponse struct {
	PolicyVersion string                     `json:"policy_version"`
	AuthoringMode string                     `json:"authoring_mode"`
	AdvancedMode  bool                       `json:"advanced_mode"`
	Structured    *compiler.StructuredPolicy `json:"structured,omitempty"`
}

func (d *Dependencies) handlePolicyStructuredCreate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesWrite)
	if err != nil {
		return err
	}

	var payload PolicyStructuredCreateRequest
	if decodeErr := json.NewDecoder(c.Request().Body).Decode(&payload); decodeErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.PolicyVersion == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "policy_version required"})
	}

	regoModule, err := compiler.Compile(&payload.Structured)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":     "structured policy invalid",
			fieldDetail: err.Error(),
		})
	}
	if _, prepErr := opa.PrepareQuery(c.Request().Context(), regoModule); prepErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":     "compiled policy failed to compile",
			fieldDetail: prepErr.Error(),
		})
	}

	structuredJSON, err := json.Marshal(payload.Structured)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode structured policy"})
	}

	tenantID := c.Param(fieldTenantID)
	if err := d.Store.CreatePolicyVersionStructured(c.Request().Context(), tenantID, payload.PolicyVersion, regoModule, structuredJSON, adminKey.AdminKeyID, payload.Notes); err != nil {
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": errAdminWritesLocked})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to create policy",
			fieldDetail: err.Error(),
		})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.CREATE", "POLICY.VERSION", payload.PolicyVersion, nil, map[string]any{
		fieldPolicyVersion:  payload.PolicyVersion,
		"created_by":        adminKey.AdminKeyID,
		"notes":             payload.Notes,
		"authoring_mode":    store.AuthoringModeStructured,
		"structured_sha256": utils.HashString(string(structuredJSON)),
		"rego_sha256":       utils.HashString(regoModule),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit policy create",
			fieldDetail: err.Error(),
		})
	}

	if payload.Publish {
		if err := d.publishStructuredVersion(c, adminKey, tenantID, payload.PolicyVersion); err != nil {
			return err
		}
	}
	return c.NoContent(http.StatusCreated)
}

// publishStructuredVersion publishes a freshly created structured version and
// invalidates caches, mirroring handlePolicyPublish's side effects.
func (d *Dependencies) publishStructuredVersion(c *echo.Context, adminKey models.AdminKey, tenantID, version string) error {
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err := d.Store.PublishPolicyVersion(c.Request().Context(), tenantID, version); err != nil {
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": errAdminWritesLocked})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to publish policy",
			fieldDetail: err.Error(),
		})
	}
	d.invalidateTenantCaches(tenantID)
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	return d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", version, map[string]any{
		auditFieldActivePolicyVer: before.ActivePolicyVersion,
	}, map[string]any{
		auditFieldActivePolicyVer: after.ActivePolicyVersion,
	})
}

func (d *Dependencies) handlePolicyStructuredGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	version := c.Param(fieldPolicyVersion)
	item, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errPolicyVersionNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load policy"})
	}

	resp := PolicyStructuredResponse{PolicyVersion: item.PolicyVersion, AuthoringMode: item.AuthoringMode}
	if item.AuthoringMode != store.AuthoringModeStructured || len(item.StructuredJSON) == 0 {
		resp.AdvancedMode = true
		return c.JSON(http.StatusOK, resp)
	}
	var structured compiler.StructuredPolicy
	if err := json.Unmarshal(item.StructuredJSON, &structured); err != nil {
		// Stored JSON is unreadable; fall back to advanced mode rather than erroring.
		resp.AdvancedMode = true
		return c.JSON(http.StatusOK, resp)
	}
	resp.Structured = &structured
	return c.JSON(http.StatusOK, resp)
}

func (d *Dependencies) handlePolicyCoverage(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesRead); err != nil {
		return err
	}

	windowDays, err := parseCoverageWindow(c.QueryParam("window_days"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid window_days"})
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}

	tenantID := c.Param(fieldTenantID)
	report, err := coverage.BuildReport(c.Request().Context(), d.Store, tenantID, windowDays, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to build coverage report",
			fieldDetail: err.Error(),
		})
	}
	return c.JSON(http.StatusOK, report)
}

func parseCoverageWindow(value string) (int, error) {
	if value == "" {
		return coverage.DefaultWindowDays, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}
