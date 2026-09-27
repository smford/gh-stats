package exporter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/github"
)

func TestNewPRPayload(t *testing.T) {
	stats := &analyzer.PRStats{
		BaseRef:        "main",
		HeadRef:        "feature/auth",
		TotalAdditions: 120,
		TotalDeletions: 20,
		NetChange:      100,
		FilesChanged:   5,
		CodeLinesAdded: 100,
		TestLinesAdded: 20,
		CommitCount:    2,
		RiskScore:      45,
		RiskLevel:      "MEDIUM",
		SensitiveFiles: []analyzer.SensitiveMatch{
			{Path: "pkg/auth/token.go", Category: "Auth & Security"},
		},
		GitHubMeta: &github.PRMetadata{
			Number:           42,
			Age:              48 * time.Hour,
			TimeToFirstReview: 2 * time.Hour,
			TotalDiscussions: 6,
			ApprovalsCount:   1,
			ReviewsCount:     2,
		},
	}

	payload := NewPRPayload(stats, "smford/gh-stats")
	if payload.Target != "pr" {
		t.Errorf("expected target 'pr', got '%s'", payload.Target)
	}
	if payload.Repository != "smford/gh-stats" {
		t.Errorf("expected repo 'smford/gh-stats', got '%s'", payload.Repository)
	}
	if payload.PR == nil {
		t.Fatalf("expected PR metrics, got nil")
	}
	if payload.PR.Number != 42 {
		t.Errorf("expected PR number 42, got %d", payload.PR.Number)
	}
	if payload.PR.LeadTimeHours != 48.0 {
		t.Errorf("expected lead time 48.0 hours, got %f", payload.PR.LeadTimeHours)
	}
	if payload.PR.TestRatio != 0.2 {
		t.Errorf("expected test ratio 0.2, got %f", payload.PR.TestRatio)
	}
	if len(payload.PR.SensitiveCategories) != 1 || payload.PR.SensitiveCategories[0] != "Auth & Security" {
		t.Errorf("expected 'Auth & Security' category, got %v", payload.PR.SensitiveCategories)
	}
}

func TestNewRepoPayload(t *testing.T) {
	stats := &analyzer.RepoStats{
		TotalFiles:          50,
		TestFilesCount:      15,
		DocFilesCount:       5,
		TestFileRatio:       30.0,
		TopAuthorPercentage: 60.0,
		ChurnHotspots: []analyzer.ChurnEntry{
			{Path: "main.go", Count: 10},
		},
		GitHubMeta: &github.RepoMetadata{
			StargazersCount: 100,
			ForksCount:      15,
			OpenIssuesCount: 3,
		},
	}

	payload := NewRepoPayload(stats, "smford/gh-stats")
	if payload.Target != "repo" {
		t.Errorf("expected target 'repo', got '%s'", payload.Target)
	}
	if payload.Repo == nil {
		t.Fatalf("expected Repo metrics, got nil")
	}
	if payload.Repo.TotalFiles != 50 {
		t.Errorf("expected total files 50, got %d", payload.Repo.TotalFiles)
	}
	if payload.Repo.StarsCount != 100 {
		t.Errorf("expected stars count 100, got %d", payload.Repo.StarsCount)
	}
}

func TestExportJSON(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "nested", "metrics.json")

	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "pr",
		Repository:    "smford/gh-stats",
	}

	if err := ExportJSON(payload, outPath); err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read exported JSON: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("expected non-empty JSON file")
	}
}

func TestExportWebhook(t *testing.T) {
	var receivedBody []byte
	var receivedAuth string
	var receivedSig string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedSig = r.Header.Get("X-Hub-Signature-256")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "pr",
		Repository:    "smford/gh-stats",
	}

	secret := "super-secret-token"
	ctx := context.Background()

	err := ExportWebhook(ctx, payload, server.URL, secret)
	if err != nil {
		t.Fatalf("ExportWebhook failed: %v", err)
	}

	if receivedAuth != "Bearer "+secret {
		t.Errorf("expected auth 'Bearer %s', got '%s'", secret, receivedAuth)
	}
	if receivedSig == "" {
		t.Errorf("expected non-empty HMAC signature header")
	}
	if len(receivedBody) == 0 {
		t.Errorf("expected non-empty webhook payload")
	}
}
