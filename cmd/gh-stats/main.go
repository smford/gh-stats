package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/reporter"
	"github.com/smford/gh-stats/pkg/sarif"
)

func main() {
	var (
		flagTarget      string
		flagBaseRef     string
		flagHeadRef     string
		flagOutput      string
		flagRepoPath    string
		flagCommitLimit int
		flagQuiet       bool
	)

	flag.StringVar(&flagTarget, "target", getEnvDefault("INPUT_TARGET", getEnvDefault("INPUT_MODE", "auto")), "Target scope: 'pr', 'repo', or 'auto'")
	flag.StringVar(&flagBaseRef, "base", getEnvDefault("INPUT_BASE_REF", os.Getenv("GITHUB_BASE_REF")), "Base ref for PR comparison (e.g. origin/main)")
	flag.StringVar(&flagHeadRef, "head", getEnvDefault("INPUT_HEAD_REF", "HEAD"), "Head ref for PR comparison (default HEAD)")
	flag.StringVar(&flagOutput, "output", getEnvDefault("INPUT_SARIF_OUTPUT", getEnvDefault("INPUT_OUTPUT", "gh-stats.sarif")), "Path to output SARIF file")
	flag.StringVar(&flagRepoPath, "repo-path", getEnvDefault("INPUT_REPO_PATH", "."), "Path to git repository")
	flag.IntVar(&flagCommitLimit, "commit-limit", 200, "Maximum commit history to examine for repo hotspots")
	flag.BoolVar(&flagQuiet, "quiet", false, "Suppress stdout output")
	flag.Parse()

	// Resolve auto mode
	target := strings.ToLower(flagTarget)
	if target == "auto" {
		if os.Getenv("GITHUB_EVENT_NAME") == "pull_request" || os.Getenv("GITHUB_BASE_REF") != "" {
			target = "pr"
		} else {
			target = "repo"
		}
	}

	if !flagQuiet {
		fmt.Printf("🔍 gh-stats: running in [%s] mode\n", strings.ToUpper(target))
	}

	runner := gitutil.NewRunner(flagRepoPath)
	builder := sarif.NewBuilder()

	var markdownSummary string

	switch target {
	case "pr":
		baseRef := flagBaseRef
		if baseRef == "" {
			baseRef = runner.GetDefaultBaseRef()
		}
		// In GitHub Actions pull_request, baseRef is often just "main"; prefix origin/ if needed
		if !strings.HasPrefix(baseRef, "origin/") && baseRef != "HEAD~1" {
			if _, err := runner.Exec("rev-parse", "--verify", "origin/"+baseRef); err == nil {
				baseRef = "origin/" + baseRef
			}
		}

		if !flagQuiet {
			fmt.Printf("📊 Analyzing PR diff: %s ... %s\n", baseRef, flagHeadRef)
		}

		stats, err := analyzer.AnalyzePR(runner, baseRef, flagHeadRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error analyzing PR: %v\n", err)
			os.Exit(1)
		}

		stats.PopulateSARIF(builder)
		markdownSummary = reporter.GeneratePRSummary(stats)

		if !flagQuiet {
			fmt.Printf("✅ PR Analysis Complete: Risk=%s (%d/100), Files=%d, Additions=+%d, Deletions=-%d\n",
				stats.RiskLevel, stats.RiskScore, stats.FilesChanged, stats.TotalAdditions, stats.TotalDeletions)
		}

	case "repo":
		if !flagQuiet {
			fmt.Printf("🏛️ Analyzing Repository (commit window: %d)...\n", flagCommitLimit)
		}

		stats, err := analyzer.AnalyzeRepo(runner, flagCommitLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error analyzing repository: %v\n", err)
			os.Exit(1)
		}

		stats.PopulateSARIF(builder)
		markdownSummary = reporter.GenerateRepoSummary(stats)

		if !flagQuiet {
			fmt.Printf("✅ Repo Analysis Complete: Total Files=%d, Test Density=%.1f%%, Hotspots Identified=%d\n",
				stats.TotalFiles, stats.TestFileRatio, len(stats.ChurnHotspots))
		}

	default:
		fmt.Fprintf(os.Stderr, "❌ Invalid target '%s'. Supported targets are 'pr' or 'repo'.\n", target)
		os.Exit(1)
	}

	// Ensure output directory exists
	outDir := filepath.Dir(flagOutput)
	if outDir != "" && outDir != "." {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create directory %s: %v\n", outDir, err)
			os.Exit(1)
		}
	}

	// Write SARIF report
	if err := builder.WriteFile(flagOutput); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to write SARIF file: %v\n", err)
		os.Exit(1)
	}

	if !flagQuiet {
		fmt.Printf("📄 SARIF report written to: %s\n", flagOutput)
	}

	// Write to GitHub Step Summary
	if err := reporter.WriteStepSummary(markdownSummary); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Warning writing to GITHUB_STEP_SUMMARY: %v\n", err)
	}

	// Set GitHub Action outputs
	setGithubOutput("sarif-file", flagOutput)
	setGithubOutput("target", target)

	if !flagQuiet && os.Getenv("GITHUB_ACTIONS") == "" {
		fmt.Println("\n" + markdownSummary)
	}
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func setGithubOutput(key, value string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return
	}
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s=%s\n", key, value)
}
