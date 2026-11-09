package storage

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// SessionExpiryCursor locates the last (expiry, ID) pair returned by a sweep page.
// A zero ID starts the scan; the expiry may be the zero time.
type SessionExpiryCursor struct {
	AccessTokenExpiresAt time.Time
	ID                   id.SessionID
}

// AccessExpiryClass places a session's access-token expiry relative to a threshold.
type AccessExpiryClass string

const (
	// AccessExpiryUnknown means no access-token expiry is recorded.
	AccessExpiryUnknown AccessExpiryClass = "unknown"
	// AccessExpiryAfterThreshold means the expiry is strictly after the threshold.
	AccessExpiryAfterThreshold AccessExpiryClass = "after_threshold"
	// AccessExpiryDue means the expiry is after now and at or before the threshold (inclusive, same as AccessTokenExpiresBy).
	AccessExpiryDue AccessExpiryClass = "due"
	// AccessExpiryExpired means the expiry is at or before now (the session's access token is no longer usable).
	AccessExpiryExpired AccessExpiryClass = "expired"
)

// SessionExpiryGroup counts stored sessions of one service sharing an access-expiry class and
// refresh usability. Each (ServiceID, AccessExpiry, RefreshUsable) appears at most once per result.
type SessionExpiryGroup struct {
	ServiceID    id.ServiceID
	AccessExpiry AccessExpiryClass
	// RefreshUsable means a refresh token is present and the provider has not rejected it (same as CanRefresh).
	RefreshUsable bool
	// Count is the number of sessions in this group, > 0.
	Count int
}

// UserSession represents an authenticated OAuth2 session between a user and a third-party service.
// This is an aggregate root - it owns the encrypted tokens and manages session lifecycle.
// One session per (principal, service_id) pair, enforced by database unique constraint.
type UserSession struct {
	ID               id.SessionID `json:"id" db:"id"`
	Principal        id.Principal `json:"principal" db:"principal"`
	ServiceID        id.ServiceID `json:"service_id" db:"service_id"`
	CallbackID       string       `json:"-" db:"callback_id"`
	UpstreamClientID *id.ClientID `json:"-" db:"upstream_client_id"`
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

	TokenType            string     `json:"token_type" db:"token_type"`
	AccessTokenExpiresAt *time.Time `json:"access_token_expires_at,omitempty" db:"access_token_expires_at"`
	// RefreshRejectedAt records when the provider terminally rejected the stored refresh token
	// (HTTP 4xx invalid_grant). Nil means no rejection has been observed; it does not prove validity.
	// The broker cannot predict a refresh token's lifetime, so the legacy refresh_token_expires_at
	// column holds only this observation and is never exposed as an expiry.
	RefreshRejectedAt *time.Time        `json:"-" db:"refresh_token_expires_at"`
	Scope             []string          `json:"scope" db:"scope"`
	EncryptionContext EncryptionContext `json:"encryption_context" db:"encryption_context"`
	InitiatedAt       time.Time         `json:"initiated_at" db:"initiated_at"`
	CreatedAt         time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at" db:"updated_at"`
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
	if s.UpstreamClientID != nil && s.UpstreamClientID.IsZero() {
		return errors.New("upstream client ID cannot be empty")
	}
	if len(s.EncryptedAccessToken) == 0 {
		return errors.New("encrypted access token is required")
	}
	if s.TokenType == "" {
		return errors.New("token_type is required")
	}
	return nil
}

// RefreshRejected reports an observed terminal provider rejection of the stored refresh token.
func (s *UserSession) RefreshRejected() bool {
	return s.RefreshRejectedAt != nil
}

// RequiresReauthentication reports that the broker cannot obtain another access token for this
// session: the provider rejected the refresh token, or the access token expired and no refresh
// token is stored. A still-valid access token does not override a recorded rejection.
func (s *UserSession) RequiresReauthentication() bool {
	return RequiresReauthentication(s.AccessTokenExpiresAt, s.RefreshRejected(), len(s.EncryptedRefreshToken) > 0)
}

// RequiresReauthentication also evaluates metadata projections without loading encrypted token blobs.
func RequiresReauthentication(accessExpiry *time.Time, refreshRejected, hasRefreshToken bool) bool {
	if refreshRejected {
		return true
	}
	accessExpired := accessExpiry != nil && !accessExpiry.After(time.Now())
	return accessExpired && !hasRefreshToken
}

// HasValidAccessToken returns true if access token has not expired.
func (s *UserSession) HasValidAccessToken() bool {
	if s.AccessTokenExpiresAt == nil {
		return true // No expiration set
	}
	return time.Now().Before(*s.AccessTokenExpiresAt)
}

// AccessTokenExpiresBy reports whether the access token expires at or before t.
// A session without an access-token expiry is never due for refresh.
func (s *UserSession) AccessTokenExpiresBy(t time.Time) bool {
	return s.AccessTokenExpiresAt != nil && !s.AccessTokenExpiresAt.After(t)
}

// CanRefresh reports a stored refresh token that the provider has not rejected.
func (s *UserSession) CanRefresh() bool {
	return len(s.EncryptedRefreshToken) > 0 && !s.RefreshRejected()
}

// UserSessionSummary is a read model for displaying session information.
// Includes computed fields like dependent agent count.
type UserSessionSummary struct {
	ID                       id.SessionID `json:"id"`
	ServiceID                id.ServiceID `json:"service_id"`
	ServiceDisplayName       string       `json:"service_display_name"`
	TokenType                string       `json:"token_type"`
	Scope                    []string     `json:"scope"`
	InitiatedAt              time.Time    `json:"initiated_at"`
	RequiresReauthentication bool         `json:"requires_reauthentication"`
	// IsExpired is a deprecated alias of RequiresReauthentication kept for existing API consumers.
	IsExpired           bool `json:"is_expired"`
	AccessTokenExpired  bool `json:"access_token_expired"`
	HasRefreshToken     bool `json:"has_refresh_token"`
	DependentAgentCount int  `json:"dependent_agent_count"`
	IsEncrypted         bool `json:"is_encrypted"` // Always true
}

// SetRequiresReauthentication sets the flag together with its deprecated alias.
func (s *UserSessionSummary) SetRequiresReauthentication(required bool) {
	s.RequiresReauthentication = required
	s.IsExpired = required
}

// UserSessionDetails is the session-detail read model: the summary plus ownership and lifecycle
// metadata. It exposes neither token material nor the rejection timestamp.
type UserSessionDetails struct {
	*UserSessionSummary
	Principal            id.Principal      `json:"principal"`
	AccessTokenExpiresAt *time.Time        `json:"access_token_expires_at,omitempty"`
	EncryptionContext    EncryptionContext `json:"encryption_context"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// NewUserSessionSummary creates a summary from a UserSession.
func NewUserSessionSummary(session *UserSession, serviceDisplayName string, agentCount int) *UserSessionSummary {
	summary := &UserSessionSummary{
		ID:                  session.ID,
		ServiceID:           session.ServiceID,
		ServiceDisplayName:  serviceDisplayName,
		TokenType:           session.TokenType,
		Scope:               session.Scope,
		InitiatedAt:         session.InitiatedAt,
		AccessTokenExpired:  !session.HasValidAccessToken(),
		HasRefreshToken:     len(session.EncryptedRefreshToken) > 0,
		DependentAgentCount: agentCount,
		IsEncrypted:         true,
	}
	summary.SetRequiresReauthentication(session.RequiresReauthentication())
	return summary
}

// NewUserSessionDetails creates the detail read model from a UserSession.
func NewUserSessionDetails(session *UserSession, serviceDisplayName string, agentCount int) *UserSessionDetails {
	return &UserSessionDetails{
		UserSessionSummary:   NewUserSessionSummary(session, serviceDisplayName, agentCount),
		Principal:            session.Principal,
		AccessTokenExpiresAt: session.AccessTokenExpiresAt,
		EncryptionContext:    session.EncryptionContext,
		CreatedAt:            session.CreatedAt,
		UpdatedAt:            session.UpdatedAt,
	}
}
