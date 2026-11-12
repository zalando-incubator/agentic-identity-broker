// Package oauth2 implements OAuth2 authorization server proxy functionality.
package oauth2

import (
	"fmt"
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
)

// OAuth2Error represents an OAuth2 error (RFC 6749 Section 5.2).
type OAuth2Error struct {
	Code        string
	Description string
	URI         string
}

// Error implements the error interface.
func (e *OAuth2Error) Error() string {
	return fmt.Sprintf("oauth2_error: %s", e.Code)
}

// NewOAuth2Error creates a new OAuth2Error with the given code and description.
func NewOAuth2Error(code, description string) *OAuth2Error {
	return &OAuth2Error{
		Code:        code,
		Description: description,
	}
}

// InvalidClientError returns an OAuth2 error for invalid client_id.
func InvalidClientError(description string) *OAuth2Error {
	return &OAuth2Error{
		Code:        "invalid_client",
		Description: description,
	}
}

// ServerError returns an OAuth2 error for server errors.
func ServerError(description string) *OAuth2Error {
	return &OAuth2Error{
		Code:        "server_error",
		Description: description,
	}
}

// TemporarilyUnavailableError returns an OAuth2 error when server is unavailable.
func TemporarilyUnavailableError(description string) *OAuth2Error {
	return &OAuth2Error{
		Code:        "temporarily_unavailable",
		Description: description,
	}
}

// BuildErrorRedirectURL constructs an OAuth2 error redirect URL per RFC 6749 §4.1.2.1.
// Parameters:
// - redirectURI: Client's registered callback URL
// - state: Original state parameter (preserved in error response)
// - errorCode: OAuth2 error code (e.g., "invalid_client")
// - errorDescription: Human-readable error description
//
// Web callbacks and validated private-use callbacks (RFC 8252 §7.1) can receive errors.
// Example: https://client.example.com/callback?error=invalid_client&state=xyz
func BuildErrorRedirectURL(redirectURI, state, errorCode, errorDescription string) (string, error) {
	if redirectURI == "" {
		return "", fmt.Errorf("redirect_uri is required")
	}

	// Parse the redirect URI
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("invalid redirect_uri: %w", err)
	}

	// Callers verify registration first. Never redirect to an unsafe native scheme.
	if !urivalidation.IsValidPrivateUseRedirectURI(redirectURI) && (u.Opaque != "" || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http")) {
		return "", fmt.Errorf("invalid redirect_uri")
	}

	// Build query parameters
	q := u.Query()
	q.Set("error", errorCode)
	if errorDescription != "" {
		q.Set("error_description", errorDescription)
	}
	if state != "" {
		q.Set("state", state)
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}
