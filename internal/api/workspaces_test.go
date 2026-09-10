package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
)

const membersJSON = `{"values":[
  {"user":{"uuid":"{aaa}","account_id":"acc1","nickname":"alice","display_name":"Alice Wonderland"}},
  {"user":{"uuid":"{bbb}","account_id":"acc2","nickname":"bob","display_name":"Bob Builder"}},
  {"user":{"uuid":"{ccc}","account_id":"acc3","nickname":"bob","display_name":"Bob Twin"}}
]}`

func membersServer(t *testing.T) *apitest.Server {
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/workspaces/ws/members", 200, membersJSON)
	return srv
}

func TestFindMemberByNickname(t *testing.T) {
	c := membersServer(t).Client()
	u, err := c.FindMember(context.Background(), "ws", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "Alice Wonderland" {
		t.Errorf("display = %q", u.DisplayName)
	}
}

func TestFindMemberByUUIDBraces(t *testing.T) {
	c := membersServer(t).Client()
	// Without braces, uppercase - should still match {aaa}.
	u, err := c.FindMember(context.Background(), "ws", "AAA")
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "alice" {
		t.Errorf("nick = %q", u.Nickname)
	}
}

func TestFindMemberByAccountID(t *testing.T) {
	c := membersServer(t).Client()
	u, err := c.FindMember(context.Background(), "ws", "acc1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Nickname != "alice" {
		t.Errorf("nick = %q", u.Nickname)
	}
}

func TestFindMemberAmbiguous(t *testing.T) {
	c := membersServer(t).Client()
	_, err := c.FindMember(context.Background(), "ws", "bob")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v, want ambiguous", err)
	}
}

func TestFindMemberNone(t *testing.T) {
	c := membersServer(t).Client()
	_, err := c.FindMember(context.Background(), "ws", "nobody")
	if err == nil || !strings.Contains(err.Error(), "no workspace member matches") {
		t.Fatalf("err = %v", err)
	}
}
