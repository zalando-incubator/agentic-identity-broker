// Package cimd implements the SSRF-hardened HTTP fetcher for Client ID Metadata Documents.
package cimd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	domaincimd "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Ensure Fetcher implements ports.CIMDFetcher at compile time.
var _ ports.CIMDFetcher = (*Fetcher)(nil)

// Fetcher implements ports.CIMDFetcher with SSRF protection, timeout, and size limits.
type Fetcher struct {
	client           *http.Client
	blocklist        domaincimd.SSRFBlocklist
	maxResponseBytes int64
}

// NewFetcher creates a new SSRF-hardened CIMD fetcher.
// fetchTimeout is the maximum time to wait for a response.
// maxResponseBytes is the maximum allowed response body size.
// extraBlockedCIDRs are operator-configured additional blocked CIDR ranges.
func NewFetcher(fetchTimeout time.Duration, maxResponseBytes int64, extraBlockedCIDRs []string) (*Fetcher, error) {
	blocklist, err := domaincimd.NewSSRFBlocklist(extraBlockedCIDRs)
	if err != nil {
		return nil, fmt.Errorf("building SSRF blocklist: %w", err)
	}

	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("loading system certificate pool: %w", err)
	}

	dialer := &net.Dialer{
		Timeout:   fetchTimeout,
		KeepAlive: -1, // no keepalive for one-shot fetches
		Control:   buildSSRFControl(blocklist),
	}

	transport := &http.Transport{
		DialContext:       dialer.DialContext,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: certPool},
		MaxConnsPerHost:   1,
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   fetchTimeout,
	}

	return NewFetcherWithClient(client, blocklist, maxResponseBytes), nil
}

// NewFetcherInsecure creates a fetcher with SSRF protection and TLS verification disabled.
// For development/test environments only — allows fetching from private IPs and
// self-signed certificates. NEVER use in production.
func NewFetcherInsecure(fetchTimeout time.Duration, maxResponseBytes int64) (*Fetcher, error) {
	// Empty blocklist: allow all IPs including RFC 1918 ranges.
	blocklist := domaincimd.SSRFBlocklist{}

	dialer := &net.Dialer{
		Timeout:   fetchTimeout,
		KeepAlive: -1,
	}

	transport := &http.Transport{
		DialContext:       dialer.DialContext,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, // #nosec G402 -- insecure constructor is restricted to development and test use.
		MaxConnsPerHost:   1,
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   fetchTimeout,
	}

	return NewFetcherWithClient(client, blocklist, maxResponseBytes), nil
}

// NewFetcherWithClient creates a Fetcher with an injected HTTP client, for testing.
// The no-redirect policy is always enforced regardless of the client's CheckRedirect setting.
func NewFetcherWithClient(client *http.Client, blocklist domaincimd.SSRFBlocklist, maxResponseBytes int64) *Fetcher {
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Fetcher{
		client:           client,
		blocklist:        blocklist,
		maxResponseBytes: maxResponseBytes,
	}
}

// WrapTransport replaces the fetcher's HTTP transport with wrap(existingTransport).
// Called at builder time to add instrumentation around the base SSRF-hardened transport.
func (f *Fetcher) WrapTransport(wrap func(http.RoundTripper) http.RoundTripper) {
	base := f.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	f.client.Transport = wrap(base)
}

// Fetch fetches and validates a CIMD document from the given URL.
// Returns an error if the URL is blocked, the response is non-200, the body
// exceeds maxResponseBytes, or the document fails validation.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*ports.CIMDFetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building CIMD request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching CIMD document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CIMD endpoint returned status %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		return nil, fmt.Errorf("CIMD endpoint returned non-JSON Content-Type: %q", ct)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading CIMD response body: %w", err)
	}
	if int64(len(body)) > f.maxResponseBytes {
		return nil, fmt.Errorf("CIMD response exceeds %d byte limit", f.maxResponseBytes)
	}

	return &ports.CIMDFetchResult{
		Body:         body,
		CacheControl: resp.Header.Get("Cache-Control"),
		Expires:      resp.Header.Get("Expires"),
	}, nil
}

// buildSSRFControl returns a net.Dialer.Control function that rejects connections
// to any IP address in the blocklist. The control callback fires after DNS
// resolution and before TCP connect, so blocked addresses never receive a packet.
func buildSSRFControl(blocklist domaincimd.SSRFBlocklist) func(string, string, syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("parsing resolved address %q: %w", address, err)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("resolved address %q is not a valid IP", host)
		}
		if blocklist.Contains(ip) {
			return &ports.SSRFBlockedError{IP: ip.String()}
		}
		return nil
	}
}
