package ticketing

const (
	ProviderJira       = "jira"
	ProviderServiceNow = "servicenow"
	ProviderLinear     = "linear"

	TicketStatusOpen       = "OPEN"
	TicketStatusInProgress = "IN_PROGRESS"
	TicketStatusResolved   = "RESOLVED"
	TicketStatusClosed     = "CLOSED"

	riskCritical = "CRITICAL"
	riskHigh     = "HIGH"
	riskMedium   = "MEDIUM"
	riskLow      = "LOW"

	priorityHigh   = "High"
	priorityMedium = "Medium"
	priorityLow    = "Low"
)

func MapPriority(provider, risk string) string {
	switch provider {
	case ProviderJira:
		return jiraPriority(risk)
	case ProviderServiceNow:
		return servicenowPriority(risk)
	case ProviderLinear:
		return linearPriority(risk)
	default:
		return risk
	}
}

func jiraPriority(risk string) string {
	switch risk {
	case riskCritical:
		return "Highest"
	case riskHigh:
		return priorityHigh
	case riskMedium:
		return priorityMedium
	case riskLow:
		return priorityLow
	default:
		return priorityMedium
	}
}

func servicenowPriority(risk string) string {
	switch risk {
	case riskCritical:
		return "1"
	case riskHigh:
		return "2"
	case riskMedium:
		return "3"
	case riskLow:
		return "4"
	default:
		return "3"
	}
}

func linearPriority(risk string) string {
	switch risk {
	case riskCritical:
		return "Urgent"
	case riskHigh:
		return priorityHigh
	case riskMedium:
		return priorityMedium
	case riskLow:
		return priorityLow
	default:
		return priorityMedium
	}
}
