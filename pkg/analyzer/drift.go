package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

// DriftStats encapsulates environment drift metrics, divergence counts, and promotion deployment risk.
type DriftStats struct {
	BaseRef             string                 `json:"baseRef"` // Target environment (e.g. origin/production)
	HeadRef             string                 `json:"headRef"` // Source environment (e.g. origin/staging)
	CommitsAhead        int                    `json:"commitsAhead"`
	CommitsBehind       int                    `json:"commitsBehind"`
	UnpromotedCommits   []gitutil.CommitInfo   `json:"unpromotedCommits"`
	BehindCommits       []gitutil.CommitInfo   `json:"behindCommits,omitempty"`
	BreakingChanges     []BreakingChange       `json:"breakingChanges"`
	TotalAdditions      int                    `json:"totalAdditions"`
	TotalDeletions      int                    `json:"totalDeletions"`
	NetChange           int                    `json:"netChange"`
	FilesChanged        int                    `json:"filesChanged"`
	CodeFilesCount      int                    `json:"codeFilesCount"`
	TestFilesCount      int                    `json:"testFilesCount"`
	DocFilesCount       int                    `json:"docFilesCount"`
	GeneratedFilesCount int                    `json:"generatedFilesCount"`
	SensitiveFiles      []SensitiveMatch       `json:"sensitiveFiles"`
	TopChangedFiles     []gitutil.FileDiffStat `json:"topChangedFiles"`
	RiskScore           int                    `json:"riskScore"` // 0-100
	RiskLevel           string                 `json:"riskLevel"` // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	PrimaryFile         string                 `json:"primaryFile"`
	Config              *config.Config         `json:"-"`
}

// AnalyzeDrift audits environment drift and promotion readiness between baseRef (target) and headRef (source).
func AnalyzeDrift(runner *gitutil.Runner, baseRef, headRef string, cfg *config.Config) (*DriftStats, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	ahead, behind, err := runner.GetAheadBehind(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate commit ahead/behind between %s and %s: %w", baseRef, headRef, err)
	}

	diffStats, err := runner.GetDiffStats(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff stats between %s and %s: %w", baseRef, headRef, err)
	}

	unpromotedCommits, err := runner.GetReleaseCommits(baseRef, headRef)
	if err != nil {
		unpromotedCommits, _ = runner.GetCommits(baseRef, headRef)
	}

	var behindCommits []gitutil.CommitInfo
	if behind > 0 {
		behindCommits, err = runner.GetReleaseCommits(headRef, baseRef)
		if err != nil {
			behindCommits, _ = runner.GetCommits(headRef, baseRef)
		}
	}

	stats := &DriftStats{
		BaseRef:           baseRef,
		HeadRef:           headRef,
		CommitsAhead:      ahead,
		CommitsBehind:     behind,
		UnpromotedCommits: unpromotedCommits,
		BehindCommits:     behindCommits,
		Config:            cfg,
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

		if IsTestFile(d.Path) {
			stats.TestFilesCount++
		} else if IsDocumentationFile(d.Path) {
			stats.DocFilesCount++
		} else if IsGeneratedFile(d.Path) {
			stats.GeneratedFilesCount++
		} else {
			stats.CodeFilesCount++
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

	// Identify breaking changes from unpromoted commits
	for _, c := range unpromotedCommits {
		subjLower := strings.ToLower(c.Subject)
		bodyLower := strings.ToLower(c.Body)

		if strings.Contains(subjLower, "!:") ||
			strings.HasPrefix(subjLower, "breaking:") ||
			strings.Contains(subjLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking-change:") {
			stats.BreakingChanges = append(stats.BreakingChanges, BreakingChange{
				CommitHash: c.Hash,
				Subject:    c.Subject,
				Reason:     "conventional_commit",
			})
		}
	}

	stats.CalculateRisk()
	return stats, nil
}

// CalculateRisk evaluates promotion deployment risk score (0-100) and risk level.
func (s *DriftStats) CalculateRisk() {
	if s.CommitsAhead == 0 && s.CommitsBehind == 0 && s.FilesChanged == 0 {
		s.RiskScore = 0
		s.RiskLevel = "LOW"
		return
	}

	score := 5 // Baseline evaluation overhead

	// 1. Commits Ahead (Unpromoted branch drift)
	if s.CommitsAhead >= 50 {
		score += 30
	} else if s.CommitsAhead >= 25 {
		score += 20
	} else if s.CommitsAhead >= 10 {
		score += 15
	} else if s.CommitsAhead > 0 {
		score += 5
	}

	// 2. Commits Behind (Upstream divergence / un-backmerged hotfixes)
	if s.CommitsBehind >= 10 {
		score += 20
	} else if s.CommitsBehind > 0 {
		score += 10
	}

	// 3. Breaking changes
	if len(s.BreakingChanges) > 0 {
		score += min(50, len(s.BreakingChanges)*25)
	}

	// 4. Sensitive infrastructure, schema migrations, and CI/CD
	hasMigrations := false
	hasInfraOrAuth := false
	hasCICD := false
	for _, sf := range s.SensitiveFiles {
		if sf.Category == "Database Migrations" {
			hasMigrations = true
		}
		if sf.Category == "Infrastructure & Containers" || sf.Category == "Auth & Security" {
			hasInfraOrAuth = true
		}
		if sf.Category == "CI/CD Pipelines" {
			hasCICD = true
		}
	}

	if hasMigrations {
		score += 20
	}
	if hasInfraOrAuth {
		score += 15
	}
	if hasCICD {
		score += 10
	}

	// 5. Cumulative unpromoted change volume
	totalLines := s.TotalAdditions + s.TotalDeletions
	if totalLines > 3000 {
		score += 25
	} else if totalLines > 1500 {
		score += 15
	} else if totalLines > 500 {
		score += 10
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

// PopulateSARIF populates SARIF rules and findings for environment drift and promotion assessment.
func (s *DriftStats) PopulateSARIF(b *sarif.Builder) {
	b.AddRule(RuleDriftSummary)

	primaryFile := s.PrimaryFile
	if primaryFile == "" {
		primaryFile = "README.md"
	}

	// 1. Overall Environment Drift Summary Finding
	summaryMsg := fmt.Sprintf(
		"Environment Drift (%s...%s): %s promotion risk (score %d/100) | %d commit(s) ahead, %d commit(s) behind | +%d / -%d across %d unpromoted file(s)",
		s.BaseRef, s.HeadRef, s.RiskLevel, s.RiskScore, s.CommitsAhead, s.CommitsBehind, s.TotalAdditions, s.TotalDeletions, s.FilesChanged,
	)

	var sb strings.Builder
	sb.WriteString("### 🌐 Environment Drift & Promotion Assessment Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Comparison:** `%s` (base/target) ➔ `%s` (head/source)\n", s.BaseRef, s.HeadRef))
	sb.WriteString(fmt.Sprintf("- **Promotion Risk:** `%s` (Score: %d/100)\n", s.RiskLevel, s.RiskScore))
	sb.WriteString(fmt.Sprintf("- **Commits Ahead (Unpromoted):** `%d` commit(s)\n", s.CommitsAhead))
	sb.WriteString(fmt.Sprintf("- **Commits Behind (Divergence):** `%d` commit(s)\n", s.CommitsBehind))
	sb.WriteString(fmt.Sprintf("- **Unpromoted Volume:** +%d / -%d lines across **%d** file(s)\n", s.TotalAdditions, s.TotalDeletions, s.FilesChanged))

	if len(s.BreakingChanges) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ **Unpromoted Breaking Changes Detected (%d):**\n", len(s.BreakingChanges)))
		for _, bc := range s.BreakingChanges {
			if bc.CommitHash != "" {
				sb.WriteString(fmt.Sprintf("  - [`%s`] %s (%s)\n", bc.CommitHash, bc.Subject, bc.Reason))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s (%s)\n", bc.Subject, bc.Reason))
			}
		}
	}

	if len(s.SensitiveFiles) > 0 {
		sb.WriteString(fmt.Sprintf("\n🛡️ **Unpromoted High Blast Radius Files (%d):**\n", len(s.SensitiveFiles)))
		for _, sf := range s.SensitiveFiles {
			sb.WriteString(fmt.Sprintf("  - `%s` (%s: +%d/-%d)\n", sf.Path, sf.Category, sf.Additions, sf.Deletions))
		}
	}

	props := map[string]any{
		"baseRef":             s.BaseRef,
		"headRef":             s.HeadRef,
		"riskLevel":           s.RiskLevel,
		"riskScore":           s.RiskScore,
		"commitsAhead":        s.CommitsAhead,
		"commitsBehind":       s.CommitsBehind,
		"additions":           s.TotalAdditions,
		"deletions":           s.TotalDeletions,
		"netChange":           s.NetChange,
		"filesChanged":        s.FilesChanged,
		"breakingChanges":     len(s.BreakingChanges),
		"sensitiveFilesCount": len(s.SensitiveFiles),
	}

	b.AddResult(
		RuleDriftSummary.ID,
		"note",
		summaryMsg,
		sb.String(),
		primaryFile,
		1,
		props,
	)

	// 2. Excessive Branch Drift Warning
	if s.CommitsAhead >= 15 || s.CommitsBehind >= 5 {
		b.AddRule(RuleDriftExcessive)
		excessiveMsg := fmt.Sprintf(
			"Excessive environment drift between %s and %s: %d commit(s) ahead, %d commit(s) behind",
			s.BaseRef, s.HeadRef, s.CommitsAhead, s.CommitsBehind,
		)
		var esb strings.Builder
		esb.WriteString(fmt.Sprintf("### ⚠️ Excessive Environment Drift (`%s` vs `%s`)\n\n", s.BaseRef, s.HeadRef))
		esb.WriteString(fmt.Sprintf("- Candidate environment has accumulated **%d** unpromoted commits ahead.\n", s.CommitsAhead))
		esb.WriteString(fmt.Sprintf("- Candidate environment is missing **%d** upstream commits from target base.\n", s.CommitsBehind))
		esb.WriteString("- SRE Recommendation: Promote pending changes or rebase/back-merge divergent commits to restore parity.\n")

		driftProps := map[string]any{
			"commitsAhead":  s.CommitsAhead,
			"commitsBehind": s.CommitsBehind,
		}
		b.AddResult(
			RuleDriftExcessive.ID,
			"warning",
			excessiveMsg,
			esb.String(),
			primaryFile,
			1,
			driftProps,
		)
	}

	// 3. Unpromoted Breaking Changes
	if len(s.BreakingChanges) > 0 {
		b.AddRule(RuleDriftUnpromotedBreaking)
		breakingMsg := fmt.Sprintf("%d unpromoted breaking change(s) or database migration(s) detected between %s and %s", len(s.BreakingChanges), s.BaseRef, s.HeadRef)
		var bsb strings.Builder
		bsb.WriteString(fmt.Sprintf("### ⚠️ Unpromoted Breaking Changes (`%s` ➔ `%s`)\n\n", s.HeadRef, s.BaseRef))
		for _, bc := range s.BreakingChanges {
			bsb.WriteString(fmt.Sprintf("- %s\n", bc.Subject))
		}
		breakingProps := map[string]any{
			"breakingChangesCount": len(s.BreakingChanges),
		}
		b.AddResult(
			RuleDriftUnpromotedBreaking.ID,
			"warning",
			breakingMsg,
			bsb.String(),
			primaryFile,
			1,
			breakingProps,
		)
	}

	// 4. Unpromoted Sensitive Files
	if len(s.SensitiveFiles) > 0 {
		b.AddRule(RuleDriftSensitive)
		catSet := make(map[string]bool)
		var catList []string
		for _, sf := range s.SensitiveFiles {
			if !catSet[sf.Category] {
				catSet[sf.Category] = true
				catList = append(catList, sf.Category)
			}
		}
		blastMsg := fmt.Sprintf("%d unpromoted high-blast-radius file(s) awaiting promotion from %s to %s (%s)", len(s.SensitiveFiles), s.HeadRef, s.BaseRef, strings.Join(catList, ", "))
		var asb strings.Builder
		asb.WriteString(fmt.Sprintf("### 🛡️ Unpromoted High Blast Radius Files (`%s` ➔ `%s`)\n\n", s.HeadRef, s.BaseRef))
		for _, sf := range s.SensitiveFiles {
			asb.WriteString(fmt.Sprintf("- `%s` (%s)\n", sf.Path, sf.Category))
		}
		blastProps := map[string]any{
			"sensitiveCount": len(s.SensitiveFiles),
			"categories":     catList,
		}
		b.AddResult(
			RuleDriftSensitive.ID,
			"warning",
			blastMsg,
			asb.String(),
			primaryFile,
			1,
			blastProps,
		)
	}
}
