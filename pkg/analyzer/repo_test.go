package analyzer

import (
	"testing"

	"github.com/smford/gh-stats/pkg/sarif"
)

func TestRepoStatsSARIFPopulation(t *testing.T) {
	stats := &RepoStats{
		TotalFiles:     100,
		TestFilesCount: 25,
		DocFilesCount:  10,
		TestFileRatio:  25.0,
		PrimaryFile:    "README.md",
		ExtensionBreakdown: []FileTypeCount{
			{Extension: ".go", Count: 60},
			{Extension: ".md", Count: 10},
		},
		ChurnHotspots: []ChurnEntry{
			{Path: "pkg/core/engine.go", Count: 42},
			{Path: "pkg/api/server.go", Count: 30},
		},
		TopContributors: []AuthorEntry{
			{Author: "Alice", Commits: 85, Percentage: 85.0},
			{Author: "Bob", Commits: 15, Percentage: 15.0},
		},
		TopAuthorPercentage: 85.0,
	}

	builder := sarif.NewBuilder()
	stats.PopulateSARIF(builder)

	report := builder.Build()
	if len(report.Runs[0].Results) == 0 {
		t.Fatalf("expected results in repo SARIF report")
	}

	foundRules := make(map[string]bool)
	for _, res := range report.Runs[0].Results {
		foundRules[res.RuleID] = true
	}

	expectedRules := []string{
		RuleRepoSummary.ID,
		RuleRepoHotspots.ID,
		RuleRepoBusFactor.ID,
	}

	for _, er := range expectedRules {
		if !foundRules[er] {
			t.Errorf("expected rule %s in results, but was not found", er)
		}
	}
}
