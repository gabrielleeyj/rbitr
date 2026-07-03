package coverage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

type fakeReader struct {
	config     models.TenantConfig
	configErr  error
	hits       []models.CoverageFallbackHit
	hitsErr    error
	unused     []string
	unusedErr  error
	gotSince   time.Time
	gotRuleIDs []string
	gotLimit   int
}

func (f *fakeReader) GetTenantConfig(_ context.Context, _ string) (models.TenantConfig, error) {
	return f.config, f.configErr
}

func (f *fakeReader) ListFallbackHitPairs(_ context.Context, _ string, ruleIDs []string, since time.Time, limit int) ([]models.CoverageFallbackHit, error) {
	f.gotRuleIDs = ruleIDs
	f.gotSince = since
	f.gotLimit = limit
	return f.hits, f.hitsErr
}

func (f *fakeReader) ListUnusedActiveTools(_ context.Context, _ string) ([]string, error) {
	return f.unused, f.unusedErr
}

func TestBuildReportMergesAndSorts(t *testing.T) {
	// Arrange
	now := time.Now().UTC()
	reader := &fakeReader{
		config: models.TenantConfig{ActivePolicyVersion: "p_v3"},
		hits: []models.CoverageFallbackHit{
			{ToolID: "jira", ActionType: "TICKET.CREATE", ActionRisk: "LOW", Decision: "DENY", RuleID: "rule_default_deny", HitCount: 3, LastSeen: now},
			{ToolID: "billing", ActionType: "PAYMENT.REFUND", ActionRisk: "HIGH", Decision: "REQUIRE_APPROVAL", RuleID: "rule_high_risk_unknown", HitCount: 10, LastSeen: now},
		},
		unused: []string{"zeta_tool", "alpha_tool"},
	}

	// Act
	report, err := BuildReport(context.Background(), reader, "t_demo", 0, 0)
	// Assert
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if report.ActivePolicyVersion != "p_v3" {
		t.Errorf("active version = %q, want p_v3", report.ActivePolicyVersion)
	}
	if report.WindowDays != DefaultWindowDays {
		t.Errorf("window = %d, want default %d", report.WindowDays, DefaultWindowDays)
	}
	if report.Summary.FallbackHitPairs != 2 || report.Summary.UnusedTools != 2 {
		t.Errorf("summary = %+v, want 2/2", report.Summary)
	}
	if len(report.Gaps) != 4 {
		t.Fatalf("gaps = %d, want 4", len(report.Gaps))
	}
	// Highest hit count first.
	if report.Gaps[0].ToolID != "billing" {
		t.Errorf("first gap tool = %q, want billing (most hits)", report.Gaps[0].ToolID)
	}
	if report.Gaps[0].Reason != reasonFallbackHit {
		t.Errorf("first gap reason = %q, want %q", report.Gaps[0].Reason, reasonFallbackHit)
	}
	// no_traffic gaps come after fallback_hit and are alphabetised.
	if report.Gaps[2].ToolID != "alpha_tool" || report.Gaps[2].Reason != reasonNoTraffic {
		t.Errorf("gap[2] = %+v, want alpha_tool/no_traffic", report.Gaps[2])
	}
	if report.Gaps[3].ToolID != "zeta_tool" {
		t.Errorf("gap[3] tool = %q, want zeta_tool", report.Gaps[3].ToolID)
	}
}

func TestBuildReportPassesFallbackRuleIDs(t *testing.T) {
	reader := &fakeReader{}
	if _, err := BuildReport(context.Background(), reader, "t_demo", 7, 50); err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if len(reader.gotRuleIDs) != 3 {
		t.Errorf("rule ids passed = %v, want 3 fallback ids", reader.gotRuleIDs)
	}
	if reader.gotLimit != 50 {
		t.Errorf("limit = %d, want 50", reader.gotLimit)
	}
}

func TestBuildReportClampsWindowAndLimit(t *testing.T) {
	reader := &fakeReader{}
	report, err := BuildReport(context.Background(), reader, "t_demo", 100000, 100000)
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if report.WindowDays != maxWindowDays {
		t.Errorf("window = %d, want clamp %d", report.WindowDays, maxWindowDays)
	}
	if reader.gotLimit != maxLimit {
		t.Errorf("limit = %d, want clamp %d", reader.gotLimit, maxLimit)
	}
}

func TestBuildReportPropagatesErrors(t *testing.T) {
	wantErr := errors.New("db down")
	reader := &fakeReader{hitsErr: wantErr}
	if _, err := BuildReport(context.Background(), reader, "t_demo", 0, 0); !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}

func TestBuildReportEmpty(t *testing.T) {
	reader := &fakeReader{}
	report, err := BuildReport(context.Background(), reader, "t_demo", 0, 0)
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if len(report.Gaps) != 0 {
		t.Errorf("gaps = %d, want 0", len(report.Gaps))
	}
	if report.Gaps == nil {
		t.Error("gaps should be non-nil empty slice for stable JSON")
	}
}
