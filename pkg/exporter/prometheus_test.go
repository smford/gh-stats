package exporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratePrometheusMetrics_PR(t *testing.T) {
	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "pr",
		Repository:    "smford/gh-stats",
		Ref:           "feature/auth",
		PR: &PRMetrics{
			BaseRef:                "main",
			HeadRef:                "feature/auth",
			Number:                 42,
			RiskScore:              65,
			RiskLevel:              "HIGH",
			LeadTimeHours:          48.5,
			TimeToFirstReviewHours: 3.2,
			TotalAdditions:         420,
			TotalDeletions:         30,
			NetChange:              390,
			FilesChanged:           8,
			TestRatio:              0.45,
			SensitiveFilesCount:    2,
			CommitsCount:           5,
			DiscussionCount:        12,
			ReviewsCount:           3,
			ApprovalsCount:         2,
			CIPipeline: &CIPipelineMetrics{
				TotalCheckRuns:            10,
				SuccessfulRuns:            9,
				FailedRuns:                1,
				TotalDurationSeconds:      360.5,
				LongestRunDurationSeconds: 120.0,
				FlakyRunsCount:            2,
			},
		},
	}

	metrics := GeneratePrometheusMetrics(payload)

	// Check syntax and structure
	requiredStrings := []string{
		"# HELP gh_stats_risk_score",
		"# TYPE gh_stats_risk_score gauge",
		"gh_stats_risk_score{pr=\"42\",ref=\"feature/auth\",repository=\"smford/gh-stats\",target=\"pr\"} 65",

		"# HELP gh_stats_pr_lead_time_hours",
		"# TYPE gh_stats_pr_lead_time_hours gauge",
		"gh_stats_pr_lead_time_hours{pr=\"42\",ref=\"feature/auth\",repository=\"smford/gh-stats\",target=\"pr\"} 48.5",

		"# HELP gh_stats_lines_added_total",
		"# TYPE gh_stats_lines_added_total counter",
		"gh_stats_lines_added_total{pr=\"42\",ref=\"feature/auth\",repository=\"smford/gh-stats\",target=\"pr\"} 420",

		"# HELP gh_stats_ci_latency_seconds",
		"# TYPE gh_stats_ci_latency_seconds gauge",
		"gh_stats_ci_latency_seconds{pr=\"42\",ref=\"feature/auth\",repository=\"smford/gh-stats\",target=\"pr\"} 360.5",

		"# HELP gh_stats_flaky_checks_total",
		"# TYPE gh_stats_flaky_checks_total counter",
		"gh_stats_flaky_checks_total{pr=\"42\",ref=\"feature/auth\",repository=\"smford/gh-stats\",target=\"pr\"} 2",

		"# HELP gh_stats_test_ratio",
		"# TYPE gh_stats_test_ratio gauge",
		"gh_stats_test_ratio{pr=\"42\",ref=\"feature/auth\",repository=\"smford/gh-stats\",target=\"pr\"} 0.45",
	}

	for _, expected := range requiredStrings {
		if !strings.Contains(metrics, expected) {
			t.Errorf("Prometheus output missing expected string:\n%q\nIn output:\n%s", expected, metrics)
		}
	}
}

func TestGeneratePrometheusMetrics_Release(t *testing.T) {
	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "release",
		Repository:    "smford/gh-stats",
		Ref:           "v0.7.0",
		Release: &ReleaseMetrics{
			BaseRef:              "v0.6.0",
			HeadRef:              "v0.7.0",
			RiskScore:            50,
			RiskLevel:            "MEDIUM",
			TotalCommits:         15,
			ContributorsCount:    4,
			TotalAdditions:       1200,
			TotalDeletions:       150,
			NetChange:            1050,
			FilesChanged:         18,
			TestRatio:            0.32,
			BreakingChangesCount: 3,
			SensitiveFilesCount:  2,
		},
	}

	metrics := GeneratePrometheusMetrics(payload)

	requiredStrings := []string{
		"# HELP gh_stats_release_breaking_changes_total",
		"# TYPE gh_stats_release_breaking_changes_total counter",
		"gh_stats_release_breaking_changes_total{base_ref=\"v0.6.0\",head_ref=\"v0.7.0\",ref=\"v0.7.0\",repository=\"smford/gh-stats\",target=\"release\"} 3",

		"# HELP gh_stats_release_commits_total",
		"# TYPE gh_stats_release_commits_total counter",
		"gh_stats_release_commits_total{base_ref=\"v0.6.0\",head_ref=\"v0.7.0\",ref=\"v0.7.0\",repository=\"smford/gh-stats\",target=\"release\"} 15",
	}

	for _, expected := range requiredStrings {
		if !strings.Contains(metrics, expected) {
			t.Errorf("Prometheus output missing expected string:\n%q\nIn output:\n%s", expected, metrics)
		}
	}
}

func TestGeneratePrometheusMetrics_Drift(t *testing.T) {
	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "drift",
		Repository:    "smford/gh-stats",
		Ref:           "origin/staging",
		Drift: &DriftMetrics{
			BaseRef:              "origin/production",
			HeadRef:              "origin/staging",
			RiskScore:            75,
			RiskLevel:            "HIGH",
			CommitsAhead:         12,
			CommitsBehind:        3,
			TotalAdditions:       800,
			TotalDeletions:       60,
			NetChange:            740,
			FilesChanged:         10,
			BreakingChangesCount: 1,
			SensitiveFilesCount:  2,
		},
	}

	metrics := GeneratePrometheusMetrics(payload)

	requiredStrings := []string{
		"# HELP gh_stats_drift_commits_ahead",
		"# TYPE gh_stats_drift_commits_ahead gauge",
		"gh_stats_drift_commits_ahead{base_ref=\"origin/production\",head_ref=\"origin/staging\",ref=\"origin/staging\",repository=\"smford/gh-stats\",target=\"drift\"} 12",

		"# HELP gh_stats_drift_commits_behind",
		"# TYPE gh_stats_drift_commits_behind gauge",
		"gh_stats_drift_commits_behind{base_ref=\"origin/production\",head_ref=\"origin/staging\",ref=\"origin/staging\",repository=\"smford/gh-stats\",target=\"drift\"} 3",
	}

	for _, expected := range requiredStrings {
		if !strings.Contains(metrics, expected) {
			t.Errorf("Prometheus output missing expected string:\n%q\nIn output:\n%s", expected, metrics)
		}
	}
}

func TestExportPrometheus_File(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sub", "metrics.prom")

	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "pr",
		Repository:    "smford/gh-stats",
		PR: &PRMetrics{
			RiskScore:      20,
			TotalAdditions: 50,
			TotalDeletions: 10,
		},
	}

	if err := ExportPrometheus(payload, filePath); err != nil {
		t.Fatalf("ExportPrometheus failed: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if !strings.Contains(string(content), "gh_stats_risk_score") {
		t.Errorf("expected gh_stats_risk_score in file, got:\n%s", string(content))
	}
}
