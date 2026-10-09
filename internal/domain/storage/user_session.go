package storage

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// UserSession represents an authenticated OAuth2 session between a user and a third-party service.
// This is an aggregate root - it owns the encrypted tokens and manages session lifecycle.
// One session per (principal, service_id) pair, enforced by database unique constraint.
type UserSession struct {
	ID        id.SessionID `json:"id" db:"id"`
	Principal id.Principal `json:"principal" db:"principal"`
	ServiceID id.ServiceID `json:"service_id" db:"service_id"`
	// ExpectedIssuerURI binds a discovered callback to the issuer that initiated it.
	// Repositories check this value before storage; they do not persist it.
	ExpectedIssuerURI string `json:"-" db:"-"`
	// ExpectedResource is the discovered audience sealed in the callback state.
	// Repositories check it before insert or upsert; they do not persist it.
	ExpectedResource string `json:"-" db:"-"`

	// EncryptedAccessToken contains the OAuth2 access token ENCRYPTED by OAuth2SessionService
	// using envelope encryption with service_id as Additional Authenticated Data (AAD).
	//
	// IMPORTANT: This field contains encrypted bytes that MUST be decrypted by OAuth2SessionService
	// using DecryptAccessToken(). Storage adapters treat this as opaque binary data and MUST NOT
	// attempt to decrypt or re-encrypt this field.
	//
	// Encryption/Decryption Lifecycle:
	// - Encryption: OAuth2SessionService.storeSession() encrypts plaintext token → UserSession
	// - Storage: Repository stores encrypted bytes as BYTEA (opaque)
	// - Retrieval: Repository returns encrypted bytes on Get()
	// - Decryption: OAuth2SessionService.DecryptAccessToken() decrypts bytes → plaintext
	EncryptedAccessToken []byte `json:"-" db:"encrypted_access_token"`

	// EncryptedRefreshToken contains the OAuth2 refresh token ENCRYPTED by OAuth2SessionService.
	// See EncryptedAccessToken for encryption lifecycle details.
	EncryptedRefreshToken []byte `json:"-" db:"encrypted_refresh_token"`

	TokenType             string            `json:"token_type" db:"token_type"`
	AccessTokenExpiresAt  *time.Time        `json:"access_token_expires_at,omitempty" db:"access_token_expires_at"`
	RefreshTokenExpiresAt *time.Time        `json:"refresh_token_expires_at,omitempty" db:"refresh_token_expires_at"`
	Scope                 []string          `json:"scope" db:"scope"`
	EncryptionContext     EncryptionContext `json:"encryption_context" db:"encryption_context"`
	InitiatedAt           time.Time         `json:"initiated_at" db:"initiated_at"`
	CreatedAt             time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at" db:"updated_at"`
}

// EncryptionContext holds metadata for token encryption/decryption.
// This is stored in JSONB and included as AAD (Additional Authenticated Data).
// Simplified to service_id-only for performance optimization (ADR 008).
type EncryptionContext struct {
	ServiceID id.ServiceID `json:"service_id"` // OAuth service identifier
}

// Value implements driver.Valuer for EncryptionContext (JSONB serialization).
func (ec EncryptionContext) Value() (driver.Value, error) {
	return json.Marshal(ec)
}

// Scan implements sql.Scanner for EncryptionContext (JSONB deserialization).
func (ec *EncryptionContext) Scan(value any) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("invalid type for EncryptionContext")
	}
	return json.Unmarshal(bytes, ec)
}

// Validate performs domain validation on UserSession.
func (s *UserSession) Validate() error {
	if s.ID.IsZero() {
		return errors.New("session ID cannot be empty")
	}
	if s.Principal.IsZero() {
		return errors.New("principal is required")
	}
	if len(s.Principal.String()) > 200 {
		return errors.New("principal exceeds 200 characters")
	}
	if s.ServiceID.IsZero() {
		return errors.New("service_id is required")
	}
	if len(s.EncryptedAccessToken) == 0 {
		return errors.New("encrypted access token is required")
	}
	if s.TokenType == "" {
		return errors.New("token_type is required")
	}
	return nil
}

// IsExpired returns true if the session's refresh token has expired.
// Access token expiration does not mark session as expired (future auto-refresh).
// Session without refresh token expiration is considered non-expiring.
func (s *UserSession) IsExpired() bool {
	if s.RefreshTokenExpiresAt == nil {
		return false // No expiration set
	}
	return time.Now().After(*s.RefreshTokenExpiresAt)
}

// HasValidAccessToken returns true if access token has not expired.
func (s *UserSession) HasValidAccessToken() bool {
	if s.AccessTokenExpiresAt == nil {
		return true // No expiration set
	}
	return time.Now().Before(*s.AccessTokenExpiresAt)
}

// CanRefresh returns true if session has a refresh token that hasn't expired.
func (s *UserSession) CanRefresh() bool {
	if len(s.EncryptedRefreshToken) == 0 {
		return false // No refresh token
	}
	if s.RefreshTokenExpiresAt == nil {
		return true // No expiration set
	}
	return time.Now().Before(*s.RefreshTokenExpiresAt)
}

// UserSessionSummary is a read model for displaying session information.
// Includes computed fields like dependent agent count.
type UserSessionSummary struct {
	ID                    id.SessionID `json:"id"`
	ServiceID             id.ServiceID `json:"service_id"`
	ServiceDisplayName    string       `json:"service_display_name"`
	TokenType             string       `json:"token_type"`
	Scope                 []string     `json:"scope"`
	InitiatedAt           time.Time    `json:"initiated_at"`
	IsExpired             bool         `json:"is_expired"`
	AccessTokenExpired    bool         `json:"access_token_expired"`
	HasRefreshToken       bool         `json:"has_refresh_token"`
	RefreshTokenExpiresAt *time.Time   `json:"refresh_token_expires_at,omitempty"`
	DependentAgentCount   int          `json:"dependent_agent_count"`
	IsEncrypted           bool         `json:"is_encrypted"` // Always true
}

// NewUserSessionSummary creates a summary from a UserSession.
func NewUserSessionSummary(session *UserSession, serviceDisplayName string, agentCount int) *UserSessionSummary {
	return &UserSessionSummary{
		ID:                    session.ID,
		ServiceID:             session.ServiceID,
		ServiceDisplayName:    serviceDisplayName,
		TokenType:             session.TokenType,
		Scope:                 session.Scope,
		InitiatedAt:           session.InitiatedAt,
		IsExpired:             session.IsExpired(),
		AccessTokenExpired:    !session.HasValidAccessToken(),
		HasRefreshToken:       len(session.EncryptedRefreshToken) > 0,
		RefreshTokenExpiresAt: session.RefreshTokenExpiresAt,
		DependentAgentCount:   agentCount,
		IsEncrypted:           true,
	}
}
