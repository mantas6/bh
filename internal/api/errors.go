package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ErrNoToken is returned when an API client is required but no token is
// configured. User-facing wording and hints live in cmdutil.
var ErrNoToken = errors.New("not logged in")

// maxErrorBodySize caps how much of an error response body is read.
const maxErrorBodySize = 64 << 10

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

// parseHTTPError reads at most maxErrorBodySize bytes of resp.Body (already
// known to be non-2xx) and builds a *HTTPError. Non-JSON bodies (e.g. an HTML
// gateway error page) are ignored in favour of the status text. The caller
// retains ownership of closing resp.Body.
func parseHTTPError(resp *http.Response, method, url string) *HTTPError {
	e := &HTTPError{
		StatusCode: resp.StatusCode,
		Method:     method,
		URL:        url,
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
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
// standard status text) with the status code, followed by the detail and a
// per-field summary when present. The request method and URL are
// intentionally omitted to keep the message readable.
func (e *HTTPError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (HTTP %d)", msg, e.StatusCode)
	if e.Detail != "" && e.Detail != msg {
		b.WriteString(": " + e.Detail)
	}
	if f := e.fieldsSummary(); f != "" {
		b.WriteString("; " + f)
	}
	return b.String()
}

// fieldsSummary renders Fields as "name: problem, name2: problem" sorted by
// field name.
func (e *HTTPError) fieldsSummary() string {
	if len(e.Fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+fieldValue(e.Fields[k]))
	}
	return strings.Join(parts, ", ")
}

// fieldValue flattens a field error value, which Bitbucket sends as a string
// or a list of strings.
func fieldValue(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []any:
		s := make([]string, 0, len(v))
		for _, x := range v {
			s = append(s, fieldValue(x))
		}
		return strings.Join(s, "; ")
	default:
		return fmt.Sprint(v)
	}
}

func hasStatus(err error, status int) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.StatusCode == status
}

// IsNotFound reports whether err is a 404 HTTPError.
func IsNotFound(err error) bool {
	return hasStatus(err, http.StatusNotFound)
}

// IsUnauthorized reports whether err is a 401 HTTPError, i.e. the
// credentials were missing or rejected. A 403 (valid credentials lacking
// permission) is not included; see IsForbidden.
func IsUnauthorized(err error) bool {
	return hasStatus(err, http.StatusUnauthorized)
}

// IsForbidden reports whether err is a 403 HTTPError, i.e. the credentials
// were accepted but lack permission (scope or repository access).
func IsForbidden(err error) bool {
	return hasStatus(err, http.StatusForbidden)
}
