// Package client is a thin HTTP client for the broker admin API. DTOs are
// declared from api/admin/openapi.yaml; nothing is imported from the broker.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	preferCanonical = "reference-id=canonical"
	userAgent       = "aibctl"
)

// Authenticator attaches credentials to a request.
type Authenticator interface {
	Apply(ctx context.Context, req *http.Request) error
}

type Client struct {
	BaseURL    string
	HTTP       *http.Client
	Auth       Authenticator
	HostHeader string
	Headers    map[string]string
	Verbose    io.Writer
	UserAgent  string
}

type request struct {
	method  string
	path    string
	query   url.Values
	body    any
	ifMatch string
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (c *Client) do(ctx context.Context, r request) (*response, error) {
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/") + r.path)
	if err != nil {
		return nil, fmt.Errorf("build request URL: %w", err)
	}
	if len(r.query) > 0 {
		u.RawQuery = r.query.Encode()
	}

	var payload io.Reader
	if r.body != nil {
		data, err := json.Marshal(r.body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		payload = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	ua := c.UserAgent
	if ua == "" {
		ua = userAgent
	}
	req.Header.Set("User-Agent", ua)
	switch r.method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		req.Header.Set("Content-Type", "application/json")
	case http.MethodGet:
		req.Header.Set("Prefer", preferCanonical)
	}
	if r.ifMatch != "" {
		req.Header.Set("If-Match", r.ifMatch)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	if c.HostHeader != "" {
		req.Host = c.HostHeader
	}
	if c.Auth != nil {
		if err := c.Auth.Apply(ctx, req); err != nil {
			return nil, err
		}
	}

	c.logf("> %s %s", req.Method, req.URL)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", r.method, u.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.logf("< %s", resp.Status)

	if resp.StatusCode >= 400 {
		return nil, newAPIError(resp)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return &response{status: resp.StatusCode, header: resp.Header, body: body}, nil
}

func (c *Client) logf(format string, args ...any) {
	if c.Verbose != nil {
		_, _ = fmt.Fprintf(c.Verbose, format+"\n", args...)
	}
}

func decode(resp *response, out any) error {
	if out == nil || len(resp.body) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) (*response, error) {
	resp, err := c.do(ctx, request{method: http.MethodGet, path: path, query: query})
	if err != nil {
		return nil, err
	}
	return resp, decode(resp, out)
}

func (c *Client) sendJSON(ctx context.Context, method, path string, body any, ifMatch string, out any) (*response, error) {
	resp, err := c.do(ctx, request{method: method, path: path, body: body, ifMatch: ifMatch})
	if err != nil {
		return nil, err
	}
	return resp, decode(resp, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	_, err := c.do(ctx, request{method: http.MethodDelete, path: path})
	return err
}

// Health checks the public health endpoint. It is the only unauthenticated call.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var out Health
	if _, err := c.getJSON(ctx, "/health", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func pathSegment(id string) string {
	return url.PathEscape(id)
}
