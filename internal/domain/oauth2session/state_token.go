package oauth2session

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// OAuth2StateTokenClaims contains the claims embedded in a JWE state token.
// These claims bind the OAuth2 callback to the initiating request.
type OAuth2StateTokenClaims struct {
	// Principal is the authenticated user who initiated the OAuth2 flow.
	// Must match the principal at callback time (CSRF protection).
	Principal id.Principal `json:"principal"`

	// PKCEVerifier is the PKCE code verifier for token exchange.
	// Base64url-encoded, 32-128 bytes per RFC 7636.
	PKCEVerifier string `json:"pkce_verifier"`

	// ServiceID is the third-party service being authorized.
	// Must match the serviceId path parameter at callback.
	ServiceID id.ServiceID `json:"service_id"`

	// RedirectURI is where to redirect after flow completes.
	// Must be same-origin with the authorize request.
	RedirectURI string `json:"redirect_uri"`

	// ConsentStateID is the optional current-tab selection reference.
	ConsentStateID string `json:"consent_state_id,omitempty"`

	// IssuedAt is when the token was created.
	IssuedAt time.Time `json:"iat"`

	// ExpiresAt is when the token expires (TTL: 10 min default).
	ExpiresAt time.Time `json:"exp"`
}

// Validate checks that all required claims are present.
func (c *OAuth2StateTokenClaims) Validate() error {
	if c.Principal.IsZero() {
		return errors.New("principal is required")
	}
	if c.PKCEVerifier == "" {
		return errors.New("pkce_verifier is required")
	}
	if c.ServiceID.IsZero() {
		return errors.New("service_id is required")
	}
	if c.RedirectURI == "" {
		return errors.New("redirect_uri is required")
	}
	if c.ConsentStateID != "" {
		if _, err := uuid.Parse(c.ConsentStateID); err != nil {
			return errors.New("consent_state_id must be a UUID")
		}
	}
	if c.IssuedAt.IsZero() {
		return errors.New("iat is required")
	}
	if c.ExpiresAt.IsZero() {
		return errors.New("exp is required")
	}
	return nil
}

// IsExpired returns true if the token has expired.
func (c *OAuth2StateTokenClaims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}
