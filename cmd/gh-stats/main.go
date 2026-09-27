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
	"github.com/smford/gh-stats/pkg/exporter"
	"github.com/smford/gh-stats/pkg/github"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/hook"
	"github.com/smford/gh-stats/pkg/reporter"
	"github.com/smford/gh-stats/pkg/sarif"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		handleHookCommand(os.Args[2:])
		return
	}

	var (
		flagTarget        string
		flagBaseRef       string
		flagHeadRef       string
		flagOutput        string
		flagRepoPath      string
		flagCommitLimit   int
		flagConfigPath    string
		flagToken         string
		flagPRNumber      int
		flagRepoSlug      string
		flagFailOn        string
		flagCommentPR     bool
		flagQuiet         bool
		flagVersion       bool
		flagExportJSON    string
		flagExportWebhook string
		flagWebhookSecret string
		flagMaxCILatency  int
		flagFailOnFlaky   bool
		flagCheckRuns     bool
		flagSuggestBump   bool
	)

	flag.StringVar(&flagTarget, "target", getEnvDefault("INPUT_TARGET", getEnvDefault("INPUT_MODE", "auto")), "Target scope: 'pr', 'repo', 'release'/'range', 'drift', or 'auto'")
	flag.StringVar(&flagBaseRef, "base", getEnvDefault("INPUT_BASE_REF", os.Getenv("GITHUB_BASE_REF")), "Base ref for PR or release comparison (e.g. origin/main or v0.3.0)")
	flag.StringVar(&flagHeadRef, "head", getEnvDefault("INPUT_HEAD_REF", "HEAD"), "Head ref for PR or release comparison (default HEAD)")
	flag.StringVar(&flagOutput, "output", getEnvDefault("INPUT_SARIF_OUTPUT", getEnvDefault("INPUT_OUTPUT", "gh-stats.sarif")), "Path to output SARIF file")
	flag.StringVar(&flagRepoPath, "repo-path", getEnvDefault("INPUT_REPO_PATH", "."), "Path to git repository")
	flag.IntVar(&flagCommitLimit, "commit-limit", 0, "Maximum commit history to examine for repo hotspots (default 200)")
	flag.StringVar(&flagConfigPath, "config", getEnvDefault("INPUT_CONFIG_PATH", getEnvDefault("INPUT_CONFIG", "")), "Path to .gh-stats.yml configuration file")
	flag.StringVar(&flagToken, "token", getEnvDefault("INPUT_TOKEN", os.Getenv("GITHUB_TOKEN")), "GitHub API token for metadata enrichment")
	flag.IntVar(&flagPRNumber, "pr-number", 0, "Pull request number (auto-detected if omitted)")
	flag.StringVar(&flagRepoSlug, "repo", getEnvDefault("GITHUB_REPOSITORY", ""), "GitHub repository slug owner/repo (auto-detected if omitted)")
	flag.StringVar(&flagFailOn, "fail-on", getEnvDefault("INPUT_FAIL_ON", ""), "Fail workflow if PR or release risk meets/exceeds threshold (e.g. 'HIGH', 'CRITICAL')")
	flag.BoolVar(&flagCommentPR, "comment-pr", getEnvDefault("INPUT_COMMENT_PR", "false") == "true", "Post or update a sticky summary comment on the PR")
	flag.StringVar(&flagExportJSON, "export-json", getEnvDefault("INPUT_EXPORT_JSON", ""), "Path to export DORA & SRE metrics JSON file")
	flag.StringVar(&flagExportWebhook, "export-webhook", getEnvDefault("INPUT_EXPORT_WEBHOOK", ""), "Webhook URL to export DORA & SRE metrics")
	flag.StringVar(&flagWebhookSecret, "webhook-secret", getEnvDefault("INPUT_WEBHOOK_SECRET", os.Getenv("WEBHOOK_SECRET")), "Secret key or bearer token for webhook export")
	flag.IntVar(&flagMaxCILatency, "max-ci-latency", 0, "Maximum acceptable CI check latency in minutes (default 15)")
	flag.BoolVar(&flagFailOnFlaky, "fail-on-flaky", getEnvDefault("INPUT_FAIL_ON_FLAKY", "false") == "true", "Fail quality gate if flaky CI checks are detected")
	flag.BoolVar(&flagCheckRuns, "check-runs", true, "Fetch CI check runs for latency and flakiness analysis")
	flag.BoolVar(&flagSuggestBump, "suggest-bump", getEnvDefault("INPUT_SUGGEST_BUMP", "false") == "true", "Deterministically recommend next semantic version bump and version string")
	flag.BoolVar(&flagQuiet, "quiet", false, "Suppress stdout output")
	flag.BoolVar(&flagVersion, "version", false, "Print gh-stats version and exit")
	flag.Parse()

	if flagVersion {
		fmt.Printf("gh-stats version %s\n", version)
		os.Exit(0)
	}

	// Resolve auto mode
	target := strings.ToLower(flagTarget)
	if target == "auto" {
		if flagSuggestBump {
			target = "release"
		} else if os.Getenv("GITHUB_EVENT_NAME") == "pull_request" || os.Getenv("GITHUB_BASE_REF") != "" {
			target = "pr"
		} else if os.Getenv("GITHUB_EVENT_NAME") == "release" {
			target = "release"
		} else {
			target = "repo"
		}
	}
	if target == "range" {
		target = "release"
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
	if flagExportJSON == "" && cfg.Export.JSONPath != "" {
		flagExportJSON = cfg.Export.JSONPath
	}
	if flagExportWebhook == "" && cfg.Export.WebhookURL != "" {
		flagExportWebhook = cfg.Export.WebhookURL
	}
	if flagWebhookSecret == "" && cfg.Export.WebhookSecret != "" {
		flagWebhookSecret = cfg.Export.WebhookSecret
	}
	if flagMaxCILatency <= 0 {
		flagMaxCILatency = cfg.Thresholds.MaxCILatencyMinutes
		if flagMaxCILatency <= 0 {
			flagMaxCILatency = 15
		}
	}
	if !flagFailOnFlaky && cfg.Thresholds.FailOnFlakyCI {
		flagFailOnFlaky = true
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
	var repoStats *analyzer.RepoStats
	var releaseStats *analyzer.ReleaseStats
	var driftStats *analyzer.DriftStats

	switch target {
	case "pr":
		baseRef := flagBaseRef
		if baseRef == "" {
			baseRef = runner.GetDefaultBaseRef()
		}
		// In GitHub Actions pull_request, baseRef is often just "main"; prefix origin/ if needed
		if !strings.HasPrefix(baseRef, "origin/") && baseRef != "HEAD~1" && baseRef != "HEAD" && baseRef != "staged" && baseRef != "--cached" {
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

		// Optional CI Pipeline Check Runs analysis
		if flagCheckRuns && ghClient != nil && repoSlug != "" {
			ref := flagHeadRef
			if stats.GitHubMeta != nil && stats.GitHubMeta.Head.SHA != "" {
				ref = stats.GitHubMeta.Head.SHA
			} else {
				if sha, err := runner.Exec("rev-parse", flagHeadRef); err == nil && strings.TrimSpace(sha) != "" {
					ref = strings.TrimSpace(sha)
				}
			}

			if !flagQuiet {
				fmt.Printf("⚡ Fetching CI Check Runs from GitHub API (%s, ref: %s)...\n", repoSlug, ref)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			ciStats, err := ghClient.GetCIPipelineStats(ctx, repoSlug, ref, flagMaxCILatency)
			cancel()
			if err != nil {
				if !flagQuiet {
					fmt.Printf("⚠️ GitHub API Check Runs skipped: %v\n", err)
				}
			} else if ciStats != nil && ciStats.TotalCheckRuns > 0 {
				stats.CIPipelineStats = ciStats
				if !flagQuiet {
					flakyCount := 0
					for _, f := range ciStats.FlakyRuns {
						if f.IsFlaky {
							flakyCount++
						}
					}
					fmt.Printf("⚡ CI Pipeline Stats: %d checks, latency=%s, bottleneck=%s (%s), flaky=%d\n",
						ciStats.TotalCheckRuns, ciStats.TotalDuration.Round(time.Second),
						ciStats.LongestRunName, ciStats.LongestRunDuration.Round(time.Second),
						flakyCount)
				}
			}
		}

		stats.RecalculateRisk()

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
		repoStats = stats

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

	case "release":
		baseRef := flagBaseRef
		headRef := flagHeadRef

		if flagSuggestBump && baseRef == "" && (headRef == "" || headRef == "HEAD") {
			latestTag, err := runner.GetLatestTag()
			if err == nil && latestTag != "" {
				baseRef = latestTag
				headRef = "HEAD"
			}
		}

		// If headRef not provided or is default HEAD, auto-detect from tags
		if headRef == "" || headRef == "HEAD" {
			if baseRef == "" {
				bTag, hTag, err := runner.GetLatestTwoTags()
				if err == nil {
					baseRef = bTag
					headRef = hTag
				} else {
					baseRef = "HEAD~1"
					headRef = "HEAD"
				}
			} else {
				if headRef == "" {
					headRef = "HEAD"
				}
			}
		} else if baseRef == "" {
			prevTag, err := runner.GetPreviousTag(headRef)
			if err == nil && prevTag != "" {
				baseRef = prevTag
			} else {
				baseRef = headRef + "~1"
			}
		}

		if !flagQuiet {
			fmt.Printf("📦 Comparing Release: %s ... %s\n", baseRef, headRef)
		}

		stats, err := analyzer.AnalyzeRelease(runner, baseRef, headRef, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error analyzing release: %v\n", err)
			os.Exit(1)
		}
		releaseStats = stats

		stats.PopulateSARIF(builder)
		markdownSummary = reporter.GenerateReleaseSummary(stats)

		if !flagQuiet {
			fmt.Printf("✅ Release Analysis Complete: Risk=%s (%d/100), Commits=%d, Breaking Changes=%d, Files=%d, Additions=+%d, Deletions=-%d\n",
				stats.RiskLevel, stats.RiskScore, stats.TotalCommits, len(stats.BreakingChanges), stats.FilesChanged, stats.TotalAdditions, stats.TotalDeletions)
		}

	case "drift":
		baseRef := flagBaseRef
		headRef := flagHeadRef

		if baseRef == "" {
			if _, err := runner.Exec("rev-parse", "--verify", "origin/production"); err == nil {
				baseRef = "origin/production"
			} else if _, err := runner.Exec("rev-parse", "--verify", "production"); err == nil {
				baseRef = "production"
			} else {
				baseRef = runner.GetDefaultBaseRef()
			}
		} else {
			if !strings.HasPrefix(baseRef, "origin/") && baseRef != "HEAD" && !strings.HasPrefix(baseRef, "HEAD~") {
				if _, err := runner.Exec("rev-parse", "--verify", "origin/"+baseRef); err == nil {
					baseRef = "origin/" + baseRef
				}
			}
		}

		if headRef == "" || headRef == "HEAD" {
			if _, err := runner.Exec("rev-parse", "--verify", "origin/staging"); err == nil {
				headRef = "origin/staging"
			} else if _, err := runner.Exec("rev-parse", "--verify", "staging"); err == nil {
				headRef = "staging"
			} else {
				headRef = "HEAD"
			}
		} else {
			if !strings.HasPrefix(headRef, "origin/") && headRef != "HEAD" && !strings.HasPrefix(headRef, "HEAD~") {
				if _, err := runner.Exec("rev-parse", "--verify", "origin/"+headRef); err == nil {
					headRef = "origin/" + headRef
				}
			}
		}

		if !flagQuiet {
			fmt.Printf("🌐 Auditing Environment Drift: %s (target) ➔ %s (candidate)\n", baseRef, headRef)
		}

		stats, err := analyzer.AnalyzeDrift(runner, baseRef, headRef, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error auditing environment drift: %v\n", err)
			os.Exit(1)
		}
		driftStats = stats

		stats.PopulateSARIF(builder)
		markdownSummary = reporter.GenerateDriftSummary(stats)

		if !flagQuiet {
			fmt.Printf("✅ Drift Audit Complete: Promotion Risk=%s (%d/100), Commits Ahead=%d, Commits Behind=%d, Files=%d, Additions=+%d, Deletions=-%d\n",
				stats.RiskLevel, stats.RiskScore, stats.CommitsAhead, stats.CommitsBehind, stats.FilesChanged, stats.TotalAdditions, stats.TotalDeletions)
		}

	default:
		fmt.Fprintf(os.Stderr, "❌ Invalid target '%s'. Supported targets are 'pr', 'repo', 'release', 'range', or 'drift'.\n", target)
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
	if prStats != nil && prStats.CIPipelineStats != nil {
		flakyCount := 0
		for _, f := range prStats.CIPipelineStats.FlakyRuns {
			if f.IsFlaky {
				flakyCount++
			}
		}
		setGithubOutput("flaky-checks-count", fmt.Sprintf("%d", flakyCount))
		setGithubOutput("ci-latency-seconds", fmt.Sprintf("%.0f", prStats.CIPipelineStats.TotalDuration.Seconds()))
	}
	if releaseStats != nil {
		setGithubOutput("release-base-ref", releaseStats.BaseRef)
		setGithubOutput("release-head-ref", releaseStats.HeadRef)
		setGithubOutput("release-commits-count", fmt.Sprintf("%d", releaseStats.TotalCommits))
		setGithubOutput("release-breaking-count", fmt.Sprintf("%d", len(releaseStats.BreakingChanges)))
		setGithubOutput("release-risk-level", releaseStats.RiskLevel)
		setGithubOutput("release-risk-score", fmt.Sprintf("%d", releaseStats.RiskScore))
	}
	if driftStats != nil {
		setGithubOutput("drift-base-ref", driftStats.BaseRef)
		setGithubOutput("drift-head-ref", driftStats.HeadRef)
		setGithubOutput("drift-commits-ahead", fmt.Sprintf("%d", driftStats.CommitsAhead))
		setGithubOutput("drift-commits-behind", fmt.Sprintf("%d", driftStats.CommitsBehind))
		setGithubOutput("drift-breaking-count", fmt.Sprintf("%d", len(driftStats.BreakingChanges)))
		setGithubOutput("drift-sensitive-count", fmt.Sprintf("%d", len(driftStats.SensitiveFiles)))
		setGithubOutput("drift-risk-level", driftStats.RiskLevel)
		setGithubOutput("drift-risk-score", fmt.Sprintf("%d", driftStats.RiskScore))
	}

	// Compute and expose recommended Semantic Version bump outputs
	var suggestedBump string
	var suggestedVersion string
	var currentVersionTag string

	if releaseStats != nil {
		suggestedBump = releaseStats.SuggestedBump
		suggestedVersion = releaseStats.SuggestedVersion
		currentVersionTag = releaseStats.BaseRef
		if !gitutil.IsSemverTag(currentVersionTag) {
			if lt, err := runner.GetLatestTag(); err == nil && lt != "" {
				currentVersionTag = lt
			}
		}
	} else if prStats != nil {
		suggestedBump = prStats.SuggestedBump
		suggestedVersion = prStats.SuggestedVersion
		if lt, err := runner.GetLatestTag(); err == nil && lt != "" {
			currentVersionTag = lt
		}
	}

	if (suggestedVersion == "" || suggestedVersion == currentVersionTag) && suggestedBump != "" && suggestedBump != analyzer.BumpNone {
		if nextVer, err := analyzer.CalculateNextVersion(currentVersionTag, suggestedBump); err == nil {
			suggestedVersion = nextVer
		}
	}

	if suggestedBump != "" {
		isNewVersion := "true"
		if suggestedBump == analyzer.BumpNone || (currentVersionTag != "" && suggestedVersion == currentVersionTag) {
			isNewVersion = "false"
		}
		versionNum := strings.TrimPrefix(suggestedVersion, "v")
		majorTag := ""
		if strings.HasPrefix(suggestedVersion, "v") {
			parts := strings.Split(suggestedVersion, ".")
			if len(parts) > 0 {
				majorTag = parts[0]
			}
		}

		setGithubOutput("suggested-bump", suggestedBump)
		setGithubOutput("suggested_bump", suggestedBump)
		setGithubOutput("suggested-version", suggestedVersion)
		setGithubOutput("suggested_version", suggestedVersion)
		setGithubOutput("version", suggestedVersion)
		setGithubOutput("version_number", versionNum)
		setGithubOutput("is_new_version", isNewVersion)
		if majorTag != "" {
			setGithubOutput("major_tag", majorTag)
		}

		if !flagQuiet {
			fmt.Printf("🏷️ Suggested Version Bump: %s (current: %s ➔ next: %s)\n",
				strings.ToUpper(suggestedBump), currentVersionTag, suggestedVersion)
		}
	}

	if !flagQuiet && os.Getenv("GITHUB_ACTIONS") == "" {
		fmt.Println("\n" + markdownSummary)
	}

	// DORA & SRE Observability Export (JSON and/or Webhook)
	var metricsPayload *exporter.DORAMetricsPayload
	if prStats != nil {
		metricsPayload = exporter.NewPRPayload(prStats, repoSlug)
	} else if repoStats != nil {
		metricsPayload = exporter.NewRepoPayload(repoStats, repoSlug)
	} else if releaseStats != nil {
		metricsPayload = exporter.NewReleasePayload(releaseStats, repoSlug)
	} else if driftStats != nil {
		metricsPayload = exporter.NewDriftPayload(driftStats, repoSlug)
	}

	if metricsPayload != nil {
		if flagExportJSON != "" {
			if err := exporter.ExportJSON(metricsPayload, flagExportJSON); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ Failed to export metrics JSON: %v\n", err)
			} else {
				if !flagQuiet {
					fmt.Printf("📊 DORA metrics exported to: %s\n", flagExportJSON)
				}
				setGithubOutput("metrics-json", flagExportJSON)
			}
		}

		if flagExportWebhook != "" {
			if !flagQuiet {
				fmt.Printf("🌐 Dispatching DORA metrics to webhook: %s...\n", flagExportWebhook)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := exporter.ExportWebhook(ctx, metricsPayload, flagExportWebhook, flagWebhookSecret)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ Failed to dispatch metrics webhook: %v\n", err)
			} else if !flagQuiet {
				fmt.Println("✅ Metrics webhook dispatched successfully")
			}
		}
	}

	// Quality gate enforcement (fail-on)
	if target == "pr" && prStats != nil && flagFailOn != "" {
		if analyzer.IsRiskThresholdMet(prStats.RiskLevel, flagFailOn) {
			fmt.Fprintf(os.Stderr, "❌ Quality Gate Failed: PR SRE risk level [%s] meets or exceeds fail-on threshold [%s]\n",
				prStats.RiskLevel, strings.ToUpper(flagFailOn))
			os.Exit(2)
		}
	} else if target == "release" && releaseStats != nil && flagFailOn != "" {
		if analyzer.IsRiskThresholdMet(releaseStats.RiskLevel, flagFailOn) {
			fmt.Fprintf(os.Stderr, "❌ Quality Gate Failed: Release deployment risk level [%s] meets or exceeds fail-on threshold [%s]\n",
				releaseStats.RiskLevel, strings.ToUpper(flagFailOn))
			os.Exit(2)
		}
	} else if target == "drift" && driftStats != nil && flagFailOn != "" {
		if analyzer.IsRiskThresholdMet(driftStats.RiskLevel, flagFailOn) {
			fmt.Fprintf(os.Stderr, "❌ Quality Gate Failed: Environment drift promotion risk level [%s] meets or exceeds fail-on threshold [%s]\n",
				driftStats.RiskLevel, strings.ToUpper(flagFailOn))
			os.Exit(2)
		}
	}

	// Quality gate enforcement for flaky CI checks if configured
	if target == "pr" && prStats != nil && prStats.CIPipelineStats != nil && flagFailOnFlaky {
		flakyCount := 0
		for _, f := range prStats.CIPipelineStats.FlakyRuns {
			if f.IsFlaky {
				flakyCount++
			}
		}
		if flakyCount > 0 {
			fmt.Fprintf(os.Stderr, "❌ Quality Gate Failed: %d flaky CI check run(s) detected\n", flakyCount)
			os.Exit(2)
		}
	}
}

func handleHookCommand(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: gh-stats hook <install|uninstall|run> [options]")
		fmt.Println()
		fmt.Println("Shift-Left Git Hook management and execution.")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  install     Install shift-left git hook into .git/hooks")
		fmt.Println("  uninstall   Remove installed git hook from .git/hooks")
		fmt.Println("  run         Execute risk evaluation for a git hook")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  gh-stats hook install")
		fmt.Println("  gh-stats hook install --fail-on=HIGH")
		fmt.Println("  gh-stats hook install --type=pre-commit --fail-on=CRITICAL")
		fmt.Println("  gh-stats hook uninstall")
		fmt.Println("  gh-stats hook run --type=pre-push")
		return
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "install":
		installCmd := flag.NewFlagSet("install", flag.ExitOnError)
		hookType := installCmd.String("type", hook.HookPrePush, "Hook type to install: 'pre-push' or 'pre-commit'")
		failOn := installCmd.String("fail-on", "", "Risk threshold to fail on (e.g. 'HIGH', 'CRITICAL')")
		repoPath := installCmd.String("repo-path", ".", "Path to git repository")
		force := installCmd.Bool("force", false, "Force overwrite of existing non-gh-stats hook")
		quiet := installCmd.Bool("quiet", false, "Suppress output")
		_ = installCmd.Parse(subargs)

		hookPath, err := hook.Install(hook.Options{
			RepoPath: *repoPath,
			HookType: *hookType,
			FailOn:   *failOn,
			Force:    *force,
			Quiet:    *quiet,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to install %s hook: %v\n", *hookType, err)
			os.Exit(1)
		}

		if !*quiet {
			fmt.Printf("✅ Installed shift-left %s hook at %s\n", *hookType, hookPath)
			if *hookType == hook.HookPrePush {
				fmt.Println("💡 The pre-push hook will evaluate risk before 'git push' to block risky deployments.")
				fmt.Println("   Bypass temporarily if needed using: git push --no-verify")
			} else {
				fmt.Println("💡 The pre-commit hook will evaluate staged risk before 'git commit'.")
				fmt.Println("   Bypass temporarily if needed using: git commit --no-verify")
			}
		}

	case "uninstall":
		uninstallCmd := flag.NewFlagSet("uninstall", flag.ExitOnError)
		hookType := uninstallCmd.String("type", hook.HookPrePush, "Hook type to uninstall: 'pre-push' or 'pre-commit'")
		repoPath := uninstallCmd.String("repo-path", ".", "Path to git repository")
		force := uninstallCmd.Bool("force", false, "Force remove even if hook was modified")
		quiet := uninstallCmd.Bool("quiet", false, "Suppress output")
		_ = uninstallCmd.Parse(subargs)

		err := hook.Uninstall(hook.Options{
			RepoPath: *repoPath,
			HookType: *hookType,
			Force:    *force,
			Quiet:    *quiet,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to uninstall %s hook: %v\n", *hookType, err)
			os.Exit(1)
		}

		if !*quiet {
			fmt.Printf("✅ Uninstalled %s hook from .git/hooks/%s\n", *hookType, *hookType)
		}

	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		hookType := runCmd.String("type", hook.HookPrePush, "Hook type to run: 'pre-push' or 'pre-commit'")
		baseRef := runCmd.String("base", "", "Base reference branch (auto-detected if omitted)")
		failOn := runCmd.String("fail-on", "", "Risk threshold to fail on (e.g. 'HIGH', 'CRITICAL')")
		repoPath := runCmd.String("repo-path", ".", "Path to git repository")
		quiet := runCmd.Bool("quiet", false, "Suppress output")
		_ = runCmd.Parse(subargs)

		err := hook.Run(hook.RunOptions{
			RepoPath: *repoPath,
			HookType: *hookType,
			BaseRef:  *baseRef,
			FailOn:   *failOn,
			Quiet:    *quiet,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "❌ Unknown hook command: %s. Use 'install', 'uninstall', or 'run'.\n", subcmd)
		os.Exit(1)
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
