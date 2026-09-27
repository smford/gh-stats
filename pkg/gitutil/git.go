package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Runner abstracts git commands execution for testability and safety.
type Runner struct {
	RepoDir string
	Timeout time.Duration
}

// NewRunner creates a new git command runner rooted at the repository top-level.
func NewRunner(repoDir string) *Runner {
	r := &Runner{
		RepoDir: repoDir,
		Timeout: 30 * time.Second,
	}

	// Resolve the top-level repo directory to ensure all relative paths are repo-relative
	if topLevel, err := r.Exec("rev-parse", "--show-toplevel"); err == nil && topLevel != "" {
		r.RepoDir = topLevel
	}

	return r
}

// Exec runs a git command with context and returns standard output trimmed.
func (r *Runner) Exec(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if r.RepoDir != "" {
		cmd.Dir = r.RepoDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// FileDiffStat represents line additions and deletions for a single file.
type FileDiffStat struct {
	Path      string
	Additions int
	Deletions int
	Status    string // Added (A), Modified (M), Deleted (D), etc.
}

// CommitInfo holds basic commit metadata.
type CommitInfo struct {
	Hash    string
	Author  string
	Subject string
	Date    string
	Body    string
}

// IsShallow returns true if the repository is a shallow clone.
func (r *Runner) IsShallow() bool {
	out, err := r.Exec("rev-parse", "--is-shallow-repository")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// GetDefaultBaseRef attempts to find a suitable base ref if none is provided.
func (r *Runner) GetDefaultBaseRef() string {
	candidates := []string{"origin/main", "origin/master", "main", "master", "HEAD~1"}
	for _, ref := range candidates {
		if _, err := r.Exec("rev-parse", "--verify", ref); err == nil {
			return ref
		}
	}
	return "HEAD~1"
}

// GetHooksDir returns the absolute path to the git hooks directory for this repository.
func (r *Runner) GetHooksDir() (string, error) {
	out, err := r.Exec("rev-parse", "--git-path", "hooks")
	if err != nil {
		return filepath.Join(r.RepoDir, ".git", "hooks"), nil
	}
	path := strings.TrimSpace(out)
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.RepoDir, path)
	}
	return filepath.Clean(path), nil
}

// GetDiffStats retrieves file addition and deletion stats between two references.
func (r *Runner) GetDiffStats(baseRef, headRef string) ([]FileDiffStat, error) {
	var out, nameStatusOut string
	var err error

	if baseRef == "staged" || baseRef == "--cached" {
		out, err = r.Exec("diff", "--numstat", "--cached")
		if err != nil {
			return nil, err
		}
		nameStatusOut, _ = r.Exec("diff", "--name-status", "--cached")
	} else {
		// Try three-dot diff first (merge-base), fallback to two-dot
		diffRange := fmt.Sprintf("%s...%s", baseRef, headRef)
		out, err = r.Exec("diff", "--numstat", diffRange)
		if err != nil {
			// Fallback to two-dot diff
			diffRange = fmt.Sprintf("%s..%s", baseRef, headRef)
			out, err = r.Exec("diff", "--numstat", diffRange)
			if err != nil {
				return nil, err
			}
		}
		nameStatusOut, _ = r.Exec("diff", "--name-status", diffRange)
	}

	statusMap := make(map[string]string)
	if nameStatusOut != "" {
		lines := strings.Split(nameStatusOut, "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				status := parts[0]
				path := parts[len(parts)-1] // handles renames
				statusMap[path] = status
			}
		}
	}

	var results []FileDiffStat
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		adds, _ := strconv.Atoi(parts[0]) // binary files display "-" which parses as 0
		dels, _ := strconv.Atoi(parts[1])
		path := parts[2]

		status := statusMap[path]
		if status == "" {
			status = "M"
		}

		results = append(results, FileDiffStat{
			Path:      path,
			Additions: adds,
			Deletions: dels,
			Status:    status,
		})
	}

	return results, nil
}

// GetCommits returns commits between baseRef and headRef.
func (r *Runner) GetCommits(baseRef, headRef string) ([]CommitInfo, error) {
	if baseRef == "staged" || baseRef == "--cached" {
		return nil, nil
	}

	diffRange := fmt.Sprintf("%s...%s", baseRef, headRef)
	out, err := r.Exec("log", "--format=%h|%an|%s", diffRange)
	if err != nil {
		diffRange = fmt.Sprintf("%s..%s", baseRef, headRef)
		out, err = r.Exec("log", "--format=%h|%an|%s", diffRange)
		if err != nil {
			return nil, err
		}
	}

	var commits []CommitInfo
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) == 3 {
			commits = append(commits, CommitInfo{
				Hash:    parts[0],
				Author:  parts[1],
				Subject: parts[2],
			})
		}
	}
	return commits, nil
}

// GetAheadBehind returns the number of commits headRef is ahead of baseRef (unpromoted commits)
// and behind baseRef (upstream divergence).
func (r *Runner) GetAheadBehind(baseRef, headRef string) (ahead, behind int, err error) {
	out, err := r.Exec("rev-list", "--left-right", "--count", fmt.Sprintf("%s...%s", baseRef, headRef))
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 2 {
		b, err1 := strconv.Atoi(parts[0])
		a, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			return a, b, nil
		}
	}
	return 0, 0, fmt.Errorf("unexpected rev-list --left-right --count output: %q", out)
}

// GetFileChurnFrequency returns the most frequently modified files over recent commits.
func (r *Runner) GetFileChurnFrequency(commitLimit int) (map[string]int, error) {
	out, err := r.Exec("log", fmt.Sprintf("-n%d", commitLimit), "--name-only", "--format=")
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		counts[line]++
	}
	return counts, nil
}

// GetAuthorStats returns author commit counts.
func (r *Runner) GetAuthorStats(commitLimit int) (map[string]int, error) {
	out, err := r.Exec("log", fmt.Sprintf("-n%d", commitLimit), "--format=%an")
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		counts[line]++
	}
	return counts, nil
}

// GetFileAuthors returns the author commit frequency for a specific file.
func (r *Runner) GetFileAuthors(filePath string, commitLimit int) (map[string]int, error) {
	if commitLimit <= 0 {
		commitLimit = 50
	}
	out, err := r.Exec("log", "--follow", fmt.Sprintf("-n%d", commitLimit), "--format=%an", "--", filePath)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		author := strings.TrimSpace(line)
		if author != "" {
			counts[author]++
		}
	}
	return counts, nil
}

// ListTrackedFiles returns all tracked files in the repo.
func (r *Runner) ListTrackedFiles() ([]string, error) {
	out, err := r.Exec("ls-files")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// GetReleaseTags returns all repository tags sorted by version descending.
func (r *Runner) GetReleaseTags() ([]string, error) {
	out, err := r.Exec("tag", "--sort=-v:refname")
	if err != nil || strings.TrimSpace(out) == "" {
		out, err = r.Exec("tag", "--sort=-creatordate")
		if err != nil {
			return nil, err
		}
	}

	var tags []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tags = append(tags, line)
		}
	}
	return tags, nil
}

// GetLatestTag returns the most recent SemVer release tag (e.g. "v0.4.0").
func (r *Runner) GetLatestTag() (string, error) {
	out, err := r.Exec("tag", "-l", "v*.*.*", "--sort=-v:refname")
	if err == nil && strings.TrimSpace(out) != "" {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && IsSemverTag(line) {
				return line, nil
			}
		}
	}

	// Fallback to GetReleaseTags
	tags, err := r.GetReleaseTags()
	if err != nil {
		return "", err
	}
	for _, t := range tags {
		if IsSemverTag(t) {
			return t, nil
		}
	}
	if len(tags) > 0 {
		return tags[0], nil
	}
	return "", fmt.Errorf("no release tags found in repository")
}

// IsSemverTag returns true if tag matches semantic versioning conventions (e.g. v1.2.3 or 1.2.3).
func IsSemverTag(tag string) bool {
	t := strings.TrimSpace(tag)
	if strings.HasPrefix(t, "v") {
		t = t[1:]
	}
	parts := strings.Split(t, ".")
	if len(parts) < 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		p := parts[i]
		if i == 2 {
			if idx := strings.IndexAny(p, "-+"); idx != -1 {
				p = p[:idx]
			}
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// GetInitialCommit returns the repository's root/first commit SHA.
func (r *Runner) GetInitialCommit() (string, error) {
	out, err := r.Exec("rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 0 && lines[0] != "" {
		return strings.TrimSpace(lines[0]), nil
	}
	return "", fmt.Errorf("could not determine initial commit")
}

// GetLatestTwoTags returns the latest tag (headTag) and the one directly preceding it (baseTag).
// If only one tag exists, baseTag is the initial commit hash.
func (r *Runner) GetLatestTwoTags() (baseTag, headTag string, err error) {
	tags, err := r.GetReleaseTags()
	if err != nil {
		return "", "", err
	}
	if len(tags) == 0 {
		return "", "", fmt.Errorf("no release tags found in repository")
	}
	headTag = tags[0]
	if len(tags) >= 2 {
		baseTag = tags[1]
		return baseTag, headTag, nil
	}

	// Only 1 tag exists; resolve initial commit as base
	initCommit, initErr := r.GetInitialCommit()
	if initErr == nil && initCommit != "" {
		baseTag = initCommit
	} else {
		baseTag = headTag + "~1"
	}
	return baseTag, headTag, nil
}

// GetPreviousTag finds the release tag immediately preceding headTag.
func (r *Runner) GetPreviousTag(headTag string) (string, error) {
	tags, err := r.GetReleaseTags()
	if err != nil {
		return "", err
	}
	for i, t := range tags {
		if t == headTag {
			if i+1 < len(tags) {
				return tags[i+1], nil
			}
			// headTag is the oldest tag; return initial commit
			initCommit, err := r.GetInitialCommit()
			if err == nil && initCommit != "" {
				return initCommit, nil
			}
			return headTag + "~1", nil
		}
	}
	if len(tags) > 0 {
		return tags[0], nil
	}
	return "", fmt.Errorf("tag %s not found and no previous tag available", headTag)
}

// GetReleaseCommits retrieves commits between baseRef and headRef with date and body.
func (r *Runner) GetReleaseCommits(baseRef, headRef string) ([]CommitInfo, error) {
	if baseRef == "staged" || baseRef == "--cached" {
		return nil, nil
	}

	var diffRange string
	if baseRef == "" || baseRef == "root" {
		diffRange = headRef
	} else {
		diffRange = fmt.Sprintf("%s..%s", baseRef, headRef)
	}

	out, err := r.Exec("log", "--format=%h\x1f%an\x1f%aI\x1f%s\x1f%b\x1e", diffRange)
	if err != nil {
		// Fallback to three-dot if two-dot failed
		if baseRef != "" && baseRef != "root" {
			diffRange = fmt.Sprintf("%s...%s", baseRef, headRef)
			out, err = r.Exec("log", "--format=%h\x1f%an\x1f%aI\x1f%s\x1f%b\x1e", diffRange)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	var commits []CommitInfo
	records := strings.Split(out, "\x1e")
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) >= 4 {
			c := CommitInfo{
				Hash:    strings.TrimSpace(parts[0]),
				Author:  strings.TrimSpace(parts[1]),
				Date:    strings.TrimSpace(parts[2]),
				Subject: strings.TrimSpace(parts[3]),
			}
			if len(parts) >= 5 {
				c.Body = strings.TrimSpace(parts[4])
			}
			commits = append(commits, c)
		}
	}
	return commits, nil
}

