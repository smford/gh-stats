package tui

import (
	"fmt"
	"io"
)

// Run starts the interactive TUI event loop, or gracefully falls back to plain text in non-interactive / CI environments.
func Run(model *DashboardModel, in io.Reader, out io.Writer) error {
	if model == nil {
		return fmt.Errorf("dashboard model cannot be nil")
	}

	// 1. Graceful fallback for non-interactive / CI terminal environments
	if !IsInteractive() {
		_, err := fmt.Fprint(out, RenderPlainText(model))
		return err
	}

	// 2. Interactive Terminal UI
	restoreTerminal, err := SetRawMode()
	if err != nil {
		// Fallback to plain text if terminal raw mode cannot be set
		_, fallbackErr := fmt.Fprint(out, RenderPlainText(model))
		return fallbackErr
	}
	defer restoreTerminal()

	// Switch to alternate screen and hide cursor
	fmt.Fprint(out, EnterAltScreen+HideCursor)
	defer fmt.Fprint(out, ExitAltScreen+ShowCursor)

	// Main event loop
	for {
		w, h := GetTerminalSize()
		model.Width = w
		model.Height = h

		// Render screen
		viewBuffer := Render(model)
		fmt.Fprint(out, CursorHome+ClearScreen+viewBuffer)

		// Read key input
		event, readErr := ReadKeyEvent(in)
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}

		if model.ShowHelp {
			if event.Type == KeyHelp || event.Type == KeyEsc || event.Type == KeyQuit || event.Type == KeyEnter {
				model.ShowHelp = false
			}
			continue
		}

		switch event.Type {
		case KeyQuit:
			return nil
		case KeyEsc:
			return nil
		case KeyTab:
			model.NextTab()
		case KeyBackTab, KeyLeft:
			model.PrevTab()
		case KeyRight:
			model.NextTab()
		case KeyUp:
			model.CursorUp()
		case KeyDown:
			model.CursorDown()
		case KeyPageUp:
			model.PageUp(10)
		case KeyPageDown:
			model.PageDown(10)
		case KeyHome:
			model.ScrollTop()
		case KeyEnd:
			model.ScrollBottom()
		case KeyHelp:
			model.ToggleHelp()
		case KeyRune:
			switch event.Rune {
			case '1':
				model.SetTab(TabOverview)
			case '2':
				model.SetTab(TabFiles)
			case '3':
				model.SetTab(TabCommits)
			case '4':
				model.SetTab(TabReviewers)
			case 'q', 'Q':
				return nil
			case '?':
				model.ToggleHelp()
			}
		}

		if model.Quit {
			break
		}
	}

	return nil
}
