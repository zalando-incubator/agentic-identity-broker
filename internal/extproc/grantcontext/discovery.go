// Package grantcontext resolves broker grant context through the AuthZEN Access
// Evaluation API (ADR 048). It imports no broker domain, port, or adapter package.
package grantcontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const wellKnownPath = "/.well-known/authzen-configuration"

const maxMetadataBytes = 64 << 10

// Metadata is the verified PDP metadata. It is immutable after discovery.
type Metadata struct {
	PolicyDecisionPoint      string
	AccessEvaluationEndpoint *url.URL
}

// ErrDiscoveryFailed reports unusable PDP metadata.
var ErrDiscoveryFailed = errors.New("grant context discovery failed")

// DiscoveryURL inserts the AuthZEN well-known string between the host and the path of the
// PDP identifier (AuthZEN 1.0 PDP metadata access).
func DiscoveryURL(policyDecisionPoint string) (string, error) {
	pdp, err := url.Parse(policyDecisionPoint)
	if err != nil || !pdp.IsAbs() || pdp.Host == "" {
		return "", ErrDiscoveryFailed
	}
	path := strings.TrimRight(pdp.EscapedPath(), "/")
	// Serialize through url.URL: Host is decoded, so an escaped IPv6 zone must be re-escaped.
	return (&url.URL{Scheme: pdp.Scheme, Host: pdp.Host}).String() + wellKnownPath + path, nil
}

// Discover fetches the PDP metadata without credentials, requires the advertised
// policy_decision_point to equal the expected identifier exactly, and validates the
// advertised evaluation endpoint before any credential is ever sent to it.
func Discover(ctx context.Context, client *http.Client, expectedPDP string, allowHTTP bool) (*Metadata, error) {
	discoveryURL, err := DiscoveryURL(expectedPDP)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, ErrDiscoveryFailed
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, discoveryTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrDiscoveryFailed, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes+1))
	if err != nil {
		return nil, discoveryTransportError(err)
	}
	if len(raw) > maxMetadataBytes {
		return nil, fmt.Errorf("%w: metadata too large", ErrDiscoveryFailed)
	}
	var document struct {
		PolicyDecisionPoint      *string `json:"policy_decision_point"`
		AccessEvaluationEndpoint *string `json:"access_evaluation_endpoint"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrDiscoveryFailed)
	}
	if document.PolicyDecisionPoint == nil || document.AccessEvaluationEndpoint == nil {
		return nil, fmt.Errorf("%w: missing metadata members", ErrDiscoveryFailed)
	}
	if *document.PolicyDecisionPoint != expectedPDP {
		return nil, fmt.Errorf("grant context discovery: policy_decision_point mismatch (expected %s, advertised %s)",
			expectedPDP, boundedValue(*document.PolicyDecisionPoint))
	}
	endpoint, err := url.Parse(*document.AccessEvaluationEndpoint)
	if err != nil || !endpoint.IsAbs() || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" ||
		(endpoint.Scheme != "https" && (endpoint.Scheme != "http" || !allowHTTP)) {
		return nil, errors.New("grant context discovery: invalid access_evaluation_endpoint")
	}
	return &Metadata{PolicyDecisionPoint: expectedPDP, AccessEvaluationEndpoint: endpoint}, nil
}

// discoveryTransportError keeps caller cancellation typed so startup can treat a shutdown
// signal as a normal exit, without exposing the raw transport error.
func discoveryTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", ErrDiscoveryFailed, context.Canceled)
	}
	return fmt.Errorf("%w: transport error", ErrDiscoveryFailed)
}

// boundedValue truncates a remote value and escapes control characters, so a hostile
// discovery document cannot inject log lines through the startup error.
func boundedValue(value string) string {
	const limit = 256
	suffix := ""
	if len(value) > limit {
		value, suffix = value[:limit], "…"
	}
	quoted := strconv.Quote(value)
	return quoted[1:len(quoted)-1] + suffix
}
