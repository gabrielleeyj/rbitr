package classification

import (
	"strings"
)

type Result struct {
	ActionType    string
	ActionRisk    string
	ActionSummary string
}

const (
	RiskLow      = "LOW"
	RiskMedium   = "MEDIUM"
	RiskHigh     = "HIGH"
	RiskCritical = "CRITICAL"
)

func Classify(toolID, method, path, query string, headers map[string]string) Result {
	toolID = strings.ToLower(toolID)
	method = strings.ToUpper(method)

	if toolID == "jira" {
		return classifyJira(method, path)
	}
	if toolID == "mock_internal" {
		return classifyMockInternal(method, path)
	}
	return classifyGeneric(method, path)
}

func classifyGeneric(method, path string) Result {
	actionType := "DATA.READ"
	if method == "POST" || method == "PUT" || method == "PATCH" {
		actionType = "DATA.UPDATE"
	}
	if method == "DELETE" {
		actionType = "DATA.DELETE"
	}
	return Result{
		ActionType:    actionType,
		ActionRisk:    defaultRisk(actionType),
		ActionSummary: method + " " + path,
	}
}

func classifyJira(method, path string) Result {
	if method == "POST" && path == "/rest/api/3/issue" {
		return Result{
			ActionType:    "TICKET.CREATE",
			ActionRisk:    defaultRisk("TICKET.CREATE"),
			ActionSummary: "Create Jira issue",
		}
	}
	if strings.Contains(path, "/comment") {
		return Result{
			ActionType:    "TICKET.COMMENT",
			ActionRisk:    defaultRisk("TICKET.COMMENT"),
			ActionSummary: "Comment on Jira issue",
		}
	}
	return classifyGeneric(method, path)
}

func classifyMockInternal(method, path string) Result {
	if path == "/refund" {
		return Result{
			ActionType:    "PAYMENT.REFUND",
			ActionRisk:    defaultRisk("PAYMENT.REFUND"),
			ActionSummary: "Refund payment",
		}
	}
	if path == "/export_customer_data" {
		return Result{
			ActionType:    "DATA.EXPORT",
			ActionRisk:    defaultRisk("DATA.EXPORT"),
			ActionSummary: "Export customer data",
		}
	}
	if path == "/change_role" {
		return Result{
			ActionType:    "ACCESS.ROLE_CHANGE",
			ActionRisk:    defaultRisk("ACCESS.ROLE_CHANGE"),
			ActionSummary: "Change user role",
		}
	}
	return classifyGeneric(method, path)
}

func defaultRisk(actionType string) string {
	switch actionType {
	case "PAYMENT.REFUND", "ACCESS.ROLE_CHANGE":
		return RiskHigh
	case "DATA.EXPORT", "ACCESS.GRANT", "DATA.BULK_EXPORT", "DATA.DELETE", "CRM.DELETE":
		return RiskCritical
	case "TICKET.CREATE", "TICKET.COMMENT", "TICKET.UPDATE", "DATA.READ", "DATA.QUERY", "CRM.READ":
		return RiskLow
	default:
		return RiskMedium
	}
}
