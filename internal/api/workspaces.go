package api

import (
	"context"
	"fmt"
	"strings"
)

// WorkspaceMembers lists members of a workspace (up to limit; <=0 unlimited).
func (c *Client) WorkspaceMembers(ctx context.Context, workspace string, limit int) ([]WorkspaceMember, error) {
	path := fmt.Sprintf("/workspaces/%s/members", workspace)
	return PaginateAll[WorkspaceMember](ctx, c, path, nil, limit)
}

// normalizeUUID strips surrounding braces and lowercases a UUID for matching.
func normalizeUUID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	return strings.ToLower(s)
}

// FindMember resolves a workspace member by nickname, display name (both
// case-insensitive), uuid (with or without braces), or account_id. It returns
// an error when there is no match or when the query is ambiguous.
func (c *Client) FindMember(ctx context.Context, workspace, query string) (*User, error) {
	members, err := c.WorkspaceMembers(ctx, workspace, 0)
	if err != nil {
		return nil, err
	}

	q := strings.TrimSpace(query)
	lower := strings.ToLower(q)
	qUUID := normalizeUUID(q)

	var matches []User
	seen := map[string]bool{}
	for _, m := range members {
		u := m.User
		switch {
		case u.AccountID != "" && u.AccountID == q:
		case u.UUID != "" && normalizeUUID(u.UUID) == qUUID:
		case u.Nickname != "" && strings.ToLower(u.Nickname) == lower:
		case u.DisplayName != "" && strings.ToLower(u.DisplayName) == lower:
		default:
			continue
		}
		key := u.UUID
		if key == "" {
			key = u.AccountID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		matches = append(matches, u)
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no workspace member matches %q", query)
	case 1:
		u := matches[0]
		return &u, nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.DisplayName)
		}
		return nil, fmt.Errorf("%q is ambiguous; matches: %s", query, strings.Join(names, ", "))
	}
}
