package storage

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// RefreshTokenSession stores an issued refresh token by signature for single-use rotation.
// The signature is the SHA-256 hash of the opaque refresh token value.
type RefreshTokenSession struct {
	Signature   string       `db:"signature"`
	RequestID   string       `db:"request_id"`
	AgentID     id.AgentID   `db:"agent_id"`
	ClientID    id.ClientID  `db:"client_id"`
	Principal   id.Principal `db:"principal"`
	GrantID     *id.GrantID  `db:"grant_id"`
	Email       *string      `db:"email"`
	DisplayName string       `db:"display_name"`
	Scope       string       `db:"scope"`
	ExpiresAt   time.Time    `db:"expires_at"`
	UsedAt      *time.Time   `db:"used_at"`
	CreatedAt   time.Time    `db:"created_at"`
}

// Validate checks that all required fields are present.
func (s *RefreshTokenSession) Validate() error {
	if s.Signature == "" {
		return NewStorageError("RefreshTokenSession.Validate", ErrorKindValidation, nil, "signature is required")
	}
	if s.RequestID == "" {
		return NewStorageError("RefreshTokenSession.Validate", ErrorKindValidation, nil, "request_id is required")
	}
	if s.AgentID.IsZero() {
		return NewStorageError("RefreshTokenSession.Validate", ErrorKindValidation, nil, "agent_id is required")
	}
	if s.ClientID.IsZero() {
		return NewStorageError("RefreshTokenSession.Validate", ErrorKindValidation, nil, "client_id is required")
	}
	if s.Principal.IsZero() {
		return NewStorageError("RefreshTokenSession.Validate", ErrorKindValidation, nil, "principal is required")
	}
	if s.ExpiresAt.IsZero() {
		return NewStorageError("RefreshTokenSession.Validate", ErrorKindValidation, nil, "expires_at is required")
	}
	return nil
}
