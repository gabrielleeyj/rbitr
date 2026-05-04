package audit

import "strings"

const fieldActivePolicyVersion = "active_policy_version"

//nolint:gochecknoglobals // centralized allowlist used by redaction helpers.
var resourceAllowlist = map[string]map[string]struct{}{
	"TENANT.CONFIG": {
		"name":     {},
		"key_hash": {},
	},
	"TOOL": {
		"base_url":  {},
		"auth_type": {},
		"auth_set":  {},
	},
	"POLICY.VERSION": {
		"policy_version": {},
		"created_by":     {},
		"notes":          {},
		"rego_sha256":    {},
	},
	"POLICY.ACTIVE": {
		fieldActivePolicyVersion: {},
	},
	"RISK_OVERRIDE": {
		"action_type": {},
		"action_risk": {},
	},
	"TENANT.NOTIFICATIONS": {
		"slack_webhook_enabled":         {},
		"slack_bot_enabled":             {},
		"email_enabled":                 {},
		"email_provider":                {},
		"email_from":                    {},
		"email_region":                  {},
		"email_domain":                  {},
		"email_default_mailing_list_id": {},
		"notify_approval_expiring":      {},
		"notify_token_abuse":            {},
		"notify_policy_invalid":         {},
	},
	"MAILING_LIST": {
		"name":         {},
		"description":  {},
		"member_count": {},
	},
	"SETTINGS": {
		"value": {},
	},
	"APPROVAL.REQUEST": {
		"status":           {},
		"decided_at":       {},
		"decided_by":       {},
		"decision_comment": {},
	},
}

func RedactPayload(resourceType string, payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	allowlist, ok := resourceAllowlist[strings.ToUpper(resourceType)]
	if !ok {
		return map[string]any{}
	}
	redacted := map[string]any{}
	for key, value := range payload {
		if _, ok := allowlist[key]; !ok {
			continue
		}
		redacted[key] = value
	}
	return redacted
}
