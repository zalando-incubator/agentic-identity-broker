package memory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type transactionFixture struct {
	adapter    *storageadapter.Adapter
	principal  id.Principal
	user       *ports.User
	agent      *storage.Agent
	provider   *model.ThirdpartyOAuth2ProviderEntity
	permission *storage.PermissionSet
	now        time.Time
}

func newTransactionFixture(t *testing.T) *transactionFixture {
	t.Helper()
	adapter, err := storageadapter.NewAdapter(&ports.StorageConfig{
		Backend:  "memory",
		Timeouts: ports.StorageTimeouts{Read: 2 * time.Second, Write: 2 * time.Second},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close(context.Background())) })

	f := &transactionFixture{adapter: adapter, principal: id.NewPrincipal("memory-tx-subject"), now: time.Now().UTC()}
	f.user = &ports.User{ID: id.NewUserID(), Email: "before@example.com", CreatedAt: f.now, UpdatedAt: f.now}
	require.NoError(t, adapter.Users().CreateUser(context.Background(), f.user))
	f.provider = &model.ThirdpartyOAuth2ProviderEntity{
		ID: id.NewServiceID(), DisplayName: "Original provider", ClientID: id.ClientID("provider-client"),
		Secret: model.NewEncryptedSecret([]byte("encrypted-provider-secret")), IssuerURI: "https://issuer.example.com",
		ProtectedResources: []string{"https://api.example.com/old"},
	}
	require.NoError(t, adapter.Services().Create(context.Background(), f.provider))
	f.permission = &storage.PermissionSet{
		ID: id.NewPermissionSetID(), Name: "Original permission", Description: "Original description",
		ServiceScopes: []storage.ServiceScope{{ServiceID: f.provider.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}},
	}
	require.NoError(t, adapter.PermissionSets().Create(context.Background(), f.permission))
	clientID := id.ClientID("upstream-client")
	f.agent = &storage.Agent{
		ID: id.NewAgentID(), ClientID: &clientID, DisplayName: "Original agent", Description: "Original description",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: f.permission.ID, RequirementType: storage.RequirementTypeOptional}},
	}
	require.NoError(t, adapter.Agents().Create(context.Background(), f.agent))
	return f
}

func (f *transactionFixture) begin(t *testing.T) context.Context {
	t.Helper()
	ctx, err := f.adapter.BeginTX(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.adapter.Rollback(ctx) })
	return ctx
}

func (f *transactionFixture) grant(t *testing.T) *storage.UserGrant {
	t.Helper()
	grant := &storage.UserGrant{
		ID: id.NewGrantID(), Principal: f.principal, AgentID: f.agent.ID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: f.permission.ID, IncludedServiceIDs: []id.ServiceID{f.provider.ID}}},
		CreatedAt:             f.now, UpdatedAt: f.now,
	}
	require.NoError(t, f.adapter.UserGrants().Create(context.Background(), grant))
	return grant
}

func (f *transactionFixture) session(t *testing.T) *storage.UserSession {
	t.Helper()
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: f.principal, ServiceID: f.provider.ID,
		EncryptedAccessToken: []byte("encrypted-access-before"), EncryptedRefreshToken: []byte("encrypted-refresh-before"),
		TokenType: "Bearer", Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: f.provider.ID},
		InitiatedAt: f.now, CreatedAt: f.now, UpdatedAt: f.now,
	}
	require.NoError(t, f.adapter.UserSessions().Create(context.Background(), session))
	return session
}

func (f *transactionFixture) approval(t *testing.T) *storage.ToolApproval {
	t.Helper()
	approval := &storage.ToolApproval{
		ID: id.NewApprovalID(), Principal: f.principal, AgentID: f.agent.ID,
		ToolName: "read_file", ToolPattern: "read_file", ParamsPattern: map[string]string{"path": "safe/*"},
		ArgumentsHash: "approval-hash", Status: storage.ApprovalStatusPending,
		ApprovalURL: "https://broker.example.com/approvals/pending", CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}
	_, err := f.adapter.ToolApprovals().Create(context.Background(), approval)
	require.NoError(t, err)
	return approval
}

func (f *transactionFixture) signingKey(kid string, current bool) *storage.SigningKey {
	return &storage.SigningKey{
		ID: id.NewSigningKeyID(), KID: id.NewKeyID(kid), Algorithm: "ES256", PrivateKeyEncrypted: []byte("encrypted-private-key"),
		IsCurrent: current, ActivatesAt: f.now.Add(-time.Minute), CreatedAt: f.now,
	}
}

func TestMemoryTransactionRollbackRestoresRowsAndIndexes(t *testing.T) {
	t.Run("users and agent client/canonical indexes", func(t *testing.T) {
		f := newTransactionFixture(t)
		originalCanonical, changedCanonical := "original-agent", "changed-agent"
		original := *f.agent
		original.CanonicalID = &originalCanonical
		require.NoError(t, f.adapter.Agents().Update(context.Background(), &original))

		ctx := f.begin(t)
		user := *f.user
		user.Email = "during@example.com"
		require.NoError(t, f.adapter.Users().UpdateUser(ctx, &user))
		agent := original
		agent.ClientID = nil
		agent.ClientURIs = []string{"https://cimd.example.com/client.json"}
		agent.CanonicalID = &changedCanonical
		agent.DisplayName = "Changed agent"
		require.NoError(t, f.adapter.Agents().Update(ctx, &agent))
		require.NoError(t, f.adapter.Rollback(ctx))

		storedUser, err := f.adapter.Users().GetUser(context.Background(), f.user.ID)
		require.NoError(t, err)
		require.Equal(t, f.user.Email, storedUser.Email)
		storedAgent, err := f.adapter.Agents().Get(context.Background(), f.agent.ID)
		require.NoError(t, err)
		require.Equal(t, original.DisplayName, storedAgent.DisplayName)
		require.Equal(t, original.ClientID, storedAgent.ClientID)
		byClient, err := f.adapter.Agents().GetByClientID(context.Background(), *original.ClientID)
		require.NoError(t, err)
		require.Equal(t, f.agent.ID, byClient.ID)
		_, err = f.adapter.Agents().GetByClientURI(context.Background(), agent.ClientURIs[0])
		require.True(t, ports.IsNotFoundErr(err), "rolled-back client URI must not remain indexed: %v", err)
		canonical := f.adapter.Agents().(ports.AgentCanonicalIDRepository)
		byCanonical, err := canonical.GetByCanonicalID(context.Background(), originalCanonical)
		require.NoError(t, err)
		require.Equal(t, f.agent.ID, byCanonical.ID)
		_, err = canonical.GetByCanonicalID(context.Background(), changedCanonical)
		require.True(t, ports.IsNotFoundErr(err), "rolled-back canonical ID must be reusable: %v", err)
	})

	t.Run("providers and permission-set child/index changes", func(t *testing.T) {
		f := newTransactionFixture(t)
		oldCanonical, newCanonical := "original-permission", "new-permission"
		permission := *f.permission
		permission.CanonicalID = &oldCanonical
		require.NoError(t, f.adapter.PermissionSets().Update(context.Background(), &permission))

		ctx := f.begin(t)
		_, err := f.adapter.Services().RenameProtectedResource(ctx, f.provider.ID, "https://api.example.com/old", "https://api.example.com/new")
		require.NoError(t, err)
		permission.Name = "Changed permission"
		permission.CanonicalID = &newCanonical
		permission.ServiceScopes = []storage.ServiceScope{{ServiceID: f.provider.ID, Scopes: []string{"write"}, RequirementType: storage.RequirementTypeOptional}}
		require.NoError(t, f.adapter.PermissionSets().Update(ctx, &permission))
		require.NoError(t, f.adapter.Rollback(ctx))

		oldOwner, err := f.adapter.Services().FindByProtectedResource(context.Background(), "https://api.example.com/old")
		require.NoError(t, err)
		require.Equal(t, f.provider.ID, oldOwner.ID)
		require.Equal(t, f.provider.ProtectedResources, oldOwner.ProtectedResources, "rollback must restore the authoritative child set, not only its owner index")
		resources, version, err := f.adapter.Services().ListProtectedResources(context.Background(), f.provider.ID)
		require.NoError(t, err)
		require.Equal(t, f.provider.ProtectedResources, resources)
		require.Equal(t, f.provider.Version, version)
		_, err = f.adapter.Services().FindByProtectedResource(context.Background(), "https://api.example.com/new")
		var targetErr *tokenexchange.TokenExchangeError
		require.ErrorAs(t, err, &targetErr, "rolled-back protected resource must be unclaimed")
		require.Equal(t, "invalid_target", targetErr.Code())
		stored, err := f.adapter.PermissionSets().Get(context.Background(), f.permission.ID)
		require.NoError(t, err)
		require.Equal(t, f.permission.Name, stored.Name)
		require.Equal(t, []string{"read"}, stored.ServiceScopes[0].Scopes)
		canonical := f.adapter.PermissionSets().(ports.PermissionSetCanonicalIDRepository)
		indexed, err := canonical.GetByCanonicalID(context.Background(), oldCanonical)
		require.NoError(t, err)
		require.Equal(t, f.permission.ID, indexed.ID)
		_, err = canonical.GetByCanonicalID(context.Background(), newCanonical)
		require.True(t, ports.IsNotFoundErr(err), "rolled-back canonical ID must not remain indexed: %v", err)
		other := &storage.PermissionSet{ID: id.NewPermissionSetID(), Name: permission.Name, Description: "Another permission", CanonicalID: &newCanonical,
			ServiceScopes: []storage.ServiceScope{{ServiceID: f.provider.ID, Scopes: []string{"write"}, RequirementType: storage.RequirementTypeOptional}}}
		require.NoError(t, f.adapter.PermissionSets().Create(context.Background(), other), "rolled-back name and canonical ID must be reusable")
	})

	t.Run("grant cascade index and session upsert", func(t *testing.T) {
		f := newTransactionFixture(t)
		grant := f.grant(t)
		session := f.session(t)
		ctx := f.begin(t)
		require.NoError(t, f.adapter.UserGrants().DeleteByAgent(ctx, f.agent.ID))
		replacement := *session
		replacement.ID = id.NewSessionID()
		proposedID := replacement.ID
		replacement.EncryptedAccessToken = []byte("encrypted-access-during")
		replacement.Scope = []string{"write"}
		require.NoError(t, f.adapter.UserSessions().Create(ctx, &replacement))
		require.NoError(t, f.adapter.Rollback(ctx))

		byID, err := f.adapter.UserGrants().Get(context.Background(), grant.ID)
		require.NoError(t, err)
		require.Equal(t, grant.ID, byID.ID)
		byPair, err := f.adapter.UserGrants().FindByPrincipalAndAgent(context.Background(), f.principal, f.agent.ID)
		require.NoError(t, err)
		require.Equal(t, grant.ID, byPair.ID)
		count, err := f.adapter.UserGrants().CountGrantsReferencingPermissionSet(context.Background(), f.permission.ID)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		stored, err := f.adapter.UserSessions().FindByPrincipalAndService(context.Background(), f.principal, f.provider.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, stored.ID)
		require.Equal(t, session.EncryptedAccessToken, stored.EncryptedAccessToken)
		require.Equal(t, []string{"read"}, stored.Scope)
		_, err = f.adapter.UserSessions().Get(context.Background(), proposedID)
		require.True(t, ports.IsNotFoundErr(err), "rolled-back session ID must not remain indexed: %v", err)
	})

	t.Run("approval decision and sync counter", func(t *testing.T) {
		f := newTransactionFixture(t)
		approval := f.approval(t)
		ctx := f.begin(t)
		_, err := f.adapter.ToolApprovals().Approve(ctx, approval.ID,
			storage.ApprovalDecision{Persistence: storage.ApprovalPersistencePermanent, ToolPattern: "read_*", ParamsPattern: map[string]string{"path": "*"}}, f.now)
		require.NoError(t, err)
		version, err := f.adapter.ApprovalSyncState().IncrementVersion(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, version)
		require.NoError(t, f.adapter.Rollback(ctx))

		stored, err := f.adapter.ToolApprovals().Get(context.Background(), approval.ID)
		require.NoError(t, err)
		require.Equal(t, storage.ApprovalStatusPending, stored.Status)
		require.Nil(t, stored.Persistence)
		pending, err := f.adapter.ToolApprovalMetrics().CountPendingByPrincipalAndAgent(context.Background(), f.principal, f.agent.ID)
		require.NoError(t, err)
		require.Equal(t, 1, pending)
		version, err = f.adapter.ApprovalSyncState().GetVersion(context.Background())
		require.NoError(t, err)
		require.Zero(t, version)
	})

	t.Run("credential rotation and signing current index", func(t *testing.T) {
		f := newTransactionFixture(t)
		credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: f.agent.ID, SecretHash: "original-hash", CreatedAt: f.now}
		require.NoError(t, f.adapter.BrokerCredentials().Create(context.Background(), credential))
		current := f.signingKey("original-key", true)
		standby := f.signingKey("standby-key", false)
		require.NoError(t, f.adapter.SigningKeys().Create(context.Background(), current))
		require.NoError(t, f.adapter.SigningKeys().Create(context.Background(), standby))
		ctx := f.begin(t)
		rotated := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: f.agent.ID, SecretHash: "rotated-hash", CreatedAt: f.now}
		require.NoError(t, f.adapter.BrokerCredentials().Rotate(ctx, f.agent.ID, rotated))
		_, err := f.adapter.SigningKeys().SetCurrent(ctx, standby.KID, f.now.Add(-time.Second))
		require.NoError(t, err)
		require.NoError(t, f.adapter.Rollback(ctx))

		stored, err := f.adapter.BrokerCredentials().GetByClientID(context.Background(), id.ClientID(f.agent.ID.String()))
		require.NoError(t, err)
		require.Equal(t, credential.ID, stored.ID)
		require.Equal(t, credential.SecretHash, stored.SecretHash)
		selected, err := f.adapter.SigningKeys().GetCurrent(context.Background())
		require.NoError(t, err)
		require.Equal(t, current.KID, selected.KID)
		unpromoted, err := f.adapter.SigningKeys().GetByKID(context.Background(), standby.KID)
		require.NoError(t, err)
		require.False(t, unpromoted.IsCurrent)
	})

	t.Run("Fosite code refresh and PKCE mutations", func(t *testing.T) {
		f := newTransactionFixture(t)
		code := &storage.AuthorizationCode{
			ID: id.NewAuthorizationCodeID(), CodeHash: "tx-code-hash", AgentID: f.agent.ID, ClientID: id.ClientID(f.agent.ID.String()),
			Principal: f.principal, RedirectURI: "https://client.example.com/callback", CodeChallenge: "challenge", ExpiresAt: f.now.Add(time.Hour), CreatedAt: f.now,
		}
		refresh := &storage.RefreshTokenSession{
			Signature: "tx-refresh-signature", RequestID: "tx-request", AgentID: f.agent.ID, ClientID: id.ClientID(f.agent.ID.String()),
			Principal: f.principal, ExpiresAt: f.now.Add(time.Hour), CreatedAt: f.now,
		}
		pkce := &storage.PKCESession{Signature: "tx-code-signature", CodeChallenge: "challenge", CodeChallengeMethod: "S256", ExpiresAt: f.now.Add(time.Hour), CreatedAt: f.now}
		require.NoError(t, f.adapter.AuthorizationCodes().Create(context.Background(), code))
		require.NoError(t, f.adapter.RefreshTokenSessions().Create(context.Background(), refresh))
		require.NoError(t, f.adapter.PKCESessions().Create(context.Background(), pkce))
		ctx := f.begin(t)
		require.NoError(t, f.adapter.AuthorizationCodes().MarkUsed(ctx, code.ID))
		require.NoError(t, f.adapter.RefreshTokenSessions().MarkUsed(ctx, refresh.Signature))
		require.NoError(t, f.adapter.PKCESessions().Delete(ctx, pkce.Signature))
		require.NoError(t, f.adapter.Rollback(ctx))

		storedCode, err := f.adapter.AuthorizationCodes().FindByCodeHash(context.Background(), code.CodeHash)
		require.NoError(t, err)
		require.Nil(t, storedCode.UsedAt)
		storedRefresh, err := f.adapter.RefreshTokenSessions().FindBySignature(context.Background(), refresh.Signature)
		require.NoError(t, err)
		require.Nil(t, storedRefresh.UsedAt)
		storedPKCE, err := f.adapter.PKCESessions().FindBySignature(context.Background(), pkce.Signature)
		require.NoError(t, err)
		require.Equal(t, pkce.CodeChallenge, storedPKCE.CodeChallenge)
	})
}

func TestMemoryTransactionRollbackRemovesInsertedRowsAndIndexes(t *testing.T) {
	f := newTransactionFixture(t)
	ctx := f.begin(t)
	user := &ports.User{ID: id.NewUserID(), Email: "temporary@example.com", CreatedAt: f.now, UpdatedAt: f.now}
	require.NoError(t, f.adapter.Users().CreateUser(ctx, user))
	canonicalAgent := "temporary-agent"
	agent := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &canonicalAgent,
		ClientURIs: []string{"https://cimd.example.com/temporary.json"}, DisplayName: "Temporary agent", Description: "Temporary description",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: f.permission.ID, RequirementType: storage.RequirementTypeOptional}},
	}
	require.NoError(t, f.adapter.Agents().Create(ctx, agent))
	canonicalService := "temporary-service"
	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID: id.NewServiceID(), CanonicalID: &canonicalService, DisplayName: "Temporary provider", ClientID: id.ClientID("temporary-provider"),
		Secret: model.NewEncryptedSecret([]byte("encrypted-secret")), IssuerURI: "https://issuer.example.com",
		ProtectedResources: []string{"https://api.example.com/temporary"},
	}
	require.NoError(t, f.adapter.Services().Create(ctx, provider))
	canonicalPermission := "temporary-permission"
	permission := &storage.PermissionSet{ID: id.NewPermissionSetID(), Name: "Temporary permission", CanonicalID: &canonicalPermission,
		Description: "Temporary description", ServiceScopes: []storage.ServiceScope{{ServiceID: provider.ID,
			Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}}}
	require.NoError(t, f.adapter.PermissionSets().Create(ctx, permission))
	principal := id.NewPrincipal("temporary-principal")
	grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: principal, AgentID: agent.ID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: permission.ID, IncludedServiceIDs: []id.ServiceID{provider.ID}}},
		CreatedAt:             f.now, UpdatedAt: f.now}
	require.NoError(t, f.adapter.UserGrants().Create(ctx, grant))
	session := &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: provider.ID,
		EncryptedAccessToken: []byte("encrypted-access"), TokenType: "Bearer", EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID}}
	require.NoError(t, f.adapter.UserSessions().Create(ctx, session))
	approval := &storage.ToolApproval{ID: id.NewApprovalID(), Principal: principal, AgentID: agent.ID, ToolName: "read_file",
		ToolPattern: "read_file", ArgumentsHash: "temporary-hash", Status: storage.ApprovalStatusPending, ExpiresAt: f.now.Add(time.Hour)}
	_, err := f.adapter.ToolApprovals().Create(ctx, approval)
	require.NoError(t, err)
	credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "temporary-hash"}
	require.NoError(t, f.adapter.BrokerCredentials().Create(ctx, credential))
	key := f.signingKey("temporary-key", true)
	require.NoError(t, f.adapter.SigningKeys().Create(ctx, key))
	code := &storage.AuthorizationCode{ID: id.NewAuthorizationCodeID(), CodeHash: "temporary-code", AgentID: agent.ID,
		ClientID: id.ClientID(agent.ID.String()), Principal: principal, RedirectURI: "https://client.example.com/callback",
		CodeChallenge: "challenge", ExpiresAt: f.now.Add(time.Hour)}
	require.NoError(t, f.adapter.AuthorizationCodes().Create(ctx, code))
	refresh := &storage.RefreshTokenSession{Signature: "temporary-refresh", RequestID: "temporary-request", AgentID: agent.ID,
		ClientID: id.ClientID(agent.ID.String()), Principal: principal, ExpiresAt: f.now.Add(time.Hour)}
	require.NoError(t, f.adapter.RefreshTokenSessions().Create(ctx, refresh))
	pkce := &storage.PKCESession{Signature: "temporary-pkce", CodeChallenge: "challenge", ExpiresAt: f.now.Add(time.Hour)}
	require.NoError(t, f.adapter.PKCESessions().Create(ctx, pkce))
	require.NoError(t, f.adapter.Rollback(ctx))

	_, err = f.adapter.Users().GetUser(context.Background(), user.ID)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back user persisted: %v", err)
	_, err = f.adapter.Agents().GetByClientURI(context.Background(), agent.ClientURIs[0])
	require.True(t, ports.IsNotFoundErr(err), "rolled-back client URI remains indexed: %v", err)
	_, err = f.adapter.Agents().(ports.AgentCanonicalIDRepository).GetByCanonicalID(context.Background(), canonicalAgent)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back agent canonical ID remains indexed: %v", err)
	_, err = f.adapter.Services().FindByProtectedResource(context.Background(), provider.ProtectedResources[0])
	var targetErr *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &targetErr, "rolled-back provider resource remains claimed")
	require.Equal(t, "invalid_target", targetErr.Code())
	_, err = f.adapter.Services().(ports.ThirdpartyOAuth2ProviderCanonicalIDRepository).GetByCanonicalID(context.Background(), canonicalService)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back provider canonical ID remains indexed: %v", err)
	_, err = f.adapter.PermissionSets().(ports.PermissionSetCanonicalIDRepository).GetByCanonicalID(context.Background(), canonicalPermission)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back permission canonical ID remains indexed: %v", err)
	require.NoError(t, f.adapter.PermissionSets().Create(context.Background(), &storage.PermissionSet{
		ID: id.NewPermissionSetID(), Name: permission.Name, CanonicalID: &canonicalPermission, Description: "Reused after rollback",
		ServiceScopes: []storage.ServiceScope{{ServiceID: f.provider.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}},
	}), "rollback must release both permission-set uniqueness indexes")
	_, err = f.adapter.Services().AddProtectedResource(context.Background(), f.provider.ID, provider.ProtectedResources[0])
	require.NoError(t, err, "rollback must release provider resource ownership")
	_, err = f.adapter.UserGrants().FindByPrincipalAndAgent(context.Background(), principal, agent.ID)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back grant remains indexed: %v", err)
	storedSession, err := f.adapter.UserSessions().FindByPrincipalAndService(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession)
	_, err = f.adapter.ToolApprovals().Get(context.Background(), approval.ID)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back approval persisted: %v", err)
	_, err = f.adapter.BrokerCredentials().GetByClientID(context.Background(), id.ClientID(agent.ID.String()))
	require.True(t, ports.IsNotFoundErr(err), "rolled-back credential persisted: %v", err)
	_, err = f.adapter.SigningKeys().GetByKID(context.Background(), key.KID)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back signing key persisted: %v", err)
	_, err = f.adapter.AuthorizationCodes().FindByCodeHash(context.Background(), code.CodeHash)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back authorization code persisted: %v", err)
	_, err = f.adapter.RefreshTokenSessions().FindBySignature(context.Background(), refresh.Signature)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back refresh session persisted: %v", err)
	_, err = f.adapter.PKCESessions().FindBySignature(context.Background(), pkce.Signature)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back PKCE session persisted: %v", err)
}

func TestMemoryTransactionJoinedScopesDoNotCommitOwner(t *testing.T) {
	f := newTransactionFixture(t)
	owner := f.begin(t)
	first := &ports.User{ID: id.NewUserID(), Email: "owner@example.com", CreatedAt: f.now, UpdatedAt: f.now}
	second := &ports.User{ID: id.NewUserID(), Email: "joined@example.com", CreatedAt: f.now, UpdatedAt: f.now}
	require.NoError(t, f.adapter.Users().CreateUser(owner, first))
	joined, err := f.adapter.BeginTX(owner)
	require.NoError(t, err)
	require.NoError(t, f.adapter.Users().CreateUser(joined, second))
	require.NoError(t, f.adapter.Commit(joined))

	// A standalone reader must not see either write merely because the joined scope committed.
	started := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		close(started)
		_, err := f.adapter.Users().GetUser(context.Background(), second.ID)
		readDone <- err
	}()
	<-started
	select {
	case err := <-readDone:
		t.Fatalf("joined commit exposed the owner's uncommitted row: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	require.NoError(t, f.adapter.Commit(owner))
	select {
	case err := <-readDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("standalone reader stayed blocked after owner commit")
	}
	for _, user := range []*ports.User{first, second} {
		stored, err := f.adapter.Users().GetUser(context.Background(), user.ID)
		require.NoError(t, err)
		require.Equal(t, user.Email, stored.Email)
	}
}

func TestMemoryTransactionJoinedRollbackPoisonsOwner(t *testing.T) {
	f := newTransactionFixture(t)
	owner := f.begin(t)
	user := &ports.User{ID: id.NewUserID(), Email: "rollback-only@example.com", CreatedAt: f.now, UpdatedAt: f.now}
	require.NoError(t, f.adapter.Users().CreateUser(owner, user))
	joined, err := f.adapter.BeginTX(owner)
	require.NoError(t, err)
	require.NoError(t, f.adapter.Agents().Update(joined, &storage.Agent{
		ID: f.agent.ID, ClientID: f.agent.ClientID, DisplayName: "Joined update", Description: f.agent.Description,
		PermissionSets: f.agent.PermissionSets,
	}))
	require.NoError(t, f.adapter.Rollback(joined))
	require.Error(t, f.adapter.Commit(owner), "a poisoned owner must not report a successful commit")
	_, err = f.adapter.Users().GetUser(context.Background(), user.ID)
	require.True(t, ports.IsNotFoundErr(err), "owner's writes must roll back: %v", err)
	stored, err := f.adapter.Agents().Get(context.Background(), f.agent.ID)
	require.NoError(t, err)
	require.Equal(t, f.agent.DisplayName, stored.DisplayName)
}

func TestMemoryTransactionRejectsStrongerJoinedIsolation(t *testing.T) {
	f := newTransactionFixture(t)
	owner := f.begin(t)
	_, err := f.adapter.BeginTX(ports.WithStorageTransactionHints(owner, ports.StorageTransactionHints{Isolation: ports.StorageSerializable}))
	require.Error(t, err, "serializable work cannot join a weaker transaction")
	require.NoError(t, f.adapter.Rollback(owner))
}

func TestMemoryTransactionExcludesStandaloneReadersAndWriters(t *testing.T) {
	f := newTransactionFixture(t)
	owner := f.begin(t)
	changed := *f.user
	changed.Email = "uncommitted@example.com"
	require.NoError(t, f.adapter.Users().UpdateUser(owner, &changed))
	other := &ports.User{ID: id.NewUserID(), Email: "independent@example.com", CreatedAt: f.now, UpdatedAt: f.now}
	readerStarted, writerStarted := make(chan struct{}), make(chan struct{})
	readDone := make(chan *ports.User, 1)
	writeDone := make(chan error, 1)
	go func() {
		close(readerStarted)
		user, _ := f.adapter.Users().GetUser(context.Background(), changed.ID)
		readDone <- user
	}()
	go func() {
		close(writerStarted)
		writeDone <- f.adapter.Users().CreateUser(context.Background(), other)
	}()
	<-readerStarted
	<-writerStarted
	select {
	case user := <-readDone:
		t.Fatalf("standalone read passed the uncommitted visibility gate: %+v", user)
	case err := <-writeDone:
		t.Fatalf("standalone write passed the uncommitted visibility gate: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	require.NoError(t, f.adapter.Rollback(owner))
	select {
	case user := <-readDone:
		require.NotNil(t, user)
		require.Equal(t, f.user.Email, user.Email, "reader must observe rolled-back committed state")
	case <-time.After(2 * time.Second):
		t.Fatal("reader stayed blocked after rollback")
	}
	select {
	case err := <-writeDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("independent writer stayed blocked after rollback")
	}
	stored, err := f.adapter.Users().GetUser(context.Background(), other.ID)
	require.NoError(t, err)
	require.Equal(t, other.Email, stored.Email)
}

func TestMemoryTransactionAfterCommitNotifications(t *testing.T) {
	t.Run("join does not broadcast until owning commit and releases the gate first", func(t *testing.T) {
		f := newTransactionFixture(t)
		owner := f.begin(t)
		effects, ok := ports.StorageTransactionEffectsFromContext(owner)
		require.True(t, ok, "owner must expose commit-only effects")
		user := &ports.User{ID: id.NewUserID(), Email: "notified@example.com", CreatedAt: f.now, UpdatedAt: f.now}
		require.NoError(t, f.adapter.Users().CreateUser(owner, user))
		joined, err := f.adapter.BeginTX(owner)
		require.NoError(t, err)
		joinedEffects, ok := ports.StorageTransactionEffectsFromContext(joined)
		require.True(t, ok, "joined scopes must share the owner's effects")
		broadcast := make(chan error, 2)
		require.NoError(t, effects.AfterCommit(func() {
			stored, err := f.adapter.Users().GetUser(context.Background(), user.ID)
			if err == nil && stored.Email != user.Email {
				err = fmt.Errorf("committed email %q, want %q", stored.Email, user.Email)
			}
			broadcast <- err
		}))
		require.NoError(t, joinedEffects.AfterCommit(func() { broadcast <- nil }))
		require.NoError(t, f.adapter.Commit(joined))
		select {
		case <-broadcast:
			t.Fatal("joined commit broadcast an uncommitted change")
		default:
		}
		commitDone := make(chan error, 1)
		go func() { commitDone <- f.adapter.Commit(owner) }()
		select {
		case err := <-commitDone:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("commit callback tried to read while the transaction still held the visibility gate")
		}
		for range 2 {
			select {
			case err := <-broadcast:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("committed notification was not delivered")
			}
		}
		select {
		case <-broadcast:
			t.Fatal("commit invoked a notification more than once")
		default:
		}
	})

	t.Run("rollback discards owner and joined notifications", func(t *testing.T) {
		f := newTransactionFixture(t)
		owner := f.begin(t)
		joined, err := f.adapter.BeginTX(owner)
		require.NoError(t, err)
		ownerEffects, ok := ports.StorageTransactionEffectsFromContext(owner)
		require.True(t, ok)
		joinedEffects, ok := ports.StorageTransactionEffectsFromContext(joined)
		require.True(t, ok)
		broadcasts := make(chan struct{}, 2)
		require.NoError(t, ownerEffects.AfterCommit(func() { broadcasts <- struct{}{} }))
		require.NoError(t, joinedEffects.AfterCommit(func() { broadcasts <- struct{}{} }))
		require.NoError(t, f.adapter.Commit(joined))
		require.NoError(t, f.adapter.Rollback(owner))
		select {
		case <-broadcasts:
			t.Fatal("rollback released a commit-only notification")
		default:
		}
	})
}

func TestMemoryStorageReturnsIndependentMutableValues(t *testing.T) {
	f := newTransactionFixture(t)
	ctx := context.Background()

	f.agent.PermissionSets[0].RequirementType = "changed-input"
	agent, err := f.adapter.Agents().Get(ctx, f.agent.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RequirementTypeOptional, agent.PermissionSets[0].RequirementType)
	agent.PermissionSets[0].RequirementType = "changed-result"
	again, err := f.adapter.Agents().Get(ctx, f.agent.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RequirementTypeOptional, again.PermissionSets[0].RequirementType)

	f.permission.ServiceScopes[0].Scopes[0] = "changed-input"
	permission, err := f.adapter.PermissionSets().Get(ctx, f.permission.ID)
	require.NoError(t, err)
	require.Equal(t, "read", permission.ServiceScopes[0].Scopes[0])
	permission.ServiceScopes[0].Scopes[0] = "changed-result"
	permission, err = f.adapter.PermissionSets().Get(ctx, f.permission.ID)
	require.NoError(t, err)
	require.Equal(t, "read", permission.ServiceScopes[0].Scopes[0])

	session := f.session(t)
	session.EncryptedAccessToken[0] = 'X'
	stored, err := f.adapter.UserSessions().Get(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, []byte("encrypted-access-before"), stored.EncryptedAccessToken)
	stored.EncryptedAccessToken[0] = 'Y'
	stored.Scope[0] = "write"
	listed, err := f.adapter.UserSessions().ListByPrincipal(ctx, f.principal)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, []byte("encrypted-access-before"), listed[0].EncryptedAccessToken)
	require.Equal(t, []string{"read"}, listed[0].Scope)

	grant := f.grant(t)
	grant.GrantedPermissionSets[0].IncludedServiceIDs[0] = id.NewServiceID()
	gotGrant, err := f.adapter.UserGrants().Get(ctx, grant.ID)
	require.NoError(t, err)
	require.Equal(t, f.provider.ID, gotGrant.GrantedPermissionSets[0].IncludedServiceIDs[0])
	gotGrant.GrantedPermissionSets[0].IncludedServiceIDs[0] = id.NewServiceID()
	gotGrant, err = f.adapter.UserGrants().Get(ctx, grant.ID)
	require.NoError(t, err)
	require.Equal(t, f.provider.ID, gotGrant.GrantedPermissionSets[0].IncludedServiceIDs[0])

	approval := f.approval(t)
	approval.ParamsPattern["path"] = "changed-input"
	gotApproval, err := f.adapter.ToolApprovals().Get(ctx, approval.ID)
	require.NoError(t, err)
	require.Equal(t, "safe/*", gotApproval.ParamsPattern["path"])
	gotApproval.ParamsPattern["path"] = "changed-result"
	gotApproval, err = f.adapter.ToolApprovals().Get(ctx, approval.ID)
	require.NoError(t, err)
	require.Equal(t, "safe/*", gotApproval.ParamsPattern["path"])

	f.provider.ProtectedResources[0] = "https://api.example.com/changed"
	provider, err := f.adapter.Services().Get(ctx, f.provider.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"https://api.example.com/old"}, provider.ProtectedResources)
	provider.ProtectedResources[0] = "https://api.example.com/mutated"
	provider, err = f.adapter.Services().Get(ctx, f.provider.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"https://api.example.com/old"}, provider.ProtectedResources)

	key := f.signingKey("independent-key", true)
	require.NoError(t, f.adapter.SigningKeys().Create(ctx, key))
	key.PrivateKeyEncrypted[0] = 'X'
	gotKey, err := f.adapter.SigningKeys().GetByKID(ctx, key.KID)
	require.NoError(t, err)
	require.Equal(t, []byte("encrypted-private-key"), gotKey.PrivateKeyEncrypted)
	gotKey.PrivateKeyEncrypted[0] = 'Y'
	gotKey, err = f.adapter.SigningKeys().GetByKID(ctx, key.KID)
	require.NoError(t, err)
	require.Equal(t, []byte("encrypted-private-key"), gotKey.PrivateKeyEncrypted)

	credentialTime := f.now
	credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: f.agent.ID,
		SecretHash: "independent-hash", RotatedAt: &credentialTime}
	require.NoError(t, f.adapter.BrokerCredentials().Create(ctx, credential))
	credentialTime = credentialTime.Add(time.Hour)
	gotCredential, err := f.adapter.BrokerCredentials().GetByAgentID(ctx, f.agent.ID)
	require.NoError(t, err)
	require.Equal(t, f.now, *gotCredential.RotatedAt)
	*gotCredential.RotatedAt = f.now.Add(2 * time.Hour)
	gotCredential, err = f.adapter.BrokerCredentials().GetByAgentID(ctx, f.agent.ID)
	require.NoError(t, err)
	require.Equal(t, f.now, *gotCredential.RotatedAt)

	email := "original@example.com"
	code := &storage.AuthorizationCode{ID: id.NewAuthorizationCodeID(), CodeHash: "independent-code", AgentID: f.agent.ID,
		ClientID: id.ClientID(f.agent.ID.String()), Principal: f.principal, Email: &email,
		RedirectURI: "https://client.example.com/callback", CodeChallenge: "challenge", ExpiresAt: f.now.Add(time.Hour)}
	require.NoError(t, f.adapter.AuthorizationCodes().Create(ctx, code))
	refresh := &storage.RefreshTokenSession{Signature: "independent-refresh", RequestID: "independent-request",
		AgentID: f.agent.ID, ClientID: id.ClientID(f.agent.ID.String()), Principal: f.principal, Email: &email, ExpiresAt: f.now.Add(time.Hour)}
	require.NoError(t, f.adapter.RefreshTokenSessions().Create(ctx, refresh))
	email = "mutated-input@example.com"
	gotCode, err := f.adapter.AuthorizationCodes().FindByCodeHash(ctx, code.CodeHash)
	require.NoError(t, err)
	require.Equal(t, "original@example.com", *gotCode.Email)
	gotRefresh, err := f.adapter.RefreshTokenSessions().FindBySignature(ctx, refresh.Signature)
	require.NoError(t, err)
	require.Equal(t, "original@example.com", *gotRefresh.Email)
	*gotCode.Email = "mutated-result@example.com"
	*gotRefresh.Email = "mutated-result@example.com"
	gotCode, err = f.adapter.AuthorizationCodes().FindByCodeHash(ctx, code.CodeHash)
	require.NoError(t, err)
	require.Equal(t, "original@example.com", *gotCode.Email)
	gotRefresh, err = f.adapter.RefreshTokenSessions().FindBySignature(ctx, refresh.Signature)
	require.NoError(t, err)
	require.Equal(t, "original@example.com", *gotRefresh.Email)
}

func TestMemoryDeliveryCallbackDoesNotHoldBusinessVisibilityGate(t *testing.T) {
	f := newTransactionFixture(t)
	grant := f.grant(t)
	actor := f.principal.String()
	event := &model.BusinessEvent{
		ID: id.NewBusinessEventID(), Type: "agentic-identity-broker.grant-created", Source: model.BusinessEventSource,
		OccurredAt: f.now, Subject: &f.principal, Actor: model.BusinessEventActor{Kind: "user", ID: &actor},
		AgentID: f.agent.ID, GrantID: grant.ID, Outcome: model.BusinessEventSuccess,
		ReasonUser: "A new user-to-agent delegation is committed.", ReasonAdmin: "A new user-to-agent delegation is committed.", Data: map[string]any{},
	}
	require.NoError(t, f.adapter.BusinessEvents().Append(context.Background(), event, true))
	require.False(t, event.RecordedAt.IsZero(), "the retained event must have a storage-assigned key before dispatch")
	key := model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	entered := make(chan id.BusinessEventID, 1)
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	type dispatchResult struct {
		delivered bool
		err       error
	}
	dispatched := make(chan dispatchResult, 1)
	go func() {
		ok, err := f.adapter.BusinessEventDelivery().DispatchOne(ctx, key, func(ctx context.Context, retained *model.BusinessEvent) error {
			entered <- retained.ID
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		dispatched <- dispatchResult{delivered: ok, err: err}
	}()
	select {
	case idSeen := <-entered:
		require.Equal(t, event.ID, idSeen)
	case result := <-dispatched:
		t.Fatalf("dispatch returned without synchronously invoking the collector callback: %+v", result)
	case <-ctx.Done():
		t.Fatal("dispatch did not enter the collector callback")
	}

	// The callback still holds deletion protection, but unrelated business storage must
	// use the visibility gate while collector I/O waits outside that gate.
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- f.adapter.Users().CreateUser(context.Background(), &ports.User{
			ID: id.NewUserID(), Email: "during-collector@example.com", CreatedAt: f.now, UpdatedAt: f.now,
		})
	}()
	select {
	case err := <-writerDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("collector callback held the business visibility gate")
	}
	close(release)
	select {
	case result := <-dispatched:
		require.NoError(t, result.err)
		require.True(t, result.delivered)
	case <-ctx.Done():
		t.Fatal("dispatch did not finish after collector release")
	}
}
