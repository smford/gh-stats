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
}
