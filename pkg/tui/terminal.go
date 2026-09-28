package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ANSI escape sequences
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Underline   = "\033[4m"
	Inverse     = "\033[7m"

	// Colors
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"

	// Bright Colors
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	// Backgrounds
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"

	// Screen controls
	ClearScreen   = "\033[2J"
	CursorHome    = "\033[H"
	HideCursor    = "\033[?25l"
	ShowCursor    = "\033[?25h"
	EnterAltScreen = "\033[?1049h"
	ExitAltScreen  = "\033[?1049l"
)

// KeyType identifies the parsed terminal key event.
type KeyType int

const (
	KeyRune KeyType = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
	KeyTab
	KeyBackTab
	KeyEnter
	KeyEsc
	KeyQuit
	KeyHelp
	KeyUnknown
)

// KeyEvent represents a user input event.
type KeyEvent struct {
	Type KeyType
	Rune rune
}

// IsTerminal returns true if the specified file is a terminal (character device).
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// IsInteractive checks if stdout and stdin are character devices and not running under CI.
func IsInteractive() bool {
	if !IsTerminal(os.Stdout) || !IsTerminal(os.Stdin) {
		return false
	}
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		return false
	}
	if term := os.Getenv("TERM"); term == "dumb" || term == "" {
		return false
	}
	return true
}

// GetTerminalSize returns the current terminal width and height in columns and rows.
func GetTerminalSize() (width, height int) {
	width, height = 100, 30 // defaults

	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		var r, c int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &r, &c); err == nil && r > 0 && c > 0 {
			return c, r
		}
	}

	if cols := os.Getenv("COLUMNS"); cols != "" {
		if c, err := strconv.Atoi(cols); err == nil && c > 0 {
			width = c
		}
	}
	if lines := os.Getenv("LINES"); lines != "" {
		if l, err := strconv.Atoi(lines); err == nil && l > 0 {
			height = l
		}
	}

	return width, height
}

// SetRawMode puts the terminal in raw mode using stty, returning a restore callback.
func SetRawMode() (func(), error) {
	// Save current state
	saveCmd := exec.Command("stty", "-g")
	saveCmd.Stdin = os.Stdin
	savedState, err := saveCmd.Output()
	if err != nil {
		return func() {}, err
	}
	stateStr := strings.TrimSpace(string(savedState))

	// Enable non-canonical, unbuffered input without echo, preserving output processing (onlcr)
	rawCmd := exec.Command("stty", "-echo", "-icanon", "min", "1")
	rawCmd.Stdin = os.Stdin
	if err := rawCmd.Run(); err != nil {
		// Fallback to cbreak
		cbCmd := exec.Command("stty", "cbreak", "-echo")
		cbCmd.Stdin = os.Stdin
		_ = cbCmd.Run()
	}

	restore := func() {
		restCmd := exec.Command("stty", stateStr)
		restCmd.Stdin = os.Stdin
		_ = restCmd.Run()
	}

	return restore, nil
}

// ReadKeyEvent reads bytes from an io.Reader and decodes ANSI escape sequences into KeyEvents.
func ReadKeyEvent(r io.Reader) (KeyEvent, error) {
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if err != nil {
		return KeyEvent{Type: KeyUnknown}, err
	}

	if n == 0 {
		return KeyEvent{Type: KeyUnknown}, nil
	}

	// 1. Single character controls
	switch buf[0] {
	case 3: // Ctrl+C
		return KeyEvent{Type: KeyQuit}, nil
	case 4: // Ctrl+D
		return KeyEvent{Type: KeyQuit}, nil
	case 13, 10: // Enter
		return KeyEvent{Type: KeyEnter}, nil
	case 9: // Tab
		return KeyEvent{Type: KeyTab}, nil
	case 27: // Escape or Escape Sequence
		if n == 1 {
			return KeyEvent{Type: KeyEsc}, nil
		}
		// Multi-byte sequence
		if n >= 3 && buf[1] == '[' {
			switch buf[2] {
			case 'A':
				return KeyEvent{Type: KeyUp}, nil
			case 'B':
				return KeyEvent{Type: KeyDown}, nil
			case 'C':
				return KeyEvent{Type: KeyRight}, nil
			case 'D':
				return KeyEvent{Type: KeyLeft}, nil
			case 'H':
				return KeyEvent{Type: KeyHome}, nil
			case 'F':
				return KeyEvent{Type: KeyEnd}, nil
			case 'Z':
				return KeyEvent{Type: KeyBackTab}, nil
			case '5':
				if n >= 4 && buf[3] == '~' {
					return KeyEvent{Type: KeyPageUp}, nil
				}
			case '6':
				if n >= 4 && buf[3] == '~' {
					return KeyEvent{Type: KeyPageDown}, nil
				}
			case '1', '7':
				if n >= 4 && buf[3] == '~' {
					return KeyEvent{Type: KeyHome}, nil
				}
			case '4', '8':
				if n >= 4 && buf[3] == '~' {
					return KeyEvent{Type: KeyEnd}, nil
				}
			}
		}
		return KeyEvent{Type: KeyEsc}, nil
	case 'q', 'Q':
		return KeyEvent{Type: KeyQuit, Rune: rune(buf[0])}, nil
	case '?':
		return KeyEvent{Type: KeyHelp, Rune: '?'}, nil
	case 'j', 'J':
		return KeyEvent{Type: KeyDown, Rune: 'j'}, nil
	case 'k', 'K':
		return KeyEvent{Type: KeyUp, Rune: 'k'}, nil
	case 'h', 'H':
		return KeyEvent{Type: KeyLeft, Rune: 'h'}, nil
	case 'l', 'L':
		return KeyEvent{Type: KeyRight, Rune: 'l'}, nil
	case ' ':
		return KeyEvent{Type: KeyPageDown, Rune: ' '}, nil
	case 'g':
		return KeyEvent{Type: KeyHome, Rune: 'g'}, nil
	case 'G':
		return KeyEvent{Type: KeyEnd, Rune: 'G'}, nil
	default:
		return KeyEvent{Type: KeyRune, Rune: rune(buf[0])}, nil
	}
}
