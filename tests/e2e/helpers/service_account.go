package helpers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/onsi/gomega/types"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
)

// ServiceAccountCredentialSummary is the credential_summary member of a service account.
type ServiceAccountCredentialSummary struct {
	Format      string  `json:"format"`
	Use         string  `json:"use"`
	Iss         *string `json:"iss"`
	Sub         *string `json:"sub"`
	Kid         *string `json:"kid"`
	Exp         *int64  `json:"exp"`
	Fingerprint string  `json:"fingerprint"`
}

// ServiceAccount is the representation of a service account in the Admin API.
type ServiceAccount struct {
	ID                string                          `json:"id"`
	ServiceID         string                          `json:"service_id"`
	CanonicalID       string                          `json:"canonical_id"`
	Kind              string                          `json:"kind"`
	Scopes            []string                        `json:"scopes"`
	Narrowing         string                          `json:"narrowing"`
	CredentialSummary ServiceAccountCredentialSummary `json:"credential_summary"`
	CreatedAt         string                          `json:"created_at"`
	UpdatedAt         string                          `json:"updated_at"`
}

// ServiceAccountStatus is the body of the status operation.
type ServiceAccountStatus struct {
	Status              string  `json:"status"`
	FailureReason       *string `json:"failure_reason"`
	CredentialExpiresAt *string `json:"credential_expires_at"`
}

// ServiceAccountError is the Admin API error envelope.
type ServiceAccountError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// CapturedResponse is a fully read response whose body is already closed.
type CapturedResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Text renders the headers and the body, for absence assertions.
func (c CapturedResponse) Text() string {
	var text strings.Builder
	for name, values := range c.Header {
		text.WriteString(name + ": " + strings.Join(values, ", ") + "\n")
	}
	text.Write(c.Body)
	return text.String()
}

// Object decodes the body as one JSON object, keeping every member.
func (c CapturedResponse) Object() (map[string]any, error) {
	var object map[string]any
	err := json.Unmarshal(c.Body, &object)
	return object, err
}

// ServiceAccount decodes the body as one service account.
func (c CapturedResponse) ServiceAccount() (ServiceAccount, error) {
	var account ServiceAccount
	err := strictDecode(c.Body, &account)
	return account, err
}

// ServiceAccounts decodes the body as a list of service accounts.
func (c CapturedResponse) ServiceAccounts() ([]ServiceAccount, error) {
	var accounts []ServiceAccount
	err := strictDecode(c.Body, &accounts)
	return accounts, err
}

// Status decodes the body as a service account status.
func (c CapturedResponse) Status() (ServiceAccountStatus, error) {
	var status ServiceAccountStatus
	err := strictDecode(c.Body, &status)
	return status, err
}

// Error decodes the body as the error envelope.
func (c CapturedResponse) Error() (ServiceAccountError, error) {
	var envelope ServiceAccountError
	err := strictDecode(c.Body, &envelope)
	return envelope, err
}

func strictDecode(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// ServiceAccountsPath returns the collection path under a service identifier.
func ServiceAccountsPath(serviceRef string) string {
	return "/api/services/" + serviceRef + "/service-accounts"
}

// ServiceAccountPath returns the path of one account under a service identifier.
func ServiceAccountPath(serviceRef, accountRef string) string {
	return ServiceAccountsPath(serviceRef) + "/" + accountRef
}

// ServiceAccountRequest sends a JSON request to the admin server and reads the whole response.
// The body is omitted when nil; If-Match is sent only when ifMatch is not empty.
func ServiceAccountRequest(server *bootstrap.TestServer, method, path, principal, ifMatch string, body any) (CapturedResponse, error) {
	headers := map[string]string{}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return CapturedResponse{}, err
		}
		reader = bytes.NewReader(encoded)
		headers["Content-Type"] = "application/json"
	}
	if ifMatch != "" {
		headers["If-Match"] = ifMatch
	}
	response, err := server.DirectRequest(method, path, principal, headers, reader)
	if err != nil {
		return CapturedResponse{}, err
	}
	defer func() { _ = response.Body.Close() }()
	read, err := io.ReadAll(response.Body)
	if err != nil {
		return CapturedResponse{}, err
	}
	return CapturedResponse{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: read}, nil
}

// ServiceAccountFingerprint returns the fingerprint the broker reports for a stored JWT.
func ServiceAccountFingerprint(jwt string) string {
	digest := sha256.Sum256([]byte(jwt))
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
}

// ContainJWTMaterial succeeds when the actual string or byte slice contains the JWT or any of its
// non-empty dot-separated segments. Use it negated on every response that follows a submission.
func ContainJWTMaterial(jwt string) types.GomegaMatcher {
	return &jwtMaterialMatcher{jwt: jwt}
}

type jwtMaterialMatcher struct {
	jwt   string
	found string
}

func (m *jwtMaterialMatcher) Match(actual any) (bool, error) {
	var text string
	switch value := actual.(type) {
	case string:
		text = value
	case []byte:
		text = string(value)
	case CapturedResponse:
		text = value.Text()
	default:
		return false, fmt.Errorf("ContainJWTMaterial expects a string, []byte or CapturedResponse, got %T", actual)
	}
	candidates := append([]string{strings.TrimSpace(m.jwt)}, strings.Split(strings.TrimSpace(m.jwt), ".")...)
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(text, candidate) {
			m.found = candidate
			return true, nil
		}
	}
	return false, nil
}

func (m *jwtMaterialMatcher) FailureMessage(any) string {
	return "expected the text to contain the JWT or one of its segments"
}

func (m *jwtMaterialMatcher) NegatedFailureMessage(any) string {
	return fmt.Sprintf("expected the text not to contain the JWT or any of its segments, but it contains a %d-byte part of it", len(m.found))
}

// ServiceAccountTokenExp returns the exp claim of a JWT the tests created, without verification.
func ServiceAccountTokenExp(jwt string) (int64, error) {
	segments := strings.Split(jwt, ".")
	if len(segments) != 3 {
		return 0, fmt.Errorf("expected three segments, got %d", len(segments))
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return 0, err
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, err
	}
	return claims.Exp, nil
}
