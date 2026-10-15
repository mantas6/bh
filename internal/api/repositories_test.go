package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
)

func TestEffectiveDefaultReviewers(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `{"values":[
			  {"reviewer_type":"project","user":{"uuid":"{bbb}","account_id":"acc2","display_name":"Bob"}},
			  {"reviewer_type":"repository","user":{}}
			]}`)
			return
		}
		fmt.Fprintf(w, `{"values":[
		  {"reviewer_type":"repository","user":{"uuid":"{aaa}","account_id":"acc1","display_name":"Alice"}}
		],"next":%q}`, "http://"+r.Host+"/repositories/ws/repo/effective-default-reviewers?page=2")
	})

	users, err := srv.APIClient().EffectiveDefaultReviewers(t.Context(), "ws/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].UUID != "{aaa}" || users[1].DisplayName != "Bob" || users[1].AccountID != "acc2" {
		t.Errorf("users = %+v", users)
	}
}

func TestEffectiveDefaultReviewersError(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/effective-default-reviewers", 403, `{"type":"error","error":{"message":"forbidden"}}`)
	if _, err := srv.APIClient().EffectiveDefaultReviewers(t.Context(), "ws/repo"); err == nil {
		t.Fatal("expected an error")
	}
}
