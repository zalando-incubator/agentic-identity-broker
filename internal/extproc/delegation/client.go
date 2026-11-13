package delegation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	subjectTypeJWT         = "urn:ietf:params:oauth:token-type:jwt" // #nosec G101 -- RFC-defined token type URI, not a credential.
	actionTokenExchange    = "urn:ietf:params:oauth:grant-type:token-exchange"
	resourceTypeProtected  = "protected_resource"
	maxEvaluationBodyBytes = 1 << 20
)

// Error categories for a failed resolution. Every category fails closed with 503.
const (
	CategoryTimeout         = "timeout"
	CategoryUnavailable     = "unavailable"
	CategoryRejected        = "rejected"
	CategoryInvalidResponse = "invalid_response"
)

// Error is a credential-free resolution failure.
type Error struct {
	Category string
	cause    error
}

func (e *Error) Error() string { return "delegation context unavailable: " + e.Category }
func (e *Error) Unwrap() error { return e.cause }

func newError(category string, cause error) *Error { return &Error{Category: category, cause: cause} }

// DeniedError is a delegation denial from the broker. Code is an allowlisted grant detail
// or "delegation_denied"; ConsentURL is an absolute http(s) URL.
type DeniedError struct {
	Code       string
	ConsentURL string
}

func (e *DeniedError) Error() string { return "delegation denied: " + e.Code }

var allowlistedDenialCodes = map[string]bool{
	"grant_missing":                      true,
	"grant_expired":                      true,
	"grant_empty":                        true,
	"grant_permission_set_undeclared":    true,
	"grant_permission_set_missing":       true,
	"grant_service_omitted":              true,
	"grant_service_definition_missing":   true,
	"grant_service_requirement_excluded": true,
	"grant_scope_intersection_empty":     true,
}

// Context is the typed, validated permit context. Timestamps keep the received RFC 3339
// strings. Optional members are empty when the broker omitted them.
type Context struct {
	Principal      string
	Agent          EntityRef
	Resource       Resource
	Grant          Grant
	PermissionSets []PermissionSet
}

type EntityRef struct {
	ID          string
	CanonicalID string
}

type Resource struct {
	ID          string
	CanonicalID string
	ResourceURI string
	Scopes      []string
}

type Grant struct {
	ID         string
	UpdatedAt  string
	ValidUntil string
}

type PermissionSet struct {
	ID          string
	CanonicalID string
	Services    []Service
}

type Service struct {
	ID          string
	CanonicalID string
	Scopes      []string
}

// Result is an immutable permit usable for policy until Deadline. Consumers copy slices.
type Result struct {
	Context  Context
	Deadline time.Time
}

// AssertionProvider supplies the current client assertion.
type AssertionProvider interface {
	ClientAssertion() (string, error)
}

// Client calls the advertised AuthZEN Access Evaluation endpoint.
type Client struct {
	httpClient   *http.Client
	endpoint     string
	assertions   AssertionProvider
	maxStaleness time.Duration
	now          func() time.Time
}

// NewClient creates an evaluation client for verified metadata.
func NewClient(httpClient *http.Client, metadata *Metadata, assertions AssertionProvider, maxStaleness time.Duration) (*Client, error) {
	if httpClient == nil || metadata == nil || metadata.AccessEvaluationEndpoint == nil || assertions == nil {
		return nil, errors.New("delegation client requires an HTTP client, verified metadata and an assertion provider")
	}
	if maxStaleness <= 0 {
		return nil, errors.New("delegation client requires a positive max staleness")
	}
	return &Client{httpClient: httpClient, endpoint: metadata.AccessEvaluationEndpoint.String(), assertions: assertions,
		maxStaleness: maxStaleness, now: time.Now}, nil
}

type evaluationRequest struct {
	Subject  evaluationEntity `json:"subject"`
	Action   evaluationAction `json:"action"`
	Resource evaluationEntity `json:"resource"`
}

type evaluationEntity struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type evaluationAction struct {
	Name string `json:"name"`
}

// Evaluate asks the broker for the delegation of subjectToken to resourceURI. A permit's
// deadline is computed once from the clock read immediately before the request is sent.
func (c *Client) Evaluate(ctx context.Context, subjectToken, resourceURI string) (Result, error) {
	assertion, err := c.assertions.ClientAssertion()
	if err != nil {
		return Result{}, newError(CategoryUnavailable, err)
	}
	body, err := json.Marshal(evaluationRequest{
		Subject:  evaluationEntity{Type: subjectTypeJWT, ID: subjectToken},
		Action:   evaluationAction{Name: actionTokenExchange},
		Resource: evaluationEntity{Type: resourceTypeProtected, ID: resourceURI},
	})
	if err != nil {
		return Result{}, newError(CategoryInvalidResponse, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, newError(CategoryUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+assertion)

	requestStart := c.now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return Result{}, newError(CategoryTimeout, nil)
		}
		return Result{}, newError(CategoryUnavailable, nil)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Result{}, newError(CategoryRejected, fmt.Errorf("status %d", resp.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxEvaluationBodyBytes+1))
	if err != nil || len(raw) > maxEvaluationBodyBytes {
		return Result{}, newError(CategoryInvalidResponse, nil)
	}
	var decision struct {
		Decision *bool           `json:"decision"`
		Context  json.RawMessage `json:"context"`
	}
	if err := json.Unmarshal(raw, &decision); err != nil || decision.Decision == nil {
		return Result{}, newError(CategoryInvalidResponse, nil)
	}
	if !*decision.Decision {
		return Result{}, parseDenial(decision.Context)
	}
	maxAge, ok := parseMaxAge(resp.Header.Values("Cache-Control"))
	if !ok {
		return Result{}, newError(CategoryInvalidResponse, errors.New("unusable max-age"))
	}
	permit, err := parsePermitContext(decision.Context)
	if err != nil {
		return Result{}, newError(CategoryInvalidResponse, err)
	}
	return Result{Context: permit, Deadline: requestStart.Add(min(maxAge, c.maxStaleness))}, nil
}

func isTimeout(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}

// parseMaxAge requires exactly one positive integer max-age directive across all
// Cache-Control values.
func parseMaxAge(values []string) (time.Duration, bool) {
	found := 0
	var seconds int64
	for _, value := range values {
		for _, directive := range strings.Split(value, ",") {
			name, arg, hasArg := strings.Cut(strings.TrimSpace(directive), "=")
			if !strings.EqualFold(strings.TrimSpace(name), "max-age") {
				continue
			}
			found++
			arg = strings.TrimSpace(arg)
			if !hasArg || arg == "" || strings.TrimLeft(arg, "0123456789") != "" {
				return 0, false
			}
			parsed, err := strconv.ParseInt(arg, 10, 32)
			if err != nil {
				return 0, false
			}
			seconds = parsed
		}
	}
	if found != 1 || seconds <= 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func parseDenial(raw json.RawMessage) error {
	var denial struct {
		Context *struct {
			ReasonAdmin *struct {
				Code string `json:"code"`
			} `json:"reason_admin"`
			ReasonUser *struct {
				ConsentURL string `json:"consent_url"`
			} `json:"reason_user"`
		}
	}
	if err := json.Unmarshal([]byte(`{"context":`+orNull(raw)+`}`), &denial); err != nil ||
		denial.Context == nil || denial.Context.ReasonUser == nil {
		return newError(CategoryInvalidResponse, nil)
	}
	consentURL, err := url.Parse(denial.Context.ReasonUser.ConsentURL)
	if err != nil || !consentURL.IsAbs() || consentURL.Host == "" || (consentURL.Scheme != "http" && consentURL.Scheme != "https") {
		return newError(CategoryInvalidResponse, nil)
	}
	code := "delegation_denied"
	if denial.Context.ReasonAdmin != nil && allowlistedDenialCodes[denial.Context.ReasonAdmin.Code] {
		code = denial.Context.ReasonAdmin.Code
	}
	return &DeniedError{Code: code, ConsentURL: consentURL.String()}
}

func orNull(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}
