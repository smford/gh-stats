package exporter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildOTLPMetricsPayload(t *testing.T) {
	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		Target:        "pr",
		Repository:    "smford/gh-stats",
		Ref:           "feature/otlp",
		PR: &PRMetrics{
			Number:                 101,
			RiskScore:              45,
			RiskLevel:              "MEDIUM",
			LeadTimeHours:          24.5,
			TimeToFirstReviewHours: 2.1,
			TotalAdditions:         300,
			TotalDeletions:         50,
			NetChange:              250,
			FilesChanged:           6,
			TestRatio:              0.35,
			SensitiveFilesCount:    1,
			CommitsCount:           3,
			DiscussionCount:        8,
			CIPipeline: &CIPipelineMetrics{
				TotalCheckRuns:       4,
				TotalDurationSeconds: 180.0,
				FlakyRunsCount:       1,
			},
		},
	}

	otlp := BuildOTLPMetricsPayload(payload)
	if len(otlp.ResourceMetrics) == 0 {
		t.Fatalf("expected at least 1 ResourceMetric")
	}

	res := otlp.ResourceMetrics[0]
	var foundRepo, foundService bool
	for _, attr := range res.Resource.Attributes {
		if attr.Key == "service.name" && attr.Value.StringValue != nil && *attr.Value.StringValue == "gh-stats" {
			foundService = true
		}
		if attr.Key == "vcs.repository.name" && attr.Value.StringValue != nil && *attr.Value.StringValue == "smford/gh-stats" {
			foundRepo = true
		}
	}
	if !foundService || !foundRepo {
		t.Errorf("missing service.name or vcs.repository.name in resource attributes")
	}

	if len(res.ScopeMetrics) == 0 {
		t.Fatalf("expected at least 1 ScopeMetric")
	}

	metrics := res.ScopeMetrics[0].Metrics
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.Name] = true
	}

	expectedMetrics := []string{
		"gh_stats.risk_score",
		"gh_stats.lines_added",
		"gh_stats.lines_deleted",
		"gh_stats.files_changed",
		"gh_stats.test_ratio",
		"gh_stats.pr.lead_time",
		"gh_stats.ci.latency",
		"gh_stats.ci.flaky_checks",
	}

	for _, name := range expectedMetrics {
		if !metricNames[name] {
			t.Errorf("expected metric %q in OTLP payload", name)
		}
	}

	// Verify valid JSON marshaling
	jsonData, err := json.Marshal(otlp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !strings.Contains(string(jsonData), "gh_stats.risk_score") {
		t.Errorf("expected serialized JSON to contain gh_stats.risk_score")
	}
}

func TestParseOTLPHeaders(t *testing.T) {
	// Comma-separated
	h1 := ParseOTLPHeaders("api-key=secret123,tenant-id=456")
	if h1["api-key"] != "secret123" || h1["tenant-id"] != "456" {
		t.Errorf("unexpected parsed comma headers: %+v", h1)
	}

	// Newline-separated with spaces
	h2 := ParseOTLPHeaders("Authorization = Bearer my-token \n X-Env = prod")
	if h2["Authorization"] != "Bearer my-token" || h2["X-Env"] != "prod" {
		t.Errorf("unexpected parsed newline headers: %+v", h2)
	}

	// Empty
	h3 := ParseOTLPHeaders("")
	if len(h3) != 0 {
		t.Errorf("expected empty headers, got: %+v", h3)
	}
}

func TestExportOTLP(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedAuth = r.Header.Get("Authorization")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	payload := &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "pr",
		Repository:    "smford/gh-stats",
		PR: &PRMetrics{
			RiskScore:      30,
			TotalAdditions: 100,
		},
	}

	ctx := context.Background()
	headers := map[string]string{
		"Authorization": "Bearer test-otel-token",
	}

	if err := ExportOTLP(ctx, payload, server.URL, headers); err != nil {
		t.Fatalf("ExportOTLP failed: %v", err)
	}

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}
	if receivedAuth != "Bearer test-otel-token" {
		t.Errorf("expected Authorization Bearer test-otel-token, got %q", receivedAuth)
	}
	if !strings.Contains(string(receivedBody), "gh_stats.risk_score") {
		t.Errorf("expected body to contain gh_stats.risk_score, got:\n%s", string(receivedBody))
	}

	// Test non-2xx response error handling
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"collector unavailable"}`))
	}))
	defer failServer.Close()

	err := ExportOTLP(ctx, payload, failServer.URL, nil)
	if err == nil {
		t.Errorf("expected error from 502 Bad Gateway response")
	} else if !strings.Contains(err.Error(), "502") {
		t.Errorf("expected error to mention 502 status, got: %v", err)
	}
}
