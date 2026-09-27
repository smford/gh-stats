package reporter

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/smford/gh-stats/pkg/analyzer"
)

// GeneratePRSummary generates a GitHub Step Summary Markdown string for PR analysis.
func GeneratePRSummary(stats *analyzer.PRStats) string {
	var sb strings.Builder
	sb.WriteString("# 🚀 GitHub Stats: Pull Request SRE Assessment\n\n")

	badgeColor := "blue"
	switch stats.RiskLevel {
	case "LOW":
		badgeColor = "green"
	case "MEDIUM":
		badgeColor = "yellow"
	case "HIGH":
		badgeColor = "orange"
	case "CRITICAL":
		badgeColor = "red"
	}

	sb.WriteString(fmt.Sprintf("> **Overall SRE Risk Rating:** `%s` (Score: **%d / 100**) 🛡️\n\n", stats.RiskLevel, stats.RiskScore))
	_ = badgeColor

	sb.WriteString("## 📊 Change Metrics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Lines Added** | `+%d` |\n", stats.TotalAdditions))
	sb.WriteString(fmt.Sprintf("| **Lines Deleted** | `-%d` |\n", stats.TotalDeletions))
	fileBreakdown := fmt.Sprintf("Code: `%d`, Tests: `%d`, Docs: `%d`", stats.CodeFilesCount, stats.TestFilesCount, stats.DocFilesCount)
	if stats.GeneratedFilesCount > 0 {
		fileBreakdown = fmt.Sprintf("Code: `%d`, Tests: `%d`, Gen: `%d`, Docs: `%d`", stats.CodeFilesCount, stats.TestFilesCount, stats.GeneratedFilesCount, stats.DocFilesCount)
	}
	sb.WriteString(fmt.Sprintf("| **Files Modified** | `%d` (%s) |\n", stats.FilesChanged, fileBreakdown))
	sb.WriteString(fmt.Sprintf("| **Test vs Code Delta** | `+%d` test lines / `+%d` code lines |\n", stats.TestLinesAdded, stats.CodeLinesAdded))
	sb.WriteString(fmt.Sprintf("| **Commits** | `%d` |\n", stats.CommitCount))

	if len(stats.SensitiveFiles) > 0 {
		sb.WriteString("\n## ⚠️ High Blast Radius Files\n\n")
		sb.WriteString("| Path | Category | Delta |\n")
		sb.WriteString("| :--- | :--- | :--- |\n")
		for _, sf := range stats.SensitiveFiles {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | `+%d / -%d` |\n", sf.Path, sf.Category, sf.Additions, sf.Deletions))
		}
	}

	if len(stats.TopChangedFiles) > 0 {
		sb.WriteString("\n## 📁 Top Modified Files\n\n")
		sb.WriteString("| File | Additions | Deletions | Net |\n")
		sb.WriteString("| :--- | :---: | :---: | :---: |\n")
		limit := len(stats.TopChangedFiles)
		if limit > 7 {
			limit = 7
		}
		for i := 0; i < limit; i++ {
			f := stats.TopChangedFiles[i]
			sb.WriteString(fmt.Sprintf("| `%s` | `+%d` | `-%d` | `%+d` |\n", f.Path, f.Additions, f.Deletions, f.Additions-f.Deletions))
		}
	}

	if stats.GitHubMeta != nil {
		sb.WriteString("\n## ⏱️ PR Lifecycle & Review Velocity\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("| :--- | :--- |\n")
		days := int(stats.GitHubMeta.Age.Hours() / 24)
		hours := int(stats.GitHubMeta.Age.Hours()) % 24
		sb.WriteString(fmt.Sprintf("| **PR Age / Lead Time** | `%dd %dh` |\n", days, hours))
		if stats.GitHubMeta.TimeToFirstReview > 0 {
			ttfrHours := int(stats.GitHubMeta.TimeToFirstReview.Hours())
			ttfrMins := int(stats.GitHubMeta.TimeToFirstReview.Minutes()) % 60
			sb.WriteString(fmt.Sprintf("| **Time to First Review (TTFR)** | `%dh %dm` |\n", ttfrHours, ttfrMins))
		}
		sb.WriteString(fmt.Sprintf("| **Discussions & Comments** | `%d` total comments |\n", stats.GitHubMeta.TotalDiscussions))
		sb.WriteString(fmt.Sprintf("| **Review Status** | `%d` approval(s) across `%d` review(s) |\n", stats.GitHubMeta.ApprovalsCount, stats.GitHubMeta.ReviewsCount))
		sb.WriteString(fmt.Sprintf("| **Author Trust Tier** | `%s` |\n", stats.GitHubMeta.AuthorAssociation))
	}

	if len(stats.RecommendedReviewers) > 0 {
		sb.WriteString("\n## 👥 Suggested Reviewers (Domain Experts)\n\n")
		sb.WriteString("| Reviewer | Historical Commits | Expertise Area |\n")
		sb.WriteString("| :--- | :---: | :--- |\n")
		for _, r := range stats.RecommendedReviewers {
			filesStr := strings.Join(r.TopFiles, "`, `")
			sb.WriteString(fmt.Sprintf("| **%s** | `%d` | `%s` |\n", r.Author, r.CommitCount, filesStr))
		}
	}

	if stats.CIPipelineStats != nil && stats.CIPipelineStats.TotalCheckRuns > 0 {
		sb.WriteString("\n## ⚡ CI Pipeline Latency & Reliability (Check Runs)\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("| :--- | :--- |\n")
		sb.WriteString(fmt.Sprintf("| **Check Runs Evaluated** | `%d` total (`%d` passed, `%d` failed) |\n",
			stats.CIPipelineStats.TotalCheckRuns, stats.CIPipelineStats.SuccessfulRuns, stats.CIPipelineStats.FailedRuns))
		sb.WriteString(fmt.Sprintf("| **Cumulative CI Runtime** | `%s` |\n", formatDuration(stats.CIPipelineStats.TotalDuration)))
		if stats.CIPipelineStats.LongestRunName != "" {
			sb.WriteString(fmt.Sprintf("| **Critical Path Bottleneck** | `%s` (`%s`) |\n",
				stats.CIPipelineStats.LongestRunName, formatDuration(stats.CIPipelineStats.LongestRunDuration)))
		}

		flakyCount := 0
		for _, f := range stats.CIPipelineStats.FlakyRuns {
			if f.IsFlaky {
				flakyCount++
			}
		}
		if flakyCount > 0 {
			sb.WriteString(fmt.Sprintf("| **Flaky Checks** | `🚨 %d detected` |\n", flakyCount))
		} else {
			sb.WriteString("| **Flaky Checks** | `0 (stable)` |\n")
		}

		if len(stats.CIPipelineStats.FlakyRuns) > 0 {
			sb.WriteString("\n### ⚠️ Flaky & Retried Checks\n\n")
			sb.WriteString("| Check Run | Retries | Initial Outcome | Final Outcome | Verdict |\n")
			sb.WriteString("| :--- | :---: | :---: | :---: | :--- |\n")
			for _, f := range stats.CIPipelineStats.FlakyRuns {
				statusBadge := "Retried"
				if f.IsFlaky {
					statusBadge = "🚨 **Flaky** (Passed on retry)"
				}
				sb.WriteString(fmt.Sprintf("| `%s` | `%d` | `%s` | `%s` | %s |\n",
					f.Name, f.RetryCount, f.InitialResult, f.FinalResult, statusBadge))
			}
		}

		if len(stats.CIPipelineStats.BottleneckRuns) > 0 {
			sb.WriteString("\n### ⏳ CI Latency Bottlenecks\n\n")
			sb.WriteString("| Check Run | Duration | Status |\n")
			sb.WriteString("| :--- | :---: | :--- |\n")
			for _, b := range stats.CIPipelineStats.BottleneckRuns {
				sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` |\n",
					b.Name, formatDuration(b.Duration), b.Conclusion))
			}
		}
	}

	return sb.String()
}

// GenerateRepoSummary generates a GitHub Step Summary Markdown string for repository analysis.
func GenerateRepoSummary(stats *analyzer.RepoStats) string {
	var sb strings.Builder
	sb.WriteString("# 🏛️ GitHub Stats: Repository Architecture Assessment\n\n")

	if stats.GitHubMeta != nil {
		sb.WriteString("## 🌐 GitHub Ecosystem\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("| :--- | :--- |\n")
		sb.WriteString(fmt.Sprintf("| **Open Issues & PRs** | `%d` |\n", stats.GitHubMeta.OpenIssuesCount))
		sb.WriteString(fmt.Sprintf("| **Stargazers** | `%d` |\n", stats.GitHubMeta.StargazersCount))
		sb.WriteString(fmt.Sprintf("| **Forks** | `%d` |\n", stats.GitHubMeta.ForksCount))
		sb.WriteString(fmt.Sprintf("| **Default Branch** | `%s` |\n\n", stats.GitHubMeta.DefaultBranch))
	}

	sb.WriteString("## 📊 Codebase Composition\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Total Tracked Files** | `%d` |\n", stats.TotalFiles))
	sb.WriteString(fmt.Sprintf("| **Test Files** | `%d` (`%.1f%%` test density) |\n", stats.TestFilesCount, stats.TestFileRatio))
	sb.WriteString(fmt.Sprintf("| **Documentation Files** | `%d` |\n", stats.DocFilesCount))
	if len(stats.TopContributors) > 0 {
		sb.WriteString(fmt.Sprintf("| **Top Contributor Ratio** | `%.1f%%` (%s) |\n", stats.TopAuthorPercentage, stats.TopContributors[0].Author))
	}

	if len(stats.ExtensionBreakdown) > 0 {
		sb.WriteString("\n## 📂 File Types\n\n")
		sb.WriteString("| Extension | Count |\n")
		sb.WriteString("| :--- | :--- |\n")
		limit := len(stats.ExtensionBreakdown)
		if limit > 6 {
			limit = 6
		}
		for i := 0; i < limit; i++ {
			e := stats.ExtensionBreakdown[i]
			sb.WriteString(fmt.Sprintf("| `%s` | `%d` |\n", e.Extension, e.Count))
		}
	}

	if len(stats.ChurnHotspots) > 0 {
		sb.WriteString("\n## 🔥 Top Churn Hotspots\n\n")
		sb.WriteString("| File Path | Modification Frequency |\n")
		sb.WriteString("| :--- | :---: |\n")
		limit := len(stats.ChurnHotspots)
		if limit > 7 {
			limit = 7
		}
		for i := 0; i < limit; i++ {
			h := stats.ChurnHotspots[i]
			sb.WriteString(fmt.Sprintf("| `%s` | `%d` commits |\n", h.Path, h.Count))
		}
	}

	return sb.String()
}

// WriteStepSummary writes the markdown summary to GITHUB_STEP_SUMMARY if available.
func WriteStepSummary(summary string) error {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		return nil
	}

	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open GITHUB_STEP_SUMMARY: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(summary + "\n")
	return err
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

