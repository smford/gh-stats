package gitutil

import (
	"regexp"
	"strings"
)

// CleanRefName cleans and normalizes a git branch or tag reference name.
func CleanRefName(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	ref = strings.TrimPrefix(ref, "refs/tags/")
	ref = strings.TrimPrefix(ref, "refs/pull/")
	return ref
}

// IsMainOrMaster checks if a ref represents the primary production branch.
func IsMainOrMaster(ref string) bool {
	cleaned := CleanRefName(ref)
	cleaned = strings.TrimPrefix(cleaned, "origin/")
	return cleaned == "main" || cleaned == "master"
}

// TruncateCommitHash returns a 7-character short SHA-1 hash.
func TruncateCommitHash(hash string) string {
	clean := strings.TrimSpace(hash)
	if len(clean) > 7 {
		return clean[:7]
	}
	return clean
}

// ExtractIssueReferences parses issue IDs (e.g. #123) from a commit subject.
func ExtractIssueReferences(subject string) []string {
	re := regexp.MustCompile(`#(\d+)`)
	matches := re.FindAllString(subject, -1)
	return matches
}
