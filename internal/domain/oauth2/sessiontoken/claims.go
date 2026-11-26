package sessiontoken

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const ttl = 10 * time.Minute

// AuthorizationSessionClaims carries the full authorization context in a JWE token.
// OriginalURL is the raw authorize request URL; scope and redirect_uri are parsed from
// it at the consumption site rather than duplicated as flat fields.
type AuthorizationSessionClaims struct {
	AgentID      id.AgentID                 `json:"agent_id"`
	Principal    id.Principal               `json:"principal"`
	OriginalURL  string                     `json:"original_url"`
	CIMDMetadata *ports.SessionCIMDMetadata `json:"cimd_metadata,omitempty"`
	// RequestedServiceIDs and ServiceScopeBindings are derived by the server from the virtual
	// scopes in OriginalURL; browser-supplied values never replace them.
	RequestedServiceIDs  []id.ServiceID          `json:"requested_service_ids,omitempty"`
	ServiceScopeBindings map[string]id.ServiceID `json:"service_scope_bindings,omitempty"`
	IssuedAt             time.Time               `json:"iat"`
	ExpiresAt            time.Time               `json:"exp"`
}

// IsExpired reports whether the session token TTL has elapsed.
func (c *AuthorizationSessionClaims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// NewAuthorizationSessionClaims initialises claims with IssuedAt and ExpiresAt set from now.
func NewAuthorizationSessionClaims(
	agentID id.AgentID,
	principal id.Principal,
	originalURL string,
	cimdMetadata *ports.SessionCIMDMetadata,
) (*AuthorizationSessionClaims, error) {
	if agentID.IsZero() {
		return nil, errors.New("agentID must not be zero")
	}
	if principal.IsZero() {
		return nil, errors.New("principal must not be zero")
	}
	if originalURL == "" {
		return nil, errors.New("originalURL must not be empty")
	}
	// Normalize backslashes only in the path/scheme portion, not in query parameters
	// or fragments. Browsers normalize \\evil.com to //evil.com (open-redirect vector),
	// but backslashes in state= or other query values must be preserved as-is to avoid
	// mutating CSRF tokens the client will verify on return.
	qIdx := strings.IndexByte(originalURL, '?')
	if qIdx == -1 {
		qIdx = len(originalURL)
	}
	normalized := strings.ReplaceAll(originalURL[:qIdx], "\\", "/") + originalURL[qIdx:]
	if u, err := url.Parse(normalized); err != nil ||
		(u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") ||
		(u.Scheme == "" && u.Host != "") {
		return nil, errors.New("originalURL must be a valid relative or http(s) URL")
	}
	now := time.Now()
	return &AuthorizationSessionClaims{
		AgentID:      agentID,
		Principal:    principal,
		OriginalURL:  normalized,
		CIMDMetadata: cimdMetadata,
		IssuedAt:     now,
		ExpiresAt:    now.Add(ttl),
	}, nil
}
