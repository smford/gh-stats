package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GeneratePrometheusMetrics transforms a DORAMetricsPayload into standard Prometheus text exposition format.
func GeneratePrometheusMetrics(payload *DORAMetricsPayload) string {
	if payload == nil {
		return ""
	}

	var sb strings.Builder

	labelsMap := map[string]string{
		"repository": payload.Repository,
		"target":     payload.Target,
	}
	if payload.Ref != "" {
		labelsMap["ref"] = payload.Ref
	}

	writeMetric := func(name, help, metricType string, val float64, extraLabels map[string]string) {
		merged := make(map[string]string, len(labelsMap)+len(extraLabels))
		for k, v := range labelsMap {
			merged[k] = v
		}
		for k, v := range extraLabels {
			merged[k] = v
		}

		// Sort label keys for deterministic output
		keys := make([]string, 0, len(merged))
		for k := range merged {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var labelPairs []string
		for _, k := range keys {
			v := merged[k]
			v = strings.ReplaceAll(v, "\\", "\\\\")
			v = strings.ReplaceAll(v, "\"", "\\\"")
			v = strings.ReplaceAll(v, "\n", "\\n")
			labelPairs = append(labelPairs, fmt.Sprintf("%s=\"%s\"", k, v))
		}

		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
		sb.WriteString(fmt.Sprintf("# TYPE %s %s\n", name, metricType))
		sb.WriteString(fmt.Sprintf("%s{%s} %g\n", name, strings.Join(labelPairs, ","), val))
	}

	// 1. Target: PR
	if payload.PR != nil {
		pr := payload.PR
		prExtra := map[string]string{}
		if pr.Number > 0 {
			prExtra["pr"] = fmt.Sprintf("%d", pr.Number)
		}

		writeMetric("gh_stats_risk_score", "SRE reliability and deployment risk score (0-100)", "gauge", float64(pr.RiskScore), prExtra)
		writeMetric("gh_stats_lines_added_total", "Total production lines added", "counter", float64(pr.TotalAdditions), prExtra)
		writeMetric("gh_stats_lines_deleted_total", "Total production lines deleted", "counter", float64(pr.TotalDeletions), prExtra)
		writeMetric("gh_stats_lines_net", "Net change in lines", "gauge", float64(pr.NetChange), prExtra)
		writeMetric("gh_stats_files_changed", "Total number of modified files", "gauge", float64(pr.FilesChanged), prExtra)
		writeMetric("gh_stats_test_ratio", "Ratio of automated test lines added to code lines added", "gauge", pr.TestRatio, prExtra)
		writeMetric("gh_stats_sensitive_files_total", "Number of high-blast-radius sensitive files modified", "gauge", float64(pr.SensitiveFilesCount), prExtra)
		writeMetric("gh_stats_commits_total", "Number of commits in change delta", "counter", float64(pr.CommitsCount), prExtra)

		if pr.LeadTimeHours > 0 {
			writeMetric("gh_stats_pr_lead_time_hours", "PR lead time duration from open to current in hours", "gauge", pr.LeadTimeHours, prExtra)
		}
		if pr.TimeToFirstReviewHours > 0 {
			writeMetric("gh_stats_pr_time_to_first_review_hours", "Time to first human review in hours", "gauge", pr.TimeToFirstReviewHours, prExtra)
		}
		if pr.DiscussionCount > 0 {
			writeMetric("gh_stats_pr_discussions_total", "Total discussion comments count on the PR", "counter", float64(pr.DiscussionCount), prExtra)
		}
		if pr.ReviewsCount > 0 {
			writeMetric("gh_stats_pr_reviews_total", "Total review submissions count", "counter", float64(pr.ReviewsCount), prExtra)
		}
		if pr.ApprovalsCount > 0 {
			writeMetric("gh_stats_pr_approvals_total", "Total review approvals count", "counter", float64(pr.ApprovalsCount), prExtra)
		}

		if pr.CIPipeline != nil {
			ci := pr.CIPipeline
			writeMetric("gh_stats_ci_latency_seconds", "Total cumulative duration of all CI check runs in seconds", "gauge", ci.TotalDurationSeconds, prExtra)
			writeMetric("gh_stats_ci_longest_run_seconds", "Duration of the single longest running CI check run in seconds", "gauge", ci.LongestRunDurationSeconds, prExtra)
			writeMetric("gh_stats_flaky_checks_total", "Number of flaky CI check runs detected on retry", "counter", float64(ci.FlakyRunsCount), prExtra)
			writeMetric("gh_stats_ci_runs_total", "Total number of evaluated CI check runs", "counter", float64(ci.TotalCheckRuns), prExtra)
			writeMetric("gh_stats_ci_runs_failed_total", "Number of failed CI check runs", "counter", float64(ci.FailedRuns), prExtra)
		}

		if pr.RiskBudget != nil {
			rbExtra := make(map[string]string)
			for k, v := range prExtra {
				rbExtra[k] = v
			}
			rbExtra["status"] = pr.RiskBudget.Status
			writeMetric("gh_stats_risk_budget_utilization_percent", "SRE risk budget utilization percentage for rolling window", "gauge", pr.RiskBudget.UtilizationPercent, rbExtra)
			writeMetric("gh_stats_risk_budget_burn_rate", "SRE SLO risk budget burn rate relative to sustainable pace", "gauge", pr.RiskBudget.BurnRate, rbExtra)
			writeMetric("gh_stats_risk_budget_points_total", "Cumulative projected risk points in rolling window", "gauge", float64(pr.RiskBudget.TotalProjectedPoints), prExtra)
		}
	}

	// 2. Target: Release
	if payload.Release != nil {
		rel := payload.Release
		relExtra := map[string]string{
			"base_ref": rel.BaseRef,
			"head_ref": rel.HeadRef,
		}

		writeMetric("gh_stats_risk_score", "SRE reliability and deployment risk score (0-100)", "gauge", float64(rel.RiskScore), relExtra)
		writeMetric("gh_stats_lines_added_total", "Total production lines added in release", "counter", float64(rel.TotalAdditions), relExtra)
		writeMetric("gh_stats_lines_deleted_total", "Total production lines deleted in release", "counter", float64(rel.TotalDeletions), relExtra)
		writeMetric("gh_stats_lines_net", "Net change in lines in release", "gauge", float64(rel.NetChange), relExtra)
		writeMetric("gh_stats_files_changed", "Total number of modified files in release", "gauge", float64(rel.FilesChanged), relExtra)
		writeMetric("gh_stats_test_ratio", "Ratio of automated test lines to code lines added in release", "gauge", rel.TestRatio, relExtra)
		writeMetric("gh_stats_release_commits_total", "Total commits in release delta", "counter", float64(rel.TotalCommits), relExtra)
		writeMetric("gh_stats_release_breaking_changes_total", "Total breaking changes and schema migrations in release", "counter", float64(rel.BreakingChangesCount), relExtra)
		writeMetric("gh_stats_release_contributors_total", "Number of unique contributors in release", "gauge", float64(rel.ContributorsCount), relExtra)
		writeMetric("gh_stats_sensitive_files_total", "Number of high-blast-radius sensitive files modified in release", "gauge", float64(rel.SensitiveFilesCount), relExtra)
	}

	// 3. Target: Drift
	if payload.Drift != nil {
		drift := payload.Drift
		driftExtra := map[string]string{
			"base_ref": drift.BaseRef,
			"head_ref": drift.HeadRef,
		}

		writeMetric("gh_stats_risk_score", "SRE promotion deployment risk score (0-100)", "gauge", float64(drift.RiskScore), driftExtra)
		writeMetric("gh_stats_lines_added_total", "Total unpromoted lines added", "counter", float64(drift.TotalAdditions), driftExtra)
		writeMetric("gh_stats_lines_deleted_total", "Total unpromoted lines deleted", "counter", float64(drift.TotalDeletions), driftExtra)
		writeMetric("gh_stats_lines_net", "Net unpromoted lines changed", "gauge", float64(drift.NetChange), driftExtra)
		writeMetric("gh_stats_files_changed", "Total unpromoted files modified", "gauge", float64(drift.FilesChanged), driftExtra)
		writeMetric("gh_stats_drift_commits_ahead", "Number of unpromoted commits in candidate environment awaiting promotion", "gauge", float64(drift.CommitsAhead), driftExtra)
		writeMetric("gh_stats_drift_commits_behind", "Number of missing upstream commits in candidate environment diverged from base", "gauge", float64(drift.CommitsBehind), driftExtra)
		writeMetric("gh_stats_release_breaking_changes_total", "Number of unpromoted breaking changes or schema migrations waiting between environments", "counter", float64(drift.BreakingChangesCount), driftExtra)
		writeMetric("gh_stats_sensitive_files_total", "Number of unpromoted high-blast-radius files modified", "gauge", float64(drift.SensitiveFilesCount), driftExtra)
	}

	// 4. Target: Repo
	if payload.Repo != nil {
		repo := payload.Repo
		writeMetric("gh_stats_repo_files_total", "Total tracked repository files", "gauge", float64(repo.TotalFiles), nil)
		writeMetric("gh_stats_repo_test_density_percent", "Percentage of repository files categorized as automated tests", "gauge", repo.TestDensityPercent, nil)
		writeMetric("gh_stats_repo_top_author_percentage", "Commit share percentage of the most frequent contributor (bus factor metric)", "gauge", repo.TopAuthorPercentage, nil)
		if repo.StarsCount > 0 {
			writeMetric("gh_stats_repo_stars_total", "GitHub stargazers count", "gauge", float64(repo.StarsCount), nil)
		}
		if repo.ForksCount > 0 {
			writeMetric("gh_stats_repo_forks_total", "GitHub forks count", "gauge", float64(repo.ForksCount), nil)
		}
		if repo.OpenIssuesCount > 0 {
			writeMetric("gh_stats_repo_open_issues_total", "GitHub open issues count", "gauge", float64(repo.OpenIssuesCount), nil)
		}
	}

	return sb.String()
}

// ExportPrometheus writes Prometheus formatted metrics to a target file.
func ExportPrometheus(payload *DORAMetricsPayload, filePath string) error {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	content := GeneratePrometheusMetrics(payload)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write Prometheus metrics to %s: %w", filePath, err)
	}
	return nil
}
