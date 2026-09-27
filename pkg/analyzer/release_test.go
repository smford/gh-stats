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

func TestAnalyzeRelease(t *testing.T) {
	runner := gitutil.NewRunner(".")
	baseTag, headTag, err := runner.GetLatestTwoTags()
	if err != nil {
		t.Fatalf("failed to get latest two tags: %v", err)
	}

	cfg := config.DefaultConfig()
	stats, err := AnalyzeRelease(runner, baseTag, headTag, cfg)
	if err != nil {
		t.Fatalf("AnalyzeRelease failed: %v", err)
	}

	if stats.BaseRef != baseTag || stats.HeadRef != headTag {
		t.Errorf("expected base %s and head %s, got %s and %s", baseTag, headTag, stats.BaseRef, stats.HeadRef)
	}

	if stats.TotalCommits == 0 {
		t.Logf("no commits between %s and %s", baseTag, headTag)
	}

	if stats.RiskLevel == "" {
		t.Errorf("expected non-empty risk level")
	}

	builder := sarif.NewBuilder()
	stats.PopulateSARIF(builder)
	report := builder.Build()
	if len(report.Runs) == 0 {
		t.Fatalf("expected at least one SARIF run")
	}
	if len(report.Runs[0].Results) == 0 {
		t.Errorf("expected at least one SARIF result for release summary")
	}
}

func TestConventionalMatch(t *testing.T) {
	tests := []struct {
		subject string
		prefix  string
		want    bool
	}{
		{"feat: add new feature", "feat", true},
		{"feat(cli): add release flag", "feat", true},
		{"feat!: breaking change", "feat", true},
		{"feat(api)!: breaking change", "feat", true},
		{"fix: fix bug", "fix", true},
		{"fix(ci): fix workflow", "fix", true},
		{"fix!: breaking bug fix", "fix", true},
		{"chore: update deps", "chore", true},
		{"docs: update readme", "docs", true},
		{"refactor: clean up code", "refactor", true},
		{"random commit message", "feat", false},
	}

	for _, tt := range tests {
		got := isConventionalMatch(tt.subject, tt.prefix)
		if got != tt.want {
			t.Errorf("isConventionalMatch(%q, %q) = %v; want %v", tt.subject, tt.prefix, got, tt.want)
		}
	}
}

func TestReleaseStatsRiskCalculation(t *testing.T) {
	stats := &ReleaseStats{
		BaseRef:        "v0.1.0",
		HeadRef:        "v0.2.0",
		TotalAdditions: 4000,
		TotalDeletions: 100,
		CodeLinesAdded: 500,
		TestLinesAdded: 10,
		TestRatio:      0.02,
		BreakingChanges: []BreakingChange{
			{Subject: "feat!: breaking api change", Reason: "conventional_commit"},
		},
		SensitiveFiles: []SensitiveMatch{
			{Path: "db/migrations/001_create.sql", Category: "Database Migrations"},
			{Path: ".github/workflows/ci.yml", Category: "CI/CD Pipelines"},
		},
	}

	stats.CalculateRisk()
	if stats.RiskScore < 80 {
		t.Errorf("expected high risk score for breaking change + migrations + large diff, got %d", stats.RiskScore)
	}
	if stats.RiskLevel != "CRITICAL" {
		t.Errorf("expected CRITICAL risk level, got %s", stats.RiskLevel)
	}
}

func TestSyntheticReleaseComparison(t *testing.T) {
	tempDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.name", "Synthetic Bot")
	runGit("config", "user.email", "bot@example.com")
	runGit("config", "commit.gpgsign", "false")

	// Base release v1.0.0
	_ = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Synthetic Project\n"), 0644)
	_ = os.MkdirAll(filepath.Join(tempDir, "pkg", "api"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "pkg", "api", "api.go"), []byte("package api\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "chore: initial v1.0.0 baseline")
	runGit("tag", "v1.0.0")

	// Delta for v1.1.0
	// 1. Feature
	_ = os.WriteFile(filepath.Join(tempDir, "pkg", "api", "api.go"), []byte("package api\nfunc Login() {}\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "feat(api): add login endpoint")

	// 2. Breaking change
	_ = os.WriteFile(filepath.Join(tempDir, "pkg", "api", "api.go"), []byte("package api\nfunc LoginV2() {}\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "feat(api)!: break legacy endpoint format")

	// 3. Database migration (Sensitive & Breaking)
	_ = os.MkdirAll(filepath.Join(tempDir, "migrations"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "migrations", "001_users.sql"), []byte("CREATE TABLE users (id INT);\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "feat(db): add user table migration")

	// 4. CI/CD workflow (Blast radius)
	_ = os.MkdirAll(filepath.Join(tempDir, ".github", "workflows"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, ".github", "workflows", "ci.yml"), []byte("name: CI\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "ci: add continuous integration workflow")

	// 5. Unit test
	_ = os.WriteFile(filepath.Join(tempDir, "pkg", "api", "api_test.go"), []byte("package api\nfunc TestLogin() {}\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "test(api): add tests for login")

	// Tag v1.1.0
	runGit("tag", "v1.1.0")

	runner := gitutil.NewRunner(tempDir)
	cfg := config.DefaultConfig()

	stats, err := AnalyzeRelease(runner, "v1.0.0", "v1.1.0", cfg)
	if err != nil {
		t.Fatalf("AnalyzeRelease failed: %v", err)
	}

	if stats.TotalCommits != 5 {
		t.Errorf("expected 5 commits, got %d", stats.TotalCommits)
	}
	if len(stats.CategorizedCommits.Features) != 3 {
		t.Errorf("expected 3 feature commits, got %d", len(stats.CategorizedCommits.Features))
	}
	if len(stats.BreakingChanges) < 2 {
		t.Errorf("expected at least 2 breaking changes (conventional + migration), got %d", len(stats.BreakingChanges))
	}
	if len(stats.SensitiveFiles) < 2 {
		t.Errorf("expected at least 2 sensitive files, got %d", len(stats.SensitiveFiles))
	}
	if stats.RiskLevel != "CRITICAL" && stats.RiskLevel != "HIGH" {
		t.Errorf("expected HIGH or CRITICAL risk, got %s", stats.RiskLevel)
	}

	builder := sarif.NewBuilder()
	stats.PopulateSARIF(builder)
	report := builder.Build()

	ruleIDs := make(map[string]bool)
	for _, rule := range report.Runs[0].Tool.Driver.Rules {
		ruleIDs[rule.ID] = true
	}
	if !ruleIDs[RuleReleaseSummary.ID] {
		t.Errorf("expected RuleReleaseSummary in SARIF rules")
	}
	if !ruleIDs[RuleReleaseBreaking.ID] {
		t.Errorf("expected RuleReleaseBreaking in SARIF rules")
	}
	if !ruleIDs[RuleReleaseBlastRadius.ID] {
		t.Errorf("expected RuleReleaseBlastRadius in SARIF rules")
	}
}

