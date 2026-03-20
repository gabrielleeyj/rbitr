package ticketing

import "testing"

func TestMapWebhookStatus(t *testing.T) {
	tests := []struct {
		provider string
		status   string
		expected ApprovalAction
	}{
		{ProviderJira, "Done", ActionApprove},
		{ProviderJira, "Closed", ActionApprove},
		{ProviderJira, "Resolved", ActionApprove},
		{ProviderJira, "Rejected", ActionDeny},
		{ProviderJira, "Won't Do", ActionDeny},
		{ProviderJira, "In Progress", ActionNone},

		{ProviderServiceNow, "resolved", ActionApprove},
		{ProviderServiceNow, "closed", ActionApprove},
		{ProviderServiceNow, "canceled", ActionDeny},
		{ProviderServiceNow, "rejected", ActionDeny},
		{ProviderServiceNow, "in_progress", ActionNone},

		{ProviderLinear, "Done", ActionApprove},
		{ProviderLinear, "Completed", ActionApprove},
		{ProviderLinear, "Canceled", ActionDeny},
		{ProviderLinear, "Cancelled", ActionDeny},
		{ProviderLinear, "Started", ActionNone},

		{"unknown", "Done", ActionNone},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+tt.status, func(t *testing.T) {
			got := MapWebhookStatus(tt.provider, tt.status)
			if got != tt.expected {
				t.Errorf("MapWebhookStatus(%q, %q) = %q, want %q", tt.provider, tt.status, got, tt.expected)
			}
		})
	}
}
