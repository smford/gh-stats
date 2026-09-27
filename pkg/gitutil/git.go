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
