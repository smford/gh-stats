package reporter

import (
	"strings"
	"testing"
	"time"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/github"
	"github.com/smford/gh-stats/pkg/gitutil"
)

func TestGeneratePRSummary(t *testing.T) {
	stats := &analyzer.PRStats{
		TotalAdditions: 120,
		TotalDeletions: 30,
		NetChange:      90,
		FilesChanged:   4,
		RiskLevel:      "LOW",
		RiskScore:      15,
		CodeLinesAdded: 100,
		TestLinesAdded: 50,
	}

	summary := GeneratePRSummary(stats)

	if !strings.Contains(summary, "Pull Request SRE Assessment") {
		t.Errorf("expected header in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "LOW") {
		t.Errorf("expected risk level in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "+120") {
		t.Errorf("expected additions in summary, got: %s", summary)
	}

	// With recommended reviewers
	stats.RecommendedReviewers = []analyzer.ReviewerRecommendation{
		{Author: "Alice", CommitCount: 12, TopFiles: []string{"pkg/auth/token.go"}},
	}
	summaryWithReviewers := GeneratePRSummary(stats)
	if !strings.Contains(summaryWithReviewers, "Suggested Reviewers") {
		t.Errorf("expected Suggested Reviewers section, got: %s", summaryWithReviewers)
	}
	if !strings.Contains(summaryWithReviewers, "Alice") {
		t.Errorf("expected Alice in reviewers table, got: %s", summaryWithReviewers)
	}

	// With CIPipelineStats
	stats.CIPipelineStats = &github.CIPipelineStats{
		TotalCheckRuns:     3,
		SuccessfulRuns:     2,
		FailedRuns:         1,
		TotalDuration:      15 * time.Minute,
		LongestRunName:     "integration-tests",
		LongestRunDuration: 12 * time.Minute,
		FlakyRuns: []github.FlakyCheck{
			{Name: "flaky-test", RetryCount: 1, IsFlaky: true, InitialResult: "failure", FinalResult: "success"},
		},
		BottleneckRuns: []github.CheckRunSummary{
			{Name: "integration-tests", Duration: 12 * time.Minute, Conclusion: "success"},
		},
	}
	summaryWithCI := GeneratePRSummary(stats)
	if !strings.Contains(summaryWithCI, "CI Pipeline Latency & Reliability") {
		t.Errorf("expected CI Pipeline section, got: %s", summaryWithCI)
	}
	if !strings.Contains(summaryWithCI, "Flaky Checks") {
		t.Errorf("expected Flaky Checks in CI table, got: %s", summaryWithCI)
	}
	if !strings.Contains(summaryWithCI, "integration-tests") {
		t.Errorf("expected integration-tests in CI table, got: %s", summaryWithCI)
	}
}

func TestGenerateRepoSummary(t *testing.T) {
	stats := &analyzer.RepoStats{
		TotalFiles:     50,
		TestFilesCount: 10,
		TestFileRatio:  20.0,
		DocFilesCount:  5,
	}

	summary := GenerateRepoSummary(stats)

	if !strings.Contains(summary, "Repository Architecture Assessment") {
		t.Errorf("expected header in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "20.0%") {
		t.Errorf("expected test density in summary, got: %s", summary)
	}
}

func TestGenerateReleaseSummary(t *testing.T) {
	stats := &analyzer.ReleaseStats{
		BaseRef:        "v0.3.0",
		HeadRef:        "v0.4.0",
		TotalCommits:   5,
		TotalAdditions: 350,
		TotalDeletions: 40,
		NetChange:      310,
		FilesChanged:   8,
		RiskScore:      45,
		RiskLevel:      "MEDIUM",
		CodeLinesAdded: 250,
		TestLinesAdded: 100,
		TestRatio:      0.4,
		Contributors: []analyzer.ContributorStat{
			{Name: "Alice", CommitCount: 3, Percentage: 60.0},
			{Name: "Bob", CommitCount: 2, Percentage: 40.0},
		},
		BreakingChanges: []analyzer.BreakingChange{
			{CommitHash: "a1b2c3d", Subject: "feat!: breaking change to auth API", Reason: "conventional_commit"},
		},
		SensitiveFiles: []analyzer.SensitiveMatch{
			{Path: ".github/workflows/ci.yml", Category: "CI/CD Pipelines", Additions: 15, Deletions: 2},
		},
	}

	summary := GenerateReleaseSummary(stats)

	if !strings.Contains(summary, "Release Comparison (`v0.3.0...v0.4.0`)") {
		t.Errorf("expected header in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "MEDIUM") {
		t.Errorf("expected risk level MEDIUM, got: %s", summary)
	}
	if !strings.Contains(summary, "Breaking Changes & Schema Migrations") {
		t.Errorf("expected breaking changes section, got: %s", summary)
	}
	if !strings.Contains(summary, "feat!: breaking change to auth API") {
		t.Errorf("expected breaking commit in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "High Blast Radius Files") {
		t.Errorf("expected high blast radius section, got: %s", summary)
	}
	if !strings.Contains(summary, "Alice") || !strings.Contains(summary, "Bob") {
		t.Errorf("expected contributors in summary, got: %s", summary)
	}
}

func TestGenerateDriftSummary(t *testing.T) {
	stats := &analyzer.DriftStats{
		BaseRef:        "origin/production",
		HeadRef:        "origin/staging",
		CommitsAhead:  8,
		CommitsBehind: 2,
		TotalAdditions: 450,
		TotalDeletions: 30,
		NetChange:      420,
		FilesChanged:   6,
		RiskScore:      65,
		RiskLevel:      "HIGH",
		BreakingChanges: []analyzer.BreakingChange{
			{CommitHash: "c0ffee1", Subject: "feat!: breaking API v2 overhaul", Reason: "conventional_commit"},
		},
		SensitiveFiles: []analyzer.SensitiveMatch{
			{Path: "migrations/002_orders.sql", Category: "Database Migrations", Additions: 30, Deletions: 0},
		},
		UnpromotedCommits: []gitutil.CommitInfo{
			{Hash: "c0ffee1", Author: "Charlie", Subject: "feat!: breaking API v2 overhaul"},
		},
	}

	summary := GenerateDriftSummary(stats)

	if !strings.Contains(summary, "# 🌐 Environment Drift & Promotion Assessment") {
		t.Errorf("expected header in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "origin/production") || !strings.Contains(summary, "origin/staging") {
		t.Errorf("expected environments in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "HIGH") {
		t.Errorf("expected HIGH risk rating, got: %s", summary)
	}
	if !strings.Contains(summary, "`8` commit(s) awaiting promotion") {
		t.Errorf("expected commits ahead in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "Unpromoted Breaking Changes & Migrations") {
		t.Errorf("expected breaking changes section, got: %s", summary)
	}
	if !strings.Contains(summary, "Unpromoted High Blast Radius Files") {
		t.Errorf("expected high blast radius section, got: %s", summary)
	}

	// Test In-Sync summary
	syncStats := &analyzer.DriftStats{
		BaseRef:       "origin/production",
		HeadRef:       "origin/staging",
		CommitsAhead:  0,
		CommitsBehind: 0,
		FilesChanged:  0,
		RiskScore:     0,
		RiskLevel:     "LOW",
	}
	syncSummary := GenerateDriftSummary(syncStats)
	if !strings.Contains(syncSummary, "Environments are in sync!") {
		t.Errorf("expected in-sync notice, got: %s", syncSummary)
	}
}


