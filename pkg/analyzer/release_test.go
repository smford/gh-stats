package analyzer

import (
	"testing"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

func TestAnalyzeRelease(t *testing.T) {
	runner := gitutil.NewRunner(".")
	baseTag, headTag, err := runner.GetLatestTwoTags()
	if err != nil {
		t.Fatalf("failed to get latest two tags: %v", err)
	}

	cfg := config.DefaultConfig()
	stats, err := AnalyzeRelease(runner, baseTag, headTag, cfg)
	if err != nil {
		t.Fatalf("AnalyzeRelease failed: %v", err)
	}

	if stats.BaseRef != baseTag || stats.HeadRef != headTag {
		t.Errorf("expected base %s and head %s, got %s and %s", baseTag, headTag, stats.BaseRef, stats.HeadRef)
	}

	if stats.TotalCommits == 0 {
		t.Logf("no commits between %s and %s", baseTag, headTag)
	}

	if stats.RiskLevel == "" {
		t.Errorf("expected non-empty risk level")
	}

	builder := sarif.NewBuilder()
	stats.PopulateSARIF(builder)
	report := builder.Build()
	if len(report.Runs) == 0 {
		t.Fatalf("expected at least one SARIF run")
	}
	if len(report.Runs[0].Results) == 0 {
		t.Errorf("expected at least one SARIF result for release summary")
	}
}

func TestConventionalMatch(t *testing.T) {
	tests := []struct {
		subject string
		prefix  string
		want    bool
	}{
		{"feat: add new feature", "feat", true},
		{"feat(cli): add release flag", "feat", true},
		{"feat!: breaking change", "feat", true},
		{"feat(api)!: breaking change", "feat", true},
		{"fix: fix bug", "fix", true},
		{"fix(ci): fix workflow", "fix", true},
		{"fix!: breaking bug fix", "fix", true},
		{"chore: update deps", "chore", true},
		{"docs: update readme", "docs", true},
		{"refactor: clean up code", "refactor", true},
		{"random commit message", "feat", false},
	}

	for _, tt := range tests {
		got := isConventionalMatch(tt.subject, tt.prefix)
		if got != tt.want {
			t.Errorf("isConventionalMatch(%q, %q) = %v; want %v", tt.subject, tt.prefix, got, tt.want)
		}
	}
}

func TestReleaseStatsRiskCalculation(t *testing.T) {
	stats := &ReleaseStats{
		BaseRef:        "v0.1.0",
		HeadRef:        "v0.2.0",
		TotalAdditions: 4000,
		TotalDeletions: 100,
		CodeLinesAdded: 500,
		TestLinesAdded: 10,
		TestRatio:      0.02,
		BreakingChanges: []BreakingChange{
			{Subject: "feat!: breaking api change", Reason: "conventional_commit"},
		},
		SensitiveFiles: []SensitiveMatch{
			{Path: "db/migrations/001_create.sql", Category: "Database Migrations"},
			{Path: ".github/workflows/ci.yml", Category: "CI/CD Pipelines"},
		},
	}

	stats.CalculateRisk()
	if stats.RiskScore < 80 {
		t.Errorf("expected high risk score for breaking change + migrations + large diff, got %d", stats.RiskScore)
	}
	if stats.RiskLevel != "CRITICAL" {
		t.Errorf("expected CRITICAL risk level, got %s", stats.RiskLevel)
	}
}
