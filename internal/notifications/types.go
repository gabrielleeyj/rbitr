package notifications

import "sort"

// EventTypes returns the supported notification event types.
func EventTypes() []string {
	types := []string{
		EventApprovalExpiring,
		EventApprovalExpired,
		EventTokenAbuse,
		EventPolicyInvalidOutput,
		EventPolicyEvalError,
		"NOTIFICATIONS.TEST",
	}
	sort.Strings(types)
	return types
}

// Severities returns the supported notification severities.
func Severities() []string {
	values := []string{"INFO", SeverityWarn, SeverityCritical}
	sort.Strings(values)
	return values
}
