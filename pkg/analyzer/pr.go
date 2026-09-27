package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

// PRStats contains calculated statistics and SRE metrics for a pull request.
type PRStats struct {
	BaseRef            string
	HeadRef            string
	TotalAdditions     int
	TotalDeletions     int
	NetChange          int
	FilesChanged       int
	TestFilesCount     int
	DocFilesCount      int
	CodeFilesCount     int
	TestLinesAdded     int
	TestLinesDeleted   int
	CodeLinesAdded     int
	CodeLinesDeleted   int
	SensitiveFiles     []SensitiveMatch
	TopChangedFiles    []gitutil.FileDiffStat
	CommitCount        int
	Commits            []gitutil.CommitInfo
	RiskScore          int    // 0-100 (higher = riskier)
	RiskLevel          string // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	PrimaryFile        string // representative file for PR-wide SARIF results
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
func AnalyzePR(runner *gitutil.Runner, baseRef, headRef string) (*PRStats, error) {
	diffStats, err := runner.GetDiffStats(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff stats between %s and %s: %w", baseRef, headRef, err)
	}

	commits, _ := runner.GetCommits(baseRef, headRef)

	stats := &PRStats{
		BaseRef:      baseRef,
		HeadRef:      headRef,
		FilesChanged: len(diffStats),
		CommitCount:  len(commits),
		Commits:      commits,
	}

	patterns := DefaultSensitivePatterns()

	for _, d := range diffStats {
		stats.TotalAdditions += d.Additions
		stats.TotalDeletions += d.Deletions

		isTest := IsTestFile(d.Path)
		isDoc := IsDocumentationFile(d.Path)

		if isTest {
			stats.TestFilesCount++
			stats.TestLinesAdded += d.Additions
			stats.TestLinesDeleted += d.Deletions
		} else if isDoc {
			stats.DocFilesCount++
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

	stats.NetChange = stats.TotalAdditions - stats.TotalDeletions

	// Sort files by total churn
	sortedFiles := make([]gitutil.FileDiffStat, len(diffStats))
	copy(sortedFiles, diffStats)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return (sortedFiles[i].Additions + sortedFiles[i].Deletions) > (sortedFiles[j].Additions + sortedFiles[j].Deletions)
	})
	stats.TopChangedFiles = sortedFiles

	if len(sortedFiles) > 0 {
		stats.PrimaryFile = sortedFiles[0].Path
	} else {
		stats.PrimaryFile = "README.md"
	}

	calculateRisk(stats)
	return stats, nil
}

// calculateRisk computes an SRE risk score (0-100) based on size, test coverage, and blast radius.
func calculateRisk(stats *PRStats) {
	score := 10 // baseline

	totalLines := stats.TotalAdditions + stats.TotalDeletions

	// 1. Size penalty
	switch {
	case totalLines > 1500:
		score += 35
	case totalLines > 800:
		score += 25
	case totalLines > 400:
		score += 15
	case totalLines > 150:
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
		if ratio < 0.2 {
			score += 10
		} else if ratio >= 0.5 {
			score -= 10 // reward good test hygiene
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
	if totalLines > 800 {
		builder.AddResult(
			RulePRSize.ID,
			"warning",
			fmt.Sprintf("Large PR detected: %d lines changed (+%d/-%d). SRE best practice recommends <400 lines to minimize deployment risk.", totalLines, stats.TotalAdditions, stats.TotalDeletions),
			fmt.Sprintf("### ⚠️ Large Pull Request Warning\nThis PR touches **%d lines of code** across **%d files**.\nLarge changes significantly elevate the mean time to detect (MTTD) and mean time to recover (MTTR) during deployment rollouts.\n\n**Recommendation:** Consider decomposing this PR into smaller, independently testable units.", totalLines, stats.FilesChanged),
			stats.PrimaryFile,
			1,
			map[string]any{"totalLines": totalLines, "threshold": 800},
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
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
