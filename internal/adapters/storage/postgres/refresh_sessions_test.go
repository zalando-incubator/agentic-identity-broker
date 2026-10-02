//go:build integration
// +build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func pgRefreshSignature(label string) string {
	sum := sha256.Sum256([]byte(label))
	return hex.EncodeToString(sum[:])
}

func createPGRefreshRoot(t *testing.T, adapter *Adapter, agentID id.AgentID, principal id.Principal) (*storage.RefreshSession, *storage.RefreshToken) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var clientID id.ClientID
	if agentID.IsZero() {
		agent := createTestAgent(t, adapter)
		agentID = agent.ID
		clientID = *agent.ClientID
	} else {
		agent, err := NewAgentRepository(adapter).Get(ctx, agentID)
		require.NoError(t, err)
		clientID = *agent.ClientID
	}
	grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: principal, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, NewUserGrantRepository(adapter).Create(ctx, grant))
	rootID := id.NewRefreshSessionID()
	firstSignature := pgRefreshSignature(rootID.String())
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: grant.ID, OriginalTokenSignature: firstSignature,
		AgentID: agentID, Principal: principal, ClientID: clientID, Scope: "offline_access read",
		StartedAt: now, LastFreshAt: now, InactivityExpiresAt: now.Add(time.Hour),
		RetainUntil: now.Add(time.Hour), BranchKeyID: "refresh_" + rootID.String() + "_branch_key",
		CurrentSignature: firstSignature,
	}
	first := &storage.RefreshToken{Signature: firstSignature, SessionID: rootID, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	repo := NewRefreshSessionRepo(adapter)
	tokens := NewRefreshTokenRepo(adapter)
	coordinator := NewAuthorizationSessionCoordinator(adapter)
	require.NoError(t, coordinator.Run(ctx, agentID, func(scope context.Context, at time.Time) error {
		root.StartedAt, root.LastFreshAt = at, at
		root.InactivityExpiresAt, root.RetainUntil = at.Add(time.Hour), at.Add(time.Hour)
		first.IssuedAt, first.ExpiresAt = at, at.Add(time.Hour)
		if err := repo.Create(scope, root); err != nil {
			return err
		}
		return tokens.Create(scope, first)
	}))
	var anchored bool
	require.NoError(t, adapter.db.GetContext(ctx, &anchored, `
		SELECT session_id = $2 AND request_id = $2::text AND predecessor_signature IS NULL
		FROM refresh_token_sessions WHERE signature = $1`, firstSignature, rootID))
	require.True(t, anchored, "first issuance must have an anchored old-binary-compatible mirror")
	return root, first
}

func pgRotateRefresh(ctx context.Context, sessions ports.RefreshSessionRepository, tokens ports.RefreshTokenRepository, rootID id.RefreshSessionID, successor string, at time.Time) error {
	root, err := sessions.FindByID(ctx, rootID)
	if err != nil {
		return err
	}
	current, err := tokens.FindBySignature(ctx, root.CurrentSignature)
	if err != nil {
		return err
	}
	if current.UsedAt != nil {
		return fmt.Errorf("current token already consumed")
	}
	if err := tokens.MarkUsed(ctx, current.Signature, at); err != nil {
		return err
	}
	child := &storage.RefreshToken{Signature: successor, SessionID: rootID, IssuedAt: at, ExpiresAt: at.Add(time.Hour)}
	if err := tokens.Create(ctx, child); err != nil {
		return err
	}
	previous, empty, fingerprint := root.CurrentSignature, "", pgRefreshSignature("redacted context")
	reuseUntil, accessUntil := at.Add(30*time.Second), at.Add(5*time.Minute)
	root.CurrentSignature = successor
	root.PreviousSignature = &previous
	root.PreviousConsumedAt = &at
	root.ReuseUntil = &reuseUntil
	root.OriginalRequestedScope = &empty
	root.OriginalRequestContextFingerprint = &fingerprint
	root.RetryAccessExpiresAt = &accessUntil
	root.RetryExpiresAt = &reuseUntil
	root.RetryCiphertext = []byte{0x93, 0x12, 0x40, 0xff} // Opaque, not a plaintext credential.
	root.LastFreshAt = at
	root.InactivityExpiresAt = child.ExpiresAt
	root.RetainUntil = child.ExpiresAt
	return sessions.Save(ctx, root)
}

func TestPGRefreshRollbackAcrossParticipantsAndMirror(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	repo := NewRefreshSessionRepo(adapter)
	coordinator := NewAuthorizationSessionCoordinator(adapter)
	cred := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: root.AgentID, SecretHash: "initial-hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, NewClientCredentialRepo(adapter).Create(ctx, cred))
	code := &storage.AuthorizationCode{
		ID: id.NewAuthorizationCodeID(), CodeHash: pgRefreshSignature("code-" + root.ID.String()),
		AgentID: root.AgentID, ClientID: root.ClientID, Principal: root.Principal,
		RedirectURI: "https://client.example.test/callback", CodeChallenge: "S256challenge", Scope: root.Scope,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, NewAuthorizationCodeRepo(adapter).Create(ctx, code))
	pkce := &storage.PKCESession{Signature: code.CodeHash, CodeChallenge: code.CodeChallenge, CodeChallengeMethod: "S256", ExpiresAt: code.ExpiresAt, CreatedAt: code.CreatedAt}
	require.NoError(t, NewPKCESessionRepo(adapter).Create(ctx, pkce))

	abort := errors.New("pre-commit verification denied")
	err := coordinator.Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		if err := NewUserGrantRepository(adapter).Delete(scope, root.OriginalGrantID); err != nil {
			return err
		}
		if err := NewClientCredentialRepo(adapter).Rotate(scope, root.AgentID,
			&storage.ClientCredential{ID: id.NewCredentialID(), AgentID: root.AgentID, SecretHash: "new-hash", CreatedAt: at}); err != nil {
			return err
		}
		if err := NewAuthorizationCodeRepo(adapter).MarkUsed(scope, code.ID); err != nil {
			return err
		}
		if err := NewPKCESessionRepo(adapter).Delete(scope, pkce.Signature); err != nil {
			return err
		}
		if err := repo.RevokeByID(scope, root.ID, at, storage.RefreshReasonGrantDeleted); err != nil {
			return err
		}
		return abort
	})
	require.ErrorIs(t, err, abort)
	var durable bool
	require.NoError(t, adapter.db.GetContext(ctx, &durable, `
		SELECT r.revoked_at IS NULL AND r.terminal_reason IS NULL
		 AND legacy.used_at IS NULL AND tok.used_at IS NULL
		 AND (SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = r.id) = 0
		 AND (SELECT count(*) FROM user_grants WHERE id = $2) = 1
		 AND (SELECT count(*) FROM client_credentials WHERE agent_id = r.agent_id AND id = $3) = 1
		 AND (SELECT count(*) FROM authorization_codes WHERE id = $4 AND used_at IS NULL) = 1
		 AND (SELECT count(*) FROM pkce_sessions WHERE signature = $5) = 1
		 FROM refresh_sessions r
		 JOIN refresh_tokens tok ON tok.signature = r.current_signature
		 JOIN refresh_token_sessions legacy ON legacy.signature = tok.signature
		 WHERE r.id = $1`, root.ID, root.OriginalGrantID, cred.ID, code.ID, pkce.Signature))
	require.True(t, durable, "a confirmed rollback must undo grant, credential, code, PKCE, root, token and legacy changes")
	current, err := NewRefreshTokenRepo(adapter).FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, current.UsedAt)
}

func TestPGRefreshConfirmedRotationRollbackPreservesFirstAndMirror(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	repo, tokens := NewRefreshSessionRepo(adapter), NewRefreshTokenRepo(adapter)
	abort := errors.New("token signer failed before commit")
	successor := pgRefreshSignature("discarded-rotation" + root.ID.String())
	err := NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		if err := pgRotateRefresh(scope, repo, tokens, root.ID, successor, at); err != nil {
			return err
		}
		return abort
	})
	require.ErrorIs(t, err, abort)
	var predecessorUsable bool
	require.NoError(t, adapter.db.GetContext(ctx, &predecessorUsable, `
		SELECT root.current_signature = $2 AND root.previous_signature IS NULL
		 AND root.last_fresh_at = $3 AND root.inactivity_expires_at = $4
		 AND original.used_at IS NULL AND mirror.used_at IS NULL
		 AND (SELECT count(*) FROM refresh_tokens WHERE session_id = root.id) = 1
		 AND (SELECT count(*) FROM refresh_token_sessions WHERE session_id = root.id) = 1
		 FROM refresh_sessions root JOIN refresh_tokens original ON original.signature = root.current_signature
		 JOIN refresh_token_sessions mirror ON mirror.signature = original.signature
		 WHERE root.id = $1`, root.ID, first.Signature, root.LastFreshAt, root.InactivityExpiresAt))
	require.True(t, predecessorUsable, "a confirmed rollback preserves time, first token, and old mirror")
	_, err = tokens.FindBySignature(ctx, successor)
	require.True(t, ports.IsNotFoundErr(err))
}

func TestPGRefreshReceiptSurvivesAgentCascadeAndRetainsOtherOwners(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	owner, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	second, _ := createPGRefreshRoot(t, adapter, owner.AgentID, id.NewPrincipal("neighbor@example.test"))
	unrelated, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	coordinator := NewAuthorizationSessionCoordinator(adapter)
	repo := NewRefreshSessionRepo(adapter)
	require.NoError(t, coordinator.Run(ctx, owner.AgentID, func(scope context.Context, at time.Time) error {
		if err := repo.RevokeByAgent(scope, owner.AgentID, at, storage.RefreshReasonAgentDeleted); err != nil {
			return err
		}
		return NewAgentRepository(adapter).Delete(scope, owner.AgentID)
	}))
	for _, root := range []*storage.RefreshSession{owner, second} {
		var survived bool
		require.NoError(t, adapter.db.GetContext(ctx, &survived, `
			SELECT agent_id = $2 AND principal = $3 AND client_id = $4 AND reason = 'agent_deleted'
			 AND "at" >= $5 AND redacted_context::text NOT LIKE '%hash%'
			 FROM refresh_revocation_receipts WHERE session_id = $1`,
			root.ID, owner.AgentID, root.Principal, root.ClientID, root.StartedAt))
		require.True(t, survived, "each removed root needs its own independent non-credential audit receipt")
		var count int
		require.NoError(t, adapter.db.GetContext(ctx, &count, `SELECT count(*) FROM refresh_sessions WHERE id = $1`, root.ID))
		require.Zero(t, count)
	}
	var count int
	require.NoError(t, adapter.db.GetContext(ctx, &count, `SELECT count(*) FROM refresh_revocation_receipts WHERE agent_id = $1`, owner.AgentID))
	require.Equal(t, 2, count)
	active, err := repo.FindByID(ctx, unrelated.ID)
	require.NoError(t, err)
	require.Nil(t, active.TerminalReason)
}

func TestPGRefreshFailedReceiptWriteRollsBackLifecycle(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	_, err := adapter.db.ExecContext(ctx, `ALTER TABLE refresh_revocation_receipts ADD CONSTRAINT reject_refresh_receipt CHECK (reason <> 'grant_deleted')`)
	require.NoError(t, err)
	repo := NewRefreshSessionRepo(adapter)
	err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		if err := NewUserGrantRepository(adapter).Delete(scope, root.OriginalGrantID); err != nil {
			return err
		}
		return repo.RevokeByPrincipalAndAgent(scope, root.Principal, root.AgentID, at, storage.RefreshReasonGrantDeleted)
	})
	require.Error(t, err, "a missing receipt makes the lifecycle outcome fail")
	var unchanged bool
	require.NoError(t, adapter.db.GetContext(ctx, &unchanged, `
		SELECT r.revoked_at IS NULL AND r.terminal_reason IS NULL AND m.used_at IS NULL
		 AND (SELECT count(*) FROM user_grants WHERE id = r.original_grant_id) = 1
		 AND (SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = r.id) = 0
		 FROM refresh_sessions r JOIN refresh_token_sessions m ON m.signature = r.current_signature
		 WHERE r.id = $1`, root.ID))
	require.True(t, unchanged)
	found, err := NewRefreshTokenRepo(adapter).FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, found.UsedAt)
}

func TestPGRefreshOneConnectionUsesAmbientExecutor(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	code := &storage.AuthorizationCode{
		ID: id.NewAuthorizationCodeID(), CodeHash: pgRefreshSignature("single-conn" + root.ID.String()),
		AgentID: root.AgentID, ClientID: root.ClientID, Principal: root.Principal,
		RedirectURI: "https://client.example.test/callback", CodeChallenge: "challenge", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, NewAuthorizationCodeRepo(adapter).Create(ctx, code))
	pkce := &storage.PKCESession{Signature: code.CodeHash, CodeChallenge: code.CodeChallenge, CodeChallengeMethod: "S256", ExpiresAt: code.ExpiresAt, CreatedAt: code.CreatedAt}
	require.NoError(t, NewPKCESessionRepo(adapter).Create(ctx, pkce))
	require.NoError(t, NewClientCredentialRepo(adapter).Create(ctx, &storage.ClientCredential{
		ID: id.NewCredentialID(), AgentID: root.AgentID, SecretHash: "secret-hash", CreatedAt: time.Now().UTC(),
	}))
	adapter.db.SetMaxOpenConns(1)
	scopeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	repo := NewRefreshSessionRepo(adapter)
	err := NewAuthorizationSessionCoordinator(adapter).Run(scopeCtx, root.AgentID, func(scope context.Context, at time.Time) error {
		if _, err := NewAgentRepository(adapter).Get(scope, root.AgentID); err != nil {
			return err
		}
		if _, err := NewUserGrantRepository(adapter).Get(scope, root.OriginalGrantID); err != nil {
			return err
		}
		if _, err := NewClientCredentialRepo(adapter).GetByAgentID(scope, root.AgentID); err != nil {
			return err
		}
		if _, err := NewAuthorizationCodeRepo(adapter).FindByCodeHash(scope, code.CodeHash); err != nil {
			return err
		}
		if _, err := NewPKCESessionRepo(adapter).FindBySignature(scope, pkce.Signature); err != nil {
			return err
		}
		if _, err := repo.FindByID(scope, root.ID); err != nil {
			return err
		}
		if _, err := NewRefreshTokenRepo(adapter).FindBySignature(scope, root.CurrentSignature); err != nil {
			return err
		}
		clockAt, err := NewAuthorizationSessionCoordinator(adapter).Now(scope)
		if err != nil {
			return err
		}
		if clockAt.Before(at) {
			return fmt.Errorf("shared database clock moved backwards within the scope")
		}
		return nil
	})
	require.NoError(t, err, "scoped authorization reads must not wait for a second pool connection")
}

func TestPGRefreshDecisionTimeSampledAfterAgentRowLock(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	holder, err := adapter.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	var lockedID id.AgentID
	require.NoError(t, holder.GetContext(ctx, &lockedID, `SELECT id FROM agents WHERE id = $1 FOR UPDATE`, root.AgentID))
	require.Equal(t, root.AgentID, lockedID)
	decision := make(chan time.Time, 1)
	outcome := make(chan error, 1)
	go func() {
		outcome <- NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(_ context.Context, at time.Time) error {
			decision <- at
			return nil
		})
	}()
	require.Eventually(t, func() bool {
		var blocked int
		err := adapter.db.GetContext(ctx, &blocked, `
			SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`)
		return err == nil && blocked > 0
	}, 2*time.Second, 10*time.Millisecond, "the new writer must wait for the existing agent row guard")
	var justBeforeRelease time.Time
	require.NoError(t, holder.GetContext(ctx, &justBeforeRelease, `SELECT clock_timestamp()`))
	require.NoError(t, holder.Commit())
	require.NoError(t, <-outcome)
	select {
	case at := <-decision:
		require.False(t, at.Before(justBeforeRelease), "a waiting writer must sample database time after acquiring the gate")
	case <-ctx.Done():
		t.Fatal("writer never entered after agent lock was released")
	}
}

func TestPGRefreshRetentionAndPrincipalIsolation(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	owner, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	neighbor, _ := createPGRefreshRoot(t, adapter, owner.AgentID, id.NewPrincipal("neighbor@example.test"))
	otherAgent, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	repo := NewRefreshSessionRepo(adapter)
	require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, owner.AgentID, func(scope context.Context, at time.Time) error {
		return repo.RevokeByPrincipalAndAgent(scope, owner.Principal, owner.AgentID, at, storage.RefreshReasonGrantDeleted)
	}))
	terminal, err := repo.FindByID(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, storage.RefreshReasonGrantDeleted, *terminal.TerminalReason)
	count, err := repo.DeleteTerminal(ctx, owner.RetainUntil.Add(-time.Nanosecond), 10)
	require.NoError(t, err)
	require.Zero(t, count)
	_, err = NewRefreshTokenRepo(adapter).FindBySignature(ctx, first.Signature)
	require.NoError(t, err, "replay evidence survives through the issued refresh expiry")
	count, err = repo.DeleteTerminal(ctx, owner.RetainUntil, 10)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, err = repo.FindByID(ctx, owner.ID)
	require.True(t, ports.IsNotFoundErr(err))
	for _, survivor := range []*storage.RefreshSession{neighbor, otherAgent} {
		stored, err := repo.FindByID(ctx, survivor.ID)
		require.NoError(t, err)
		require.Nil(t, stored.TerminalReason)
	}
	var receipts int
	require.NoError(t, adapter.db.GetContext(ctx, &receipts, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1`, owner.ID))
	require.Equal(t, 1, receipts, "purging terminal root and tokens must never purge its audit receipt")
}

func TestPGRefreshOldFirstMirrorConsumptionBlocksNewSuccessor(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	repo := NewRefreshSessionRepo(adapter)
	oldTx, err := adapter.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = oldTx.Rollback() }()
	result, err := oldTx.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = NOW() WHERE signature = $1 AND used_at IS NULL`, first.Signature)
	require.NoError(t, err)
	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	successor := pgRefreshSignature("old-first-next" + root.ID.String())
	started := make(chan struct{})
	outcome := make(chan error, 1)
	go func() {
		outcome <- NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
			close(started)
			return pgRotateRefresh(scope, repo, NewRefreshTokenRepo(adapter), root.ID, successor, at)
		})
	}()
	select {
	case <-started:
	case err := <-outcome:
		require.NoError(t, err, "new writer should enter scope after old writer locks only the legacy row")
		t.Fatal("new writer never entered the agent scope")
	case <-ctx.Done():
		t.Fatal("new writer did not enter agent scope")
	}
	require.NoError(t, oldTx.Commit())
	require.Error(t, <-outcome, "unmatched old-writer consumption cannot spawn a new successor")
	var consistent bool
	require.NoError(t, adapter.db.GetContext(ctx, &consistent, `
		SELECT root.current_signature = $2 AND root.previous_signature IS NULL
		 AND token.used_at IS NULL AND mirror.used_at IS NOT NULL
		 AND (SELECT count(*) FROM refresh_tokens WHERE session_id = root.id) = 1
		 AND (SELECT count(*) FROM refresh_token_sessions WHERE session_id = root.id) = 1
		 FROM refresh_sessions root
		 JOIN refresh_tokens token ON token.signature = root.current_signature
		 JOIN refresh_token_sessions mirror ON mirror.signature = token.signature
		 WHERE root.id = $1`, root.ID, first.Signature))
	require.True(t, consistent, "an old-only consumption cannot mutate the evidenced new lineage")
	_, err = NewRefreshTokenRepo(adapter).FindBySignature(ctx, successor)
	require.True(t, ports.IsNotFoundErr(err))
}

func TestPGRefreshNewFirstMirrorLockRejectsOldConditionalUse(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	repo := NewRefreshSessionRepo(adapter)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unlock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unlock()
	locked := make(chan struct{})
	newOutcome := make(chan error, 1)
	successor := pgRefreshSignature("new-first-next" + root.ID.String())
	go func() {
		newOutcome <- NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
			if err := pgRotateRefresh(scope, repo, NewRefreshTokenRepo(adapter), root.ID, successor, at); err != nil {
				return err
			}
			close(locked) // New writer holds both the agent gate and the old mirror row lock.
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-locked:
	case err := <-newOutcome:
		require.NoError(t, err, "new writer must reach the mirror lock before old writer")
		t.Fatal("new writer did not rotate")
	case <-ctx.Done():
		t.Fatal("new writer did not reach mirror lock")
	}
	oldConn, err := adapter.db.Connx(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, oldConn.Close()) }()
	_, err = oldConn.ExecContext(ctx, `SET application_name = 'refresh_old_writer_conditional'`)
	require.NoError(t, err)
	oldOutcome := make(chan struct {
		rows int64
		err  error
	}, 1)
	go func() {
		result, err := oldConn.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = NOW() WHERE signature = $1 AND used_at IS NULL`, first.Signature)
		if err != nil {
			oldOutcome <- struct {
				rows int64
				err  error
			}{err: err}
			return
		}
		rows, err := result.RowsAffected()
		oldOutcome <- struct {
			rows int64
			err  error
		}{rows, err}
	}()
	require.Eventually(t, func() bool {
		var blocked int
		err := adapter.db.GetContext(ctx, &blocked, `SELECT count(*) FROM pg_stat_activity WHERE application_name = 'refresh_old_writer_conditional' AND wait_event_type = 'Lock'`)
		return err == nil && blocked == 1
	}, 2*time.Second, 10*time.Millisecond, "old binary's conditional MarkUsed must wait for the new writer's row lock")
	unlock()
	require.NoError(t, <-newOutcome)
	old := <-oldOutcome
	require.NoError(t, old.err)
	require.Zero(t, old.rows, "old conditional update cannot consume a new writer's already rotated mirror")
	var consistent bool
	require.NoError(t, adapter.db.GetContext(ctx, &consistent, `
		SELECT root.current_signature = $2 AND root.previous_signature = $3
		 AND prior.used_at IS NOT NULL AND mirror.used_at IS NULL
		 AND mirror.predecessor_signature = $3
		 AND (SELECT count(*) FROM refresh_tokens WHERE session_id = root.id AND used_at IS NULL) = 1
		 FROM refresh_sessions root
		 JOIN refresh_tokens prior ON prior.signature = $3
		 JOIN refresh_token_sessions mirror ON mirror.signature = root.current_signature
		 WHERE root.id = $1`, root.ID, successor, first.Signature))
	require.True(t, consistent)
}

func TestPGRefreshCoordinatorSerializesOneSuccessor(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
	repo := NewRefreshSessionRepo(adapter)
	candidates := []string{pgRefreshSignature("parallel-left" + root.ID.String()), pgRefreshSignature("parallel-right" + root.ID.String())}
	start := make(chan struct{})
	outcomes := make(chan error, 2)
	for _, signature := range candidates {
		go func(signature string) {
			<-start
			outcomes <- NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
				current, err := repo.FindByID(scope, root.ID)
				if err != nil {
					return err
				}
				if current.CurrentSignature != first.Signature {
					return nil // A later caller sees the committed successor rather than minting another.
				}
				return pgRotateRefresh(scope, repo, NewRefreshTokenRepo(adapter), root.ID, signature, at)
			})
		}(signature)
	}
	close(start)
	for range candidates {
		require.NoError(t, <-outcomes)
	}
	var count int
	require.NoError(t, adapter.db.GetContext(ctx, &count, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1 AND used_at IS NULL`, root.ID))
	require.Equal(t, 1, count)
	require.NoError(t, adapter.db.GetContext(ctx, &count, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, root.ID))
	require.Equal(t, 2, count, "concurrent refreshes cannot branch into two successors")
	var mirrors int
	require.NoError(t, adapter.db.GetContext(ctx, &mirrors, `SELECT count(*) FROM refresh_token_sessions WHERE session_id = $1 AND used_at IS NULL`, root.ID))
	require.Equal(t, 1, mirrors, "the old-binary mirror must agree with the one current new token")
}

// These driver wrappers execute the real PostgreSQL COMMIT (or ROLLBACK) first,
// then lose only the acknowledgement. They do not stand in for a repository.
// An adapter must not report a known rollback for either lost acknowledgement.
type pgRefreshAckFault struct {
	rollback bool
	faulted  atomic.Bool
}

type pgRefreshAckDriver struct{ states sync.Map }

type pgRefreshAckConn struct {
	driver.Conn
	state *pgRefreshAckFault
}

type pgRefreshAckTx struct {
	driver.Tx
	state *pgRefreshAckFault
}

var (
	pgRefreshAckOnce sync.Once
	pgRefreshAckSQL  = &pgRefreshAckDriver{}
)

func (d *pgRefreshAckDriver) Open(name string) (driver.Conn, error) {
	key, dsn, ok := strings.Cut(name, "|")
	if !ok {
		return nil, fmt.Errorf("ack fault DSN requires a test ID")
	}
	value, ok := d.states.Load(key)
	if !ok {
		return nil, fmt.Errorf("unknown ack fault: %s", key)
	}
	underlying, err := stdlib.GetDefaultDriver().Open(dsn)
	if err != nil {
		return nil, err
	}
	return &pgRefreshAckConn{Conn: underlying, state: value.(*pgRefreshAckFault)}, nil
}

func (c *pgRefreshAckConn) Begin() (driver.Tx, error) {
	tx, err := c.Conn.Begin()
	if err != nil {
		return nil, err
	}
	return &pgRefreshAckTx{Tx: tx, state: c.state}, nil
}

func (c *pgRefreshAckConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &pgRefreshAckTx{Tx: tx, state: c.state}, nil
}

func (c *pgRefreshAckConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *pgRefreshAckConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (c *pgRefreshAckConn) CheckNamedValue(arg *driver.NamedValue) error {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(arg)
	}
	return driver.ErrSkip
}

func (c *pgRefreshAckConn) Ping(ctx context.Context) error {
	return c.Conn.(driver.Pinger).Ping(ctx)
}

func (tx *pgRefreshAckTx) Commit() error {
	if !tx.state.faulted.CompareAndSwap(false, true) {
		return tx.Tx.Commit()
	}
	if tx.state.rollback {
		if err := tx.Tx.Rollback(); err != nil {
			return err
		}
	} else if err := tx.Tx.Commit(); err != nil {
		return err
	}
	return errors.New("test-owned connection lost commit acknowledgement")
}

func injectPGRefreshLostAck(t *testing.T, adapter *Adapter, dsn string, rollback bool) {
	t.Helper()
	pgRefreshAckOnce.Do(func() { sql.Register("refresh_pg_ack_fault", pgRefreshAckSQL) })
	key := id.NewRefreshSessionID().String()
	state := &pgRefreshAckFault{rollback: rollback}
	pgRefreshAckSQL.states.Store(key, state)
	t.Cleanup(func() { pgRefreshAckSQL.states.Delete(key) })
	faultDB, err := sql.Open("refresh_pg_ack_fault", key+"|"+dsn)
	require.NoError(t, err)
	faultDB.SetMaxOpenConns(2)
	original := adapter.db
	adapter.db = sqlx.NewDb(faultDB, "pgx")
	require.NoError(t, original.Close())
}

func TestPGRefreshLostCommitAcknowledgementResolvesDurableRotation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rollback bool
	}{
		{"commit happened", false},
		{"commit did not happen", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn, cleanup := setupMigratedConnString(t)
			defer cleanup()
			adapter, err := NewAdapter(testStorageConfig(dsn))
			require.NoError(t, err)
			require.NoError(t, adapter.Initialize(context.Background()))
			defer func() { require.NoError(t, adapter.Close(context.Background())) }()
			ctx := context.Background()
			root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
			repo := NewRefreshSessionRepo(adapter)
			independent, err := sql.Open("pgx", dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, independent.Close()) }()
			injectPGRefreshLostAck(t, adapter, dsn, tc.rollback)
			successor := pgRefreshSignature("lost-ack-next" + root.ID.String())
			err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
				return pgRotateRefresh(scope, repo, NewRefreshTokenRepo(adapter), root.ID, successor, at)
			})
			require.ErrorIs(t, err, ports.ErrCommitIndeterminate, "lost acknowledgement cannot claim rollback or return a result")
			var predecessorUsed, successorExists, mirrorUsed bool
			require.NoError(t, independent.QueryRowContext(ctx, `
				SELECT tok.used_at IS NOT NULL, EXISTS(SELECT 1 FROM refresh_tokens WHERE signature = $2), mirror.used_at IS NOT NULL
				FROM refresh_tokens tok JOIN refresh_token_sessions mirror ON mirror.signature = tok.signature
				WHERE tok.signature = $1`, first.Signature, successor).Scan(&predecessorUsed, &successorExists, &mirrorUsed))
			require.Equal(t, !tc.rollback, predecessorUsed, "authoritative state, not the error, decides recovery")
			require.Equal(t, !tc.rollback, successorExists)
			require.Equal(t, !tc.rollback, mirrorUsed)
			stored, err := repo.FindByID(ctx, root.ID)
			require.NoError(t, err)
			if tc.rollback {
				require.Equal(t, first.Signature, stored.CurrentSignature)
				require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
					return pgRotateRefresh(scope, repo, NewRefreshTokenRepo(adapter), root.ID, successor, at)
				}))
			} else {
				require.Equal(t, successor, stored.CurrentSignature)
				require.Equal(t, first.Signature, *stored.PreviousSignature)
				require.NotEmpty(t, stored.RetryCiphertext)
			}
			var childCount int
			require.NoError(t, independent.QueryRowContext(ctx, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, root.ID).Scan(&childCount))
			require.Equal(t, 2, childCount, "recovery must not create another successor")
		})
	}
}

func TestPGRefreshLostRetryCountAcknowledgementDoesNotRefund(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "committed retry"
		if rollback {
			name = "rolled back retry"
		}
		t.Run(name, func(t *testing.T) {
			dsn, cleanup := setupMigratedConnString(t)
			defer cleanup()
			adapter, err := NewAdapter(testStorageConfig(dsn))
			require.NoError(t, err)
			require.NoError(t, adapter.Initialize(context.Background()))
			defer func() { require.NoError(t, adapter.Close(context.Background())) }()
			ctx := context.Background()
			root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("owner@example.test"))
			repo := NewRefreshSessionRepo(adapter)
			second := pgRefreshSignature("retry-next" + root.ID.String())
			require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
				return pgRotateRefresh(scope, repo, NewRefreshTokenRepo(adapter), root.ID, second, at)
			}))
			independent, err := sql.Open("pgx", dsn)
			require.NoError(t, err)
			defer func() { require.NoError(t, independent.Close()) }()
			injectPGRefreshLostAck(t, adapter, dsn, rollback)
			err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, _ time.Time) error {
				stored, err := repo.FindByID(scope, root.ID)
				if err != nil {
					return err
				}
				stored.RetryCount++
				return repo.Save(scope, stored)
			})
			require.ErrorIs(t, err, ports.ErrCommitIndeterminate)
			var count int
			require.NoError(t, independent.QueryRowContext(ctx, `SELECT retry_count FROM refresh_sessions WHERE id = $1`, root.ID).Scan(&count))
			if rollback {
				require.Zero(t, count)
			} else {
				require.Equal(t, 1, count)
			}
			require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, _ time.Time) error {
				stored, err := repo.FindByID(scope, root.ID)
				if err != nil {
					return err
				}
				stored.RetryCount++
				return repo.Save(scope, stored)
			}))
			var retryCount, children int
			require.NoError(t, independent.QueryRowContext(ctx, `
				SELECT retry_count, (SELECT count(*) FROM refresh_tokens WHERE session_id = $1)
				FROM refresh_sessions WHERE id = $1`, root.ID).Scan(&retryCount, &children))
			require.Equal(t, count+1, retryCount, "a committed retry result cannot be refunded")
			require.Equal(t, 2, children, "retry accounting cannot mint another successor")
		})
	}
}
