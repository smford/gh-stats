package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/smford/gh-stats/pkg/analyzer"
	"github.com/smford/gh-stats/pkg/github"
	"github.com/smford/gh-stats/pkg/gitutil"
)

func TestNewPRDashboardModel(t *testing.T) {
	prStats := &analyzer.PRStats{
		BaseRef:        "main",
		HeadRef:        "feature/auth-overhaul",
		RiskScore:      70,
		RiskLevel:      "HIGH",
		TotalAdditions: 450,
		TotalDeletions: 60,
		NetChange:      390,
		FilesChanged:   8,
		CodeLinesAdded: 400,
		TestLinesAdded: 50,
		CommitCount:    3,
		SuggestedBump:  "minor",
		SuggestedVersion: "v0.5.0",
		SensitiveFiles: []analyzer.SensitiveMatch{
			{Path: ".github/workflows/deploy.yml", Category: "CI/CD Pipelines"},
		},
		TopChangedFiles: []gitutil.FileDiffStat{
			{Path: ".github/workflows/deploy.yml", Additions: 50, Deletions: 10, Status: "M"},
			{Path: "pkg/auth/token.go", Additions: 350, Deletions: 40, Status: "M"},
			{Path: "pkg/auth/token_test.go", Additions: 50, Deletions: 10, Status: "M"},
		},
		Commits: []gitutil.CommitInfo{
			{Hash: "abc1234", Author: "Alice", Subject: "feat: add token refresh", Date: "2026-09-28 12:00"},
			{Hash: "def5678", Author: "Bob", Subject: "fix: token expiry bug", Date: "2026-09-28 12:00"},
		},
		RecommendedReviewers: []analyzer.ReviewerRecommendation{
			{Author: "Charlie", CommitCount: 15, TopFiles: []string{"pkg/auth/token.go"}},
		},
		CIPipelineStats: &github.CIPipelineStats{
			TotalCheckRuns: 4,
			FlakyRuns: []github.FlakyCheck{
				{Name: "integration-test", IsFlaky: true},
			},
		},
		RiskBudget: &analyzer.RiskBudgetStats{
			Enabled:            true,
			MonthlyRiskPoints:  500,
			WindowDays:         30,
			UtilizationPercent: 85.0,
			BurnRate:           1.25,
			Status:             "ELEVATED",
		},
	}

	model := NewPRDashboardModel(prStats)

	if model.Target != "pr" {
		t.Errorf("expected target 'pr', got '%s'", model.Target)
	}
	if model.RiskScore != 70 || model.RiskLevel != "HIGH" {
		t.Errorf("expected score 70 HIGH, got %d %s", model.RiskScore, model.RiskLevel)
	}
	if len(model.Files) != 3 {
		t.Errorf("expected 3 files, got %d", len(model.Files))
	}
	if len(model.Commits) != 2 {
		t.Errorf("expected 2 commits, got %d", len(model.Commits))
	}
	if len(model.Reviewers) != 1 {
		t.Errorf("expected 1 reviewer, got %d", len(model.Reviewers))
	}
	if model.FlakyChecksCount != 1 {
		t.Errorf("expected 1 flaky check, got %d", model.FlakyChecksCount)
	}
	if model.RiskBudgetUtil != 85.0 {
		t.Errorf("expected 85.0 util, got %.1f", model.RiskBudgetUtil)
	}
}

func TestNewReleaseDashboardModel(t *testing.T) {
	relStats := &analyzer.ReleaseStats{
		BaseRef:        "v0.4.0",
		HeadRef:        "v0.5.0",
		TotalCommits:   4,
		TotalAdditions: 600,
		TotalDeletions: 50,
		NetChange:      550,
		FilesChanged:   6,
		RiskScore:      45,
		RiskLevel:      "MEDIUM",
		TestRatio:      0.25,
		SuggestedBump:  "major",
		SuggestedVersion: "v1.0.0",
		BreakingChanges: []analyzer.BreakingChange{
			{CommitHash: "c0ffee1", Subject: "feat!: breaking API contract", Reason: "conventional_commit"},
		},
		TopChangedFiles: []gitutil.FileDiffStat{
			{Path: "migrations/001_schema.sql", Additions: 30, Deletions: 0, Status: "A"},
			{Path: "pkg/api/v2.go", Additions: 500, Deletions: 40, Status: "M"},
		},
		Commits: []gitutil.CommitInfo{
			{Hash: "c0ffee1", Author: "Dave", Subject: "feat!: breaking API contract", Date: "2026-09-28 12:00"},
		},
		Contributors: []analyzer.ContributorStat{
			{Name: "Dave", CommitCount: 3, Percentage: 75.0},
			{Name: "Eve", CommitCount: 1, Percentage: 25.0},
		},
	}

	model := NewReleaseDashboardModel(relStats)

	if model.Target != "release" {
		t.Errorf("expected target 'release', got '%s'", model.Target)
	}
	if model.RiskScore != 45 || model.RiskLevel != "MEDIUM" {
		t.Errorf("expected score 45 MEDIUM, got %d %s", model.RiskScore, model.RiskLevel)
	}
	if len(model.Factors) == 0 {
		t.Errorf("expected factors to be populated")
	}
	if len(model.Reviewers) != 2 {
		t.Errorf("expected 2 contributor reviewers, got %d", len(model.Reviewers))
	}
}

func TestDashboardModel_Navigation(t *testing.T) {
	model := &DashboardModel{
		Files: []FileItem{
			{Path: "file1.go"},
			{Path: "file2.go"},
			{Path: "file3.go"},
			{Path: "file4.go"},
			{Path: "file5.go"},
		},
		Commits: []CommitItem{
			{Hash: "111", Subject: "feat: first"},
			{Hash: "222", Subject: "fix: second"},
		},
		Reviewers: []ReviewerItem{
			{Author: "Alice"},
			{Author: "Bob"},
		},
		Height: 20,
	}

	// Tab switching
	if model.ActiveTab != TabOverview {
		t.Errorf("expected TabOverview initial, got %v", model.ActiveTab)
	}
	model.NextTab()
	if model.ActiveTab != TabFiles {
		t.Errorf("expected TabFiles, got %v", model.ActiveTab)
	}
	model.NextTab()
	if model.ActiveTab != TabCommits {
		t.Errorf("expected TabCommits, got %v", model.ActiveTab)
	}
	model.NextTab()
	if model.ActiveTab != TabReviewers {
		t.Errorf("expected TabReviewers, got %v", model.ActiveTab)
	}
	model.NextTab()
	if model.ActiveTab != TabOverview {
		t.Errorf("expected cycle back to TabOverview, got %v", model.ActiveTab)
	}
	model.PrevTab()
	if model.ActiveTab != TabReviewers {
		t.Errorf("expected TabReviewers on PrevTab, got %v", model.ActiveTab)
	}
	model.SetTab(TabFiles)
	if model.ActiveTab != TabFiles {
		t.Errorf("expected TabFiles on SetTab, got %v", model.ActiveTab)
	}

	// Cursor movement on files
	model.CursorDown()
	if model.SelectedFileIndex != 1 {
		t.Errorf("expected selected index 1, got %d", model.SelectedFileIndex)
	}
	model.CursorUp()
	if model.SelectedFileIndex != 0 {
		t.Errorf("expected selected index 0, got %d", model.SelectedFileIndex)
	}
	model.ScrollBottom()
	if model.SelectedFileIndex != 4 {
		t.Errorf("expected selected index 4, got %d", model.SelectedFileIndex)
	}
	model.ScrollTop()
	if model.SelectedFileIndex != 0 {
		t.Errorf("expected selected index 0, got %d", model.SelectedFileIndex)
	}

	// Help modal toggle
	if model.ShowHelp {
		t.Errorf("expected ShowHelp false initially")
	}
	model.ToggleHelp()
	if !model.ShowHelp {
		t.Errorf("expected ShowHelp true after toggle")
	}
	model.ToggleHelp()
	if model.ShowHelp {
		t.Errorf("expected ShowHelp false after second toggle")
	}
}

func TestRender_AllTabs(t *testing.T) {
	model := &DashboardModel{
		Target:           "pr",
		Title:            "Pull Request SRE Assessment",
		BaseRef:          "main",
		HeadRef:          "feature/tui",
		RiskScore:        65,
		RiskLevel:        "HIGH",
		LinesAdded:       350,
		LinesDeleted:     40,
		NetChange:        310,
		FilesChanged:     3,
		SuggestedBump:    "minor",
		SuggestedVersion: "v0.6.0",
		Factors: []ScoreFactor{
			{Name: "Baseline Score", Points: 10, Assessment: "Initial risk points"},
			{Name: "Change Size", Points: 20, Assessment: "350 lines changed"},
		},
		Files: []FileItem{
			{Path: "pkg/tui/view.go", Additions: 200, Deletions: 10, Category: "Source Code"},
			{Path: ".github/workflows/ci.yml", Additions: 50, Deletions: 0, Category: "CI/CD Pipelines", IsSensitive: true},
		},
		Commits: []CommitItem{
			{Hash: "abc1234", Author: "Alice", Subject: "feat: add tui dashboard", Category: "feat"},
		},
		Reviewers: []ReviewerItem{
			{Author: "Bob", CommitCount: 8, TopFiles: []string{"pkg/tui/view.go"}},
		},
		Width:  100,
		Height: 28,
	}

	// Tab 0: Overview
	model.SetTab(TabOverview)
	outOverview := Render(model)
	if !strings.Contains(outOverview, "SRE Risk Rating") || !strings.Contains(outOverview, "HIGH") {
		t.Errorf("expected SRE Risk Rating HIGH in overview: %s", outOverview)
	}
	if !strings.Contains(outOverview, "Recommended SemVer Bump:") || !strings.Contains(outOverview, "MINOR") {
		t.Errorf("expected semver bump in overview: %s", outOverview)
	}
	if !strings.Contains(outOverview, "Change Size") {
		t.Errorf("expected Change Size factor in overview: %s", outOverview)
	}

	// Tab 1: Files
	model.SetTab(TabFiles)
	outFiles := Render(model)
	if !strings.Contains(outFiles, "pkg/tui/view.go") {
		t.Errorf("expected file path in files tab: %s", outFiles)
	}
	if !strings.Contains(outFiles, "CI/CD Pipelines") {
		t.Errorf("expected CI/CD category in files tab: %s", outFiles)
	}
	if !strings.Contains(outFiles, "Selected File Details") {
		t.Errorf("expected selected file details pane: %s", outFiles)
	}

	// Tab 2: Commits
	model.SetTab(TabCommits)
	outCommits := Render(model)
	if !strings.Contains(outCommits, "feat: add tui dashboard") {
		t.Errorf("expected commit subject in commits tab: %s", outCommits)
	}
	if !strings.Contains(outCommits, "[FEAT]") {
		t.Errorf("expected [FEAT] badge in commits tab: %s", outCommits)
	}
	if !strings.Contains(outCommits, "Selected Commit Details") {
		t.Errorf("expected selected commit details pane: %s", outCommits)
	}

	// Tab 3: Reviewers
	model.SetTab(TabReviewers)
	outReviewers := Render(model)
	if !strings.Contains(outReviewers, "Bob") {
		t.Errorf("expected Bob in reviewers tab: %s", outReviewers)
	}
	if !strings.Contains(outReviewers, "8 commits") {
		t.Errorf("expected commit count in reviewers tab: %s", outReviewers)
	}

	// Help overlay
	model.ShowHelp = true
	outHelp := Render(model)
	if !strings.Contains(outHelp, "Terminal Dashboard Help") {
		t.Errorf("expected help header in help overlay: %s", outHelp)
	}
}

func TestRenderPlainText(t *testing.T) {
	model := &DashboardModel{
		Target:       "pr",
		Title:        "Pull Request SRE Assessment",
		RiskScore:    70,
		RiskLevel:    "HIGH",
		LinesAdded:   400,
		LinesDeleted: 20,
		NetChange:    380,
		FilesChanged: 2,
		Factors: []ScoreFactor{
			{Name: "Baseline Score", Points: 10, Assessment: "Initial baseline"},
		},
		Files: []FileItem{
			{Path: "pkg/core.go", Additions: 400, Deletions: 20},
		},
		Reviewers: []ReviewerItem{
			{Author: "Alice", CommitCount: 10},
		},
	}

	plain := RenderPlainText(model)

	if !strings.Contains(plain, "Risk Rating: HIGH (Score: 70/100)") {
		t.Errorf("expected risk score in plain text output: %s", plain)
	}
	if !strings.Contains(plain, "+400 / -20 (Net: +380)") {
		t.Errorf("expected changes in plain text output: %s", plain)
	}
	if !strings.Contains(plain, "pkg/core.go") {
		t.Errorf("expected file path in plain text output: %s", plain)
	}
	if !strings.Contains(plain, "Alice (10 commits)") {
		t.Errorf("expected reviewer in plain text output: %s", plain)
	}
}

func TestRun_NonInteractiveFallback(t *testing.T) {
	model := &DashboardModel{
		Target:       "pr",
		Title:        "PR SRE Assessment",
		RiskScore:    50,
		RiskLevel:    "MEDIUM",
		LinesAdded:   100,
		LinesDeleted: 10,
		NetChange:    90,
		FilesChanged: 1,
	}

	in := bytes.NewBuffer([]byte{})
	var out bytes.Buffer

	// When running in tests, IsInteractive() is false because stdin/stdout are not character devices or under CI.
	err := Run(model, in, &out)
	if err != nil {
		t.Fatalf("unexpected error in Run: %v", err)
	}

	outputStr := out.String()
	if !strings.Contains(outputStr, "PR SRE Assessment") || !strings.Contains(outputStr, "MEDIUM") {
		t.Errorf("expected plain text fallback output, got: %s", outputStr)
	}
}

func TestReadKeyEvent(t *testing.T) {
	cases := []struct {
		input    []byte
		expected KeyType
	}{
		{[]byte{'q'}, KeyQuit},
		{[]byte{'Q'}, KeyQuit},
		{[]byte{3}, KeyQuit}, // Ctrl+C
		{[]byte{9}, KeyTab},  // Tab
		{[]byte{10}, KeyEnter},
		{[]byte{'?'}, KeyHelp},
		{[]byte{27, '[', 'A'}, KeyUp},
		{[]byte{27, '[', 'B'}, KeyDown},
		{[]byte{27, '[', 'C'}, KeyRight},
		{[]byte{27, '[', 'D'}, KeyLeft},
		{[]byte{27, '[', '5', '~'}, KeyPageUp},
		{[]byte{27, '[', '6', '~'}, KeyPageDown},
		{[]byte{'j'}, KeyDown},
		{[]byte{'k'}, KeyUp},
	}

	for _, tc := range cases {
		r := bytes.NewReader(tc.input)
		event, err := ReadKeyEvent(r)
		if err != nil {
			t.Errorf("unexpected error reading %v: %v", tc.input, err)
		}
		if event.Type != tc.expected {
			t.Errorf("for input %v, expected %v, got %v", tc.input, tc.expected, event.Type)
		}
	}
}
