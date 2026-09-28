package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxPollAttempts is the default cap on merge task-status polls.
const maxPollAttempts = 60

func prsPath(repo string) string {
	return repoPath(repo) + "/pullrequests"
}

func prPath(repo string, id int) string {
	return prsPath(repo) + "/" + strconv.Itoa(id)
}

// ListPROptions configures ListPullRequests.
type ListPROptions struct {
	// State filters by PR state, e.g. []string{"OPEN","MERGED"}. Each value
	// becomes a repeated `state` query parameter.
	State []string
	// Query is an optional raw BBQL `q` filter. Interpolate user input with
	// QuoteBBQL.
	Query string
	// Sort is an optional sort field, e.g. "-updated_on" for newest first.
	Sort string
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
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	}
	return PaginateAll[PullRequest](ctx, c, prsPath(repo), q, opts.Limit)
}

// PullRequest fetches a single pull request by id.
func (c *Client) PullRequest(ctx context.Context, repo string, id int) (*PullRequest, error) {
	var pr PullRequest
	if _, _, err := c.Do(ctx, http.MethodGet, prPath(repo, id), nil, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// PullRequestForBranch returns the open pull request whose source branch is
// branch. If several match, the most recently updated one is returned; if
// none, an error.
func (c *Client) PullRequestForBranch(ctx context.Context, repo, branch string) (*PullRequest, error) {
	q := fmt.Sprintf(`source.branch.name=%s AND state="OPEN"`, QuoteBBQL(branch))
	prs, err := c.ListPullRequests(ctx, repo, ListPROptions{Query: q, Sort: "-updated_on", Limit: 1})
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
		body["reviewers"] = uuidRefs(in.Reviewers)
	}

	var pr PullRequest
	if _, _, err := c.Do(ctx, http.MethodPost, prsPath(repo), nil, body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePRInput is a partial pull request update for UpdatePullRequest. Nil
// fields are left unchanged.
type UpdatePRInput struct {
	Title             *string
	Description       *string
	DestinationBranch *string
	// Reviewers replaces the reviewer list with these UUIDs when non-nil
	// (an empty, non-nil slice removes every reviewer).
	Reviewers []string
	Draft     *bool
}

type uuidRef struct {
	UUID string `json:"uuid"`
}

func uuidRefs(uuids []string) []uuidRef {
	refs := make([]uuidRef, 0, len(uuids))
	for _, u := range uuids {
		refs = append(refs, uuidRef{UUID: u})
	}
	return refs
}

type branchRef struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
}

// updatePRBody is the JSON shape of UpdatePRInput.
type updatePRBody struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	Destination *branchRef `json:"destination,omitempty"`
	Reviewers   *[]uuidRef `json:"reviewers,omitempty"`
	Draft       *bool      `json:"draft,omitempty"`
}

// UpdatePullRequest applies a partial update via PUT.
func (c *Client) UpdatePullRequest(ctx context.Context, repo string, id int, in UpdatePRInput) (*PullRequest, error) {
	body := updatePRBody{
		Title:       in.Title,
		Description: in.Description,
		Draft:       in.Draft,
	}
	if in.DestinationBranch != nil {
		body.Destination = &branchRef{}
		body.Destination.Branch.Name = *in.DestinationBranch
	}
	if in.Reviewers != nil {
		refs := uuidRefs(in.Reviewers)
		body.Reviewers = &refs
	}

	var pr PullRequest
	if _, _, err := c.Do(ctx, http.MethodPut, prPath(repo, id), nil, body, &pr); err != nil {
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
	var raw json.RawMessage
	status, header, err := c.Do(ctx, http.MethodPost, path, nil, body, &raw)
	if err != nil {
		return nil, err
	}

	if status == http.StatusAccepted {
		var st MergeTaskStatus
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &st); err != nil {
				return nil, fmt.Errorf("decoding merge task status: %w", err)
			}
		}
		statusURL, err := c.taskStatusURL(c.resolveURL(path), header.Get("Location"), repo, id, st.TaskID)
		if err != nil {
			return nil, err
		}
		return c.pollMerge(ctx, repo, id, statusURL, st)
	}

	var pr PullRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &pr); err != nil {
			return nil, fmt.Errorf("decoding response body: %w", err)
		}
	}
	return &pr, nil
}

// taskStatusURL picks the URL to poll for an async merge: the Location
// header (resolved against the merge request URL when relative), falling back
// to the task-status endpoint for taskID.
func (c *Client) taskStatusURL(requestURL, location, repo string, id int, taskID string) (string, error) {
	if location != "" {
		base, err := url.Parse(requestURL)
		if err != nil {
			return "", err
		}
		ref, err := url.Parse(location)
		if err != nil {
			return "", fmt.Errorf("invalid merge task Location %q: %w", location, err)
		}
		return base.ResolveReference(ref).String(), nil
	}
	if taskID != "" {
		return c.resolveURL(prPath(repo, id) + "/merge/task-status/" + url.PathEscape(taskID)), nil
	}
	return "", errors.New("merge was accepted but the response has no task status location")
}

// pollMerge polls statusURL until the merge task succeeds or fails. st is the
// status already returned with the 202 response.
func (c *Client) pollMerge(ctx context.Context, repo string, id int, statusURL string, st MergeTaskStatus) (*PullRequest, error) {
	attempts := c.maxPollAttempts()
	for i := 0; ; i++ {
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
		if i >= attempts {
			return nil, fmt.Errorf("merge did not complete after %d polls", attempts)
		}

		if err := c.sleep(ctx, c.pollInterval()); err != nil {
			return nil, err
		}
		st = MergeTaskStatus{}
		if _, _, err := c.Do(ctx, http.MethodGet, statusURL, nil, nil, &st); err != nil {
			return nil, err
		}
	}
}

// ApprovePullRequest approves a pull request.
func (c *Client) ApprovePullRequest(ctx context.Context, repo string, id int) error {
	_, _, err := c.Do(ctx, http.MethodPost, prPath(repo, id)+"/approve", nil, nil, nil)
	return err
}

// UnapprovePullRequest removes the caller's approval.
func (c *Client) UnapprovePullRequest(ctx context.Context, repo string, id int) error {
	_, _, err := c.Do(ctx, http.MethodDelete, prPath(repo, id)+"/approve", nil, nil, nil)
	return err
}

// RequestChanges marks the caller as requesting changes.
func (c *Client) RequestChanges(ctx context.Context, repo string, id int) error {
	_, _, err := c.Do(ctx, http.MethodPost, prPath(repo, id)+"/request-changes", nil, nil, nil)
	return err
}

// UnrequestChanges clears the caller's request-changes state.
func (c *Client) UnrequestChanges(ctx context.Context, repo string, id int) error {
	_, _, err := c.Do(ctx, http.MethodDelete, prPath(repo, id)+"/request-changes", nil, nil, nil)
	return err
}

// DeclinePullRequest declines (closes) a pull request.
func (c *Client) DeclinePullRequest(ctx context.Context, repo string, id int) (*PullRequest, error) {
	var pr PullRequest
	if _, _, err := c.Do(ctx, http.MethodPost, prPath(repo, id)+"/decline", nil, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
