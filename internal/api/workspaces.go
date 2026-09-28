package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// WorkspaceMembers lists members of a workspace (up to limit; <=0 unlimited).
func (c *Client) WorkspaceMembers(ctx context.Context, workspace string, limit int) ([]WorkspaceMember, error) {
	path := "/workspaces/" + url.PathEscape(workspace) + "/members"
	return PaginateAll[WorkspaceMember](ctx, c, path, nil, limit)
}

// normalizeUUID strips surrounding braces and lowercases a UUID for matching.
func normalizeUUID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	return strings.ToLower(s)
}

// FindMembers resolves each query to a workspace member by nickname, display
// name (both case-insensitive), uuid (with or without braces), or account_id,
// fetching the member list once. The result is keyed by the trimmed query;
// blank queries are skipped. Every query that has no match or is ambiguous is
// reported in the (joined) error.
func (c *Client) FindMembers(ctx context.Context, workspace string, queries []string) (map[string]User, error) {
	found := map[string]User{}
	var wanted []string
	for _, q := range queries {
		if q = strings.TrimSpace(q); q != "" {
			wanted = append(wanted, q)
		}
	}
	if len(wanted) == 0 {
		return found, nil
	}

	members, err := c.WorkspaceMembers(ctx, workspace, 0)
	if err != nil {
		return nil, err
	}

	var errs []error
	for _, q := range wanted {
		if _, ok := found[q]; ok {
			continue
		}
		u, err := matchMember(members, q)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		found[q] = u
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return found, nil
}

// matchMember returns the single member identified by q.
func matchMember(members []WorkspaceMember, q string) (User, error) {
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
		return User{}, fmt.Errorf("no workspace member matches %q", q)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.DisplayName)
		}
		return User{}, fmt.Errorf("%q is ambiguous; matches: %s", q, strings.Join(names, ", "))
	}
}
