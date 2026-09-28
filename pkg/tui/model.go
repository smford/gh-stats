package tui

import (
	"fmt"
	"strings"

	"github.com/smford/gh-stats/pkg/analyzer"
)

// Tab represents an active view tab in the TUI dashboard.
type Tab int

const (
	// TabOverview displays the SRE risk card, metrics, and score breakdown.
	TabOverview Tab = 0
	// TabFiles displays the scrollable list of modified files and blast radius.
	TabFiles Tab = 1
	// TabCommits displays the commit history and categorized conventional commits.
	TabCommits Tab = 2
	// TabReviewers displays recommended reviewers and domain experts.
	TabReviewers Tab = 3
)

// ScoreFactor represents a single risk calculation contributor.
type ScoreFactor struct {
	Name        string
	Points      int
	Assessment  string
	Severity    string // "info", "warning", "critical", "good"
}

// FileItem represents a modified file formatted for the TUI list.
type FileItem struct {
	Path        string
	Additions   int
	Deletions   int
	Category    string // e.g. "CI/CD Pipelines", "Database Migrations", "Code"
	IsSensitive bool
	IsTest      bool
	IsDoc       bool
}

// CommitItem represents a commit with conventional commit classification.
type CommitItem struct {
	Hash       string
	Author     string
	Date       string
	Subject    string
	Category   string // "feat", "fix", "perf", "docs", "chore", "refactor", "breaking", "other"
	IsBreaking bool
	Body       string
}

// ReviewerItem represents a domain expert recommended for review.
type ReviewerItem struct {
	Author      string
	CommitCount int
	TopFiles    []string
	Role        string
}

// DashboardModel holds the state and data for the interactive TUI dashboard.
type DashboardModel struct {
	Target               string // "pr" or "release"
	Title                string
	BaseRef              string
	HeadRef              string
	RiskScore            int
	RiskLevel            string // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	LinesAdded           int
	LinesDeleted         int
	NetChange            int
	FilesChanged         int
	TestRatio            float64
	CommitCount          int
	SuggestedBump        string
	SuggestedVersion     string
	LeadTime             string
	FlakyChecksCount     int
	RiskBudgetUtil       float64
	RiskBudgetBurn       float64
	RiskBudgetStatus     string
	Factors              []ScoreFactor
	Files                []FileItem
	Commits              []CommitItem
	Reviewers            []ReviewerItem
	ActiveTab            Tab
	SelectedFileIndex    int
	FileScrollOffset     int
	SelectedCommitIndex  int
	CommitScrollOffset   int
	SelectedReviewerIndex int
	Width                int
	Height               int
	ShowHelp             bool
	Quit                 bool
}

// NewPRDashboardModel builds a DashboardModel from PRStats.
func NewPRDashboardModel(stats *analyzer.PRStats) *DashboardModel {
	m := &DashboardModel{
		Target:           "pr",
		Title:            "Pull Request SRE Assessment",
		BaseRef:          stats.BaseRef,
		HeadRef:          stats.HeadRef,
		RiskScore:        stats.RiskScore,
		RiskLevel:        stats.RiskLevel,
		LinesAdded:       stats.TotalAdditions,
		LinesDeleted:     stats.TotalDeletions,
		NetChange:        stats.NetChange,
		FilesChanged:     stats.FilesChanged,
		CommitCount:      stats.CommitCount,
		SuggestedBump:    stats.SuggestedBump,
		SuggestedVersion: stats.SuggestedVersion,
		Width:            100,
		Height:           28,
	}

	if stats.CodeLinesAdded > 0 {
		m.TestRatio = float64(stats.TestLinesAdded) / float64(stats.CodeLinesAdded)
	}

	if stats.GitHubMeta != nil {
		days := int(stats.GitHubMeta.Age.Hours() / 24)
		hours := int(stats.GitHubMeta.Age.Hours()) % 24
		m.LeadTime = fmt.Sprintf("%dd %dh", days, hours)
	}

	if stats.CIPipelineStats != nil {
		for _, f := range stats.CIPipelineStats.FlakyRuns {
			if f.IsFlaky {
				m.FlakyChecksCount++
			}
		}
	}

	if stats.RiskBudget != nil && stats.RiskBudget.Enabled {
		m.RiskBudgetUtil = stats.RiskBudget.UtilizationPercent
		m.RiskBudgetBurn = stats.RiskBudget.BurnRate
		m.RiskBudgetStatus = stats.RiskBudget.Status
	}

	// 1. Score factors breakdown
	m.Factors = buildPRScoreFactors(stats)

	// 2. Files
	m.Files = buildPRFiles(stats)

	// 3. Commits
	m.Commits = buildPRCommits(stats)

	// 4. Reviewers
	for _, r := range stats.RecommendedReviewers {
		m.Reviewers = append(m.Reviewers, ReviewerItem{
			Author:      r.Author,
			CommitCount: r.CommitCount,
			TopFiles:    r.TopFiles,
			Role:        "Domain Component Expert",
		})
	}

	return m
}

// NewReleaseDashboardModel builds a DashboardModel from ReleaseStats.
func NewReleaseDashboardModel(stats *analyzer.ReleaseStats) *DashboardModel {
	m := &DashboardModel{
		Target:           "release",
		Title:            fmt.Sprintf("Release Delta Assessment (%s ... %s)", stats.BaseRef, stats.HeadRef),
		BaseRef:          stats.BaseRef,
		HeadRef:          stats.HeadRef,
		RiskScore:        stats.RiskScore,
		RiskLevel:        stats.RiskLevel,
		LinesAdded:       stats.TotalAdditions,
		LinesDeleted:     stats.TotalDeletions,
		NetChange:        stats.NetChange,
		FilesChanged:     stats.FilesChanged,
		TestRatio:        stats.TestRatio,
		CommitCount:      stats.TotalCommits,
		SuggestedBump:    stats.SuggestedBump,
		SuggestedVersion: stats.SuggestedVersion,
		Width:            100,
		Height:           28,
	}

	// Score factors breakdown
	m.Factors = buildReleaseScoreFactors(stats)

	// Files
	m.Files = buildReleaseFiles(stats)

	// Commits
	m.Commits = buildReleaseCommits(stats)

	// Contributors / Reviewers
	for _, c := range stats.Contributors {
		m.Reviewers = append(m.Reviewers, ReviewerItem{
			Author:      c.Name,
			CommitCount: c.CommitCount,
			Role:        fmt.Sprintf("%.1f%% of release commits", c.Percentage),
		})
	}

	return m
}

func buildPRScoreFactors(stats *analyzer.PRStats) []ScoreFactor {
	var factors []ScoreFactor
	factors = append(factors, ScoreFactor{
		Name:       "Baseline Score",
		Points:     10,
		Assessment: "Initial baseline risk points",
		Severity:   "info",
	})

	maxLines := 800
	if stats.Config != nil && stats.Config.Thresholds.MaxPRLines > 0 {
		maxLines = stats.Config.Thresholds.MaxPRLines
	}

	totalEffective := (stats.CodeLinesAdded + stats.CodeLinesDeleted) + (stats.TestLinesAdded + stats.TestLinesDeleted)
	switch {
	case totalEffective > maxLines*2:
		factors = append(factors, ScoreFactor{
			Name:       "Change Size",
			Points:     30,
			Assessment: fmt.Sprintf("%d lines changed (>%d double threshold)", totalEffective, maxLines*2),
			Severity:   "critical",
		})
	case totalEffective > maxLines:
		factors = append(factors, ScoreFactor{
			Name:       "Change Size",
			Points:     20,
			Assessment: fmt.Sprintf("%d lines changed (>%d threshold)", totalEffective, maxLines),
			Severity:   "warning",
		})
	case totalEffective > maxLines/2:
		factors = append(factors, ScoreFactor{
			Name:       "Change Size",
			Points:     10,
			Assessment: fmt.Sprintf("%d lines changed (moderate size)", totalEffective),
			Severity:   "info",
		})
	default:
		factors = append(factors, ScoreFactor{
			Name:       "Change Size",
			Points:     0,
			Assessment: fmt.Sprintf("%d lines changed (compact size)", totalEffective),
			Severity:   "good",
		})
	}

	if len(stats.SensitiveFiles) > 0 {
		points := len(stats.SensitiveFiles) * 25
		if points > 50 {
			points = 50
		}
		var cats []string
		for _, s := range stats.SensitiveFiles {
			cats = append(cats, s.Category)
		}
		factors = append(factors, ScoreFactor{
			Name:       "Sensitive Blast Radius",
			Points:     points,
			Assessment: fmt.Sprintf("%d sensitive file(s) touched: %s", len(stats.SensitiveFiles), strings.Join(cats, ", ")),
			Severity:   "critical",
		})
	}

	if stats.CodeLinesAdded > 80 && stats.TestLinesAdded == 0 {
		factors = append(factors, ScoreFactor{
			Name:       "Test Coverage Delta",
			Points:     25,
			Assessment: fmt.Sprintf("+%d code lines added with 0 automated tests", stats.CodeLinesAdded),
			Severity:   "warning",
		})
	} else if stats.CodeLinesAdded > 0 && stats.TestLinesAdded > 0 {
		ratio := float64(stats.TestLinesAdded) / float64(stats.CodeLinesAdded)
		if ratio < 0.2 {
			factors = append(factors, ScoreFactor{
				Name:       "Test Coverage Delta",
				Points:     10,
				Assessment: fmt.Sprintf("Low test-to-code ratio (%.1f%% < 20%%)", ratio*100),
				Severity:   "warning",
			})
		} else {
			factors = append(factors, ScoreFactor{
				Name:       "Test Coverage Delta",
				Points:     -10,
				Assessment: fmt.Sprintf("Strong test coverage delta (%.1f%%)", ratio*100),
				Severity:   "good",
			})
		}
	}

	if stats.CIPipelineStats != nil {
		flaky := 0
		for _, r := range stats.CIPipelineStats.FlakyRuns {
			if r.IsFlaky {
				flaky++
			}
		}
		if flaky > 0 {
			factors = append(factors, ScoreFactor{
				Name:       "CI Pipeline Flakiness",
				Points:     15,
				Assessment: fmt.Sprintf("%d flaky check run(s) detected", flaky),
				Severity:   "warning",
			})
		}
	}

	if stats.RiskBudget != nil && stats.RiskBudget.Enabled {
		if stats.RiskBudget.Status == "EXCEEDED" {
			factors = append(factors, ScoreFactor{
				Name:       "SRE Risk Budget",
				Points:     15,
				Assessment: fmt.Sprintf("Squad risk budget exceeded (%.1f%% util, %.2fx burn)", stats.RiskBudget.UtilizationPercent, stats.RiskBudget.BurnRate),
				Severity:   "critical",
			})
		}
	}

	return factors
}

func buildReleaseScoreFactors(stats *analyzer.ReleaseStats) []ScoreFactor {
	var factors []ScoreFactor
	factors = append(factors, ScoreFactor{
		Name:       "Baseline Score",
		Points:     10,
		Assessment: "Initial release baseline score",
		Severity:   "info",
	})

	if len(stats.BreakingChanges) > 0 {
		factors = append(factors, ScoreFactor{
			Name:       "Breaking Changes",
			Points:     35,
			Assessment: fmt.Sprintf("%d breaking change(s) or migrations detected", len(stats.BreakingChanges)),
			Severity:   "critical",
		})
	}

	if len(stats.SensitiveFiles) > 0 {
		factors = append(factors, ScoreFactor{
			Name:       "High Blast Radius",
			Points:     20,
			Assessment: fmt.Sprintf("%d sensitive infrastructure/CI/CD files", len(stats.SensitiveFiles)),
			Severity:   "warning",
		})
	}

	if stats.NetChange > 1000 {
		factors = append(factors, ScoreFactor{
			Name:       "Release Delta Volume",
			Points:     20,
			Assessment: fmt.Sprintf("Large release delta (+%d / -%d lines)", stats.TotalAdditions, stats.TotalDeletions),
			Severity:   "warning",
		})
	}

	return factors
}

func buildPRFiles(stats *analyzer.PRStats) []FileItem {
	var files []FileItem
	sensitiveMap := make(map[string]string)
	for _, s := range stats.SensitiveFiles {
		sensitiveMap[s.Path] = s.Category
	}

	for _, f := range stats.TopChangedFiles {
		cat := "Source Code"
		isSensitive := false
		if c, ok := sensitiveMap[f.Path]; ok {
			cat = c
			isSensitive = true
		} else if analyzer.IsTestFile(f.Path) {
			cat = "Automated Tests"
		} else if analyzer.IsDocumentationFile(f.Path) {
			cat = "Documentation"
		}

		files = append(files, FileItem{
			Path:        f.Path,
			Additions:   f.Additions,
			Deletions:   f.Deletions,
			Category:    cat,
			IsSensitive: isSensitive,
			IsTest:      analyzer.IsTestFile(f.Path),
			IsDoc:       analyzer.IsDocumentationFile(f.Path),
		})
	}
	return files
}

func buildReleaseFiles(stats *analyzer.ReleaseStats) []FileItem {
	var files []FileItem
	sensitiveMap := make(map[string]string)
	for _, s := range stats.SensitiveFiles {
		sensitiveMap[s.Path] = s.Category
	}

	for _, f := range stats.TopChangedFiles {
		cat := "Source Code"
		isSensitive := false
		if c, ok := sensitiveMap[f.Path]; ok {
			cat = c
			isSensitive = true
		} else if analyzer.IsTestFile(f.Path) {
			cat = "Automated Tests"
		} else if analyzer.IsDocumentationFile(f.Path) {
			cat = "Documentation"
		}

		files = append(files, FileItem{
			Path:        f.Path,
			Additions:   f.Additions,
			Deletions:   f.Deletions,
			Category:    cat,
			IsSensitive: isSensitive,
			IsTest:      analyzer.IsTestFile(f.Path),
			IsDoc:       analyzer.IsDocumentationFile(f.Path),
		})
	}
	return files
}

func buildPRCommits(stats *analyzer.PRStats) []CommitItem {
	var items []CommitItem
	for _, c := range stats.Commits {
		cat := classifyCommit(c.Subject)
		isBreaking := strings.Contains(c.Subject, "!:") || strings.Contains(c.Subject, "BREAKING CHANGE")
		if isBreaking {
			cat = "breaking"
		}
		items = append(items, CommitItem{
			Hash:       c.Hash,
			Author:     c.Author,
			Date:       c.Date,
			Subject:    c.Subject,
			Category:   cat,
			IsBreaking: isBreaking,
			Body:       c.Body,
		})
	}
	return items
}

func buildReleaseCommits(stats *analyzer.ReleaseStats) []CommitItem {
	var items []CommitItem
	for _, c := range stats.Commits {
		cat := classifyCommit(c.Subject)
		isBreaking := strings.Contains(c.Subject, "!:") || strings.Contains(c.Subject, "BREAKING CHANGE")
		if isBreaking {
			cat = "breaking"
		}
		items = append(items, CommitItem{
			Hash:       c.Hash,
			Author:     c.Author,
			Date:       c.Date,
			Subject:    c.Subject,
			Category:   cat,
			IsBreaking: isBreaking,
			Body:       c.Body,
		})
	}
	return items
}

func classifyCommit(subject string) string {
	s := strings.ToLower(subject)
	switch {
	case strings.HasPrefix(s, "feat"):
		return "feat"
	case strings.HasPrefix(s, "fix"):
		return "fix"
	case strings.HasPrefix(s, "perf"):
		return "perf"
	case strings.HasPrefix(s, "refactor"):
		return "refactor"
	case strings.HasPrefix(s, "docs"):
		return "docs"
	case strings.HasPrefix(s, "chore"):
		return "chore"
	case strings.HasPrefix(s, "test"):
		return "test"
	default:
		return "other"
	}
}

// NextTab cycles to the next tab.
func (m *DashboardModel) NextTab() {
	m.ActiveTab = (m.ActiveTab + 1) % 4
}

// PrevTab cycles to the previous tab.
func (m *DashboardModel) PrevTab() {
	if m.ActiveTab == 0 {
		m.ActiveTab = TabReviewers
	} else {
		m.ActiveTab--
	}
}

// SetTab sets the active tab.
func (m *DashboardModel) SetTab(t Tab) {
	if t >= TabOverview && t <= TabReviewers {
		m.ActiveTab = t
	}
}

// CursorDown moves selection down in the active tab's list.
func (m *DashboardModel) CursorDown() {
	switch m.ActiveTab {
	case TabFiles:
		if len(m.Files) == 0 {
			return
		}
		if m.SelectedFileIndex < len(m.Files)-1 {
			m.SelectedFileIndex++
			m.adjustFileScroll()
		}
	case TabCommits:
		if len(m.Commits) == 0 {
			return
		}
		if m.SelectedCommitIndex < len(m.Commits)-1 {
			m.SelectedCommitIndex++
			m.adjustCommitScroll()
		}
	case TabReviewers:
		if len(m.Reviewers) == 0 {
			return
		}
		if m.SelectedReviewerIndex < len(m.Reviewers)-1 {
			m.SelectedReviewerIndex++
		}
	}
}

// CursorUp moves selection up in the active tab's list.
func (m *DashboardModel) CursorUp() {
	switch m.ActiveTab {
	case TabFiles:
		if m.SelectedFileIndex > 0 {
			m.SelectedFileIndex--
			m.adjustFileScroll()
		}
	case TabCommits:
		if m.SelectedCommitIndex > 0 {
			m.SelectedCommitIndex--
			m.adjustCommitScroll()
		}
	case TabReviewers:
		if m.SelectedReviewerIndex > 0 {
			m.SelectedReviewerIndex--
		}
	}
}

// PageDown moves selection down by a page.
func (m *DashboardModel) PageDown(pageSize int) {
	if pageSize <= 0 {
		pageSize = 10
	}
	switch m.ActiveTab {
	case TabFiles:
		m.SelectedFileIndex += pageSize
		if m.SelectedFileIndex >= len(m.Files) {
			m.SelectedFileIndex = len(m.Files) - 1
		}
		if m.SelectedFileIndex < 0 {
			m.SelectedFileIndex = 0
		}
		m.adjustFileScroll()
	case TabCommits:
		m.SelectedCommitIndex += pageSize
		if m.SelectedCommitIndex >= len(m.Commits) {
			m.SelectedCommitIndex = len(m.Commits) - 1
		}
		if m.SelectedCommitIndex < 0 {
			m.SelectedCommitIndex = 0
		}
		m.adjustCommitScroll()
	}
}

// PageUp moves selection up by a page.
func (m *DashboardModel) PageUp(pageSize int) {
	if pageSize <= 0 {
		pageSize = 10
	}
	switch m.ActiveTab {
	case TabFiles:
		m.SelectedFileIndex -= pageSize
		if m.SelectedFileIndex < 0 {
			m.SelectedFileIndex = 0
		}
		m.adjustFileScroll()
	case TabCommits:
		m.SelectedCommitIndex -= pageSize
		if m.SelectedCommitIndex < 0 {
			m.SelectedCommitIndex = 0
		}
		m.adjustCommitScroll()
	}
}

// ScrollTop moves to the top of the active list.
func (m *DashboardModel) ScrollTop() {
	switch m.ActiveTab {
	case TabFiles:
		m.SelectedFileIndex = 0
		m.FileScrollOffset = 0
	case TabCommits:
		m.SelectedCommitIndex = 0
		m.CommitScrollOffset = 0
	case TabReviewers:
		m.SelectedReviewerIndex = 0
	}
}

// ScrollBottom moves to the end of the active list.
func (m *DashboardModel) ScrollBottom() {
	switch m.ActiveTab {
	case TabFiles:
		if len(m.Files) > 0 {
			m.SelectedFileIndex = len(m.Files) - 1
			m.adjustFileScroll()
		}
	case TabCommits:
		if len(m.Commits) > 0 {
			m.SelectedCommitIndex = len(m.Commits) - 1
			m.adjustCommitScroll()
		}
	case TabReviewers:
		if len(m.Reviewers) > 0 {
			m.SelectedReviewerIndex = len(m.Reviewers) - 1
		}
	}
}

// ToggleHelp toggles the help overlay.
func (m *DashboardModel) ToggleHelp() {
	m.ShowHelp = !m.ShowHelp
}

func (m *DashboardModel) adjustFileScroll() {
	pageSize := m.visibleListRows()
	if m.SelectedFileIndex < m.FileScrollOffset {
		m.FileScrollOffset = m.SelectedFileIndex
	} else if m.SelectedFileIndex >= m.FileScrollOffset+pageSize {
		m.FileScrollOffset = m.SelectedFileIndex - pageSize + 1
	}
}

func (m *DashboardModel) adjustCommitScroll() {
	pageSize := m.visibleListRows()
	if m.SelectedCommitIndex < m.CommitScrollOffset {
		m.CommitScrollOffset = m.SelectedCommitIndex
	} else if m.SelectedCommitIndex >= m.CommitScrollOffset+pageSize {
		m.CommitScrollOffset = m.SelectedCommitIndex - pageSize + 1
	}
}

func (m *DashboardModel) visibleListRows() int {
	rows := m.Height - 14
	if rows < 5 {
		rows = 8
	}
	return rows
}
