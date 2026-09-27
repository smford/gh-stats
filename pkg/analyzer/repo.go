package analyzer

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smford/gh-stats/pkg/github"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

// FileTypeCount aggregates count by extension.
type FileTypeCount struct {
	Extension string
	Count     int
}

// ChurnEntry represents a file and how often it was modified.
type ChurnEntry struct {
	Path  string
	Count int
}

// AuthorEntry represents an author and their commit count.
type AuthorEntry struct {
	Author     string
	Commits    int
	Percentage float64
}

// RepoStats holds aggregated repository health and complexity metrics.
type RepoStats struct {
	TotalFiles          int
	TestFilesCount      int
	DocFilesCount       int
	TestFileRatio       float64
	ExtensionBreakdown  []FileTypeCount
	ChurnHotspots       []ChurnEntry
	TopContributors     []AuthorEntry
	TopAuthorPercentage float64
	PrimaryFile         string
	GitHubMeta          *github.RepoMetadata         // Optional GitHub API metadata
	CommitActivity      []github.CommitActivityWeek  // Optional 52-week activity
}

// AnalyzeRepo analyzes the entire git repository.
func AnalyzeRepo(runner *gitutil.Runner, commitLimit int) (*RepoStats, error) {
	if commitLimit <= 0 {
		commitLimit = 200
	}

	trackedFiles, err := runner.ListTrackedFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list tracked files: %w", err)
	}

	stats := &RepoStats{
		TotalFiles:  len(trackedFiles),
		PrimaryFile: "README.md",
	}

	extMap := make(map[string]int)
	for _, f := range trackedFiles {
		if IsTestFile(f) {
			stats.TestFilesCount++
		}
		if IsDocumentationFile(f) {
			stats.DocFilesCount++
		}
		ext := strings.ToLower(filepath.Ext(f))
		if ext == "" {
			ext = "(no extension)"
		}
		extMap[ext]++
	}

	if stats.TotalFiles > 0 {
		stats.TestFileRatio = float64(stats.TestFilesCount) / float64(stats.TotalFiles) * 100
	}

	for ext, count := range extMap {
		stats.ExtensionBreakdown = append(stats.ExtensionBreakdown, FileTypeCount{
			Extension: ext,
			Count:     count,
		})
	}
	sort.Slice(stats.ExtensionBreakdown, func(i, j int) bool {
		return stats.ExtensionBreakdown[i].Count > stats.ExtensionBreakdown[j].Count
	})

	// Hotspots
	churnCounts, _ := runner.GetFileChurnFrequency(commitLimit)
	for path, count := range churnCounts {
		if path != "" {
			stats.ChurnHotspots = append(stats.ChurnHotspots, ChurnEntry{
				Path:  path,
				Count: count,
			})
		}
	}
	sort.Slice(stats.ChurnHotspots, func(i, j int) bool {
		return stats.ChurnHotspots[i].Count > stats.ChurnHotspots[j].Count
	})

	// Contributor concentration
	authorCounts, _ := runner.GetAuthorStats(commitLimit)
	totalCommits := 0
	for _, c := range authorCounts {
		totalCommits += c
	}

	for author, count := range authorCounts {
		pct := 0.0
		if totalCommits > 0 {
			pct = (float64(count) / float64(totalCommits)) * 100.0
		}
		stats.TopContributors = append(stats.TopContributors, AuthorEntry{
			Author:     author,
			Commits:    count,
			Percentage: pct,
		})
	}
	sort.Slice(stats.TopContributors, func(i, j int) bool {
		return stats.TopContributors[i].Commits > stats.TopContributors[j].Commits
	})

	if len(stats.TopContributors) > 0 {
		stats.TopAuthorPercentage = stats.TopContributors[0].Percentage
	}

	if len(stats.ChurnHotspots) > 0 {
		stats.PrimaryFile = stats.ChurnHotspots[0].Path
	} else if len(trackedFiles) > 0 {
		stats.PrimaryFile = trackedFiles[0]
	}

	return stats, nil
}

// PopulateSARIF adds the repository statistics rules and results to a SARIF builder.
func (stats *RepoStats) PopulateSARIF(builder *sarif.Builder) {
	builder.AddRule(RuleRepoSummary)
	builder.AddRule(RuleRepoHotspots)
	builder.AddRule(RuleRepoBusFactor)
	builder.AddRule(RuleRepoAPIMetadata)

	// 1. Repo Summary
	var sb strings.Builder
	sb.WriteString("### 🏛️ Repository Architecture & SRE Health Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Tracked Files:** %d\n", stats.TotalFiles))
	sb.WriteString(fmt.Sprintf("- **Test Files:** %d (%.1f%% of codebase)\n", stats.TestFilesCount, stats.TestFileRatio))
	sb.WriteString(fmt.Sprintf("- **Documentation Files:** %d\n", stats.DocFilesCount))

	if stats.GitHubMeta != nil {
		sb.WriteString(fmt.Sprintf("- **GitHub Ecosystem:** ⭐ %d stars | 🍴 %d forks | ❗ %d open issues/PRs\n",
			stats.GitHubMeta.StargazersCount, stats.GitHubMeta.ForksCount, stats.GitHubMeta.OpenIssuesCount))
	}

	if len(stats.ExtensionBreakdown) > 0 {
		sb.WriteString("\n**Top File Types:**\n")
		limit := min(5, len(stats.ExtensionBreakdown))
		for i := 0; i < limit; i++ {
			e := stats.ExtensionBreakdown[i]
			sb.WriteString(fmt.Sprintf("- `%s`: %d file(s)\n", e.Extension, e.Count))
		}
	}

	if len(stats.TopContributors) > 0 {
		sb.WriteString("\n**Contributor Distribution (Recent History):**\n")
		limit := min(3, len(stats.TopContributors))
		for i := 0; i < limit; i++ {
			c := stats.TopContributors[i]
			sb.WriteString(fmt.Sprintf("- **%s**: %d commits (%.1f%%)\n", c.Author, c.Commits, c.Percentage))
		}
	}

	summaryText := fmt.Sprintf(
		"Repo Health: %d files | Test density: %.1f%% (%d test files) | Top author: %.1f%%",
		stats.TotalFiles, stats.TestFileRatio, stats.TestFilesCount, stats.TopAuthorPercentage,
	)

	builder.AddResult(
		RuleRepoSummary.ID,
		"note",
		summaryText,
		sb.String(),
		stats.PrimaryFile,
		1,
		map[string]any{
			"totalFiles":          stats.TotalFiles,
			"testFileRatio":       stats.TestFileRatio,
			"topAuthorPercentage": stats.TopAuthorPercentage,
		},
	)

	// 2. Churn Hotspots
	if len(stats.ChurnHotspots) > 0 {
		topHotspot := stats.ChurnHotspots[0]
		var hotSB strings.Builder
		hotSB.WriteString("### 🔥 High-Churn Code Hotspots\n")
		hotSB.WriteString("Frequently changed files often correlate with code smell, high cognitive friction, and regression risk:\n\n")
		limit := min(5, len(stats.ChurnHotspots))
		for i := 0; i < limit; i++ {
			h := stats.ChurnHotspots[i]
			hotSB.WriteString(fmt.Sprintf("1. `%s` — changed **%d times**\n", h.Path, h.Count))
		}

		builder.AddResult(
			RuleRepoHotspots.ID,
			"note",
			fmt.Sprintf("Code hotspot detected: %s changed %d times across recent commits", topHotspot.Path, topHotspot.Count),
			hotSB.String(),
			topHotspot.Path,
			1,
			map[string]any{"path": topHotspot.Path, "churnCount": topHotspot.Count},
		)
	}

	// 3. Contributor Bus Factor Alert
	if stats.TopAuthorPercentage > 75.0 && len(stats.TopContributors) > 1 {
		builder.AddResult(
			RuleRepoBusFactor.ID,
			"warning",
			fmt.Sprintf("High contributor concentration: %s authored %.1f%% of recent commits.", stats.TopContributors[0].Author, stats.TopAuthorPercentage),
			fmt.Sprintf("### 👥 Contributor Concentration / Bus Factor Risk\nA single contributor (**%s**) accounts for **%.1f%%** of recent commits.\n\nSRE recommends cross-training and pair programming to diversify domain knowledge and prevent operational bottlenecks.", stats.TopContributors[0].Author, stats.TopAuthorPercentage),
			stats.PrimaryFile,
			1,
			map[string]any{"topAuthor": stats.TopContributors[0].Author, "percentage": stats.TopAuthorPercentage},
		)
	}

	// 4. GitHub API Ecosystem Result (if available)
	if stats.GitHubMeta != nil {
		builder.AddResult(
			RuleRepoAPIMetadata.ID,
			"note",
			fmt.Sprintf("GitHub Ecosystem: %d open issues, %d stars, %d forks", stats.GitHubMeta.OpenIssuesCount, stats.GitHubMeta.StargazersCount, stats.GitHubMeta.ForksCount),
			fmt.Sprintf("### 🌐 GitHub Repository Metrics\n- **Open Issues / Backlog:** `%d`\n- **Stargazers:** `%d`\n- **Forks:** `%d`\n- **Default Branch:** `%s`\n",
				stats.GitHubMeta.OpenIssuesCount, stats.GitHubMeta.StargazersCount, stats.GitHubMeta.ForksCount, stats.GitHubMeta.DefaultBranch),
			stats.PrimaryFile,
			1,
			map[string]any{
				"openIssues": stats.GitHubMeta.OpenIssuesCount,
				"stars":      stats.GitHubMeta.StargazersCount,
				"forks":      stats.GitHubMeta.ForksCount,
			},
		)
	}
}
