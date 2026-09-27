package reporter

import (
	"strings"
	"testing"
	"time"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/github"
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
