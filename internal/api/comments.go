package api

import (
	"context"
	"fmt"
	"net/http"
)

func commentsPath(repo string, id int) string {
	return fmt.Sprintf("/repositories/%s/pullrequests/%d/comments", repo, id)
}

// ListComments lists comments on a pull request (up to limit; <=0 unlimited).
func (c *Client) ListComments(ctx context.Context, repo string, id, limit int) ([]Comment, error) {
	return PaginateAll[Comment](ctx, c, commentsPath(repo, id), nil, limit)
}

// CommentInput is the input for CreateComment.
type CommentInput struct {
	Body     string
	ParentID int    // >0 makes this a reply
	Path     string // inline comment file path
	To       *int   // inline: line in the new file
	From     *int   // inline: line in the old file
}

// CreateComment posts a new comment (top-level, reply, or inline).
func (c *Client) CreateComment(ctx context.Context, repo string, id int, in CommentInput) (*Comment, error) {
	body := map[string]any{
		"content": map[string]any{"raw": in.Body},
	}
	if in.ParentID > 0 {
		body["parent"] = map[string]any{"id": in.ParentID}
	}
	if in.Path != "" {
		inline := map[string]any{"path": in.Path}
		if in.To != nil {
			inline["to"] = *in.To
		}
		if in.From != nil {
			inline["from"] = *in.From
		}
		body["inline"] = inline
	}

	var out Comment
	if _, err := c.Do(ctx, http.MethodPost, commentsPath(repo, id), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteComment removes a comment.
func (c *Client) DeleteComment(ctx context.Context, repo string, id, commentID int) error {
	path := fmt.Sprintf("%s/%d", commentsPath(repo, id), commentID)
	_, err := c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
	return err
}

// ResolveComment marks a comment thread as resolved.
func (c *Client) ResolveComment(ctx context.Context, repo string, id, commentID int) error {
	path := fmt.Sprintf("%s/%d/resolve", commentsPath(repo, id), commentID)
	_, err := c.Do(ctx, http.MethodPost, path, nil, nil, nil)
	return err
}

// ReopenComment reopens (unresolves) a comment thread.
func (c *Client) ReopenComment(ctx context.Context, repo string, id, commentID int) error {
	path := fmt.Sprintf("%s/%d/resolve", commentsPath(repo, id), commentID)
	_, err := c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
	return err
}
