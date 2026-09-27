package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitRunner(t *testing.T) {
	runner := NewRunner(".")

	// Test listing tracked files (at least README.md is committed)
	files, err := runner.ListTrackedFiles()
	if err != nil {
		t.Fatalf("failed to list tracked files: %v", err)
	}

	foundReadme := false
	for _, f := range files {
		if f == "README.md" {
			foundReadme = true
			break
		}
	}
	if !foundReadme {
		t.Errorf("expected README.md in tracked files, got: %v", files)
	}

	// Test shallow check does not error
	_ = runner.IsShallow()

	// Test author stats
	authorStats, err := runner.GetAuthorStats(10)
	if err != nil {
		t.Fatalf("failed to get author stats: %v", err)
	}
	if len(authorStats) == 0 {
		t.Errorf("expected at least one author in commit history")
	}

	// Test GetFileAuthors on README.md
	fileAuthors, err := runner.GetFileAuthors("README.md", 10)
	if err != nil {
		t.Fatalf("failed to get file authors for README.md: %v", err)
	}
	if len(fileAuthors) == 0 {
		t.Errorf("expected at least one author for README.md")
	}

	// Test GetInitialCommit
	initCommit, err := runner.GetInitialCommit()
	if err != nil || initCommit == "" {
		t.Fatalf("expected initial commit SHA, got %s, err: %v", initCommit, err)
	}

	// Test GetReleaseTags
	tags, err := runner.GetReleaseTags()
	if err != nil {
		t.Fatalf("failed to get release tags: %v", err)
	}
	if len(tags) == 0 {
		t.Errorf("expected at least one tag in repo history")
	}

	// Test GetLatestTwoTags
	baseTag, headTag, err := runner.GetLatestTwoTags()
	if err != nil {
		t.Fatalf("failed to get latest two tags: %v", err)
	}
	if baseTag == "" || headTag == "" {
		t.Errorf("expected non-empty base and head tags, got base=%s, head=%s", baseTag, headTag)
	}

	// Test GetPreviousTag
	prevTag, err := runner.GetPreviousTag(headTag)
	if err != nil {
		t.Fatalf("failed to get previous tag for %s: %v", headTag, err)
	}
	if prevTag == "" {
		t.Errorf("expected previous tag for %s, got empty", headTag)
	}

	// Test GetReleaseCommits between baseTag and headTag
	commits, err := runner.GetReleaseCommits(baseTag, headTag)
	if err != nil {
		t.Fatalf("failed to get release commits between %s and %s: %v", baseTag, headTag, err)
	}
	if len(commits) == 0 {
		t.Logf("no commits between %s and %s (may be identical commit)", baseTag, headTag)
	} else {
		if commits[0].Hash == "" || commits[0].Subject == "" {
			t.Errorf("expected valid commit metadata, got %+v", commits[0])
		}
	}

	// Test GetLatestTag
	latestTag, err := runner.GetLatestTag()
	if err != nil {
		t.Fatalf("failed to get latest tag: %v", err)
	}
	if !IsSemverTag(latestTag) {
		t.Errorf("expected latest tag to be valid SemVer, got %q", latestTag)
	}
}

func TestIsSemverTag(t *testing.T) {
	tests := []struct {
		tag      string
		expected bool
	}{
		{"v1.0.0", true},
		{"v0.4.0", true},
		{"0.4.0", true},
		{"v1.2.3-rc1", true},
		{"v2.0.0+build123", true},
		{"not-a-tag", false},
		{"v1", false},
		{"v1.2", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			if got := IsSemverTag(tt.tag); got != tt.expected {
				t.Errorf("IsSemverTag(%q) = %v, expected %v", tt.tag, got, tt.expected)
			}
		})
	}
}

func TestSyntheticTagOrdering(t *testing.T) {
	tempDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.name", "Tag Tester")
	runGit("config", "user.email", "tag@example.com")
	runGit("config", "commit.gpgsign", "false")

	// Create commits and tags: v0.1.0, v0.2.0, v0.10.0, v0.3.0
	_ = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("1"), 0644)
	runGit("add", "file.txt")
	runGit("commit", "-m", "commit 1")
	runGit("tag", "v0.1.0")

	_ = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("2"), 0644)
	runGit("add", "file.txt")
	runGit("commit", "-m", "commit 2")
	runGit("tag", "v0.2.0")

	_ = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("3"), 0644)
	runGit("add", "file.txt")
	runGit("commit", "-m", "commit 3")
	runGit("tag", "v0.10.0")

	_ = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("4"), 0644)
	runGit("add", "file.txt")
	runGit("commit", "-m", "commit 4")
	runGit("tag", "v0.3.0")

	runner := NewRunner(tempDir)
	latest, err := runner.GetLatestTag()
	if err != nil {
		t.Fatalf("GetLatestTag failed: %v", err)
	}

	// In SemVer ordering, v0.10.0 is greater than v0.3.0
	if latest != "v0.10.0" {
		t.Errorf("expected latest SemVer tag to be v0.10.0, got %q", latest)
	}
}

func TestGetAheadBehind(t *testing.T) {
	tempDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.name", "Drift Tester")
	runGit("config", "user.email", "drift@example.com")
	runGit("config", "commit.gpgsign", "false")

	// Commit 1 on main
	_ = os.WriteFile(filepath.Join(tempDir, "base.txt"), []byte("base"), 0644)
	runGit("add", "base.txt")
	runGit("commit", "-m", "initial commit")

	// Create and checkout staging
	runGit("checkout", "-b", "staging")

	// Commit 2 and 3 on staging
	_ = os.WriteFile(filepath.Join(tempDir, "staging1.txt"), []byte("1"), 0644)
	runGit("add", "staging1.txt")
	runGit("commit", "-m", "staging commit 1")

	_ = os.WriteFile(filepath.Join(tempDir, "staging2.txt"), []byte("2"), 0644)
	runGit("add", "staging2.txt")
	runGit("commit", "-m", "staging commit 2")

	// Switch back to main and make 1 commit
	runGit("checkout", "main")
	_ = os.WriteFile(filepath.Join(tempDir, "main_hotfix.txt"), []byte("hotfix"), 0644)
	runGit("add", "main_hotfix.txt")
	runGit("commit", "-m", "main hotfix")

	runner := NewRunner(tempDir)

	// Check ahead/behind: staging vs main
	ahead, behind, err := runner.GetAheadBehind("main", "staging")
	if err != nil {
		t.Fatalf("GetAheadBehind failed: %v", err)
	}
	if ahead != 2 {
		t.Errorf("expected staging to be 2 commits ahead of main, got %d", ahead)
	}
	if behind != 1 {
		t.Errorf("expected staging to be 1 commit behind main, got %d", behind)
	}

	// Reverse check: main vs staging
	aheadRev, behindRev, err := runner.GetAheadBehind("staging", "main")
	if err != nil {
		t.Fatalf("GetAheadBehind reverse failed: %v", err)
	}
	if aheadRev != 1 {
		t.Errorf("expected main to be 1 commit ahead of staging, got %d", aheadRev)
	}
	if behindRev != 2 {
		t.Errorf("expected main to be 2 commits behind staging, got %d", behindRev)
	}

	// Equal branches
	aheadSame, behindSame, err := runner.GetAheadBehind("main", "main")
	if err != nil {
		t.Fatalf("GetAheadBehind equal failed: %v", err)
	}
	if aheadSame != 0 || behindSame != 0 {
		t.Errorf("expected 0 ahead and 0 behind for identical branch, got ahead=%d, behind=%d", aheadSame, behindSame)
	}
}


