package oauth2server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	dstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// newTestProvider creates a Provider with in-memory storage and test encryption
// for unit testing purposes.
func newTestProvider(t *testing.T) (*Provider, *memory.AgentRepository, *SigningKeyService) {
	t.Helper()
	codeRepo := memory.NewAuthorizationCodeStore()
	credRepo := memory.NewClientCredentialStore()
	agentRepo := memory.NewAgentRepository()
	signingKeyRepo := memory.NewSigningKeyStore()
	enc := &testEncryptor{}
	pkceRepo := memory.NewPKCESessionStore()
	refreshDeps, refreshStore := newNativeTestRefresh(agentRepo, credRepo, codeRepo, pkceRepo, enc)
	logger := testSlogger()

	signingKeySvc := NewSigningKeyService(signingKeyRepo, signingKeyRepo, enc, newNoopBranchKeyManager(), logger)

	provider, err := NewProvider(
		codeRepo,
		refreshDeps,
		pkceRepo,
		credRepo,
		&testClientResolver{agentRepo: agentRepo},
		signingKeySvc,
		"https://broker.example.com",
		time.Hour, // 1h TTL
		"",        // no CEL expression
		logger,
		refreshStore,
	)
	require.NoError(t, err)

	_, err = signingKeySvc.generateAndStore(context.Background(), "ES256", true, time.Now())
	require.NoError(t, err)

	return provider, agentRepo, signingKeySvc
}

type testGrantVerifier struct{ grants ports.UserGrantRepository }

func (v testGrantVerifier) VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time) (ports.UserDelegationDecision, error) {
	grant, err := v.grants.FindByPrincipalAndAgent(ctx, principal, agentID)
	if ports.IsNotFoundErr(err) || (err == nil && grant == nil) {
		return ports.UserDelegationDecision{Status: ports.UserDelegationMissing}, nil
	}
	if err != nil {
		return ports.UserDelegationDecision{}, err
	}
	if !grant.IsActive(at) {
		return ports.UserDelegationDecision{Status: ports.UserDelegationExpired}, nil
	}
	return ports.UserDelegationDecision{Status: ports.UserDelegationActive, GrantID: grant.ID, ValidUntil: grant.ValidUntil}, nil
}

type testRefreshBranchKeyManager struct{}

func (testRefreshBranchKeyManager) Create(_ context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	return "refresh_" + subject.Identifier() + "_branch_key", nil
}

func newNativeTestRefresh(agentRepo *memory.AgentRepository, credRepo *memory.ClientCredentialStore, codeRepo *memory.AuthorizationCodeStore, pkceRepo *memory.PKCESessionStore, encryption ports.EncryptionPort) (RefreshSessionDependencies, *memory.RefreshSessionStore) {
	grants := memory.NewUserGrantRepository()
	refreshStore := memory.NewRefreshSessionStore(agentRepo, grants, credRepo, codeRepo, pkceRepo)
	return RefreshSessionDependencies{
		Agents:      agentRepo,
		Sessions:    refreshStore,
		Tokens:      memory.NewRefreshTokenStore(refreshStore),
		Revocations: refreshStore,
		Coordinator: refreshStore,
		Clock:       refreshStore,
		Verifier:    testGrantVerifier{grants: grants},
		Encryption:  encryption,
		BranchKeys:  testRefreshBranchKeyManager{},
		Policy: dstorage.RefreshSessionPolicy{
			InactivityLifetime: time.Hour,
		},
	}, refreshStore
}

func seedProviderTestGrant(t *testing.T, provider *Provider, agentID id.AgentID) {
	t.Helper()
	verifier := provider.refreshDeps.Verifier.(testGrantVerifier)
	require.NoError(t, verifier.grants.Create(context.Background(), &dstorage.UserGrant{
		ID: id.NewGrantID(), Principal: id.NewPrincipal("user@example.com"), AgentID: agentID,
	}))
}

// setupTestCredentials creates an agent with broker credentials and returns the agent, credential, and plaintext secret.
func setupTestCredentials(t *testing.T, provider *Provider, agentRepo ports.AgentRepository) (*dstorage.Agent, *dstorage.ClientCredential, string) {
	t.Helper()
	ctx := context.Background()

	// Create agent
	agent := &dstorage.Agent{
		ID:             id.NewAgentID(),
		DisplayName:    "Test OAuth2 Agent",
		Description:    "Agent for provider test",
		PermissionSets: testAgentPermissionSets(),
	}
	err := agentRepo.Create(ctx, agent)
	require.NoError(t, err)

	// Generate credentials
	cred, plaintext, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)

	err = provider.fositeStorage.credRepo.Create(ctx, cred)
	require.NoError(t, err)

	seedProviderTestGrant(t, provider, agent.ID)
	return agent, cred, plaintext
}

func TestNewProvider_RequiresNativeRefreshDependencies(t *testing.T) {
	provider, _, signer := newTestProvider(t)
	newWith := func(deps RefreshSessionDependencies, tx ports.StorageTransactionManager) error {
		_, err := NewProvider(provider.fositeStorage.codeRepo, deps, provider.fositeStorage.pkceRepo,
			provider.fositeStorage.credRepo, provider.fositeStorage.clientResolver,
			signer, "https://broker.example.com", time.Hour, "", testSlogger(), tx)
		return err
	}
	transactions := provider.fositeStorage.transactions
	for _, missing := range []struct {
		name string
		drop func(*RefreshSessionDependencies)
	}{
		{name: "agent", drop: func(d *RefreshSessionDependencies) { d.Agents = nil }},
		{name: "clock", drop: func(d *RefreshSessionDependencies) { d.Clock = nil }},
		{name: "verifier", drop: func(d *RefreshSessionDependencies) { d.Verifier = nil }},
		{name: "encryption", drop: func(d *RefreshSessionDependencies) { d.Encryption = nil }},
	} {
		t.Run(missing.name, func(t *testing.T) {
			deps := provider.refreshDeps
			missing.drop(&deps)
			require.Error(t, newWith(deps, transactions))
		})
	}
	require.Error(t, newWith(provider.refreshDeps, nil))
	deps := provider.refreshDeps
	deps.Policy.InactivityLifetime = 0
	require.Error(t, newWith(deps, transactions))
}

func TestProvider_HandleClientCredentials(t *testing.T) {
	t.Run("valid credentials return signed JWT", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)

		resp, err := provider.HandleClientCredentials(
			context.Background(),
			agent.ID.String(),
			plaintext,
			"read write",
		)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.NotEmpty(t, resp.AccessToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.True(t, resp.ExpiresIn > 0)
		assert.Equal(t, "read write", resp.Scope)
	})

	t.Run("invalid credentials return error", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)

		_, err := provider.HandleClientCredentials(
			context.Background(),
			agent.ID.String(),
			"wrong-secret",
			"read",
		)
		assert.ErrorIs(t, err, ErrInvalidClient)
	})

	t.Run("unknown agent_id returns error", func(t *testing.T) {
		provider, _, _ := newTestProvider(t)

		_, err := provider.HandleClientCredentials(
			context.Background(),
			id.NewAgentID().String(),
			"secret",
			"read",
		)
		assert.ErrorIs(t, err, ErrInvalidClient)
	})

	t.Run("empty scope works", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)

		resp, err := provider.HandleClientCredentials(
			context.Background(),
			agent.ID.String(),
			plaintext,
			"",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
	})
}

func TestProvider_HandleAuthorize(t *testing.T) {
	t.Run("redirect_uri exact match required", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost/callback/extra", // has extra path segment
			"code",
			"read",
			"state",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRedirectURI)
	})

	t.Run("empty redirect_uris list rejects", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		// Agent created by setupTestCredentials has no RedirectURIs (empty slice)

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRedirectURI)
	})

	t.Run("valid request returns authorization code", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		// Update agent with redirect URIs
		_ = agentRepo.Update(context.Background(), agent)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"test-state-123",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)
		assert.NotEmpty(t, code)
	})

	t.Run("allowed scope exact match succeeds even when redirect URI matching is port-agnostic", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:3000/callback"}
		agent.AllowedScopes = []string{"repo", "user"}
		require.NoError(t, agentRepo.Update(context.Background(), agent))

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:52341/callback",
			"code",
			"repo",
			"state",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)
		assert.NotEmpty(t, code)
	})

	t.Run("disallowed scope is rejected when allowed scopes are configured", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:3000/callback"}
		agent.AllowedScopes = []string{"repo", "user"}
		require.NoError(t, agentRepo.Update(context.Background(), agent))

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:52341/callback",
			"code",
			"admin",
			"state",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidScope)
	})

	t.Run("missing code_challenge rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			"", // empty code_challenge
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRequest)
	})

	t.Run("empty code_challenge_method rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			"challenge123",
			"", // missing method — must not default to plain
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRequest)
	})

	t.Run("plain code_challenge_method rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			"challenge123",
			"plain",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRequest)
	})

	t.Run("unregistered redirect_uri rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, _ := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		_, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://evil.example.com/callback", // not registered
			"code",
			"read",
			"state",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRedirectURI)

		var fositeErr *fosite.RFC6749Error
		assert.False(t, errors.As(err, &fositeErr))
	})

	t.Run("unknown agent_id rejected", func(t *testing.T) {
		provider, _, _ := newTestProvider(t)

		_, err := provider.HandleAuthorize(
			context.Background(),
			id.NewAgentID().String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			"challenge123",
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidClient)
	})
}

func TestProvider_HandleAuthorize_AllowsOfflineAccessReservedScope(t *testing.T) {
	provider, agentRepo, _ := newTestProvider(t)
	agent, _, _ := setupTestCredentials(t, provider, agentRepo)
	agent.RedirectURIs = []string{"http://localhost:8080/callback"}
	agent.AllowedScopes = []string{"read"}
	require.NoError(t, agentRepo.Update(context.Background(), agent))

	code, err := provider.HandleAuthorize(
		context.Background(),
		agent.ID.String(),
		"http://localhost:8080/callback",
		"code",
		"read offline_access",
		"state",
		generateS256Challenge("offline-access-verifier-1234567890123456789"),
		"S256",
		id.NewPrincipal("user@example.com"),
	)
	require.NoError(t, err)
	assert.NotEmpty(t, code)
}

func TestProvider_RefreshTokens(t *testing.T) {
	t.Run("offline_access yields refresh token and refresh grant rotates it", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		require.NoError(t, agentRepo.Update(context.Background(), agent))

		verifier := "offline-access-verifier-123456789012345678901"
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read offline_access",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			verifier,
		)
		require.NoError(t, err)
		require.NotEmpty(t, resp.RefreshToken)

		refreshed, err := provider.HandleRefreshToken(context.Background(), agent.ID.String(), plaintext, resp.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, refreshed.AccessToken)
		require.NotEmpty(t, refreshed.RefreshToken)
		assert.NotEqual(t, resp.RefreshToken, refreshed.RefreshToken)

		_, err = provider.HandleRefreshToken(context.Background(), agent.ID.String(), plaintext, resp.RefreshToken, "")
		assert.ErrorIs(t, err, ErrInvalidGrant)
		_, err = provider.HandleRefreshToken(context.Background(), agent.ID.String(), plaintext, refreshed.RefreshToken, "")
		assert.ErrorIs(t, err, ErrInvalidGrant)
		codeRecord, err := provider.fositeStorage.codeRepo.FindByCodeHash(context.Background(), sha256Hex(code))
		require.NoError(t, err)
		rootID, err := id.ParseRefreshSessionID(codeRecord.ID.String())
		require.NoError(t, err)
		root, err := provider.refreshDeps.Sessions.FindByID(context.Background(), rootID)
		require.NoError(t, err)
		require.NotNil(t, root.TerminalReason)
		assert.Equal(t, dstorage.RefreshReasonProhibitedReuse, *root.TerminalReason)
	})

	t.Run("missing offline_access omits refresh token", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		require.NoError(t, agentRepo.Update(context.Background(), agent))

		verifier := "no-offline-access-verifier-1234567890123456789"
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			verifier,
		)
		require.NoError(t, err)
		assert.Empty(t, resp.RefreshToken)
	})

	t.Run("cimd client without refresh_token grant gets no refresh token", func(t *testing.T) {
		codeRepo := memory.NewAuthorizationCodeStore()
		agentRepo := memory.NewAgentRepository()
		credRepo := memory.NewClientCredentialStore()
		signingKeyRepo := memory.NewSigningKeyStore()
		enc := &testEncryptor{}
		logger := testSlogger()
		pkceRepo := memory.NewPKCESessionStore()
		refreshDeps, refreshStore := newNativeTestRefresh(agentRepo, credRepo, codeRepo, pkceRepo, enc)
		agent := &dstorage.Agent{
			ID:             id.NewAgentID(),
			ClientID:       nil,
			ClientURIs:     []string{"https://client.example.com/metadata.json"},
			DisplayName:    "CIMD Agent",
			Description:    "CIMD agent without refresh grant",
			PermissionSets: testAgentPermissionSets(),
		}
		require.NoError(t, agentRepo.Create(context.Background(), agent))
		require.NoError(t, refreshDeps.Verifier.(testGrantVerifier).grants.Create(context.Background(), &dstorage.UserGrant{
			ID: id.NewGrantID(), Principal: id.NewPrincipal("user@example.com"), AgentID: agent.ID,
		}))
		resolver := &mockClientResolver{resolveFunc: func(ctx context.Context, clientID id.ClientID) (*ports.ClientResolution, error) {
			currentAgent, err := agentRepo.Get(ctx, agent.ID)
			if err != nil {
				return nil, err
			}
			return &ports.ClientResolution{
				Agent: currentAgent,
				CIMDMetadata: &ports.CIMDMetadataDTO{
					ClientID:     string(clientID),
					RedirectURIs: []string{"https://client.example.com/callback"},
					GrantTypes:   []string{"authorization_code"},
				},
			}, nil
		}}

		signingKeySvc := NewSigningKeyService(signingKeyRepo, signingKeyRepo, enc, newNoopBranchKeyManager(), logger)
		provider, err := NewProvider(
			codeRepo,
			refreshDeps,
			pkceRepo,
			credRepo,
			resolver,
			signingKeySvc,
			"https://broker.example.com",
			time.Hour,
			"",
			logger,
			refreshStore,
		)
		require.NoError(t, err)
		_, err = signingKeySvc.generateAndStore(context.Background(), "ES256", true, time.Now())
		require.NoError(t, err)

		verifier := "cimd-no-refresh-verifier-123456789012345678901"
		challenge := generateS256Challenge(verifier)
		clientID := "https://client.example.com/metadata.json"

		code, err := provider.HandleAuthorize(
			context.Background(),
			clientID,
			"https://client.example.com/callback",
			"code",
			"offline_access",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(),
			clientID,
			"",
			code,
			"https://client.example.com/callback",
			verifier,
		)
		require.NoError(t, err)
		assert.Empty(t, resp.RefreshToken)
	})
}

func TestProvider_AuthenticatedReplayBoundaries(t *testing.T) {
	ctx := context.Background()
	provider, agents, _ := newTestProvider(t)
	owner, _, ownerSecret := setupTestCredentials(t, provider, agents)
	other, _, otherSecret := setupTestCredentials(t, provider, agents)
	owner.RedirectURIs = []string{"http://localhost:8080/callback"}
	require.NoError(t, agents.Update(ctx, owner))

	verifier := "refresh-replay-boundary-verifier-1234567890123456"
	code, err := provider.HandleAuthorize(ctx, owner.ID.String(), owner.RedirectURIs[0], "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", id.NewPrincipal("user@example.com"))
	require.NoError(t, err)
	issued, err := provider.HandleAuthorizationCodeExchange(ctx, owner.ID.String(), ownerSecret, code, owner.RedirectURIs[0], verifier)
	require.NoError(t, err)
	require.NotEmpty(t, issued.RefreshToken)
	rotated, err := provider.HandleRefreshToken(ctx, owner.ID.String(), ownerSecret, issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, rotated.RefreshToken)

	// An authenticated stranger cannot revoke the owner's root, even with a
	// consumed predecessor. The owner's current successor must still rotate.
	_, err = provider.HandleRefreshToken(ctx, other.ID.String(), otherSecret, issued.RefreshToken, "")
	require.ErrorIs(t, err, ErrInvalidGrant)
	current, err := provider.HandleRefreshToken(ctx, owner.ID.String(), ownerSecret, rotated.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, current.RefreshToken)

	// Replaying a used authorization code is deliberately different: Fosite
	// revokes the root belonging to the original code, not the replaying client.
	_, err = provider.HandleAuthorizationCodeExchange(ctx, other.ID.String(), otherSecret, code, owner.RedirectURIs[0], verifier)
	require.ErrorIs(t, err, ErrInvalidGrant)
	codeRecord, err := provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	rootID, err := id.ParseRefreshSessionID(codeRecord.ID.String())
	require.NoError(t, err)
	root, err := provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	require.NoError(t, err)
	require.NotNil(t, root.TerminalReason)
	assert.Equal(t, dstorage.RefreshReasonCodeReplay, *root.TerminalReason)
	_, err = provider.HandleRefreshToken(ctx, owner.ID.String(), ownerSecret, current.RefreshToken, "")
	require.ErrorIs(t, err, ErrInvalidGrant)
}

func TestProvider_CodeExchangeRequiresActiveOriginalGrant(t *testing.T) {
	ctx := context.Background()
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	agent.RedirectURIs = []string{"http://localhost:8080/callback"}
	require.NoError(t, agents.Update(ctx, agent))
	verifier := "original-grant-verifier-1234567890123456789012"
	code, err := provider.HandleAuthorize(ctx, agent.ID.String(), agent.RedirectURIs[0], "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", id.NewPrincipal("user@example.com"))
	require.NoError(t, err)
	grants := provider.refreshDeps.Verifier.(testGrantVerifier).grants
	grant, err := grants.FindByPrincipalAndAgent(ctx, id.NewPrincipal("user@example.com"), agent.ID)
	require.NoError(t, err)
	require.NoError(t, grants.Delete(ctx, grant.ID))

	response, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, agent.RedirectURIs[0], verifier)
	require.ErrorIs(t, err, ErrInvalidGrant)
	assert.Nil(t, response)
	recorded, err := provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	assert.Nil(t, recorded.UsedAt)
	rootID, err := id.ParseRefreshSessionID(recorded.ID.String())
	require.NoError(t, err)
	_, err = provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	assert.True(t, ports.IsNotFoundErr(err))
}

func TestProvider_HandleAuthorizationCodeExchange(t *testing.T) {
	t.Run("valid code exchange returns token", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		// First, get an authorization code with a known verifier
		verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)
		require.NotEmpty(t, code)

		// Exchange the code
		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			verifier,
		)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.AccessToken)
		assert.Equal(t, "Bearer", resp.TokenType)
	})

	t.Run("portless loopback registration survives authorize and exchange on ephemeral port", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost/callback"}
		require.NoError(t, agentRepo.Update(context.Background(), agent))

		verifier := "loopback-ephemeral-port-verifier-abcdefghij"
		challenge := generateS256Challenge(verifier)
		redirectURI := "http://localhost:52341/callback"

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			redirectURI,
			"code",
			"read",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)
		require.NotEmpty(t, code)

		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			redirectURI,
			verifier,
		)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.AccessToken)
		assert.Equal(t, "Bearer", resp.TokenType)
	})

	t.Run("code replay rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		verifier := "replay-code-verifier-xxxxxxxxxxxxxxxxxxxxxxxxxxx" // 43 chars (RFC 7636 minimum)
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		// First exchange succeeds
		_, err = provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			verifier,
		)
		require.NoError(t, err)

		// Second exchange (replay) should fail
		_, err = provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			verifier,
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidGrant)
	})

	for _, tt := range []struct {
		name       string
		lifetime   time.Duration
		unfiltered bool
	}{
		{name: "active code with valid PKCE", lifetime: time.Minute},
		{name: "expired code rejects", lifetime: -time.Minute},
		{name: "expired code from unfiltered repository rejects", lifetime: -time.Minute, unfiltered: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider, agentRepo, _ := newTestProvider(t)
			agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
			agent.RedirectURIs = []string{"http://localhost:8080/callback"}
			require.NoError(t, agentRepo.Update(context.Background(), agent))

			verifier := "expiry-code-verifier-xxxxxxxxxxxxxxxxxxxxxxxxxxx"
			challenge := generateS256Challenge(verifier)
			rawCode := "expiry-code-value"
			codeHash := sha256Hex(rawCode)
			code := &dstorage.AuthorizationCode{
				ID:            id.NewAuthorizationCodeID(),
				CodeHash:      codeHash,
				AgentID:       agent.ID,
				ClientID:      id.NewClientID(agent.ID.String()),
				Principal:     id.NewPrincipal("user@example.com"),
				RedirectURI:   "http://localhost:8080/callback",
				CodeChallenge: challenge,
				Scope:         "read",
				ExpiresAt:     time.Now().Add(tt.lifetime),
				CreatedAt:     time.Now().Add(-2 * time.Minute),
			}
			require.NoError(t, provider.fositeStorage.codeRepo.Create(context.Background(), code))
			require.NoError(t, provider.fositeStorage.pkceRepo.Create(context.Background(), &dstorage.PKCESession{
				Signature: codeHash, CodeChallenge: challenge, CodeChallengeMethod: "S256",
				ExpiresAt: time.Now().Add(time.Minute), CreatedAt: code.CreatedAt,
			}))
			if tt.unfiltered {
				provider.fositeStorage.codeRepo = &mockCodeRepo{
					findByCodeHashFunc: func(context.Context, string) (*dstorage.AuthorizationCode, error) {
						return code, nil
					},
				}
			}

			resp, err := provider.HandleAuthorizationCodeExchange(
				context.Background(), agent.ID.String(), plaintext, rawCode, code.RedirectURI, verifier,
			)
			if tt.lifetime > 0 {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.NotEmpty(t, resp.AccessToken)
			} else {
				require.ErrorIs(t, err, ErrInvalidGrant)
				var oauthErr *RFC6749Error
				require.ErrorAs(t, err, &oauthErr)
				assert.Equal(t, "invalid_grant", oauthErr.Code())
				assert.Equal(t, 400, oauthErr.HTTPStatus())
				assert.Nil(t, resp)
			}
		})
	}

	t.Run("redirect_uri substitution rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback", "https://attacker.example.com/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		verifier := "test-verifier-for-uri-substitution"
		challenge := generateS256Challenge(verifier)

		// Authorization was issued to the legitimate URI
		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		// Attacker substitutes a different registered URI at token exchange (RFC 6749 §4.1.3)
		_, err = provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"https://attacker.example.com/callback",
			verifier,
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidGrant)
	})

	t.Run("PKCE mismatch rejected", func(t *testing.T) {
		provider, agentRepo, _ := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		verifier := "correct-verifier"
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		// Exchange with wrong verifier
		_, err = provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			"wrong-verifier",
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidGrant)
	})
}

// TestProvider_HandleAuthorizationCodeExchange_ConcurrentReplay verifies that when MarkUsed
// returns ErrorKindNotFound (the DB-level race guard fired — a concurrent request won and
// set used_at before ours could), the exchange returns ErrInvalidGrant rather than a 500.
func TestProvider_HandleAuthorizationCodeExchange_ConcurrentReplay(t *testing.T) {
	provider, agentRepo, _ := newTestProvider(t)
	agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
	agent.RedirectURIs = []string{"http://localhost:8080/callback"}
	_ = agentRepo.Update(context.Background(), agent)

	verifier := "concurrent-replay-verifier"
	challenge := generateS256Challenge(verifier)

	code, err := provider.HandleAuthorize(
		context.Background(),
		agent.ID.String(),
		"http://localhost:8080/callback",
		"code",
		"read",
		"state",
		challenge,
		"S256",
		id.NewPrincipal("user@example.com"),
	)
	require.NoError(t, err)

	// Replace codeRepo with a wrapper that simulates the race: FindByCodeHash returns the
	// unused code (used_at IS NULL), but MarkUsed returns ErrorKindNotFound (zero rows
	// affected because a concurrent request already set used_at in the database).
	provider.fositeStorage.codeRepo = &raceCodeRepo{
		delegate:      provider.fositeStorage.codeRepo,
		markUsedError: dstorage.NewStorageError("AuthorizationCodeRepo.MarkUsed", dstorage.ErrorKindNotFound, nil, "authorization code not found or already used"),
	}

	_, err = provider.HandleAuthorizationCodeExchange(
		context.Background(),
		agent.ID.String(),
		plaintext,
		code,
		"http://localhost:8080/callback",
		verifier,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidGrant)
}

// raceCodeRepo wraps an AuthorizationCodeRepository to inject a fixed error on MarkUsed,
// simulating the concurrent replay scenario where the DB guard fires first.
type raceCodeRepo struct {
	delegate      ports.AuthorizationCodeRepository
	markUsedError error
}

func (r *raceCodeRepo) Create(ctx context.Context, code *dstorage.AuthorizationCode) error {
	return r.delegate.Create(ctx, code)
}

func (r *raceCodeRepo) FindByCodeHash(ctx context.Context, codeHash string) (*dstorage.AuthorizationCode, error) {
	return r.delegate.FindByCodeHash(ctx, codeHash)
}

func (r *raceCodeRepo) MarkUsed(_ context.Context, _ id.AuthorizationCodeID) error {
	return r.markUsedError
}

func (r *raceCodeRepo) DeleteExpired(ctx context.Context) (int, error) {
	return r.delegate.DeleteExpired(ctx)
}

// TestProvider_AccessToken_ClaimsAndSignature verifies that tokens issued via both
// grant flows carry all required claims and a verifiable ECDSA signature.
func TestProvider_AccessToken_ClaimsAndSignature(t *testing.T) {
	const issuer = "https://broker.example.com"

	verifyToken := func(t *testing.T, svc *SigningKeyService, tokenStr, wantSub, wantAgentID, wantScope string) {
		t.Helper()
		ctx := context.Background()

		jwks, err := svc.BuildJWKS(ctx)
		require.NoError(t, err)

		tok, err := jwt.Parse([]byte(tokenStr), jwt.WithKeySet(jwks))
		require.NoError(t, err, "JWT signature must verify against the JWKS public key")

		iss, ok := tok.Issuer()
		require.True(t, ok, "iss must be present")
		assert.Equal(t, issuer, iss)

		sub, ok := tok.Subject()
		require.True(t, ok, "sub must be present")
		assert.Equal(t, wantSub, sub)

		iat, ok := tok.IssuedAt()
		require.True(t, ok, "iat must be present")
		assert.False(t, iat.IsZero())

		exp, ok := tok.Expiration()
		require.True(t, ok, "exp must be present")
		assert.True(t, exp.After(time.Now()), "exp must be in the future")

		jti, ok := tok.JwtID()
		require.True(t, ok, "jti must be present")
		assert.NotEmpty(t, jti)

		gotAgentID, err := jwt.Get[string](tok, "agent_id")
		require.NoError(t, err, "agent_id must be present")
		assert.Equal(t, wantAgentID, gotAgentID)

		scope, err := jwt.Get[string](tok, "scope")
		require.NoError(t, err, "scope must be present")
		assert.Equal(t, wantScope, scope)
	}

	t.Run("client_credentials flow", func(t *testing.T) {
		provider, agentRepo, svc := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)

		resp, err := provider.HandleClientCredentials(context.Background(), agent.ID.String(), plaintext, "read write")
		require.NoError(t, err)

		// In client_credentials, sub and agent_id are both the agent UUID (brokerClient.GetID() returns agent.ID).
		verifyToken(t, svc, resp.AccessToken, agent.ID.String(), agent.ID.String(), "read write")
	})

	t.Run("authorization_code flow", func(t *testing.T) {
		provider, agentRepo, svc := newTestProvider(t)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		verifier := "pkce-verifier-for-claims-test-abcdefghijklm" // 43 chars (RFC 7636 minimum)
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(),
			agent.ID.String(),
			"http://localhost:8080/callback",
			"code",
			"read write",
			"state",
			challenge,
			"S256",
			id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(),
			agent.ID.String(),
			plaintext,
			code,
			"http://localhost:8080/callback",
			verifier,
		)
		require.NoError(t, err)

		// agent_id is the agent UUID; sub is the authenticated principal.
		verifyToken(t, svc, resp.AccessToken, "user@example.com", agent.ID.String(), "read write")
	})
}

// TestProvider_CEL_RequestGrantType verifies that request.grant_type is correctly
// populated for both client_credentials and authorization_code flows end-to-end.
func TestProvider_CEL_RequestGrantType(t *testing.T) {
	newProviderWithCEL := func(t *testing.T, expr string) (*Provider, *memory.AgentRepository) {
		t.Helper()
		codeRepo := memory.NewAuthorizationCodeStore()
		credRepo := memory.NewClientCredentialStore()
		agentRepo := memory.NewAgentRepository()
		signingKeyRepo := memory.NewSigningKeyStore()
		enc := &testEncryptor{}
		pkceRepo := memory.NewPKCESessionStore()
		refreshDeps, refreshStore := newNativeTestRefresh(agentRepo, credRepo, codeRepo, pkceRepo, enc)
		logger := testSlogger()

		svc := NewSigningKeyService(signingKeyRepo, signingKeyRepo, enc, newNoopBranchKeyManager(), logger)
		p, err := NewProvider(codeRepo, refreshDeps, pkceRepo, credRepo, &testClientResolver{agentRepo: agentRepo}, svc, "https://broker.example.com", time.Hour, expr, logger, refreshStore)
		require.NoError(t, err)

		_, err = svc.generateAndStore(context.Background(), "ES256", true, time.Now())
		require.NoError(t, err)
		return p, agentRepo
	}

	getCustomClaim := func(t *testing.T, tokenStr, claim string) string {
		t.Helper()
		tok, err := jwt.Parse([]byte(tokenStr), jwt.WithValidate(false), jwt.WithVerify(false))
		require.NoError(t, err)
		v, err := jwt.Get[string](tok, claim)
		require.NoError(t, err)
		return v
	}

	t.Run("client_credentials grant populates request.grant_type", func(t *testing.T) {
		provider, agentRepo := newProviderWithCEL(t, `{"grant": request.grant_type}`)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)

		resp, err := provider.HandleClientCredentials(context.Background(), agent.ID.String(), plaintext, "read")
		require.NoError(t, err)

		assert.Equal(t, "client_credentials", getCustomClaim(t, resp.AccessToken, "grant"))
	})

	t.Run("authorization_code grant populates request.grant_type", func(t *testing.T) {
		provider, agentRepo := newProviderWithCEL(t, `{"grant": request.grant_type}`)
		agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
		agent.RedirectURIs = []string{"http://localhost:8080/callback"}
		_ = agentRepo.Update(context.Background(), agent)

		verifier := "pkce-verifier-for-grant-type-test-abcdefghi" // 43 chars
		challenge := generateS256Challenge(verifier)

		code, err := provider.HandleAuthorize(
			context.Background(), agent.ID.String(),
			"http://localhost:8080/callback", "code", "read", "state",
			challenge, "S256", id.NewPrincipal("user@example.com"),
		)
		require.NoError(t, err)

		resp, err := provider.HandleAuthorizationCodeExchange(
			context.Background(), agent.ID.String(), plaintext,
			code, "http://localhost:8080/callback", verifier,
		)
		require.NoError(t, err)

		assert.Equal(t, "authorization_code", getCustomClaim(t, resp.AccessToken, "grant"))
	})
}

func TestProvider_CEL_AudienceListClaimFailsAuthorizationCodeExchange(t *testing.T) {
	providerWithCEL := func(t *testing.T, expr string) (*Provider, *memory.AgentRepository) {
		t.Helper()
		codeRepo := memory.NewAuthorizationCodeStore()
		credRepo := memory.NewClientCredentialStore()
		agentRepo := memory.NewAgentRepository()
		signingKeyRepo := memory.NewSigningKeyStore()
		enc := &testEncryptor{}
		pkceRepo := memory.NewPKCESessionStore()
		refreshDeps, refreshStore := newNativeTestRefresh(agentRepo, credRepo, codeRepo, pkceRepo, enc)
		logger := testSlogger()

		svc := NewSigningKeyService(signingKeyRepo, signingKeyRepo, enc, newNoopBranchKeyManager(), logger)
		p, err := NewProvider(codeRepo, refreshDeps, pkceRepo, credRepo, &testClientResolver{agentRepo: agentRepo}, svc, "https://broker.example.com", time.Hour, expr, logger, refreshStore)
		require.NoError(t, err)

		_, err = svc.generateAndStore(context.Background(), "ES256", true, time.Now())
		require.NoError(t, err)

		return p, agentRepo
	}

	const expr = `{"agent_name": agent.display_name, "cid": agent.id, "https://identity.zalando.com/global-uuid": principal.id, "aud": ["https://agentic.identity.zalando.com"]}`

	provider, agentRepo := providerWithCEL(t, expr)
	agent, _, plaintext := setupTestCredentials(t, provider, agentRepo)
	agent.RedirectURIs = []string{"http://localhost:8080/callback"}
	agent.DisplayName = "Visual Studio Code"
	_ = agentRepo.Update(context.Background(), agent)

	verifier := "pkce-verifier-for-aud-list-regression-test-abcde"
	challenge := generateS256Challenge(verifier)

	code, err := provider.HandleAuthorize(
		context.Background(),
		agent.ID.String(),
		"http://localhost:8080/callback",
		"code",
		"read",
		"state",
		challenge,
		"S256",
		id.NewPrincipal("user@example.com"),
	)
	require.NoError(t, err)

	_, err = provider.HandleAuthorizationCodeExchange(
		context.Background(),
		agent.ID.String(),
		plaintext,
		code,
		"http://localhost:8080/callback",
		verifier,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServerError)
	assert.EqualError(t, err, "server_error: The authorization server encountered an unexpected condition that prevented it from fulfilling the request.")
}

// generateS256Challenge generates a PKCE S256 challenge from a verifier.
func generateS256Challenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
