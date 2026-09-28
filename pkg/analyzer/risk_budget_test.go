package analyzer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

func TestCalculateHistoricalCommitRisk(t *testing.T) {
	cfg := config.DefaultConfig()

	// 1. Small commit with tests
	smallEntry := gitutil.MergedCommitHistoryEntry{
		Hash:      "abc1234",
		Timestamp: time.Now(),
		Subject:   "feat: small change",
		DiffStats: []gitutil.FileDiffStat{
			{Path: "pkg/core/logic.go", Additions: 30, Deletions: 5, Status: "M"},
			{Path: "pkg/core/logic_test.go", Additions: 30, Deletions: 0, Status: "M"},
		},
	}
	score, level, sensitive := CalculateHistoricalCommitRisk(smallEntry, cfg)
	if score > 30 || level != "LOW" {
		t.Errorf("expected small commit to have LOW risk, got score=%d, level=%s", score, level)
	}
	if sensitive != 0 {
		t.Errorf("expected 0 sensitive files, got %d", sensitive)
	}

	// 2. High blast radius commit (sensitive CI/CD + DB migration)
	sensitiveEntry := gitutil.MergedCommitHistoryEntry{
		Hash:      "def5678",
		Timestamp: time.Now(),
		Subject:   "feat: sensitive changes",
		DiffStats: []gitutil.FileDiffStat{
			{Path: ".github/workflows/deploy.yml", Additions: 50, Deletions: 10, Status: "M"},
			{Path: "migrations/001_accounts.sql", Additions: 40, Deletions: 0, Status: "A"},
			{Path: "pkg/auth/token.go", Additions: 120, Deletions: 20, Status: "M"},
		},
	}
	score, level, sensitive = CalculateHistoricalCommitRisk(sensitiveEntry, cfg)
	if sensitive < 2 {
		t.Errorf("expected at least 2 sensitive files, got %d", sensitive)
	}
	if score < 60 {
		t.Errorf("expected sensitive commit score >= 60, got %d", score)
	}
	if level != "HIGH" && level != "CRITICAL" {
		t.Errorf("expected HIGH or CRITICAL level, got %s", level)
	}
}

func TestEvaluateRiskBudget_Calculations(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RiskBudget = config.RiskBudgetConfig{
		Enabled:            true,
		MonthlyRiskPoints:  500,
		WindowDays:         30,
		MaxCriticalPRs:     2,
		BurnRateAlertRatio: 1.0,
	}

	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// 1. Healthy state
	t.Run("Healthy", func(t *testing.T) {
		tempDir := t.TempDir()
		initGitRepoWithCommits(t, tempDir, now, []commitSpec{
			{offsetDays: 20, additions: 20, deletions: 5, path: "pkg/api/api.go"},
			{offsetDays: 10, additions: 40, deletions: 10, path: "pkg/api/api.go"},
		})

		runner := gitutil.NewRunner(tempDir)
		stats := EvaluateRiskBudget(runner, "main", 30, "LOW", cfg, now)
		if stats == nil {
			t.Fatal("expected non-nil RiskBudgetStats")
		}

		if stats.Status != "HEALTHY" {
			t.Errorf("expected HEALTHY status, got %s (util=%.1f%%, burn=%.2fx)", stats.Status, stats.UtilizationPercent, stats.BurnRate)
		}
		if stats.UtilizationPercent >= 80.0 {
			t.Errorf("expected utilization < 80%%, got %.1f%%", stats.UtilizationPercent)
		}
	})

	// 2. Elevated burn rate
	t.Run("ElevatedBurnRate", func(t *testing.T) {
		tempDir := t.TempDir()
		// Multiple high-churn commits in the last 2 days
		initGitRepoWithCommits(t, tempDir, now, []commitSpec{
			{offsetDays: 2, additions: 1500, deletions: 200, path: ".github/workflows/ci.yml"},
			{offsetDays: 1, additions: 1200, deletions: 300, path: "migrations/schema.sql"},
		})

		runner := gitutil.NewRunner(tempDir)
		stats := EvaluateRiskBudget(runner, "main", 70, "HIGH", cfg, now)
		if stats == nil {
			t.Fatal("expected non-nil stats")
		}

		if stats.BurnRate < 1.0 {
			t.Errorf("expected BurnRate >= 1.0, got %.2f", stats.BurnRate)
		}
		if stats.Status != "ELEVATED" && stats.Status != "EXCEEDED" {
			t.Errorf("expected ELEVATED or EXCEEDED status, got %s", stats.Status)
		}
	})

	// 3. Exceeded points budget
	t.Run("ExceededPoints", func(t *testing.T) {
		tempDir := t.TempDir()
		var specs []commitSpec
		for i := 1; i <= 8; i++ {
			specs = append(specs, commitSpec{
				offsetDays: i * 3,
				additions:  1000,
				deletions:  100,
				path:       ".github/workflows/deploy.yml",
			})
		}
		initGitRepoWithCommits(t, tempDir, now, specs)

		runner := gitutil.NewRunner(tempDir)
		stats := EvaluateRiskBudget(runner, "main", 85, "CRITICAL", cfg, now)
		if stats == nil {
			t.Fatal("expected non-nil stats")
		}

		if stats.Status != "EXCEEDED" {
			t.Errorf("expected EXCEEDED status, got %s (pts=%d/%d)", stats.Status, stats.TotalProjectedPoints, stats.MonthlyRiskPoints)
		}
		if stats.UtilizationPercent < 100.0 {
			t.Errorf("expected utilization >= 100%%, got %.1f%%", stats.UtilizationPercent)
		}
	})

	// 4. Exceeded critical PR limit
	t.Run("ExceededCriticalPRs", func(t *testing.T) {
		tempDir := t.TempDir()
		initGitRepoWithCommits(t, tempDir, now, []commitSpec{
			{offsetDays: 15, additions: 2500, deletions: 100, path: "migrations/1.sql"},
			{offsetDays: 5, additions: 2500, deletions: 100, path: "migrations/2.sql"},
		})

		runner := gitutil.NewRunner(tempDir)
		// Current PR is also critical
		stats := EvaluateRiskBudget(runner, "main", 90, "CRITICAL", cfg, now)
		if stats == nil {
			t.Fatal("expected non-nil stats")
		}

		if stats.TotalCriticalPRs < 3 {
			t.Errorf("expected at least 3 critical PRs, got %d", stats.TotalCriticalPRs)
		}
		if stats.Status != "EXCEEDED" {
			t.Errorf("expected EXCEEDED status due to max_critical_prs, got %s", stats.Status)
		}
	})
}

func TestSARIF_RulePRRiskBudgetExceeded(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RiskBudget = config.RiskBudgetConfig{
		Enabled:            true,
		MonthlyRiskPoints:  500,
		WindowDays:         30,
		MaxCriticalPRs:     2,
		BurnRateAlertRatio: 1.0,
	}

	builder := sarif.NewBuilder()
	prStats := &PRStats{
		BaseRef:     "main",
		HeadRef:     "HEAD",
		PrimaryFile: "pkg/core/logic.go",
		Config:      cfg,
		RiskScore:   75,
		RiskLevel:   "HIGH",
		RiskBudget: &RiskBudgetStats{
			Enabled:              true,
			MonthlyRiskPoints:    500,
			WindowDays:           30,
			MaxCriticalPRs:       2,
			BurnRateAlertRatio:   1.0,
			HistoricalPoints:     450,
			CurrentPRPoints:      75,
			TotalProjectedPoints: 525,
			TotalCriticalPRs:     3,
			UtilizationPercent:   105.0,
			BurnRate:             1.75,
			Status:               "EXCEEDED",
		},
	}

	prStats.PopulateSARIF(builder)
	report := builder.Build()

	found := false
	for _, res := range report.Runs[0].Results {
		if res.RuleID == RulePRRiskBudgetExceeded.ID {
			found = true
			if res.Level != "error" {
				t.Errorf("expected error level for EXCEEDED status, got %s", res.Level)
			}
			break
		}
	}

	if !found {
		t.Errorf("expected SARIF rule %s to be present in results", RulePRRiskBudgetExceeded.ID)
	}
}

type commitSpec struct {
	offsetDays int
	additions  int
	deletions  int
	path       string
}

func initGitRepoWithCommits(t *testing.T, dir string, now time.Time, specs []commitSpec) {
	t.Helper()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.name", "Risk Budget Tester")
	runGit("config", "user.email", "budget@example.com")
	runGit("config", "commit.gpgsign", "false")

	// Base commit
	readme := filepath.Join(dir, "README.md")
	_ = os.WriteFile(readme, []byte("# Test Repo\n"), 0644)
	runGit("add", "README.md")
	runGit("commit", "-m", "chore: initial commit")

	for idx, spec := range specs {
		targetFile := filepath.Join(dir, spec.path)
		_ = os.MkdirAll(filepath.Dir(targetFile), 0755)

		var b bytes.Buffer
		for i := 0; i < spec.additions; i++ {
			fmt.Fprintf(&b, "commit %d line %d in %s\n", idx, i, spec.path)
		}
		_ = os.WriteFile(targetFile, b.Bytes(), 0644)
		runGit("add", "-A")

		commitTime := now.AddDate(0, 0, -spec.offsetDays).Format(time.RFC3339)
		cmd := exec.Command("git", "commit", "-m", fmt.Sprintf("feat: test commit %d for risk budget", idx))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_DATE="+commitTime,
			"GIT_COMMITTER_DATE="+commitTime,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit failed: %v (output: %s)", err, string(out))
		}
	}
}
