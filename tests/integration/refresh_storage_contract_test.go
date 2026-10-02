package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func refreshContractSignature(label string) string {
	sum := sha256.Sum256([]byte(label))
	return hex.EncodeToString(sum[:])
}

func newMemoryRefreshAdapter(t *testing.T) *storageadapter.Adapter {
	t.Helper()
	adapter, err := storageadapter.NewAdapter(&ports.StorageConfig{Backend: string(storage.BackendMemory)})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close(context.Background())) })
	return adapter
}

func createMemoryRefreshRoot(t *testing.T, adapter *storageadapter.Adapter, agentID id.AgentID, principal id.Principal, now time.Time) (*storage.RefreshSession, *storage.RefreshToken) {
	t.Helper()
	ctx := context.Background()
	client := id.NewClientID("client-" + agentID.String())
	if agentID.IsZero() {
		agentID = id.NewAgentID()
		client = id.NewClientID("client-" + agentID.String())
		require.NoError(t, adapter.Agents().Create(ctx, &storage.Agent{
			ID: agentID, ClientID: &client, DisplayName: "Refresh agent", Description: "Refresh storage contract",
			CreatedAt: now, UpdatedAt: now,
			PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}},
		}))
	}
	grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: principal, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, adapter.UserGrants().Create(ctx, grant))
	rootID := id.NewRefreshSessionID()
	signature := refreshContractSignature(rootID.String())
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: grant.ID, OriginalTokenSignature: signature,
		AgentID: agentID, Principal: principal, ClientID: client, Scope: "offline_access read",
		StartedAt: now, LastFreshAt: now, InactivityExpiresAt: now.Add(time.Hour),
		RetainUntil: now.Add(time.Hour), BranchKeyID: "refresh_" + rootID.String() + "_branch_key",
		CurrentSignature: signature,
	}
	first := &storage.RefreshToken{Signature: signature, SessionID: rootID, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	require.NoError(t, adapter.AuthorizationCoordinator().Run(ctx, agentID, func(scope context.Context, _ time.Time) error {
		if err := adapter.RefreshSessions().Create(scope, root); err != nil {
			return err
		}
		return adapter.RefreshTokens().Create(scope, first)
	}))
	return root, first
}

func rotateContractRoot(ctx context.Context, sessions ports.RefreshSessionRepository, tokens ports.RefreshTokenRepository, rootID id.RefreshSessionID, submitted, successor string, at time.Time) error {
	root, err := sessions.FindByID(ctx, rootID)
	if err != nil {
		return err
	}
	if root.CurrentSignature != submitted {
		return nil
	}
	current, err := tokens.FindBySignature(ctx, submitted)
	if err != nil {
		return err
	}
	if current.UsedAt != nil {
		return nil
	}
	if err := tokens.MarkUsed(ctx, current.Signature, at); err != nil {
		return err
	}
	child := &storage.RefreshToken{Signature: successor, SessionID: rootID, IssuedAt: at, ExpiresAt: at.Add(time.Hour)}
	if err := tokens.Create(ctx, child); err != nil {
		return err
	}
	original := root.CurrentSignature
	requested := ""
	fingerprint := refreshContractSignature("redacted context")
	reuseUntil, accessUntil := at.Add(30*time.Second), at.Add(5*time.Minute)
	root.CurrentSignature = successor
	root.PreviousSignature = &original
	root.PreviousConsumedAt = &at
	root.ReuseUntil = &reuseUntil
	root.OriginalRequestedScope = &requested
	root.OriginalRequestContextFingerprint = &fingerprint
	root.RetryAccessExpiresAt = &accessUntil
	root.RetryExpiresAt = &reuseUntil
	root.RetryCiphertext = []byte{0x93, 0x12, 0x40, 0xff} // Opaque encrypted-result fixture, never a token.
	root.LastFreshAt = at
	root.InactivityExpiresAt = child.ExpiresAt
	root.RetainUntil = child.ExpiresAt
	return sessions.Save(ctx, root)
}

func TestMemoryRefreshCoordinatorRollbackAcrossAuthorizationRecords(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	root, first := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	agent, err := adapter.Agents().Get(ctx, root.AgentID)
	require.NoError(t, err)
	credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: root.AgentID, SecretHash: "old-hash", CreatedAt: now}
	require.NoError(t, adapter.BrokerCredentials().Create(ctx, credential))
	code := &storage.AuthorizationCode{
		ID: id.NewAuthorizationCodeID(), CodeHash: refreshContractSignature("authorization-" + root.ID.String()),
		AgentID: root.AgentID, ClientID: root.ClientID, Principal: root.Principal,
		RedirectURI: "https://client.example.test/callback", CodeChallenge: "S256challenge", Scope: root.Scope,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	require.NoError(t, adapter.AuthorizationCodes().Create(ctx, code))
	pkce := &storage.PKCESession{Signature: code.CodeHash, CodeChallenge: code.CodeChallenge, CodeChallengeMethod: "S256", ExpiresAt: code.ExpiresAt, CreatedAt: now}
	require.NoError(t, adapter.PKCESessions().Create(ctx, pkce))

	abort := errors.New("deny before commit")
	err = adapter.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		agent.DisplayName = "uncommitted agent"
		agent.UpdatedAt = at
		if err := adapter.Agents().Update(scope, agent); err != nil {
			return err
		}
		if err := adapter.UserGrants().Delete(scope, root.OriginalGrantID); err != nil {
			return err
		}
		replacement := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: root.AgentID, SecretHash: "replacement-hash", CreatedAt: at}
		if err := adapter.BrokerCredentials().Rotate(scope, root.AgentID, replacement); err != nil {
			return err
		}
		if err := adapter.AuthorizationCodes().MarkUsed(scope, code.ID); err != nil {
			return err
		}
		if err := adapter.PKCESessions().Delete(scope, pkce.Signature); err != nil {
			return err
		}
		if err := adapter.RefreshRevocations().RevokeByID(scope, root.ID, at, storage.RefreshReasonGrantDeleted); err != nil {
			return err
		}
		if _, err := adapter.UserGrants().Get(scope, root.OriginalGrantID); !ports.IsNotFoundErr(err) {
			return fmt.Errorf("deleted grant must not remain visible inside scope: %w", err)
		}
		changed, err := adapter.RefreshSessions().FindByID(scope, root.ID)
		if err != nil || changed.RevokedAt == nil {
			return fmt.Errorf("scoped revocation must be visible inside scope: %w", err)
		}
		return abort
	})
	require.ErrorIs(t, err, abort)
	unchanged, err := adapter.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, unchanged.RevokedAt)
	require.Equal(t, first.Signature, unchanged.CurrentSignature)
	storedToken, err := adapter.RefreshTokens().FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, storedToken.UsedAt)
	_, err = adapter.UserGrants().Get(ctx, root.OriginalGrantID)
	require.NoError(t, err)
	storedAgent, err := adapter.Agents().Get(ctx, root.AgentID)
	require.NoError(t, err)
	require.Equal(t, "Refresh agent", storedAgent.DisplayName)
	storedCredential, err := adapter.BrokerCredentials().GetByAgentID(ctx, root.AgentID)
	require.NoError(t, err)
	require.Equal(t, credential.ID, storedCredential.ID)
	storedCode, err := adapter.AuthorizationCodes().FindByCodeHash(ctx, code.CodeHash)
	require.NoError(t, err)
	require.Nil(t, storedCode.UsedAt)
	_, err = adapter.PKCESessions().FindBySignature(ctx, pkce.Signature)
	require.NoError(t, err)
}

func TestMemoryRefreshTransactionRollbackIsNotNoOp(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	root, first := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	txCtx, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	require.NoError(t, adapter.UserGrants().Delete(txCtx, root.OriginalGrantID))
	require.NoError(t, adapter.RefreshTokens().MarkUsed(txCtx, first.Signature, now.Add(time.Second)))
	require.NoError(t, adapter.Rollback(txCtx))
	_, err = adapter.UserGrants().Get(ctx, root.OriginalGrantID)
	require.NoError(t, err)
	storedToken, err := adapter.RefreshTokens().FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, storedToken.UsedAt)
}

func TestMemoryRefreshFailedRotationKeepsPredecessorUsable(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	root, first := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	abort := errors.New("candidate access token failed verification")
	successor := refreshContractSignature("discarded-rotation")
	err := adapter.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		if err := rotateContractRoot(scope, adapter.RefreshSessions(), adapter.RefreshTokens(), root.ID, first.Signature, successor, at); err != nil {
			return err
		}
		return abort
	})
	require.ErrorIs(t, err, abort)
	stored, err := adapter.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, first.Signature, stored.CurrentSignature)
	require.Nil(t, stored.PreviousSignature)
	require.True(t, stored.LastFreshAt.Equal(root.LastFreshAt))
	require.True(t, stored.InactivityExpiresAt.Equal(root.InactivityExpiresAt))
	predecessor, err := adapter.RefreshTokens().FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, predecessor.UsedAt)
	_, err = adapter.RefreshTokens().FindBySignature(ctx, successor)
	require.True(t, ports.IsNotFoundErr(err))
}

func TestMemoryRefreshCoordinatorOneSuccessorAndAgentScope(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Second)
	root, first := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	candidates := []string{refreshContractSignature("first contender"), refreshContractSignature("second contender")}
	start := make(chan struct{})
	results := make(chan error, len(candidates))
	var wg sync.WaitGroup
	for _, signature := range candidates {
		wg.Add(1)
		go func(signature string) {
			defer wg.Done()
			<-start
			results <- adapter.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
				return rotateContractRoot(scope, adapter.RefreshSessions(), adapter.RefreshTokens(), root.ID, first.Signature, signature, at)
			})
		}(signature)
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	storedRoot, err := adapter.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.NotNil(t, storedRoot.PreviousSignature)
	require.Equal(t, first.Signature, *storedRoot.PreviousSignature)
	previous, err := adapter.RefreshTokens().FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.NotNil(t, previous.UsedAt)
	created := 0
	for _, signature := range candidates {
		child, err := adapter.RefreshTokens().FindBySignature(ctx, signature)
		if ports.IsNotFoundErr(err) {
			continue
		}
		require.NoError(t, err)
		require.Equal(t, root.ID, child.SessionID)
		require.Nil(t, child.UsedAt)
		require.Equal(t, signature, storedRoot.CurrentSignature)
		created++
	}
	require.Equal(t, 1, created, "concurrent presentations cannot fork one lineage")
}

func TestMemoryRefreshRollbackCannotOverwriteOtherAgentCommit(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Second)
	owner, _ := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	other, _ := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	abort := errors.New("owner scope denied")
	staged := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- adapter.AuthorizationCoordinator().Run(ctx, owner.AgentID, func(scope context.Context, at time.Time) error {
			if err := adapter.RefreshRevocations().RevokeByAgent(scope, owner.AgentID, at, storage.RefreshReasonCredentialRevoked); err != nil {
				return err
			}
			close(staged)
			select {
			case <-release:
				return abort
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-staged:
	case err := <-first:
		require.NoError(t, err, "owner scope must stage revocation before completing")
		t.Fatal("owner scope did not stage its write")
	case <-ctx.Done():
		t.Fatal("owner scope did not stage its write")
	}
	second := make(chan error, 1)
	go func() {
		second <- adapter.AuthorizationCoordinator().Run(ctx, other.AgentID, func(scope context.Context, at time.Time) error {
			return adapter.RefreshRevocations().RevokeByAgent(scope, other.AgentID, at, storage.RefreshReasonCredentialRevoked)
		})
	}()
	select {
	case err := <-second:
		require.NoError(t, err, "independent agent must commit while owner scope is still staged")
	case <-ctx.Done():
		close(release)
		t.Fatal("one agent's staged write must not block another agent's commit")
	}
	close(release)
	require.ErrorIs(t, <-first, abort)
	rolledBack, err := adapter.RefreshSessions().FindByID(context.Background(), owner.ID)
	require.NoError(t, err)
	require.Nil(t, rolledBack.TerminalReason)
	committed, err := adapter.RefreshSessions().FindByID(context.Background(), other.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RefreshReasonCredentialRevoked, *committed.TerminalReason,
		"rolling back one agent must never restore an old snapshot of another agent")
}

func TestMemoryRefreshClockRunsAfterAgentGate(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Second)
	root, _ := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- adapter.AuthorizationCoordinator().Run(ctx, root.AgentID, func(context.Context, time.Time) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case err := <-first:
		require.NoError(t, err, "the first scoped callback must execute")
		t.Fatal("the first scoped callback never acquired the agent gate")
	case <-ctx.Done():
		t.Fatal("the first scoped callback did not acquire the agent gate")
	}
	second := make(chan time.Time, 1)
	secondErr := make(chan error, 1)
	go func() {
		secondErr <- adapter.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
			second <- at
			clockAt, err := adapter.AuthorizationClock().Now(scope)
			if err != nil {
				return err
			}
			if clockAt.Before(at) {
				return fmt.Errorf("coordinator time is ahead of shared clock")
			}
			return nil
		})
	}()
	select {
	case <-second:
		close(release)
		t.Fatal("the second callback entered before the agent gate was released")
	default:
	}
	releasedAt := time.Now().UTC()
	close(release)
	require.NoError(t, <-first)
	require.NoError(t, <-secondErr)
	select {
	case decisionAt := <-second:
		require.False(t, decisionAt.Before(releasedAt), "decision time must be sampled after acquiring the agent gate")
	case <-ctx.Done():
		t.Fatal("second callback did not run after the agent gate was released")
	}
}

func TestMemoryRefreshRevocationIsolationAndRetention(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	owner, token := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	neighbor, _ := createMemoryRefreshRoot(t, adapter, owner.AgentID, id.NewPrincipal("neighbor@example.test"), now)
	otherAgent, _ := createMemoryRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"), now)
	require.NoError(t, adapter.AuthorizationCoordinator().Run(ctx, owner.AgentID, func(scope context.Context, at time.Time) error {
		return adapter.RefreshRevocations().RevokeByPrincipalAndAgent(scope, owner.Principal, owner.AgentID, at, storage.RefreshReasonGrantDeleted)
	}))
	stopped, err := adapter.RefreshSessions().FindByID(ctx, owner.ID)
	require.NoError(t, err)
	require.NotNil(t, stopped.RevokedAt)
	require.Equal(t, storage.RefreshReasonGrantDeleted, *stopped.TerminalReason)
	for _, activeID := range []id.RefreshSessionID{neighbor.ID, otherAgent.ID} {
		active, err := adapter.RefreshSessions().FindByID(ctx, activeID)
		require.NoError(t, err)
		require.Nil(t, active.TerminalReason, "different principal or agent must keep its refresh authority")
	}
	count, err := adapter.RefreshMaintenance().DeleteTerminal(ctx, owner.RetainUntil.Add(-time.Nanosecond), 10)
	require.NoError(t, err)
	require.Zero(t, count)
	_, err = adapter.RefreshTokens().FindBySignature(ctx, token.Signature)
	require.NoError(t, err, "terminal history must survive until its issued token expires")
	count, err = adapter.RefreshMaintenance().DeleteTerminal(ctx, owner.RetainUntil, 10)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, err = adapter.RefreshSessions().FindByID(ctx, owner.ID)
	require.True(t, ports.IsNotFoundErr(err))
	_, err = adapter.RefreshTokens().FindBySignature(ctx, token.Signature)
	require.True(t, ports.IsNotFoundErr(err))
	for _, activeID := range []id.RefreshSessionID{neighbor.ID, otherAgent.ID} {
		active, err := adapter.RefreshSessions().FindByID(ctx, activeID)
		require.NoError(t, err)
		require.Nil(t, active.TerminalReason)
	}
}

func TestMemoryRefreshCoordinatorCannotRecreateMissingAgent(t *testing.T) {
	adapter := newMemoryRefreshAdapter(t)
	ctx := context.Background()
	agentID := id.NewAgentID()
	err := adapter.AuthorizationCoordinator().Run(ctx, agentID, func(scope context.Context, at time.Time) error {
		return adapter.Agents().Create(scope, &storage.Agent{
			ID: agentID, DisplayName: "Missing agent", Description: "Must not recreate authority",
			CreatedAt: at, UpdatedAt: at,
			PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}},
		})
	})
	require.True(t, ports.IsNotFoundErr(err), "a missing agent cannot acquire an authorization scope")
	_, err = adapter.Agents().Get(ctx, agentID)
	require.True(t, ports.IsNotFoundErr(err), "the denied callback must not recreate the agent")
}
