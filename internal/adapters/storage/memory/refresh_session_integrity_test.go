package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func requireRefreshStorageError(t *testing.T, err error, operation string, kind storage.ErrorKind) {
	t.Helper()
	var classified *storage.StorageError
	require.ErrorAs(t, err, &classified)
	require.Equal(t, operation, classified.Operation)
	require.Equal(t, kind, classified.Kind)
	require.Error(t, classified.Cause, "storage validation must preserve its underlying cause")
}

func memoryIntegritySignature(label string) string {
	digest := sha256.Sum256([]byte(label))
	return hex.EncodeToString(digest[:])
}

func rotateMemoryIntegrityRoot(t *testing.T, store *RefreshSessionStore, rootID id.RefreshSessionID, successor string) {
	t.Helper()
	tokens := NewRefreshTokenStore(store)
	root, err := store.FindByID(context.Background(), rootID)
	require.NoError(t, err)
	require.NoError(t, store.Run(context.Background(), root.AgentID, func(ctx context.Context, at time.Time) error {
		original, err := store.FindByID(ctx, rootID)
		if err != nil {
			return err
		}
		if err := tokens.MarkUsed(ctx, original.CurrentSignature, at); err != nil {
			return err
		}
		child := &storage.RefreshToken{Signature: successor, SessionID: rootID, IssuedAt: at, ExpiresAt: at.Add(time.Hour)}
		if err := tokens.Create(ctx, child); err != nil {
			return err
		}
		previous := original.CurrentSignature
		requested := ""
		fingerprint := memoryIntegritySignature("trusted request context")
		reuseUntil := at.Add(30 * time.Second)
		accessUntil := at.Add(5 * time.Minute)
		original.CurrentSignature = successor
		original.PreviousSignature = &previous
		original.PreviousConsumedAt = &at
		original.ReuseUntil = &reuseUntil
		original.OriginalRequestedScope = &requested
		original.OriginalRequestContextFingerprint = &fingerprint
		original.RetryAccessExpiresAt = &accessUntil
		original.RetryExpiresAt = &reuseUntil
		original.RetryCiphertext = []byte{0x91, 0x67, 0x23}
		original.LastFreshAt = at
		original.InactivityExpiresAt = child.ExpiresAt
		original.RetainUntil = child.ExpiresAt
		return store.Save(ctx, original)
	}))
}

func TestMemoryRefreshNativeErrorsClassifiedAndAtomic(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, first := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	tokens := NewRefreshTokenStore(store)

	err := store.Create(ctx, root)
	requireRefreshStorageError(t, err, "RefreshSession.Create", storage.ErrorKindValidation)

	invalid := copyRefreshSession(root)
	invalid.BranchKeyID = "not-the-original-key"
	err = store.Save(ctx, invalid)
	requireRefreshStorageError(t, err, "RefreshSession.Save", storage.ErrorKindValidation)

	badToken := copyRefreshToken(first)
	badToken.Signature = "not-a-sha256-signature"
	err = tokens.Create(ctx, badToken)
	requireRefreshStorageError(t, err, "RefreshToken.Create", storage.ErrorKindValidation)

	err = tokens.CheckCurrentLineage(ctx, root.ID)
	requireRefreshStorageError(t, err, "AuthorizationSession", storage.ErrorKindValidation)
	tx, err := store.BeginTX(ctx)
	require.NoError(t, err)
	err = tokens.CheckCurrentLineage(tx, root.ID)
	requireRefreshStorageError(t, err, "AuthorizationSession", storage.ErrorKindValidation)
	require.NoError(t, store.Rollback(tx))

	err = tokens.MarkUsed(ctx, first.Signature, first.IssuedAt.Add(-time.Second))
	requireRefreshStorageError(t, err, "RefreshToken.MarkUsed", storage.ErrorKindValidation)

	for _, list := range []struct {
		name string
		call func(int) error
	}{
		{"agent IDs", func(limit int) error { _, err := store.ListAgentIDs(ctx, id.AgentID{}, limit); return err }},
		{"active roots", func(limit int) error { _, err := store.ListActive(ctx, id.RefreshSessionID{}, limit); return err }},
		{"due roots", func(limit int) error { _, err := store.ListDue(ctx, time.Now().UTC(), limit); return err }},
		{"terminal roots", func(limit int) error { _, err := store.DeleteTerminal(ctx, time.Now().UTC(), limit); return err }},
	} {
		t.Run(list.name, func(t *testing.T) {
			for _, limit := range []int{0, 1001} {
				requireRefreshStorageError(t, list.call(limit), "RefreshSession.List", storage.ErrorKindValidation)
			}
		})
	}

	got, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, root, got)
	gotToken, err := tokens.FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Equal(t, first, gotToken)
	require.Nil(t, store.legacy[first.Signature].UsedAt)
	require.Len(t, store.tokensByRoot[root.ID], 1)
}

func TestMemoryRefreshCommitClassificationAndCallbackIdentity(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, first := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	tokens := NewRefreshTokenStore(store)

	callbackFailure := errors.New("delegation unavailable")
	err := store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		if err := tokens.MarkUsed(scoped, first.Signature, at); err != nil {
			return err
		}
		return callbackFailure
	})
	require.Equal(t, callbackFailure, err, "a domain/protocol callback error must not be reclassified")

	err = store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		return tokens.MarkUsed(scoped, first.Signature, at)
	})
	requireRefreshStorageError(t, err, "AuthorizationSession.Run", storage.ErrorKindValidation)
	got, err := tokens.FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, got.UsedAt)
	gotRoot, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, root, gotRoot)

	err = store.Run(ctx, id.AgentID{}, func(context.Context, time.Time) error { return nil })
	requireRefreshStorageError(t, err, "AuthorizationSession.Run", storage.ErrorKindValidation)
}

func TestMemoryRefreshCompleteIntermediateLineage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		corrupt func(*RefreshSessionStore, string)
	}{
		{"missing intermediate native token", func(s *RefreshSessionStore, sig string) {
			rootID := s.tokens[sig].SessionID
			delete(s.tokens, sig)
			delete(s.tokensByRoot[rootID], sig)
		}},
		{"missing intermediate private mirror", func(s *RefreshSessionStore, sig string) { delete(s.legacy, sig) }},
		{"different intermediate mirror owner", func(s *RefreshSessionStore, sig string) {
			s.legacy[sig].Principal = id.NewPrincipal("intruder@example.test")
		}},
		{"different intermediate mirror profile", func(s *RefreshSessionStore, sig string) {
			s.legacy[sig].DisplayName = "impostor"
		}},
		{"inconsistent intermediate consumption", func(s *RefreshSessionStore, sig string) {
			when := s.legacy[sig].UsedAt.Add(time.Second)
			s.legacy[sig].UsedAt = &when
		}},
		{"inconsistent intermediate issuance", func(s *RefreshSessionStore, sig string) {
			s.legacy[sig].CreatedAt = s.legacy[sig].CreatedAt.Add(time.Second)
		}},
		{"wrong intermediate origin", func(s *RefreshSessionStore, sig string) { s.legacy[sig].RequestID = id.NewRefreshSessionID().String() }},
		{"missing older predecessor link", func(s *RefreshSessionStore, sig string) { s.legacy[sig].PredecessorSignature = nil }},
		{"skipped older predecessor link", func(s *RefreshSessionStore, sig string) {
			root := s.roots[s.tokens[sig].SessionID]
			first := root.OriginalTokenSignature
			s.legacy[*root.PreviousSignature].PredecessorSignature = &first
		}},
		{"old-only detached descendant", func(s *RefreshSessionStore, sig string) {
			oldOnly := copyLegacy(s.legacy[sig])
			oldOnly.Signature = memoryIntegritySignature("old-only-" + sig)
			oldOnly.PredecessorSignature = &sig
			oldOnly.UsedAt = nil
			s.legacy[oldOnly.Signature] = oldOnly
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, owner, _ := newMemoryBridgeFixture(t)
			root, _ := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
			intermediate := memoryIntegritySignature("intermediate-" + root.ID.String())
			second := memoryIntegritySignature("second-" + root.ID.String())
			current := memoryIntegritySignature("current-" + root.ID.String())
			for _, sig := range []string{intermediate, second, current} {
				rotateMemoryIntegrityRoot(t, store, root.ID, sig)
			}
			tokens := NewRefreshTokenStore(store)
			require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, _ time.Time) error {
				return tokens.CheckCurrentLineage(scoped, root.ID)
			}), "an intact four-token history must remain eligible")
			before, err := store.FindByID(ctx, root.ID)
			require.NoError(t, err)
			beforeCurrent, err := tokens.FindBySignature(ctx, current)
			require.NoError(t, err)
			tc.corrupt(store, intermediate)
			err = store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
				if err := tokens.CheckCurrentLineage(scoped, root.ID); err != nil {
					return err
				}
				return tokens.MarkUsed(scoped, current, at)
			})
			require.True(t, ports.IsNotFoundErr(err), "unanchored intermediate evidence cannot return fresh or cached authority: %v", err)
			after, err := store.FindByID(ctx, root.ID)
			require.NoError(t, err)
			require.Equal(t, before, after, "corruption rejection cannot mutate root clocks, terminal state, or retry count")
			afterCurrent, err := tokens.FindBySignature(ctx, current)
			require.NoError(t, err)
			require.Equal(t, beforeCurrent, afterCurrent, "corruption rejection cannot consume current token")
			require.Nil(t, store.legacy[current].UsedAt, "corruption rejection cannot consume private mirror")
		})
	}
}

func TestMemoryRefreshRevocationReceiptRollbackAndTrustedContext(t *testing.T) {
	store, owner, _ := newMemoryBridgeFixture(t)
	root, first := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(security.TransportCapture{
		ClientIP: "192.0.2.4", UserAgent: "refresh-client", RequestTarget: "/oauth2/token?refresh_token=not-audit-data",
	}, "owner@example.test", ""))
	failure := errors.New("authorization change rolled back")
	key := receiptKey{session: root.ID, reason: storage.RefreshReasonGrantDeleted}
	err := store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		if err := store.RevokeByID(scoped, root.ID, at, storage.RefreshReasonGrantDeleted); err != nil {
			return err
		}
		require.NotNil(t, scopeFor(scoped, store).receipts[key], "receipt must be staged before rollback")
		require.NotNil(t, scopeFor(scoped, store).legacy[first.Signature].UsedAt, "legacy mirror must be fenced inside the same staged transition")
		staged, lookupErr := store.FindByID(scoped, root.ID)
		require.NoError(t, lookupErr)
		require.NotNil(t, staged.RevokedAt)
		return failure
	})
	require.ErrorIs(t, err, failure)
	require.NotContains(t, store.receipts, key, "rollback must not publish a successful transition receipt")
	active, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, active.TerminalReason, "confirmed rollback must leave native refresh authority active")
	firstAfter, err := NewRefreshTokenStore(store).FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, firstAfter.UsedAt)
	require.Nil(t, store.legacy[first.Signature].UsedAt, "confirmed rollback must leave legacy mirror usable")
	require.NoError(t, store.RevokeByID(ctx, root.ID, time.Now().UTC(), storage.RefreshReasonGrantDeleted))
	receipt := store.receipts[key]
	require.NotNil(t, receipt)
	require.Equal(t, map[string]string{"origin": "request", "client_ip": "192.0.2.4", "user_agent": "refresh-client"}, receipt.RedactedContext)
	require.Equal(t, root.Principal, receipt.Principal)
	require.Equal(t, root.AgentID, receipt.AgentID)
	require.Equal(t, root.ClientID, receipt.ClientID)
	require.NoError(t, store.RevokeByID(ctx, root.ID, time.Now().UTC(), storage.RefreshReasonCredentialRevoked))
	require.Len(t, store.receipts, 1, "already-terminal roots must not emit another transition receipt")
}

func TestMemoryRefreshCommitRejectsCorruptOlderMirrorWithoutPublishingRetry(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, _ := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	intermediate := memoryIntegritySignature("older-" + root.ID.String())
	rotateMemoryIntegrityRoot(t, store, root.ID, intermediate)
	rotateMemoryIntegrityRoot(t, store, root.ID, memoryIntegritySignature("recent-"+root.ID.String()))
	rotateMemoryIntegrityRoot(t, store, root.ID, memoryIntegritySignature("latest-"+root.ID.String()))
	before, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	store.legacy[intermediate].Scope = "offline_access"
	err = store.Run(ctx, owner, func(scoped context.Context, _ time.Time) error {
		changed, lookupErr := store.FindByID(scoped, root.ID)
		if lookupErr != nil {
			return lookupErr
		}
		changed.RetryCount++
		return store.Save(scoped, changed)
	})
	requireRefreshStorageError(t, err, "AuthorizationSession.Run", storage.ErrorKindValidation)
	after, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "failed commit cannot publish a stored-result authorization")
}

func TestMemoryRefreshListActiveByAgentUsesOwnerScopeAndStagedRoots(t *testing.T) {
	ctx := context.Background()
	store, owner, other := newMemoryBridgeFixture(t)
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	first, _ := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("first@example.test"), now)
	second, _ := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("second@example.test"), now)
	createMemoryBridgeRoot(t, store, other, id.NewPrincipal("other@example.test"), now)
	_, err := store.ListActiveByAgent(ctx, owner, nil)
	requireRefreshStorageError(t, err, "AuthorizationSession", storage.ErrorKindValidation)
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		all, listErr := store.ListActiveByAgent(scoped, owner, nil)
		require.NoError(t, listErr)
		require.Len(t, all, 2)
		require.True(t, bytes.Compare(all[0].ID[:], all[1].ID[:]) < 0)
		principal := first.Principal
		one, listErr := store.ListActiveByAgent(scoped, owner, &principal)
		require.NoError(t, listErr)
		require.Len(t, one, 1)
		require.Equal(t, first.ID, one[0].ID)
		require.Equal(t, first.AgentID, one[0].AgentID)
		require.Equal(t, first.Principal, one[0].Principal)
		require.Equal(t, first.ClientID, one[0].ClientID)
		_, listErr = store.ListActiveByAgent(scoped, other, nil)
		requireRefreshStorageError(t, listErr, "AuthorizationSession", storage.ErrorKindValidation)
		_, listErr = store.ListActiveByAgent(scoped, id.AgentID{}, nil)
		requireRefreshStorageError(t, listErr, "AuthorizationSession", storage.ErrorKindValidation)
		require.NoError(t, store.RevokeByID(scoped, first.ID, at, storage.RefreshReasonGrantDeleted))
		remaining, listErr := store.ListActiveByAgent(scoped, owner, nil)
		require.NoError(t, listErr)
		require.Len(t, remaining, 1)
		require.Equal(t, second.ID, remaining[0].ID)
		return nil
	}))
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, _ time.Time) error {
		remaining, listErr := store.ListActiveByAgent(scoped, owner, nil)
		require.NoError(t, listErr)
		require.Len(t, remaining, 1)
		require.Equal(t, second.ID, remaining[0].ID)
		return nil
	}))
}

func TestMemoryRefreshTerminalSaveWritesMaintenanceReceiptAndInvalidatesMirror(t *testing.T) {
	ctx := security.WithMaintenanceAuditOrigin(context.Background())
	store, owner, _ := newMemoryBridgeFixture(t)
	root, first := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		changed, err := store.FindByID(scoped, root.ID)
		if err != nil {
			return err
		}
		reason := storage.RefreshReasonRestoreInvalidation
		changed.TerminalReason = &reason
		changed.RevokedAt = &at
		return store.Save(scoped, changed)
	}))
	receipt := store.receipts[receiptKey{session: root.ID, reason: storage.RefreshReasonRestoreInvalidation}]
	require.NotNil(t, receipt)
	require.Equal(t, map[string]string{"origin": "maintenance"}, receipt.RedactedContext)
	require.NotNil(t, store.legacy[first.Signature].UsedAt, "maintenance must fence the old-writer mirror in the same commit")
	terminal, err := store.FindByID(context.Background(), root.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RefreshReasonRestoreInvalidation, *terminal.TerminalReason)
}

func TestMemoryRefreshStaleCommitIsTypedAndCannotRestoreAuthority(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, _ := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	stale, err := store.BeginTX(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Save(stale, root))
	require.NoError(t, store.Run(ctx, owner, func(scoped context.Context, at time.Time) error {
		return store.RevokeByID(scoped, root.ID, at, storage.RefreshReasonGrantDeleted)
	}))
	committed, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	err = store.Commit(stale)
	requireRefreshStorageError(t, err, "AuthorizationSession.Run", storage.ErrorKindConflict)
	after, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, committed, after, "stale commit must not resurrect revoked authority")
}

func TestMemoryRefreshLineageCannotBeRepairedAfterCorruption(t *testing.T) {
	ctx := context.Background()
	store, owner, _ := newMemoryBridgeFixture(t)
	root, _ := createMemoryBridgeRoot(t, store, owner, id.NewPrincipal("owner@example.test"), time.Now().UTC().Add(-time.Minute).Truncate(time.Second))
	firstChild := memoryIntegritySignature("poisoned-ancestor-" + root.ID.String())
	rotateMemoryIntegrityRoot(t, store, root.ID, firstChild)
	rotateMemoryIntegrityRoot(t, store, root.ID, memoryIntegritySignature("current-"+root.ID.String()))
	original := store.legacy[firstChild].RequestID
	store.legacy[firstChild].RequestID = id.NewRefreshSessionID().String()
	tokens := NewRefreshTokenStore(store)
	err := store.Run(ctx, owner, func(scoped context.Context, _ time.Time) error {
		return tokens.CheckCurrentLineage(scoped, root.ID)
	})
	require.True(t, ports.IsNotFoundErr(err), "unsupported lineage must not authorize current token")
	store.legacy[firstChild].RequestID = original
	err = store.Run(ctx, owner, func(scoped context.Context, _ time.Time) error {
		return tokens.CheckCurrentLineage(scoped, root.ID)
	})
	require.True(t, ports.IsNotFoundErr(err), "repairing a broken ancestor cannot recreate refresh authority")
	committed, err := store.FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, committed.TerminalReason, "poisoning proof does not turn rejection into a revocation")
}

func TestMemoryAuthorizeCodeCreationWaitsForAgentCoordinator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	store, owner, _ := newMemoryBridgeFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	ownerResult := make(chan error, 1)
	go func() {
		ownerResult <- store.Run(ctx, owner, func(context.Context, time.Time) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-entered
	now := time.Now().UTC()
	code := &storage.AuthorizationCode{
		ID: id.NewAuthorizationCodeID(), CodeHash: memoryIntegritySignature("parallel authorize-" + owner.String()),
		AgentID: owner, ClientID: id.ClientID(owner.String()), Principal: id.NewPrincipal("owner@example.test"),
		RedirectURI: "https://client.example.test/callback", CodeChallenge: "S256challenge", Scope: "offline_access read",
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	writeCtx, stopWriting := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stopWriting()
	err := store.codes.Create(writeCtx, code)
	require.ErrorIs(t, err, context.DeadlineExceeded, "/authorize must wait for the held agent gate, not race the owner commit")
	close(release)
	require.NoError(t, <-ownerResult, "the owner must not encounter a version conflict from concurrent code issuance")
	require.NoError(t, store.codes.Create(ctx, code))
	committed, err := store.codes.FindByCodeHash(ctx, code.CodeHash)
	require.NoError(t, err)
	require.Equal(t, code.ID, committed.ID)
}
