package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

// ReleaseStats encapsulates deployment readiness, commit volume, and risk delta across releases.
type ReleaseStats struct {
	BaseRef             string
	HeadRef             string
	TotalCommits        int
	Commits             []gitutil.CommitInfo
	Contributors        []ContributorStat
	CategorizedCommits  CommitCategories
	BreakingChanges     []BreakingChange
	TotalAdditions      int
	TotalDeletions      int
	NetChange           int
	FilesChanged        int
	CodeFilesCount      int
	TestFilesCount      int
	DocFilesCount       int
	GeneratedFilesCount int
	CodeLinesAdded      int
	CodeLinesDeleted    int
	TestLinesAdded      int
	TestLinesDeleted    int
	TestRatio           float64
	SensitiveFiles      []SensitiveMatch
	TopChangedFiles     []gitutil.FileDiffStat
	RiskScore           int    // 0-100
	RiskLevel           string // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	PrimaryFile         string
	Config              *config.Config
}

// ContributorStat represents commit count and share for an author.
type ContributorStat struct {
	Name        string  `json:"name"`
	CommitCount int     `json:"commitCount"`
	Percentage  float64 `json:"percentage"`
}

// CommitCategories groups commits by semantic conventional types.
type CommitCategories struct {
	Features    []gitutil.CommitInfo `json:"features"`
	Fixes       []gitutil.CommitInfo `json:"fixes"`
	Performance []gitutil.CommitInfo `json:"performance"`
	Refactoring []gitutil.CommitInfo `json:"refactoring"`
	Docs        []gitutil.CommitInfo `json:"docs"`
	Chores      []gitutil.CommitInfo `json:"chores"`
	Other       []gitutil.CommitInfo `json:"other"`
}

// BreakingChange details a commit or file change introducing a breaking change.
type BreakingChange struct {
	CommitHash string `json:"commitHash,omitempty"`
	Subject    string `json:"subject"`
	Reason     string `json:"reason"` // "conventional_commit", "migration_file", "body_notice"
}

// AnalyzeRelease compares baseRef and headRef to calculate release metrics and deployment risk.
func AnalyzeRelease(runner *gitutil.Runner, baseRef, headRef string, cfg *config.Config) (*ReleaseStats, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	diffStats, err := runner.GetDiffStats(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff stats between %s and %s: %w", baseRef, headRef, err)
	}

	commits, err := runner.GetReleaseCommits(baseRef, headRef)
	if err != nil {
		// Fallback to GetCommits
		commits, _ = runner.GetCommits(baseRef, headRef)
	}

	stats := &ReleaseStats{
		BaseRef:      baseRef,
		HeadRef:      headRef,
		TotalCommits: len(commits),
		Commits:      commits,
		Config:       cfg,
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
				if p.Category == "Database Migrations" {
					stats.BreakingChanges = append(stats.BreakingChanges, BreakingChange{
						Subject: fmt.Sprintf("Database Migration: %s (+%d/-%d)", d.Path, d.Additions, d.Deletions),
						Reason:  "migration_file",
					})
				}
				break
			}
		}
	}

	stats.FilesChanged = len(validDiffStats)
	stats.NetChange = stats.TotalAdditions - stats.TotalDeletions

	if stats.CodeLinesAdded > 0 {
		stats.TestRatio = float64(stats.TestLinesAdded) / float64(stats.CodeLinesAdded)
	}

	// Sort modified files by magnitude of change
	sort.Slice(validDiffStats, func(i, j int) bool {
		return (validDiffStats[i].Additions + validDiffStats[i].Deletions) > (validDiffStats[j].Additions + validDiffStats[j].Deletions)
	})
	stats.TopChangedFiles = validDiffStats
	if len(validDiffStats) > 0 {
		stats.PrimaryFile = validDiffStats[0].Path
	} else {
		stats.PrimaryFile = "README.md"
	}

	// Categorize commits and identify breaking changes
	authorCounts := make(map[string]int)
	for _, c := range commits {
		if c.Author != "" {
			authorCounts[c.Author]++
		}

		subjLower := strings.ToLower(c.Subject)
		bodyLower := strings.ToLower(c.Body)

		isBreaking := false
		if strings.Contains(subjLower, "!:") ||
			strings.HasPrefix(subjLower, "breaking:") ||
			strings.Contains(subjLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking-change:") {
			isBreaking = true
			stats.BreakingChanges = append(stats.BreakingChanges, BreakingChange{
				CommitHash: c.Hash,
				Subject:    c.Subject,
				Reason:     "conventional_commit",
			})
		}

		// Categorize commit
		if isConventionalMatch(subjLower, "feat") {
			stats.CategorizedCommits.Features = append(stats.CategorizedCommits.Features, c)
		} else if isConventionalMatch(subjLower, "fix") {
			stats.CategorizedCommits.Fixes = append(stats.CategorizedCommits.Fixes, c)
		} else if isConventionalMatch(subjLower, "perf") {
			stats.CategorizedCommits.Performance = append(stats.CategorizedCommits.Performance, c)
		} else if isConventionalMatch(subjLower, "refactor") {
			stats.CategorizedCommits.Refactoring = append(stats.CategorizedCommits.Refactoring, c)
		} else if isConventionalMatch(subjLower, "docs") {
			stats.CategorizedCommits.Docs = append(stats.CategorizedCommits.Docs, c)
		} else if isConventionalMatch(subjLower, "chore") ||
			isConventionalMatch(subjLower, "ci") ||
			isConventionalMatch(subjLower, "test") ||
			isConventionalMatch(subjLower, "build") {
			stats.CategorizedCommits.Chores = append(stats.CategorizedCommits.Chores, c)
		} else {
			if !isBreaking {
				stats.CategorizedCommits.Other = append(stats.CategorizedCommits.Other, c)
			}
		}
	}

	// Compute contributor stats
	for name, count := range authorCounts {
		pct := 0.0
		if len(commits) > 0 {
			pct = (float64(count) / float64(len(commits))) * 100.0
		}
		stats.Contributors = append(stats.Contributors, ContributorStat{
			Name:        name,
			CommitCount: count,
			Percentage:  pct,
		})
	}
	sort.Slice(stats.Contributors, func(i, j int) bool {
		return stats.Contributors[i].CommitCount > stats.Contributors[j].CommitCount
	})

	stats.CalculateRisk()
	return stats, nil
}

func isConventionalMatch(subject, prefix string) bool {
	if strings.HasPrefix(subject, prefix+":") || strings.HasPrefix(subject, prefix+"!:") {
		return true
	}
	if strings.HasPrefix(subject, prefix+"(") {
		idx := strings.Index(subject, "):")
		if idx != -1 {
			return true
		}
		idxExcl := strings.Index(subject, ")!:")
		if idxExcl != -1 {
			return true
		}
	}
	return false
}

// CalculateRisk evaluates release delta risk score (0-100) and risk level.
func (s *ReleaseStats) CalculateRisk() {
	score := 10 // Baseline release overhead

	// Breaking changes
	if len(s.BreakingChanges) > 0 {
		score += min(50, len(s.BreakingChanges)*25)
	}

	// High blast radius files
	hasMigrations := false
	hasInfraOrAuth := false
	for _, sf := range s.SensitiveFiles {
		if sf.Category == "Database Migrations" {
			hasMigrations = true
		}
		if sf.Category == "Infrastructure & Containers" || sf.Category == "Auth & Security" || sf.Category == "CI/CD Pipelines" {
			hasInfraOrAuth = true
		}
	}

	if hasMigrations {
		score += 20
	}
	if hasInfraOrAuth {
		score += 15
	}

	// Volume risk
	totalLines := s.TotalAdditions + s.TotalDeletions
	if totalLines > 3000 {
		score += 25
	} else if totalLines > 1500 {
		score += 15
	} else if totalLines > 800 {
		score += 10
	}

	// Test coverage delta risk
	if s.CodeLinesAdded > 150 {
		if s.TestRatio < 0.15 {
			score += 20
		} else if s.TestRatio < 0.3 {
			score += 10
		}
	}

	if score > 100 {
		score = 100
	}
	s.RiskScore = score

	switch {
	case score >= 80:
		s.RiskLevel = "CRITICAL"
	case score >= 60:
		s.RiskLevel = "HIGH"
	case score >= 30:
		s.RiskLevel = "MEDIUM"
	default:
		s.RiskLevel = "LOW"
	}
}

// PopulateSARIF populates SARIF rules and findings for the release delta.
func (s *ReleaseStats) PopulateSARIF(b *sarif.Builder) {
	b.AddRule(RuleReleaseSummary)

	primaryFile := s.PrimaryFile
	if primaryFile == "" {
		primaryFile = "README.md"
	}

	// 1. Overall Release Summary Finding
	summaryMsg := fmt.Sprintf(
		"Release Delta (%s...%s): %s risk (score %d/100) | %d commits by %d contributors | +%d / -%d across %d files",
		s.BaseRef, s.HeadRef, s.RiskLevel, s.RiskScore, s.TotalCommits, len(s.Contributors), s.TotalAdditions, s.TotalDeletions, s.FilesChanged,
	)

	var sb strings.Builder
	sb.WriteString("### 📦 Release Delta SRE Readiness Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Comparison:** `%s...%s`\n", s.BaseRef, s.HeadRef))
	sb.WriteString(fmt.Sprintf("- **Risk Level:** `%s` (Score: %d/100)\n", s.RiskLevel, s.RiskScore))
	sb.WriteString(fmt.Sprintf("- **Volume:** +%d / -%d lines across **%d** files (%d commits)\n", s.TotalAdditions, s.TotalDeletions, s.FilesChanged, s.TotalCommits))
	sb.WriteString(fmt.Sprintf("- **Test Ratio:** %d test lines added vs %d production lines added (%.1f%%)\n", s.TestLinesAdded, s.CodeLinesAdded, s.TestRatio*100))
	sb.WriteString(fmt.Sprintf("- **Contributors:** %d unique contributor(s)\n", len(s.Contributors)))

	if len(s.BreakingChanges) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ **Breaking Changes Detected (%d):**\n", len(s.BreakingChanges)))
		for _, bc := range s.BreakingChanges {
			if bc.CommitHash != "" {
				sb.WriteString(fmt.Sprintf("  - [`%s`] %s (%s)\n", bc.CommitHash, bc.Subject, bc.Reason))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s (%s)\n", bc.Subject, bc.Reason))
			}
		}
	}

	if len(s.SensitiveFiles) > 0 {
		sb.WriteString(fmt.Sprintf("\n🛡️ **Blast Radius (%d sensitive file(s) modified):**\n", len(s.SensitiveFiles)))
		for _, sf := range s.SensitiveFiles {
			sb.WriteString(fmt.Sprintf("  - `%s` (%s: +%d/-%d)\n", sf.Path, sf.Category, sf.Additions, sf.Deletions))
		}
	}

	props := map[string]any{
		"baseRef":             s.BaseRef,
		"headRef":             s.HeadRef,
		"riskLevel":           s.RiskLevel,
		"riskScore":           s.RiskScore,
		"totalCommits":        s.TotalCommits,
		"contributorsCount":   len(s.Contributors),
		"additions":           s.TotalAdditions,
		"deletions":           s.TotalDeletions,
		"netChange":           s.NetChange,
		"filesChanged":        s.FilesChanged,
		"breakingChanges":     len(s.BreakingChanges),
		"sensitiveFilesCount": len(s.SensitiveFiles),
		"testLinesAdded":      s.TestLinesAdded,
		"codeLinesAdded":      s.CodeLinesAdded,
		"testRatio":           s.TestRatio,
	}

	b.AddResult(
		RuleReleaseSummary.ID,
		"note",
		summaryMsg,
		sb.String(),
		primaryFile,
		1,
		props,
	)

	// 2. Breaking Changes Warning
	if len(s.BreakingChanges) > 0 {
		b.AddRule(RuleReleaseBreaking)
		breakingMsg := fmt.Sprintf("%d breaking change(s) or database migration(s) detected in release %s...%s", len(s.BreakingChanges), s.BaseRef, s.HeadRef)
		var bsb strings.Builder
		bsb.WriteString(fmt.Sprintf("### ⚠️ Breaking Changes in Release `%s...%s`\n\n", s.BaseRef, s.HeadRef))
		for _, bc := range s.BreakingChanges {
			bsb.WriteString(fmt.Sprintf("- %s\n", bc.Subject))
		}
		breakingProps := map[string]any{
			"breakingChangesCount": len(s.BreakingChanges),
		}
		b.AddResult(
			RuleReleaseBreaking.ID,
			"warning",
			breakingMsg,
			bsb.String(),
			primaryFile,
			1,
			breakingProps,
		)
	}

	// 3. High Blast Radius Warning
	if len(s.SensitiveFiles) > 0 {
		b.AddRule(RuleReleaseBlastRadius)
		catSet := make(map[string]bool)
		var catList []string
		for _, sf := range s.SensitiveFiles {
			if !catSet[sf.Category] {
				catSet[sf.Category] = true
				catList = append(catList, sf.Category)
			}
		}
		blastMsg := fmt.Sprintf("%d high-blast-radius file(s) modified in release %s...%s (%s)", len(s.SensitiveFiles), s.BaseRef, s.HeadRef, strings.Join(catList, ", "))
		var asb strings.Builder
		asb.WriteString(fmt.Sprintf("### 🛡️ High Blast Radius Files in Release `%s...%s`\n\n", s.BaseRef, s.HeadRef))
		for _, sf := range s.SensitiveFiles {
			asb.WriteString(fmt.Sprintf("- `%s` (%s)\n", sf.Path, sf.Category))
		}
		blastProps := map[string]any{
			"sensitiveCount": len(s.SensitiveFiles),
			"categories":     catList,
		}
		b.AddResult(
			RuleReleaseBlastRadius.ID,
			"warning",
			blastMsg,
			asb.String(),
			primaryFile,
			1,
			blastProps,
		)
	}
}
