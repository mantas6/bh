package api

import (
	"context"
	"fmt"
	"net/http"
)

// Repository fetches repository metadata for fullName ("ws/repo").
func (c *Client) Repository(ctx context.Context, fullName string) (*Repository, error) {
	var r Repository
	if _, _, err := c.Do(ctx, http.MethodGet, repoPath(fullName), nil, nil, &r); err != nil {
		if IsNotFound(err) {
			return nil, fmt.Errorf("repository %q not found; check -R / your git remote: %w", fullName, err)
		}
		return nil, err
	}
	return &r, nil
}

// defaultReviewer is an entry of a repository's effective default reviewers.
type defaultReviewer struct {
	User         User   `json:"user"`
	ReviewerType string `json:"reviewer_type,omitempty"`
}

// EffectiveDefaultReviewers returns the users Bitbucket suggests as reviewers
// for new pull requests in fullName ("ws/repo"): the repository's default
// reviewers together with those inherited from its project. Bitbucket only
// applies them automatically when a pull request is created in the web UI.
func (c *Client) EffectiveDefaultReviewers(ctx context.Context, fullName string) ([]User, error) {
	entries, err := PaginateAll[defaultReviewer](ctx, c, repoPath(fullName)+"/effective-default-reviewers", nil, 0)
	if err != nil {
		return nil, err
	}
	users := make([]User, 0, len(entries))
	for _, e := range entries {
		if e.User.UUID != "" || e.User.AccountID != "" {
			users = append(users, e.User)
		}
	}
	return users, nil
}
