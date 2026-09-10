// Package api implements a small Bitbucket Cloud REST 2.0 client: request
// dispatch with auth, JSON encoding/decoding, pagination over `next` links,
// typed error mapping, and resource methods for users, workspaces,
// repositories, pull requests and comments.
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

	"github.com/mantas6/bh/internal/config"
)

// Client is a Bitbucket Cloud REST 2.0 client.
type Client struct {
	// BaseURL is the API root, e.g. https://api.bitbucket.org/2.0.
	BaseURL string
	// HTTP is the underlying HTTP client.
	HTTP *http.Client
	// Token is the API token. Empty means anonymous requests.
	Token string
	// Email, when set, selects Basic auth (email:token) instead of Bearer.
	Email string
	// UserAgent is sent as the User-Agent header.
	UserAgent string

	// PollInterval is the delay between merge task-status polls.
	PollInterval time.Duration
	// MaxPollAttempts caps merge task-status polling.
	MaxPollAttempts int
}

// NewClient builds a Client. An empty baseURL defaults to
// config.DefaultAPIBase. token/email may be empty for anonymous use.
func NewClient(baseURL, token, email string) *Client {
	if baseURL == "" {
		baseURL = config.DefaultAPIBase
	}
	return &Client{
		BaseURL:         strings.TrimRight(baseURL, "/"),
		HTTP:            &http.Client{Timeout: 30 * time.Second},
		Token:           token,
		Email:           email,
		UserAgent:       "bh",
		PollInterval:    time.Second,
		MaxPollAttempts: 60,
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// resolveURL joins path with BaseURL unless path is already absolute.
func (c *Client) resolveURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (c *Client) setAuth(req *http.Request) {
	if c.Token == "" {
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
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, out any) (*http.Response, error) {
	u := c.resolveURL(path)
	if len(query) > 0 {
		if strings.Contains(u, "?") {
			u += "&" + query.Encode()
		} else {
			u += "?" + query.Encode()
		}
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	c.setAuth(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return resp, parseHTTPError(resp, method, u)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		// Drain and close so the connection can be reused.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp, nil
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp, fmt.Errorf("reading response body: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return resp, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return resp, fmt.Errorf("decoding response body: %w", err)
	}
	return resp, nil
}

// page is the shape of a Bitbucket paginated collection.
type page struct {
	Values []json.RawMessage `json:"values"`
	Next   string            `json:"next"`
}

// Paginate issues GET requests starting at path, invoking each for every raw
// value, following `next` links until exhausted or limit items have been
// yielded. limit <= 0 means unlimited. pagelen defaults to min(limit,50)/50.
func (c *Client) Paginate(ctx context.Context, path string, query url.Values, limit int, each func(raw json.RawMessage) error) error {
	if query == nil {
		query = url.Values{}
	} else {
		// Copy so we don't mutate the caller's values.
		q := url.Values{}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		query = q
	}
	if query.Get("pagelen") == "" {
		pagelen := 50
		if limit > 0 && limit < pagelen {
			pagelen = limit
		}
		query.Set("pagelen", strconv.Itoa(pagelen))
	}

	count := 0
	next := path
	first := true
	for next != "" {
		var q url.Values
		if first {
			q = query
			first = false
		}
		var p page
		if _, err := c.Do(ctx, http.MethodGet, next, q, nil, &p); err != nil {
			return err
		}
		for _, raw := range p.Values {
			if err := each(raw); err != nil {
				return err
			}
			count++
			if limit > 0 && count >= limit {
				return nil
			}
		}
		next = p.Next
	}
	return nil
}

// PaginateAll collects up to limit decoded values of type T from a paginated
// endpoint. limit <= 0 means unlimited.
func PaginateAll[T any](ctx context.Context, c *Client, path string, query url.Values, limit int) ([]T, error) {
	var out []T
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
