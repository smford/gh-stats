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

func TestPostOrUpdatePRComment(t *testing.T) {
	posted := false
	updated := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/issues/10/comments":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"id": 101, "body": "some normal comment"},
				{"id": 202, "body": "<!-- gh-stats-marker -->\nold stats"}
			]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/owner/repo/issues/comments/202":
			updated = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 202, "body": "updated"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/issues/10/comments":
			posted = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 303, "body": "created"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient("dummy-token")
	client.baseURL = server.URL

	// Test update existing
	err := client.PostOrUpdatePRComment(context.Background(), "owner/repo", 10, "<!-- gh-stats-marker -->", "new stats")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Errorf("expected comment 202 to be updated")
	}

	// Test create new (when marker not found)
	err = client.PostOrUpdatePRComment(context.Background(), "owner/repo", 10, "<!-- non-existent-marker -->", "new stats")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !posted {
		t.Errorf("expected new comment to be posted")
	}
}

func TestGetCheckRunsAndCIPipelineStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/commits/abc1234/check-runs" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"total_count": 4,
				"check_runs": [
					{
						"id": 10,
						"name": "build",
						"head_sha": "abc1234",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2026-09-27T10:00:00Z",
						"completed_at": "2026-09-27T10:03:00Z"
					},
					{
						"id": 11,
						"name": "unit-tests",
						"head_sha": "abc1234",
						"status": "completed",
						"conclusion": "failure",
						"started_at": "2026-09-27T10:00:00Z",
						"completed_at": "2026-09-27T10:02:00Z"
					},
					{
						"id": 12,
						"name": "unit-tests",
						"head_sha": "abc1234",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2026-09-27T10:05:00Z",
						"completed_at": "2026-09-27T10:07:00Z"
					},
					{
						"id": 13,
						"name": "e2e-tests",
						"head_sha": "abc1234",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2026-09-27T10:00:00Z",
						"completed_at": "2026-09-27T10:18:00Z"
					}
				]
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient("dummy-token")
	client.baseURL = server.URL

	ctx := context.Background()
	stats, err := client.GetCIPipelineStats(ctx, "owner/repo", "abc1234", 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.TotalCheckRuns != 4 {
		t.Errorf("expected 4 total check runs, got %d", stats.TotalCheckRuns)
	}
	if stats.SuccessfulRuns != 3 {
		t.Errorf("expected 3 successful runs, got %d", stats.SuccessfulRuns)
	}
	if stats.FailedRuns != 1 {
		t.Errorf("expected 1 failed run, got %d", stats.FailedRuns)
	}
	if stats.LongestRunName != "e2e-tests" {
		t.Errorf("expected longest run 'e2e-tests', got %s", stats.LongestRunName)
	}
	if stats.LongestRunDuration != 18*time.Minute {
		t.Errorf("expected longest duration 18m, got %v", stats.LongestRunDuration)
	}
	if len(stats.BottleneckRuns) != 1 || stats.BottleneckRuns[0].Name != "e2e-tests" {
		t.Errorf("expected 1 bottleneck run 'e2e-tests', got %v", stats.BottleneckRuns)
	}
	if len(stats.FlakyRuns) != 1 {
		t.Fatalf("expected 1 flaky run group, got %d", len(stats.FlakyRuns))
	}

	flaky := stats.FlakyRuns[0]
	if flaky.Name != "unit-tests" {
		t.Errorf("expected flaky run 'unit-tests', got %s", flaky.Name)
	}
	if !flaky.IsFlaky {
		t.Errorf("expected IsFlaky to be true")
	}
	if flaky.RetryCount != 1 {
		t.Errorf("expected 1 retry, got %d", flaky.RetryCount)
	}
	if flaky.InitialResult != "failure" || flaky.FinalResult != "success" {
		t.Errorf("expected initial failure and final success, got %s -> %s", flaky.InitialResult, flaky.FinalResult)
	}

	// Test empty check runs
	emptyStats, err := client.GetCIPipelineStats(ctx, "owner/repo", "non-existent", 15)
	if err != nil {
		t.Fatalf("unexpected error on 404 ref: %v", err)
	}
	if emptyStats.TotalCheckRuns != 0 {
		t.Errorf("expected 0 check runs for 404 ref, got %d", emptyStats.TotalCheckRuns)
	}
}

