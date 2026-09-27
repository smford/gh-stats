package gitutil

import (
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
}

