package oauth2server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
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
	refresh, coordinator := newNativeTestRefresh(agentRepo, credRepo, codeRepo, pkceRepo, &testEncryptor{})
	resolver := &testClientResolver{agentRepo: agentRepo}
	return NewFositeStorage(codeRepo, refresh, pkceRepo, credRepo, resolver, testSlogger(), coordinator), codeRepo, agentRepo, credRepo
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

func TestFositeStorage_AuthorizeCodeExpiry(t *testing.T) {
	for _, tt := range []struct {
		name      string
		expiresAt time.Time
		used      bool
		wantErr   error
	}{
		{name: "active", expiresAt: time.Now().Add(time.Minute)},
		{name: "expired", expiresAt: time.Now().Add(-time.Minute), wantErr: fosite.ErrInvalidGrant},
		{name: "missing expiry", wantErr: fosite.ErrInvalidGrant},
		{name: "expired used code is not replay", expiresAt: time.Now().Add(-time.Minute), used: true, wantErr: fosite.ErrInvalidGrant},
		{name: "active used code is replay", expiresAt: time.Now().Add(time.Minute), used: true, wantErr: fosite.ErrInvalidatedAuthorizeCode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _, agentRepo, credRepo := newTestFositeStorage()
			agent := testAgent()
			require.NoError(t, agentRepo.Create(context.Background(), agent))
			require.NoError(t, credRepo.Create(context.Background(), &dstorage.ClientCredential{
				ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "hash",
			}))
			code := &dstorage.AuthorizationCode{
				ID: id.NewAuthorizationCodeID(), AgentID: agent.ID,
				ClientID: id.NewClientID(agent.ID.String()), Principal: id.NewPrincipal("user@example.com"),
				ExpiresAt: tt.expiresAt,
			}
			if tt.used {
				usedAt := time.Now().Add(-time.Minute)
				code.UsedAt = &usedAt
			}
			store.codeRepo = &mockCodeRepo{
				findByCodeHashFunc: func(context.Context, string) (*dstorage.AuthorizationCode, error) {
					return code, nil
				},
			}
			session := &fosite.DefaultSession{}
			session.SetExpiresAt(fosite.AuthorizeCode, time.Now().Add(-time.Hour))
			req, err := store.GetAuthorizeCodeSession(context.Background(), "code", session)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantErr == fosite.ErrInvalidatedAuthorizeCode {
					require.NotNil(t, req)
				} else {
					assert.NotErrorIs(t, err, fosite.ErrInvalidatedAuthorizeCode)
				}
			} else {
				require.NoError(t, err)
				require.NoError(t, (&RandomCodeStrategy{}).ValidateAuthorizeCode(context.Background(), &fosite.Request{Session: session}, "code"))
			}
		})
	}
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

// issueNativeTestRefresh exchanges a real stored code in one agent-scoped
// operation, so the native root and its first token have the same origin.
func issueNativeTestRefresh(t *testing.T, store *FositeStorage, agents *memory.AgentRepository, credentials *memory.ClientCredentialStore, principalID string) (*dstorage.Agent, *confidentialClient, *dstorage.RefreshSession, string, string) {
	t.Helper()
	ctx := context.Background()
	agent := &dstorage.Agent{ID: id.NewAgentID(), DisplayName: "Local agent", Description: "Local refresh test agent", PermissionSets: testAgentPermissionSets(), AllowedScopes: []string{"offline_access", "read", "write"}, RedirectURIs: []string{"http://localhost:8080/callback"}}
	require.NoError(t, agents.Create(ctx, agent))
	credential := &dstorage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "hash"}
	require.NoError(t, credentials.Create(ctx, credential))
	client := &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: credential}
	grant := &dstorage.UserGrant{ID: id.NewGrantID(), Principal: id.NewPrincipal(principalID), AgentID: agent.ID}
	require.NoError(t, store.refresh.Verifier.(testGrantVerifier).grants.Create(ctx, grant))
	codeSignature := sha256Hex("code:" + agent.ID.String())
	firstSignature := sha256Hex("refresh:" + agent.ID.String())
	codeRequest := &fosite.Request{
		Client: client, Session: &fosite.DefaultSession{Subject: principalID, ExpiresAt: map[fosite.TokenType]time.Time{fosite.AuthorizeCode: time.Now().Add(time.Minute)}},
		RequestedScope: fosite.Arguments{"read", "offline_access", "read"}, GrantedScope: fosite.Arguments{"read", "offline_access"},
		Form: url.Values{"client_id": {agent.ID.String()}, "redirect_uri": {"http://localhost:8080/callback"}, "code_challenge": {"challenge"}},
	}
	profile := principal.WithProfile(ctx, principal.NewProfile(principalID).WithEmail(ptr.To(principalID)).WithDisplayName("Jane Doe"))
	require.NoError(t, store.CreateAuthorizeCodeSession(profile, codeSignature, codeRequest))
	var sessionID id.RefreshSessionID
	err := store.refresh.Coordinator.Run(ctx, agent.ID, func(scoped context.Context, at time.Time) error {
		owned := withRefreshOperation(scoped, at, client, nil)
		code, err := store.GetAuthorizeCodeSession(owned, codeSignature, &fosite.DefaultSession{})
		if err != nil {
			return err
		}
		if err := store.InvalidateAuthorizeCodeSession(owned, codeSignature); err != nil {
			return err
		}
		sessionID, err = id.ParseRefreshSessionID(code.GetID())
		if err != nil {
			return err
		}
		return store.CreateRefreshTokenSession(owned, firstSignature, "", code)
	})
	require.NoError(t, err)
	root, err := store.refresh.Sessions.FindByID(ctx, sessionID)
	require.NoError(t, err)
	return agent, client, root, firstSignature, codeSignature
}

func TestFositeStorage_NativeRefreshLineage(t *testing.T) {
	store, _, agents, credentials := newTestFositeStorage()
	agent, client, root, firstSignature, codeSignature := issueNativeTestRefresh(t, store, agents, credentials, "user@example.com")
	initial, err := store.refresh.Tokens.FindBySignature(context.Background(), firstSignature)
	require.NoError(t, err)
	code, err := store.codeRepo.FindByCodeHash(context.Background(), codeSignature)
	require.NoError(t, err)
	grant, err := store.refresh.Verifier.(testGrantVerifier).grants.FindByPrincipalAndAgent(context.Background(), id.NewPrincipal("user@example.com"), agent.ID)
	require.NoError(t, err)
	assert.Equal(t, code.ID.String(), root.ID.String(), "root is the immutable authorization-code UUID")
	assert.Equal(t, grant.ID, root.OriginalGrantID)
	assert.Equal(t, "offline_access read", root.Scope)
	assert.Equal(t, root.StartedAt, initial.IssuedAt)
	assert.Equal(t, root.StartedAt, root.LastFreshAt)
	assert.Equal(t, root.InactivityExpiresAt, initial.ExpiresAt)
	assert.Equal(t, initial.ExpiresAt, root.RetainUntil)
	assert.Equal(t, "refresh_"+root.ID.String()+"_branch_key", root.BranchKeyID)
	assert.Nil(t, root.AbsoluteExpiresAt)
	assert.Nil(t, root.PreviousSignature)
	assert.Empty(t, root.RetryCiphertext)

	secondSignature := sha256Hex("second:" + agent.ID.String())
	var consumedAt, accessExpiry time.Time
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{ClientIP: "203.0.113.20:443", UserAgent: "client/1.0"})
	err = store.refresh.Coordinator.Run(ctx, agent.ID, func(scoped context.Context, at time.Time) error {
		consumedAt = at
		accessExpiry = at.Add(15 * time.Minute)
		owned := withRefreshOperation(scoped, at, client, root)
		recordRefreshAccessExpiry(owned, accessExpiry)
		req, err := store.GetRefreshTokenSession(owned, firstSignature, nil)
		if err != nil {
			return err
		}
		extra := req.GetSession().(fosite.ExtraClaimsSession).GetExtraClaims()
		assert.Equal(t, "user@example.com", extra[claimEmail])
		assert.Equal(t, "Jane Doe", extra[claimDisplayName])
		if err := store.RotateRefreshToken(owned, root.ID.String(), firstSignature); err != nil {
			return err
		}
		op, _ := refreshOperationFromContext(owned)
		consumedAt = op.at
		assert.False(t, consumedAt.Before(at), "fresh consumption cannot precede its authorization decision")
		narrowed := req.(*fosite.Request)
		narrowed.GrantedScope = fosite.Arguments{"read"}
		narrowed.Form = url.Values{"scope": {"  read read  "}}
		op.requestedScope = canonicalRequestedScope(narrowed.Form.Get("scope"))
		return store.CreateRefreshTokenSession(owned, secondSignature, "", narrowed.Sanitize([]string{}))
	})
	require.NoError(t, err)
	updated, err := store.refresh.Sessions.FindByID(context.Background(), root.ID)
	require.NoError(t, err)
	previous, err := store.refresh.Tokens.FindBySignature(context.Background(), firstSignature)
	require.NoError(t, err)
	successor, err := store.refresh.Tokens.FindBySignature(context.Background(), secondSignature)
	require.NoError(t, err)
	require.NotNil(t, updated.PreviousSignature)
	require.NotNil(t, previous.UsedAt)
	require.NotNil(t, updated.PreviousConsumedAt)
	require.NotNil(t, updated.OriginalRequestedScope)
	require.NotNil(t, updated.OriginalRequestContextFingerprint)
	require.NotNil(t, updated.ReuseUntil)
	require.NotNil(t, updated.RetryExpiresAt)
	require.NotNil(t, updated.RetryAccessExpiresAt)
	assert.Equal(t, "offline_access read", updated.Scope, "narrowing the access result cannot lower the lineage ceiling")
	assert.Equal(t, secondSignature, updated.CurrentSignature)
	assert.Equal(t, firstSignature, *updated.PreviousSignature)
	assert.Equal(t, consumedAt, *previous.UsedAt)
	assert.Equal(t, consumedAt, *updated.PreviousConsumedAt)
	assert.Equal(t, consumedAt, updated.LastFreshAt)
	assert.Equal(t, consumedAt, successor.IssuedAt)
	assert.Equal(t, consumedAt.Add(time.Hour), successor.ExpiresAt)
	assert.Equal(t, successor.ExpiresAt, updated.RetainUntil)
	assert.Equal(t, "read", *updated.OriginalRequestedScope)
	assert.Equal(t, consumedAt, *updated.ReuseUntil, "zero reuse retains no cached response")
	assert.Equal(t, consumedAt, *updated.RetryExpiresAt)
	assert.Equal(t, accessExpiry, *updated.RetryAccessExpiresAt)
	assert.Empty(t, updated.RetryCiphertext)
	assert.Zero(t, updated.RetryCount)
	_, other, _, _, _ := issueNativeTestRefresh(t, store, agents, credentials, "other@example.com")
	err = store.refresh.Coordinator.Run(context.Background(), agent.ID, func(scoped context.Context, at time.Time) error {
		_, getErr := store.GetRefreshTokenSession(withRefreshOperation(scoped, at, other, updated), firstSignature, nil)
		if !errors.Is(getErr, fosite.ErrNotFound) {
			return fmt.Errorf("wrong client reached consumed-token replay path: %v", getErr)
		}
		return nil
	})
	require.NoError(t, err)
	err = store.refresh.Coordinator.Run(context.Background(), agent.ID, func(scoped context.Context, _ time.Time) error {
		_, getErr := store.GetRefreshTokenSession(withRefreshOperation(scoped, previous.ExpiresAt, client, updated), firstSignature, nil)
		if !errors.Is(getErr, fosite.ErrInactiveToken) {
			return fmt.Errorf("expired consumed signature lost replay evidence: %v", getErr)
		}
		return nil
	})
	require.NoError(t, err)
	stillActive, err := store.refresh.Sessions.FindByID(context.Background(), root.ID)
	require.NoError(t, err)
	assert.Nil(t, stillActive.TerminalReason)
	assert.Equal(t, secondSignature, stillActive.CurrentSignature)
}

func TestFositeStorage_CodeReplayRevokesOriginalRoot(t *testing.T) {
	store, _, agents, credentials := newTestFositeStorage()
	owner, ownerClient, original, signature, codeSignature := issueNativeTestRefresh(t, store, agents, credentials, "user@example.com")
	other, otherClient, unrelated, unrelatedSignature, _ := issueNativeTestRefresh(t, store, agents, credentials, "other@example.com")
	require.NotEqual(t, owner.ID, other.ID)
	require.NoError(t, credentials.Rotate(context.Background(), owner.ID, &dstorage.ClientCredential{ID: id.NewCredentialID(), AgentID: owner.ID, SecretHash: "replacement-hash"}))
	err := store.refresh.Coordinator.Run(context.Background(), owner.ID, func(scoped context.Context, at time.Time) error {
		_, getErr := store.GetRefreshTokenSession(withRefreshOperation(scoped, at, ownerClient, original), signature, nil)
		if !errors.Is(getErr, fosite.ErrNotFound) {
			return fmt.Errorf("old credential authorized retained refresh token: %v", getErr)
		}
		return nil
	})
	require.NoError(t, err)
	var replayAt time.Time
	err = store.refresh.Coordinator.Run(context.Background(), owner.ID, func(scoped context.Context, at time.Time) error {
		replayAt = at
		owned := withRefreshOperation(scoped, at, otherClient, nil)
		request, err := store.GetAuthorizeCodeSession(owned, codeSignature, &fosite.DefaultSession{})
		if !errors.Is(err, fosite.ErrInvalidatedAuthorizeCode) || request == nil || request.GetID() != original.ID.String() {
			return fmt.Errorf("used code did not hydrate original request: %v", err)
		}
		borrowed, err := store.BeginTX(owned)
		if err != nil {
			return err
		}
		if err := store.RevokeRefreshToken(borrowed, request.GetID()); err != nil {
			return err
		}
		if err := store.Rollback(borrowed); err != nil {
			return err
		}
		return store.RevokeRefreshToken(borrowed, request.GetID())
	})
	require.NoError(t, err)
	revoked, err := store.refresh.Sessions.FindByID(context.Background(), original.ID)
	require.NoError(t, err)
	require.NotNil(t, revoked.TerminalReason)
	require.NotNil(t, revoked.RevokedAt)
	assert.Equal(t, dstorage.RefreshReasonCodeReplay, *revoked.TerminalReason)
	assert.Equal(t, replayAt, *revoked.RevokedAt)
	assert.Empty(t, revoked.RetryCiphertext)
	untouched, err := store.refresh.Sessions.FindByID(context.Background(), unrelated.ID)
	require.NoError(t, err)
	assert.Nil(t, untouched.TerminalReason)
	assert.Equal(t, unrelatedSignature, untouched.CurrentSignature)
	currentOwner, err := store.GetClient(context.Background(), owner.ID.String())
	require.NoError(t, err)
	err = store.refresh.Coordinator.Run(context.Background(), owner.ID, func(scoped context.Context, at time.Time) error {
		_, err := store.GetRefreshTokenSession(withRefreshOperation(scoped, at, currentOwner, revoked), signature, nil)
		if !errors.Is(err, fosite.ErrInactiveToken) {
			return fmt.Errorf("revoked native token was still usable: %v", err)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestFositeStorage_RejectsUnownedNativeRefresh(t *testing.T) {
	store, _, agents, credentials := newTestFositeStorage()
	agent, client, root, signature, _ := issueNativeTestRefresh(t, store, agents, credentials, "user@example.com")
	_, err := store.GetRefreshTokenSession(context.Background(), signature, nil)
	require.Error(t, err)
	_, other, _, _, _ := issueNativeTestRefresh(t, store, agents, credentials, "other@example.com")
	err = store.refresh.Coordinator.Run(context.Background(), agent.ID, func(scoped context.Context, at time.Time) error {
		_, lookupErr := store.GetRefreshTokenSession(withRefreshOperation(scoped, at, other, root), signature, nil)
		if !errors.Is(lookupErr, fosite.ErrNotFound) {
			return fmt.Errorf("wrong client obtained refresh authority: %v", lookupErr)
		}
		if revokeErr := store.RevokeRefreshToken(withRefreshOperation(scoped, at, other, root), root.ID.String()); !errors.Is(revokeErr, fosite.ErrNotFound) {
			return fmt.Errorf("wrong client reached family revocation: %v", revokeErr)
		}
		return nil
	})
	require.NoError(t, err)
	err = store.refresh.Coordinator.Run(context.Background(), agent.ID, func(scoped context.Context, at time.Time) error {
		return store.DeleteRefreshTokenSession(withRefreshOperation(scoped, at, client, root), sha256Hex("never-issued"))
	})
	require.NoError(t, err)
	unchanged, err := store.refresh.Sessions.FindByID(context.Background(), root.ID)
	require.NoError(t, err)
	assert.Nil(t, unchanged.TerminalReason)
	assert.Equal(t, signature, unchanged.CurrentSignature)
}

func TestFositeStorage_BorrowedCommitDefersOwnerFailure(t *testing.T) {
	store, _, agents, credentials := newTestFositeStorage()
	owner, _, root, signature, codeSignature := issueNativeTestRefresh(t, store, agents, credentials, "user@example.com")
	_, replayer, _, _, _ := issueNativeTestRefresh(t, store, agents, credentials, "other@example.com")
	rejected := errors.New("owner rejected token response")
	err := store.refresh.Coordinator.Run(context.Background(), owner.ID, func(scoped context.Context, at time.Time) error {
		owned := withRefreshOperation(scoped, at, replayer, nil)
		request, getErr := store.GetAuthorizeCodeSession(owned, codeSignature, &fosite.DefaultSession{})
		if !errors.Is(getErr, fosite.ErrInvalidatedAuthorizeCode) {
			return getErr
		}
		if err := store.RevokeRefreshToken(owned, request.GetID()); err != nil {
			return err
		}
		if err := store.Commit(owned); err != nil {
			return err
		}
		return rejected
	})
	require.ErrorIs(t, err, rejected)
	current, err := store.refresh.Sessions.FindByID(context.Background(), root.ID)
	require.NoError(t, err)
	assert.Nil(t, current.TerminalReason, "only the coordinator may commit code replay revocation")
	token, err := store.refresh.Tokens.FindBySignature(context.Background(), signature)
	require.NoError(t, err)
	assert.Nil(t, token.UsedAt)
}

func TestInitializeRefreshSession_FiniteIndependentDeadlines(t *testing.T) {
	at := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	root := &dstorage.RefreshSession{ID: id.NewRefreshSessionID()}
	policy := dstorage.RefreshSessionPolicy{InactivityLifetime: time.Hour, AbsoluteLifetime: 5 * time.Minute}
	require.NoError(t, initializeRefreshSession(root, policy, at))
	assert.Equal(t, at, root.StartedAt)
	assert.Equal(t, at, root.LastFreshAt)
	assert.Equal(t, at.Add(time.Hour), root.InactivityExpiresAt)
	assert.Equal(t, at.Add(time.Hour), root.RetainUntil, "retention follows token expiry despite earlier absolute denial")
	assert.Equal(t, at.Add(5*time.Minute), *root.AbsoluteExpiresAt)
	assert.Equal(t, "refresh_"+root.ID.String()+"_branch_key", root.BranchKeyID)
	require.Error(t, initializeRefreshSession(&dstorage.RefreshSession{ID: id.NewRefreshSessionID()}, dstorage.RefreshSessionPolicy{}, at))
}

func TestFositeStorage_FailedRotationPreservesPredecessor(t *testing.T) {
	store, _, agents, credentials := newTestFositeStorage()
	agent, client, root, signature, _ := issueNativeTestRefresh(t, store, agents, credentials, "user@example.com")
	successor := sha256Hex("uncommitted:" + signature)
	err := store.refresh.Coordinator.Run(context.Background(), agent.ID, func(scoped context.Context, at time.Time) error {
		owned := withRefreshOperation(scoped, at, client, root)
		req, err := store.GetRefreshTokenSession(owned, signature, nil)
		if err != nil {
			return err
		}
		if err := store.RotateRefreshToken(owned, root.ID.String(), signature); err != nil {
			return err
		}
		return store.CreateRefreshTokenSession(owned, successor, "", req) // no signed access expiry
	})
	require.Error(t, err)
	previous, err := store.refresh.Tokens.FindBySignature(context.Background(), signature)
	require.NoError(t, err)
	assert.Nil(t, previous.UsedAt)
	_, err = store.refresh.Tokens.FindBySignature(context.Background(), successor)
	require.True(t, ports.IsNotFoundErr(err), "failed mint must leave no successor")
	unchanged, err := store.refresh.Sessions.FindByID(context.Background(), root.ID)
	require.NoError(t, err)
	assert.Equal(t, signature, unchanged.CurrentSignature)
	assert.Nil(t, unchanged.PreviousSignature)
}

func TestFositeStorage_RefreshExpiryUsesOwnerDecisionTime(t *testing.T) {
	store, _, agents, credentials := newTestFositeStorage()
	agent, client, root, signature, _ := issueNativeTestRefresh(t, store, agents, credentials, "user@example.com")
	token, err := store.refresh.Tokens.FindBySignature(context.Background(), signature)
	require.NoError(t, err)
	err = store.refresh.Coordinator.Run(context.Background(), agent.ID, func(scoped context.Context, _ time.Time) error {
		owned := withRefreshOperation(scoped, token.ExpiresAt, client, root)
		_, getErr := store.GetRefreshTokenSession(owned, signature, nil)
		if !errors.Is(getErr, fosite.ErrNotFound) {
			return fmt.Errorf("token usable at exact shared expiry: %v", getErr)
		}
		return nil
	})
	require.NoError(t, err)
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
	store, codeRepo, _, _ := newTestFositeStorage()

	req := &fosite.Request{
		Client:  &fosite.DefaultClient{ID: "not-a-broker-client"},
		Session: &fosite.DefaultSession{Subject: "user@example.com"},
		Form:    map[string][]string{},
	}

	err := store.CreateAuthorizeCodeSession(context.Background(), "somecode", req)
	require.Error(t, err)
	_, err = codeRepo.FindByCodeHash(context.Background(), "somecode")
	require.True(t, ports.IsNotFoundErr(err), "invalid client must not create an authorization code")
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
		store, _, _, _ := newTestFositeStorage()
		store.codeRepo = codeRepo

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
		store, _, _, _ := newTestFositeStorage()
		store.codeRepo = codeRepo

		_, err := store.GetAuthorizeCodeSession(context.Background(), "anycode", nil)
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient non-UUID input returns ErrNotFound", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()

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
		store, _, _, _ := newTestFositeStorage()
		store.clientResolver = resolver

		_, err := store.GetClient(context.Background(), agentID.String())
		assert.Error(t, err)
		assert.NotErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient valid-but-unknown agent UUID returns ErrNotFound", func(t *testing.T) {
		store, _, _, _ := newTestFositeStorage()

		_, err := store.GetClient(context.Background(), id.NewAgentID().String())
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("GetClient agent-exists-but-no-credential returns ErrNotFound", func(t *testing.T) {
		store, _, agentRepo, _ := newTestFositeStorage()
		agentID := id.NewAgentID()
		agent := &dstorage.Agent{ID: agentID, ClientID: ptr.To(id.ClientID("upstream-client")), DisplayName: "Test Agent", Description: "Test agent for infrastructure error tests", PermissionSets: testAgentPermissionSets()}
		require.NoError(t, agentRepo.Create(context.Background(), agent))
		// No credential created — credential repo is empty

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
		store, _, _, _ := newTestFositeStorage()
		store.codeRepo = codeRepo

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
		store, _, _, _ := newTestFositeStorage()
		store.codeRepo = codeRepo

		err := store.InvalidateAuthorizeCodeSession(context.Background(), "anycode")
		assert.ErrorIs(t, err, fosite.ErrInvalidatedAuthorizeCode)
	})

	t.Run("DeletePKCERequestSession infrastructure error is logged and returned", func(t *testing.T) {
		connErr := dstorage.NewStorageError("Delete", dstorage.ErrorKindConnection, nil, "connection refused")
		pkceRepo := &mockPKCERepo{
			deleteFunc: func(_ context.Context, _ string) error { return connErr },
		}
		store, _, _, _ := newTestFositeStorage()
		store.pkceRepo = pkceRepo

		err := store.DeletePKCERequestSession(context.Background(), "anysig")
		assert.Error(t, err)
		assert.NotErrorIs(t, err, fosite.ErrNotFound)
	})

	t.Run("DeletePKCERequestSession not-found returns ErrNotFound", func(t *testing.T) {
		notFoundErr := dstorage.NewStorageError("Delete", dstorage.ErrorKindNotFound, nil, "not found")
		pkceRepo := &mockPKCERepo{
			deleteFunc: func(_ context.Context, _ string) error { return notFoundErr },
		}
		store, _, _, _ := newTestFositeStorage()
		store.pkceRepo = pkceRepo

		err := store.DeletePKCERequestSession(context.Background(), "anysig")
		assert.ErrorIs(t, err, fosite.ErrNotFound)
	})

}
