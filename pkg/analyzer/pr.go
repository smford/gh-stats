package analyzer

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/github"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

// PRStats contains calculated statistics and SRE metrics for a pull request.
type PRStats struct {
	BaseRef              string
	HeadRef              string
	TotalAdditions       int
	TotalDeletions       int
	NetChange            int
	FilesChanged         int
	TestFilesCount       int
	DocFilesCount        int
	CodeFilesCount       int
	GeneratedFilesCount  int
	TestLinesAdded       int
	TestLinesDeleted     int
	CodeLinesAdded       int
	CodeLinesDeleted     int
	GeneratedLinesAdded  int
	GeneratedLinesDeleted int
	SensitiveFiles       []SensitiveMatch
	TopChangedFiles      []gitutil.FileDiffStat
	CommitCount          int
	Commits              []gitutil.CommitInfo
	GitHubMeta           *github.PRMetadata       // Optional enrichment from GitHub API
	RecommendedReviewers []ReviewerRecommendation // Suggested domain expert reviewers
	RiskScore            int                      // 0-100 (higher = riskier)
	RiskLevel            string                   // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	PrimaryFile          string                   // representative file for PR-wide SARIF results
	Config               *config.Config           // Active repository configuration
}

// ReviewerRecommendation represents a suggested code reviewer with domain expertise.
type ReviewerRecommendation struct {
	Author      string   `json:"author"`
	CommitCount int      `json:"commitCount"`
	TopFiles    []string `json:"topFiles"`
}

// SensitiveMatch notes a sensitive file and its classification.
type SensitiveMatch struct {
	Path        string
	Category    string
	Description string
	Additions   int
	Deletions   int
}

// AnalyzePR inspects git differences between baseRef and headRef and generates PRStats.
func AnalyzePR(runner *gitutil.Runner, baseRef, headRef string, cfg *config.Config) (*PRStats, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	diffStats, err := runner.GetDiffStats(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff stats between %s and %s: %w", baseRef, headRef, err)
	}

	commits, _ := runner.GetCommits(baseRef, headRef)

	stats := &PRStats{
		BaseRef:     baseRef,
		HeadRef:     headRef,
		CommitCount: len(commits),
		Commits:     commits,
		Config:      cfg,
	}

	patterns := SensitivePatternsWithConfig(cfg)

	var validDiffStats []gitutil.FileDiffStat
	for _, d := range diffStats {
		if cfg.IsIgnored(d.Path) {
			continue
		}
		validDiffStats = append(validDiffStats, d)

		stats.TotalAdditions += d.Additions
		stats.TotalDeletions += d.Deletions

		isTest := IsTestFile(d.Path)
		isDoc := IsDocumentationFile(d.Path)
		isGen := IsGeneratedFile(d.Path)

		if isTest {
			stats.TestFilesCount++
			stats.TestLinesAdded += d.Additions
			stats.TestLinesDeleted += d.Deletions
		} else if isDoc {
			stats.DocFilesCount++
		} else if isGen {
			stats.GeneratedFilesCount++
			stats.GeneratedLinesAdded += d.Additions
			stats.GeneratedLinesDeleted += d.Deletions
		} else {
			stats.CodeFilesCount++
			stats.CodeLinesAdded += d.Additions
			stats.CodeLinesDeleted += d.Deletions
		}

		for _, p := range patterns {
			if p.Match(d.Path) {
				stats.SensitiveFiles = append(stats.SensitiveFiles, SensitiveMatch{
					Path:        d.Path,
					Category:    p.Category,
					Description: p.Description,
					Additions:   d.Additions,
					Deletions:   d.Deletions,
				})
				break
			}
		}
	}

	stats.FilesChanged = len(validDiffStats)
	stats.NetChange = stats.TotalAdditions - stats.TotalDeletions

	// Sort files by total churn
	sortedFiles := make([]gitutil.FileDiffStat, len(validDiffStats))
	copy(sortedFiles, validDiffStats)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return (sortedFiles[i].Additions + sortedFiles[i].Deletions) > (sortedFiles[j].Additions + sortedFiles[j].Deletions)
	})
	stats.TopChangedFiles = sortedFiles

	if len(sortedFiles) > 0 {
		stats.PrimaryFile = sortedFiles[0].Path
	} else {
		stats.PrimaryFile = "README.md"
	}

	stats.RecommendedReviewers = findRecommendedReviewers(runner, stats)
	calculateRisk(stats)
	return stats, nil
}

// findRecommendedReviewers identifies historical contributors to the modified files.
func findRecommendedReviewers(runner *gitutil.Runner, stats *PRStats) []ReviewerRecommendation {
	if runner == nil || len(stats.TopChangedFiles) == 0 {
		return nil
	}

	// PR author to exclude from self-recommendation
	prAuthor := ""
	if len(stats.Commits) > 0 {
		prAuthor = strings.ToLower(strings.TrimSpace(stats.Commits[0].Author))
	}

	authorScores := make(map[string]int)
	authorFiles := make(map[string]map[string]bool)

	// Inspect top changed files (up to 7)
	limit := min(7, len(stats.TopChangedFiles))
	for i := 0; i < limit; i++ {
		file := stats.TopChangedFiles[i].Path
		if IsGeneratedFile(file) || IsDocumentationFile(file) {
			continue
		}

		authors, err := runner.GetFileAuthors(file, 25)
		if err != nil {
			continue
		}

		for author, count := range authors {
			cleanAuthor := strings.TrimSpace(author)
			if cleanAuthor == "" || strings.ToLower(cleanAuthor) == prAuthor {
				continue
			}
			authorScores[cleanAuthor] += count
			if authorFiles[cleanAuthor] == nil {
				authorFiles[cleanAuthor] = make(map[string]bool)
			}
			authorFiles[cleanAuthor][file] = true
		}
	}

	var recommendations []ReviewerRecommendation
	for author, totalCommits := range authorScores {
		var files []string
		for f := range authorFiles[author] {
			files = append(files, f)
		}
		sort.Strings(files)
		recommendations = append(recommendations, ReviewerRecommendation{
			Author:      author,
			CommitCount: totalCommits,
			TopFiles:    files,
		})
	}

	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].CommitCount > recommendations[j].CommitCount
	})

	if len(recommendations) > 3 {
		recommendations = recommendations[:3]
	}

	return recommendations
}

// calculateRisk computes an SRE risk score (0-100) based on size, test coverage, and blast radius.
func calculateRisk(stats *PRStats) {
	score := 10 // baseline

	maxLines := 800
	staleDays := 14
	minTestRatio := 0.2
	maxDiscussions := 15

	if stats.Config != nil {
		if stats.Config.Thresholds.MaxPRLines > 0 {
			maxLines = stats.Config.Thresholds.MaxPRLines
		}
		if stats.Config.Thresholds.StalePRDays > 0 {
			staleDays = stats.Config.Thresholds.StalePRDays
		}
		if stats.Config.Thresholds.MinTestRatio > 0 {
			minTestRatio = stats.Config.Thresholds.MinTestRatio
		}
		if stats.Config.Thresholds.MaxDiscussions > 0 {
			maxDiscussions = stats.Config.Thresholds.MaxDiscussions
		}
	}

	// Discount generated code volume by 90% for human cognitive review load
	effectiveLines := (stats.CodeLinesAdded + stats.CodeLinesDeleted) +
		(stats.TestLinesAdded + stats.TestLinesDeleted) +
		(stats.GeneratedLinesAdded+stats.GeneratedLinesDeleted)/10

	// 1. Size penalty (scaled dynamically relative to maxLines threshold)
	switch {
	case effectiveLines > maxLines*2:
		score += 35
	case effectiveLines > maxLines:
		score += 25
	case effectiveLines > maxLines/2:
		score += 15
	case effectiveLines > maxLines/5:
		score += 5
	}

	// 2. Blast radius penalty
	if len(stats.SensitiveFiles) > 0 {
		score += min(len(stats.SensitiveFiles)*10, 30)
	}

	// 3. Test delta penalty
	if stats.CodeLinesAdded > 80 && stats.TestLinesAdded == 0 {
		score += 25
	} else if stats.CodeLinesAdded > 0 && stats.TestLinesAdded > 0 {
		ratio := float64(stats.TestLinesAdded) / float64(stats.CodeLinesAdded)
		if ratio < minTestRatio {
			score += 10
		} else if ratio >= minTestRatio*2.5 {
			score -= 10 // reward good test hygiene
		}
	}

	// 4. Lifecycle & Discussion penalties (from GitHub API if available)
	if stats.GitHubMeta != nil {
		if stats.GitHubMeta.Age > time.Duration(staleDays)*24*time.Hour {
			score += 15 // stale PR / branch drift
		}
		if stats.GitHubMeta.TotalDiscussions > maxDiscussions {
			score += 10 // review friction / misalignment
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	stats.RiskScore = score
	switch {
	case score >= 75:
		stats.RiskLevel = "CRITICAL"
	case score >= 50:
		stats.RiskLevel = "HIGH"
	case score >= 25:
		stats.RiskLevel = "MEDIUM"
	default:
		stats.RiskLevel = "LOW"
	}
}

// PopulateSARIF adds the PR analysis rules and results to a SARIF builder.
func (stats *PRStats) PopulateSARIF(builder *sarif.Builder) {
	builder.AddRule(RulePRSummary)
	builder.AddRule(RulePRSize)
	builder.AddRule(RulePRTestRatio)
	builder.AddRule(RulePRBlastRadius)
	builder.AddRule(RulePRStale)
	builder.AddRule(RulePRDiscussionChurn)
	builder.AddRule(RulePRReviewers)

	maxLines := 800
	staleDays := 14
	maxDiscussions := 15
	if stats.Config != nil {
		if stats.Config.Thresholds.MaxPRLines > 0 {
			maxLines = stats.Config.Thresholds.MaxPRLines
		}
		if stats.Config.Thresholds.StalePRDays > 0 {
			staleDays = stats.Config.Thresholds.StalePRDays
		}
		if stats.Config.Thresholds.MaxDiscussions > 0 {
			maxDiscussions = stats.Config.Thresholds.MaxDiscussions
		}
	}

	totalLines := stats.TotalAdditions + stats.TotalDeletions

	// 1. Overall Summary Result
	summaryText := fmt.Sprintf(
		"PR SRE Health: %s risk (score %d/100) | +%d / -%d across %d files (%d commits)",
		stats.RiskLevel, stats.RiskScore, stats.TotalAdditions, stats.TotalDeletions, stats.FilesChanged, stats.CommitCount,
	)

	var sb strings.Builder
	sb.WriteString("### 📊 Pull Request SRE Statistics\n\n")
	sb.WriteString(fmt.Sprintf("- **Risk Assessment:** `%s` (Score: %d/100)\n", stats.RiskLevel, stats.RiskScore))
	sb.WriteString(fmt.Sprintf("- **Volume:** +%d / -%d lines across **%d** files\n", stats.TotalAdditions, stats.TotalDeletions, stats.FilesChanged))
	sb.WriteString(fmt.Sprintf("- **Test Ratio:** %d test lines added vs %d production lines added\n", stats.TestLinesAdded, stats.CodeLinesAdded))
	sb.WriteString(fmt.Sprintf("- **Commits:** %d commit(s)\n", stats.CommitCount))

	if len(stats.RecommendedReviewers) > 0 {
		sb.WriteString("\n**Recommended Reviewers (Domain Experts):**\n")
		for _, r := range stats.RecommendedReviewers {
			sb.WriteString(fmt.Sprintf("- **%s** (%d historical commits)\n", r.Author, r.CommitCount))
		}
	}

	if stats.GitHubMeta != nil {
		sb.WriteString(fmt.Sprintf("- **PR Lifecycle Age:** `%s` (Created: %s)\n", formatDuration(stats.GitHubMeta.Age), stats.GitHubMeta.CreatedAt.Format("2006-01-02 15:04 MST")))
		if stats.GitHubMeta.TimeToFirstReview > 0 {
			sb.WriteString(fmt.Sprintf("- **Time to First Review (TTFR):** `%s`\n", formatDuration(stats.GitHubMeta.TimeToFirstReview)))
		}
		sb.WriteString(fmt.Sprintf("- **Discussions & Reviews:** %d total discussions (%d approvals, %d reviews)\n", stats.GitHubMeta.TotalDiscussions, stats.GitHubMeta.ApprovalsCount, stats.GitHubMeta.ReviewsCount))
	}

	if len(stats.SensitiveFiles) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ **Blast Radius (%d sensitive file(s) modified):**\n", len(stats.SensitiveFiles)))
		for _, sf := range stats.SensitiveFiles {
			sb.WriteString(fmt.Sprintf("  - `%s` (%s: +%d/-%d)\n", sf.Path, sf.Category, sf.Additions, sf.Deletions))
		}
	}

	if len(stats.TopChangedFiles) > 0 {
		sb.WriteString("\n**Top Modified Files:**\n")
		limit := min(5, len(stats.TopChangedFiles))
		for i := 0; i < limit; i++ {
			f := stats.TopChangedFiles[i]
			sb.WriteString(fmt.Sprintf("1. `%s` (+%d / -%d)\n", f.Path, f.Additions, f.Deletions))
		}
	}

	summaryLevel := "note"
	if stats.RiskLevel == "CRITICAL" || stats.RiskLevel == "HIGH" {
		summaryLevel = "warning"
	}

	props := map[string]any{
		"riskLevel":      stats.RiskLevel,
		"riskScore":      stats.RiskScore,
		"additions":      stats.TotalAdditions,
		"deletions":      stats.TotalDeletions,
		"filesChanged":   stats.FilesChanged,
		"testLinesAdded": stats.TestLinesAdded,
		"codeLinesAdded": stats.CodeLinesAdded,
		"sensitiveCount": len(stats.SensitiveFiles),
	}
	if stats.GitHubMeta != nil {
		props["prAgeHours"] = stats.GitHubMeta.Age.Hours()
		props["totalDiscussions"] = stats.GitHubMeta.TotalDiscussions
		props["approvalsCount"] = stats.GitHubMeta.ApprovalsCount
	}

	builder.AddResult(
		RulePRSummary.ID,
		summaryLevel,
		summaryText,
		sb.String(),
		stats.PrimaryFile,
		1,
		props,
	)

	// 2. Excessive Size Result (if applicable)
	if totalLines > maxLines {
		builder.AddResult(
			RulePRSize.ID,
			"warning",
			fmt.Sprintf("Large PR detected: %d lines changed (+%d/-%d). SRE best practice recommends <%d lines to minimize deployment risk.", totalLines, stats.TotalAdditions, stats.TotalDeletions, maxLines/2),
			fmt.Sprintf("### ⚠️ Large Pull Request Warning\nThis PR touches **%d lines of code** across **%d files** (threshold: %d lines).\nLarge changes significantly elevate the mean time to detect (MTTD) and mean time to recover (MTTR) during deployment rollouts.\n\n**Recommendation:** Consider decomposing this PR into smaller, independently testable units.", totalLines, stats.FilesChanged, maxLines),
			stats.PrimaryFile,
			1,
			map[string]any{"totalLines": totalLines, "threshold": maxLines},
		)
	}

	// 3. Test Coverage Delta (if substantial code added without tests)
	if stats.CodeLinesAdded > 80 && stats.TestLinesAdded == 0 {
		builder.AddResult(
			RulePRTestRatio.ID,
			"warning",
			fmt.Sprintf("Zero test additions detected alongside %d lines of new production code.", stats.CodeLinesAdded),
			fmt.Sprintf("### ⚠️ Missing Test Coverage Delta\n**%d lines** of non-test code were added with **0** test modifications.\n\nSRE recommends adding unit or integration tests to maintain high confidence during continuous deployment.", stats.CodeLinesAdded),
			stats.PrimaryFile,
			1,
			map[string]any{"codeLinesAdded": stats.CodeLinesAdded, "testLinesAdded": stats.TestLinesAdded},
		)
	}

	// 4. Sensitive Files / Blast Radius alerts on specific files
	for _, sf := range stats.SensitiveFiles {
		builder.AddResult(
			RulePRBlastRadius.ID,
			"warning",
			fmt.Sprintf("High blast radius file modified: %s (%s)", sf.Path, sf.Category),
			fmt.Sprintf("### 🚨 High Blast Radius: %s\n**Category:** %s\n\n%s\n\nEnsure infrastructure/pipeline changes have passed staging validation before merging.", sf.Path, sf.Category, sf.Description),
			sf.Path,
			1,
			map[string]any{"path": sf.Path, "category": sf.Category},
		)
	}

	// 5. Stale PR Warning (if PR open > configured stale days)
	staleThreshold := time.Duration(staleDays) * 24 * time.Hour
	if stats.GitHubMeta != nil && stats.GitHubMeta.Age > staleThreshold {
		builder.AddResult(
			RulePRStale.ID,
			"warning",
			fmt.Sprintf("Stale PR detected: open for %s (threshold: %d days). SRE recommends rebasing frequently to reduce merge skew.", formatDuration(stats.GitHubMeta.Age), staleDays),
			fmt.Sprintf("### ⚠️ Stale Pull Request Warning\nThis pull request has been open for **%s** (configured threshold: %d days).\nLong-lived branches drift from the primary trunk, significantly increasing integration conflict probability and deployment surprises.", formatDuration(stats.GitHubMeta.Age), staleDays),
			stats.PrimaryFile,
			1,
			map[string]any{"ageHours": stats.GitHubMeta.Age.Hours(), "thresholdDays": staleDays},
		)
	}

	// 6. Discussion Churn / Review Friction
	if stats.GitHubMeta != nil && stats.GitHubMeta.TotalDiscussions > maxDiscussions {
		builder.AddResult(
			RulePRDiscussionChurn.ID,
			"note",
			fmt.Sprintf("High review friction: %d comments across %d reviews (threshold: %d).", stats.GitHubMeta.TotalDiscussions, stats.GitHubMeta.ReviewsCount, maxDiscussions),
			fmt.Sprintf("### 💬 High Review Discussion Volume\nThis PR has accumulated **%d comments** across **%d reviews** (threshold: %d).\nHigh discussion density often signals architectural ambiguity. Consider a synchronous huddle to unblock.", stats.GitHubMeta.TotalDiscussions, stats.GitHubMeta.ReviewsCount, maxDiscussions),
			stats.PrimaryFile,
			1,
			map[string]any{"totalDiscussions": stats.GitHubMeta.TotalDiscussions, "reviews": stats.GitHubMeta.ReviewsCount, "threshold": maxDiscussions},
		)
	}

	// 7. Recommended Domain Expert Reviewers
	if len(stats.RecommendedReviewers) > 0 {
		var revSB strings.Builder
		revSB.WriteString("### 👥 Recommended Domain Expert Reviewers\n")
		revSB.WriteString("Based on historical git commit patterns, the following engineers have deep context on the modified components:\n\n")
		for _, r := range stats.RecommendedReviewers {
			revSB.WriteString(fmt.Sprintf("- **%s**: **%d commits** across `%s`\n", r.Author, r.CommitCount, strings.Join(r.TopFiles, "`, `")))
		}
		revSB.WriteString("\n*Routing PR reviews to domain experts reduces defect escape rates and accelerates turnaround times.*")

		topAuthor := stats.RecommendedReviewers[0].Author
		builder.AddResult(
			RulePRReviewers.ID,
			"note",
			fmt.Sprintf("Recommended reviewer: %s has highest historical context on modified files (%d commits)", topAuthor, stats.RecommendedReviewers[0].CommitCount),
			revSB.String(),
			stats.PrimaryFile,
			1,
			map[string]any{"recommendedReviewers": stats.RecommendedReviewers},
		)
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RiskLevelPriority returns the numeric severity priority for a risk level string.
func RiskLevelPriority(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "CRITICAL":
		return 4
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

// IsRiskThresholdMet checks if currentLevel meets or exceeds thresholdLevel.
func IsRiskThresholdMet(currentLevel, thresholdLevel string) bool {
	curr := RiskLevelPriority(currentLevel)
	thresh := RiskLevelPriority(thresholdLevel)
	return thresh > 0 && curr >= thresh
}

