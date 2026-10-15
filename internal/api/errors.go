package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
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
	// RequiredScopes and GrantedScopes are set when Bitbucket rejects a
	// request because the token lacks a scope; its body then carries
	// {"detail":{"required":[...],"granted":[...]}}.
	RequiredScopes []string
	GrantedScopes  []string
}

// errorEnvelope matches Bitbucket's error body:
// {"error":{"message":..., "detail":..., "fields":...}}. Each member is kept
// raw and decoded leniently, because "detail" is a string for most errors
// but an object for scope errors, and one odd member must not lose the rest.
type errorEnvelope struct {
	Error struct {
		Message json.RawMessage `json:"message"`
		Detail  json.RawMessage `json:"detail"`
		Fields  json.RawMessage `json:"fields"`
	} `json:"error"`
}

// scopeDetail is the "detail" object Bitbucket sends for scope errors.
type scopeDetail struct {
	Required []string `json:"required"`
	Granted  []string `json:"granted"`
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
			_ = json.Unmarshal(env.Error.Message, &e.Message)
			_ = json.Unmarshal(env.Error.Fields, &e.Fields)
			e.decodeDetail(env.Error.Detail)
		}
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	return e
}

// decodeDetail fills Detail, or RequiredScopes/GrantedScopes for a scope
// error, from the raw "detail" member. Any other non-string value is kept as
// compact JSON so it is not silently dropped.
func (e *HTTPError) decodeDetail(raw json.RawMessage) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return
	}
	if json.Unmarshal(raw, &e.Detail) == nil {
		return
	}
	var sd scopeDetail
	if json.Unmarshal(raw, &sd) == nil && (len(sd.Required) > 0 || len(sd.Granted) > 0) {
		e.RequiredScopes = sd.Required
		e.GrantedScopes = sd.Granted
		return
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) == nil {
		e.Detail = buf.String()
	}
}

// MissingScopes returns the required scopes that were not granted, in the
// order Bitbucket listed them. It is empty unless the error was a scope
// error.
func (e *HTTPError) MissingScopes() []string {
	var missing []string
	for _, s := range e.RequiredScopes {
		if !slices.Contains(e.GrantedScopes, s) {
			missing = append(missing, s)
		}
	}
	return missing
}

// scopeSummary renders "required scopes: a, b; granted: c" (with "none" for
// an empty grant), or "" when the error carries no scope information.
func (e *HTTPError) scopeSummary() string {
	if len(e.RequiredScopes) == 0 && len(e.GrantedScopes) == 0 {
		return ""
	}
	list := func(s []string) string {
		if len(s) == 0 {
			return "none"
		}
		return strings.Join(s, ", ")
	}
	return "required scopes: " + list(e.RequiredScopes) + "; granted: " + list(e.GrantedScopes)
}

// Error implements error. It renders the server's message (falling back to the
// standard status text) with the status code, followed by the detail, the
// scope summary and a per-field summary when present. The request method and
// URL are intentionally omitted to keep the message readable.
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
	if s := e.scopeSummary(); s != "" {
		b.WriteString(": " + s)
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

// IsBadRequest reports whether err is a 400 HTTPError, e.g. an invalid BBQL
// filter query.
func IsBadRequest(err error) bool {
	return hasStatus(err, http.StatusBadRequest)
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
