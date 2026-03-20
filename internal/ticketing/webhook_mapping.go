package ticketing

// ApprovalAction represents the action to take on an rbitr approval.
type ApprovalAction string

const (
	ActionApprove ApprovalAction = "approve"
	ActionDeny    ApprovalAction = "deny"
	ActionNone    ApprovalAction = "none"
)

// MapWebhookStatus maps a provider-specific ticket status transition to an rbitr approval action.
func MapWebhookStatus(provider, status string) ApprovalAction {
	switch provider {
	case ProviderJira:
		return mapJiraStatus(status)
	case ProviderServiceNow:
		return mapServiceNowStatus(status)
	case ProviderLinear:
		return mapLinearStatus(status)
	default:
		return ActionNone
	}
}

func mapJiraStatus(status string) ApprovalAction {
	switch status {
	case "Done", "Closed", "Resolved":
		return ActionApprove
	case "Rejected", "Won't Do", "Declined":
		return ActionDeny
	default:
		return ActionNone
	}
}

func mapServiceNowStatus(status string) ApprovalAction {
	switch status {
	case "resolved", "closed":
		return ActionApprove
	case "canceled", "rejected":
		return ActionDeny
	default:
		return ActionNone
	}
}

func mapLinearStatus(status string) ApprovalAction {
	switch status {
	case "Done", "Completed":
		return ActionApprove
	case "Canceled", "Cancelled":
		return ActionDeny
	default:
		return ActionNone
	}
}
