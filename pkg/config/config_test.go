package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Thresholds.MaxPRLines != 800 {
		t.Errorf("expected 800 MaxPRLines, got %d", cfg.Thresholds.MaxPRLines)
	}
	if cfg.Thresholds.StalePRDays != 14 {
		t.Errorf("expected 14 StalePRDays, got %d", cfg.Thresholds.StalePRDays)
	}
	if cfg.Thresholds.CommitLimit != 200 {
		t.Errorf("expected 200 CommitLimit, got %d", cfg.Thresholds.CommitLimit)
	}
	if cfg.Thresholds.MaxCILatencyMinutes != 15 {
		t.Errorf("expected 15 MaxCILatencyMinutes, got %d", cfg.Thresholds.MaxCILatencyMinutes)
	}
	if cfg.Thresholds.FailOnFlakyCI != false {
		t.Errorf("expected false FailOnFlakyCI, got %v", cfg.Thresholds.FailOnFlakyCI)
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
thresholds:
  max_pr_lines: 450
  stale_pr_days: 7
  min_test_ratio: 0.35
  max_discussions: 10
  commit_limit: 100
  max_ci_latency_minutes: 20
  fail_on_flaky_ci: true
  max_ci_retries: 2

blast_radius:
  custom_patterns:
    - category: "Payment & Billing"
      pattern: "services/billing/**"
      description: "Payment gateway modifications"
    - category: "Public API Contract"
      pattern: "api/openapi.yaml"
      description: "Public API contract"

ignore:
  paths:
    - "legacy/**"
    - "docs/*"

fail_on: "HIGH"
`
	configPath := filepath.Join(tmpDir, ".gh-stats.yml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath, tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Thresholds.MaxPRLines != 450 {
		t.Errorf("expected 450 MaxPRLines, got %d", cfg.Thresholds.MaxPRLines)
	}
	if cfg.Thresholds.StalePRDays != 7 {
		t.Errorf("expected 7 StalePRDays, got %d", cfg.Thresholds.StalePRDays)
	}
	if cfg.Thresholds.MinTestRatio != 0.35 {
		t.Errorf("expected 0.35 MinTestRatio, got %f", cfg.Thresholds.MinTestRatio)
	}
	if cfg.Thresholds.MaxCILatencyMinutes != 20 {
		t.Errorf("expected 20 MaxCILatencyMinutes, got %d", cfg.Thresholds.MaxCILatencyMinutes)
	}
	if !cfg.Thresholds.FailOnFlakyCI {
		t.Errorf("expected true FailOnFlakyCI, got %v", cfg.Thresholds.FailOnFlakyCI)
	}
	if cfg.Thresholds.MaxCIRetries != 2 {
		t.Errorf("expected 2 MaxCIRetries, got %d", cfg.Thresholds.MaxCIRetries)
	}
	if cfg.FailOn != "HIGH" {
		t.Errorf("expected HIGH fail_on, got %s", cfg.FailOn)
	}
	if len(cfg.BlastRadius.CustomPatterns) != 2 {
		t.Errorf("expected 2 custom patterns, got %d", len(cfg.BlastRadius.CustomPatterns))
	}
	if len(cfg.Ignore.Paths) != 2 {
		t.Errorf("expected 2 ignore paths, got %d", len(cfg.Ignore.Paths))
	}

	// Test IsIgnored
	if !cfg.IsIgnored("legacy/old_service.go") {
		t.Errorf("expected legacy/old_service.go to be ignored")
	}
	if cfg.IsIgnored("pkg/auth/token.go") {
		t.Errorf("did not expect pkg/auth/token.go to be ignored")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern  string
		target   string
		expected bool
	}{
		{"services/billing/**", "services/billing/payment.go", true},
		{"services/billing/**", "services/billing/stripe/charge.go", true},
		{"services/billing/**", "services/auth/token.go", false},
		{"api/*.yaml", "api/openapi.yaml", true},
		{"api/*.yaml", "api/v1/openapi.yaml", false},
		{"docs/**", "docs/architecture/diagram.png", true},
	}

	for _, tt := range tests {
		got := MatchGlob(tt.pattern, tt.target)
		if got != tt.expected {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", tt.pattern, tt.target, got, tt.expected)
		}
	}
}

func TestExportConfigPromAndOTel(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
export:
  json_path: "dora.json"
  prom_path: "metrics.prom"
  otel_endpoint: "http://otel-collector:4318/v1/metrics"
  otel_headers:
    X-API-Key: "secret-key"
`
	configPath := filepath.Join(tmpDir, ".gh-stats.yml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath, tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Export.GetPromPath() != "metrics.prom" {
		t.Errorf("expected prom_path 'metrics.prom', got %q", cfg.Export.GetPromPath())
	}
	if cfg.Export.GetOTelEndpoint() != "http://otel-collector:4318/v1/metrics" {
		t.Errorf("expected otel_endpoint 'http://otel-collector:4318/v1/metrics', got %q", cfg.Export.GetOTelEndpoint())
	}
	if cfg.Export.OTelHeaders["X-API-Key"] != "secret-key" {
		t.Errorf("expected X-API-Key header 'secret-key', got %q", cfg.Export.OTelHeaders["X-API-Key"])
	}

	// Test aliases export_prom and export_otel_endpoint
	cfgAliases := &Config{
		Export: ExportConfig{
			ExportProm:         "alias.prom",
			ExportOTelEndpoint: "http://otel-alias:4318/v1/metrics",
		},
	}
	if cfgAliases.Export.GetPromPath() != "alias.prom" {
		t.Errorf("expected alias.prom, got %q", cfgAliases.Export.GetPromPath())
	}
	if cfgAliases.Export.GetOTelEndpoint() != "http://otel-alias:4318/v1/metrics" {
		t.Errorf("expected http://otel-alias:4318/v1/metrics, got %q", cfgAliases.Export.GetOTelEndpoint())
	}
}

