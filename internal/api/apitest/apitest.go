// Package apitest provides an httptest-backed fake Bitbucket API server and a
// preconfigured api.Client for use in command and client tests. It records
// every request (method, path, query, header, body) for later assertions.
package apitest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
)

// Request is a recorded inbound request.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// DecodeJSON unmarshals the request body into v, failing the test on error.
func (r Request) DecodeJSON(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("apitest: decoding %s %s body %q: %v", r.Method, r.Path, r.Body, err)
	}
}

// Server wraps an httptest.Server with request recording and a route mux.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	requests []Request

	mux *http.ServeMux
	t   testing.TB
}

// New starts a recording server. It is automatically closed via t.Cleanup.
// Requests to unregistered routes fail t and get a 404.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		mux: http.NewServeMux(),
		t:   t,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Server.Close)
	return s
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Header: r.Header.Clone(),
		Body:   body,
	})
	s.mu.Unlock()

	h, pattern := s.mux.Handler(r)
	if pattern == "" {
		s.t.Errorf("apitest: unexpected request %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"no such route"}}`)
		return
	}
	h.ServeHTTP(w, r)
}

// Requests returns a copy of the requests recorded so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// LastRequest returns the most recent recorded request with exactly this
// method and path, failing t if there is none.
func (s *Server) LastRequest(t testing.TB, method, path string) Request {
	t.Helper()
	reqs := s.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Method == method && reqs[i].Path == path {
			return reqs[i]
		}
	}
	t.Fatalf("apitest: no recorded request %s %s", method, path)
	return Request{}
}

// pattern builds a method-scoped ServeMux pattern (Go 1.22+ routing).
func pattern(method, path string) string {
	if method == "" {
		return path
	}
	return method + " " + path
}

// Handle registers a route returning status with response JSON-encoded. If
// response is a string it is written verbatim; if nil, no body is written.
func (s *Server) Handle(method, path string, status int, response any) {
	s.mux.HandleFunc(pattern(method, path), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		switch v := response.(type) {
		case nil:
			// no body
		case string:
			_, _ = io.WriteString(w, v)
		case []byte:
			_, _ = w.Write(v)
		default:
			_ = json.NewEncoder(w).Encode(v)
		}
	})
}

// HandleFunc registers a custom handler for a method+path.
func (s *Server) HandleFunc(method, path string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern(method, path), h)
}

// APIClient returns an api.Client pointed at this server, authenticated with
// a dummy Bearer token. Retry backoff and merge polling do not sleep (but
// still observe context cancellation), and polling is capped at 5 attempts.
func (s *Server) APIClient() *api.Client {
	c := api.NewClient(s.Server.URL, "t", "")
	c.HTTP = s.Server.Client()
	c.MaxPollAttempts = 5
	c.Sleep = NoSleep
	return c
}

// NoSleep is an api.Client.Sleep that returns immediately, reporting
// ctx.Err() if the context is already done.
func NoSleep(ctx context.Context, _ time.Duration) error {
	return ctx.Err()
}
