package classification

import "testing"

func TestClassifyMockInternalRefund(t *testing.T) {
	result := Classify("mock_internal", "POST", "/refund", "", nil)
	if result.ActionType != "PAYMENT.REFUND" {
		t.Fatalf("expected PAYMENT.REFUND, got %s", result.ActionType)
	}
	if result.ActionRisk != RiskHigh {
		t.Fatalf("expected risk %s, got %s", RiskHigh, result.ActionRisk)
	}
}

func TestClassifyJiraCreate(t *testing.T) {
	result := Classify("jira", "POST", "/rest/api/3/issue", "", nil)
	if result.ActionType != "TICKET.CREATE" {
		t.Fatalf("expected TICKET.CREATE, got %s", result.ActionType)
	}
}
