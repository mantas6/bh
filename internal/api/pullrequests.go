package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func prPath(repo string, id int) string {
	return fmt.Sprintf("/repositories/%s/pullrequests/%d", repo, id)
}

// ListPROptions configures ListPullRequests.
type ListPROptions struct {
	// State filters by PR state, e.g. []string{"OPEN","MERGED"}. Each value
	// becomes a repeated `state` query parameter.
	State []string
	// Query is an optional raw BBQL `q` filter.
	Query string
	// Limit caps the number of results; <=0 means unlimited.
	Limit int
}

// ListPullRequests lists pull requests for repo ("ws/repo").
func (c *Client) ListPullRequests(ctx context.Context, repo string, opts ListPROptions) ([]PullRequest, error) {
	q := url.Values{}
	for _, s := range opts.State {
		if s != "" {
			q.Add("state", s)
		}
	}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	path := fmt.Sprintf("/repositories/%s/pullrequests", repo)
	return PaginateAll[PullRequest](ctx, c, path, q, opts.Limit)
}

// PullRequest fetches a single pull request by id.
func (c *Client) PullRequest(ctx context.Context, repo string, id int) (*PullRequest, error) {
	var pr PullRequest
	if _, err := c.Do(ctx, http.MethodGet, prPath(repo, id), nil, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// PullRequestForBranch returns the open pull request whose source branch is
// branch. If multiple match, the first is returned; if none, an error.
func (c *Client) PullRequestForBranch(ctx context.Context, repo, branch string) (*PullRequest, error) {
	q := fmt.Sprintf(`source.branch.name="%s" AND state="OPEN"`, branch)
	prs, err := c.ListPullRequests(ctx, repo, ListPROptions{Query: q, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, fmt.Errorf("no open pull request found for branch %q", branch)
	}
	pr := prs[0]
	return &pr, nil
}

// CreatePRInput is the input for CreatePullRequest.
type CreatePRInput struct {
	Title             string
	Description       string
	SourceBranch      string
	SourceRepo        string   // full_name of the source repo (for forks); optional
	DestinationBranch string   // optional; omitted uses repo default
	Reviewers         []string // reviewer UUIDs
	Draft             bool
	CloseSourceBranch bool
}

// CreatePullRequest opens a new pull request.
func (c *Client) CreatePullRequest(ctx context.Context, repo string, in CreatePRInput) (*PullRequest, error) {
	body := map[string]any{
		"title":               in.Title,
		"draft":               in.Draft,
		"close_source_branch": in.CloseSourceBranch,
	}
	if in.Description != "" {
		body["description"] = in.Description
	}

	source := map[string]any{
		"branch": map[string]any{"name": in.SourceBranch},
	}
	if in.SourceRepo != "" {
		source["repository"] = map[string]any{"full_name": in.SourceRepo}
	}
	body["source"] = source

	if in.DestinationBranch != "" {
		body["destination"] = map[string]any{
			"branch": map[string]any{"name": in.DestinationBranch},
		}
	}

	if in.Reviewers != nil {
		reviewers := make([]map[string]any, 0, len(in.Reviewers))
		for _, uuid := range in.Reviewers {
			reviewers = append(reviewers, map[string]any{"uuid": uuid})
		}
		body["reviewers"] = reviewers
	}

	var pr PullRequest
	path := fmt.Sprintf("/repositories/%s/pullrequests", repo)
	if _, err := c.Do(ctx, http.MethodPost, path, nil, body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePullRequest applies a partial update via PUT.
func (c *Client) UpdatePullRequest(ctx context.Context, repo string, id int, body map[string]any) (*PullRequest, error) {
	var pr PullRequest
	if _, err := c.Do(ctx, http.MethodPut, prPath(repo, id), nil, body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// MergeInput configures MergePullRequest.
type MergeInput struct {
	Message           string
	Strategy          string // merge_commit|squash|fast_forward; empty = API default
	CloseSourceBranch bool
}

// MergePullRequest merges a pull request, handling async (202) merges by
// polling the task-status endpoint until SUCCESS.
func (c *Client) MergePullRequest(ctx context.Context, repo string, id int, in MergeInput) (*PullRequest, error) {
	body := map[string]any{
		"type":                "pullrequest",
		"message":             in.Message,
		"close_source_branch": in.CloseSourceBranch,
	}
	if in.Strategy != "" {
		body["merge_strategy"] = in.Strategy
	}

	path := prPath(repo, id) + "/merge"
	var pr PullRequest
	resp, err := c.Do(ctx, http.MethodPost, path, nil, body, &pr)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusAccepted {
		return c.pollMerge(ctx, repo, id, resp)
	}
	return &pr, nil
}

// pollMerge polls the merge task-status endpoint until SUCCESS or failure.
func (c *Client) pollMerge(ctx context.Context, repo string, id int, resp *http.Response) (*PullRequest, error) {
	taskID := ""
	if loc := resp.Header.Get("Location"); loc != "" {
		parts := strings.Split(strings.TrimRight(loc, "/"), "/")
		taskID = parts[len(parts)-1]
	}

	attempts := c.MaxPollAttempts
	if attempts <= 0 {
		attempts = 60
	}
	statusPath := prPath(repo, id) + "/merge/task-status/" + taskID

	for i := 0; i < attempts; i++ {
		var st MergeTaskStatus
		if _, err := c.Do(ctx, http.MethodGet, statusPath, nil, nil, &st); err != nil {
			return nil, err
		}
		if st.TaskID != "" && taskID == "" {
			taskID = st.TaskID
			statusPath = prPath(repo, id) + "/merge/task-status/" + taskID
		}
		switch strings.ToUpper(st.TaskStatus) {
		case "SUCCESS":
			if st.MergeResult != nil {
				return st.MergeResult, nil
			}
			return c.PullRequest(ctx, repo, id)
		case "", "PENDING":
			// keep polling
		default:
			return nil, fmt.Errorf("merge failed with task status %q", st.TaskStatus)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.PollInterval):
		}
	}
	return nil, fmt.Errorf("merge did not complete after %d polls", attempts)
}

// ApprovePullRequest approves a pull request.
func (c *Client) ApprovePullRequest(ctx context.Context, repo string, id int) error {
	_, err := c.Do(ctx, http.MethodPost, prPath(repo, id)+"/approve", nil, nil, nil)
	return err
}

// UnapprovePullRequest removes the caller's approval.
func (c *Client) UnapprovePullRequest(ctx context.Context, repo string, id int) error {
	_, err := c.Do(ctx, http.MethodDelete, prPath(repo, id)+"/approve", nil, nil, nil)
	return err
}

// RequestChanges marks the caller as requesting changes.
func (c *Client) RequestChanges(ctx context.Context, repo string, id int) error {
	_, err := c.Do(ctx, http.MethodPost, prPath(repo, id)+"/request-changes", nil, nil, nil)
	return err
}

// UnrequestChanges clears the caller's request-changes state.
func (c *Client) UnrequestChanges(ctx context.Context, repo string, id int) error {
	_, err := c.Do(ctx, http.MethodDelete, prPath(repo, id)+"/request-changes", nil, nil, nil)
	return err
}

// DeclinePullRequest declines (closes) a pull request.
func (c *Client) DeclinePullRequest(ctx context.Context, repo string, id int) (*PullRequest, error) {
	var pr PullRequest
	if _, err := c.Do(ctx, http.MethodPost, prPath(repo, id)+"/decline", nil, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
