package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func newMemoryBridgeFixture(t *testing.T) (*RefreshSessionStore, id.AgentID, id.AgentID) {
	t.Helper()
	agents := NewAgentRepository()
	store := NewRefreshSessionStore(agents, NewUserGrantRepository(), NewClientCredentialStore(), NewAuthorizationCodeStore(), NewPKCESessionStore())
	owner, other := id.NewAgentID(), id.NewAgentID()
	for _, agentID := range []id.AgentID{owner, other} {
		require.NoError(t, agents.Create(context.Background(), &storage.Agent{
			ID: agentID, DisplayName: "Refresh agent", Description: "Bridge fixture",
			PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}},
		}))
	}
	return store, owner, other
}

func seedMemoryLegacyBridge(t *testing.T, store *RefreshSessionStore, agentID id.AgentID, principal id.Principal, requestID string, expiresAt time.Time) *legacyRefreshToken {
	t.Helper()
	digest := sha256.Sum256([]byte(id.NewAuthorizationCodeID().String()))
	row := &legacyRefreshToken{
		Signature: hex.EncodeToString(digest[:]), RequestID: requestID,
		AgentID: agentID, ClientID: id.ClientID(agentID.String()), Principal: principal,
		Scope: "offline_access read", CreatedAt: expiresAt.Add(-time.Hour), ExpiresAt: expiresAt,
	}
	store.mu.Lock()
	store.legacy[row.Signature] = row
	store.mu.Unlock()
	return row
}

func createMemoryBridgeRoot(t *testing.T, store *RefreshSessionStore, agentID id.AgentID, principal id.Principal, startedAt time.Time) (*storage.RefreshSession, *storage.RefreshToken) {
	t.Helper()
	ctx := context.Background()
	grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: principal, CreatedAt: startedAt, UpdatedAt: startedAt}
	require.NoError(t, store.grants.Create(ctx, grant))
	rootID := id.NewRefreshSessionID()
	digest := sha256.Sum256([]byte(rootID.String()))
	signature := hex.EncodeToString(digest[:])
	expiresAt := startedAt.Add(time.Hour)
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: grant.ID, OriginalTokenSignature: signature,
		AgentID: agentID, Principal: principal, ClientID: id.ClientID(agentID.String()), Scope: "offline_access read",
		StartedAt: startedAt, LastFreshAt: startedAt, InactivityExpiresAt: expiresAt, RetainUntil: expiresAt,
		BranchKeyID: "refresh_" + rootID.String() + "_branch_key", CurrentSignature: signature,
	}
	token := &storage.RefreshToken{Signature: signature, SessionID: rootID, IssuedAt: startedAt, ExpiresAt: expiresAt}
	require.NoError(t, store.Run(ctx, agentID, func(scoped context.Context, _ time.Time) error {
		if err := store.Create(scoped, root); err != nil {
			return err
		}
		return NewRefreshTokenStore(store).Create(scoped, token)
	}))
	return root, token
}

func TestMemoryRefreshBridgeRootlessLifecycleInvalidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		agentWide bool
		reason    storage.RefreshRevocationReason
	}{
		{name: "grant deletion", reason: storage.RefreshReasonGrantDeleted},
		{name: "expired grant renewal", reason: storage.RefreshReasonExpiredGrantRenewal},
		{name: "credential revocation", agentWide: true, reason: storage.RefreshReasonCredentialRevoked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, owner, other := newMemoryBridgeFixture(t)
			principal := id.Principal("owner@example.test")
			expiresAt := time.Now().UTC().Add(time.Hour)
			rows := []*legacyRefreshToken{
				seedMemoryLegacyBridge(t, store, owner, principal, "owner-original", expiresAt),
				seedMemoryLegacyBridge(t, store, owner, id.Principal("other@example.test"), "other-principal", expiresAt),
				seedMemoryLegacyBridge(t, store, other, principal, "other-agent", expiresAt),
			}
			var decision time.Time
			require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
				decision = at
				if tc.agentWide {
					require.NoError(t, store.RevokeByAgent(scoped, owner, at, tc.reason))
				} else {
					require.NoError(t, store.RevokeByPrincipalAndAgent(scoped, principal, owner, at, tc.reason))
				}
				staged := scopeFor(scoped, store).legacySession(rows[0].Signature)
				require.NotNil(t, staged.UsedAt)
				require.True(t, staged.UsedAt.Equal(at))
				require.Nil(t, store.legacy[rows[0].Signature].UsedAt, "uncommitted revocation must not leak")
				return nil
			}))
			for i, row := range rows {
				stored := store.legacy[row.Signature]
				if i == 0 || i == 1 && tc.agentWide {
					require.NotNil(t, stored.UsedAt, "matched old-only authority must be invalidated")
					require.True(t, stored.UsedAt.Equal(decision))
				} else {
					require.Nil(t, stored.UsedAt, "unrelated authority must survive")
				}
			}
			require.Empty(t, store.roots, "rootless legacy entries must not fabricate native origin")
			require.Empty(t, store.tokens)
		})
	}
}

func TestMemoryRefreshBridgeRollbackKeepsRootlessAuthority(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	row := seedMemoryLegacyBridge(t, store, owner, id.Principal("owner@example.test"), "rootless-rollback", time.Now().UTC().Add(time.Hour))
	aborted := errors.New("lifecycle failed after revocation")
	err := store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		require.NoError(t, store.RevokeByAgent(scoped, owner, at, storage.RefreshReasonCredentialRevoked))
		require.NotNil(t, scopeFor(scoped, store).legacySession(row.Signature).UsedAt)
		require.Nil(t, store.legacy[row.Signature].UsedAt, "staged state is invisible to other requests")
		return aborted
	})
	require.ErrorIs(t, err, aborted)
	require.Nil(t, store.legacy[row.Signature].UsedAt)
	remaining, err := store.HasRemainingAuthority(ctx)
	require.NoError(t, err)
	require.True(t, remaining)
}

func TestMemoryRefreshBridgeConsumedMirrorCannotRotate(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, token := createMemoryBridgeRoot(t, store, owner, id.Principal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	tokens := NewRefreshTokenStore(store)
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, _ time.Time) error {
		return tokens.CheckCurrentLineage(scoped, root.ID)
	}))
	consumed := time.Now().UTC()
	legacy := copyLegacy(store.legacy[token.Signature])
	legacy.UsedAt = &consumed // An older writer used the mirror, not the native token.
	store.legacy[token.Signature] = legacy
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		found, err := tokens.FindBySignature(scoped, token.Signature)
		require.NoError(t, err, "native reads must remain usable by cleanup after old-first consumption")
		require.Nil(t, found.UsedAt)
		err = tokens.CheckCurrentLineage(scoped, root.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		err = tokens.MarkUsed(scoped, token.Signature, at)
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		return nil
	}))
	committed, err := tokens.FindBySignature(ctx, token.Signature)
	require.NoError(t, err)
	require.Nil(t, committed.UsedAt, "unsupported old-writer lineage must not mutate native authority")
	require.True(t, store.legacy[token.Signature].UsedAt.Equal(consumed))
	require.NoError(t, store.RevokeByID(ctx, root.ID, time.Now().UTC(), storage.RefreshReasonGrantDeleted), "old-first consumption cannot block lifecycle cleanup")
	terminal, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RefreshReasonGrantDeleted, *terminal.TerminalReason)
	require.True(t, store.legacy[token.Signature].UsedAt.Equal(consumed))
}

func TestMemoryRefreshBridgeRetainsDescendantsUntilLastExpiry(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, token := createMemoryBridgeRoot(t, store, owner, id.Principal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	lateExpiry := token.ExpiresAt.Add(time.Hour)
	descendant := seedMemoryLegacyBridge(t, store, owner, root.Principal, root.ID.String(), lateExpiry)
	unrelated := seedMemoryLegacyBridge(t, store, owner, root.Principal, "unrelated-rootless", lateExpiry)
	var decision time.Time
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		decision = at
		return store.RevokeByID(scoped, root.ID, at, storage.RefreshReasonProhibitedReuse)
	}))
	for _, signature := range []string{token.Signature, descendant.Signature} {
		used := store.legacy[signature].UsedAt
		require.NotNil(t, used)
		require.True(t, used.Equal(decision))
	}
	require.Nil(t, store.legacy[unrelated.Signature].UsedAt)
	terminal, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.True(t, terminal.RetainUntil.Equal(lateExpiry))
	receipt := store.receipts[receiptKey{session: root.ID, reason: storage.RefreshReasonProhibitedReuse}]
	require.NotNil(t, receipt)
	require.True(t, receipt.At.Equal(decision))
	count, err := store.DeleteTerminal(ctx, lateExpiry.Add(-time.Nanosecond), 10)
	require.NoError(t, err)
	require.Zero(t, count, "tombstone must remain through the descendant's lifetime")
	count, err = store.DeleteTerminal(ctx, lateExpiry, 10)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NotContains(t, store.legacy, token.Signature)
	require.NotContains(t, store.legacy, descendant.Signature)
	require.Contains(t, store.legacy, unrelated.Signature)
	require.Contains(t, store.receipts, receiptKey{session: root.ID, reason: storage.RefreshReasonProhibitedReuse})
}

func TestMemoryRefreshBridgeRestoreInvalidatesRootlessRows(t *testing.T) {
	ctx := context.Background()
	store, owner, other := newMemoryBridgeFixture(t)
	root, token := createMemoryBridgeRoot(t, store, owner, id.Principal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	previouslyTerminal, _ := createMemoryBridgeRoot(t, store, owner, id.Principal("ended@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	require.NoError(t, store.RevokeByID(ctx, previouslyTerminal.ID, time.Now().UTC(), storage.RefreshReasonGrantDeleted))
	priorReceipt := store.receipts[receiptKey{session: previouslyTerminal.ID, reason: storage.RefreshReasonGrantDeleted}]
	require.NotNil(t, priorReceipt)
	rootless := seedMemoryLegacyBridge(t, store, owner, id.Principal("other@example.test"), "unanchored-owner", token.ExpiresAt)
	unrelated := seedMemoryLegacyBridge(t, store, other, root.Principal, "unanchored-other-agent", token.ExpiresAt)
	ids, err := store.ListAgentIDs(ctx, id.AgentID{}, 10)
	require.NoError(t, err)
	require.ElementsMatch(t, []id.AgentID{owner, other}, ids)
	var decision time.Time
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		decision = at
		return store.RevokeByAgent(scoped, owner, at, storage.RefreshReasonRestoreInvalidation)
	}))
	for _, signature := range []string{token.Signature, rootless.Signature} {
		used := store.legacy[signature].UsedAt
		require.NotNil(t, used)
		require.True(t, used.Equal(decision))
	}
	require.Nil(t, store.legacy[unrelated.Signature].UsedAt)
	terminal, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RefreshReasonRestoreInvalidation, *terminal.TerminalReason)
	prior, err := store.FindByID(ctx, previouslyTerminal.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RefreshReasonGrantDeleted, *prior.TerminalReason, "restore must preserve existing terminal reasons")
	require.Equal(t, priorReceipt, store.receipts[receiptKey{session: previouslyTerminal.ID, reason: storage.RefreshReasonGrantDeleted}])
	require.NoError(t, store.RevokeByAgent(ctx, owner, time.Now().UTC(), storage.RefreshReasonRestoreInvalidation))
	require.True(t, store.legacy[rootless.Signature].UsedAt.Equal(decision), "reruns cannot rewrite consumed history")
	require.NoError(t, store.RevokeByAgent(ctx, other, time.Now().UTC(), storage.RefreshReasonRestoreInvalidation))
	remaining, err := store.HasRemainingAuthority(ctx)
	require.NoError(t, err)
	require.False(t, remaining)
	ids, err = store.ListAgentIDs(ctx, id.AgentID{}, 10)
	require.NoError(t, err)
	require.ElementsMatch(t, []id.AgentID{owner, other}, ids, "used rootless rows remain discoverable for restore scans")
}
