// Package api implements a small Bitbucket Cloud REST 2.0 client: request
// dispatch with auth, retries of transient failures, JSON encoding/decoding,
// pagination over `next` links, typed error mapping, and resource methods for
// users, workspaces, repositories, pull requests and comments.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultHost is the canonical Bitbucket Cloud host.
	DefaultHost = "bitbucket.org"
	// DefaultBaseURL is the base URL for the Bitbucket Cloud REST 2.0 API.
	DefaultBaseURL = "https://api.bitbucket.org/2.0"

	defaultUserAgent    = "bh"
	defaultTimeout      = 30 * time.Second
	defaultPollInterval = time.Second

	// maxRetries is how many times a request that failed with a transient
	// status is re-sent (so at most maxRetries+1 attempts).
	maxRetries = 3
	// retryBaseDelay is the first backoff delay; it doubles per retry.
	retryBaseDelay = 500 * time.Millisecond
	// maxRetryDelay caps a single backoff wait. A Retry-After asking for
	// longer than this is not honoured; the error is returned instead.
	maxRetryDelay = 30 * time.Second
)

// defaultHTTPClient is used when Client.HTTP is nil. Unlike
// http.DefaultClient it has a timeout so a hung server cannot block forever.
var defaultHTTPClient = &http.Client{Timeout: defaultTimeout}

// Client is a Bitbucket Cloud REST 2.0 client. The zero value is usable: it
// talks to DefaultBaseURL anonymously with a default HTTP client.
type Client struct {
	// BaseURL is the API root, e.g. https://api.bitbucket.org/2.0. Empty
	// means DefaultBaseURL.
	BaseURL string
	// HTTP is the underlying HTTP client. Nil means a default client with a
	// 30s timeout.
	HTTP *http.Client
	// Token is the API token. Empty means anonymous requests. It is only
	// sent to the BaseURL host.
	Token string
	// Email, when set, selects Basic auth (email:token) instead of Bearer.
	Email string
	// UserAgent is sent as the User-Agent header. Empty means "bh".
	UserAgent string

	// PollInterval is the delay between merge task-status polls. Zero means
	// one second.
	PollInterval time.Duration
	// MaxPollAttempts caps merge task-status polling. Zero means
	// maxPollAttempts.
	MaxPollAttempts int

	// Sleep waits for d or until ctx is done, returning ctx.Err() in the
	// latter case. It is used for retry backoff and merge polling. Nil
	// means a real timer; tests inject a no-op.
	Sleep func(ctx context.Context, d time.Duration) error
}

// NewClient builds a Client. An empty baseURL defaults to
// DefaultBaseURL. token/email may be empty for anonymous use.
func NewClient(baseURL, token, email string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		HTTP:      &http.Client{Timeout: defaultTimeout},
		Token:     token,
		Email:     email,
		UserAgent: defaultUserAgent,
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTPClient
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) userAgent() string {
	if c.UserAgent == "" {
		return defaultUserAgent
	}
	return c.UserAgent
}

func (c *Client) pollInterval() time.Duration {
	if c.PollInterval <= 0 {
		return defaultPollInterval
	}
	return c.PollInterval
}

func (c *Client) maxPollAttempts() int {
	if c.MaxPollAttempts <= 0 {
		return maxPollAttempts
	}
	return c.MaxPollAttempts
}

// sleep waits for d via c.Sleep, or a timer when unset.
func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// resolveURL joins path with BaseURL unless path is already absolute.
func (c *Client) resolveURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL() + path
}

// sameHost reports whether u points at the BaseURL host, i.e. whether it is
// safe to attach credentials to a request for u.
func (c *Client) sameHost(u *url.URL) bool {
	base, err := url.Parse(c.baseURL())
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Host, u.Host)
}

func (c *Client) setAuth(req *http.Request) {
	if c.Token == "" || !c.sameHost(req.URL) {
		return
	}
	if c.Email != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(c.Email + ":" + c.Token))
		req.Header.Set("Authorization", "Basic "+creds)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
}

// Do issues an HTTP request. path may be absolute (for `next` links) or
// relative to BaseURL. body, when non-nil, is JSON-encoded. When out is
// non-nil and the response is a 2xx with a body, the body is JSON-decoded into
// out. Non-2xx responses yield a *HTTPError.
//
// Idempotent requests (GET, HEAD, PUT, DELETE, OPTIONS) that fail with 429,
// 502, 503 or 504 are retried with capped exponential backoff, honouring
// Retry-After.
//
// The returned status and header describe the final response; they are zero
// when no response was received.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, out any) (int, http.Header, error) {
	u := c.resolveURL(path)
	if len(query) > 0 {
		if strings.Contains(u, "?") {
			u += "&" + query.Encode()
		} else {
			u += "?" + query.Encode()
		}
	}

	var payload []byte
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("encoding request body: %w", err)
		}
		payload = buf
	}

	for attempt := 0; ; attempt++ {
		status, header, err := c.do(ctx, method, u, payload, out)
		if err == nil || attempt >= maxRetries || !retryable(method, status) {
			return status, header, err
		}
		delay, ok := retryDelay(attempt, header, time.Now())
		if !ok {
			return status, header, err
		}
		if serr := c.sleep(ctx, delay); serr != nil {
			return status, header, serr
		}
	}
}

// do performs a single request attempt against the absolute URL u.
func (c *Client) do(ctx context.Context, method, u string, payload []byte, out any) (int, http.Header, error) {
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.userAgent())
	c.setAuth(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header, parseHTTPError(resp, method, u)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, resp.Header, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, fmt.Errorf("reading response body: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return resp.StatusCode, resp.Header, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return resp.StatusCode, resp.Header, fmt.Errorf("decoding response body: %w", err)
	}
	return resp.StatusCode, resp.Header, nil
}

// retryable reports whether a request with method that failed with status
// may be safely re-sent.
func retryable(method string, status int) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
	default:
		return false
	}
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay returns how long to wait before retry number attempt (0-based).
// A Retry-After header (delay-seconds or HTTP-date) takes precedence over
// exponential backoff. ok is false when the server asks for a wait longer
// than maxRetryDelay, in which case the caller should give up.
func retryDelay(attempt int, header http.Header, now time.Time) (d time.Duration, ok bool) {
	if ra := strings.TrimSpace(header.Get("Retry-After")); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			d = time.Duration(max(secs, 0)) * time.Second
		} else if t, err := http.ParseTime(ra); err == nil {
			d = max(t.Sub(now), 0)
		} else {
			d = backoff(attempt)
		}
		return d, d <= maxRetryDelay
	}
	return backoff(attempt), true
}

// backoff is retryBaseDelay * 2^attempt, capped at maxRetryDelay.
func backoff(attempt int) time.Duration {
	d := retryBaseDelay
	for range attempt {
		d *= 2
		if d >= maxRetryDelay {
			return maxRetryDelay
		}
	}
	return d
}

// page is the shape of a Bitbucket paginated collection.
type page struct {
	Values []json.RawMessage `json:"values"`
	Next   string            `json:"next"`
}

// Paginate issues GET requests starting at path, invoking each for every raw
// value, following `next` links until exhausted or limit items have been
// yielded. limit <= 0 means unlimited. pagelen defaults to min(limit,50)/50.
// query is only sent with the first request: `next` links already carry it.
func (c *Client) Paginate(ctx context.Context, path string, query url.Values, limit int, each func(raw json.RawMessage) error) error {
	// Copy so we don't mutate the caller's values.
	q := url.Values{}
	for k, v := range query {
		q[k] = append([]string(nil), v...)
	}
	query = q
	if query.Get("pagelen") == "" {
		pagelen := 50
		if limit > 0 && limit < pagelen {
			pagelen = limit
		}
		query.Set("pagelen", strconv.Itoa(pagelen))
	}

	count := 0
	seen := map[string]bool{}
	next := path
	for next != "" {
		var p page
		if _, _, err := c.Do(ctx, http.MethodGet, next, query, nil, &p); err != nil {
			return err
		}
		query = nil
		seen[next] = true
		for _, raw := range p.Values {
			if err := each(raw); err != nil {
				return err
			}
			count++
			if limit > 0 && count >= limit {
				return nil
			}
		}
		if p.Next != "" && seen[p.Next] {
			return fmt.Errorf("pagination loop: next link %s was already fetched", p.Next)
		}
		next = p.Next
	}
	return nil
}

// PaginateAll collects up to limit decoded values of type T from a paginated
// endpoint. limit <= 0 means unlimited. An empty collection yields a non-nil
// empty slice so it encodes as `[]` rather than `null`.
func PaginateAll[T any](ctx context.Context, c *Client, path string, query url.Values, limit int) ([]T, error) {
	out := []T{}
	err := c.Paginate(ctx, path, query, limit, func(raw json.RawMessage) error {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("decoding page value: %w", err)
		}
		out = append(out, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// repoPath returns the escaped /repositories/{workspace}/{repo} path for
// fullName ("ws/repo").
func repoPath(fullName string) string {
	ws, name, ok := strings.Cut(fullName, "/")
	if !ok {
		return "/repositories/" + url.PathEscape(fullName)
	}
	return "/repositories/" + url.PathEscape(ws) + "/" + url.PathEscape(name)
}
