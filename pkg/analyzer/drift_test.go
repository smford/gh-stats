package analyzer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
	"github.com/smford/gh-stats/pkg/sarif"
)

func TestSyntheticDriftAudit(t *testing.T) {
	tempDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGit("init", "-b", "production")
	runGit("config", "user.name", "SRE Auditor")
	runGit("config", "user.email", "sre@example.com")
	runGit("config", "commit.gpgsign", "false")

	// Base commit on production
	_ = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Production Service\n"), 0644)
	runGit("add", "README.md")
	runGit("commit", "-m", "chore: initialize production service")

	// Branch out to staging
	runGit("checkout", "-b", "staging")

	// In-sync check
	runner := gitutil.NewRunner(tempDir)
	cfg := config.DefaultConfig()

	statsSync, err := AnalyzeDrift(runner, "production", "staging", cfg)
	if err != nil {
		t.Fatalf("AnalyzeDrift failed on in-sync branches: %v", err)
	}
	if statsSync.CommitsAhead != 0 || statsSync.CommitsBehind != 0 {
		t.Errorf("expected 0 ahead and 0 behind for synced branches, got ahead=%d, behind=%d", statsSync.CommitsAhead, statsSync.CommitsBehind)
	}
	if statsSync.RiskScore != 0 || statsSync.RiskLevel != "LOW" {
		t.Errorf("expected 0 risk score and LOW risk for in-sync branches, got score=%d, level=%s", statsSync.RiskScore, statsSync.RiskLevel)
	}

	// Make unpromoted commits on staging:
	// 1. Regular feature
	_ = os.WriteFile(filepath.Join(tempDir, "service.go"), []byte("package main\nfunc Run() {}\n"), 0644)
	runGit("add", "service.go")
	runGit("commit", "-m", "feat: add core service runner")

	// 2. Breaking change commit
	_ = os.WriteFile(filepath.Join(tempDir, "api.go"), []byte("package main\n// New v2 API\n"), 0644)
	runGit("add", "api.go")
	runGit("commit", "-m", "feat!: breaking api contract overhaul")

	// 3. Database schema migration
	_ = os.MkdirAll(filepath.Join(tempDir, "migrations"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "migrations", "001_users.sql"), []byte("CREATE TABLE users (id SERIAL PRIMARY KEY);\n"), 0644)
	runGit("add", "migrations/001_users.sql")
	runGit("commit", "-m", "feat(db): add user table migration")

	// 4. Sensitive infrastructure file
	_ = os.WriteFile(filepath.Join(tempDir, "Dockerfile"), []byte("FROM alpine:latest\n"), 0644)
	runGit("add", "Dockerfile")
	runGit("commit", "-m", "ci: update Dockerfile alpine base")

	// Now switch back to production and add 1 hotfix (making staging behind production by 1 commit)
	runGit("checkout", "production")
	_ = os.WriteFile(filepath.Join(tempDir, "hotfix.txt"), []byte("critical hotfix\n"), 0644)
	runGit("add", "hotfix.txt")
	runGit("commit", "-m", "fix: urgent production hotfix")

	// Run AnalyzeDrift comparing production (base) and staging (head)
	statsDrift, err := AnalyzeDrift(runner, "production", "staging", cfg)
	if err != nil {
		t.Fatalf("AnalyzeDrift failed on diverged branches: %v", err)
	}

	if statsDrift.CommitsAhead != 4 {
		t.Errorf("expected 4 commits ahead on staging, got %d", statsDrift.CommitsAhead)
	}
	if statsDrift.CommitsBehind != 1 {
		t.Errorf("expected 1 commit behind on staging, got %d", statsDrift.CommitsBehind)
	}
	if len(statsDrift.BreakingChanges) < 2 {
		t.Errorf("expected at least 2 breaking changes (1 conventional commit + 1 migration), got %d", len(statsDrift.BreakingChanges))
	}
	if len(statsDrift.SensitiveFiles) < 2 {
		t.Errorf("expected at least 2 sensitive files (migration + Dockerfile), got %d", len(statsDrift.SensitiveFiles))
	}

	if statsDrift.RiskScore < 50 {
		t.Errorf("expected elevated risk score due to breaking changes & migrations, got %d", statsDrift.RiskScore)
	}

	// Test SARIF rule population
	b := sarif.NewBuilder()
	statsDrift.PopulateSARIF(b)
	doc := b.Build()

	if len(doc.Runs) == 0 {
		t.Fatalf("expected at least 1 SARIF run")
	}

	ruleMap := make(map[string]bool)
	for _, r := range doc.Runs[0].Tool.Driver.Rules {
		ruleMap[r.ID] = true
	}

	if !ruleMap[RuleDriftSummary.ID] {
		t.Errorf("expected %s in SARIF rules", RuleDriftSummary.ID)
	}
	if !ruleMap[RuleDriftUnpromotedBreaking.ID] {
		t.Errorf("expected %s in SARIF rules", RuleDriftUnpromotedBreaking.ID)
	}
	if !ruleMap[RuleDriftSensitive.ID] {
		t.Errorf("expected %s in SARIF rules", RuleDriftSensitive.ID)
	}
}

func TestDriftExcessiveWarning(t *testing.T) {
	stats := &DriftStats{
		BaseRef:       "origin/production",
		HeadRef:       "origin/staging",
		CommitsAhead:  25,
		CommitsBehind: 6,
		FilesChanged:  10,
	}
	stats.CalculateRisk()

	if stats.RiskScore < 35 {
		t.Errorf("expected elevated risk for 25 commits ahead and 6 behind, got %d", stats.RiskScore)
	}

	b := sarif.NewBuilder()
	stats.PopulateSARIF(b)
	doc := b.Build()

	ruleMap := make(map[string]bool)
	for _, r := range doc.Runs[0].Tool.Driver.Rules {
		ruleMap[r.ID] = true
	}

	if !ruleMap[RuleDriftExcessive.ID] {
		t.Errorf("expected %s in SARIF rules when commits ahead >= 15 or behind >= 5", RuleDriftExcessive.ID)
	}
}
