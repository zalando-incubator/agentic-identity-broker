package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AssertionProvider interface {
	ClientAssertion() (string, error)
}

type Client struct {
	baseURL        string
	http           *http.Client
	assertion      AssertionProvider
	requestTimeout time.Duration
}

type StatusError struct {
	Operation  string
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("approval %s returned HTTP %d", e.Operation, e.StatusCode)
}

func NewClient(baseURL string, timeout time.Duration, assertion AssertionProvider, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("approval HTTP client must not be nil")
	}
	if assertion == nil {
		return nil, fmt.Errorf("approval assertion provider must not be nil")
	}
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("approval broker URL must be an absolute HTTP(S) URL with a host and no query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return &Client{
		baseURL:        parsed.String(),
		http:           httpClient,
		assertion:      assertion,
		requestTimeout: timeout,
	}, nil
}

type syncResponse struct {
	Data *syncResponseData `json:"data"`
}

type syncResponseData struct {
	Pairs *[]syncResponsePair `json:"pairs"`
}

type syncResponsePair struct {
	Principal string   `json:"principal"`
	AgentID   string   `json:"agent_id"`
	Approvals []Record `json:"approvals"`
}

func (c *Client) Read(ctx context.Context, principal string, sessionIDs []string) ([]Pair, string, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	pairs, etag, unchanged, err := c.read(ctx, principal, sessionIDs, "", 0)
	if err != nil {
		return nil, "", err
	}
	if unchanged {
		return nil, "", &StatusError{Operation: "sync", StatusCode: http.StatusNotModified}
	}
	return pairs, etag, nil
}

func (c *Client) Poll(ctx context.Context, sessionIDs []string, etag string, longPollTimeout time.Duration) ([]Pair, string, bool, error) {
	return c.read(ctx, "", sessionIDs, etag, longPollTimeout)
}

func (c *Client) read(ctx context.Context, principal string, sessionIDs []string, etag string, longPollTimeout time.Duration) ([]Pair, string, bool, error) {
	assertion, err := c.clientAssertion()
	if err != nil {
		return nil, "", false, err
	}

	u, err := url.Parse(c.baseURL + "/api/approvals")
	if err != nil {
		return nil, "", false, fmt.Errorf("building approval sync URL: %w", err)
	}
	query := u.Query()
	if principal != "" {
		query.Set("principal", principal)
	}
	for _, sessionID := range sessionIDs {
		if sessionID != "" {
			query.Add("agent_session_id", sessionID)
		}
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", false, fmt.Errorf("building approval sync request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+assertion)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if longPollTimeout > 0 {
		req.Header.Set("X-Long-Poll-Timeout", fmt.Sprintf("%d", int(longPollTimeout/time.Second)))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("sending approval sync request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		if etag == "" {
			return nil, "", false, &StatusError{Operation: "sync", StatusCode: resp.StatusCode}
		}
		return nil, "", true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, &StatusError{Operation: "sync", StatusCode: resp.StatusCode}
	}
	responseETag := resp.Header.Get("ETag")
	if responseETag == "" {
		return nil, "", false, fmt.Errorf("approval sync response missing ETag")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", false, fmt.Errorf("reading approval sync response: %w", err)
	}
	var decoded syncResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, "", false, fmt.Errorf("decoding approval sync response: %w", err)
	}
	if decoded.Data == nil || decoded.Data.Pairs == nil {
		return nil, "", false, fmt.Errorf("approval sync response missing data.pairs")
	}

	pairs := make([]Pair, 0, len(*decoded.Data.Pairs))
	for _, pair := range *decoded.Data.Pairs {
		identity := Identity{Principal: pair.Principal, AgentID: pair.AgentID}
		if !identity.Valid() {
			return nil, "", false, fmt.Errorf("approval sync response contains pair without principal or agent_id")
		}
		pairs = append(pairs, Pair{Identity: identity, Approvals: pair.Approvals})
	}
	return pairs, responseETag, false, nil
}

func (c *Client) clientAssertion() (string, error) {
	assertion, err := c.assertion.ClientAssertion()
	if err != nil {
		return "", fmt.Errorf("getting client assertion: %w", err)
	}
	if strings.TrimSpace(assertion) == "" {
		return "", fmt.Errorf("getting client assertion: assertion is empty")
	}
	return assertion, nil
}

func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

type CreateRequest struct {
	Metadata struct {
		MCPSessionID   string `json:"mcp_session_id,omitempty"`
		AgentSessionID string `json:"agent_session_id,omitempty"`
		InvocationID   string `json:"tool_invocation_id,omitempty"`
		Description    string `json:"description"`
	} `json:"metadata"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
	RiskLevel string         `json:"risk_level,omitempty"`
}

func (c *Client) Create(ctx context.Context, subjectToken string, request CreateRequest) (string, error) {
	if strings.TrimSpace(subjectToken) == "" {
		return "", fmt.Errorf("approval subject token must not be empty")
	}
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	assertion, err := c.clientAssertion()
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encoding approval create request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/approvals", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building approval create request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+subjectToken)
	httpRequest.Header.Set("X-Client-Assertion", assertion)

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("sending approval create request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return "", &StatusError{Operation: "create", StatusCode: response.StatusCode}
	}

	var decoded struct {
		Data struct {
			ApprovalURL string `json:"approval_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decoding approval create response: %w", err)
	}
	if strings.TrimSpace(decoded.Data.ApprovalURL) == "" {
		return "", fmt.Errorf("approval create response missing approval_url")
	}
	return decoded.Data.ApprovalURL, nil
}

func (c *Client) Consume(ctx context.Context, subjectToken, approvalID string) error {
	if strings.TrimSpace(subjectToken) == "" {
		return fmt.Errorf("approval subject token must not be empty")
	}
	if approvalID == "" {
		return fmt.Errorf("approval ID must not be empty")
	}
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/approvals/"+url.PathEscape(approvalID)+"/consume", nil)
	if err != nil {
		return fmt.Errorf("building approval consume request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+subjectToken)
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sending approval consume request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return &StatusError{Operation: "consume", StatusCode: response.StatusCode}
	}
	return nil
}
