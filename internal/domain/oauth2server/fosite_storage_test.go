package oauth2server

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	dstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// testClientResolver wraps an AgentRepository into a ClientResolver for tests.
type testClientResolver struct {
	agentRepo *memory.AgentRepository
}

func (r *testClientResolver) ResolveClient(ctx context.Context, clientID id.ClientID) (*ports.ClientResolution, error) {
	agentID, err := id.ParseAgentID(string(clientID))
	if err != nil {
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "not a valid UUID"}
	}
	agent, err := r.agentRepo.Get(ctx, agentID)
	if err != nil {
		return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "agent not found"}
	}
	return &ports.ClientResolution{Agent: agent}, nil
}

// mockClientResolver is a configurable test double for ports.ClientResolver.
type mockClientResolver struct {
	resolveFunc func(context.Context, id.ClientID) (*ports.ClientResolution, error)
}

func (r *mockClientResolver) ResolveClient(ctx context.Context, clientID id.ClientID) (*ports.ClientResolution, error) {
	return r.resolveFunc(ctx, clientID)
}

type mockCodeRepo struct {
	findByCodeHashFunc func(context.Context, string) (*dstorage.AuthorizationCode, error)
	markUsedFunc       func(context.Context, id.AuthorizationCodeID) error
}

func (m *mockCodeRepo) Create(_ context.Context, _ *dstorage.AuthorizationCode) error { return nil }
func (m *mockCodeRepo) FindByCodeHash(ctx context.Context, hash string) (*dstorage.AuthorizationCode, error) {
	return m.findByCodeHashFunc(ctx, hash)
}
func (m *mockCodeRepo) MarkUsed(ctx context.Context, codeID id.AuthorizationCodeID) error {
	if m.markUsedFunc != nil {
		return m.markUsedFunc(ctx, codeID)
	}
	return nil
}
func (m *mockCodeRepo) DeleteExpired(_ context.Context) (int, error) { return 0, nil }

type mockPKCERepo struct {
	deleteFunc func(context.Context, string) error
}

func (m *mockPKCERepo) Create(_ context.Context, _ *dstorage.PKCESession) error { return nil }
func (m *mockPKCERepo) FindBySignature(_ context.Context, _ string) (*dstorage.PKCESession, error) {
	return nil, dstorage.NewStorageError("FindBySignature", dstorage.ErrorKindNotFound, nil, "not found")
}
func (m *mockPKCERepo) Delete(ctx context.Context, sig string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, sig)
	}
	return nil
}
func (m *mockPKCERepo) DeleteExpired(_ context.Context) (int, error) { return 0, nil }

func newTestFositeStorage() (*FositeStorage, *memory.AuthorizationCodeStore, *memory.AgentRepository, *memory.ClientCredentialStore) {
	codeRepo := memory.NewAuthorizationCodeStore()
	pkceRepo := memory.NewPKCESessionStore()
	agentRepo := memory.NewAgentRepository()
	credRepo := memory.NewClientCredentialStore()
	resolver := &testClientResolver{agentRepo: agentRepo}
	return NewFositeStorage(codeRepo, memory.NewRefreshTokenSessionStore(), pkceRepo, credRepo, resolver, testSlogger()), codeRepo, agentRepo, credRepo
}

func TestFositeStorage_AuthorizeCodeSessions(t *testing.T) {
	t.Run("create and retrieve code session", func(t *testing.T) {
		store, _, agentRepo, credRepo := newTestFositeStorage()
		ctx := context.Background()

		// Setup: create agent and credential
		agent := testAgent()
		err := agentRepo.Create(ctx, agent)
		require.NoError(t, err)

		cred := &dstorage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "hash",
		}
		err = credRepo.Create(ctx, cred)
		require.NoError(t, err)

		// Create a code session
		client := &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: cred}
		session := &fosite.DefaultSession{
			Subject: "user@example.com",
			ExpiresAt: map[fosite.TokenType]time.Time{
				fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
			},
		}
		req := &fosite.Request{
			ID:             "req-1",
			Client:         client,
			Session:        session,
			RequestedScope: fosite.Arguments{"read", "write"},
			GrantedScope:   fosite.Arguments{"read", "write"},
			Form: map[string][]string{
				"redirect_uri":   {"http://localhost:8080/callback"},
				"code_challenge": {"challenge123"},
			},
			RequestedAt: time.Now(),
		}

		code := "test-code-value"
		err = store.CreateAuthorizeCodeSession(ctx, code, req)
		require.NoError(t, err)

		// Retrieve
		retrieved, err := store.GetAuthorizeCodeSession(ctx, code, session)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "user@example.com", retrieved.GetSession().GetSubject())
	})

	t.Run("invalidate code session marks as used", func(t *testing.T) {
		store, _, agentRepo, credRepo := newTestFositeStorage()
		ctx := context.Background()

		// Setup
		agent := testAgent()
		err := agentRepo.Create(ctx, agent)
		require.NoError(t, err)

		cred := &dstorage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "hash",
		}
		err = credRepo.Create(ctx, cred)
		require.NoError(t, err)

		client := &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: cred}
		session := &fosite.DefaultSession{
			Subject: "user@example.com",
			ExpiresAt: map[fosite.TokenType]time.Time{
				fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
			},
		}
		req := &fosite.Request{
			ID:      "req-2",
			Client:  client,
			Session: session,
			Form: map[string][]string{
				"redirect_uri": {"http://localhost:8080/callback"},
			},
			RequestedAt: time.Now(),
		}

		code := "test-code-value-2"
		err = store.CreateAuthorizeCodeSession(ctx, code, req)
		require.NoError(t, err)

		// Invalidate
		err = store.InvalidateAuthorizeCodeSession(ctx, code)
		require.NoError(t, err)

		// Should return error for invalidated code
		_, err = store.GetAuthorizeCodeSession(ctx, code, session)
		assert.Error(t, err)
	})

	t.Run("non-existent code returns not found", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		_, err := store.GetAuthorizeCodeSession(context.Background(), "non-existent", &fosite.DefaultSession{})
		assert.Error(t, err)
	})

	t.Run("empty scope round-trips as empty not [\"\"]", func(t *testing.T) {
		store, _, agentRepo, credRepo := newTestFositeStorage()
		ctx := context.Background()

		agent := testAgent()
		require.NoError(t, agentRepo.Create(ctx, agent))

		cred := &dstorage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "hash",
		}
		require.NoError(t, credRepo.Create(ctx, cred))

		client := &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: cred}
		session := &fosite.DefaultSession{
			Subject: "user@example.com",
			ExpiresAt: map[fosite.TokenType]time.Time{
				fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
			},
		}
		req := &fosite.Request{
			ID:             "req-empty-scope",
			Client:         client,
			Session:        session,
			RequestedScope: fosite.Arguments{},
			GrantedScope:   fosite.Arguments{},
			Form: map[string][]string{
				"redirect_uri":   {"http://localhost:8080/callback"},
				"code_challenge": {"challenge"},
			},
			RequestedAt: time.Now(),
		}

		require.NoError(t, store.CreateAuthorizeCodeSession(ctx, "empty-scope-code", req))

		retrieved, err := store.GetAuthorizeCodeSession(ctx, "empty-scope-code", session)
		require.NoError(t, err)
		assert.Empty(t, retrieved.GetRequestedScopes())
		assert.Empty(t, retrieved.GetGrantedScopes())
	})
}

func TestFositeStorage_AuthorizeCode_ProfileRoundTrip(t *testing.T) {
	store, _, agentRepo, credRepo := newTestFositeStorage()
	agent := testAgent()
	require.NoError(t, agentRepo.Create(context.Background(), agent))
	cred := &dstorage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "hash"}
	require.NoError(t, credRepo.Create(context.Background(), cred))

	email := "u@example.com"
	ctx := principal.WithProfile(context.Background(), principal.NewProfile("u@example.com").WithEmail(&email).WithDisplayName("Jane Doe"))
	req := &fosite.Request{
		ID:      "profile-auth-code-request",
		Client:  &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: cred},
		Session: &fosite.DefaultSession{Subject: "u@example.com", ExpiresAt: map[fosite.TokenType]time.Time{fosite.AuthorizeCode: time.Now().Add(time.Minute)}},
		Form: url.Values{
			"client_id":      {agent.ID.String()},
			"redirect_uri":   {"http://localhost:8080/callback"},
			"code_challenge": {"challenge"},
		},
	}
	require.NoError(t, store.CreateAuthorizeCodeSession(ctx, "profile-code", req))

	got, err := store.GetAuthorizeCodeSession(context.Background(), "profile-code", nil)
	require.NoError(t, err)
	extra := got.GetSession().(fosite.ExtraClaimsSession).GetExtraClaims()
	assert.Equal(t, "u@example.com", extra[claimEmail])
	assert.Equal(t, "Jane Doe", extra[claimDisplayName])
}

func TestFositeStorage_AccessTokenSessions(t *testing.T) {
	t.Run("create access token session is no-op for stateless JWT", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		err := store.CreateAccessTokenSession(context.Background(), "sig", nil)
		assert.NoError(t, err)
	})

	t.Run("get access token session returns not found for stateless JWT", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		_, err := store.GetAccessTokenSession(context.Background(), "sig", nil)
		assert.Error(t, err)
	})

	t.Run("delete access token session is no-op for stateless JWT", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		err := store.DeleteAccessTokenSession(context.Background(), "sig")
		assert.NoError(t, err)
	})
}

func TestFositeStorage_RefreshTokenSessions(t *testing.T) {
	setupRequest := func(t *testing.T, store *FositeStorage, agentRepo *memory.AgentRepository, credRepo *memory.ClientCredentialStore, requestID string, scopes []string) *fosite.Request {
		t.Helper()
		agent := testAgent()
		require.NoError(t, agentRepo.Create(context.Background(), agent))

		cred := &dstorage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "hash",
		}
		require.NoError(t, credRepo.Create(context.Background(), cred))

		return &fosite.Request{
			ID:             requestID,
			Client:         &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: cred},
			Session:        &fosite.DefaultSession{Subject: "user@example.com", ExpiresAt: map[fosite.TokenType]time.Time{fosite.RefreshToken: time.Now().Add(time.Hour)}},
			RequestedScope: fosite.Arguments(scopes),
			GrantedScope:   fosite.Arguments(scopes),
		}
	}

	t.Run("create and retrieve refresh token session", func(t *testing.T) {
		store, _, agentRepo, credRepo := newTestFositeStorage()
		req := setupRequest(t, store, agentRepo, credRepo, "req-1", []string{"offline_access", "read"})

		require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "sig-1", "access-1", req))

		got, err := store.GetRefreshTokenSession(context.Background(), "sig-1", nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "req-1", got.GetID())
		assert.Equal(t, "user@example.com", got.GetSession().GetSubject())
		assert.ElementsMatch(t, []string{"offline_access", "read"}, []string(got.GetGrantedScopes()))
		assert.Equal(t, req.GetClient().GetID(), got.GetClient().GetID())
	})

	t.Run("rotated refresh token becomes inactive", func(t *testing.T) {
		store, _, agentRepo, credRepo := newTestFositeStorage()
		req := setupRequest(t, store, agentRepo, credRepo, "req-2", []string{"offline_access"})

		require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "sig-2", "access-2", req))
		require.NoError(t, store.RotateRefreshToken(context.Background(), "req-2", "sig-2"))

		got, err := store.GetRefreshTokenSession(context.Background(), "sig-2", nil)
		require.ErrorIs(t, err, fosite.ErrInactiveToken)
		require.NotNil(t, got)
		assert.Equal(t, "req-2", got.GetID())
	})

	t.Run("unknown signature returns ErrNotFound", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		_, err := store.GetRefreshTokenSession(context.Background(), "unknown-sig", nil)
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("revoke by request ID inactivates all sessions in the chain", func(t *testing.T) {
		store, _, agentRepo, credRepo := newTestFositeStorage()
		req1 := setupRequest(t, store, agentRepo, credRepo, "req-chain", []string{"offline_access"})
		req2 := setupRequest(t, store, agentRepo, credRepo, "req-chain", []string{"offline_access", "read"})

		require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "sig-3", "access-3", req1))
		require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "sig-4", "access-4", req2))
		require.NoError(t, store.RevokeRefreshToken(context.Background(), "req-chain"))

		for _, signature := range []string{"sig-3", "sig-4"} {
			got, err := store.GetRefreshTokenSession(context.Background(), signature, nil)
			require.ErrorIs(t, err, fosite.ErrInactiveToken)
			require.NotNil(t, got)
		}
	})
}

func TestFositeStorage_RefreshToken_ProfileRoundTrip(t *testing.T) {
	store, _, agentRepo, credRepo := newTestFositeStorage()
	agent := testAgent()
	require.NoError(t, agentRepo.Create(context.Background(), agent))
	cred := &dstorage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "hash"}
	require.NoError(t, credRepo.Create(context.Background(), cred))

	session := &fosite.DefaultSession{Subject: "u@example.com", ExpiresAt: map[fosite.TokenType]time.Time{fosite.RefreshToken: time.Now().Add(time.Hour)}}
	setSessionProfile(session, ptr.To("u@example.com"), "Jane Doe")
	req := &fosite.Request{
		ID:           "profile-refresh-request",
		Client:       &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: cred},
		Session:      session,
		GrantedScope: fosite.Arguments{"offline_access", "read"},
	}
	require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "profile-refresh", "", req))

	got, err := store.GetRefreshTokenSession(context.Background(), "profile-refresh", nil)
	require.NoError(t, err)
	extra := got.GetSession().(fosite.ExtraClaimsSession).GetExtraClaims()
	assert.Equal(t, "u@example.com", extra[claimEmail])
	assert.Equal(t, "Jane Doe", extra[claimDisplayName])
}

func TestFositeStorage_PKCESessions(t *testing.T) {
	t.Run("create stores challenge and method", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		ctx := context.Background()

		req := &fosite.Request{
			Form: url.Values{
				"code_challenge":        {"challenge-abc"},
				"code_challenge_method": {"S256"},
			},
			Session: &fosite.DefaultSession{
				ExpiresAt: map[fosite.TokenType]time.Time{
					fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
				},
			},
		}
		err := store.CreatePKCERequestSession(ctx, "sig-1", req)
		require.NoError(t, err)
	})

	t.Run("get returns stored challenge and method", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		ctx := context.Background()

		req := &fosite.Request{
			Form: url.Values{
				"code_challenge":        {"challenge-xyz"},
				"code_challenge_method": {"S256"},
			},
			Session: &fosite.DefaultSession{
				ExpiresAt: map[fosite.TokenType]time.Time{
					fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
				},
			},
		}
		require.NoError(t, store.CreatePKCERequestSession(ctx, "sig-2", req))

		got, err := store.GetPKCERequestSession(ctx, "sig-2", nil)
		require.NoError(t, err)
		assert.Equal(t, "challenge-xyz", got.GetRequestForm().Get("code_challenge"))
		assert.Equal(t, "S256", got.GetRequestForm().Get("code_challenge_method"))
	})

	t.Run("get unknown signature returns ErrNotFound", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		_, err := store.GetPKCERequestSession(context.Background(), "unknown-sig", nil)
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("delete removes session, subsequent get returns ErrNotFound", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		ctx := context.Background()

		req := &fosite.Request{
			Form: url.Values{
				"code_challenge":        {"challenge-del"},
				"code_challenge_method": {"S256"},
			},
			Session: &fosite.DefaultSession{
				ExpiresAt: map[fosite.TokenType]time.Time{
					fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
				},
			},
		}
		require.NoError(t, store.CreatePKCERequestSession(ctx, "sig-3", req))
		require.NoError(t, store.DeletePKCERequestSession(ctx, "sig-3"))

		_, err := store.GetPKCERequestSession(ctx, "sig-3", nil)
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})
}

func TestExtractAgentID(t *testing.T) {
	t.Run("returns agent ID for confidentialClient", func(t *testing.T) {
		agent := testAgent()
		bc := &confidentialClient{clientID: agent.ID.String(), agent: agent}
		gotID, err := extractAgentID(bc)
		require.NoError(t, err)
		assert.Equal(t, agent.ID, gotID)
	})

	t.Run("returns agent ID for publicClient", func(t *testing.T) {
		agent := testAgent()
		pc := &publicClient{clientID: "https://example.com/client", agent: agent}
		gotID, err := extractAgentID(pc)
		require.NoError(t, err)
		assert.Equal(t, agent.ID, gotID)
	})

	t.Run("returns error for unexpected client type", func(t *testing.T) {
		_, err := extractAgentID(&fosite.DefaultClient{ID: "unexpected"})
		require.Error(t, err)
	})

	t.Run("returns error when agent is nil", func(t *testing.T) {
		_, err := extractAgentID(&confidentialClient{agent: nil})
		require.Error(t, err)
	})

	t.Run("returns error when agent ID is zero", func(t *testing.T) {
		agent := testAgent()
		agent.ID = id.AgentID{}
		_, err := extractAgentID(&confidentialClient{agent: agent})
		require.Error(t, err)
	})
}

func TestCreateAuthorizeCodeSession_WrongClientType(t *testing.T) {
	store := NewFositeStorage(&mockCodeRepo{}, memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), &mockCredentialRepo{}, &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

	req := &fosite.Request{
		Client:  &fosite.DefaultClient{ID: "not-a-broker-client"},
		Session: &fosite.DefaultSession{Subject: "user@example.com"},
		Form:    map[string][]string{},
	}

	err := store.CreateAuthorizeCodeSession(context.Background(), "somecode", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CreateAuthorizeCodeSession")
}

func TestFositeStorage_ClientAssertionJWT(t *testing.T) {
	t.Run("ClientAssertionJWTValid rejects all JTIs (fail-closed)", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		err := store.ClientAssertionJWTValid(context.Background(), "any-jti")
		assert.ErrorIs(t, err, fosite.ErrJTIKnown)
	})

	t.Run("SetClientAssertionJWT is a no-op", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()
		err := store.SetClientAssertionJWT(context.Background(), "any-jti", time.Now().Add(time.Minute))
		assert.NoError(t, err)
	})
}

func TestFositeStorage_InfrastructureErrors(t *testing.T) {
	connectionErr := dstorage.NewStorageError("FindByCodeHash", dstorage.ErrorKindConnection, nil, "connection refused")

	t.Run("GetAuthorizeCodeSession connection error is not ErrNotFound", func(t *testing.T) {
		codeRepo := &mockCodeRepo{
			findByCodeHashFunc: func(_ context.Context, _ string) (*dstorage.AuthorizationCode, error) {
				return nil, connectionErr
			},
		}
		store := NewFositeStorage(codeRepo, memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		_, err := store.GetAuthorizeCodeSession(context.Background(), "anycode", nil)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetAuthorizeCodeSession not-found returns ErrNotFound", func(t *testing.T) {
		notFoundErr := dstorage.NewStorageError("FindByCodeHash", dstorage.ErrorKindNotFound, nil, "not found")
		codeRepo := &mockCodeRepo{
			findByCodeHashFunc: func(_ context.Context, _ string) (*dstorage.AuthorizationCode, error) {
				return nil, notFoundErr
			},
		}
		store := NewFositeStorage(codeRepo, memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		_, err := store.GetAuthorizeCodeSession(context.Background(), "anycode", nil)
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient non-UUID input returns ErrNotFound", func(t *testing.T) {
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		_, err := store.GetClient(context.Background(), "broker_any")
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient resolver infrastructure error is not ErrNotFound", func(t *testing.T) {
		agentID := id.NewAgentID()
		resolver := &mockClientResolver{
			resolveFunc: func(_ context.Context, _ id.ClientID) (*ports.ClientResolution, error) {
				return nil, dstorage.NewStorageError("Get", dstorage.ErrorKindTimeout, nil, "query timeout")
			},
		}
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), resolver, testSlogger())

		_, err := store.GetClient(context.Background(), agentID.String())
		assert.Error(t, err)
		assert.NotErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient valid-but-unknown agent UUID returns ErrNotFound", func(t *testing.T) {
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		_, err := store.GetClient(context.Background(), id.NewAgentID().String())
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient agent-exists-but-no-credential returns ErrNotFound", func(t *testing.T) {
		agentRepo := memory.NewAgentRepository()
		agentID := id.NewAgentID()
		agent := &dstorage.Agent{ID: agentID, ClientID: ptr.To(id.ClientID("upstream-client")), DisplayName: "Test Agent", Description: "Test agent for infrastructure error tests", PermissionSets: testAgentPermissionSets()}
		require.NoError(t, agentRepo.Create(context.Background(), agent))
		// No credential created — credential repo is empty
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: agentRepo}, testSlogger())

		_, err := store.GetClient(context.Background(), agentID.String())
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("InvalidateAuthorizeCodeSession MarkUsed infrastructure error is logged and returned", func(t *testing.T) {
		authCode := &dstorage.AuthorizationCode{
			ID:      id.NewAuthorizationCodeID(),
			AgentID: id.NewAgentID(),
		}
		timeoutErr := dstorage.NewStorageError("MarkUsed", dstorage.ErrorKindTimeout, nil, "query timeout")
		codeRepo := &mockCodeRepo{
			findByCodeHashFunc: func(_ context.Context, _ string) (*dstorage.AuthorizationCode, error) {
				return authCode, nil
			},
			markUsedFunc: func(_ context.Context, _ id.AuthorizationCodeID) error {
				return timeoutErr
			},
		}
		store := NewFositeStorage(codeRepo, memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		err := store.InvalidateAuthorizeCodeSession(context.Background(), "anycode")
		assert.Error(t, err)
		assert.NotErrorIs(t, err, fosite.ErrInvalidatedAuthorizeCode)
		assert.NotErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("InvalidateAuthorizeCodeSession MarkUsed not-found returns ErrInvalidatedAuthorizeCode", func(t *testing.T) {
		authCode := &dstorage.AuthorizationCode{
			ID:      id.NewAuthorizationCodeID(),
			AgentID: id.NewAgentID(),
		}
		notFoundErr := dstorage.NewStorageError("MarkUsed", dstorage.ErrorKindNotFound, nil, "not found")
		codeRepo := &mockCodeRepo{
			findByCodeHashFunc: func(_ context.Context, _ string) (*dstorage.AuthorizationCode, error) {
				return authCode, nil
			},
			markUsedFunc: func(_ context.Context, _ id.AuthorizationCodeID) error {
				return notFoundErr
			},
		}
		store := NewFositeStorage(codeRepo, memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		err := store.InvalidateAuthorizeCodeSession(context.Background(), "anycode")
		assert.ErrorIs(t, err, fosite.ErrInvalidatedAuthorizeCode)
	})

	t.Run("DeletePKCERequestSession infrastructure error is logged and returned", func(t *testing.T) {
		connErr := dstorage.NewStorageError("Delete", dstorage.ErrorKindConnection, nil, "connection refused")
		pkceRepo := &mockPKCERepo{
			deleteFunc: func(_ context.Context, _ string) error { return connErr },
		}
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), pkceRepo, memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		err := store.DeletePKCERequestSession(context.Background(), "anysig")
		assert.Error(t, err)
		assert.NotErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("DeletePKCERequestSession not-found returns ErrNotFound", func(t *testing.T) {
		notFoundErr := dstorage.NewStorageError("Delete", dstorage.ErrorKindNotFound, nil, "not found")
		pkceRepo := &mockPKCERepo{
			deleteFunc: func(_ context.Context, _ string) error { return notFoundErr },
		}
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), pkceRepo, memory.NewClientCredentialStore(), &testClientResolver{agentRepo: memory.NewAgentRepository()}, testSlogger())

		err := store.DeletePKCERequestSession(context.Background(), "anysig")
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient round-trips client.GetID() to agent UUID", func(t *testing.T) {
		_, _, agentRepo, credRepo := newTestFositeStorage()
		agentID := id.NewAgentID()
		agent := &dstorage.Agent{ID: agentID, ClientID: ptr.To(id.ClientID("upstream-client")), DisplayName: "Test Agent", Description: "Test agent for round-trip tests", PermissionSets: testAgentPermissionSets()}
		require.NoError(t, agentRepo.Create(context.Background(), agent))
		cred := &dstorage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agentID,
			SecretHash: "hash",
		}
		require.NoError(t, credRepo.Create(context.Background(), cred))
		store := NewFositeStorage(memory.NewAuthorizationCodeStore(), memory.NewRefreshTokenSessionStore(), memory.NewPKCESessionStore(), credRepo, &testClientResolver{agentRepo: agentRepo}, testSlogger())

		client, err := store.GetClient(context.Background(), agentID.String())
		require.NoError(t, err)
		assert.Equal(t, agentID.String(), client.GetID())
	})
}
