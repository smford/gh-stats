package analyzer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Semantic Version bump type constants.
const (
	BumpMajor = "major"
	BumpMinor = "minor"
	BumpPatch = "patch"
	BumpNone  = "none"
)

var semverRegex = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(.*)$`)

// CalculateNextVersion computes the next version string based on currentTag and bump type.
// bump must be "major", "minor", "patch", or "none" (case-insensitive).
// If currentTag is empty, it returns default initial versions ("v1.0.0" for major, "v0.1.0" for minor, "v0.0.1" for patch).
func CalculateNextVersion(currentTag string, bump string) (string, error) {
	bumpLower := strings.ToLower(strings.TrimSpace(bump))
	if bumpLower == BumpNone {
		if currentTag != "" {
			return currentTag, nil
		}
		return "v0.1.0", nil
	}

	if currentTag == "" {
		switch bumpLower {
		case BumpMajor:
			return "v1.0.0", nil
		case BumpMinor:
			return "v0.1.0", nil
		case BumpPatch:
			return "v0.0.1", nil
		default:
			return "v0.1.0", nil
		}
	}

	trimmedTag := strings.TrimSpace(currentTag)
	matches := semverRegex.FindStringSubmatch(trimmedTag)
	if len(matches) < 4 {
		return "", fmt.Errorf("current tag %q is not a valid semantic version (expected vX.Y.Z)", currentTag)
	}

	hasV := strings.HasPrefix(trimmedTag, "v")

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return "", fmt.Errorf("invalid major version in %q: %w", currentTag, err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", fmt.Errorf("invalid minor version in %q: %w", currentTag, err)
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return "", fmt.Errorf("invalid patch version in %q: %w", currentTag, err)
	}

	switch bumpLower {
	case BumpMajor:
		major++
		minor = 0
		patch = 0
	case BumpMinor:
		minor++
		patch = 0
	case BumpPatch:
		patch++
	default:
		return "", fmt.Errorf("unrecognized bump type %q (expected 'major', 'minor', or 'patch')", bump)
	}

	vPrefix := ""
	if hasV {
		vPrefix = "v"
	}

	return fmt.Sprintf("%s%d.%d.%d", vPrefix, major, minor, patch), nil
}

// DetermineBump inspects release delta commits and files to deterministically recommend the next bump:
// - MAJOR: If any breaking changes (feat!:, fix!:, BREAKING CHANGE:) or database schema migrations (*.sql, migrations/) are detected.
// - MINOR: If new features (feat:, feat(...)) are present without breaking changes.
// - PATCH: If only fixes (fix:), performance (perf:), docs, or chores are present.
// - NONE: If no commits and no files changed.
func (s *ReleaseStats) DetermineBump() string {
	// 1. Check explicitly captured breaking changes (conventional commits or migrations)
	if len(s.BreakingChanges) > 0 {
		return BumpMajor
	}

	// Double-check sensitive files for database schema migrations
	for _, sf := range s.SensitiveFiles {
		if sf.Category == "Database Migrations" || isMigrationPath(sf.Path) {
			return BumpMajor
		}
	}

	// Double-check changed files list for migrations
	for _, f := range s.TopChangedFiles {
		if isMigrationPath(f.Path) {
			return BumpMajor
		}
	}

	// Double-check commits for breaking change indicators
	for _, c := range s.Commits {
		subjLower := strings.ToLower(c.Subject)
		bodyLower := strings.ToLower(c.Body)
		if strings.Contains(subjLower, "!:") ||
			strings.HasPrefix(subjLower, "breaking:") ||
			strings.Contains(subjLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking-change:") {
			return BumpMajor
		}
	}

	// 2. Check for features (without breaking changes)
	if len(s.CategorizedCommits.Features) > 0 {
		return BumpMinor
	}
	for _, c := range s.Commits {
		if isConventionalMatch(strings.ToLower(c.Subject), "feat") {
			return BumpMinor
		}
	}

	// 3. Check for empty changes
	if s.TotalCommits == 0 && s.FilesChanged == 0 {
		return BumpNone
	}

	// 4. Default to patch (fixes, perf, docs, chores, refactors, etc.)
	return BumpPatch
}

// DetermineBump inspects pull request commits and files to recommend the next bump.
func (s *PRStats) DetermineBump() string {
	for _, sf := range s.SensitiveFiles {
		if sf.Category == "Database Migrations" || isMigrationPath(sf.Path) {
			return BumpMajor
		}
	}
	for _, f := range s.TopChangedFiles {
		if isMigrationPath(f.Path) {
			return BumpMajor
		}
	}

	hasFeat := false
	hasOther := false

	for _, c := range s.Commits {
		subjLower := strings.ToLower(c.Subject)
		bodyLower := strings.ToLower(c.Body)
		if strings.Contains(subjLower, "!:") ||
			strings.HasPrefix(subjLower, "breaking:") ||
			strings.Contains(subjLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking change:") ||
			strings.Contains(bodyLower, "breaking-change:") {
			return BumpMajor
		}
		if isConventionalMatch(subjLower, "feat") {
			hasFeat = true
		} else {
			hasOther = true
		}
	}

	if hasFeat {
		return BumpMinor
	}
	if hasOther || len(s.TopChangedFiles) > 0 {
		return BumpPatch
	}
	return BumpNone
}

func isMigrationPath(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	return strings.HasSuffix(p, ".sql") ||
		strings.Contains(p, "migration") ||
		strings.Contains(p, "/migrations/")
}
