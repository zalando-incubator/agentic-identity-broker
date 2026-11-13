package storage

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// AuthorizationCode represents an ephemeral, single-use code issued by the authorization
// endpoint and exchanged for an access token. Stored with SHA-256 hash of the code value.
// Expires after 60 seconds. Invalidated atomically on first use.
type AuthorizationCode struct {
	ID            id.AuthorizationCodeID `json:"id" db:"id"`
	CodeHash      string                 `json:"code_hash" db:"code_hash"`
	AgentID       id.AgentID             `json:"agent_id" db:"agent_id"`
	ClientID      id.ClientID            `json:"client_id" db:"client_id"`
	Principal     id.Principal           `json:"principal" db:"principal"`
	Email         *string                `json:"email,omitempty" db:"email"`
	DisplayName   string                 `json:"display_name" db:"display_name"`
	RedirectURI   string                 `json:"redirect_uri" db:"redirect_uri"`
	CodeChallenge string                 `json:"code_challenge" db:"code_challenge"`
	Scope         string                 `json:"scope" db:"scope"`
	ExpiresAt     time.Time              `json:"expires_at" db:"expires_at"`
	UsedAt        *time.Time             `json:"used_at,omitempty" db:"used_at"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
	// ServiceScopeBindings pins each granted Virtual Service Scope to its service UUID.
	// Nil when no virtual scope was granted.
	ServiceScopeBindings ServiceScopeBindings `json:"service_scope_bindings,omitempty" db:"-"`
}

// Validate validates the AuthorizationCode fields.
func (c *AuthorizationCode) Validate() error {
	if c.CodeHash == "" {
		return NewStorageError("AuthorizationCode.Validate", ErrorKindValidation, nil, "code_hash is required")
	}
	if c.AgentID.IsZero() {
		return NewStorageError("AuthorizationCode.Validate", ErrorKindValidation, nil, "agent_id is required")
	}
	if c.Principal.IsZero() {
		return NewStorageError("AuthorizationCode.Validate", ErrorKindValidation, nil, "principal is required")
	}
	if c.RedirectURI == "" {
		return NewStorageError("AuthorizationCode.Validate", ErrorKindValidation, nil, "redirect_uri is required")
	}
	if c.CodeChallenge == "" {
		return NewStorageError("AuthorizationCode.Validate", ErrorKindValidation, nil, "code_challenge is required")
	}
	if c.ExpiresAt.IsZero() {
		return NewStorageError("AuthorizationCode.Validate", ErrorKindValidation, nil, "expires_at is required")
	}
	return c.ServiceScopeBindings.Validate(c.Scope, true)
}
