// Package ports defines interfaces for hexagonal architecture boundaries.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Sentinel errors for storage operations
var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("entity not found")
)

// IsNotFoundErr returns true if err represents a not-found condition from any
// storage adapter. Handles both ErrNotFound and storage.StorageError with
// ErrorKindNotFound.
func IsNotFoundErr(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var storageErr *storage.StorageError
	return errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound
}

// HealthChecker verifies storage backend health.
type HealthChecker interface {
	// HealthCheck verifies storage backend is operational.
	// Lightweight operation for monitoring (5-20ms typical).
	// Returns nil if healthy, error with details if unhealthy.
	// Used by monitoring and readiness probes.
	HealthCheck(ctx context.Context) error
}

// OAuth2TransactionManager supplies atomic storage operations for Fosite token flows.
type OAuth2TransactionManager interface {
	BeginTX(ctx context.Context) (context.Context, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// User represents a user entity in the storage layer.
// Domain entity - does not expose storage implementation details.
type User struct {
	ID        id.UserID `db:"id"`
	Email     string    `db:"email"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// UserFilter represents optional filtering criteria for ListUsers operations.
// Allows consumers to filter results by common fields.
type UserFilter struct {
	EmailPattern string // SQL LIKE pattern for email filtering (optional)
	Limit        int    // Maximum results to return (0 = no limit)
	Offset       int    // Number of results to skip (pagination)
}

// UserRepository defines storage operations for user entities.
// Follows Repository pattern with focused CRUD operations.
// This interface isolates domain logic from storage implementation.
// Following Interface Segregation Principle: small, focused interfaces.
type UserRepository interface {
	// CreateUser creates a new user entity in storage.
	// Returns error if:
	// - User ID already exists (StorageError with Kind=Conflict)
	// - Storage connection fails (StorageError with Kind=Connection)
	// - Operation timeout (StorageError with Kind=Timeout)
	CreateUser(ctx context.Context, user *User) error

	// GetUser retrieves a user entity by ID.
	// Returns nil if user not found (checked by caller to distinguish from error).
	// Returns StorageError with Kind=NotFound if user not found.
	// Returns StorageError for connection/timeout issues.
	GetUser(ctx context.Context, id id.UserID) (*User, error)

	// UpdateUser updates an existing user entity.
	// Returns error if:
	// - User ID not found (StorageError with Kind=NotFound)
	// - Storage connection fails (StorageError with Kind=Connection)
	// - Operation timeout (StorageError with Kind=Timeout)
	UpdateUser(ctx context.Context, user *User) error

	// DeleteUser deletes a user entity by ID.
	// Returns error if storage operation fails.
	// It is safe to delete non-existent users (idempotent).
	DeleteUser(ctx context.Context, id id.UserID) error

	// ListUsers retrieves multiple user entities matching optional filter.
	// Returns empty slice if no users match filter (not an error).
	// Returns StorageError for connection/timeout issues.
	ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error)
}

// AgentRepository defines storage operations for agent entities.
// Agents represent AI agents registered in the identity broker.
// Following Interface Segregation Principle: focused interface for agent operations.
type AgentRepository interface {
	Create(ctx context.Context, agent *storage.Agent) error
	Get(ctx context.Context, id id.AgentID) (*storage.Agent, error)
	Update(ctx context.Context, agent *storage.Agent) error
	Delete(ctx context.Context, id id.AgentID) error
	List(ctx context.Context) ([]*storage.Agent, error)
	GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error)
	ExistsOtherWithClientID(ctx context.Context, clientID id.ClientID, excludeAgentID *id.AgentID) (bool, error)
	// GetByClientURI resolves an exact CIMD URL first, then a URI pattern.
	// It returns a conflict error when patterns match multiple agents.
	GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error)
}

// AgentCanonicalIDRepository resolves type-scoped canonical agent IDs.
type AgentCanonicalIDRepository interface {
	GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.Agent, error)
}

// UserGrantRepository defines storage operations for user grant entities.
// Grants represent users delegating permissions to agents for third-party services.
// One grant per user-agent pair (upsert semantics).
type UserGrantRepository interface {
	// Create creates a new user grant or updates existing grant for same principal+agent (upsert).
	// The grant ID should be generated before calling this method.
	// Returns error if:
	// - Agent ID doesn't exist (StorageError with Kind=NotFound)
	// - Storage connection fails (StorageError with Kind=Connection)
	// - Operation timeout (StorageError with Kind=Timeout)
	Create(ctx context.Context, grant *storage.UserGrant) error

	// Get retrieves a user grant by ID.
	// Returns StorageError with Kind=NotFound if grant not found.
	// Returns StorageError for connection/timeout issues.
	Get(ctx context.Context, id id.GrantID) (*storage.UserGrant, error)

	// Update updates an existing user grant.
	// Returns error if:
	// - Grant ID not found (StorageError with Kind=NotFound)
	// - Storage connection fails (StorageError with Kind=Connection)
	// - Operation timeout (StorageError with Kind=Timeout)
	Update(ctx context.Context, grant *storage.UserGrant) error

	// Delete deletes a user grant by ID.
	// Returns error if storage operation fails.
	// It is safe to delete non-existent grants (idempotent).
	Delete(ctx context.Context, id id.GrantID) error

	// ListByPrincipalAndAgent retrieves all active grants for a principal and specific agent.
	// Filters expired grants (valid_until < NOW()).
	// Returns empty slice if no active grants exist (not an error).
	// Returns StorageError for connection/timeout issues.
	ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error)

	// FindByPrincipalAndAgent retrieves the grant for a principal and agent (if exists).
	// Returns nil if no grant exists (checked by caller to distinguish from error).
	// Returns StorageError with Kind=NotFound if grant not found.
	// Returns StorageError for connection/timeout issues.
	FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error)

	// DeleteByAgent deletes all grants associated with an agent.
	// Used during cascade deletion when agent is deleted (FR-021).
	// Returns error if storage operation fails.
	// It is safe to call with non-existent agent (idempotent).
	DeleteByAgent(ctx context.Context, agentID id.AgentID) error

	// DeleteByPrincipalAndAgentID deletes the grant owned by principal for the given agent.
	// Returns StorageError wrapping ports.ErrNotFound when no active grant exists for
	// the (principal, agent_id) pair — never returns raw sql.ErrNoRows.
	// This is the revocation operation for FR-014 (user-initiated grant deletion).
	// Unlike DeleteByAgent, this is NOT idempotent: absence of the grant is an error.
	DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error

	// ListByPrincipal retrieves all active grants for a principal across all agents.
	// Filters expired grants (valid_until < NOW()).
	// Returns empty slice if no active grants exist (not an error).
	// Returns StorageError for connection/timeout issues.
	ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error)

	// CountAgentsByPrincipalAndServiceID counts distinct agents for the exact principal whose GrantedPermissionSets include the service ID.
	// It includes expired grants for session dependency warnings.
	CountAgentsByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (int, error)

	// ListByPrincipalAndServiceID returns distinct agent IDs for the exact principal whose GrantedPermissionSets include the service ID.
	// It includes expired grants for session dependency warnings.
	// Returns empty slice if no matching grants exist.
	// Returns StorageError for connection/timeout issues.
	ListByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) ([]id.AgentID, error)

	// CountGrantsReferencingPermissionSet counts user grants whose granted_permission_sets
	// array contains an entry with the given permission set ID.
	// Used for permission set deletion protection — blocks deletion if count > 0.
	// Returns the count of user grants referencing this permission set.
	CountGrantsReferencingPermissionSet(ctx context.Context, psID id.PermissionSetID) (int, error)
}

// UserSessionRepository stores and retrieves user OAuth2 sessions.
//
// IMPORTANT: This repository operates on encrypted tokens. The contract is:
// - Input (Create): UserSession with tokens ALREADY ENCRYPTED by caller (OAuth2SessionService)
// - Output (Get): UserSession with tokens STILL ENCRYPTED (opaque binary data)
// - Caller (OAuth2SessionService) is responsible for decrypt/encrypt lifecycle
//
// This design maintains hexagonal architecture purity: the storage adapter is unaware
// of encryption concerns and focuses solely on persistence.
//
// Encryption Context (EncryptionContext field):
// - Stored as JSONB for audit/debugging purposes
// - Contains {"service_id": "<oauth2-service-id>"}
// - Used by OAuth2SessionService.Decrypt* methods as Additional Authenticated Data (AAD)
//
// One session per (principal, service_id) pair.
type UserSessionRepository interface {
	// Create creates a new user session.
	// Uses upsert semantics: if session exists for (principal, service_id), replaces tokens.
	// Returns error if:
	// - Service ID doesn't exist (StorageError with Kind=NotFound via FK constraint)
	// - Storage connection fails (StorageError with Kind=Connection)
	// - Operation timeout (StorageError with Kind=Timeout)
	Create(ctx context.Context, session *storage.UserSession) error

	// Get retrieves a session by ID.
	// Returns StorageError with Kind=NotFound if session not found.
	Get(ctx context.Context, id id.SessionID) (*storage.UserSession, error)

	// FindByPrincipalAndService retrieves the session for a principal and service.
	// Returns nil if no session exists (not an error).
	FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error)

	// ListByPrincipal retrieves all sessions for a principal, including expired ones.
	// Used for user-visible session lists (DELETE /api/third-party/sessions).
	// Returns empty slice if no sessions exist (not an error).
	ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error)

	// ListActiveByPrincipal retrieves only non-expired sessions for a principal.
	// Used for FR-020 consent submission validation and anyDelegatedSessionExpired checks.
	// Returns empty slice if no active sessions exist (not an error).
	ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error)

	// Delete deletes a session by ID.
	// Returns error if storage operation fails.
	// Idempotent: safe to delete non-existent session.
	Delete(ctx context.Context, id id.SessionID) error

	// DeleteByPrincipalAndService deletes the session for a principal and service.
	// Returns error if storage operation fails.
	// Idempotent: safe to delete non-existent session.
	DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error

	// CountByService counts sessions referencing a service.
	// Used to enforce deletion protection (cannot delete service with active sessions).
	CountByService(ctx context.Context, serviceID id.ServiceID) (int, error)
}

// PermissionSetRepository defines storage operations for permission set entities.
// Permission sets are admin-defined bundles of OAuth2 scopes spanning one or more third-party services.
// Following Interface Segregation Principle: focused interface for permission set operations.
type PermissionSetRepository interface {
	Create(ctx context.Context, ps *storage.PermissionSet) error
	Get(ctx context.Context, id id.PermissionSetID) (*storage.PermissionSet, error)
	GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error)
	Update(ctx context.Context, ps *storage.PermissionSet) error
	Delete(ctx context.Context, id id.PermissionSetID) error
	List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error)
	CountAgentsReferencingPermissionSet(ctx context.Context, id id.PermissionSetID) (int, error)
	CountPermissionSetsForService(ctx context.Context, serviceID id.ServiceID) (int, error)
}

// PermissionSetCanonicalIDRepository resolves and batch-loads type-scoped canonical permission-set IDs.
type PermissionSetCanonicalIDRepository interface {
	GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.PermissionSet, error)
	GetCanonicalIDs(ctx context.Context, ids []id.PermissionSetID) (map[id.PermissionSetID]string, error)
}

// ToolApprovalRepository defines core CRUD operations for tool approval entities.
// Follows ISP: max 6 methods. Implementations return approvals with a non-nil ParamsPattern.
type ToolApprovalRepository interface {
	// Create creates a new pending tool approval.
	// Uses partial unique index for deduplication (principal, agent_id, tool_name, arguments_hash)
	// where status=pending AND consumed=false.
	// Returns the existing record if a duplicate is found (idempotent).
	Create(ctx context.Context, approval *storage.ToolApproval) (*storage.ToolApproval, error)

	// Get retrieves a tool approval by ID.
	// Returns StorageError{Kind: NotFound} if not found.
	Get(ctx context.Context, id id.ApprovalID) (*storage.ToolApproval, error)

	// Approve transitions a pending approval to approved status with the user decision.
	Approve(ctx context.Context, id id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error)

	// Deny transitions a pending approval to denied status.
	Deny(ctx context.Context, id id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error)

	// RevokePermanent atomically clears a permanent approval or denial.
	RevokePermanent(ctx context.Context, id id.ApprovalID, revokedAt time.Time) (*storage.ToolApproval, error)

	// Consume marks an approved once-persistence approval as consumed.
	Consume(ctx context.Context, id id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error)
}

// ToolApprovalQueryRepository defines read-side list queries for tool approvals.
// Used by the long-poll sync endpoint and the consent management UI.
type ToolApprovalQueryRepository interface {
	// ListAllActive lists all active approvals, optionally filtered by principal and active agent sessions.
	ListAllActive(ctx context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) ([]*storage.ToolApproval, error)

	// ListActiveByPrincipalAndAgent lists active approvals for a principal-agent pair.
	ListActiveByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error)

	// ListPermanentByPrincipal lists permanent approvals/denials for a principal.
	ListPermanentByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.ToolApproval, error)
}

// ToolApprovalMetricsRepository defines the count query for rate limiting.
type ToolApprovalMetricsRepository interface {
	// CountPendingByPrincipalAndAgent counts pending (non-expired) approvals for rate limiting.
	CountPendingByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error)
}

// ApprovalSyncStateRepository manages the global approval sync version counter.
type ApprovalSyncStateRepository interface {
	// GetVersion returns the current sync version.
	GetVersion(ctx context.Context) (int64, error)

	// IncrementVersion atomically increments the version counter.
	// Returns the new version.
	IncrementVersion(ctx context.Context) (int64, error)
}

// ApprovalMutationSyncRepository reports whether approval mutations update the sync version atomically.
type ApprovalMutationSyncRepository interface {
	ApprovalMutationsSyncAtomically() bool
}

// ClientCredentialRepository manages broker-issued OAuth2 client credentials.
// One credential set per agent (enforced by UNIQUE on agent_id).
type ClientCredentialRepository interface {
	// Create stores a new broker client credential.
	Create(ctx context.Context, credential *storage.ClientCredential) error

	// GetByAgentID retrieves the credential for a given agent.
	GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error)

	// GetByClientID retrieves the credential by its client ID.
	GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.ClientCredential, error)

	// Delete removes the credential for a given agent.
	Delete(ctx context.Context, agentID id.AgentID) error

	// Rotate atomically replaces the existing credential for an agent with a new one.
	// The old credential is only removed after the new one is successfully stored.
	// Returns an error if no existing credential is found for the agent.
	Rotate(ctx context.Context, agentID id.AgentID, newCredential *storage.ClientCredential) error
}

// SigningKeyRepository manages asymmetric signing keys for JWT access tokens.
type SigningKeyRepository interface {
	// Create stores a new signing key.
	Create(ctx context.Context, key *storage.SigningKey) error

	// CreateAndSetCurrent stores a new signing key and atomically promotes it to
	// current while demoting all other keys, in a single transaction.
	CreateAndSetCurrent(ctx context.Context, key *storage.SigningKey) error

	// GetByKID retrieves a signing key by its key ID (kid).
	GetByKID(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error)

	// GetCurrent retrieves the current active signing key.
	GetCurrent(ctx context.Context) (*storage.SigningKey, error)

	// ListActive returns all signing keys that have not been removed.
	ListActive(ctx context.Context) ([]*storage.SigningKey, error)

	// SetCurrent promotes a key to be the current signing key using the domain-supplied
	// activation timestamp and returns the updated metadata.
	SetCurrent(ctx context.Context, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error)

	// SetPublicJWK backfills a legacy signing key's public JWK if it is missing.
	// Implementations must not overwrite an existing value.
	SetPublicJWK(ctx context.Context, kid id.KeyID, publicJWK []byte) error

	// Delete soft-deletes a signing key by setting removed_at.
	// Implementations must enforce signing-key invariants atomically:
	// the current key cannot be removed and at least one active key must remain.
	Delete(ctx context.Context, kid id.KeyID) error

	// CountActive returns the number of non-removed signing keys.
	CountActive(ctx context.Context) (int, error)
}

// AuthorizationCodeRepository manages ephemeral authorization codes.
type AuthorizationCodeRepository interface {
	// Create stores a new authorization code.
	Create(ctx context.Context, code *storage.AuthorizationCode) error

	// FindByCodeHash retrieves an authorization code by its SHA-256 hash.
	FindByCodeHash(ctx context.Context, codeHash string) (*storage.AuthorizationCode, error)

	// MarkUsed marks an authorization code as used (single-use enforcement).
	MarkUsed(ctx context.Context, id id.AuthorizationCodeID) error

	// DeleteExpired removes expired authorization codes. Returns the count of deleted codes.
	DeleteExpired(ctx context.Context) (int, error)
}

// RefreshTokenSessionRepository stores issued refresh tokens for single-use rotation.
// The signature (SHA-256 of the opaque token) is the primary key.
type RefreshTokenSessionRepository interface {
	// Create stores a refresh token session keyed by token signature.
	Create(ctx context.Context, session *storage.RefreshTokenSession) error

	// FindBySignature retrieves a refresh token session by its signature.
	FindBySignature(ctx context.Context, signature string) (*storage.RefreshTokenSession, error)

	// MarkUsed marks a refresh token session as used or revoked.
	MarkUsed(ctx context.Context, signature string) error

	// RevokeByRequestID revokes all refresh token sessions for a fosite request ID.
	RevokeByRequestID(ctx context.Context, requestID string) error

	// DeleteExpired removes expired refresh token sessions.
	DeleteExpired(ctx context.Context) (int, error)
}

// PKCESessionRepository stores the PKCE challenge for pending authorization codes.
// The signature (fosite code signature) is the primary key; sessions are one-shot
// and deleted immediately after the token endpoint consumes them.
type PKCESessionRepository interface {
	// Create stores a PKCE session keyed by fosite code signature.
	Create(ctx context.Context, session *storage.PKCESession) error

	// FindBySignature retrieves a PKCE session by its code signature.
	FindBySignature(ctx context.Context, signature string) (*storage.PKCESession, error)

	// Delete removes a PKCE session by its code signature.
	Delete(ctx context.Context, signature string) error

	// DeleteExpired removes PKCE sessions whose authorization codes have expired.
	DeleteExpired(ctx context.Context) (int, error)
}
