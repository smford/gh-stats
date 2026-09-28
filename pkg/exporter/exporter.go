package exporter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/smford/gh-stats/pkg/analyzer"
)

// DORAMetricsPayload models structured DORA and SRE observability metrics.
type DORAMetricsPayload struct {
	SchemaVersion string          `json:"schemaVersion"`
	Timestamp     time.Time       `json:"timestamp"`
	Target        string          `json:"target"` // "pr", "repo", "release", or "drift"
	Repository    string          `json:"repository"`
	Ref           string          `json:"ref,omitempty"`
	PR            *PRMetrics      `json:"pr,omitempty"`
	Repo          *RepoMetrics    `json:"repo,omitempty"`
	Release       *ReleaseMetrics `json:"release,omitempty"`
	Drift         *DriftMetrics   `json:"drift,omitempty"`
}

// DriftMetrics encapsulates environment drift, commits ahead/behind, unpromoted blast radius, and promotion risk.
type DriftMetrics struct {
	BaseRef              string   `json:"baseRef"`
	HeadRef              string   `json:"headRef"`
	RiskScore            int      `json:"riskScore"`
	RiskLevel            string   `json:"riskLevel"`
	CommitsAhead         int      `json:"commitsAhead"`
	CommitsBehind        int      `json:"commitsBehind"`
	TotalAdditions       int      `json:"totalAdditions"`
	TotalDeletions       int      `json:"totalDeletions"`
	NetChange            int      `json:"netChange"`
	FilesChanged         int      `json:"filesChanged"`
	BreakingChangesCount int      `json:"breakingChangesCount"`
	BreakingChanges      []string `json:"breakingChanges,omitempty"`
	SensitiveFilesCount  int      `json:"sensitiveFilesCount"`
	SensitiveCategories  []string `json:"sensitiveCategories"`
}

// ReleaseMetrics encapsulates release comparison delta, commit volume, breaking changes, and risk.
type ReleaseMetrics struct {
	BaseRef              string   `json:"baseRef"`
	HeadRef              string   `json:"headRef"`
	RiskScore            int      `json:"riskScore"`
	RiskLevel            string   `json:"riskLevel"`
	TotalCommits         int      `json:"totalCommits"`
	ContributorsCount    int      `json:"contributorsCount"`
	TotalAdditions       int      `json:"totalAdditions"`
	TotalDeletions       int      `json:"totalDeletions"`
	NetChange            int      `json:"netChange"`
	FilesChanged         int      `json:"filesChanged"`
	TestLinesAdded       int      `json:"testLinesAdded"`
	CodeLinesAdded       int      `json:"codeLinesAdded"`
	TestRatio            float64  `json:"testRatio"`
	BreakingChangesCount int      `json:"breakingChangesCount"`
	BreakingChanges      []string `json:"breakingChanges,omitempty"`
	SensitiveFilesCount  int      `json:"sensitiveFilesCount"`
	SensitiveCategories  []string `json:"sensitiveCategories"`
}

// PRMetrics encapsulates DORA change lead time, reliability risk, and size metrics.
type PRMetrics struct {
	BaseRef                string   `json:"baseRef"`
	HeadRef                string   `json:"headRef"`
	Number                 int      `json:"number,omitempty"`
	RiskScore              int      `json:"riskScore"`
	RiskLevel              string   `json:"riskLevel"`
	LeadTimeHours          float64  `json:"leadTimeHours,omitempty"`
	TimeToFirstReviewHours float64  `json:"timeToFirstReviewHours,omitempty"`
	TotalAdditions         int      `json:"totalAdditions"`
	TotalDeletions         int      `json:"totalDeletions"`
	NetChange              int      `json:"netChange"`
	FilesChanged           int      `json:"filesChanged"`
	TestLinesAdded         int      `json:"testLinesAdded"`
	CodeLinesAdded         int      `json:"codeLinesAdded"`
	TestRatio              float64  `json:"testRatio"`
	SensitiveFilesCount    int      `json:"sensitiveFilesCount"`
	SensitiveCategories    []string `json:"sensitiveCategories"`
	DiscussionCount        int                `json:"discussionCount,omitempty"`
	ApprovalsCount         int                `json:"approvalsCount,omitempty"`
	ReviewsCount           int                `json:"reviewsCount,omitempty"`
	CommitsCount           int                `json:"commitsCount"`
	CIPipeline             *CIPipelineMetrics `json:"ciPipeline,omitempty"`
	RiskBudget             *RiskBudgetMetrics `json:"riskBudget,omitempty"`
}

// RiskBudgetMetrics encapsulates SRE risk budget and SLO burn rate for telemetry export.
type RiskBudgetMetrics struct {
	MonthlyRiskPoints    int     `json:"monthlyRiskPoints"`
	WindowDays           int     `json:"windowDays"`
	MaxCriticalPRs       int     `json:"maxCriticalPRs"`
	HistoricalPoints     int     `json:"historicalPoints"`
	CurrentPRPoints      int     `json:"currentPRPoints"`
	TotalProjectedPoints int     `json:"totalProjectedPoints"`
	TotalCriticalPRs     int     `json:"totalCriticalPRs"`
	UtilizationPercent   float64 `json:"utilizationPercent"`
	BurnRate             float64 `json:"burnRate"`
	Status               string  `json:"status"`
}

// CIPipelineMetrics encapsulates CI build latency and test flakiness telemetry.
type CIPipelineMetrics struct {
	TotalCheckRuns            int     `json:"totalCheckRuns"`
	SuccessfulRuns            int     `json:"successfulRuns"`
	FailedRuns                int     `json:"failedRuns"`
	TimedOutRuns              int     `json:"timedOutRuns"`
	CancelledRuns             int     `json:"cancelledRuns"`
	TotalDurationSeconds      float64 `json:"totalDurationSeconds"`
	LongestRunDurationSeconds float64 `json:"longestRunDurationSeconds"`
	LongestRunName            string  `json:"longestRunName,omitempty"`
	FlakyRunsCount            int     `json:"flakyRunsCount"`
}

// RepoMetrics encapsulates architectural health, bus factor, and churn statistics.
type RepoMetrics struct {
	TotalFiles          int                   `json:"totalFiles"`
	TestFilesCount      int                   `json:"testFilesCount"`
	DocFilesCount       int                   `json:"docFilesCount"`
	TestDensityPercent  float64               `json:"testDensityPercent"`
	TopAuthorPercentage float64               `json:"topAuthorPercentage"`
	TopChurnHotspots    []analyzer.ChurnEntry `json:"topChurnHotspots"`
	StarsCount          int                   `json:"starsCount,omitempty"`
	ForksCount          int                   `json:"forksCount,omitempty"`
	OpenIssuesCount     int                   `json:"openIssuesCount,omitempty"`
}

// NewPRPayload constructs a DORAMetricsPayload from PRStats.
func NewPRPayload(stats *analyzer.PRStats, repoSlug string) *DORAMetricsPayload {
	var categories []string
	catSet := make(map[string]bool)
	for _, sf := range stats.SensitiveFiles {
		if !catSet[sf.Category] {
			catSet[sf.Category] = true
			categories = append(categories, sf.Category)
		}
	}

	testRatio := 0.0
	if stats.CodeLinesAdded > 0 {
		testRatio = float64(stats.TestLinesAdded) / float64(stats.CodeLinesAdded)
	}

	prMetrics := &PRMetrics{
		BaseRef:             stats.BaseRef,
		HeadRef:             stats.HeadRef,
		RiskScore:           stats.RiskScore,
		RiskLevel:           stats.RiskLevel,
		TotalAdditions:      stats.TotalAdditions,
		TotalDeletions:      stats.TotalDeletions,
		NetChange:           stats.NetChange,
		FilesChanged:        stats.FilesChanged,
		TestLinesAdded:      stats.TestLinesAdded,
		CodeLinesAdded:      stats.CodeLinesAdded,
		TestRatio:           testRatio,
		SensitiveFilesCount: len(stats.SensitiveFiles),
		SensitiveCategories: categories,
		CommitsCount:        stats.CommitCount,
	}

	if stats.GitHubMeta != nil {
		prMetrics.Number = stats.GitHubMeta.Number
		prMetrics.LeadTimeHours = stats.GitHubMeta.Age.Hours()
		if stats.GitHubMeta.TimeToFirstReview > 0 {
			prMetrics.TimeToFirstReviewHours = stats.GitHubMeta.TimeToFirstReview.Hours()
		}
		prMetrics.DiscussionCount = stats.GitHubMeta.TotalDiscussions
		prMetrics.ApprovalsCount = stats.GitHubMeta.ApprovalsCount
		prMetrics.ReviewsCount = stats.GitHubMeta.ReviewsCount
	}

	if stats.CIPipelineStats != nil && stats.CIPipelineStats.TotalCheckRuns > 0 {
		flakyCount := 0
		for _, f := range stats.CIPipelineStats.FlakyRuns {
			if f.IsFlaky {
				flakyCount++
			}
		}
		prMetrics.CIPipeline = &CIPipelineMetrics{
			TotalCheckRuns:            stats.CIPipelineStats.TotalCheckRuns,
			SuccessfulRuns:            stats.CIPipelineStats.SuccessfulRuns,
			FailedRuns:                stats.CIPipelineStats.FailedRuns,
			TimedOutRuns:              stats.CIPipelineStats.TimedOutRuns,
			CancelledRuns:             stats.CIPipelineStats.CancelledRuns,
			TotalDurationSeconds:      stats.CIPipelineStats.TotalDuration.Seconds(),
			LongestRunDurationSeconds: stats.CIPipelineStats.LongestRunDuration.Seconds(),
			LongestRunName:            stats.CIPipelineStats.LongestRunName,
			FlakyRunsCount:            flakyCount,
		}
	}

	if stats.RiskBudget != nil && stats.RiskBudget.Enabled {
		prMetrics.RiskBudget = &RiskBudgetMetrics{
			MonthlyRiskPoints:    stats.RiskBudget.MonthlyRiskPoints,
			WindowDays:           stats.RiskBudget.WindowDays,
			MaxCriticalPRs:       stats.RiskBudget.MaxCriticalPRs,
			HistoricalPoints:     stats.RiskBudget.HistoricalPoints,
			CurrentPRPoints:      stats.RiskBudget.CurrentPRPoints,
			TotalProjectedPoints: stats.RiskBudget.TotalProjectedPoints,
			TotalCriticalPRs:     stats.RiskBudget.TotalCriticalPRs,
			UtilizationPercent:   stats.RiskBudget.UtilizationPercent,
			BurnRate:             stats.RiskBudget.BurnRate,
			Status:               stats.RiskBudget.Status,
		}
	}

	return &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "pr",
		Repository:    repoSlug,
		Ref:           stats.HeadRef,
		PR:            prMetrics,
	}
}

// NewRepoPayload constructs a DORAMetricsPayload from RepoStats.
func NewRepoPayload(stats *analyzer.RepoStats, repoSlug string) *DORAMetricsPayload {
	hotspots := stats.ChurnHotspots
	if len(hotspots) > 10 {
		hotspots = hotspots[:10]
	}

	repoMetrics := &RepoMetrics{
		TotalFiles:          stats.TotalFiles,
		TestFilesCount:      stats.TestFilesCount,
		DocFilesCount:       stats.DocFilesCount,
		TestDensityPercent:  stats.TestFileRatio,
		TopAuthorPercentage: stats.TopAuthorPercentage,
		TopChurnHotspots:    hotspots,
	}

	if stats.GitHubMeta != nil {
		repoMetrics.StarsCount = stats.GitHubMeta.StargazersCount
		repoMetrics.ForksCount = stats.GitHubMeta.ForksCount
		repoMetrics.OpenIssuesCount = stats.GitHubMeta.OpenIssuesCount
	}

	return &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "repo",
		Repository:    repoSlug,
		Repo:          repoMetrics,
	}
}

// NewReleasePayload constructs a DORAMetricsPayload from ReleaseStats.
func NewReleasePayload(stats *analyzer.ReleaseStats, repoSlug string) *DORAMetricsPayload {
	var categories []string
	catSet := make(map[string]bool)
	for _, sf := range stats.SensitiveFiles {
		if !catSet[sf.Category] {
			catSet[sf.Category] = true
			categories = append(categories, sf.Category)
		}
	}

	var breakingList []string
	for _, bc := range stats.BreakingChanges {
		breakingList = append(breakingList, bc.Subject)
	}

	relMetrics := &ReleaseMetrics{
		BaseRef:              stats.BaseRef,
		HeadRef:              stats.HeadRef,
		RiskScore:            stats.RiskScore,
		RiskLevel:            stats.RiskLevel,
		TotalCommits:         stats.TotalCommits,
		ContributorsCount:    len(stats.Contributors),
		TotalAdditions:       stats.TotalAdditions,
		TotalDeletions:       stats.TotalDeletions,
		NetChange:            stats.NetChange,
		FilesChanged:         stats.FilesChanged,
		TestLinesAdded:       stats.TestLinesAdded,
		CodeLinesAdded:       stats.CodeLinesAdded,
		TestRatio:            stats.TestRatio,
		BreakingChangesCount: len(stats.BreakingChanges),
		BreakingChanges:      breakingList,
		SensitiveFilesCount:  len(stats.SensitiveFiles),
		SensitiveCategories:  categories,
	}

	return &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "release",
		Repository:    repoSlug,
		Ref:           stats.HeadRef,
		Release:       relMetrics,
	}
}

// NewDriftPayload constructs a DORAMetricsPayload from DriftStats.
func NewDriftPayload(stats *analyzer.DriftStats, repoSlug string) *DORAMetricsPayload {
	var categories []string
	catSet := make(map[string]bool)
	for _, sf := range stats.SensitiveFiles {
		if !catSet[sf.Category] {
			catSet[sf.Category] = true
			categories = append(categories, sf.Category)
		}
	}

	var breakingList []string
	for _, bc := range stats.BreakingChanges {
		breakingList = append(breakingList, bc.Subject)
	}

	driftMetrics := &DriftMetrics{
		BaseRef:              stats.BaseRef,
		HeadRef:              stats.HeadRef,
		RiskScore:            stats.RiskScore,
		RiskLevel:            stats.RiskLevel,
		CommitsAhead:         stats.CommitsAhead,
		CommitsBehind:        stats.CommitsBehind,
		TotalAdditions:       stats.TotalAdditions,
		TotalDeletions:       stats.TotalDeletions,
		NetChange:            stats.NetChange,
		FilesChanged:         stats.FilesChanged,
		BreakingChangesCount: len(stats.BreakingChanges),
		BreakingChanges:      breakingList,
		SensitiveFilesCount:  len(stats.SensitiveFiles),
		SensitiveCategories:  categories,
	}

	return &DORAMetricsPayload{
		SchemaVersion: "1.0.0",
		Timestamp:     time.Now().UTC(),
		Target:        "drift",
		Repository:    repoSlug,
		Ref:           stats.HeadRef,
		Drift:         driftMetrics,
	}
}


// ExportJSON serializes payload to a formatted JSON file.
func ExportJSON(payload *DORAMetricsPayload, filePath string) error {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize metrics JSON: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metrics JSON to %s: %w", filePath, err)
	}

	return nil
}

// ExportWebhook posts the JSON payload to an HTTP/HTTPS endpoint with optional HMAC signature.
func ExportWebhook(ctx context.Context, payload *DORAMetricsPayload, webhookURL, secret string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize metrics for webhook: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gh-stats-exporter/1.0")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(data)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request to %s failed: %w", webhookURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d %s", resp.StatusCode, resp.Status)
	}

	return nil
}
