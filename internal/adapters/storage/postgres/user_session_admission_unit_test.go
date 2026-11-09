package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func newAdmissionTestRepository(t *testing.T, cfg signingKeyRepoTestConfig) *PostgresUserSessionRepository {
	t.Helper()
	serviceID := id.NewServiceID()
	cfg.queryColumns = []string{"id", "principal", "service_id", "encrypted_access_token", "token_type"}
	cfg.queryRows = [][]driver.Value{{id.NewSessionID().String(), "admission-user", serviceID.String(), []byte("encrypted-access"), "Bearer"}}
	adapter, err := NewAdapter(&ports.StorageConfig{
		Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: "postgres://admission-test"},
		Timeouts: ports.StorageTimeouts{Read: time.Second, Write: time.Second},
	})
	require.NoError(t, err)
	adapter.db = newUnitTestSigningKeyRepo(t, cfg).adapter.db
	adapter.db.SetMaxOpenConns(25)
	return NewUserSessionRepository(adapter)
}

func holdAdmissionTestBatch(t *testing.T, repo *PostgresUserSessionRepository) func() {
	t.Helper()
	entered := make(chan struct{}, 20)
	done := make(chan error, 20)
	proceed := make(chan struct{})
	release := sync.OnceFunc(func() {
		close(proceed)
		for range 20 {
			assert.NoError(t, <-done)
		}
	})
	t.Cleanup(release)
	for i := range 20 {
		go func() {
			_, err := repo.WithLockedSession(context.Background(), id.Principal(fmt.Sprintf("admission-%d", i)), id.NewServiceID(), func(context.Context, *storage.UserSession) error {
				entered <- struct{}{}
				<-proceed
				return nil
			})
			done <- err
		}()
	}
	synctest.Wait()
	require.Len(t, entered, 20, "all 20 admission slots must be usable concurrently")
	require.Equal(t, 20, repo.adapter.db.Stats().InUse)
	return release
}

func TestUserSessionAdmissionWaitDoesNotBorrowConnection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newAdmissionTestRepository(t, signingKeyRepoTestConfig{})
		release := holdAdmissionTestBatch(t, repo)
		defer release()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			otherRepo := NewUserSessionRepository(repo.adapter)
			_, err := otherRepo.WithLockedSession(ctx, "waiting-user", id.NewServiceID(), func(context.Context, *storage.UserSession) error {
				return errors.New("excess callback ran before admission")
			})
			done <- err
		}()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("excess operation returned before cancellation: %v", err)
		default:
		}
		require.Equal(t, 20, repo.adapter.db.Stats().InUse, "a blocked admission must not borrow a connection")
		connections := make([]*sqlx.Conn, 0, 5)
		defer func() {
			for _, conn := range connections {
				assert.NoError(t, conn.Close())
			}
		}()
		for range 5 {
			conn, err := repo.adapter.db.Connx(context.Background())
			require.NoError(t, err)
			connections = append(connections, conn)
		}
		assert.Equal(t, 25, repo.adapter.db.Stats().InUse)
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestUserSessionAdmissionAndConnectionShareTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := newAdmissionTestRepository(t, signingKeyRepoTestConfig{})
		repo.adapter.db.SetMaxOpenConns(1)
		conn, err := repo.adapter.db.Connx(context.Background())
		require.NoError(t, err)
		closeConn := sync.OnceFunc(func() { assert.NoError(t, conn.Close()) })
		defer closeConn()
		for range cap(repo.adapter.lockedSessionSlots) {
			repo.adapter.lockedSessionSlots <- struct{}{}
		}
		start := time.Now()
		done := make(chan error, 1)
		go func() {
			_, err := repo.WithLockedSession(context.Background(), "waiting-user", id.NewServiceID(), func(context.Context, *storage.UserSession) error {
				return errors.New("callback ran while connection pool was full")
			})
			done <- err
		}()
		synctest.Wait()
		time.Sleep(750 * time.Millisecond)
		<-repo.adapter.lockedSessionSlots
		synctest.Wait()
		assert.Equal(t, int64(1), repo.adapter.db.Stats().WaitCount, "operation must reach connection acquisition after admission")
		require.ErrorIs(t, <-done, context.DeadlineExceeded)
		assert.Equal(t, repo.adapter.timeouts.Write, time.Since(start), "connection acquisition must not restart the admission deadline")
		assert.Len(t, repo.adapter.lockedSessionSlots, cap(repo.adapter.lockedSessionSlots)-1, "timed-out acquisition must return its slot")
		for range cap(repo.adapter.lockedSessionSlots) - 1 {
			<-repo.adapter.lockedSessionSlots
		}
		closeConn()
		repo.adapter.db.SetMaxOpenConns(25)
		release := holdAdmissionTestBatch(t, repo)
		release()
	})
}

func TestUserSessionAdmissionRecoversFullCapacityAfterFailures(t *testing.T) {
	failure := errors.New("injected refresh failure")
	for _, tc := range []struct {
		name         string
		cfg          signingKeyRepoTestConfig
		callbackErr  error
		wantCallback bool
	}{
		{name: "connection acquisition"},
		{name: "lock acquisition", cfg: signingKeyRepoTestConfig{execErr: failure}},
		{name: "coordinated read", cfg: signingKeyRepoTestConfig{queryErr: failure}},
		{name: "callback", callbackErr: failure, wantCallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := newAdmissionTestRepository(t, tc.cfg)
				wantErr := failure
				if tc.name == "connection acquisition" {
					repo.adapter.db.SetMaxOpenConns(1)
					conn, err := repo.adapter.db.Connx(context.Background())
					require.NoError(t, err)
					defer func() { assert.NoError(t, conn.Close()) }()
					wantErr = context.DeadlineExceeded
				}
				called := false
				_, err := repo.WithLockedSession(context.Background(), "failed-user", id.NewServiceID(), func(_ context.Context, current *storage.UserSession) error {
					called = true
					current.EncryptedAccessToken = []byte("renewed-access")
					return tc.callbackErr
				})
				require.ErrorIs(t, err, wantErr)
				assert.Equal(t, tc.wantCallback, called)
				if tc.name != "connection acquisition" {
					assert.Zero(t, repo.adapter.db.Stats().InUse, "failed operations must return their connection")
				}
				// Remove the database fault without replacing the adapter's admission state.
				healthy := newAdmissionTestRepository(t, signingKeyRepoTestConfig{})
				repo.adapter.db = healthy.adapter.db
				for range 2 {
					release := holdAdmissionTestBatch(t, repo)
					release()
				}
			})
		})
	}
}
