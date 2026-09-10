// Package apitest provides an httptest-backed fake Bitbucket API server and a
// preconfigured api.Client for use in command and client tests. It records
// every request (method, path, query, body) for later assertions.
package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/mantas6/bh/internal/api"
)

// Request is a recorded inbound request.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
}

// Server wraps an httptest.Server with request recording and a route mux.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	Requests []Request

	mux *http.ServeMux
	t   *testing.T
}

// New starts a recording server. It is automatically closed via t.Cleanup.
func New(t *testing.T) *Server {
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
	r.Body.Close()

	s.mu.Lock()
	s.Requests = append(s.Requests, Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Body:   body,
	})
	s.mu.Unlock()

	h, pattern := s.mux.Handler(r)
	if pattern == "" {
		s.t.Errorf("apitest: unexpected request %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"message":"no such route"}}`)
		return
	}
	h.ServeHTTP(w, r)
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
			io.WriteString(w, v)
		case []byte:
			w.Write(v)
		default:
			_ = json.NewEncoder(w).Encode(v)
		}
	})
}

// HandleFunc registers a custom handler for a method+path.
func (s *Server) HandleFunc(method, path string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern(method, path), h)
}

// Client returns an api.Client pointed at this server, authenticated with a
// dummy Bearer token and configured for fast (non-blocking) merge polling.
func (s *Server) Client() *api.Client {
	c := api.NewClient(s.Server.URL, "t", "")
	c.HTTP = s.Server.Client()
	c.PollInterval = 0
	c.MaxPollAttempts = 5
	return c
}
