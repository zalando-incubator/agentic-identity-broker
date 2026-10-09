//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitRefreshBlock(t *testing.T, adapter *Adapter, ctx context.Context, blockerPID int) int {
	t.Helper()
	for {
		var pid int
		err := adapter.db.GetContext(ctx, &pid, `SELECT pid FROM pg_stat_activity
			WHERE datname = current_database() AND $1 = ANY(pg_blocking_pids(pid))`, blockerPID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		require.NoError(t, err, "expected a database lock wait on backend %d", blockerPID)
		return pid
	}
}

func refreshRaceSession(t *testing.T, adapter *Adapter, signature string, agentID id.AgentID) *storage.RefreshTokenSession {
	t.Helper()
	session := &storage.RefreshTokenSession{
		Signature: signature, RequestID: signature, AgentID: agentID,
		ClientID: id.NewClientID("race-client"), Principal: id.NewPrincipal("race@example.com"),
		ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, NewRefreshTokenSessionRepo(adapter).Create(context.Background(), session))
	return session
}

func TestRefreshTokenSessionRepoRevocationRotation(t *testing.T) {
	adapter, cleanup := setupRefreshTokenSessionTestDB(t)
	defer cleanup()
	repo := NewRefreshTokenSessionRepo(adapter)
	for _, action := range []string{"principal revocation", "agent revocation", "agent deletion"} {
		for _, rotationFirst := range []bool{true, false} {
			order := "revocation first"
			if rotationFirst {
				order = "rotation first"
			}
			t.Run(action+"/"+order, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				agent := createTestAgent(t, adapter)
				root := refreshRaceSession(t, adapter, agent.ID.String(), agent.ID)
				successor := *root
				successor.Signature += "-successor"
				operation := func() (int, error) {
					switch action {
					case "principal revocation":
						return repo.RevokeByPrincipalAndAgent(ctx, root.Principal, agent.ID)
					case "agent revocation":
						return repo.RevokeByAgent(ctx, agent.ID)
					default:
						_, err := NewAgentRepository(adapter).Delete(ctx, agent.ID)
						return 0, err
					}
				}
				type result struct {
					count int
					err   error
				}
				completed := make(chan result, 1)
				rotationCtx, err := adapter.BeginTX(ctx)
				require.NoError(t, err)
				defer func() { _ = adapter.Rollback(rotationCtx) }()
				rotation, _ := storageTransaction(rotationCtx)
				if rotationFirst {
					require.NoError(t, repo.MarkUsed(rotationCtx, root.Signature))
					var rotationPID int
					require.NoError(t, rotation.GetContext(ctx, &rotationPID, `SELECT pg_backend_pid()`))
					go func() {
						count, err := operation()
						completed <- result{count, err}
					}()
					waitRefreshBlock(t, adapter, ctx, rotationPID)
					require.NoError(t, repo.Create(rotationCtx, &successor))
					require.NoError(t, adapter.Commit(rotationCtx))
				} else {
					blocker, err := adapter.db.BeginTxx(ctx, nil)
					require.NoError(t, err)
					defer func() { _ = blocker.Rollback() }()
					var blockerPID int
					require.NoError(t, blocker.GetContext(ctx, &blockerPID, `SELECT pg_backend_pid()`))
					_, err = blocker.ExecContext(ctx, `SELECT 1 FROM refresh_token_sessions WHERE signature = $1 FOR UPDATE`, root.Signature)
					require.NoError(t, err)
					go func() {
						count, err := operation()
						completed <- result{count, err}
					}()
					revocationPID := waitRefreshBlock(t, adapter, ctx, blockerPID)
					rotated := make(chan error, 1)
					go func() {
						err := repo.MarkUsed(rotationCtx, root.Signature)
						if err == nil {
							err = repo.Create(rotationCtx, &successor)
						}
						rotated <- err
					}()
					waitRefreshBlock(t, adapter, ctx, revocationPID)
					require.NoError(t, blocker.Commit())
					select {
					case err := <-rotated:
						var storageErr *storage.StorageError
						require.ErrorAs(t, err, &storageErr)
						assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					require.NoError(t, adapter.Rollback(rotationCtx))
				}
				select {
				case got := <-completed:
					require.NoError(t, got.err)
					if action != "agent deletion" {
						assert.Equal(t, 1, got.count)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				for _, signature := range []string{root.Signature, successor.Signature} {
					got, err := repo.FindBySignature(ctx, signature)
					if action == "agent deletion" || (!rotationFirst && signature == successor.Signature) {
						var storageErr *storage.StorageError
						require.ErrorAs(t, err, &storageErr)
						assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
					} else {
						require.NoError(t, err)
						assert.NotNil(t, got.UsedAt, "no live refresh token may survive")
					}
				}
			})
		}
	}
}

func TestRefreshTokenSessionRepoSameAgentRotationsDoNotSerialize(t *testing.T) {
	adapter, cleanup := setupRefreshTokenSessionTestDB(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agent := createTestAgent(t, adapter)
	first := refreshRaceSession(t, adapter, "parallel-first", agent.ID)
	second := refreshRaceSession(t, adapter, "parallel-second", agent.ID)
	repo := NewRefreshTokenSessionRepo(adapter)
	firstCtx, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(firstCtx) }()
	require.NoError(t, repo.MarkUsed(firstCtx, first.Signature))
	secondCtx, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(secondCtx) }()
	require.NoError(t, repo.MarkUsed(secondCtx, second.Signature))
	require.NoError(t, adapter.Commit(secondCtx))
	require.NoError(t, adapter.Commit(firstCtx))
	for _, session := range []*storage.RefreshTokenSession{first, second} {
		got, err := repo.FindBySignature(ctx, session.Signature)
		require.NoError(t, err)
		assert.NotNil(t, got.UsedAt)
	}
}
