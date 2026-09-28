package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
)

// membersServer serves the workspace member list split over two pages so
// FindMembers has to paginate.
func membersServer(t *testing.T) *apitest.Server {
	t.Helper()
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodGet, "/workspaces/ws/members", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `{"values":[
			  {"user":{"uuid":"{ccc}","account_id":"acc3","nickname":"bob","display_name":"Bob Twin"}},
			  {"user":{"uuid":"{ddd}","account_id":"acc4","nickname":"dora","display_name":"Dora"}}
			]}`)
			return
		}
		fmt.Fprintf(w, `{"values":[
		  {"user":{"uuid":"{aaa}","account_id":"acc1","nickname":"alice","display_name":"Alice Wonderland"}},
		  {"user":{"uuid":"{bbb}","account_id":"acc2","nickname":"bob","display_name":"Bob Builder"}}
		],"next":%q}`, "http://"+r.Host+"/workspaces/ws/members?page=2")
	})
	return srv
}

func TestFindMembers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		queries []string
		want    map[string]string // query -> nickname
		wantErr []string          // substrings of the error
	}{
		{"nickname", []string{"alice"}, map[string]string{"alice": "alice"}, nil},
		{"display name case-insensitive", []string{"alice WONDERLAND"}, map[string]string{"alice WONDERLAND": "alice"}, nil},
		{"uuid without braces", []string{"AAA"}, map[string]string{"AAA": "alice"}, nil},
		{"uuid with braces", []string{"{ddd}"}, map[string]string{"{ddd}": "dora"}, nil},
		{"account id", []string{"acc1"}, map[string]string{"acc1": "alice"}, nil},
		{"second page", []string{"dora"}, map[string]string{"dora": "dora"}, nil},
		{
			"several at once, trimmed, blanks skipped",
			[]string{" alice ", "", "acc4", "alice"},
			map[string]string{"alice": "alice", "acc4": "dora"},
			nil,
		},
		{"ambiguous", []string{"bob"}, nil, []string{`"bob" is ambiguous; matches: Bob Builder, Bob Twin`}},
		{"none", []string{"nobody"}, nil, []string{`no workspace member matches "nobody"`}},
		{
			"all failures reported",
			[]string{"alice", "nobody", "bob"},
			nil,
			[]string{`no workspace member matches "nobody"`, `"bob" is ambiguous`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := membersServer(t)
			got, err := srv.APIClient().FindMembers(t.Context(), "ws", tt.queries)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("err = nil, want %v", tt.wantErr)
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("err = %q, want to contain %q", err, w)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d results %v, want %d", len(got), got, len(tt.want))
			}
			for q, nick := range tt.want {
				if got[q].Nickname != nick {
					t.Errorf("%q -> %q, want %q", q, got[q].Nickname, nick)
				}
			}
			// One listing (two pages) regardless of how many queries.
			if n := len(srv.Requests()); n != 2 {
				t.Errorf("requests = %d, want 2", n)
			}
		})
	}
}

func TestFindMembersNoQueriesSkipsRequest(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // no routes
	got, err := srv.APIClient().FindMembers(t.Context(), "ws", []string{"", "  "})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("requests = %d, want 0", n)
	}
}
