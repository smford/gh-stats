package gitutil

import (
	"reflect"
	"testing"
)

func TestCleanRefName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"refs/heads/main", "main"},
		{"refs/remotes/origin/feature-1", "origin/feature-1"},
		{"refs/pull/42/merge", "42/merge"},
		{"  feature-xyz  ", "feature-xyz"},
	}

	for _, tt := range tests {
		got := CleanRefName(tt.input)
		if got != tt.expected {
			t.Errorf("CleanRefName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsMainOrMaster(t *testing.T) {
	if !IsMainOrMaster("main") {
		t.Errorf("expected main to be recognized as main branch")
	}
	if !IsMainOrMaster("origin/main") {
		t.Errorf("expected origin/main to be recognized as main branch")
	}
	if !IsMainOrMaster("master") {
		t.Errorf("expected master to be recognized as master branch")
	}
	if IsMainOrMaster("feature/demo") {
		t.Errorf("did not expect feature/demo to be recognized as main branch")
	}
}

func TestTruncateCommitHash(t *testing.T) {
	if got := TruncateCommitHash("da8ad6c41d7579"); got != "da8ad6c" {
		t.Errorf("expected da8ad6c, got %s", got)
	}
	if got := TruncateCommitHash("short"); got != "short" {
		t.Errorf("expected short, got %s", got)
	}
}

func TestExtractIssueReferences(t *testing.T) {
	subject := "feat: resolve #10 and fix #42 regression"
	got := ExtractIssueReferences(subject)
	expected := []string{"#10", "#42"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}
