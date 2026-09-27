package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
)

// HookMarker identifies files managed by gh-stats.
const (
	HookMarker    = "# gh-stats-git-hook"
	HookPrePush   = "pre-push"
	HookPreCommit = "pre-commit"
)

// Options holds configuration for hook installation and uninstallation.
type Options struct {
	RepoPath string
	HookType string
	FailOn   string
	Force    bool
	Quiet    bool
}

// RunOptions holds parameters for running an in-hook evaluation.
type RunOptions struct {
	RepoPath string
	HookType string
	BaseRef  string
	FailOn   string
	Quiet    bool
}

// GenerateHookScript returns the shell script content for the requested hook type.
func GenerateHookScript(hookType, failOn string) (string, error) {
	if hookType != HookPrePush && hookType != HookPreCommit {
		return "", fmt.Errorf("unsupported hook type: %q (supported: pre-push, pre-commit)", hookType)
	}

	var failOnArg string
	if failOn != "" {
		failOnArg = fmt.Sprintf(" --fail-on=%q", failOn)
	}

	tmpl := `#!/usr/bin/env bash
# gh-stats-git-hook: {{HOOK_TYPE}}
# Installed by gh-stats (https://github.com/smford/gh-stats)
# Shift-left local risk evaluation before git actions.

set -euo pipefail

# Allow bypassing via SKIP_GH_STATS=1 or standard git flags (--no-verify)
if [ "${SKIP_GH_STATS:-0}" = "1" ]; then
    echo "⚡ gh-stats {{HOOK_TYPE}} hook skipped (SKIP_GH_STATS=1)"
    exit 0
fi

# Locate gh-stats executable
if command -v gh-stats >/dev/null 2>&1; then
    CMD=(gh-stats)
elif command -v go >/dev/null 2>&1 && [ -f "cmd/gh-stats/main.go" ]; then
    CMD=(go run ./cmd/gh-stats)
else
    echo "⚠️  gh-stats not found in PATH. Skipping {{HOOK_TYPE}} risk evaluation."
    echo "   To install: brew install smford/tap/gh-stats OR go install github.com/smford/gh-stats/cmd/gh-stats@latest"
    exit 0
fi

exec "${CMD[@]}" hook run --type={{HOOK_TYPE}}{{FAIL_ON_ARG}}
`

	script := strings.ReplaceAll(tmpl, "{{HOOK_TYPE}}", hookType)
	script = strings.ReplaceAll(script, "{{FAIL_ON_ARG}}", failOnArg)
	return script, nil
}

// Install installs the shift-left git hook into the target repository's .git/hooks directory.
func Install(opts Options) (string, error) {
	hookType := opts.HookType
	if hookType == "" {
		hookType = HookPrePush
	}
	if hookType != HookPrePush && hookType != HookPreCommit {
		return "", fmt.Errorf("unsupported hook type: %q (supported: pre-push, pre-commit)", hookType)
	}

	repoPath := opts.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	runner := gitutil.NewRunner(repoPath)
	hooksDir, err := runner.GetHooksDir()
	if err != nil {
		return "", fmt.Errorf("failed to locate git hooks directory: %w", err)
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create hooks directory %s: %w", hooksDir, err)
	}

	hookFile := filepath.Join(hooksDir, hookType)
	if existing, err := os.ReadFile(hookFile); err == nil {
		// File already exists
		if !opts.Force && !strings.Contains(string(existing), HookMarker) {
			return "", fmt.Errorf("existing %s hook at %s was not created by gh-stats; use --force to overwrite", hookType, hookFile)
		}
	}

	scriptContent, err := GenerateHookScript(hookType, opts.FailOn)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(hookFile, []byte(scriptContent), 0755); err != nil {
		return "", fmt.Errorf("failed to write hook file %s: %w", hookFile, err)
	}

	_ = os.Chmod(hookFile, 0755)
	return hookFile, nil
}

// Uninstall removes the shift-left git hook from the target repository's .git/hooks directory.
func Uninstall(opts Options) error {
	hookType := opts.HookType
	if hookType == "" {
		hookType = HookPrePush
	}
	if hookType != HookPrePush && hookType != HookPreCommit {
		return fmt.Errorf("unsupported hook type: %q (supported: pre-push, pre-commit)", hookType)
	}

	repoPath := opts.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	runner := gitutil.NewRunner(repoPath)
	hooksDir, err := runner.GetHooksDir()
	if err != nil {
		return fmt.Errorf("failed to locate git hooks directory: %w", err)
	}

	hookFile := filepath.Join(hooksDir, hookType)
	existing, err := os.ReadFile(hookFile)
	if os.IsNotExist(err) {
		return fmt.Errorf("no %s hook found at %s", hookType, hookFile)
	} else if err != nil {
		return fmt.Errorf("failed to read hook at %s: %w", hookFile, err)
	}

	if !opts.Force && !strings.Contains(string(existing), HookMarker) {
		return fmt.Errorf("refusing to remove %s at %s: hook was not created by gh-stats (use --force to override)", hookType, hookFile)
	}

	if err := os.Remove(hookFile); err != nil {
		return fmt.Errorf("failed to remove %s: %w", hookFile, err)
	}

	return nil
}

// Run executes shift-left risk evaluation for the specified hook type.
func Run(opts RunOptions) error {
	hookType := opts.HookType
	if hookType == "" {
		hookType = HookPrePush
	}

	repoPath := opts.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	runner := gitutil.NewRunner(repoPath)
	cfg, err := config.LoadConfig("", repoPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	failOn := opts.FailOn
	if failOn == "" {
		failOn = cfg.FailOn
	}

	switch hookType {
	case HookPrePush:
		if failOn == "" {
			failOn = "HIGH"
		}

		baseRef := opts.BaseRef
		if baseRef == "" {
			baseRef = runner.GetDefaultBaseRef()
		}
		if !strings.HasPrefix(baseRef, "origin/") && baseRef != "HEAD~1" && baseRef != "HEAD" {
			if _, err := runner.Exec("rev-parse", "--verify", "origin/"+baseRef); err == nil {
				baseRef = "origin/" + baseRef
			}
		}

		headRef := "HEAD"

		// If baseRef is identical to HEAD, allow push
		baseCommit, _ := runner.Exec("rev-parse", baseRef)
		headCommit, _ := runner.Exec("rev-parse", headRef)
		if strings.TrimSpace(baseCommit) != "" && strings.TrimSpace(baseCommit) == strings.TrimSpace(headCommit) {
			if !opts.Quiet {
				fmt.Printf("✅ gh-stats pre-push: HEAD is in sync with %s. Push allowed.\n", baseRef)
			}
			return nil
		}

		if !opts.Quiet {
			fmt.Printf("🔍 gh-stats: Running shift-left pre-push risk evaluation against [%s]...\n", baseRef)
		}

		stats, err := analyzer.AnalyzePR(runner, baseRef, headRef, cfg)
		if err != nil {
			return fmt.Errorf("pre-push analysis failed: %w", err)
		}

		if stats.FilesChanged == 0 {
			if !opts.Quiet {
				fmt.Printf("✅ gh-stats pre-push: No files changed relative to %s. Push allowed.\n", baseRef)
			}
			return nil
		}

		if !opts.Quiet {
			fmt.Printf("📊 Risk Rating: %s (Score: %d/100) | Files: %d | +%d / -%d\n",
				stats.RiskLevel, stats.RiskScore, stats.FilesChanged, stats.TotalAdditions, stats.TotalDeletions)
			if len(stats.SensitiveFiles) > 0 {
				fmt.Printf("🚨 Sensitive Files (%d):\n", len(stats.SensitiveFiles))
				for _, sf := range stats.SensitiveFiles {
					fmt.Printf("   • %s (%s)\n", sf.Path, sf.Category)
				}
			}
		}

		if analyzer.IsRiskThresholdMet(stats.RiskLevel, failOn) {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("\n❌ PUSH BLOCKED: Risk level [%s] meets or exceeds threshold [%s] (Score: %d/100)\n",
				stats.RiskLevel, strings.ToUpper(failOn), stats.RiskScore))
			sb.WriteString("💡 SRE Shift-Left Guidance:\n")
			if stats.CodeLinesAdded > 50 && stats.TestLinesAdded == 0 {
				sb.WriteString(fmt.Sprintf("   • Add automated tests for the +%d lines of newly added code.\n", stats.CodeLinesAdded))
			}
			if stats.TotalAdditions+stats.TotalDeletions > 500 {
				sb.WriteString("   • Consider decomposing large PRs to reduce blast radius and rollback complexity.\n")
			}
			if len(stats.SensitiveFiles) > 0 {
				sb.WriteString("   • Sensitive infrastructure, security, or database migration files were modified.\n")
			}
			sb.WriteString("💡 To bypass this check:\n")
			sb.WriteString("   git push --no-verify\n")
			sb.WriteString("   OR: SKIP_GH_STATS=1 git push\n")
			return fmt.Errorf("%s", sb.String())
		}

		if !opts.Quiet {
			fmt.Printf("✅ gh-stats pre-push: Risk check passed [%s (%d/100) < %s]. Proceeding with push.\n",
				stats.RiskLevel, stats.RiskScore, strings.ToUpper(failOn))
		}
		return nil

	case HookPreCommit:
		if failOn == "" {
			failOn = "CRITICAL"
		}

		diffStats, err := runner.GetDiffStats("staged", "")
		if err != nil {
			return fmt.Errorf("pre-commit staged diff check failed: %w", err)
		}
		if len(diffStats) == 0 {
			return nil
		}

		if !opts.Quiet {
			fmt.Println("🔍 gh-stats: Running shift-left pre-commit risk evaluation on staged changes...")
		}

		stats, err := analyzer.AnalyzePR(runner, "staged", "", cfg)
		if err != nil {
			return fmt.Errorf("pre-commit analysis failed: %w", err)
		}

		if !opts.Quiet {
			fmt.Printf("📊 Staged Risk Rating: %s (Score: %d/100) | Files: %d | +%d / -%d\n",
				stats.RiskLevel, stats.RiskScore, stats.FilesChanged, stats.TotalAdditions, stats.TotalDeletions)
		}

		if analyzer.IsRiskThresholdMet(stats.RiskLevel, failOn) {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("\n❌ COMMIT BLOCKED: Staged risk [%s] meets or exceeds threshold [%s] (Score: %d/100)\n",
				stats.RiskLevel, strings.ToUpper(failOn), stats.RiskScore))
			sb.WriteString("💡 To bypass this check:\n")
			sb.WriteString("   git commit --no-verify\n")
			sb.WriteString("   OR: SKIP_GH_STATS=1 git commit\n")
			return fmt.Errorf("%s", sb.String())
		}

		if !opts.Quiet {
			fmt.Printf("✅ gh-stats pre-commit: Risk check passed [%s (%d/100) < %s]. Proceeding with commit.\n",
				stats.RiskLevel, stats.RiskScore, strings.ToUpper(failOn))
		}
		return nil

	default:
		return fmt.Errorf("unsupported hook type: %q (supported: pre-push, pre-commit)", hookType)
	}
}
