package outboundhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/netpolicy"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.OAuthDiscoveryClient = (*DiscoveryClient)(nil)

const discoveryBodyLimit = 256 << 10

// DiscoveryClient holds the outbound client for protected-resource discovery.
type DiscoveryClient struct {
	client *http.Client
}

// NewDiscoveryClientWithClient injects an HTTP client for the E2E TLS provider.
// Only the production constructor's transport enforces the dial-time IP policy;
// URL validation, no redirects, and response bounds apply to injected clients too.
func NewDiscoveryClientWithClient(client *http.Client) *DiscoveryClient {
	if client == nil {
		panic("outboundhttp: discovery client requires an HTTP client")
	}
	copy := *client
	copy.Jar = nil // Probes and discovery requests must not send stored cookies.
	copy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &DiscoveryClient{client: &copy}
}

// NewGuardedTransport constructs a proxy-free transport with dial-time IP checks.
// Builder uses it for discovery, registration, and discovery-backed token calls.
func NewGuardedTransport(extraBlockedCIDRs []string) (*http.Transport, error) {
	blocklist, err := netpolicy.NewSSRFBlocklist(extraBlockedCIDRs)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // A proxy would bypass the target-IP dial-time guard.
	transport.DialContext = (&net.Dialer{Control: buildSSRFControl(blocklist)}).DialContext
	config := &tls.Config{}
	if transport.TLSClientConfig != nil {
		config = transport.TLSClientConfig.Clone()
	}
	config.MinVersion = tls.VersionTLS12
	config.InsecureSkipVerify = false
	transport.TLSClientConfig = config
	return transport, nil
}

func validateDiscoveryURL(raw string) error {
	if err := model.ValidatePublicHTTPSURL(raw); err != nil {
		return ports.ErrOAuthDiscoveryUnsafeDestination
	}
	return nil
}

func discoveryRequestError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ports.ErrOAuthDiscoveryTimeout
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return ports.ErrOAuthDiscoveryTimeout
	}
	var blocked *ports.SSRFBlockedError
	if errors.As(err, &blocked) {
		return ports.ErrOAuthDiscoveryUnsafeDestination
	}
	return ports.ErrOAuthDiscoveryUnavailable // Do not expose URL queries or transport diagnostics.
}

func (c *DiscoveryClient) request(ctx context.Context, method, rawURL string, body []byte, acceptJSON bool) (*http.Response, error) {
	if err := validateDiscoveryURL(rawURL); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, discoveryRequestError(ctx, err)
	}
	var reader io.Reader
	if method == http.MethodPost {
		if len(body) > discoveryBodyLimit {
			return nil, ports.ErrOAuthDiscoveryResponseTooLarge
		}
		if !json.Valid(body) {
			return nil, ports.ErrOAuthDiscoveryInvalidResponse
		}
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, ports.ErrOAuthDiscoveryUnsafeDestination
	}
	if acceptJSON {
		req.Header.Set("Accept", "application/json")
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, discoveryRequestError(ctx, err)
	}
	return resp, nil
}

func rejectDiscoveryRedirect(resp *http.Response) error {
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return ports.ErrOAuthDiscoveryUnsafeDestination
	}
	return nil
}

func readDiscoveryBody(ctx context.Context, resp *http.Response) ([]byte, error) {
	if resp.ContentLength > discoveryBodyLimit {
		return nil, ports.ErrOAuthDiscoveryResponseTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, discoveryBodyLimit+1))
	if err != nil {
		return nil, discoveryRequestError(ctx, err)
	}
	if len(body) > discoveryBodyLimit {
		return nil, ports.ErrOAuthDiscoveryResponseTooLarge
	}
	return body, nil
}

func discardDiscoveryBody(ctx context.Context, resp *http.Response) error {
	if resp.ContentLength > discoveryBodyLimit {
		return ports.ErrOAuthDiscoveryResponseTooLarge
	}
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, discoveryBodyLimit+1))
	if err != nil {
		return discoveryRequestError(ctx, err)
	}
	if n > discoveryBodyLimit {
		return ports.ErrOAuthDiscoveryResponseTooLarge
	}
	return nil
}

func (c *DiscoveryClient) Probe(ctx context.Context, rawURL string) (int, []string, error) {
	resp, err := c.request(ctx, http.MethodGet, rawURL, nil, false)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := rejectDiscoveryRedirect(resp); err != nil {
		return 0, nil, err
	}
	if err := discardDiscoveryBody(ctx, resp); err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, resp.Header.Values("WWW-Authenticate"), nil
}

func (c *DiscoveryClient) GetJSON(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := c.request(ctx, http.MethodGet, rawURL, nil, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := rejectDiscoveryRedirect(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if err := discardDiscoveryBody(ctx, resp); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ports.ErrOAuthDiscoveryNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ports.ErrOAuthDiscoveryUnavailable
	}
	return readDiscoveryJSON(ctx, resp)
}

func (c *DiscoveryClient) PostJSON(ctx context.Context, rawURL string, body []byte) ([]byte, error) {
	resp, err := c.request(ctx, http.MethodPost, rawURL, body, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := rejectDiscoveryRedirect(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		if err := discardDiscoveryBody(ctx, resp); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, ports.ErrOAuthDiscoveryRejected
	}
	return readDiscoveryJSON(ctx, resp)
}

func readDiscoveryJSON(ctx context.Context, resp *http.Response) ([]byte, error) {
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, ports.ErrOAuthDiscoveryInvalidResponse
	}
	body, err := readDiscoveryBody(ctx, resp)
	if err != nil {
		return nil, err
	}
	if !json.Valid(body) {
		return nil, ports.ErrOAuthDiscoveryInvalidResponse
	}
	return body, nil
}
