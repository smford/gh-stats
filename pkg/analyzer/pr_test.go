package analyzer

import (
	"testing"

	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"pkg/analyzer/pr_test.go", true},
		{"src/components/Button.test.tsx", true},
		{"src/components/Button.test.ts", true},
		{"tests/integration/api.py", true},
		{"cmd/gh-stats/main.go", false},
		{"README.md", false},
	}

	for _, tt := range tests {
		got := IsTestFile(tt.path)
		if got != tt.expected {
			t.Errorf("IsTestFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestSensitivePatterns(t *testing.T) {
	patterns := DefaultSensitivePatterns()
	sensitivePaths := []string{
		".github/workflows/deploy.yml",
		"infra/terraform/main.tf",
		"db/migrations/001_init.sql",
		"go.mod",
		"pkg/auth/token.go",
	}

	for _, path := range sensitivePaths {
		matched := false
		for _, p := range patterns {
			if p.Match(path) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("expected %s to match sensitive pattern, but it did not", path)
		}
	}
}

func TestPRStatsRiskCalculationAndSARIF(t *testing.T) {
	stats := &PRStats{
		TotalAdditions: 900,
		TotalDeletions: 100,
		CodeLinesAdded: 900,
		FilesChanged:   15,
		CommitCount:    3,
		PrimaryFile:    ".github/workflows/ci.yml",
		SensitiveFiles: []SensitiveMatch{
			{
				Path:        ".github/workflows/ci.yml",
				Category:    "CI/CD Pipelines",
				Description: "Pipeline change",
				Additions:   20,
				Deletions:   5,
			},
		},
		TopChangedFiles: []gitutil.FileDiffStat{
			{Path: ".github/workflows/ci.yml", Additions: 20, Deletions: 5},
		},
	}

	calculateRisk(stats)

	if stats.RiskLevel != "HIGH" && stats.RiskLevel != "CRITICAL" {
		t.Errorf("expected high/critical risk for large PR with sensitive files and 0 tests, got %s (score %d)", stats.RiskLevel, stats.RiskScore)
	}

	builder := sarif.NewBuilder()
	stats.PopulateSARIF(builder)

	report := builder.Build()
	if len(report.Runs[0].Results) == 0 {
		t.Fatalf("expected SARIF results, got none")
	}

	// Should contain PR-SUMMARY, PR-SIZE, PR-TEST-RATIO, PR-BLAST-RADIUS
	foundRules := make(map[string]bool)
	for _, res := range report.Runs[0].Results {
		foundRules[res.RuleID] = true
	}

	expectedRules := []string{
		RulePRSummary.ID,
		RulePRSize.ID,
		RulePRTestRatio.ID,
		RulePRBlastRadius.ID,
	}

	for _, er := range expectedRules {
		if !foundRules[er] {
			t.Errorf("expected rule %s in results, but was not found", er)
		}
	}
}
