package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrNoToken is returned when an API client is required but no token is
// configured.
var ErrNoToken = errors.New("not logged in; run `bh auth login` or set BH_TOKEN")

// HTTPError describes a non-2xx API response.
type HTTPError struct {
	StatusCode int
	Method     string
	URL        string
	Message    string
	Detail     string
	Fields     map[string]any
}

// errorEnvelope matches Bitbucket's error body:
// {"error":{"message":..., "detail":..., "fields":...}}.
type errorEnvelope struct {
	Error struct {
		Message string         `json:"message"`
		Detail  string         `json:"detail"`
		Fields  map[string]any `json:"fields"`
	} `json:"error"`
}

// parseHTTPError reads resp.Body (already known to be non-2xx) and builds a
// *HTTPError. The caller retains ownership of closing resp.Body.
func parseHTTPError(resp *http.Response, method, url string) *HTTPError {
	e := &HTTPError{
		StatusCode: resp.StatusCode,
		Method:     method,
		URL:        url,
	}
	data, _ := io.ReadAll(resp.Body)
	if len(data) > 0 {
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err == nil {
			e.Message = env.Error.Message
			e.Detail = env.Error.Detail
			e.Fields = env.Error.Fields
		}
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	return e
}

// Error implements error. It renders the server's message (falling back to the
// standard status text) with the status code appended, plus an optional hint
// on a second line. The request method and URL are intentionally omitted to
// keep the message readable.
func (e *HTTPError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	s := fmt.Sprintf("%s (HTTP %d)", msg, e.StatusCode)
	if h := e.Hint(); h != "" {
		s += "\n" + h
	}
	return s
}

// Hint returns an actionable suggestion for common status codes, or "".
func (e *HTTPError) Hint() string {
	switch e.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "To authenticate, run: bh auth login"
	default:
		return ""
	}
}

// IsNotFound reports whether err is a 404 HTTPError.
func IsNotFound(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.StatusCode == http.StatusNotFound
	}
	return false
}

// IsUnauthorized reports whether err is a 401/403 HTTPError.
func IsUnauthorized(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.StatusCode == http.StatusUnauthorized || he.StatusCode == http.StatusForbidden
	}
	return false
}
