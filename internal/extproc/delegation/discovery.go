// Package delegation resolves broker delegation context through the AuthZEN Access
// Evaluation API (ADR 045). It imports no broker domain, port, or adapter package.
package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
var ErrDiscoveryFailed = errors.New("delegation context discovery failed")

// DiscoveryURL inserts the AuthZEN well-known string between the host and the path of the
// PDP identifier (AuthZEN 1.0 PDP metadata access).
func DiscoveryURL(policyDecisionPoint string) (string, error) {
	pdp, err := url.Parse(policyDecisionPoint)
	if err != nil || !pdp.IsAbs() || pdp.Host == "" {
		return "", ErrDiscoveryFailed
	}
	path := strings.TrimRight(pdp.EscapedPath(), "/")
	return pdp.Scheme + "://" + pdp.Host + wellKnownPath + path, nil
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
		return nil, fmt.Errorf("%w: transport error", ErrDiscoveryFailed)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrDiscoveryFailed, resp.StatusCode)
	}
	var document struct {
		PolicyDecisionPoint      *string `json:"policy_decision_point"`
		AccessEvaluationEndpoint *string `json:"access_evaluation_endpoint"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxMetadataBytes)).Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrDiscoveryFailed)
	}
	if document.PolicyDecisionPoint == nil || document.AccessEvaluationEndpoint == nil {
		return nil, fmt.Errorf("%w: missing metadata members", ErrDiscoveryFailed)
	}
	if *document.PolicyDecisionPoint != expectedPDP {
		return nil, fmt.Errorf("delegation context discovery: policy_decision_point mismatch (expected %s, advertised %s)",
			expectedPDP, boundedValue(*document.PolicyDecisionPoint))
	}
	endpoint, err := url.Parse(*document.AccessEvaluationEndpoint)
	if err != nil || !endpoint.IsAbs() || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" ||
		(endpoint.Scheme != "https" && (endpoint.Scheme != "http" || !allowHTTP)) {
		return nil, errors.New("delegation context discovery: invalid access_evaluation_endpoint")
	}
	return &Metadata{PolicyDecisionPoint: expectedPDP, AccessEvaluationEndpoint: endpoint}, nil
}

func boundedValue(value string) string {
	const limit = 256
	if len(value) > limit {
		return value[:limit] + "…"
	}
	return value
}
