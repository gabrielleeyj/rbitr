package classification

import "testing"

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		toolID       string
		method       string
		path         string
		expectedType string
		expectedRisk string
		expectedSum  string
	}{
		{
			name:         "mock refund",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/refund",
			expectedType: "PAYMENT.REFUND",
			expectedRisk: RiskHigh,
			expectedSum:  "Refund payment",
		},
		{
			name:         "mock export",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/export_customer_data",
			expectedType: "DATA.EXPORT",
			expectedRisk: RiskCritical,
			expectedSum:  "Export customer data",
		},
		{
			name:         "mock role change",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/change_role",
			expectedType: "ACCESS.ROLE_CHANGE",
			expectedRisk: RiskHigh,
			expectedSum:  "Change user role",
		},
		{
			name:         "jira create",
			toolID:       "jira",
			method:       "POST",
			path:         "/rest/api/3/issue",
			expectedType: "TICKET.CREATE",
			expectedRisk: RiskLow,
			expectedSum:  "Create Jira issue",
		},
		{
			name:         "jira comment",
			toolID:       "jira",
			method:       "POST",
			path:         "/rest/api/3/issue/123/comment",
			expectedType: "TICKET.COMMENT",
			expectedRisk: RiskLow,
			expectedSum:  "Comment on Jira issue",
		},
		{
			name:         "generic read",
			toolID:       "other",
			method:       "GET",
			path:         "/info",
			expectedType: "DATA.READ",
			expectedRisk: RiskLow,
			expectedSum:  "GET /info",
		},
		{
			name:         "generic update",
			toolID:       "other",
			method:       "post",
			path:         "/update",
			expectedType: "DATA.UPDATE",
			expectedRisk: RiskMedium,
			expectedSum:  "POST /update",
		},
		{
			name:         "generic delete",
			toolID:       "other",
			method:       "DELETE",
			path:         "/item",
			expectedType: "DATA.DELETE",
			expectedRisk: RiskCritical,
			expectedSum:  "DELETE /item",
		},
		{
			name:         "case-insensitive tool",
			toolID:       "MoCk_InTeRnAl",
			method:       "post",
			path:         "/refund",
			expectedType: "PAYMENT.REFUND",
			expectedRisk: RiskHigh,
			expectedSum:  "Refund payment",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := Classify(tc.toolID, tc.method, tc.path, "", nil)
			if result.ActionType != tc.expectedType {
				t.Fatalf("expected action type %s got %s", tc.expectedType, result.ActionType)
			}
			if result.ActionRisk != tc.expectedRisk {
				t.Fatalf("expected risk %s got %s", tc.expectedRisk, result.ActionRisk)
			}
			if result.ActionSummary != tc.expectedSum {
				t.Fatalf("expected summary %q got %q", tc.expectedSum, result.ActionSummary)
			}
		})
	}
}
