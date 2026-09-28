package analyzer

import (
	"math"
	"time"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/gitutil"
)

// RiskBudgetStats contains calculated SRE risk budget and SLO burn rate telemetry.
type RiskBudgetStats struct {
	Enabled               bool               `json:"enabled"`
	MonthlyRiskPoints     int                `json:"monthlyRiskPoints"`
	WindowDays            int                `json:"windowDays"`
	MaxCriticalPRs        int                `json:"maxCriticalPRs"`
	BurnRateAlertRatio    float64            `json:"burnRateAlertRatio"`
	HistoricalPoints      int                `json:"historicalPoints"`
	HistoricalPRCount     int                `json:"historicalPRCount"`
	HistoricalCriticalPRs int                `json:"historicalCriticalPRs"`
	CurrentPRPoints       int                `json:"currentPRPoints"`
	IsCurrentPRCritical   bool               `json:"isCurrentPRCritical"`
	TotalProjectedPoints  int                `json:"totalProjectedPoints"`
	TotalCriticalPRs      int                `json:"totalCriticalPRs"`
	UtilizationPercent    float64            `json:"utilizationPercent"`
	BurnRate              float64            `json:"burnRate"`
	Status                string             `json:"status"` // "HEALTHY", "ELEVATED", "EXCEEDED"
	OldestCommitDate      time.Time          `json:"oldestCommitDate"`
	DaysSpan              float64            `json:"daysSpan"`
	MergedPRs             []MergedPRRiskInfo `json:"mergedPRs,omitempty"`
}

// MergedPRRiskInfo captures risk metadata for a historical merged commit on the base branch.
type MergedPRRiskInfo struct {
	Hash       string    `json:"hash"`
	Subject    string    `json:"subject"`
	Author     string    `json:"author"`
	MergedAt   time.Time `json:"mergedAt"`
	Additions  int       `json:"additions"`
	Deletions  int       `json:"deletions"`
	FilesCount int       `json:"filesCount"`
	RiskScore  int       `json:"riskScore"`
	RiskLevel  string    `json:"riskLevel"`
}

// CalculateHistoricalCommitRisk estimates the SRE risk score of a historical merged commit.
func CalculateHistoricalCommitRisk(entry gitutil.MergedCommitHistoryEntry, cfg *config.Config) (int, string, int) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	score := 10 // baseline
	maxLines := 800
	minTestRatio := 0.2
	if cfg.Thresholds.MaxPRLines > 0 {
		maxLines = cfg.Thresholds.MaxPRLines
	}
	if cfg.Thresholds.MinTestRatio > 0 {
		minTestRatio = cfg.Thresholds.MinTestRatio
	}

	patterns := SensitivePatternsWithConfig(cfg)
	sensitiveCount := 0

	var codeAdded, codeDeleted int
	var testAdded, testDeleted int
	var genAdded, genDeleted int

	for _, d := range entry.DiffStats {
		if cfg.IsIgnored(d.Path) {
			continue
		}

		isTest := IsTestFile(d.Path)
		isDoc := IsDocumentationFile(d.Path)
		isGen := IsGeneratedFile(d.Path)

		if isGen {
			genAdded += d.Additions
			genDeleted += d.Deletions
		} else if isTest {
			testAdded += d.Additions
			testDeleted += d.Deletions
		} else if !isDoc {
			codeAdded += d.Additions
			codeDeleted += d.Deletions
		}

		for _, p := range patterns {
			if p.Match(d.Path) {
				sensitiveCount++
				break
			}
		}
	}

	effectiveLines := (codeAdded + codeDeleted) + (testAdded + testDeleted) + (genAdded+genDeleted)/10

	// 1. Size penalty
	switch {
	case effectiveLines > maxLines*2:
		score += 35
	case effectiveLines > maxLines:
		score += 25
	case effectiveLines > maxLines/2:
		score += 15
	case effectiveLines > maxLines/5:
		score += 5
	}

	// 2. Blast radius penalty
	if sensitiveCount > 0 {
		penalty := sensitiveCount * 10
		if penalty > 30 {
			penalty = 30
		}
		score += penalty
	}

	// 3. Test coverage penalty
	if codeAdded > 80 && testAdded == 0 {
		score += 25
	} else if codeAdded > 0 && testAdded > 0 {
		ratio := float64(testAdded) / float64(codeAdded)
		if ratio < minTestRatio {
			score += 10
		} else if ratio >= minTestRatio*2.5 {
			score -= 10
		}
	}

	if score > 100 {
		score = 100
	}
	if score < 10 {
		score = 10
	}

	var level string
	switch {
	case score >= 75:
		level = "CRITICAL"
	case score >= 50:
		level = "HIGH"
	case score >= 25:
		level = "MEDIUM"
	default:
		level = "LOW"
	}

	return score, level, sensitiveCount
}

// EvaluateRiskBudget computes the rolling risk budget consumption and SLO burn rate.
func EvaluateRiskBudget(runner *gitutil.Runner, baseRef string, currentPRRiskScore int, currentPRRiskLevel string, cfg *config.Config, now time.Time) *RiskBudgetStats {
	if cfg == nil || !cfg.RiskBudget.IsEnabled() {
		return nil
	}

	if now.IsZero() {
		now = time.Now()
	}

	windowDays := cfg.RiskBudget.WindowDays
	if windowDays <= 0 {
		windowDays = 30
	}

	alertRatio := cfg.RiskBudget.BurnRateAlertRatio
	if alertRatio <= 0 {
		alertRatio = 1.0
	}

	since := now.AddDate(0, 0, -windowDays)

	var entries []gitutil.MergedCommitHistoryEntry
	if runner != nil {
		if history, err := runner.GetMergedPRHistory(baseRef, since); err == nil {
			entries = history
		}
	}

	stats := &RiskBudgetStats{
		Enabled:            true,
		MonthlyRiskPoints:  cfg.RiskBudget.MonthlyRiskPoints,
		WindowDays:         windowDays,
		MaxCriticalPRs:     cfg.RiskBudget.MaxCriticalPRs,
		BurnRateAlertRatio: alertRatio,
		CurrentPRPoints:    currentPRRiskScore,
		MergedPRs:          make([]MergedPRRiskInfo, 0, len(entries)),
	}

	if currentPRRiskLevel == "CRITICAL" || currentPRRiskScore >= 75 {
		stats.IsCurrentPRCritical = true
	}

	var oldestDate time.Time

	for _, entry := range entries {
		// Don't evaluate commits outside the time window
		if entry.Timestamp.Before(since) {
			continue
		}

		score, level, _ := CalculateHistoricalCommitRisk(entry, cfg)
		stats.HistoricalPoints += score
		stats.HistoricalPRCount++
		if level == "CRITICAL" {
			stats.HistoricalCriticalPRs++
		}

		if oldestDate.IsZero() || entry.Timestamp.Before(oldestDate) {
			oldestDate = entry.Timestamp
		}

		var adds, dels int
		for _, d := range entry.DiffStats {
			adds += d.Additions
			dels += d.Deletions
		}

		stats.MergedPRs = append(stats.MergedPRs, MergedPRRiskInfo{
			Hash:       entry.Hash,
			Subject:    entry.Subject,
			Author:     entry.Author,
			MergedAt:   entry.Timestamp,
			Additions:  adds,
			Deletions:  dels,
			FilesCount: len(entry.DiffStats),
			RiskScore:  score,
			RiskLevel:  level,
		})
	}

	stats.OldestCommitDate = oldestDate
	stats.TotalProjectedPoints = stats.HistoricalPoints + currentPRRiskScore
	stats.TotalCriticalPRs = stats.HistoricalCriticalPRs
	if stats.IsCurrentPRCritical {
		stats.TotalCriticalPRs++
	}

	// Calculate days span for burn rate
	daysSpan := float64(windowDays)
	if !oldestDate.IsZero() {
		calculatedSpan := now.Sub(oldestDate).Hours() / 24.0
		if calculatedSpan < 1.0 {
			calculatedSpan = 1.0
		}
		if calculatedSpan < float64(windowDays) {
			daysSpan = calculatedSpan
		}
	}
	stats.DaysSpan = daysSpan

	// Utilization
	if stats.MonthlyRiskPoints > 0 {
		stats.UtilizationPercent = (float64(stats.TotalProjectedPoints) / float64(stats.MonthlyRiskPoints)) * 100.0
		stats.UtilizationPercent = math.Round(stats.UtilizationPercent*10) / 10
	}

	// Burn Rate
	if stats.MonthlyRiskPoints > 0 {
		sustainableDailyRate := float64(stats.MonthlyRiskPoints) / float64(windowDays)
		observedDailyRate := float64(stats.TotalProjectedPoints) / daysSpan
		rawBurnRate := observedDailyRate / sustainableDailyRate
		stats.BurnRate = math.Round(rawBurnRate*100) / 100
	}

	// Determine Status
	isExceeded := false
	if stats.MonthlyRiskPoints > 0 && stats.TotalProjectedPoints > stats.MonthlyRiskPoints {
		isExceeded = true
	}
	if stats.MaxCriticalPRs > 0 && stats.TotalCriticalPRs > stats.MaxCriticalPRs {
		isExceeded = true
	}

	if isExceeded {
		stats.Status = "EXCEEDED"
	} else if (stats.MonthlyRiskPoints > 0 && stats.BurnRate >= alertRatio) || stats.UtilizationPercent >= 80.0 {
		stats.Status = "ELEVATED"
	} else {
		stats.Status = "HEALTHY"
	}

	return stats
}
