package oauth2server

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func credentialLifecycleService(f *providerRefreshFixture) *CredentialService {
	return NewCredentialService(
		f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth, testSlogger(),
		f.provider.refreshDeps.Coordinator, f.provider.refreshDeps.Revocations,
	)
}

func newPublicCredentialLifecycleFixture(t *testing.T) *providerRefreshFixture {
	t.Helper()
	provider, agents, _ := newTestProvider(t)
	agent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "Public local agent", Description: "Credential generation over an existing public refresh session",
		PermissionSets: testAgentPermissionSets(), RedirectURIs: []string{providerRefreshRedirect}, AllowedScopes: []string{"offline_access", "read"},
	}
	require.NoError(t, agents.Create(context.Background(), agent))
	seedProviderTestGrant(t, provider, agent.ID)
	fixture := &providerRefreshFixture{provider: provider, agents: agents, agent: agent}
	issueProviderRefresh(t, fixture, agent.ID.String(), providerRefreshRedirect, "offline_access read")
	return fixture
}

func issueCredentialLifecycleForPrincipal(t *testing.T, f *providerRefreshFixture, principal id.Principal) (*ports.TokenResponse, *storage.RefreshSession) {
	t.Helper()
	ctx := context.Background()
	grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
	require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), Principal: principal, AgentID: f.agent.ID}))
	verifier := "credential-lifecycle-verifier-01234567890123456789"
	code, err := f.provider.HandleAuthorize(ctx, f.agent.ID.String(), providerRefreshRedirect, "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", principal)
	require.NoError(t, err)
	issued, err := f.provider.HandleAuthorizationCodeExchange(ctx, f.agent.ID.String(), f.secret, code, providerRefreshRedirect, verifier)
	require.NoError(t, err)
	require.NotEmpty(t, issued.RefreshToken)
	record, err := f.provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	rootID, err := id.ParseRefreshSessionID(record.ID.String())
	require.NoError(t, err)
	root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	require.NoError(t, err)
	require.Equal(t, principal, root.Principal)
	return issued, root
}

func requireCredentialRevokedRoot(t *testing.T, before, after *storage.RefreshSession) {
	t.Helper()
	require.NotNil(t, after.RevokedAt)
	require.False(t, after.RevokedAt.Before(before.StartedAt))
	require.Nil(t, after.ExpiredAt)
	reason := storage.RefreshReasonCredentialRevoked
	expected := *before
	expected.RevokedAt = after.RevokedAt
	expected.TerminalReason = &reason
	expected.RetryCiphertext = nil
	require.Equal(t, &expected, after, "revocation must preserve origin, clocks, token history, and retry metadata except erased retry result")
}

func TestCredentialService_RevokeTerminatesEveryPrincipalWithoutPublicFallback(t *testing.T) {
	f := newProviderRefreshFixture(t)
	ctx := context.Background()
	secondIssued, secondRoot := issueCredentialLifecycleForPrincipal(t, f, id.NewPrincipal("second@example.com"))
	otherAgent, _, otherSecret := setupTestCredentials(t, f.provider, f.agents)
	otherAgent.RedirectURIs = []string{providerRefreshRedirect}
	otherAgent.AllowedScopes = []string{"offline_access", "read"}
	require.NoError(t, f.agents.Update(ctx, otherAgent))
	other := &providerRefreshFixture{provider: f.provider, agents: f.agents, agent: otherAgent, secret: otherSecret}
	issueProviderRefresh(t, other, otherAgent.ID.String(), providerRefreshRedirect, "offline_access read")
	beforeOther := other.root
	beforeFirst := f.root

	coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator}
	service := NewCredentialService(f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth, testSlogger(), coordinator, f.provider.refreshDeps.Revocations)
	require.NoError(t, service.Revoke(ctx, f.agent.ID))
	_, err := f.provider.fositeStorage.credRepo.GetByAgentID(ctx, f.agent.ID)
	require.True(t, ports.IsNotFoundErr(err))
	for _, before := range []*storage.RefreshSession{beforeFirst, secondRoot} {
		after, lookupErr := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
		require.NoError(t, lookupErr)
		requireCredentialRevokedRoot(t, before, after)
		require.Equal(t, coordinator.decisionAt.UTC(), *after.RevokedAt)
	}
	unaffected, err := f.provider.refreshDeps.Sessions.FindByID(ctx, beforeOther.ID)
	require.NoError(t, err)
	require.Equal(t, beforeOther, unaffected)

	// Once credentials are deleted, this local client falls back to public PKCE.
	// Neither that fallback nor new credentials may revive its old refresh roots.
	f.requireDenial(t, f.agent.ID.String(), "", f.issued.RefreshToken, "", "invalid_grant")
	second := &providerRefreshFixture{provider: f.provider, agents: f.agents, agent: f.agent, secret: f.secret, issued: secondIssued, root: secondRoot}
	second.requireDenial(t, f.agent.ID.String(), "", secondIssued.RefreshToken, "", "invalid_grant")
	generated, err := credentialLifecycleService(f).Generate(ctx, f.agent.ID)
	require.NoError(t, err)
	require.False(t, generated.Rotated)
	require.NotEmpty(t, generated.PlaintextSecret)
	f.requireDenial(t, f.agent.ID.String(), generated.PlaintextSecret, f.issued.RefreshToken, "", "invalid_grant")
	second.requireDenial(t, f.agent.ID.String(), generated.PlaintextSecret, secondIssued.RefreshToken, "", "invalid_grant")
	refreshedOther, err := other.provider.HandleRefreshToken(ctx, otherAgent.ID.String(), otherSecret, other.issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, refreshedOther.RefreshToken)
}

func TestCredentialService_GeneratePreservesExistingRefreshAuthority(t *testing.T) {
	t.Run("first credential generation preserves public session and requires its new secret", func(t *testing.T) {
		f := newPublicCredentialLifecycleFixture(t)
		ctx := context.Background()
		before := f.root
		coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator}
		service := NewCredentialService(f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth, testSlogger(), coordinator, f.provider.refreshDeps.Revocations)
		result, err := service.Generate(ctx, f.agent.ID)
		require.NoError(t, err)
		require.False(t, result.Rotated)
		require.NotNil(t, result.Credential)
		require.NotEmpty(t, result.PlaintextSecret)
		require.Equal(t, coordinator.decisionAt.UTC(), result.Credential.CreatedAt)
		require.Nil(t, result.Credential.RotatedAt)
		stored, err := f.provider.fositeStorage.credRepo.GetByAgentID(ctx, f.agent.ID)
		require.NoError(t, err)
		require.Equal(t, result.Credential, stored)
		root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
		require.NoError(t, err)
		require.Equal(t, before, root)
		f.requireDenial(t, f.agent.ID.String(), "", f.issued.RefreshToken, "", "invalid_client")
		refreshed, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), result.PlaintextSecret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, refreshed.RefreshToken)
		root, err = f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
		require.NoError(t, err)
		require.Equal(t, before.OriginalGrantID, root.OriginalGrantID)
		require.Equal(t, before.OriginalTokenSignature, root.OriginalTokenSignature)
		require.Equal(t, before.StartedAt, root.StartedAt)
		require.Nil(t, root.TerminalReason)
	})

	t.Run("replacement preserves retry metadata and only the new secret can refresh", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		ctx := context.Background()
		f.provider.refreshDeps.Policy.ReuseInterval = 30 * time.Second
		f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
		fresh, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "read")
		require.NoError(t, err)
		require.NotEmpty(t, fresh.RefreshToken)
		before, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
		require.NoError(t, err)
		require.NotNil(t, before.PreviousSignature)
		require.NotNil(t, before.RetryExpiresAt)
		require.NotNil(t, before.RetryAccessExpiresAt)
		require.NotNil(t, before.OriginalRequestedScope)
		require.Equal(t, "read", *before.OriginalRequestedScope)
		oldCredential, err := f.provider.fositeStorage.credRepo.GetByAgentID(ctx, f.agent.ID)
		require.NoError(t, err)

		coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator}
		service := NewCredentialService(f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth, testSlogger(), coordinator, f.provider.refreshDeps.Revocations)
		result, err := service.Generate(ctx, f.agent.ID)
		require.NoError(t, err)
		require.True(t, result.Rotated)
		require.NotNil(t, result.Credential)
		require.NotEmpty(t, result.PlaintextSecret)
		require.NotEqual(t, oldCredential.ID, result.Credential.ID)
		require.NotNil(t, result.Credential.RotatedAt)
		require.Equal(t, coordinator.decisionAt.UTC(), result.Credential.CreatedAt)
		require.Equal(t, coordinator.decisionAt.UTC(), *result.Credential.RotatedAt)
		stored, err := f.provider.fositeStorage.credRepo.GetByAgentID(ctx, f.agent.ID)
		require.NoError(t, err)
		require.Equal(t, result.Credential, stored)
		unchanged, err := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
		require.NoError(t, err)
		require.Equal(t, before, unchanged)
		f.requireDenial(t, f.agent.ID.String(), f.secret, fresh.RefreshToken, "", "invalid_client")
		next, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), result.PlaintextSecret, fresh.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, next.RefreshToken)
		updated, err := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
		require.NoError(t, err)
		require.Equal(t, before.OriginalGrantID, updated.OriginalGrantID)
		require.Equal(t, before.OriginalTokenSignature, updated.OriginalTokenSignature)
		require.Equal(t, before.StartedAt, updated.StartedAt)
		require.Equal(t, before.Scope, updated.Scope)
		require.Nil(t, updated.TerminalReason)
	})
}

func TestCredentialService_GenerateMissingAgentReturnsNoSecret(t *testing.T) {
	f := newProviderRefreshFixture(t)
	result, err := credentialLifecycleService(f).Generate(context.Background(), id.NewAgentID())
	require.ErrorIs(t, err, ports.ErrCredentialAgentNotFound)
	require.Equal(t, ports.CredentialGenerationResult{}, result)
}

func TestCredentialService_RevokeMissingCredentialPreservesPublicSession(t *testing.T) {
	f := newPublicCredentialLifecycleFixture(t)
	ctx := context.Background()
	before := f.root
	require.True(t, ports.IsNotFoundErr(credentialLifecycleService(f).Revoke(ctx, f.agent.ID)))
	root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before, root)
	refreshed, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), "", f.issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, refreshed.RefreshToken)
}

type credentialLifecycleFaultRepository struct {
	ports.ClientCredentialRepository
	readErr, createErr, rotateErr, deleteErr error
}

func (r *credentialLifecycleFaultRepository) GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	credential, err := r.ClientCredentialRepository.GetByAgentID(ctx, agentID)
	if r.readErr != nil {
		return nil, r.readErr
	}
	return credential, err
}

func (r *credentialLifecycleFaultRepository) Create(ctx context.Context, credential *storage.ClientCredential) error {
	if err := r.ClientCredentialRepository.Create(ctx, credential); err != nil {
		return err
	}
	return r.createErr
}

func (r *credentialLifecycleFaultRepository) Rotate(ctx context.Context, agentID id.AgentID, credential *storage.ClientCredential) error {
	if err := r.ClientCredentialRepository.Rotate(ctx, agentID, credential); err != nil {
		return err
	}
	return r.rotateErr
}

func (r *credentialLifecycleFaultRepository) Delete(ctx context.Context, agentID id.AgentID) error {
	if err := r.ClientCredentialRepository.Delete(ctx, agentID); err != nil {
		return err
	}
	return r.deleteErr
}

type credentialLifecycleFaultRevocations struct {
	ports.RefreshSessionRevocationRepository
	beforeErr, afterErr error
	rootID              id.RefreshSessionID
	sessions            ports.RefreshSessionRepository
	staged              bool
}

func (r *credentialLifecycleFaultRevocations) RevokeByAgent(ctx context.Context, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	if r.beforeErr != nil {
		return r.beforeErr
	}
	if err := r.RefreshSessionRevocationRepository.RevokeByAgent(ctx, agentID, at, reason); err != nil {
		return err
	}
	if r.afterErr != nil {
		root, err := r.sessions.FindByID(ctx, r.rootID)
		r.staged = err == nil && root.TerminalReason != nil && *root.TerminalReason == storage.RefreshReasonCredentialRevoked
		return r.afterErr
	}
	return nil
}

type credentialLifecycleFaultAgents struct {
	ports.AgentRepository
	readErr error
}

func (r *credentialLifecycleFaultAgents) Get(context.Context, id.AgentID) (*storage.Agent, error) {
	return nil, r.readErr
}

type credentialLifecycleFaultCoordinator struct {
	ports.AuthorizationSessionCoordinator
	beforeErr, afterErr, afterCommitErr error
	afterStaged                         func(context.Context) error
	decisionAt                          time.Time
	staged                              bool
}

func (c *credentialLifecycleFaultCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	if c.beforeErr != nil {
		return c.beforeErr
	}
	err := c.AuthorizationSessionCoordinator.Run(ctx, agentID, func(owner context.Context, at time.Time) error {
		c.decisionAt = at
		if err := operation(owner, at); err != nil {
			return err
		}
		if c.afterStaged != nil {
			if err := c.afterStaged(owner); err != nil {
				return err
			}
			c.staged = true
		}
		return c.afterErr
	})
	if err != nil {
		return err
	}
	return c.afterCommitErr
}

func TestCredentialService_GenerateFaultsReturnNoSecretAndPreserveAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, fault string
		public      bool
	}{
		{name: "existing credential read", fault: "read"},
		{name: "agent infrastructure read", fault: "agent"},
		{name: "missing credential read", fault: "read", public: true},
		{name: "credential create after staging", fault: "create", public: true},
		{name: "credential replacement after staging", fault: "rotate"},
		{name: "coordinator unavailable", fault: "coordinator"},
		{name: "owner abort after staging replacement", fault: "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var f *providerRefreshFixture
			if tc.public {
				f = newPublicCredentialLifecycleFixture(t)
			} else {
				f = newProviderRefreshFixture(t)
			}
			ctx := context.Background()
			storedRepo := f.provider.fositeStorage.credRepo
			beforeCredential, credentialErr := storedRepo.GetByAgentID(ctx, f.agent.ID)
			if tc.public {
				require.True(t, ports.IsNotFoundErr(credentialErr))
			} else {
				require.NoError(t, credentialErr)
			}
			beforeRoot := f.root
			beforeToken, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRoot.CurrentSignature)
			require.NoError(t, err)
			fault := errors.New("credential generation dependency failed")
			repository := &credentialLifecycleFaultRepository{ClientCredentialRepository: storedRepo}
			coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator}
			agentRepo := ports.AgentRepository(f.agents)
			if tc.fault == "agent" {
				agentRepo = &credentialLifecycleFaultAgents{AgentRepository: f.agents, readErr: fault}
			}
			switch tc.fault {
			case "read":
				repository.readErr = fault
			case "create":
				repository.createErr = fault
			case "rotate":
				repository.rotateErr = fault
			case "coordinator":
				coordinator.beforeErr = fault
			case "owner":
				coordinator.afterErr = fault
				coordinator.afterStaged = func(owner context.Context) error {
					staged, err := storedRepo.GetByAgentID(owner, f.agent.ID)
					if err != nil {
						return err
					}
					if staged.ID == beforeCredential.ID {
						return errors.New("replacement was not staged")
					}
					return nil
				}
			}
			service := NewCredentialService(agentRepo, repository, f.provider.clientAuth, testSlogger(), coordinator, f.provider.refreshDeps.Revocations)

			result, err := service.Generate(ctx, f.agent.ID)
			require.ErrorIs(t, err, fault)
			if tc.fault == "read" || tc.fault == "agent" {
				require.False(t, errors.Is(err, ports.ErrCredentialAgentNotFound), "infrastructure lookup failure is not a missing-agent result")
			}
			require.True(t, result.Credential == nil && result.PlaintextSecret == "" && !result.Rotated, "failed generation must not return a credential or secret")
			if tc.fault == "owner" {
				require.True(t, coordinator.staged)
			}
			afterCredential, credentialErr := storedRepo.GetByAgentID(ctx, f.agent.ID)
			if tc.public {
				require.True(t, ports.IsNotFoundErr(credentialErr))
				require.Nil(t, afterCredential)
			} else {
				require.NoError(t, credentialErr)
				require.Equal(t, beforeCredential, afterCredential)
			}
			afterRoot, err := f.provider.refreshDeps.Sessions.FindByID(ctx, beforeRoot.ID)
			require.NoError(t, err)
			require.Equal(t, beforeRoot, afterRoot)
			afterToken, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRoot.CurrentSignature)
			require.NoError(t, err)
			require.Equal(t, beforeToken, afterToken)
		})
	}
}

func TestCredentialService_UnknownCommitOutcomeNeverReturnsSecret(t *testing.T) {
	f := newPublicCredentialLifecycleFixture(t)
	ctx := context.Background()
	before := f.root
	coordinator := &credentialLifecycleFaultCoordinator{
		AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator,
		afterCommitErr:                  fmt.Errorf("%w: acknowledgement lost", ports.ErrCommitIndeterminate),
	}
	service := NewCredentialService(
		f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth, testSlogger(),
		coordinator, f.provider.refreshDeps.Revocations,
	)
	result, err := service.Generate(ctx, f.agent.ID)
	require.ErrorIs(t, err, ports.ErrCommitIndeterminate)
	require.Equal(t, ports.CredentialGenerationResult{}, result)
	committed, err := f.provider.fositeStorage.credRepo.GetByAgentID(ctx, f.agent.ID)
	require.NoError(t, err)
	require.Equal(t, coordinator.decisionAt.UTC(), committed.CreatedAt)
	root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before, root)
}

func TestCredentialService_RevokeFaultsKeepCredentialAndRoots(t *testing.T) {
	for _, tc := range []struct{ name, fault string }{
		{name: "credential read fails", fault: "read"},
		{name: "credential removal after staging", fault: "delete"},
		{name: "revocation fails", fault: "revoke"},
		{name: "revocation and receipt stage aborts", fault: "receipt"},
		{name: "coordinator unavailable", fault: "coordinator"},
		{name: "owner abort after staging both records", fault: "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProviderRefreshFixture(t)
			ctx := context.Background()
			storedRepo := f.provider.fositeStorage.credRepo
			beforeCredential, err := storedRepo.GetByAgentID(ctx, f.agent.ID)
			require.NoError(t, err)
			beforeRoot := f.root
			beforeToken, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRoot.CurrentSignature)
			require.NoError(t, err)
			fault := errors.New("credential revocation dependency failed")
			repository := &credentialLifecycleFaultRepository{ClientCredentialRepository: storedRepo}
			revocations := &credentialLifecycleFaultRevocations{
				RefreshSessionRevocationRepository: f.provider.refreshDeps.Revocations,
				rootID:                             beforeRoot.ID, sessions: f.provider.refreshDeps.Sessions,
			}
			coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator}
			switch tc.fault {
			case "read":
				repository.readErr = fault
			case "delete":
				repository.deleteErr = fault
			case "revoke":
				revocations.beforeErr = fault
			case "receipt":
				revocations.afterErr = fault
			case "coordinator":
				coordinator.beforeErr = fault
			case "owner":
				coordinator.afterErr = fault
				coordinator.afterStaged = func(owner context.Context) error {
					_, err := storedRepo.GetByAgentID(owner, f.agent.ID)
					if !ports.IsNotFoundErr(err) {
						return fmt.Errorf("credential removal was not staged: %v", err)
					}
					root, err := f.provider.refreshDeps.Sessions.FindByID(owner, beforeRoot.ID)
					if err != nil {
						return err
					}
					if root.TerminalReason == nil || *root.TerminalReason != storage.RefreshReasonCredentialRevoked {
						return errors.New("refresh revocation was not staged")
					}
					return nil
				}
			}
			service := NewCredentialService(f.agents, repository, f.provider.clientAuth, testSlogger(), coordinator, revocations)

			require.ErrorIs(t, service.Revoke(ctx, f.agent.ID), fault)
			if tc.fault == "receipt" {
				require.True(t, revocations.staged, "native revocation and its receipt must be staged before the injected abort")
			}
			if tc.fault == "owner" {
				require.True(t, coordinator.staged)
			}
			afterCredential, err := storedRepo.GetByAgentID(ctx, f.agent.ID)
			require.NoError(t, err)
			require.Equal(t, beforeCredential, afterCredential)
			afterRoot, err := f.provider.refreshDeps.Sessions.FindByID(ctx, beforeRoot.ID)
			require.NoError(t, err)
			require.Equal(t, beforeRoot, afterRoot)
			afterToken, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRoot.CurrentSignature)
			require.NoError(t, err)
			require.Equal(t, beforeToken, afterToken)
		})
	}
}

func TestCredentialService_CommitConflictRollsBackStagedCredentialAndRevocation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "replacement"
		if revoke {
			name = "explicit revocation"
		}
		t.Run(name, func(t *testing.T) {
			f := newProviderRefreshFixture(t)
			ctx := context.Background()
			storedRepo := f.provider.fositeStorage.credRepo
			beforeCredential, err := storedRepo.GetByAgentID(ctx, f.agent.ID)
			require.NoError(t, err)
			beforeRoot := f.root
			beforeToken, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRoot.CurrentSignature)
			require.NoError(t, err)
			coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator}
			coordinator.afterStaged = func(owner context.Context) error {
				credential, err := storedRepo.GetByAgentID(owner, f.agent.ID)
				if revoke {
					if !ports.IsNotFoundErr(err) {
						return fmt.Errorf("credential removal was not staged: %v", err)
					}
					root, err := f.provider.refreshDeps.Sessions.FindByID(owner, beforeRoot.ID)
					if err != nil {
						return err
					}
					if root.TerminalReason == nil || *root.TerminalReason != storage.RefreshReasonCredentialRevoked {
						return errors.New("refresh revocation was not staged")
					}
				} else if err != nil || credential.ID == beforeCredential.ID {
					return fmt.Errorf("credential replacement was not staged: %v", err)
				}
				// A concurrent no-op agent update changes only the backend version.
				// The real native Commit must reject this scope after both writes have staged.
				committedAgent, err := f.agents.Get(ctx, f.agent.ID)
				if err != nil {
					return err
				}
				return f.agents.Update(ctx, committedAgent)
			}
			service := NewCredentialService(
				f.agents, storedRepo, f.provider.clientAuth, testSlogger(), coordinator, f.provider.refreshDeps.Revocations,
			)
			if revoke {
				err = service.Revoke(ctx, f.agent.ID)
			} else {
				result, generationErr := service.Generate(ctx, f.agent.ID)
				err = generationErr
				require.True(t, result.Credential == nil && result.PlaintextSecret == "" && !result.Rotated, "commit failure must not return a credential or secret")
			}
			require.ErrorContains(t, err, "changed during transaction")
			require.True(t, coordinator.staged)
			credential, err := storedRepo.GetByAgentID(ctx, f.agent.ID)
			require.NoError(t, err)
			require.Equal(t, beforeCredential, credential)
			root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, beforeRoot.ID)
			require.NoError(t, err)
			require.Equal(t, beforeRoot, root)
			token, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRoot.CurrentSignature)
			require.NoError(t, err)
			require.Equal(t, beforeToken, token)
		})
	}
}
