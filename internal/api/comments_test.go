package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
)

func TestCreateCommentReply(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/3/comments", 201,
		map[string]any{"id": 100})
	c := srv.Client()
	if _, err := c.CreateComment(context.Background(), "ws/repo", 3, api.CommentInput{
		Body:     "reply text",
		ParentID: 55,
	}); err != nil {
		t.Fatal(err)
	}
	req := lastRequest(t, srv, http.MethodPost, "/repositories/ws/repo/pullrequests/3/comments")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	if body["content"].(map[string]any)["raw"] != "reply text" {
		t.Errorf("content wrong: %v", body)
	}
	if body["parent"].(map[string]any)["id"].(float64) != 55 {
		t.Errorf("parent wrong: %v", body["parent"])
	}
	if _, ok := body["inline"]; ok {
		t.Errorf("inline should be absent for reply")
	}
}

func TestCreateCommentInline(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/3/comments", 201,
		map[string]any{"id": 101})
	c := srv.Client()
	to := 12
	if _, err := c.CreateComment(context.Background(), "ws/repo", 3, api.CommentInput{
		Body: "look here",
		Path: "main.go",
		To:   &to,
	}); err != nil {
		t.Fatal(err)
	}
	req := lastRequest(t, srv, http.MethodPost, "/repositories/ws/repo/pullrequests/3/comments")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	inline := body["inline"].(map[string]any)
	if inline["path"] != "main.go" {
		t.Errorf("inline path wrong: %v", inline)
	}
	if inline["to"].(float64) != 12 {
		t.Errorf("inline to wrong: %v", inline)
	}
	if _, ok := inline["from"]; ok {
		t.Errorf("inline from should be absent when nil")
	}
	if _, ok := body["parent"]; ok {
		t.Errorf("parent should be absent for top-level inline")
	}
}

func TestListComments(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/pullrequests/3/comments", 200,
		`{"values":[{"id":1,"content":{"raw":"a"}},{"id":2,"content":{"raw":"b"}}]}`)
	c := srv.Client()
	cs, err := c.ListComments(context.Background(), "ws/repo", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Content.Raw != "a" {
		t.Errorf("comments = %+v", cs)
	}
}

func TestResolveReopenDeleteComment(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/3/comments/9/resolve", 200, nil)
	srv.Handle(http.MethodDelete, "/repositories/ws/repo/pullrequests/3/comments/9/resolve", 204, nil)
	srv.Handle(http.MethodDelete, "/repositories/ws/repo/pullrequests/3/comments/9", 204, nil)
	c := srv.Client()
	ctx := context.Background()
	if err := c.ResolveComment(ctx, "ws/repo", 3, 9); err != nil {
		t.Errorf("resolve: %v", err)
	}
	if err := c.ReopenComment(ctx, "ws/repo", 3, 9); err != nil {
		t.Errorf("reopen: %v", err)
	}
	if err := c.DeleteComment(ctx, "ws/repo", 3, 9); err != nil {
		t.Errorf("delete: %v", err)
	}
}
