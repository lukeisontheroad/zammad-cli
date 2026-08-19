// Package api is a minimal client for the Zammad REST API
// (https://docs.zammad.org/en/latest/api/intro.html).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
	Verbose bool
}

func New(rawURL, token string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid instance URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("instance URL must start with http:// or https://, got %q", rawURL)
	}
	return &Client{
		baseURL: u,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// BaseURL returns the instance base URL without a trailing slash.
func (c *Client) BaseURL() string { return c.baseURL.String() }

// Error is a Zammad API error response.
type Error struct {
	StatusCode int
	Message    string `json:"error"`
	Human      string `json:"error_human"`
}

func (e *Error) Error() string {
	msg := e.Human
	if msg == "" {
		msg = e.Message
	}
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("API error (HTTP %d): %s", e.StatusCode, msg)
}

// Do performs an API request. path is relative to the instance root (e.g.
// "/api/v1/tickets"). If body is non-nil it is JSON-encoded. If out is
// non-nil the response body is JSON-decoded into it.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token token="+c.token)
	req.Header.Set("User-Agent", "zammad-cli")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "> %s %s\n", method, u.String())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "< HTTP %d (%d bytes)\n", resp.StatusCode, len(data))
	}

	if resp.StatusCode >= 400 {
		apiErr := &Error{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(data, apiErr) // best effort; error text may not be JSON
		return apiErr
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Get is a convenience wrapper for GET requests.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.Do(ctx, http.MethodGet, path, query, nil, out)
}
