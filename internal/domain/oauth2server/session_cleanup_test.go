package oauth2server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
)

type cleanupCodeRepository struct {
	ports.AuthorizationCodeRepository
	deleteExpired func(context.Context) (int, error)
}

func (r cleanupCodeRepository) DeleteExpired(ctx context.Context) (int, error) {
	return r.deleteExpired(ctx)
}

type cleanupPKCERepository struct {
	ports.PKCESessionRepository
	deleteExpired func(context.Context) (int, error)
}

func (r cleanupPKCERepository) DeleteExpired(ctx context.Context) (int, error) {
	return r.deleteExpired(ctx)
}

type cleanupRefreshRepository struct {
	ports.RefreshTokenSessionRepository
	deleteExpired func(context.Context) (int, error)
}

func (r cleanupRefreshRepository) DeleteExpired(ctx context.Context) (int, error) {
	return r.deleteExpired(ctx)
}

func assertCleanupCounts(t *testing.T, want [3]int32, counts *[3]atomic.Int32) {
	t.Helper()
	for i, expected := range want {
		assert.Equal(t, expected, counts[i].Load())
	}
}

func TestSessionCleanup_StartupAndPeriodicSweeps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls [3]atomic.Int32
		deletion := func(index int) func(context.Context) (int, error) {
			return func(context.Context) (int, error) {
				calls[index].Add(1)
				return 1, nil
			}
		}
		cleanup := NewSessionCleanup(
			cleanupCodeRepository{deleteExpired: deletion(0)},
			cleanupPKCERepository{deleteExpired: deletion(1)},
			cleanupRefreshRepository{deleteExpired: deletion(2)},
			slog.Default(),
		)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go cleanup.Run(ctx)
		synctest.Wait()
		assertCleanupCounts(t, [3]int32{1, 1, 1}, &calls)

		time.Sleep(time.Minute - time.Nanosecond)
		synctest.Wait()
		assertCleanupCounts(t, [3]int32{1, 1, 1}, &calls)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		assertCleanupCounts(t, [3]int32{2, 2, 2}, &calls)

		cancel()
		synctest.Wait()
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		assertCleanupCounts(t, [3]int32{2, 2, 2}, &calls)
	})
}

func TestSessionCleanup_ContinuesAfterRepositoryFailure(t *testing.T) {
	for failedRepo, name := range []string{"authorization_codes", "pkce_sessions", "refresh_token_sessions"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs bytes.Buffer
				var successes [3]atomic.Int32
				failed := false
				deletion := func(index int) func(context.Context) (int, error) {
					return func(context.Context) (int, error) {
						if index == failedRepo && !failed {
							failed = true
							return 0, errors.New("storage failure with sensitive-token-value")
						}
						successes[index].Add(1)
						return 1, nil
					}
				}
				cleanup := NewSessionCleanup(
					cleanupCodeRepository{deleteExpired: deletion(0)},
					cleanupPKCERepository{deleteExpired: deletion(1)},
					cleanupRefreshRepository{deleteExpired: deletion(2)},
					slog.New(slog.NewJSONHandler(&logs, nil)),
				)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				go cleanup.Run(ctx)
				synctest.Wait()
				want := [3]int32{1, 1, 1}
				want[failedRepo] = 0
				assertCleanupCounts(t, want, &successes)
				assert.Contains(t, logs.String(), name)
				assert.NotContains(t, logs.String(), "sensitive-token-value")

				time.Sleep(time.Minute)
				synctest.Wait()
				for i := range want {
					want[i]++
				}
				assertCleanupCounts(t, want, &successes)
			})
		})
	}
}

func TestSessionCleanup_CancellationStopsInflightDeletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		finished := make(chan struct{})
		unexpectedDeletion := func(context.Context) (int, error) {
			t.Error("started another deletion after cancellation")
			return 0, nil
		}
		var deletionErr error
		cleanup := NewSessionCleanup(
			cleanupCodeRepository{deleteExpired: func(ctx context.Context) (int, error) {
				close(started)
				<-ctx.Done()
				deletionErr = ctx.Err()
				return 0, deletionErr
			}},
			cleanupPKCERepository{deleteExpired: unexpectedDeletion},
			cleanupRefreshRepository{deleteExpired: unexpectedDeletion},
			slog.Default(),
		)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			defer close(finished)
			cleanup.Run(ctx)
		}()
		<-started
		cancel()
		<-finished
		assert.ErrorIs(t, deletionErr, context.Canceled)
	})
}
