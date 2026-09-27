package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/github"
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
		flagConfigPath  string
		flagToken       string
		flagPRNumber    int
		flagRepoSlug    string
		flagFailOn      string
		flagCommentPR   bool
		flagQuiet       bool
	)

	flag.StringVar(&flagTarget, "target", getEnvDefault("INPUT_TARGET", getEnvDefault("INPUT_MODE", "auto")), "Target scope: 'pr', 'repo', or 'auto'")
	flag.StringVar(&flagBaseRef, "base", getEnvDefault("INPUT_BASE_REF", os.Getenv("GITHUB_BASE_REF")), "Base ref for PR comparison (e.g. origin/main)")
	flag.StringVar(&flagHeadRef, "head", getEnvDefault("INPUT_HEAD_REF", "HEAD"), "Head ref for PR comparison (default HEAD)")
	flag.StringVar(&flagOutput, "output", getEnvDefault("INPUT_SARIF_OUTPUT", getEnvDefault("INPUT_OUTPUT", "gh-stats.sarif")), "Path to output SARIF file")
	flag.StringVar(&flagRepoPath, "repo-path", getEnvDefault("INPUT_REPO_PATH", "."), "Path to git repository")
	flag.IntVar(&flagCommitLimit, "commit-limit", 0, "Maximum commit history to examine for repo hotspots (default 200)")
	flag.StringVar(&flagConfigPath, "config", getEnvDefault("INPUT_CONFIG_PATH", getEnvDefault("INPUT_CONFIG", "")), "Path to .gh-stats.yml configuration file")
	flag.StringVar(&flagToken, "token", getEnvDefault("INPUT_TOKEN", os.Getenv("GITHUB_TOKEN")), "GitHub API token for metadata enrichment")
	flag.IntVar(&flagPRNumber, "pr-number", 0, "Pull request number (auto-detected if omitted)")
	flag.StringVar(&flagRepoSlug, "repo", getEnvDefault("GITHUB_REPOSITORY", ""), "GitHub repository slug owner/repo (auto-detected if omitted)")
	flag.StringVar(&flagFailOn, "fail-on", getEnvDefault("INPUT_FAIL_ON", ""), "Fail workflow if PR risk meets/exceeds threshold (e.g. 'HIGH', 'CRITICAL')")
	flag.BoolVar(&flagCommentPR, "comment-pr", getEnvDefault("INPUT_COMMENT_PR", "false") == "true", "Post or update a sticky summary comment on the PR")
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

	cfg, err := config.LoadConfig(flagConfigPath, flagRepoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if flagFailOn == "" && cfg.FailOn != "" {
		flagFailOn = cfg.FailOn
	}

	// Auto-detect repo slug from git remote if not provided
	repoSlug := flagRepoSlug
	if repoSlug == "" {
		if remoteURL, err := runner.Exec("config", "--get", "remote.origin.url"); err == nil {
			if detected, err := github.DetectRepoSlug(remoteURL); err == nil {
				repoSlug = detected
			}
		}
	}

	var ghClient *github.Client
	if flagToken != "" {
		ghClient = github.NewClient(flagToken)
	}

	var markdownSummary string
	var prStats *analyzer.PRStats

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

		stats, err := analyzer.AnalyzePR(runner, baseRef, flagHeadRef, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error analyzing PR: %v\n", err)
			os.Exit(1)
		}
		prStats = stats

		// Optional GitHub API enrichment
		prNumber := flagPRNumber
		if prNumber == 0 {
			prNumber = github.DetectPRNumber()
		}

		if ghClient != nil && repoSlug != "" && prNumber > 0 {
			if !flagQuiet {
				fmt.Printf("🌐 Fetching PR #%d metadata from GitHub API (%s)...\n", prNumber, repoSlug)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			prMeta, err := ghClient.GetPRMetadata(ctx, repoSlug, prNumber)
			cancel()
			if err != nil {
				if !flagQuiet {
					fmt.Printf("⚠️ GitHub API PR enrichment skipped: %v\n", err)
				}
			} else {
				stats.GitHubMeta = prMeta
			}
		}

		stats.PopulateSARIF(builder)
		markdownSummary = reporter.GeneratePRSummary(stats)

		// Optional Sticky Comment on PR
		if flagCommentPR && ghClient != nil && repoSlug != "" && prNumber > 0 {
			if !flagQuiet {
				fmt.Printf("💬 Posting sticky comment to PR #%d...\n", prNumber)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			err := ghClient.PostOrUpdatePRComment(ctx, repoSlug, prNumber, "<!-- gh-stats-sticky-comment -->", markdownSummary)
			cancel()
			if err != nil && !flagQuiet {
				fmt.Printf("⚠️ Failed to post PR sticky comment: %v\n", err)
			}
		}

		if !flagQuiet {
			fmt.Printf("✅ PR Analysis Complete: Risk=%s (%d/100), Files=%d, Additions=+%d, Deletions=-%d\n",
				stats.RiskLevel, stats.RiskScore, stats.FilesChanged, stats.TotalAdditions, stats.TotalDeletions)
		}

	case "repo":
		commitLimit := flagCommitLimit
		if commitLimit <= 0 {
			commitLimit = cfg.Thresholds.CommitLimit
			if commitLimit <= 0 {
				commitLimit = 200
			}
		}

		if !flagQuiet {
			fmt.Printf("🏛️ Analyzing Repository (commit window: %d)...\n", commitLimit)
		}

		stats, err := analyzer.AnalyzeRepo(runner, commitLimit, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error analyzing repository: %v\n", err)
			os.Exit(1)
		}

		// Optional GitHub API enrichment for repo
		if ghClient != nil && repoSlug != "" {
			if !flagQuiet {
				fmt.Printf("🌐 Fetching Repository metadata from GitHub API (%s)...\n", repoSlug)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			repoMeta, err := ghClient.GetRepoMetadata(ctx, repoSlug)
			cancel()
			if err != nil {
				if !flagQuiet {
					fmt.Printf("⚠️ GitHub API repo metadata skipped: %v\n", err)
				}
			} else {
				stats.GitHubMeta = repoMeta
			}
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

	// Quality gate enforcement (fail-on)
	if target == "pr" && prStats != nil && flagFailOn != "" {
		prPriority := riskLevelPriority(prStats.RiskLevel)
		gatePriority := riskLevelPriority(flagFailOn)
		if gatePriority > 0 && prPriority >= gatePriority {
			fmt.Fprintf(os.Stderr, "❌ Quality Gate Failed: PR SRE risk level [%s] meets or exceeds fail-on threshold [%s]\n",
				prStats.RiskLevel, strings.ToUpper(flagFailOn))
			os.Exit(2)
		}
	}
}

func riskLevelPriority(level string) int {
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
