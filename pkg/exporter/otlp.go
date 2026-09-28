package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// OTLPMetricsPayload models the standard OpenTelemetry OTLP/HTTP v1/metrics JSON schema.
type OTLPMetricsPayload struct {
	ResourceMetrics []OTLPResourceMetrics `json:"resourceMetrics"`
}

type OTLPResourceMetrics struct {
	Resource     OTLPResource       `json:"resource"`
	ScopeMetrics []OTLPScopeMetrics `json:"scopeMetrics"`
}

type OTLPResource struct {
	Attributes []OTLPAttribute `json:"attributes"`
}

type OTLPAttribute struct {
	Key   string       `json:"key"`
	Value OTLPAnyValue `json:"value"`
}

type OTLPAnyValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *int64   `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

type OTLPScopeMetrics struct {
	Scope   OTLPInstrumentationScope `json:"scope"`
	Metrics []OTLPMetric             `json:"metrics"`
}

type OTLPInstrumentationScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type OTLPMetric struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	Gauge       *OTLPGauge `json:"gauge,omitempty"`
	Sum         *OTLPSum   `json:"sum,omitempty"`
}

type OTLPGauge struct {
	DataPoints []OTLPDataPoint `json:"dataPoints"`
}

type OTLPSum struct {
	AggregationTemporality int             `json:"aggregationTemporality"` // 2 = AGGREGATION_TEMPORALITY_CUMULATIVE
	IsMonotonic            bool            `json:"isMonotonic"`
	DataPoints             []OTLPDataPoint `json:"dataPoints"`
}

type OTLPDataPoint struct {
	Attributes   []OTLPAttribute `json:"attributes,omitempty"`
	TimeUnixNano string          `json:"timeUnixNano"`
	AsInt        *int64          `json:"asInt,omitempty"`
	AsDouble     *float64        `json:"asDouble,omitempty"`
}

func otlpStringAttr(key, val string) OTLPAttribute {
	return OTLPAttribute{
		Key: key,
		Value: OTLPAnyValue{
			StringValue: &val,
		},
	}
}

func otlpGaugeInt(name, desc, unit string, val int64, timeNano string, attrs []OTLPAttribute) OTLPMetric {
	return OTLPMetric{
		Name:        name,
		Description: desc,
		Unit:        unit,
		Gauge: &OTLPGauge{
			DataPoints: []OTLPDataPoint{
				{
					Attributes:   attrs,
					TimeUnixNano: timeNano,
					AsInt:        &val,
				},
			},
		},
	}
}

func otlpGaugeDouble(name, desc, unit string, val float64, timeNano string, attrs []OTLPAttribute) OTLPMetric {
	return OTLPMetric{
		Name:        name,
		Description: desc,
		Unit:        unit,
		Gauge: &OTLPGauge{
			DataPoints: []OTLPDataPoint{
				{
					Attributes:   attrs,
					TimeUnixNano: timeNano,
					AsDouble:     &val,
				},
			},
		},
	}
}

func otlpSumInt(name, desc, unit string, val int64, timeNano string, attrs []OTLPAttribute) OTLPMetric {
	return OTLPMetric{
		Name:        name,
		Description: desc,
		Unit:        unit,
		Sum: &OTLPSum{
			AggregationTemporality: 2, // Cumulative
			IsMonotonic:            true,
			DataPoints: []OTLPDataPoint{
				{
					Attributes:   attrs,
					TimeUnixNano: timeNano,
					AsInt:        &val,
				},
			},
		},
	}
}

// BuildOTLPMetricsPayload constructs a standards-compliant OTLP JSON metrics request from DORAMetricsPayload.
func BuildOTLPMetricsPayload(payload *DORAMetricsPayload) *OTLPMetricsPayload {
	if payload == nil {
		return &OTLPMetricsPayload{}
	}

	timeNano := strconv.FormatInt(payload.Timestamp.UnixNano(), 10)

	resourceAttrs := []OTLPAttribute{
		otlpStringAttr("service.name", "gh-stats"),
	}
	if payload.Repository != "" {
		resourceAttrs = append(resourceAttrs, otlpStringAttr("vcs.repository.name", payload.Repository))
	}
	if payload.Ref != "" {
		resourceAttrs = append(resourceAttrs, otlpStringAttr("vcs.ref.name", payload.Ref))
	}

	baseAttrs := []OTLPAttribute{
		otlpStringAttr("target", payload.Target),
	}

	var metrics []OTLPMetric

	// 1. Target: PR
	if payload.PR != nil {
		pr := payload.PR
		attrs := append([]OTLPAttribute{}, baseAttrs...)
		if pr.Number > 0 {
			attrs = append(attrs, otlpStringAttr("pr.number", fmt.Sprintf("%d", pr.Number)))
		}
		if pr.RiskLevel != "" {
			attrs = append(attrs, otlpStringAttr("risk_level", pr.RiskLevel))
		}

		metrics = append(metrics,
			otlpGaugeInt("gh_stats.risk_score", "SRE reliability and deployment risk score (0-100)", "1", int64(pr.RiskScore), timeNano, attrs),
			otlpSumInt("gh_stats.lines_added", "Total production lines added", "lines", int64(pr.TotalAdditions), timeNano, attrs),
			otlpSumInt("gh_stats.lines_deleted", "Total production lines deleted", "lines", int64(pr.TotalDeletions), timeNano, attrs),
			otlpGaugeInt("gh_stats.files_changed", "Total number of modified files", "files", int64(pr.FilesChanged), timeNano, attrs),
			otlpGaugeDouble("gh_stats.test_ratio", "Ratio of automated test lines added to code lines added", "1", pr.TestRatio, timeNano, attrs),
			otlpGaugeInt("gh_stats.sensitive_files", "Number of high-blast-radius sensitive files modified", "files", int64(pr.SensitiveFilesCount), timeNano, attrs),
			otlpSumInt("gh_stats.commits", "Number of commits in change delta", "commits", int64(pr.CommitsCount), timeNano, attrs),
		)

		if pr.LeadTimeHours > 0 {
			metrics = append(metrics, otlpGaugeDouble("gh_stats.pr.lead_time", "PR lead time duration in hours", "h", pr.LeadTimeHours, timeNano, attrs))
		}
		if pr.TimeToFirstReviewHours > 0 {
			metrics = append(metrics, otlpGaugeDouble("gh_stats.pr.time_to_first_review", "Time to first human review in hours", "h", pr.TimeToFirstReviewHours, timeNano, attrs))
		}
		if pr.DiscussionCount > 0 {
			metrics = append(metrics, otlpSumInt("gh_stats.pr.discussions", "Total discussion comments count", "discussions", int64(pr.DiscussionCount), timeNano, attrs))
		}

		if pr.CIPipeline != nil {
			ci := pr.CIPipeline
			metrics = append(metrics,
				otlpGaugeDouble("gh_stats.ci.latency", "Total cumulative duration of all CI check runs in seconds", "s", ci.TotalDurationSeconds, timeNano, attrs),
				otlpSumInt("gh_stats.ci.flaky_checks", "Number of flaky CI check runs detected on retry", "checks", int64(ci.FlakyRunsCount), timeNano, attrs),
				otlpSumInt("gh_stats.ci.check_runs", "Total number of evaluated CI check runs", "runs", int64(ci.TotalCheckRuns), timeNano, attrs),
			)
		}

		if pr.RiskBudget != nil {
			rbAttrs := append([]OTLPAttribute{}, attrs...)
			rbAttrs = append(rbAttrs, otlpStringAttr("status", pr.RiskBudget.Status))
			metrics = append(metrics,
				otlpGaugeDouble("gh_stats.risk_budget.utilization", "SRE risk budget utilization percentage", "%", pr.RiskBudget.UtilizationPercent, timeNano, rbAttrs),
				otlpGaugeDouble("gh_stats.risk_budget.burn_rate", "SRE SLO risk budget burn rate", "1", pr.RiskBudget.BurnRate, timeNano, rbAttrs),
				otlpGaugeInt("gh_stats.risk_budget.projected_points", "Total cumulative projected risk points", "points", int64(pr.RiskBudget.TotalProjectedPoints), timeNano, attrs),
			)
		}
	}

	// 2. Target: Release
	if payload.Release != nil {
		rel := payload.Release
		attrs := append([]OTLPAttribute{}, baseAttrs...)
		if rel.BaseRef != "" {
			attrs = append(attrs, otlpStringAttr("base_ref", rel.BaseRef))
		}
		if rel.HeadRef != "" {
			attrs = append(attrs, otlpStringAttr("head_ref", rel.HeadRef))
		}
		if rel.RiskLevel != "" {
			attrs = append(attrs, otlpStringAttr("risk_level", rel.RiskLevel))
		}

		metrics = append(metrics,
			otlpGaugeInt("gh_stats.risk_score", "SRE reliability and deployment risk score (0-100)", "1", int64(rel.RiskScore), timeNano, attrs),
			otlpSumInt("gh_stats.lines_added", "Total production lines added in release", "lines", int64(rel.TotalAdditions), timeNano, attrs),
			otlpSumInt("gh_stats.lines_deleted", "Total production lines deleted in release", "lines", int64(rel.TotalDeletions), timeNano, attrs),
			otlpGaugeInt("gh_stats.files_changed", "Total number of modified files in release", "files", int64(rel.FilesChanged), timeNano, attrs),
			otlpGaugeDouble("gh_stats.test_ratio", "Ratio of automated test lines to code lines added in release", "1", rel.TestRatio, timeNano, attrs),
			otlpSumInt("gh_stats.release.commits", "Total commits in release delta", "commits", int64(rel.TotalCommits), timeNano, attrs),
			otlpSumInt("gh_stats.release.breaking_changes", "Total breaking changes and schema migrations in release", "changes", int64(rel.BreakingChangesCount), timeNano, attrs),
			otlpGaugeInt("gh_stats.release.contributors", "Number of unique contributors in release", "contributors", int64(rel.ContributorsCount), timeNano, attrs),
			otlpGaugeInt("gh_stats.sensitive_files", "Number of high-blast-radius sensitive files modified in release", "files", int64(rel.SensitiveFilesCount), timeNano, attrs),
		)
	}

	// 3. Target: Drift
	if payload.Drift != nil {
		drift := payload.Drift
		attrs := append([]OTLPAttribute{}, baseAttrs...)
		if drift.BaseRef != "" {
			attrs = append(attrs, otlpStringAttr("base_ref", drift.BaseRef))
		}
		if drift.HeadRef != "" {
			attrs = append(attrs, otlpStringAttr("head_ref", drift.HeadRef))
		}
		if drift.RiskLevel != "" {
			attrs = append(attrs, otlpStringAttr("risk_level", drift.RiskLevel))
		}

		metrics = append(metrics,
			otlpGaugeInt("gh_stats.risk_score", "SRE promotion deployment risk score (0-100)", "1", int64(drift.RiskScore), timeNano, attrs),
			otlpSumInt("gh_stats.lines_added", "Total unpromoted lines added", "lines", int64(drift.TotalAdditions), timeNano, attrs),
			otlpSumInt("gh_stats.lines_deleted", "Total unpromoted lines deleted", "lines", int64(drift.TotalDeletions), timeNano, attrs),
			otlpGaugeInt("gh_stats.files_changed", "Total unpromoted files modified", "files", int64(drift.FilesChanged), timeNano, attrs),
			otlpGaugeInt("gh_stats.drift.commits_ahead", "Number of unpromoted commits awaiting promotion", "commits", int64(drift.CommitsAhead), timeNano, attrs),
			otlpGaugeInt("gh_stats.drift.commits_behind", "Number of missing upstream commits diverged from base", "commits", int64(drift.CommitsBehind), timeNano, attrs),
			otlpSumInt("gh_stats.release.breaking_changes", "Number of unpromoted breaking changes or schema migrations", "changes", int64(drift.BreakingChangesCount), timeNano, attrs),
			otlpGaugeInt("gh_stats.sensitive_files", "Number of unpromoted high-blast-radius files modified", "files", int64(drift.SensitiveFilesCount), timeNano, attrs),
		)
	}

	// 4. Target: Repo
	if payload.Repo != nil {
		repo := payload.Repo
		metrics = append(metrics,
			otlpGaugeInt("gh_stats.repo.files", "Total tracked repository files", "files", int64(repo.TotalFiles), timeNano, baseAttrs),
			otlpGaugeDouble("gh_stats.repo.test_density", "Percentage of repository files categorized as automated tests", "%", repo.TestDensityPercent, timeNano, baseAttrs),
			otlpGaugeDouble("gh_stats.repo.top_author_share", "Commit share percentage of the most frequent contributor", "%", repo.TopAuthorPercentage, timeNano, baseAttrs),
		)
		if repo.StarsCount > 0 {
			metrics = append(metrics, otlpGaugeInt("gh_stats.repo.stars", "GitHub stargazers count", "stars", int64(repo.StarsCount), timeNano, baseAttrs))
		}
		if repo.OpenIssuesCount > 0 {
			metrics = append(metrics, otlpGaugeInt("gh_stats.repo.open_issues", "GitHub open issues count", "issues", int64(repo.OpenIssuesCount), timeNano, baseAttrs))
		}
	}

	return &OTLPMetricsPayload{
		ResourceMetrics: []OTLPResourceMetrics{
			{
				Resource: OTLPResource{
					Attributes: resourceAttrs,
				},
				ScopeMetrics: []OTLPScopeMetrics{
					{
						Scope: OTLPInstrumentationScope{
							Name:    "github.com/smford/gh-stats",
							Version: "1.0.0",
						},
						Metrics: metrics,
					},
				},
			},
		},
	}
}

// ParseOTLPHeaders parses key=value pairs separated by commas or newlines.
func ParseOTLPHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if strings.TrimSpace(headerStr) == "" {
		return headers
	}

	// Split by newlines or commas
	tokens := strings.FieldsFunc(headerStr, func(r rune) bool {
		return r == '\n' || r == ','
	})

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if k != "" {
				headers[k] = v
			}
		}
	}
	return headers
}

// ExportOTLP exports metrics to an OpenTelemetry OTLP/HTTP endpoint via JSON POST.
func ExportOTLP(ctx context.Context, payload *DORAMetricsPayload, endpoint string, headers map[string]string) error {
	otlpPayload := BuildOTLPMetricsPayload(payload)
	jsonData, err := json.Marshal(otlpPayload)
	if err != nil {
		return fmt.Errorf("failed to serialize OTLP metrics payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create OTLP HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gh-stats-otlp/1.0")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("OTLP request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("OTLP endpoint returned non-2xx status: %d %s (body: %s)", resp.StatusCode, resp.Status, strings.TrimSpace(string(bodySnippet)))
	}

	return nil
}
