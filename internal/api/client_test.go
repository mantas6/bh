package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
)

func TestNewClientDefaults(t *testing.T) {
	t.Parallel()
	c := api.NewClient("", "tok", "")
	if c.BaseURL != api.DefaultBaseURL {
		t.Errorf("BaseURL = %q, want default", c.BaseURL)
	}
	if c.HTTP == nil || c.HTTP.Timeout == 0 {
		t.Errorf("HTTP client should have a timeout, got %+v", c.HTTP)
	}
}

// TestZeroClientUsable checks that a zero-value Client falls back to
// defaults lazily: the request goes out with the default User-Agent and no
// Authorization header.
func TestZeroClientUsable(t *testing.T) {
	t.Parallel()
	var c api.Client
	var got *http.Request
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       http.NoBody,
			Request:    r,
		}, nil
	})}
	if _, _, err := c.Do(t.Context(), http.MethodGet, "/user", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got.URL.String() != api.DefaultBaseURL+"/user" {
		t.Errorf("URL = %s, want default base", got.URL)
	}
	if ua := got.Header.Get("User-Agent"); ua != "bh" {
		t.Errorf("User-Agent = %q, want bh", ua)
	}
	if _, ok := got.Header["Authorization"]; ok {
		t.Error("zero client should be anonymous")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAuthAndUserAgentHeaders(t *testing.T) {
	t.Parallel()
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@example.com:tok"))
	tests := []struct {
		name      string
		token     string
		email     string
		userAgent string
		wantAuth  string // "" means the header must be absent
		wantUA    string
	}{
		{"bearer", "sekret", "", "", "Bearer sekret", "bh"},
		{"basic", "tok", "me@example.com", "", basic, "bh"},
		{"anonymous", "", "", "", "", "bh"},
		{"anonymous with email", "", "me@example.com", "", "", "bh"},
		{"custom user agent", "t", "", "bh/1.2.3", "Bearer t", "bh/1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle(http.MethodGet, "/user", 200, `{}`)
			c := api.NewClient(srv.URL, tt.token, tt.email)
			c.HTTP = srv.Client()
			c.UserAgent = tt.userAgent
			if _, err := c.CurrentUser(t.Context()); err != nil {
				t.Fatal(err)
			}
			req := srv.LastRequest(t, http.MethodGet, "/user")
			auth, hasAuth := req.Header["Authorization"]
			switch {
			case tt.wantAuth == "" && hasAuth:
				t.Errorf("Authorization = %q, want none", auth)
			case tt.wantAuth != "" && req.Header.Get("Authorization") != tt.wantAuth:
				t.Errorf("Authorization = %q, want %q", req.Header.Get("Authorization"), tt.wantAuth)
			}
			if got := req.Header.Get("User-Agent"); got != tt.wantUA {
				t.Errorf("User-Agent = %q, want %q", got, tt.wantUA)
			}
		})
	}
}

// TestNoAuthToForeignHost ensures credentials are not leaked when a `next`
// link points at a different host than BaseURL.
func TestNoAuthToForeignHost(t *testing.T) {
	t.Parallel()
	foreign := apitest.New(t)
	foreign.Handle(http.MethodGet, "/page2", 200, `{"values":[{"id":2}]}`)

	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/pullrequests", 200,
		fmt.Sprintf(`{"values":[{"id":1}],"next":%q}`, foreign.URL+"/page2"))

	c := srv.APIClient()
	prs, err := c.ListPullRequests(t.Context(), "ws/repo", api.ListPROptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}
	if got := srv.LastRequest(t, http.MethodGet, "/repositories/ws/repo/pullrequests").Header.Get("Authorization"); got != "Bearer t" {
		t.Errorf("base host Authorization = %q, want Bearer t", got)
	}
	if got, ok := foreign.LastRequest(t, http.MethodGet, "/page2").Header["Authorization"]; ok {
		t.Errorf("foreign host got Authorization %q", got)
	}
}

func TestDoRelativeAndAbsolute(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/user", 200, map[string]any{"display_name": "Rel"})
	c := srv.APIClient()

	for _, path := range []string{"/user", "user", srv.URL + "/user"} {
		var u api.User
		status, _, err := c.Do(t.Context(), http.MethodGet, path, nil, nil, &u)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if status != 200 || u.DisplayName != "Rel" {
			t.Errorf("%s: status=%d display=%q", path, status, u.DisplayName)
		}
	}
}

func TestDoReturnsStatusAndHeader(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodPost, "/thing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusCreated)
	})
	status, header, err := srv.APIClient().Do(t.Context(), http.MethodPost, "/thing", nil, map[string]string{"a": "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated || header.Get("X-Test") != "yes" {
		t.Errorf("status=%d header=%v", status, header)
	}
	req := srv.LastRequest(t, http.MethodPost, "/thing")
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
	var body map[string]string
	req.DecodeJSON(t, &body)
	if body["a"] != "b" {
		t.Errorf("body = %v", body)
	}
}

func TestDoNoContent(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/approve", 204, nil)
	c := srv.APIClient()
	if err := c.ApprovePullRequest(t.Context(), "ws/repo", 1); err != nil {
		t.Fatal(err)
	}
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/user", 401,
		`{"error":{"message":"Access token expired."}}`)
	c := srv.APIClient()
	_, err := c.CurrentUser(t.Context())
	if err == nil {
		t.Fatal("expected error")
	}
	if !api.IsUnauthorized(err) {
		t.Errorf("IsUnauthorized = false, want true")
	}
	if api.IsForbidden(err) {
		t.Errorf("IsForbidden = true, want false")
	}
	he := asHTTPError(t, err)
	if he.StatusCode != 401 || he.Message != "Access token expired." {
		t.Errorf("error = %+v", he)
	}
	if got, want := err.Error(), "Access token expired. (HTTP 401)"; got != want {
		t.Errorf("Error() = %q, want %q (CLI hints live in cmdutil)", got, want)
	}
}

func TestStatusPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status                            int
		notFound, unauthorized, forbidden bool
	}{
		{401, false, true, false},
		{403, false, false, true},
		{404, true, false, false},
		{500, false, false, false},
	}
	for _, tt := range tests {
		err := fmt.Errorf("wrapped: %w", &api.HTTPError{StatusCode: tt.status})
		if got := api.IsNotFound(err); got != tt.notFound {
			t.Errorf("IsNotFound(%d) = %v", tt.status, got)
		}
		if got := api.IsUnauthorized(err); got != tt.unauthorized {
			t.Errorf("IsUnauthorized(%d) = %v", tt.status, got)
		}
		if got := api.IsForbidden(err); got != tt.forbidden {
			t.Errorf("IsForbidden(%d) = %v", tt.status, got)
		}
	}
	if api.IsNotFound(errors.New("x")) || api.IsUnauthorized(nil) || api.IsForbidden(nil) {
		t.Error("predicates should be false for non-HTTP errors")
	}
}

func TestHTTPErrorMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  api.HTTPError
		want string
	}{
		{"message", api.HTTPError{StatusCode: 400, Message: "Bad request"}, "Bad request (HTTP 400)"},
		{"status text fallback", api.HTTPError{StatusCode: 502}, "Bad Gateway (HTTP 502)"},
		{
			"detail",
			api.HTTPError{StatusCode: 400, Message: "Bad request", Detail: "branch does not exist"},
			"Bad request (HTTP 400): branch does not exist",
		},
		{
			"detail equal to message",
			api.HTTPError{StatusCode: 400, Message: "Oops", Detail: "Oops"},
			"Oops (HTTP 400)",
		},
		{
			"fields",
			api.HTTPError{StatusCode: 400, Message: "Invalid", Fields: map[string]any{
				"title":  "required",
				"source": []any{"no such branch", "bad"},
			}},
			"Invalid (HTTP 400); source: no such branch; bad, title: required",
		},
		{
			"detail and fields",
			api.HTTPError{StatusCode: 400, Message: "Invalid", Detail: "see fields", Fields: map[string]any{"n": 1.0}},
			"Invalid (HTTP 400): see fields; n: 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorBodyDetailAndFields(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests", 400,
		`{"error":{"message":"Bad request","detail":"nope","fields":{"source":["branch not found"]}}}`)
	_, err := srv.APIClient().CreatePullRequest(t.Context(), "ws/repo", api.CreatePRInput{Title: "t"})
	he := asHTTPError(t, err)
	if he.Detail != "nope" || he.Fields["source"] == nil {
		t.Errorf("error = %+v", he)
	}
	if got, want := err.Error(), "Bad request (HTTP 400): nope; source: branch not found"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodPost, "/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		// Larger than the error body limit: must not be read in full.
		fmt.Fprint(w, "<html>"+strings.Repeat("x", 200<<10)+"</html>")
	})
	status, _, err := srv.APIClient().Do(t.Context(), http.MethodPost, "/user", nil, nil, nil)
	if status != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", status)
	}
	he := asHTTPError(t, err)
	if he.Message != "Bad Gateway" || he.Detail != "" {
		t.Errorf("error = %+v", he)
	}
	if got := len(srv.Requests()); got != 1 {
		t.Errorf("POST should not be retried; requests = %d", got)
	}
}

func TestErrorNotFound(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo", 404,
		`{"error":{"message":"No such repo"}}`)
	c := srv.APIClient()
	_, err := c.Repository(t.Context(), "ws/repo")
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

func TestPathSegmentsEscaped(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	var paths []string
	srv.HandleFunc("", "/", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		fmt.Fprint(w, `{"values":[]}`)
	})
	c := srv.APIClient()
	ctx := t.Context()
	if _, err := c.Repository(ctx, "ws/re po"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.WorkspaceMembers(ctx, "a/b", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PullRequest(ctx, "w?x/r#1", 2); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/repositories/ws/re%20po",
		"/workspaces/a%2Fb/members",
		"/repositories/w%3Fx/r%231/pullrequests/2",
	}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Errorf("paths = %q, want %q", paths, want)
	}
}

// retryServer serves /flaky, failing with status (and optional Retry-After)
// for the first fails requests and returning {} afterwards.
func retryServer(t *testing.T, method string, status, fails int, retryAfter string) (*apitest.Server, *atomic.Int32) {
	t.Helper()
	srv := apitest.New(t)
	var n atomic.Int32
	srv.HandleFunc(method, "/flaky", func(w http.ResponseWriter, r *http.Request) {
		if int(n.Add(1)) <= fails {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(status)
			return
		}
		fmt.Fprint(w, `{}`)
	})
	return srv, &n
}

// recordSleeps returns a Sleep func that records requested delays.
func recordSleeps(delays *[]time.Duration) func(context.Context, time.Duration) error {
	return func(ctx context.Context, d time.Duration) error {
		*delays = append(*delays, d)
		return ctx.Err()
	}
}

func TestRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		method     string
		status     int
		fails      int
		retryAfter string
		wantErr    bool
		wantCalls  int
		wantDelays []time.Duration
	}{
		{"GET 503 then ok", http.MethodGet, 503, 2, "", false, 3, []time.Duration{500 * time.Millisecond, time.Second}},
		{"GET 429 Retry-After seconds", http.MethodGet, 429, 1, "7", false, 2, []time.Duration{7 * time.Second}},
		{"PUT 502", http.MethodPut, 502, 1, "", false, 2, []time.Duration{500 * time.Millisecond}},
		{"DELETE 504", http.MethodDelete, 504, 1, "", false, 2, []time.Duration{500 * time.Millisecond}},
		{"gives up after max retries", http.MethodGet, 503, 10, "", true, 4,
			[]time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}},
		{"POST not retried", http.MethodPost, 503, 1, "", true, 1, nil},
		{"500 not retried", http.MethodGet, 500, 1, "", true, 1, nil},
		{"Retry-After too long", http.MethodGet, 429, 1, "3600", true, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, calls := retryServer(t, tt.method, tt.status, tt.fails, tt.retryAfter)
			c := srv.APIClient()
			var delays []time.Duration
			c.Sleep = recordSleeps(&delays)
			status, _, err := c.Do(t.Context(), tt.method, "/flaky", nil, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && status != tt.status {
				t.Errorf("status = %d, want %d", status, tt.status)
			}
			if got := int(calls.Load()); got != tt.wantCalls {
				t.Errorf("calls = %d, want %d", got, tt.wantCalls)
			}
			if fmt.Sprint(delays) != fmt.Sprint(tt.wantDelays) {
				t.Errorf("delays = %v, want %v", delays, tt.wantDelays)
			}
		})
	}
}

func TestRetryResendsBody(t *testing.T) {
	t.Parallel()
	srv, _ := retryServer(t, http.MethodPut, 503, 1, "")
	if _, _, err := srv.APIClient().Do(t.Context(), http.MethodPut, "/flaky", nil, map[string]int{"n": 1}, nil); err != nil {
		t.Fatal(err)
	}
	for _, r := range srv.Requests() {
		if string(r.Body) != `{"n":1}` {
			t.Errorf("attempt body = %q", r.Body)
		}
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	t.Parallel()
	srv, calls := retryServer(t, http.MethodGet, 503, 1, time.Now().Add(5*time.Second).UTC().Format(http.TimeFormat))
	c := srv.APIClient()
	var delays []time.Duration
	c.Sleep = recordSleeps(&delays)
	if _, _, err := c.Do(t.Context(), http.MethodGet, "/flaky", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(delays) != 1 {
		t.Fatalf("calls = %d, delays = %v", calls.Load(), delays)
	}
	// HTTP-date has one-second resolution.
	if delays[0] < 3*time.Second || delays[0] > 5*time.Second {
		t.Errorf("delay = %v, want ~5s", delays[0])
	}
}

func TestRetryCancelledDuringBackoff(t *testing.T) {
	t.Parallel()
	srv, calls := retryServer(t, http.MethodGet, 503, 10, "")
	c := srv.APIClient()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c.Sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}
	_, _, err := c.Do(ctx, http.MethodGet, "/flaky", nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

// TestDefaultSleepHonoursContext exercises the real timer-based sleep: an
// already-cancelled context must abort the backoff immediately.
func TestDefaultSleepHonoursContext(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "20")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(hs.Close)
	c := api.NewClient(hs.URL, "", "")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := c.Do(ctx, http.MethodGet, "/x", nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("sleep ignored context")
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestPaginateThreePages(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	base := srv.URL + "/repositories/ws/repo/pullrequests"
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		switch p {
		case "", "1":
			fmt.Fprintf(w, `{"values":[{"id":1},{"id":2}],"next":%q}`, base+"?page=2&state=OPEN&pagelen=50")
		case "2":
			fmt.Fprintf(w, `{"values":[{"id":3},{"id":4}],"next":%q}`, base+"?page=3&state=OPEN&pagelen=50")
		case "3":
			fmt.Fprint(w, `{"values":[{"id":5}]}`)
		default:
			t.Errorf("unexpected page %q", p)
		}
	})
	c := srv.APIClient()
	prs, err := c.ListPullRequests(t.Context(), "ws/repo", api.ListPROptions{State: []string{"OPEN"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 5 {
		t.Fatalf("got %d PRs, want 5", len(prs))
	}
	if prs[0].ID != 1 || prs[4].ID != 5 {
		t.Errorf("unexpected ids: %+v", prs)
	}
	// The query is only added to the first request; `next` links carry it.
	for i, r := range srv.Requests() {
		if got := r.Query["state"]; len(got) != 1 {
			t.Errorf("request %d state = %v, want exactly one", i, got)
		}
		if got := r.Query["pagelen"]; len(got) != 1 {
			t.Errorf("request %d pagelen = %v, want exactly one", i, got)
		}
	}
}

func TestPaginateLimitStopsFetching(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	base := srv.URL + "/repositories/ws/repo/pullrequests"
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
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
	c := srv.APIClient()
	prs, err := c.ListPullRequests(t.Context(), "ws/repo", api.ListPROptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 3 {
		t.Fatalf("got %d, want 3", len(prs))
	}
	if got := len(srv.Requests()); got != 2 {
		t.Errorf("fetched %d pages, want 2", got)
	}
}

func TestPaginatePagelen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		limit int
		query map[string]string
		want  string
	}{
		{"unlimited", 0, nil, "50"},
		{"small limit", 7, nil, "7"},
		{"large limit", 500, nil, "50"},
		{"explicit", 3, map[string]string{"pagelen": "100"}, "100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle(http.MethodGet, "/items", 200, `{"values":[]}`)
			q := map[string][]string{}
			for k, v := range tt.query {
				q[k] = []string{v}
			}
			if err := srv.APIClient().Paginate(t.Context(), "/items", q, tt.limit, func(json.RawMessage) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if got := srv.LastRequest(t, http.MethodGet, "/items").Query.Get("pagelen"); got != tt.want {
				t.Errorf("pagelen = %q, want %q", got, tt.want)
			}
			if tt.query != nil && len(q["pagelen"]) != 1 {
				t.Errorf("caller's query was mutated: %v", q)
			}
		})
	}
}

func TestPaginateLoopGuard(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	next := srv.URL + "/items?page=2"
	srv.HandleFunc(http.MethodGet, "/items", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"values":[{"id":1}],"next":%q}`, next)
	})
	var n int
	err := srv.APIClient().Paginate(t.Context(), "/items", nil, 0, func(json.RawMessage) error { n++; return nil })
	if err == nil || !strings.Contains(err.Error(), "pagination loop") {
		t.Fatalf("err = %v, want pagination loop error", err)
	}
	if got := len(srv.Requests()); got != 2 {
		t.Errorf("requests = %d, want 2", got)
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
