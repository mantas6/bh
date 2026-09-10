package api

import (
	"context"
	"fmt"
	"net/http"
)

// Repository fetches repository metadata for fullName ("ws/repo").
func (c *Client) Repository(ctx context.Context, fullName string) (*Repository, error) {
	var r Repository
	path := "/repositories/" + fullName
	if _, err := c.Do(ctx, http.MethodGet, path, nil, nil, &r); err != nil {
		if IsNotFound(err) {
			return nil, fmt.Errorf("repository %q not found; check -R / your git remote: %w", fullName, err)
		}
		return nil, err
	}
	return &r, nil
}
