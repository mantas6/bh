package api

import (
	"context"
	"net/http"
)

// CurrentUser returns the authenticated account (GET /user).
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	var u User
	if _, err := c.Do(ctx, http.MethodGet, "/user", nil, nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
