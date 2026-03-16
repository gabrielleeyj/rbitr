package public

import (
	"log/slog"
	"net/http"

	"github.com/gabrielleeyj/rbitr/internal/auth"
)

// provenanceContext holds the provenance chain state extracted from an incoming request.
type provenanceContext struct {
	SourceTenantID   string
	SourceDecisionID string
	ChainDepth       int
}

// extractProvenance reads the X-Provenance-Chain header, validates it, and
// returns the provenance context. Returns a zero-value context if the header
// is absent or the feature is disabled.
func (d *Dependencies) extractProvenance(r *http.Request) provenanceContext {
	if d.ProvenanceManager == nil {
		return provenanceContext{}
	}

	token := auth.ProvenanceFromRequest(r)
	if token == "" {
		return provenanceContext{}
	}

	claims, err := d.ProvenanceManager.ValidateToken(token)
	if err != nil {
		slog.Warn("invalid provenance token",
			"error", err,
		)
		return provenanceContext{}
	}

	return provenanceContext{
		SourceTenantID:   claims.SourceTenantID,
		SourceDecisionID: claims.SourceDecisionID,
		ChainDepth:       claims.ChainDepth,
	}
}

// injectProvenanceInput adds cross-tenant provenance fields to the policy input
// when a valid provenance context is present.
func injectProvenanceInput(policyInput map[string]any, prov provenanceContext) map[string]any {
	if prov.SourceTenantID == "" {
		return policyInput
	}

	// Create a new map to avoid mutating the original.
	result := make(map[string]any, len(policyInput)+2) //nolint:mnd // source_tenant_id + chain_depth
	for k, v := range policyInput {
		result[k] = v
	}
	result["source_tenant_id"] = prov.SourceTenantID
	result["chain_depth"] = prov.ChainDepth

	return result
}
