package analyzer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
)

func TestCalculateNextVersion(t *testing.T) {
	tests := []struct {
		name       string
		currentTag string
		bump       string
		expected   string
		expectErr  bool
	}{
		{
			name:       "major bump with v prefix",
			currentTag: "v0.4.0",
			bump:       "major",
			expected:   "v1.0.0",
		},
		{
			name:       "minor bump with v prefix",
			currentTag: "v0.4.0",
			bump:       "minor",
			expected:   "v0.5.0",
		},
		{
			name:       "patch bump with v prefix",
			currentTag: "v0.4.0",
			bump:       "patch",
			expected:   "v0.4.1",
		},
		{
			name:       "minor bump without v prefix",
			currentTag: "0.4.0",
			bump:       "minor",
			expected:   "0.5.0",
		},
		{
			name:       "major bump past v1",
			currentTag: "v1.9.9",
			bump:       "major",
			expected:   "v2.0.0",
		},
		{
			name:       "minor bump past 9",
			currentTag: "v1.9.9",
			bump:       "minor",
			expected:   "v1.10.0",
		},
		{
			name:       "patch bump past 9",
			currentTag: "v1.9.9",
			bump:       "patch",
			expected:   "v1.9.10",
		},
		{
			name:       "none bump returns current tag",
			currentTag: "v0.4.0",
			bump:       "none",
			expected:   "v0.4.0",
		},
		{
			name:       "empty tag with major bump",
			currentTag: "",
			bump:       "major",
			expected:   "v1.0.0",
		},
		{
			name:       "empty tag with minor bump",
			currentTag: "",
			bump:       "minor",
			expected:   "v0.1.0",
		},
		{
			name:       "empty tag with patch bump",
			currentTag: "",
			bump:       "patch",
			expected:   "v0.0.1",
		},
		{
			name:       "case-insensitive bump parameter",
			currentTag: "v0.4.0",
			bump:       "MINOR",
			expected:   "v0.5.0",
		},
		{
			name:       "invalid tag format",
			currentTag: "invalid-tag",
			bump:       "minor",
			expectErr:  true,
		},
		{
			name:       "invalid bump type",
			currentTag: "v0.4.0",
			bump:       "super-major",
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateNextVersion(tt.currentTag, tt.bump)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("CalculateNextVersion(%q, %q) = %q, expected %q", tt.currentTag, tt.bump, got, tt.expected)
			}
		})
	}
}

func TestDetermineBumpRelease(t *testing.T) {
	t.Run("breaking change commit forces major", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 2,
			Commits: []gitutil.CommitInfo{
				{Subject: "feat!: drop legacy authentication API"},
				{Subject: "fix: fix edge case"},
			},
			BreakingChanges: []BreakingChange{
				{Subject: "feat!: drop legacy authentication API", Reason: "conventional_commit"},
			},
		}
		if got := stats.DetermineBump(); got != BumpMajor {
			t.Errorf("expected %q, got %q", BumpMajor, got)
		}
	})

	t.Run("breaking change in commit body forces major", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 1,
			Commits: []gitutil.CommitInfo{
				{Subject: "refactor: restructure models", Body: "BREAKING CHANGE: table names have changed"},
			},
		}
		if got := stats.DetermineBump(); got != BumpMajor {
			t.Errorf("expected %q, got %q", BumpMajor, got)
		}
	})

	t.Run("database schema migration forces major", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 1,
			Commits: []gitutil.CommitInfo{
				{Subject: "feat(db): add user profiles table"},
			},
			TopChangedFiles: []gitutil.FileDiffStat{
				{Path: "migrations/003_profiles.sql", Additions: 40, Deletions: 0},
			},
		}
		if got := stats.DetermineBump(); got != BumpMajor {
			t.Errorf("expected %q, got %q", BumpMajor, got)
		}
	})

	t.Run("root sql file forces major", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 1,
			Commits: []gitutil.CommitInfo{
				{Subject: "chore: update schema"},
			},
			SensitiveFiles: []SensitiveMatch{
				{Path: "schema.sql", Category: "Database Migrations"},
			},
		}
		if got := stats.DetermineBump(); got != BumpMajor {
			t.Errorf("expected %q, got %q", BumpMajor, got)
		}
	})

	t.Run("feature commits without breaking changes force minor", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 2,
			Commits: []gitutil.CommitInfo{
				{Subject: "feat: add webhook dispatch support"},
				{Subject: "fix: prevent panic on empty token"},
			},
			CategorizedCommits: CommitCategories{
				Features: []gitutil.CommitInfo{{Subject: "feat: add webhook dispatch support"}},
				Fixes:    []gitutil.CommitInfo{{Subject: "fix: prevent panic on empty token"}},
			},
		}
		if got := stats.DetermineBump(); got != BumpMinor {
			t.Errorf("expected %q, got %q", BumpMinor, got)
		}
	})

	t.Run("scoped feature commit forces minor", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 1,
			Commits: []gitutil.CommitInfo{
				{Subject: "feat(observability): export open telemetry spans"},
			},
		}
		if got := stats.DetermineBump(); got != BumpMinor {
			t.Errorf("expected %q, got %q", BumpMinor, got)
		}
	})

	t.Run("fix, chore, docs, and perf commits force patch", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 3,
			FilesChanged: 2,
			Commits: []gitutil.CommitInfo{
				{Subject: "fix: handle nil runner pointer"},
				{Subject: "perf: cache git log queries"},
				{Subject: "docs: update installation instructions"},
			},
			CategorizedCommits: CommitCategories{
				Fixes:       []gitutil.CommitInfo{{Subject: "fix: handle nil runner pointer"}},
				Performance: []gitutil.CommitInfo{{Subject: "perf: cache git log queries"}},
				Docs:        []gitutil.CommitInfo{{Subject: "docs: update installation instructions"}},
			},
		}
		if got := stats.DetermineBump(); got != BumpPatch {
			t.Errorf("expected %q, got %q", BumpPatch, got)
		}
	})

	t.Run("empty release delta returns none", func(t *testing.T) {
		stats := &ReleaseStats{
			TotalCommits: 0,
			FilesChanged: 0,
		}
		if got := stats.DetermineBump(); got != BumpNone {
			t.Errorf("expected %q, got %q", BumpNone, got)
		}
	})
}

func TestDetermineBumpPR(t *testing.T) {
	t.Run("PR with migration returns major", func(t *testing.T) {
		pr := &PRStats{
			TopChangedFiles: []gitutil.FileDiffStat{
				{Path: "migrations/20260927_init.sql"},
			},
		}
		if got := pr.DetermineBump(); got != BumpMajor {
			t.Errorf("expected major, got %s", got)
		}
	})

	t.Run("PR with breaking commit returns major", func(t *testing.T) {
		pr := &PRStats{
			Commits: []gitutil.CommitInfo{
				{Subject: "refactor!: rewrite public api"},
			},
		}
		if got := pr.DetermineBump(); got != BumpMajor {
			t.Errorf("expected major, got %s", got)
		}
	})

	t.Run("PR with feat returns minor", func(t *testing.T) {
		pr := &PRStats{
			Commits: []gitutil.CommitInfo{
				{Subject: "feat: add dark mode theme"},
			},
		}
		if got := pr.DetermineBump(); got != BumpMinor {
			t.Errorf("expected minor, got %s", got)
		}
	})

	t.Run("PR with fix returns patch", func(t *testing.T) {
		pr := &PRStats{
			Commits: []gitutil.CommitInfo{
				{Subject: "fix: correct css padding"},
			},
		}
		if got := pr.DetermineBump(); got != BumpPatch {
			t.Errorf("expected patch, got %s", got)
		}
	})
}

func TestSyntheticSemverBump(t *testing.T) {
	tempDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.name", "SemVer Tester")
	runGit("config", "user.email", "semver@example.com")
	runGit("config", "commit.gpgsign", "false")

	// Baseline tag v0.4.0
	_ = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# App\n"), 0644)
	runGit("add", "README.md")
	runGit("commit", "-m", "chore: baseline commit")
	runGit("tag", "v0.4.0")

	runner := gitutil.NewRunner(tempDir)
	cfg := config.DefaultConfig()

	// 1. Add Feature -> Expect MINOR bump (v0.4.0 -> v0.5.0)
	_ = os.WriteFile(filepath.Join(tempDir, "feature.txt"), []byte("new feature\n"), 0644)
	runGit("add", "feature.txt")
	runGit("commit", "-m", "feat: add automated bump calculator")

	stats, err := AnalyzeRelease(runner, "v0.4.0", "HEAD", cfg)
	if err != nil {
		t.Fatalf("AnalyzeRelease failed: %v", err)
	}

	if stats.SuggestedBump != BumpMinor {
		t.Errorf("expected suggested bump %q, got %q", BumpMinor, stats.SuggestedBump)
	}
	if stats.SuggestedVersion != "v0.5.0" {
		t.Errorf("expected suggested version %q, got %q", "v0.5.0", stats.SuggestedVersion)
	}

	// 2. Add Database Migration -> Expect MAJOR bump (v0.4.0 -> v1.0.0)
	_ = os.MkdirAll(filepath.Join(tempDir, "migrations"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "migrations", "001_accounts.sql"), []byte("CREATE TABLE accounts (id INT);\n"), 0644)
	runGit("add", "migrations/001_accounts.sql")
	runGit("commit", "-m", "chore(db): add accounts migration")

	statsWithMigration, err := AnalyzeRelease(runner, "v0.4.0", "HEAD", cfg)
	if err != nil {
		t.Fatalf("AnalyzeRelease with migration failed: %v", err)
	}

	if statsWithMigration.SuggestedBump != BumpMajor {
		t.Errorf("expected suggested bump %q, got %q", BumpMajor, statsWithMigration.SuggestedBump)
	}
	if statsWithMigration.SuggestedVersion != "v1.0.0" {
		t.Errorf("expected suggested version %q, got %q", "v1.0.0", statsWithMigration.SuggestedVersion)
	}
}
