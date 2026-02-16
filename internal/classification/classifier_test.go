package classification

import "testing"

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		toolID       string
		method       string
		path         string
		query        string
		expectedType string
		expectedRisk string
		expectedSum  string
	}{
		{
			name:         "mock refund",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/refund",
			query:        "",
			expectedType: "PAYMENT.REFUND",
			expectedRisk: RiskHigh,
			expectedSum:  "Refund payment",
		},
		{
			name:         "mock export",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/export_customer_data",
			query:        "",
			expectedType: "DATA.EXPORT",
			expectedRisk: RiskCritical,
			expectedSum:  "Export customer data",
		},
		{
			name:         "mock role change",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/change_role",
			query:        "",
			expectedType: "ACCESS.ROLE_CHANGE",
			expectedRisk: RiskHigh,
			expectedSum:  "Change user role",
		},
		{
			name:         "jira create",
			toolID:       "jira",
			method:       "POST",
			path:         "/rest/api/3/issue",
			query:        "",
			expectedType: "TICKET.CREATE",
			expectedRisk: RiskLow,
			expectedSum:  "Create Jira issue",
		},
		{
			name:         "jira comment",
			toolID:       "jira",
			method:       "POST",
			path:         "/rest/api/3/issue/123/comment",
			query:        "",
			expectedType: "TICKET.COMMENT",
			expectedRisk: RiskLow,
			expectedSum:  "Comment on Jira issue",
		},
		{
			name:         "jira update issue",
			toolID:       "jira",
			method:       "PUT",
			path:         "/rest/api/3/issue/ABC-123",
			query:        "",
			expectedType: "TICKET.UPDATE",
			expectedRisk: RiskLow,
			expectedSum:  "Update Jira issue",
		},
		{
			name:         "jira transition",
			toolID:       "jira",
			method:       "POST",
			path:         "/rest/api/3/issue/ABC-123/transitions",
			query:        "",
			expectedType: "TICKET.UPDATE",
			expectedRisk: RiskLow,
			expectedSum:  "Update Jira issue",
		},
		{
			name:         "jira search",
			toolID:       "jira",
			method:       "GET",
			path:         "/rest/api/3/search",
			query:        "jql=project%3DSEC",
			expectedType: "DATA.QUERY",
			expectedRisk: RiskLow,
			expectedSum:  "Query Jira issues",
		},
		{
			name:         "jira delete issue",
			toolID:       "jira",
			method:       "DELETE",
			path:         "/rest/api/3/issue/ABC-123",
			query:        "",
			expectedType: "DATA.DELETE",
			expectedRisk: RiskCritical,
			expectedSum:  "Delete Jira issue",
		},
		{
			name:         "jira delete comment",
			toolID:       "jira",
			method:       "DELETE",
			path:         "/rest/api/3/issue/ABC-123/comment/1001",
			query:        "",
			expectedType: "DATA.DELETE",
			expectedRisk: RiskCritical,
			expectedSum:  "Delete Jira comment",
		},
		{
			name:         "mock refund nested path",
			toolID:       "mock_internal",
			method:       "post",
			path:         "/payments/txn_1/refund/",
			query:        "",
			expectedType: "PAYMENT.REFUND",
			expectedRisk: RiskHigh,
			expectedSum:  "Refund payment",
		},
		{
			name:         "mock role change nested path",
			toolID:       "mock_internal",
			method:       "PATCH",
			path:         "/users/42/roles/assign",
			query:        "",
			expectedType: "ACCESS.ROLE_CHANGE",
			expectedRisk: RiskHigh,
			expectedSum:  "Change user role",
		},
		{
			name:         "mock access grant",
			toolID:       "mock_internal",
			method:       "POST",
			path:         "/users/42/grant_access",
			query:        "",
			expectedType: "ACCESS.GRANT",
			expectedRisk: RiskCritical,
			expectedSum:  "Grant access",
		},
		{
			name:         "generic read",
			toolID:       "other",
			method:       "GET",
			path:         "/info",
			query:        "",
			expectedType: "DATA.READ",
			expectedRisk: RiskLow,
			expectedSum:  "GET /info",
		},
		{
			name:         "generic update",
			toolID:       "other",
			method:       "post",
			path:         "/update",
			query:        "",
			expectedType: "DATA.UPDATE",
			expectedRisk: RiskMedium,
			expectedSum:  "POST /update",
		},
		{
			name:         "generic delete",
			toolID:       "other",
			method:       "DELETE",
			path:         "/item",
			query:        "",
			expectedType: "DATA.DELETE",
			expectedRisk: RiskCritical,
			expectedSum:  "DELETE /item",
		},
		{
			name:         "generic query by path",
			toolID:       "other",
			method:       "POST",
			path:         "/v1/search/users",
			query:        "",
			expectedType: "DATA.QUERY",
			expectedRisk: RiskLow,
			expectedSum:  "POST /v1/search/users",
		},
		{
			name:         "generic query by query string",
			toolID:       "other",
			method:       "GET",
			path:         "/users",
			query:        "q=alice",
			expectedType: "DATA.QUERY",
			expectedRisk: RiskLow,
			expectedSum:  "GET /users",
		},
		{
			name:         "generic export",
			toolID:       "other",
			method:       "GET",
			path:         "/reports/export",
			query:        "",
			expectedType: "DATA.EXPORT",
			expectedRisk: RiskCritical,
			expectedSum:  "GET /reports/export",
		},
		{
			name:         "generic bulk export",
			toolID:       "other",
			method:       "POST",
			path:         "/reports/export",
			query:        "bulk=true",
			expectedType: "DATA.BULK_EXPORT",
			expectedRisk: RiskCritical,
			expectedSum:  "POST /reports/export",
		},
		{
			name:         "generic crm read",
			toolID:       "other",
			method:       "GET",
			path:         "/crm/contacts",
			query:        "",
			expectedType: "CRM.READ",
			expectedRisk: RiskLow,
			expectedSum:  "GET /crm/contacts",
		},
		{
			name:         "generic crm delete",
			toolID:       "other",
			method:       "DELETE",
			path:         "/crm/contacts/123",
			query:        "",
			expectedType: "CRM.DELETE",
			expectedRisk: RiskCritical,
			expectedSum:  "DELETE /crm/contacts/123",
		},
		{
			name:         "generic access grant",
			toolID:       "other",
			method:       "POST",
			path:         "/users/1/permissions/grant",
			query:        "",
			expectedType: "ACCESS.GRANT",
			expectedRisk: RiskCritical,
			expectedSum:  "POST /users/1/permissions/grant",
		},
		{
			name:         "case-insensitive tool",
			toolID:       "MoCk_InTeRnAl",
			method:       "post",
			path:         "/refund",
			query:        "",
			expectedType: "PAYMENT.REFUND",
			expectedRisk: RiskHigh,
			expectedSum:  "Refund payment",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := Classify(tc.toolID, tc.method, tc.path, tc.query, nil)
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
