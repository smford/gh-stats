package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDetectRepoSlug(t *testing.T) {
	// From environment variable
	t.Setenv("GITHUB_REPOSITORY", "octocat/Hello-World")
	slug, err := DetectRepoSlug("")
	if err != nil || slug != "octocat/Hello-World" {
		t.Errorf("expected octocat/Hello-World, got %s (err: %v)", slug, err)
	}

	// From git SSH remote
	os.Unsetenv("GITHUB_REPOSITORY")
	slug, err = DetectRepoSlug("git@github.com:smford/gh-stats.git")
	if err != nil || slug != "smford/gh-stats" {
		t.Errorf("expected smford/gh-stats, got %s (err: %v)", slug, err)
	}

	// From HTTPS remote
	slug, err = DetectRepoSlug("https://github.com/smford/gh-stats.git")
	if err != nil || slug != "smford/gh-stats" {
		t.Errorf("expected smford/gh-stats from HTTPS, got %s (err: %v)", slug, err)
	}
}

func TestDetectPRNumber(t *testing.T) {
	t.Setenv("GITHUB_REF", "refs/pull/42/merge")
	num := DetectPRNumber()
	if num != 42 {
		t.Errorf("expected PR number 42, got %d", num)
	}
}

func TestGetPRMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/pulls/10":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"number": 10,
				"title": "Fix memory leak",
				"created_at": "2026-09-20T10:00:00Z",
				"author_association": "MEMBER",
				"comments": 4,
				"review_comments": 8
			}`))
		case "/repos/owner/repo/pulls/10/reviews":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"state": "APPROVED",
					"submitted_at": "2026-09-20T12:30:00Z"
				}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient("dummy-token")
	client.baseURL = server.URL

	meta, err := client.GetPRMetadata(context.Background(), "owner/repo", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Number != 10 {
		t.Errorf("expected PR 10, got %d", meta.Number)
	}
	if meta.TotalDiscussions != 12 {
		t.Errorf("expected 12 total comments, got %d", meta.TotalDiscussions)
	}
	if meta.ApprovalsCount != 1 {
		t.Errorf("expected 1 approval, got %d", meta.ApprovalsCount)
	}
	if meta.TimeToFirstReview != 2*time.Hour+30*time.Minute {
		t.Errorf("expected 2h30m TTFR, got %v", meta.TimeToFirstReview)
	}
}
