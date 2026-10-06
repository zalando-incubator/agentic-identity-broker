// Package storage implements storage adapters for different backends.
// Adapters implement storage repositories and lifecycle helpers.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface check
var _ ports.StorageTransactionManager = (*Adapter)(nil)

// lifecycleAdapter defines the lifecycle operations expected on storage adapters.
type lifecycleAdapter interface {
	Initialize(context.Context) error
	Close(context.Context) error
	HealthCheck(context.Context) error
}

type signingKeyAdapter interface {
	ports.SigningKeyRepository
	ports.SigningKeyBootstrapCoordinator
}

// Adapter composes storage functionality.
// Adapters implement repository interfaces (UserRepository, etc.)
// This struct is returned by NewAdapter factory function.
type Adapter struct {
	lifecycle                 lifecycleAdapter
	users                     ports.UserRepository
	agents                    ports.AgentRepository
	providers                 ports.ThirdpartyOAuth2ProviderRepository
	userGrants                ports.UserGrantRepository
	userSessions              ports.UserSessionRepository
	sessionRefresh            ports.UserSessionRefreshRepository
	toolApprovals             ports.ToolApprovalRepository
	toolApprovalQueries       ports.ToolApprovalQueryRepository
	toolApprovalMetrics       ports.ToolApprovalMetricsRepository
	approvalSyncState         ports.ApprovalSyncStateRepository
	permissionSets            ports.PermissionSetRepository
	brokerCredentials         ports.ClientCredentialRepository
	signingKeys               signingKeyAdapter
	refreshSessions           ports.RefreshSessionRepository
	refreshTokens             ports.RefreshTokenRepository
	refreshRevocations        ports.RefreshSessionRevocationRepository
	refreshMaintenance        ports.RefreshSessionMaintenanceRepository
	refreshMaintenanceControl ports.RefreshSessionMaintenanceControl
	authorizationClock        ports.AuthorizationClock
	authorizationCoordinator  ports.AuthorizationSessionCoordinator
	transactions              ports.StorageTransactionManager
	authorizationCodes        ports.AuthorizationCodeRepository
	pkceSessions              ports.PKCESessionRepository
}

// NewAdapter creates a storage adapter based on configuration.
// Returns appropriate adapter implementation (memory or PostgreSQL).
// Factory pattern allows backend selection without domain logic knowing details.
//
// Returns error if:
// - Backend type is unsupported
// - Backend-specific initialization fails
func NewAdapter(config *ports.StorageConfig) (*Adapter, error) {
	backend := storage.StorageBackend(config.Backend)

	switch backend {
	case storage.BackendMemory:
		return newMemoryAdapter(config)

	case storage.BackendPostgres:
		return newPostgresAdapter(config)

	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", config.Backend)
	}
}

// newMemoryAdapter creates an in-memory storage adapter.
func newMemoryAdapter(config *ports.StorageConfig) (*Adapter, error) {
	memAdapter := memory.NewAdapter()
	if err := initializeAdapter(context.Background(), memAdapter, config); err != nil {
		return nil, err
	}
	agentRepo := memory.NewAgentRepository()
	permissionSets := memory.NewPermissionSetRepository().WithAgentRepository(agentRepo)
	userGrants := memory.NewUserGrantRepository().WithPermissionSetRepository(permissionSets)
	toolApprovalRepo := memory.NewToolApprovalRepository()
	signingKeys := memory.NewSigningKeyStore()
	sessions := memory.NewInMemoryUserSessionRepository()
	credentials := memory.NewClientCredentialStore()
	codes := memory.NewAuthorizationCodeStore()
	pkce := memory.NewPKCESessionStore()
	refresh := memory.NewRefreshSessionStore(agentRepo, userGrants, credentials, codes, pkce)
	tokens := memory.NewRefreshTokenStore(refresh)
	return &Adapter{
		lifecycle:                 memAdapter,
		users:                     memAdapter,
		agents:                    agentRepo,
		providers:                 memory.NewInMemoryThirdpartyOAuth2ProviderRepository(),
		userGrants:                userGrants,
		userSessions:              sessions,
		sessionRefresh:            sessions,
		toolApprovals:             toolApprovalRepo,
		toolApprovalQueries:       toolApprovalRepo,
		toolApprovalMetrics:       toolApprovalRepo,
		approvalSyncState:         memory.NewApprovalSyncStateRepository(),
		permissionSets:            permissionSets,
		brokerCredentials:         credentials,
		signingKeys:               signingKeys,
		authorizationCodes:        codes,
		refreshSessions:           refresh,
		refreshTokens:             tokens,
		refreshRevocations:        refresh,
		refreshMaintenance:        refresh,
		refreshMaintenanceControl: memory.RefreshMaintenanceControl{},
		authorizationClock:        refresh,
		authorizationCoordinator:  refresh,
		transactions:              refresh,
		pkceSessions:              pkce,
	}, nil
}

// newPostgresAdapter creates a PostgreSQL storage adapter.
func newPostgresAdapter(config *ports.StorageConfig) (*Adapter, error) {
	pgAdapter, err := postgres.NewAdapter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL adapter: %w", err)
	}
	if err := initializeAdapter(context.Background(), pgAdapter, config); err != nil {
		return nil, err
	}

	toolApprovalRepo := postgres.NewToolApprovalRepository(pgAdapter)
	signingKeys := postgres.NewSigningKeyRepo(pgAdapter)
	sessions := postgres.NewUserSessionRepository(pgAdapter)

	refresh := postgres.NewRefreshSessionRepo(pgAdapter)
	tokens := postgres.NewRefreshTokenRepo(pgAdapter)
	coordinator := postgres.NewAuthorizationSessionCoordinator(pgAdapter)
	return &Adapter{
		lifecycle:                 pgAdapter,
		users:                     pgAdapter,
		agents:                    postgres.NewAgentRepository(pgAdapter),
		providers:                 postgres.NewPostgresThirdpartyOAuth2ProviderRepository(pgAdapter),
		userGrants:                postgres.NewUserGrantRepository(pgAdapter),
		userSessions:              sessions,
		sessionRefresh:            sessions,
		toolApprovals:             toolApprovalRepo,
		toolApprovalQueries:       toolApprovalRepo,
		toolApprovalMetrics:       toolApprovalRepo,
		approvalSyncState:         postgres.NewApprovalSyncStateRepository(pgAdapter),
		permissionSets:            postgres.NewPermissionSetRepository(pgAdapter),
		brokerCredentials:         postgres.NewClientCredentialRepo(pgAdapter),
		signingKeys:               signingKeys,
		authorizationCodes:        postgres.NewAuthorizationCodeRepo(pgAdapter),
		refreshSessions:           refresh,
		refreshTokens:             tokens,
		refreshRevocations:        refresh,
		refreshMaintenance:        refresh,
		refreshMaintenanceControl: postgres.NewRefreshMaintenanceControl(pgAdapter),
		authorizationClock:        coordinator,
		authorizationCoordinator:  coordinator,
		transactions:              pgAdapter,
		pkceSessions:              postgres.NewPKCESessionRepo(pgAdapter),
	}, nil
}

// initializeAdapter configures and runs the initialization lifecycle step for a storage adapter.
//
// Timeout behaviour:
//   - If Read/Write timeouts are configured on StorageConfig, the larger of the two is used.
//   - If neither Read nor Write timeouts are configured (zero or negative), a 30s default is used.
//
// The longer default is intended to accommodate database connection establishment and schema
// verification, which may legitimately take longer than steady-state read/write operations.
//
// The provided ctx is used as the parent context; cancellation or deadline on ctx is propagated
// to the Initialize call (the computed timeout is applied on top of the parent).
func initializeAdapter(ctx context.Context, adapter lifecycleAdapter, config *ports.StorageConfig) error {
	initTimeout := config.Timeouts.Write
	if config.Timeouts.Read > initTimeout {
		initTimeout = config.Timeouts.Read
	}
	if initTimeout <= 0 {
		initTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()

	if err := adapter.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize storage adapter: %w", err)
	}

	return nil
}

// Close closes the underlying storage adapter.
func (a *Adapter) Close(ctx context.Context) error {
	return a.lifecycle.Close(ctx)
}

// HealthCheck verifies the underlying storage adapter health.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	return a.lifecycle.HealthCheck(ctx)
}

// Users returns the UserRepository interface implementation.
// Used for CRUD operations on user entities.
func (a *Adapter) Users() ports.UserRepository {
	return a.users
}

// Agents returns the AgentRepository interface implementation.
// Used for agent entity CRUD operations.
func (a *Adapter) Agents() ports.AgentRepository {
	return a.agents
}

// Services returns the ThirdpartyOAuth2ProviderRepository interface implementation.
// Used for OAuth2 service configuration CRUD operations.
func (a *Adapter) Services() ports.ThirdpartyOAuth2ProviderRepository {
	return a.providers
}

// UserGrants returns the UserGrantRepository interface implementation.
// Used for user grant CRUD operations.
func (a *Adapter) UserGrants() ports.UserGrantRepository {
	return a.userGrants
}

// UserSessions returns the UserSessionRepository interface implementation.
// Used for user session CRUD operations.
func (a *Adapter) UserSessions() ports.UserSessionRepository {
	return a.userSessions
}

// SessionRefresh returns the refresh transaction interface for user sessions.
func (a *Adapter) SessionRefresh() ports.UserSessionRefreshRepository {
	return a.sessionRefresh
}

// ToolApprovals returns the ToolApprovalRepository interface implementation.
func (a *Adapter) ToolApprovals() ports.ToolApprovalRepository {
	return a.toolApprovals
}

// ToolApprovalQueries returns the ToolApprovalQueryRepository interface implementation.
func (a *Adapter) ToolApprovalQueries() ports.ToolApprovalQueryRepository {
	return a.toolApprovalQueries
}

// ToolApprovalMetrics returns the ToolApprovalMetricsRepository interface implementation.
func (a *Adapter) ToolApprovalMetrics() ports.ToolApprovalMetricsRepository {
	return a.toolApprovalMetrics
}

// ApprovalSyncState returns the ApprovalSyncStateRepository interface implementation.
func (a *Adapter) ApprovalSyncState() ports.ApprovalSyncStateRepository {
	return a.approvalSyncState
}

// PermissionSets returns the PermissionSetRepository interface implementation.
// Used for permission set CRUD operations.
func (a *Adapter) PermissionSets() ports.PermissionSetRepository {
	return a.permissionSets
}

// BrokerCredentials returns the ClientCredentialRepository interface implementation.
func (a *Adapter) BrokerCredentials() ports.ClientCredentialRepository {
	return a.brokerCredentials
}

// SigningKeys returns the SigningKeyRepository interface implementation.
func (a *Adapter) SigningKeys() ports.SigningKeyRepository {
	return a.signingKeys
}

// SigningKeyBootstrapCoordinator returns the bootstrap coordinator for signing-key startup.
func (a *Adapter) SigningKeyBootstrapCoordinator() ports.SigningKeyBootstrapCoordinator {
	return a.signingKeys
}

// AuthorizationCodes returns the AuthorizationCodeRepository interface implementation.
func (a *Adapter) AuthorizationCodes() ports.AuthorizationCodeRepository {
	return a.authorizationCodes
}

// BeginTX starts the configured backend's transaction scope.
func (a *Adapter) BeginTX(ctx context.Context) (context.Context, error) {
	return a.transactions.BeginTX(ctx)
}

// Commit commits the configured backend's transaction scope.
func (a *Adapter) Commit(ctx context.Context) error {
	return a.transactions.Commit(ctx)
}

// Rollback rolls back the configured backend's transaction scope.
func (a *Adapter) Rollback(ctx context.Context) error {
	return a.transactions.Rollback(ctx)
}

// PKCESessions returns the PKCESessionRepository interface implementation.
func (a *Adapter) PKCESessions() ports.PKCESessionRepository {
	return a.pkceSessions
}

func (a *Adapter) RefreshSessions() ports.RefreshSessionRepository {
	return a.refreshSessions
}

func (a *Adapter) RefreshTokens() ports.RefreshTokenRepository {
	return a.refreshTokens
}

func (a *Adapter) RefreshRevocations() ports.RefreshSessionRevocationRepository {
	return a.refreshRevocations
}

func (a *Adapter) RefreshMaintenance() ports.RefreshSessionMaintenanceRepository {
	return a.refreshMaintenance
}

func (a *Adapter) RefreshMaintenanceControl() ports.RefreshSessionMaintenanceControl {
	return a.refreshMaintenanceControl
}

func (a *Adapter) AuthorizationClock() ports.AuthorizationClock {
	return a.authorizationClock
}

func (a *Adapter) AuthorizationCoordinator() ports.AuthorizationSessionCoordinator {
	return a.authorizationCoordinator
}
