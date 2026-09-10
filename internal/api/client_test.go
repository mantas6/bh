package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
)

func TestNewClientDefaults(t *testing.T) {
	c := api.NewClient("", "tok", "")
	if c.BaseURL != "https://api.bitbucket.org/2.0" {
		t.Errorf("BaseURL = %q, want default", c.BaseURL)
	}
	if c.PollInterval == 0 {
		t.Errorf("PollInterval should default non-zero")
	}
	if c.MaxPollAttempts != 60 {
		t.Errorf("MaxPollAttempts = %d, want 60", c.MaxPollAttempts)
	}
}

func TestAuthBearer(t *testing.T) {
	var got string
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodGet, "/user", func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{"uuid":"{1}"}`))
	})
	c := api.NewClient(srv.URL, "sekret", "")
	c.HTTP = srv.Client().HTTP
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer sekret" {
		t.Errorf("Authorization = %q, want Bearer sekret", got)
	}
}

func TestAuthBasic(t *testing.T) {
	var got string
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodGet, "/user", func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	})
	c := api.NewClient(srv.URL, "tok", "me@example.com")
	c.HTTP = srv.Client().HTTP
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	// base64("me@example.com:tok")
	if !strings.HasPrefix(got, "Basic ") {
		t.Fatalf("Authorization = %q, want Basic", got)
	}
}

func TestAuthAnonymous(t *testing.T) {
	var hadAuth bool
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodGet, "/user", func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.Write([]byte(`{}`))
	})
	c := api.NewClient(srv.URL, "", "")
	c.HTTP = srv.Client().HTTP
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hadAuth {
		t.Errorf("anonymous request should not send Authorization")
	}
}

func TestUserAgent(t *testing.T) {
	var got string
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodGet, "/user", func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.Write([]byte(`{}`))
	})
	c := api.NewClient(srv.URL, "t", "")
	c.HTTP = srv.Client().HTTP
	c.UserAgent = "bh/1.2.3"
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "bh/1.2.3" {
		t.Errorf("User-Agent = %q, want bh/1.2.3", got)
	}
}

func TestDoRelativeAndAbsolute(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/user", 200, map[string]any{"display_name": "Rel"})
	c := srv.Client()

	// relative
	var u api.User
	if _, err := c.Do(context.Background(), http.MethodGet, "/user", nil, nil, &u); err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "Rel" {
		t.Errorf("relative path decode = %q", u.DisplayName)
	}

	// absolute
	var u2 api.User
	abs := srv.URL + "/user"
	if _, err := c.Do(context.Background(), http.MethodGet, abs, nil, nil, &u2); err != nil {
		t.Fatal(err)
	}
	if u2.DisplayName != "Rel" {
		t.Errorf("absolute path decode = %q", u2.DisplayName)
	}
}

func TestDoNoContent(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/approve", 204, nil)
	c := srv.Client()
	if err := c.ApprovePullRequest(context.Background(), "ws/repo", 1); err != nil {
		t.Fatal(err)
	}
}

func TestErrorMapping(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/user", 401,
		`{"error":{"message":"Access token expired."}}`)
	c := srv.Client()
	_, err := c.CurrentUser(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var he *api.HTTPError
	if !api.IsUnauthorized(err) {
		t.Errorf("IsUnauthorized = false, want true")
	}
	if he = asHTTPError(t, err); he.StatusCode != 401 {
		t.Errorf("status = %d, want 401", he.StatusCode)
	}
	if he.Message != "Access token expired." {
		t.Errorf("message = %q", he.Message)
	}
	if !strings.Contains(err.Error(), "bh auth login") {
		t.Errorf("401 error should include auth hint, got %q", err.Error())
	}
}

func TestErrorNotFound(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo", 404,
		`{"error":{"message":"No such repo"}}`)
	c := srv.Client()
	_, err := c.Repository(context.Background(), "ws/repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !api.IsNotFound(err) {
		t.Errorf("IsNotFound = false, want true")
	}
	if !strings.Contains(err.Error(), "git remote") {
		t.Errorf("repo 404 should hint about -R/remote, got %q", err.Error())
	}
}

func TestPaginateThreePages(t *testing.T) {
	srv := apitest.New(t)
	base := srv.URL + "/repositories/ws/repo/pullrequests"
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		switch p {
		case "", "1":
			fmt.Fprintf(w, `{"values":[{"id":1},{"id":2}],"next":%q}`, base+"?page=2")
		case "2":
			fmt.Fprintf(w, `{"values":[{"id":3},{"id":4}],"next":%q}`, base+"?page=3")
		case "3":
			fmt.Fprint(w, `{"values":[{"id":5}]}`)
		default:
			t.Errorf("unexpected page %q", p)
		}
	})
	c := srv.Client()
	prs, err := c.ListPullRequests(context.Background(), "ws/repo", api.ListPROptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 5 {
		t.Fatalf("got %d PRs, want 5", len(prs))
	}
	if prs[0].ID != 1 || prs[4].ID != 5 {
		t.Errorf("unexpected ids: %+v", prs)
	}
}

func TestPaginateLimitStopsFetching(t *testing.T) {
	srv := apitest.New(t)
	base := srv.URL + "/repositories/ws/repo/pullrequests"
	var pagesFetched int
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pagesFetched++
		p := r.URL.Query().Get("page")
		switch p {
		case "", "1":
			fmt.Fprintf(w, `{"values":[{"id":1},{"id":2}],"next":%q}`, base+"?page=2")
		case "2":
			fmt.Fprintf(w, `{"values":[{"id":3},{"id":4}],"next":%q}`, base+"?page=3")
		default:
			t.Errorf("should not fetch page %q when limit reached", p)
		}
	})
	c := srv.Client()
	prs, err := c.ListPullRequests(context.Background(), "ws/repo", api.ListPROptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 3 {
		t.Fatalf("got %d, want 3", len(prs))
	}
	if pagesFetched != 2 {
		t.Errorf("fetched %d pages, want 2", pagesFetched)
	}
}

func TestPaginateDefaultPagelen(t *testing.T) {
	srv := apitest.New(t)
	var pagelen string
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pagelen = r.URL.Query().Get("pagelen")
		fmt.Fprint(w, `{"values":[]}`)
	})
	c := srv.Client()
	if _, err := c.ListPullRequests(context.Background(), "ws/repo", api.ListPROptions{}); err != nil {
		t.Fatal(err)
	}
	if pagelen != "50" {
		t.Errorf("pagelen = %q, want 50", pagelen)
	}
}

func asHTTPError(t *testing.T, err error) *api.HTTPError {
	t.Helper()
	var he *api.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("error %v is not *HTTPError", err)
	}
	return he
}
