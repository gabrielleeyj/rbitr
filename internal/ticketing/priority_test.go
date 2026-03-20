package ticketing

import "testing"

func TestMapPriority(t *testing.T) {
	tests := []struct {
		provider string
		risk     string
		expected string
	}{
		{ProviderJira, "CRITICAL", "Highest"},
		{ProviderJira, "HIGH", "High"},
		{ProviderJira, "MEDIUM", "Medium"},
		{ProviderJira, "LOW", "Low"},
		{ProviderJira, "UNKNOWN", "Medium"},

		{ProviderServiceNow, "CRITICAL", "1"},
		{ProviderServiceNow, "HIGH", "2"},
		{ProviderServiceNow, "MEDIUM", "3"},
		{ProviderServiceNow, "LOW", "4"},

		{ProviderLinear, "CRITICAL", "Urgent"},
		{ProviderLinear, "HIGH", "High"},
		{ProviderLinear, "MEDIUM", "Medium"},
		{ProviderLinear, "LOW", "Low"},

		{"unknown", "HIGH", "HIGH"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+tt.risk, func(t *testing.T) {
			got := MapPriority(tt.provider, tt.risk)
			if got != tt.expected {
				t.Errorf("MapPriority(%q, %q) = %q, want %q", tt.provider, tt.risk, got, tt.expected)
			}
		})
	}
}
