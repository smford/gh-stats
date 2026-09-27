package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client handles interaction with the GitHub REST API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new GitHub API client.
func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.github.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// PRMetadata holds key PR lifecycle and discussion metrics from the API.
type PRMetadata struct {
	Number            int           `json:"number"`
	Title             string        `json:"title"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	MergedAt          *time.Time    `json:"merged_at"`
	AuthorAssociation string        `json:"author_association"`
	CommentsCount     int           `json:"comments"`
	ReviewComments    int           `json:"review_comments"`
	TotalDiscussions  int           `json:"-"`
	Age               time.Duration `json:"-"`
	TimeToFirstReview time.Duration `json:"-"`
	ReviewsCount      int           `json:"-"`
	ApprovalsCount    int           `json:"-"`
	Head              struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
}

// CheckRun represents an execution of a CI workflow or check.
type CheckRun struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	HeadSHA     string     `json:"head_sha"`
	Status      string     `json:"status"`      // "queued", "in_progress", "completed"
	Conclusion  string     `json:"conclusion"`  // "success", "failure", "timed_out", "cancelled", "neutral", "skipped"
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	HTMLURL     string     `json:"html_url"`
	DetailsURL  string     `json:"details_url"`
}

// Duration returns the runtime of the check run.
func (cr *CheckRun) Duration() time.Duration {
	if cr.StartedAt.IsZero() {
		return 0
	}
	if cr.CompletedAt != nil && !cr.CompletedAt.IsZero() {
		if cr.CompletedAt.After(cr.StartedAt) {
			return cr.CompletedAt.Sub(cr.StartedAt)
		}
		return 0
	}
	return 0
}

// CheckRunsResponse wraps the GitHub Check Runs list API response.
type CheckRunsResponse struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []CheckRun `json:"check_runs"`
}

// CIPipelineStats aggregates latency, bottlenecks, and flakiness across CI check runs.
type CIPipelineStats struct {
	TotalCheckRuns     int               `json:"totalCheckRuns"`
	SuccessfulRuns     int               `json:"successfulRuns"`
	FailedRuns         int               `json:"failedRuns"`
	TimedOutRuns       int               `json:"timedOutRuns"`
	CancelledRuns      int               `json:"cancelledRuns"`
	InProgressRuns     int               `json:"inProgressRuns"`
	TotalDuration      time.Duration     `json:"totalDuration"`
	LongestRunDuration time.Duration     `json:"longestRunDuration"`
	LongestRunName     string            `json:"longestRunName,omitempty"`
	AverageDuration    time.Duration     `json:"averageDuration"`
	FlakyRuns          []FlakyCheck      `json:"flakyRuns,omitempty"`
	BottleneckRuns     []CheckRunSummary `json:"bottleneckRuns,omitempty"`
	CheckRuns          []CheckRun        `json:"checkRuns,omitempty"`
}

// FlakyCheck details a test or job that exhibited retries or conflicting results on the same commit.
type FlakyCheck struct {
	Name           string   `json:"name"`
	RetryCount     int      `json:"retryCount"`
	InitialResult  string   `json:"initialResult"`
	FinalResult    string   `json:"finalResult"`
	IsFlaky        bool     `json:"isFlaky"`
	ObservedStates []string `json:"observedStates"`
}

// CheckRunSummary summarizes a bottleneck or critical check run.
type CheckRunSummary struct {
	Name       string        `json:"name"`
	Duration   time.Duration `json:"duration"`
	Status     string        `json:"status"`
	Conclusion string        `json:"conclusion"`
	HTMLURL    string        `json:"htmlUrl,omitempty"`
}

// Review represents a PR review.
type Review struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED
	SubmittedAt time.Time `json:"submitted_at"`
}

// RepoMetadata holds repository metrics from the GitHub API.
type RepoMetadata struct {
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	OpenIssuesCount int       `json:"open_issues_count"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	WatchersCount   int       `json:"watchers_count"`
	SizeKB          int       `json:"size"`
	DefaultBranch   string    `json:"default_branch"`
	CreatedAt       time.Time `json:"created_at"`
	PushedAt        time.Time `json:"pushed_at"`
}

// CommitActivityWeek represents 1 week of commit activity.
type CommitActivityWeek struct {
	Days  []int `json:"days"`
	Total int   `json:"total"`
	Week  int64 `json:"week"`
}

// DetectRepoSlug finds "owner/repo" from env or git remote.
func DetectRepoSlug(defaultRemote string) (string, error) {
	if repo := os.Getenv("GITHUB_REPOSITORY"); repo != "" {
		return repo, nil
	}
	// Try parsing from git remote string, e.g., git@github.com:owner/repo.git or https://github.com/owner/repo.git
	re := regexp.MustCompile(`(?:github\.com[:/])([^/]+)/([^/.]+)(?:\.git)?`)
	matches := re.FindStringSubmatch(defaultRemote)
	if len(matches) == 3 {
		return fmt.Sprintf("%s/%s", matches[1], matches[2]), nil
	}
	return "", fmt.Errorf("unable to detect GitHub repository slug")
}

// DetectPRNumber attempts to extract the PR number from GitHub event context.
func DetectPRNumber() int {
	// Check GITHUB_REF: refs/pull/123/merge
	ref := os.Getenv("GITHUB_REF")
	if strings.HasPrefix(ref, "refs/pull/") {
		parts := strings.Split(ref, "/")
		if len(parts) >= 3 {
			if n, err := strconv.Atoi(parts[2]); err == nil {
				return n
			}
		}
	}

	// Check GITHUB_EVENT_PATH json file
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath != "" {
		data, err := os.ReadFile(eventPath)
		if err == nil {
			var evt struct {
				Number      int `json:"number"`
				PullRequest struct {
					Number int `json:"number"`
				} `json:"pull_request"`
			}
			if err := json.Unmarshal(data, &evt); err == nil {
				if evt.PullRequest.Number > 0 {
					return evt.PullRequest.Number
				}
				if evt.Number > 0 {
					return evt.Number
				}
			}
		}
	}

	return 0
}

func (c *Client) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gh-stats-action")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// PRComment represents an issue/PR comment on GitHub.
type PRComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// PostOrUpdatePRComment posts or updates a sticky comment on a PR.
func (c *Client) PostOrUpdatePRComment(ctx context.Context, ownerRepo string, prNumber int, marker, body string) error {
	listEndpoint := fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100", ownerRepo, prNumber)
	req, err := c.newRequest(ctx, http.MethodGet, listEndpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var existingComments []PRComment
	if resp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(resp.Body).Decode(&existingComments)
	}

	fullBody := fmt.Sprintf("%s\n\n%s", marker, body)
	payload, err := json.Marshal(map[string]string{"body": fullBody})
	if err != nil {
		return err
	}

	// Update existing comment if marker matches
	for _, comm := range existingComments {
		if strings.Contains(comm.Body, marker) {
			updateEndpoint := fmt.Sprintf("/repos/%s/issues/comments/%d", ownerRepo, comm.ID)
			updateReq, err := c.newRequest(ctx, http.MethodPatch, updateEndpoint, bytes.NewReader(payload))
			if err != nil {
				return err
			}
			updateReq.Header.Set("Content-Type", "application/json")
			uResp, err := c.httpClient.Do(updateReq)
			if err != nil {
				return err
			}
			defer uResp.Body.Close()
			if uResp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to update comment, status: %d", uResp.StatusCode)
			}
			return nil
		}
	}

	// Create new comment
	createEndpoint := fmt.Sprintf("/repos/%s/issues/%d/comments", ownerRepo, prNumber)
	createReq, err := c.newRequest(ctx, http.MethodPost, createEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	createReq.Header.Set("Content-Type", "application/json")
	cResp, err := c.httpClient.Do(createReq)
	if err != nil {
		return err
	}
	defer cResp.Body.Close()
	if cResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create comment, status: %d", cResp.StatusCode)
	}

	return nil
}

// GetPRMetadata fetches pull request data and reviews.
func (c *Client) GetPRMetadata(ctx context.Context, ownerRepo string, prNumber int) (*PRMetadata, error) {
	endpoint := fmt.Sprintf("/repos/%s/pulls/%d", ownerRepo, prNumber)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d for %s", resp.StatusCode, endpoint)
	}

	var meta PRMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}

	meta.TotalDiscussions = meta.CommentsCount + meta.ReviewComments
	if meta.MergedAt != nil {
		meta.Age = meta.MergedAt.Sub(meta.CreatedAt)
	} else {
		meta.Age = time.Since(meta.CreatedAt)
	}

	// Fetch reviews for TTFR & approval stats
	reviews, err := c.getPRReviews(ctx, ownerRepo, prNumber)
	if err == nil && len(reviews) > 0 {
		meta.ReviewsCount = len(reviews)
		for _, r := range reviews {
			if r.State == "APPROVED" {
				meta.ApprovalsCount++
			}
		}
		// First review timestamp
		firstReview := reviews[0].SubmittedAt
		if !firstReview.IsZero() && firstReview.After(meta.CreatedAt) {
			meta.TimeToFirstReview = firstReview.Sub(meta.CreatedAt)
		}
	}

	return &meta, nil
}

func (c *Client) getPRReviews(ctx context.Context, ownerRepo string, prNumber int) ([]Review, error) {
	endpoint := fmt.Sprintf("/repos/%s/pulls/%d/reviews?per_page=100", ownerRepo, prNumber)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API status %d", resp.StatusCode)
	}

	var reviews []Review
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

// GetRepoMetadata fetches repository overview data.
func (c *Client) GetRepoMetadata(ctx context.Context, ownerRepo string) (*RepoMetadata, error) {
	endpoint := fmt.Sprintf("/repos/%s", ownerRepo)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API status %d for %s", resp.StatusCode, endpoint)
	}

	var meta RepoMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// GetCommitActivity fetches weekly commit activity with handling for HTTP 202 Accepted.
func (c *Client) GetCommitActivity(ctx context.Context, ownerRepo string) ([]CommitActivityWeek, error) {
	endpoint := fmt.Sprintf("/repos/%s/stats/commit_activity", ownerRepo)

	// Poll up to 3 times if GitHub returns 202 Accepted (computing in background)
	for attempt := 0; attempt < 3; attempt++ {
		req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			var weeks []CommitActivityWeek
			err := json.NewDecoder(resp.Body).Decode(&weeks)
			resp.Body.Close()
			return weeks, err
		}

		resp.Body.Close()
		if resp.StatusCode == http.StatusAccepted {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1500 * time.Millisecond):
				continue
			}
		}

		return nil, fmt.Errorf("API status %d for %s", resp.StatusCode, endpoint)
	}

	return nil, fmt.Errorf("commit activity not ready (timed out)")
}

// GetCheckRuns fetches all check runs for a commit reference.
func (c *Client) GetCheckRuns(ctx context.Context, ownerRepo string, ref string) ([]CheckRun, error) {
	if ref == "" {
		return nil, fmt.Errorf("ref cannot be empty")
	}

	endpoint := fmt.Sprintf("/repos/%s/commits/%s/check-runs?filter=all&per_page=100", ownerRepo, ref)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d for check-runs on %s", resp.StatusCode, ref)
	}

	var crResp CheckRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&crResp); err != nil {
		return nil, err
	}

	return crResp.CheckRuns, nil
}

// GetCIPipelineStats fetches check runs and computes pipeline latency, bottlenecks, and flakiness.
func (c *Client) GetCIPipelineStats(ctx context.Context, ownerRepo string, ref string, maxLatencyMinutes int) (*CIPipelineStats, error) {
	if maxLatencyMinutes <= 0 {
		maxLatencyMinutes = 15
	}

	checkRuns, err := c.GetCheckRuns(ctx, ownerRepo, ref)
	if err != nil {
		return nil, err
	}

	stats := &CIPipelineStats{
		TotalCheckRuns: len(checkRuns),
		CheckRuns:      checkRuns,
	}

	if len(checkRuns) == 0 {
		return stats, nil
	}

	runsByName := make(map[string][]CheckRun)
	for _, cr := range checkRuns {
		runsByName[cr.Name] = append(runsByName[cr.Name], cr)

		switch cr.Status {
		case "in_progress", "queued":
			stats.InProgressRuns++
		case "completed":
			switch cr.Conclusion {
			case "success":
				stats.SuccessfulRuns++
			case "failure":
				stats.FailedRuns++
			case "timed_out":
				stats.TimedOutRuns++
			case "cancelled":
				stats.CancelledRuns++
			}
		}

		dur := cr.Duration()
		stats.TotalDuration += dur
		if dur > stats.LongestRunDuration {
			stats.LongestRunDuration = dur
			stats.LongestRunName = cr.Name
		}

		if dur > time.Duration(maxLatencyMinutes)*time.Minute {
			stats.BottleneckRuns = append(stats.BottleneckRuns, CheckRunSummary{
				Name:       cr.Name,
				Duration:   dur,
				Status:     cr.Status,
				Conclusion: cr.Conclusion,
				HTMLURL:    cr.HTMLURL,
			})
		}
	}

	if stats.TotalCheckRuns > 0 {
		stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalCheckRuns)
	}

	// Detect Flaky and Retried Checks
	for name, runs := range runsByName {
		if len(runs) < 2 {
			continue
		}

		sort.Slice(runs, func(i, j int) bool {
			if runs[i].StartedAt.Equal(runs[j].StartedAt) {
				return runs[i].ID < runs[j].ID
			}
			return runs[i].StartedAt.Before(runs[j].StartedAt)
		})

		var states []string
		hasFailureOrTimeout := false
		hasSuccess := false

		for _, r := range runs {
			conc := r.Conclusion
			if conc == "" {
				conc = r.Status
			}
			states = append(states, conc)
			if conc == "failure" || conc == "timed_out" {
				hasFailureOrTimeout = true
			}
			if conc == "success" {
				hasSuccess = true
			}
		}

		initialResult := runs[0].Conclusion
		if initialResult == "" {
			initialResult = runs[0].Status
		}
		finalResult := runs[len(runs)-1].Conclusion
		if finalResult == "" {
			finalResult = runs[len(runs)-1].Status
		}

		isFlaky := hasFailureOrTimeout && hasSuccess

		stats.FlakyRuns = append(stats.FlakyRuns, FlakyCheck{
			Name:           name,
			RetryCount:     len(runs) - 1,
			InitialResult:  initialResult,
			FinalResult:    finalResult,
			IsFlaky:        isFlaky,
			ObservedStates: states,
		})
	}

	sort.Slice(stats.FlakyRuns, func(i, j int) bool {
		if stats.FlakyRuns[i].IsFlaky != stats.FlakyRuns[j].IsFlaky {
			return stats.FlakyRuns[i].IsFlaky
		}
		return stats.FlakyRuns[i].RetryCount > stats.FlakyRuns[j].RetryCount
	})

	return stats, nil
}

