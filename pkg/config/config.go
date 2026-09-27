package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config defines configurable thresholds, blast radius paths, and ignore patterns.
type Config struct {
	Thresholds  ThresholdsConfig  `yaml:"thresholds"`
	BlastRadius BlastRadiusConfig `yaml:"blast_radius"`
	Ignore      IgnoreConfig      `yaml:"ignore"`
	Export      ExportConfig      `yaml:"export"`
	FailOn      string            `yaml:"fail_on"`
}

// ExportConfig configures telemetry, metrics JSON export, and webhook destinations.
type ExportConfig struct {
	JSONPath      string `yaml:"json_path"`
	WebhookURL    string `yaml:"webhook_url"`
	WebhookSecret string `yaml:"webhook_secret"`
}

// ThresholdsConfig configures limits for PR risk scoring and alerts.
type ThresholdsConfig struct {
	MaxPRLines          int     `yaml:"max_pr_lines"`           // default 800
	StalePRDays         int     `yaml:"stale_pr_days"`          // default 14
	MinTestRatio        float64 `yaml:"min_test_ratio"`         // default 0.2
	MaxDiscussions      int     `yaml:"max_discussions"`        // default 15
	CommitLimit         int     `yaml:"commit_limit"`           // default 200
	MaxCILatencyMinutes int     `yaml:"max_ci_latency_minutes"` // default 15
	FailOnFlakyCI       bool    `yaml:"fail_on_flaky_ci"`       // default false
	MaxCIRetries        int     `yaml:"max_ci_retries"`         // default 1
}

// BlastRadiusConfig specifies custom sensitive paths.
type BlastRadiusConfig struct {
	CustomPatterns []CustomPattern `yaml:"custom_patterns"`
}

// CustomPattern defines a custom sensitive pattern.
type CustomPattern struct {
	Category    string `yaml:"category"`
	Pattern     string `yaml:"pattern"`
	Description string `yaml:"description"`
}

// IgnoreConfig lists file paths or globs to ignore from analysis.
type IgnoreConfig struct {
	Paths []string `yaml:"paths"`
}

// DefaultConfig returns baseline SRE configurations.
func DefaultConfig() *Config {
	return &Config{
		Thresholds: ThresholdsConfig{
			MaxPRLines:          800,
			StalePRDays:         14,
			MinTestRatio:        0.2,
			MaxDiscussions:      15,
			CommitLimit:         200,
			MaxCILatencyMinutes: 15,
			FailOnFlakyCI:       false,
			MaxCIRetries:        1,
		},
		BlastRadius: BlastRadiusConfig{
			CustomPatterns: make([]CustomPattern, 0),
		},
		Ignore: IgnoreConfig{
			Paths: make([]string, 0),
		},
		Export: ExportConfig{},
	}
}

// LoadConfig attempts to read configuration from a file or search default locations.
func LoadConfig(customPath, repoDir string) (*Config, error) {
	cfg := DefaultConfig()

	candidatePaths := []string{}
	if customPath != "" {
		if filepath.IsAbs(customPath) {
			candidatePaths = append(candidatePaths, customPath)
		} else {
			candidatePaths = append(candidatePaths, filepath.Join(repoDir, customPath))
		}
	} else {
		candidatePaths = append(candidatePaths,
			filepath.Join(repoDir, ".gh-stats.yml"),
			filepath.Join(repoDir, ".gh-stats.yaml"),
			filepath.Join(repoDir, ".github", ".gh-stats.yml"),
			filepath.Join(repoDir, ".github", ".gh-stats.yaml"),
		)
	}

	var foundPath string
	for _, p := range candidatePaths {
		if _, err := os.Stat(p); err == nil {
			foundPath = p
			break
		}
	}

	if foundPath == "" {
		if customPath != "" {
			return nil, fmt.Errorf("configuration file not found: %s", customPath)
		}
		return cfg, nil // Return defaults if no configuration file is present
	}

	data, err := os.ReadFile(foundPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", foundPath, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", foundPath, err)
	}

	// Apply minimum sanity checks
	if cfg.Thresholds.MaxPRLines <= 0 {
		cfg.Thresholds.MaxPRLines = 800
	}
	if cfg.Thresholds.StalePRDays <= 0 {
		cfg.Thresholds.StalePRDays = 14
	}
	if cfg.Thresholds.CommitLimit <= 0 {
		cfg.Thresholds.CommitLimit = 200
	}

	return cfg, nil
}

// MatchGlob checks if a target path matches a glob pattern, supporting '**' for recursive dirs.
func MatchGlob(pattern, target string) bool {
	pattern = filepath.ToSlash(pattern)
	target = filepath.ToSlash(target)

	// Direct equality or contains
	if pattern == target {
		return true
	}

	// Handle '**' wildcard
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			if strings.HasPrefix(target, prefix) && strings.HasSuffix(target, suffix) {
				return true
			}
		}
	}

	// Standard filepath.Match fallback
	matched, err := filepath.Match(pattern, target)
	if err == nil && matched {
		return true
	}

	// Prefix match if pattern is a directory ending with /
	if strings.HasSuffix(pattern, "/") && strings.HasPrefix(target, pattern) {
		return true
	}

	return false
}

// IsIgnored tests if a path matches any ignore patterns.
func (c *Config) IsIgnored(path string) bool {
	for _, p := range c.Ignore.Paths {
		if MatchGlob(p, path) {
			return true
		}
	}
	return false
}

// Validate ensures configuration values are well-formed.
func (c *Config) Validate() error {
	if c.Thresholds.MinTestRatio < 0 || c.Thresholds.MinTestRatio > 1.0 {
		return errors.New("thresholds.min_test_ratio must be between 0.0 and 1.0")
	}
	return nil
}
