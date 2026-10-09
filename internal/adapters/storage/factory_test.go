package storage

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdapter_Memory(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)

	require.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.Users())
}

func TestNewAdapter_MemoryExposesSigningKeyBootstrapCoordinator(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)
	require.NoError(t, err)

	coordinator := adapter.SigningKeyBootstrapCoordinator()
	require.NotNil(t, coordinator)

	called := false
	err = coordinator.WithBootstrapLock(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestNewAdapter_Postgres(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	// NewAdapter now initializes the adapter at creation time.
	// Without a running database, initialization must fail.
	adapter, err := NewAdapter(config)

	assert.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "failed to initialize storage adapter")
}

func TestNewAdapter_InvalidBackend(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "invalid",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)

	assert.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "unsupported storage backend")
}

func TestNewAdapter_PostgresNoURL(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)

	assert.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "connection URL cannot be empty")
}

// TestBackendSwitching verifies that different adapters can be created independently
func TestBackendSwitching(t *testing.T) {
	// Create memory adapter
	memConfig := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}
	memAdapter, err := NewAdapter(memConfig)
	require.NoError(t, err)
	require.NotNil(t, memAdapter)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create user in memory adapter
	userID := id.MustParseUserID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	user := &ports.User{
		ID:        userID,
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = memAdapter.Users().CreateUser(ctx, user)
	require.NoError(t, err)

	// Verify user exists in memory adapter
	retrieved, err := memAdapter.Users().GetUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, userID, retrieved.ID)

	// Memory adapter implements all repository interfaces
	assert.NotNil(t, memAdapter.Users())
	assert.NotNil(t, memAdapter.Agents())
	assert.NotNil(t, memAdapter.Services())
	assert.NotNil(t, memAdapter.UserGrants())
	assert.NotNil(t, memAdapter.UserSessions())

	// Close memory adapter
	err = memAdapter.Close(context.Background())
	assert.NoError(t, err)
}

// TestBackendSelectionValidation verifies backend selection validation during factory creation
func TestBackendSelectionValidation(t *testing.T) {
	tests := []struct {
		name       string
		backend    string
		shouldFail bool
	}{
		{
			name:       "valid memory backend",
			backend:    "memory",
			shouldFail: false,
		},
		{
			name:       "valid postgres backend",
			backend:    "postgres",
			shouldFail: true, // Init fails without a running database in unit tests
		},
		{
			name:       "invalid backend sqlite",
			backend:    "sqlite",
			shouldFail: true,
		},
		{
			name:       "invalid backend mongodb",
			backend:    "mongodb",
			shouldFail: true,
		},
		{
			name:       "empty backend",
			backend:    "",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ports.StorageConfig{
				Backend: tt.backend,
				Postgres: ports.PostgresConfig{
					ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
				},
				Timeouts: ports.StorageTimeouts{
					Read:  5 * time.Second,
					Write: 10 * time.Second,
				},
			}

			adapter, err := NewAdapter(config)

			if tt.shouldFail {
				assert.Error(t, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)
			}
		})
	}
}

func TestMemoryAdapter_IssuerChangeRejectsStaleAndActiveSessions(t *testing.T) {
	ctx := context.Background()
	adapter, err := NewAdapter(&ports.StorageConfig{Backend: "memory"})
	require.NoError(t, err)
	serviceID := id.NewServiceID()
	resourceURL := "https://mcp.example.test/mcp"
	completedAt := time.Now().UTC().Add(-time.Hour)
	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "MCP", ClientID: "registered-client", IssuerURI: "https://old-issuer.example.test",
		Secret: model.NewAbsentSecret(), TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
		Discovery:           model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: model.ClientBootstrapDCR},
		AuthorizationParams: map[string]string{"resource": resourceURL},
		DiscoveryStatus:     model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt},
	}
	require.NoError(t, adapter.Services().Create(ctx, provider))
	changed := provider.Copy()
	changed.IssuerURI = "https://new-issuer.example.test"
	nextAt := completedAt.Add(time.Minute)
	changed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &nextAt, LastSuccessAt: &nextAt}
	require.NoError(t, adapter.Services().Update(ctx, changed, &provider.Version))

	session := &domainstorage.UserSession{
		ID: id.NewSessionID(), Principal: id.Principal("issuer-race@example.test"), ServiceID: serviceID,
		EncryptedAccessToken: []byte("encrypted-token"), TokenType: "Bearer", Scope: []string{"read"},
		EncryptionContext: domainstorage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       nextAt, CreatedAt: nextAt, UpdatedAt: nextAt,
		ExpectedIssuerURI: provider.IssuerURI,
		ExpectedResource:  resourceURL,
	}
	err = adapter.UserSessions().Create(ctx, session)
	var storageErr *domainstorage.StorageError
	require.ErrorAs(t, err, &storageErr, "the old issuer's callback cannot insert after its service moves")
	assert.Equal(t, domainstorage.ErrorKindConflict, storageErr.Kind)

	session.ExpectedIssuerURI = changed.IssuerURI
	require.NoError(t, adapter.UserSessions().Create(ctx, session))
	third := changed.Copy()
	third.IssuerURI = "https://third-issuer.example.test"
	laterAt := nextAt.Add(time.Minute)
	third.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &laterAt, LastSuccessAt: &laterAt}
	err = adapter.Services().Update(ctx, third, &changed.Version)
	require.ErrorAs(t, err, &storageErr, "the final issuer update must recheck sessions")
	assert.Equal(t, domainstorage.ErrorKindConflict, storageErr.Kind)
	stored, err := adapter.Services().Get(ctx, serviceID)
	require.NoError(t, err)
	assert.Equal(t, changed.IssuerURI, stored.IssuerURI)
}

func TestMemoryAdapter_ConcurrentIssuerSwitchAndCallbackCannotCoexist(t *testing.T) {
	ctx := context.Background()
	for range 32 {
		adapter, err := NewAdapter(&ports.StorageConfig{Backend: "memory"})
		require.NoError(t, err)
		serviceID := id.NewServiceID()
		resourceURL := "https://mcp.example.test/mcp"
		initialAt := time.Now().UTC().Add(-time.Hour)
		provider := &model.ThirdpartyOAuth2ProviderEntity{
			ID: serviceID, DisplayName: "MCP", ClientID: "registered-client", IssuerURI: "https://old-issuer.example.test",
			Secret: model.NewAbsentSecret(), TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
			Discovery:           model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: model.ClientBootstrapDCR},
			AuthorizationParams: map[string]string{"resource": resourceURL},
			DiscoveryStatus:     model.DiscoveryStatus{LastAttemptAt: &initialAt, LastSuccessAt: &initialAt},
		}
		require.NoError(t, adapter.Services().Create(ctx, provider))
		changed := provider.Copy()
		changed.IssuerURI = "https://new-issuer.example.test"
		readyAt := initialAt.Add(time.Minute)
		changed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &readyAt, LastSuccessAt: &readyAt}
		session := &domainstorage.UserSession{
			ID: id.NewSessionID(), Principal: id.Principal("concurrent@example.test"), ServiceID: serviceID,
			EncryptedAccessToken: []byte("encrypted-token"), TokenType: "Bearer", Scope: []string{"read"},
			EncryptionContext: domainstorage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt:       readyAt, CreatedAt: readyAt, UpdatedAt: readyAt,
			ExpectedIssuerURI: provider.IssuerURI,
			ExpectedResource:  resourceURL,
		}
		start := make(chan struct{})
		insertResult := make(chan error, 1)
		changeResult := make(chan error, 1)
		go func() { <-start; insertResult <- adapter.UserSessions().Create(ctx, session) }()
		go func() { <-start; changeResult <- adapter.Services().Update(ctx, changed, &provider.Version) }()
		close(start)
		insertErr, changeErr := <-insertResult, <-changeResult
		count, err := adapter.UserSessions().CountByService(ctx, serviceID)
		require.NoError(t, err)
		stored, err := adapter.Services().Get(ctx, serviceID)
		require.NoError(t, err)
		if changeErr == nil {
			require.Error(t, insertErr)
			assert.Zero(t, count)
			assert.Equal(t, changed.IssuerURI, stored.IssuerURI)
		} else {
			require.NoError(t, insertErr)
			require.ErrorIs(t, changeErr, domainstorage.ErrIssuerChangeHasSessions)
			assert.Equal(t, 1, count)
			assert.Equal(t, provider.IssuerURI, stored.IssuerURI)
		}
	}
}

func TestMemoryAdapter_StaleAudienceCannotInsertOrReplaceSession(t *testing.T) {
	ctx := context.Background()
	adapter, err := NewAdapter(&ports.StorageConfig{Backend: "memory"})
	require.NoError(t, err)
	serviceID := id.NewServiceID()
	const audienceA = "https://mcp.example.test/a"
	const audienceB = "https://mcp.example.test/b"
	resourceA := audienceA
	initialAt := time.Now().UTC().Add(-time.Hour)
	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "MCP", ClientID: "registered-client", IssuerURI: "https://issuer.example.test",
		Secret: model.NewAbsentSecret(), TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
		Discovery:           model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceA, ClientMethod: model.ClientBootstrapDCR},
		AuthorizationParams: map[string]string{"resource": audienceA},
		DiscoveryStatus:     model.DiscoveryStatus{LastAttemptAt: &initialAt, LastSuccessAt: &initialAt},
	}
	require.NoError(t, adapter.Services().Create(ctx, provider))
	principal := id.Principal("callback@example.test")
	stale := &domainstorage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: []byte("stale-access"), EncryptedRefreshToken: []byte("stale-refresh"),
		TokenType: "Bearer", Scope: []string{"read"},
		EncryptionContext: domainstorage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       initialAt, CreatedAt: initialAt, UpdatedAt: initialAt,
		ExpectedIssuerURI: provider.IssuerURI, ExpectedResource: audienceA,
	}
	changed := provider.Copy()
	resourceB := audienceB
	changed.Discovery.ResourceURL = &resourceB
	changed.AuthorizationParams["resource"] = audienceB
	committedAt := initialAt.Add(time.Minute)
	changed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &committedAt, LastSuccessAt: &committedAt}
	require.NoError(t, adapter.Services().Update(ctx, changed, &provider.Version))

	err = adapter.UserSessions().Create(ctx, stale)
	var storageErr *domainstorage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, domainstorage.ErrorKindConflict, storageErr.Kind)
	count, err := adapter.UserSessions().CountByService(ctx, serviceID)
	require.NoError(t, err)
	assert.Zero(t, count)

	current := *stale
	current.ExpectedResource = audienceB
	current.EncryptedAccessToken = []byte("current-access")
	current.EncryptedRefreshToken = []byte("current-refresh")
	require.NoError(t, adapter.UserSessions().Create(ctx, &current))
	err = adapter.UserSessions().Create(ctx, stale)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, domainstorage.ErrorKindConflict, storageErr.Kind)
	stored, err := adapter.UserSessions().FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, current.ID, stored.ID)
	assert.Equal(t, current.EncryptedAccessToken, stored.EncryptedAccessToken)
	assert.Equal(t, current.EncryptedRefreshToken, stored.EncryptedRefreshToken)
	assert.Empty(t, stored.ExpectedIssuerURI)
	assert.Empty(t, stored.ExpectedResource)
}
