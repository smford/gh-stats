package tui

import (
	"fmt"
	"strings"
)

// Render generates the complete ANSI terminal screen buffer for the dashboard.
func Render(m *DashboardModel) string {
	if m.ShowHelp {
		return renderHelpOverlay(m)
	}

	var sb strings.Builder

	// Top banner & tabs
	sb.WriteString(renderHeader(m))
	sb.WriteString(renderTabs(m))
	sb.WriteString("\n")

	// Active tab content
	switch m.ActiveTab {
	case TabOverview:
		sb.WriteString(renderOverviewTab(m))
	case TabFiles:
		sb.WriteString(renderFilesTab(m))
	case TabCommits:
		sb.WriteString(renderCommitsTab(m))
	case TabReviewers:
		sb.WriteString(renderReviewersTab(m))
	}

	// Bottom navigation bar
	sb.WriteString(renderFooter(m))

	return sb.String()
}

func renderHeader(m *DashboardModel) string {
	targetBadge := fmt.Sprintf("%s%s[%s]%s", BgBlue, BrightWhite, strings.ToUpper(m.Target), Reset)
	refSpan := fmt.Sprintf("%s (%s ➔ %s)", m.Title, m.BaseRef, m.HeadRef)
	if m.BaseRef == "" && m.HeadRef == "" {
		refSpan = m.Title
	}

	return fmt.Sprintf(" %s %s%s%s\n", targetBadge, Bold, refSpan, Reset)
}

func renderTabs(m *DashboardModel) string {
	tabDefs := []struct {
		tab   Tab
		key   string
		label string
	}{
		{TabOverview, "1", "SRE Risk Overview"},
		{TabFiles, "2", fmt.Sprintf("Files & Blast Radius (%d)", len(m.Files))},
		{TabCommits, "3", fmt.Sprintf("Commits & Changelog (%d)", len(m.Commits))},
		{TabReviewers, "4", fmt.Sprintf("Domain Reviewers (%d)", len(m.Reviewers))},
	}

	var pills []string
	for _, td := range tabDefs {
		if m.ActiveTab == td.tab {
			pills = append(pills, fmt.Sprintf("%s%s [%s: %s] %s", BgCyan, Black, td.key, td.label, Reset))
		} else {
			pills = append(pills, fmt.Sprintf("%s [%s: %s] %s", Dim, td.key, td.label, Reset))
		}
	}

	return " " + strings.Join(pills, " ") + "\n"
}

func renderOverviewTab(m *DashboardModel) string {
	var sb strings.Builder

	// 1. SRE Risk Card
	var badgeColor string
	switch m.RiskLevel {
	case "CRITICAL":
		badgeColor = BgRed + BrightWhite
	case "HIGH":
		badgeColor = BgYellow + Black
	case "MEDIUM":
		badgeColor = BgBlue + BrightWhite
	default:
		badgeColor = BgGreen + Black
	}

	meter := renderProgressBar(m.RiskScore, 100, 24)

	sb.WriteString(fmt.Sprintf(" %s┌────────────────────────────────────────────────────────────────────────────────────────┐%s\n", Gray, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s  SRE Risk Rating: %s %s %s   Score: %s%d / 100%s   Gauge: %s  %s│%s\n",
		Gray, Reset, badgeColor, m.RiskLevel, Reset, Bold, m.RiskScore, Reset, meter, Gray, Reset))

	if m.SuggestedBump != "" && m.SuggestedBump != "none" {
		bumpColor := BrightGreen
		if m.SuggestedBump == "major" {
			bumpColor = BrightRed
		} else if m.SuggestedBump == "minor" {
			bumpColor = BrightYellow
		}
		sb.WriteString(fmt.Sprintf(" %s│%s  Recommended SemVer Bump: %s%s%s (next: %s%s%s)%s%s│%s\n",
			Gray, Reset, bumpColor+Bold, strings.ToUpper(m.SuggestedBump), Reset, Bold, m.SuggestedVersion, Reset,
			strings.Repeat(" ", max(0, 48-len(m.SuggestedBump)-len(m.SuggestedVersion))), Gray, Reset))
	}

	sb.WriteString(fmt.Sprintf(" %s└────────────────────────────────────────────────────────────────────────────────────────┘%s\n\n", Gray, Reset))

	// 2. Metrics Grid
	sb.WriteString(fmt.Sprintf(" %s📊 Change & Velocity Metrics:%s\n", Bold, Reset))
	sb.WriteString(fmt.Sprintf("   • Lines Added / Deleted: %s+%d%s / %s-%d%s (Net: %s%+d%s)\n",
		BrightGreen, m.LinesAdded, Reset, BrightRed, m.LinesDeleted, Reset, Bold, m.NetChange, Reset))
	sb.WriteString(fmt.Sprintf("   • Files Modified: %s%d%s   Commits: %s%d%s   Test Delta Ratio: %s%.1f%%%s\n",
		Bold, m.FilesChanged, Reset, Bold, m.CommitCount, Reset, Bold, m.TestRatio*100, Reset))

	if m.LeadTime != "" || m.FlakyChecksCount > 0 || m.RiskBudgetUtil > 0 {
		var extra []string
		if m.LeadTime != "" {
			extra = append(extra, fmt.Sprintf("PR Lead Time: %s%s%s", Cyan, m.LeadTime, Reset))
		}
		if m.FlakyChecksCount > 0 {
			extra = append(extra, fmt.Sprintf("Flaky Checks: %s🚨 %d detected%s", BrightRed+Bold, m.FlakyChecksCount, Reset))
		}
		if m.RiskBudgetUtil > 0 {
			statColor := BrightGreen
			if m.RiskBudgetStatus == "EXCEEDED" {
				statColor = BrightRed + Bold
			} else if m.RiskBudgetStatus == "ELEVATED" {
				statColor = BrightYellow
			}
			extra = append(extra, fmt.Sprintf("Risk Budget: %s%.1f%% util (%.2fx burn, %s)%s",
				statColor, m.RiskBudgetUtil, m.RiskBudgetBurn, m.RiskBudgetStatus, Reset))
		}
		sb.WriteString("   • " + strings.Join(extra, "   ") + "\n")
	}

	// 3. SRE Risk Score Factor Breakdown Table
	sb.WriteString(fmt.Sprintf("\n %s🔍 SRE Risk Factor Breakdown:%s\n", Bold, Reset))
	sb.WriteString(fmt.Sprintf("   %s%-28s  %-10s  %-46s%s\n", Dim, "Factor", "Points", "Assessment Rationale", Reset))
	sb.WriteString(fmt.Sprintf("   %s────────────────────────────  ──────────  ──────────────────────────────────────────────%s\n", Gray, Reset))

	for _, f := range m.Factors {
		ptsStr := fmt.Sprintf("+%d", f.Points)
		ptsColor := Yellow
		if f.Points <= 0 {
			ptsStr = fmt.Sprintf("%d", f.Points)
			ptsColor = Green
		} else if f.Points >= 25 {
			ptsColor = BrightRed + Bold
		}

		sb.WriteString(fmt.Sprintf("   %-28s  %s%-10s%s  %-46s\n",
			f.Name, ptsColor, ptsStr, Reset, f.Assessment))
	}

	return sb.String()
}

func renderFilesTab(m *DashboardModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(" %sModified Files & Blast Radius Analysis:%s\n", Bold, Reset))
	sb.WriteString(fmt.Sprintf(" %s   %-4s  %-48s  %-12s  %-24s%s\n", Dim, "#", "File Path", "+ / -", "Blast Radius / Category", Reset))
	sb.WriteString(fmt.Sprintf(" %s──────────────────────────────────────────────────────────────────────────────────────────%s\n", Gray, Reset))

	if len(m.Files) == 0 {
		sb.WriteString(fmt.Sprintf("   %sNo modified files detected in diff.%s\n", Dim, Reset))
		return sb.String()
	}

	pageSize := m.visibleListRows()
	start := m.FileScrollOffset
	end := start + pageSize
	if end > len(m.Files) {
		end = len(m.Files)
	}

	if start > 0 {
		sb.WriteString(fmt.Sprintf("   %s▲ (%d more files above)%s\n", Cyan, start, Reset))
	}

	for i := start; i < end; i++ {
		f := m.Files[i]
		isSel := i == m.SelectedFileIndex

		cursor := "  "
		lineStyle := ""
		if isSel {
			cursor = "➔ "
			lineStyle = Inverse
		}

		catColor := Reset
		if f.IsSensitive {
			catColor = BrightRed + Bold
		} else if f.IsTest {
			catColor = Green
		} else if f.IsDoc {
			catColor = Blue
		}

		diffStr := fmt.Sprintf("+%d/-%d", f.Additions, f.Deletions)

		pathDisplay := f.Path
		if len(pathDisplay) > 46 {
			pathDisplay = "..." + pathDisplay[len(pathDisplay)-43:]
		}

		sb.WriteString(fmt.Sprintf(" %s%s%-3d  %-48s  %-12s  %s%-24s%s%s\n",
			cursor, lineStyle, i+1, pathDisplay, diffStr, catColor, f.Category, Reset, lineStyle))
		if isSel {
			sb.WriteString(Reset)
		}
	}

	if end < len(m.Files) {
		sb.WriteString(fmt.Sprintf("   %s▼ (%d more files below)%s\n", Cyan, len(m.Files)-end, Reset))
	}

	// Selected File Details Pane
	if m.SelectedFileIndex >= 0 && m.SelectedFileIndex < len(m.Files) {
		sel := m.Files[m.SelectedFileIndex]
		sb.WriteString(fmt.Sprintf("\n %s┌── Selected File Details ───────────────────────────────────────────────────────────────┐%s\n", Gray, Reset))
		sb.WriteString(fmt.Sprintf(" %s│%s Path: %s%s%s\n", Gray, Reset, Bold, sel.Path, Reset))
		blastAlert := "Standard Source Code"
		if sel.IsSensitive {
			blastAlert = fmt.Sprintf("%s🚨 HIGH BLAST RADIUS (%s)%s", BrightRed+Bold, sel.Category, Reset)
		} else if sel.IsTest {
			blastAlert = "Automated Test Delta"
		}
		sb.WriteString(fmt.Sprintf(" %s│%s Impact: %s   Changes: %s+%d%s lines added, %s-%d%s lines deleted\n",
			Gray, Reset, blastAlert, BrightGreen, sel.Additions, Reset, BrightRed, sel.Deletions, Reset))
		sb.WriteString(fmt.Sprintf(" %s└────────────────────────────────────────────────────────────────────────────────────────┘%s\n", Gray, Reset))
	}

	return sb.String()
}

func renderCommitsTab(m *DashboardModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(" %sCommit History & Conventional Changelog:%s\n", Bold, Reset))
	sb.WriteString(fmt.Sprintf(" %s   %-8s  %-14s  %-12s  %-50s%s\n", Dim, "Hash", "Author", "Type", "Subject", Reset))
	sb.WriteString(fmt.Sprintf(" %s──────────────────────────────────────────────────────────────────────────────────────────%s\n", Gray, Reset))

	if len(m.Commits) == 0 {
		sb.WriteString(fmt.Sprintf("   %sNo commits found in comparison range.%s\n", Dim, Reset))
		return sb.String()
	}

	pageSize := m.visibleListRows()
	start := m.CommitScrollOffset
	end := start + pageSize
	if end > len(m.Commits) {
		end = len(m.Commits)
	}

	if start > 0 {
		sb.WriteString(fmt.Sprintf("   %s▲ (%d more commits above)%s\n", Cyan, start, Reset))
	}

	for i := start; i < end; i++ {
		c := m.Commits[i]
		isSel := i == m.SelectedCommitIndex

		cursor := "  "
		lineStyle := ""
		if isSel {
			cursor = "➔ "
			lineStyle = Inverse
		}

		typeBadge := fmt.Sprintf("[%s]", strings.ToUpper(c.Category))
		badgeColor := Cyan
		switch c.Category {
		case "breaking":
			typeBadge = "[BREAKING]"
			badgeColor = BrightRed + Bold
		case "feat":
			badgeColor = BrightGreen
		case "fix":
			badgeColor = BrightYellow
		case "perf":
			badgeColor = Magenta
		}

		hashShort := c.Hash
		if len(hashShort) > 7 {
			hashShort = hashShort[:7]
		}

		subj := c.Subject
		if len(subj) > 48 {
			subj = subj[:45] + "..."
		}

		author := c.Author
		if len(author) > 13 {
			author = author[:12] + "…"
		}

		sb.WriteString(fmt.Sprintf(" %s%s%-8s  %-14s  %s%-12s%s  %-50s%s\n",
			cursor, lineStyle, hashShort, author, badgeColor, typeBadge, Reset, subj, lineStyle))
		if isSel {
			sb.WriteString(Reset)
		}
	}

	if end < len(m.Commits) {
		sb.WriteString(fmt.Sprintf("   %s▼ (%d more commits below)%s\n", Cyan, len(m.Commits)-end, Reset))
	}

	// Selected Commit Details Pane
	if m.SelectedCommitIndex >= 0 && m.SelectedCommitIndex < len(m.Commits) {
		sel := m.Commits[m.SelectedCommitIndex]
		sb.WriteString(fmt.Sprintf("\n %s┌── Selected Commit Details ─────────────────────────────────────────────────────────────┐%s\n", Gray, Reset))
		sb.WriteString(fmt.Sprintf(" %s│%s Commit: %s%s%s   Author: %s%s%s   Date: %s\n",
			Gray, Reset, Bold, sel.Hash, Reset, Cyan, sel.Author, Reset, sel.Date))
		sb.WriteString(fmt.Sprintf(" %s│%s Subject: %s%s%s\n", Gray, Reset, Bold, sel.Subject, Reset))
		if sel.Body != "" {
			bodyPreview := strings.TrimSpace(sel.Body)
			lines := strings.Split(bodyPreview, "\n")
			for j := 0; j < len(lines) && j < 2; j++ {
				sb.WriteString(fmt.Sprintf(" %s│%s %s%s%s\n", Gray, Reset, Dim, lines[j], Reset))
			}
		}
		sb.WriteString(fmt.Sprintf(" %s└────────────────────────────────────────────────────────────────────────────────────────┘%s\n", Gray, Reset))
	}

	return sb.String()
}

func renderReviewersTab(m *DashboardModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(" %sSuggested Reviewers & Component Domain Experts:%s\n", Bold, Reset))
	sb.WriteString(fmt.Sprintf(" %s   %-20s  %-16s  %-50s%s\n", Dim, "Reviewer", "Commit Velocity", "Expertise / Modified Components", Reset))
	sb.WriteString(fmt.Sprintf(" %s──────────────────────────────────────────────────────────────────────────────────────────%s\n", Gray, Reset))

	if len(m.Reviewers) == 0 {
		sb.WriteString(fmt.Sprintf("   %sNo domain expert recommendations available for this change set.%s\n", Dim, Reset))
		return sb.String()
	}

	for i, r := range m.Reviewers {
		filesStr := strings.Join(r.TopFiles, ", ")
		if len(filesStr) > 48 {
			filesStr = filesStr[:45] + "..."
		}
		if filesStr == "" {
			filesStr = r.Role
		}

		cursor := "  "
		lineStyle := ""
		if i == m.SelectedReviewerIndex {
			cursor = "➔ "
			lineStyle = Inverse
		}

		sb.WriteString(fmt.Sprintf(" %s%s%-20s  %s%d commits%s        %-50s%s\n",
			cursor, lineStyle, r.Author, BrightGreen, r.CommitCount, Reset, filesStr, lineStyle))
		if i == m.SelectedReviewerIndex {
			sb.WriteString(Reset)
		}
	}

	sb.WriteString(fmt.Sprintf("\n %s💡 SRE Review Routing Guidance:%s\n", Dim, Reset))
	sb.WriteString(fmt.Sprintf("   Domain experts are selected by analyzing the historical git commit log of the exact\n"))
	sb.WriteString(fmt.Sprintf("   files modified in this pull request to minimize review fatigue and identify true code owners.\n"))

	return sb.String()
}

func renderFooter(m *DashboardModel) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(" %s[1-4 / Tab] Switch Views   [↑/↓ / j/k] Navigate   [g/G] Top/Bottom   [?] Help   [q] Quit%s\n",
		Dim, Reset))
	return sb.String()
}

func renderHelpOverlay(m *DashboardModel) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n %s┌── gh-stats Terminal Dashboard Help ────────────────────────────────────────────────────┐%s\n", Cyan+Bold, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s                                                                                        %s│%s\n", Cyan, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s   %sNavigation & Controls:%s                                                               %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %s1, 2, 3, 4%s        : Jump directly to Tab (1:Overview, 2:Files, 3:Commits, 4:Team) %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %sTab / Shift-Tab%s   : Cycle forward / backward through tabs                         %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %sh / l / ← / →%s     : Switch left / right tabs                                      %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %sj / k / ↑ / ↓%s     : Move cursor down / up in active lists                         %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %sPgDown / PgUp%s     : Page down / page up scroll                                    %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %sg / G%s             : Jump to top / bottom of active list                           %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %s?%s                 : Toggle this Help overlay modal                                %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s     • %sq / Esc / Ctrl+C%s  : Exit interactive dashboard                                    %s│%s\n", Cyan, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s                                                                                        %s│%s\n", Cyan, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s│%s   Press %s[?]%s or %s[Esc]%s to return to dashboard...                                           %s│%s\n", Cyan, Reset, Bold, Reset, Bold, Reset, Cyan, Reset))
	sb.WriteString(fmt.Sprintf(" %s└────────────────────────────────────────────────────────────────────────────────────────┘%s\n", Cyan+Bold, Reset))
	return sb.String()
}

func renderProgressBar(current, total, width int) string {
	if total <= 0 {
		total = 100
	}
	ratio := float64(current) / float64(total)
	if ratio > 1.0 {
		ratio = 1.0
	}
	filled := int(ratio * float64(width))
	if filled > width {
		filled = width
	}
	unfilled := width - filled

	fillColor := BrightGreen
	if current >= 75 {
		fillColor = BrightRed
	} else if current >= 50 {
		fillColor = BrightYellow
	} else if current >= 25 {
		fillColor = BrightBlue
	}

	return fmt.Sprintf("[%s%s%s%s%s]",
		fillColor, strings.Repeat("█", filled), Reset+Dim, strings.Repeat("░", unfilled), Reset)
}

// RenderPlainText produces clean, non-ANSI text output for CI or non-interactive environments.
func RenderPlainText(m *DashboardModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("=== gh-stats SRE Assessment: %s ===\n", m.Title))
	sb.WriteString(fmt.Sprintf("Risk Rating: %s (Score: %d/100)\n", m.RiskLevel, m.RiskScore))
	if m.SuggestedBump != "" && m.SuggestedBump != "none" {
		sb.WriteString(fmt.Sprintf("Suggested Version: %s (%s bump)\n", m.SuggestedVersion, strings.ToUpper(m.SuggestedBump)))
	}
	sb.WriteString(fmt.Sprintf("Changes: +%d / -%d (Net: %+d) across %d files\n",
		m.LinesAdded, m.LinesDeleted, m.NetChange, m.FilesChanged))
	sb.WriteString(fmt.Sprintf("Commits: %d   Test Ratio: %.1f%%\n", m.CommitCount, m.TestRatio*100))

	if m.RiskBudgetUtil > 0 {
		sb.WriteString(fmt.Sprintf("Risk Budget: %.1f%% util (%.2fx burn, Status: %s)\n",
			m.RiskBudgetUtil, m.RiskBudgetBurn, m.RiskBudgetStatus))
	}

	sb.WriteString("\nRisk Score Breakdown:\n")
	for _, f := range m.Factors {
		sb.WriteString(fmt.Sprintf(" - %s: %+d pts (%s)\n", f.Name, f.Points, f.Assessment))
	}

	if len(m.Files) > 0 {
		sb.WriteString(fmt.Sprintf("\nModified Files (%d):\n", len(m.Files)))
		limit := 10
		if len(m.Files) < limit {
			limit = len(m.Files)
		}
		for i := 0; i < limit; i++ {
			f := m.Files[i]
			flag := ""
			if f.IsSensitive {
				flag = " [HIGH BLAST RADIUS]"
			}
			sb.WriteString(fmt.Sprintf(" - %s (+%d/-%d)%s\n", f.Path, f.Additions, f.Deletions, flag))
		}
		if len(m.Files) > limit {
			sb.WriteString(fmt.Sprintf("   ... and %d more files\n", len(m.Files)-limit))
		}
	}

	if len(m.Reviewers) > 0 {
		sb.WriteString("\nSuggested Reviewers:\n")
		for _, r := range m.Reviewers {
			sb.WriteString(fmt.Sprintf(" - %s (%d commits)\n", r.Author, r.CommitCount))
		}
	}

	return sb.String()
}
