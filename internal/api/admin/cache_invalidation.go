package admin

func (d Dependencies) invalidateTenantCaches(tenantID string) {
	if tenantID == "" {
		return
	}
	prefix := tenantID + ":"
	if d.ToolCache != nil {
		d.ToolCache.InvalidatePrefix(prefix)
	}
	if d.RiskCache != nil {
		d.RiskCache.InvalidatePrefix(prefix)
	}
}
