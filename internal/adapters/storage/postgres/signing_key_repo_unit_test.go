package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const signingKeyRepoTestDriverName = "signing_key_repo_test_driver"

var (
	signingKeyRepoTestDriverOnce sync.Once
	signingKeyRepoTestConfigs    = struct {
		sync.Mutex
		values map[string]signingKeyRepoTestConfig
	}{values: make(map[string]signingKeyRepoTestConfig)}
)

type signingKeyRepoTestConfig struct {
	beginErr        error
	execErr         error
	execErrs        []error
	queryErr        error
	queryColumns    []string
	queryRows       [][]driver.Value
	queryResults    []signingKeyRepoTestQueryResult
	commitErr       error
	rowsAffected    int64
	recordedQueries *[]string
	recordedExecs   *[]string
}

type signingKeyRepoTestQueryResult struct {
	err     error
	columns []string
	rows    [][]driver.Value
}

type signingKeyRepoTestDriver struct{}

type signingKeyRepoTestConn struct {
	cfg        signingKeyRepoTestConfig
	execCalls  int
	queryCalls int
}

type signingKeyRepoTestTx struct {
	cfg signingKeyRepoTestConfig
}

var _ driver.Conn = (*signingKeyRepoTestConn)(nil)
var _ driver.ExecerContext = (*signingKeyRepoTestConn)(nil)
var _ driver.QueryerContext = (*signingKeyRepoTestConn)(nil)
var _ driver.ConnBeginTx = (*signingKeyRepoTestConn)(nil)
var _ driver.Tx = (*signingKeyRepoTestTx)(nil)

func (d *signingKeyRepoTestDriver) Open(name string) (driver.Conn, error) {
	signingKeyRepoTestConfigs.Lock()
	cfg, ok := signingKeyRepoTestConfigs.values[name]
	signingKeyRepoTestConfigs.Unlock()
	if !ok {
		return nil, fmt.Errorf("missing signing key repo test config for %q", name)
	}
	return &signingKeyRepoTestConn{cfg: cfg}, nil
}

func (c *signingKeyRepoTestConn) Prepare(_ string) (driver.Stmt, error) {
	return nil, errors.New("Prepare not implemented")
}

func (c *signingKeyRepoTestConn) Close() error {
	return nil
}

func (c *signingKeyRepoTestConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *signingKeyRepoTestConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if c.cfg.beginErr != nil {
		return nil, c.cfg.beginErr
	}
	return &signingKeyRepoTestTx{cfg: c.cfg}, nil
}

func (c *signingKeyRepoTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if c.cfg.recordedExecs != nil {
		*c.cfg.recordedExecs = append(*c.cfg.recordedExecs, query)
	}
	if len(c.cfg.execErrs) > 0 {
		var err error
		if c.execCalls < len(c.cfg.execErrs) {
			err = c.cfg.execErrs[c.execCalls]
		}
		c.execCalls++
		if err != nil {
			return nil, err
		}
		return driver.RowsAffected(c.cfg.rowsAffected), nil
	}
	if c.cfg.execErr != nil {
		return nil, c.cfg.execErr
	}
	c.execCalls++
	return driver.RowsAffected(c.cfg.rowsAffected), nil
}

func (c *signingKeyRepoTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.cfg.recordedQueries != nil {
		*c.cfg.recordedQueries = append(*c.cfg.recordedQueries, query)
	}
	if len(c.cfg.queryResults) > 0 {
		result := signingKeyRepoTestQueryResult{}
		if c.queryCalls < len(c.cfg.queryResults) {
			result = c.cfg.queryResults[c.queryCalls]
		}
		c.queryCalls++
		if result.err != nil {
			return nil, result.err
		}
		columns := result.columns
		if len(columns) == 0 {
			columns = []string{"id"}
		}
		rows := make([][]driver.Value, len(result.rows))
		for i, row := range result.rows {
			rows[i] = append([]driver.Value(nil), row...)
		}
		return &signingKeyRepoTestRows{columns: columns, rows: rows}, nil
	}
	if c.cfg.queryErr != nil {
		return nil, c.cfg.queryErr
	}

	columns := c.cfg.queryColumns
	if len(columns) == 0 {
		columns = []string{"id"}
	}

	rows := make([][]driver.Value, len(c.cfg.queryRows))
	for i, row := range c.cfg.queryRows {
		rows[i] = append([]driver.Value(nil), row...)
	}

	return &signingKeyRepoTestRows{columns: columns, rows: rows}, nil
}

func (t *signingKeyRepoTestTx) Commit() error {
	return t.cfg.commitErr
}

func (t *signingKeyRepoTestTx) Rollback() error {
	return nil
}

type signingKeyRepoTestRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *signingKeyRepoTestRows) Columns() []string {
	return r.columns
}

func (r *signingKeyRepoTestRows) Close() error {
	return nil
}

func (r *signingKeyRepoTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.index]
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		} else {
			dest[i] = nil
		}
	}
	r.index++
	return nil
}

func newUnitTestSigningKeyRepo(t *testing.T, cfg signingKeyRepoTestConfig) *SigningKeyRepo {
	t.Helper()
	return newUnitTestSigningKeyRepoWithTimeouts(t, cfg, ports.StorageTimeouts{
		Read:  time.Second,
		Write: time.Second,
	})
}

func newUnitTestSigningKeyRepoWithTimeouts(t *testing.T, cfg signingKeyRepoTestConfig, timeouts ports.StorageTimeouts) *SigningKeyRepo {
	t.Helper()

	signingKeyRepoTestDriverOnce.Do(func() {
		sql.Register(signingKeyRepoTestDriverName, &signingKeyRepoTestDriver{})
	})

	dsn := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	signingKeyRepoTestConfigs.Lock()
	signingKeyRepoTestConfigs.values[dsn] = cfg
	signingKeyRepoTestConfigs.Unlock()

	db, err := sql.Open(signingKeyRepoTestDriverName, dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		signingKeyRepoTestConfigs.Lock()
		delete(signingKeyRepoTestConfigs.values, dsn)
		signingKeyRepoTestConfigs.Unlock()
	})

	return NewSigningKeyRepo(&Adapter{
		db:       sqlx.NewDb(db, signingKeyRepoTestDriverName),
		timeouts: timeouts,
	})
}

func stubSigningKey() *storage.SigningKey {
	return &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-unit-test"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("ciphertext"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
}

func assertStorageErrorKind(t *testing.T, err error, operation string, kind storage.ErrorKind) {
	t.Helper()

	var se *storage.StorageError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, operation, se.Operation)
	assert.Equal(t, kind, se.Kind)
}

func TestClassifySigningKeyRepoError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		operation string
		kind      storage.ErrorKind
		message   string
	}{
		{
			name:      "maps sql ErrNoRows to not found",
			err:       sql.ErrNoRows,
			operation: "SigningKeyRepo.ListActive",
			kind:      storage.ErrorKindNotFound,
			message:   "failed to list signing keys",
		},
		{
			name:      "maps pgx ErrNoRows to not found",
			err:       pgx.ErrNoRows,
			operation: "SigningKeyRepo.ListActive",
			kind:      storage.ErrorKindNotFound,
			message:   "failed to list signing keys",
		},
		{
			name:      "maps serialization failure to conflict",
			err:       &pgconn.PgError{Code: "40001"},
			operation: "SigningKeyRepo.CreateAndSetCurrent",
			kind:      storage.ErrorKindConflict,
			message:   "failed to insert signing key",
		},
		{
			name:      "maps context canceled to timeout",
			err:       context.Canceled,
			operation: "SigningKeyRepo.ListActive",
			kind:      storage.ErrorKindTimeout,
			message:   "failed to list signing keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifySigningKeyRepoError(tt.operation, tt.err, tt.message)
			require.Error(t, err)
			assertStorageErrorKind(t, err, tt.operation, tt.kind)
		})
	}
}

func TestSigningKeyRepo_ErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		cfg         signingKeyRepoTestConfig
		operation   string
		kind        storage.ErrorKind
		wantMessage string
		call        func(repo *SigningKeyRepo) error
	}{
		{
			name:      "Create maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{execErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.Create",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				return repo.Create(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "Create maps generic exec error to connection",
			cfg:       signingKeyRepoTestConfig{execErr: errors.New("database unavailable")},
			operation: "SigningKeyRepo.Create",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				return repo.Create(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "Create maps unique violation to conflict",
			cfg:       signingKeyRepoTestConfig{execErr: &pgconn.PgError{Code: "23505"}},
			operation: "SigningKeyRepo.Create",
			kind:      storage.ErrorKindConflict,
			call: func(repo *SigningKeyRepo) error {
				return repo.Create(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "CreateAndSetCurrent maps begin deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{beginErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.CreateAndSetCurrent",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				return repo.CreateAndSetCurrent(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "CreateAndSetCurrent maps exec deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{execErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.CreateAndSetCurrent",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				return repo.CreateAndSetCurrent(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "CreateAndSetCurrent maps unique violation on insert to conflict",
			cfg:       signingKeyRepoTestConfig{execErrs: []error{nil, &pgconn.PgError{Code: "23505"}}},
			operation: "SigningKeyRepo.CreateAndSetCurrent",
			kind:      storage.ErrorKindConflict,
			call: func(repo *SigningKeyRepo) error {
				return repo.CreateAndSetCurrent(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "CreateAndSetCurrent maps commit generic error to connection",
			cfg:       signingKeyRepoTestConfig{commitErr: errors.New("commit failed")},
			operation: "SigningKeyRepo.CreateAndSetCurrent",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				return repo.CreateAndSetCurrent(context.Background(), stubSigningKey())
			},
		},
		{
			name:      "GetByKID maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.GetByKID",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetByKID(context.Background(), id.NewKeyID("kid-get-by-kid"))
				return err
			},
		},
		{
			name:      "GetByKID maps context canceled to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.Canceled},
			operation: "SigningKeyRepo.GetByKID",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetByKID(context.Background(), id.NewKeyID("kid-get-by-kid"))
				return err
			},
		},
		{
			name:      "GetByKID maps generic query error to connection",
			cfg:       signingKeyRepoTestConfig{queryErr: errors.New("query failed")},
			operation: "SigningKeyRepo.GetByKID",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetByKID(context.Background(), id.NewKeyID("kid-get-by-kid"))
				return err
			},
		},
		{
			name:      "GetByKID maps missing row to not found",
			cfg:       signingKeyRepoTestConfig{},
			operation: "SigningKeyRepo.GetByKID",
			kind:      storage.ErrorKindNotFound,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetByKID(context.Background(), id.NewKeyID("kid-get-by-kid"))
				return err
			},
		},
		{
			name:      "GetCurrent maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.GetCurrent",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetCurrent(context.Background())
				return err
			},
		},
		{
			name:      "GetCurrent maps context canceled to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.Canceled},
			operation: "SigningKeyRepo.GetCurrent",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetCurrent(context.Background())
				return err
			},
		},
		{
			name:      "GetCurrent maps generic query error to connection",
			cfg:       signingKeyRepoTestConfig{queryErr: errors.New("query failed")},
			operation: "SigningKeyRepo.GetCurrent",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetCurrent(context.Background())
				return err
			},
		},
		{
			name:      "GetCurrent maps missing row to not found",
			cfg:       signingKeyRepoTestConfig{},
			operation: "SigningKeyRepo.GetCurrent",
			kind:      storage.ErrorKindNotFound,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.GetCurrent(context.Background())
				return err
			},
		},
		{
			name:      "ListActive maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.ListActive",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.ListActive(context.Background())
				return err
			},
		},
		{
			name:      "ListActive maps generic query error to connection",
			cfg:       signingKeyRepoTestConfig{queryErr: errors.New("query failed")},
			operation: "SigningKeyRepo.ListActive",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.ListActive(context.Background())
				return err
			},
		},
		{
			name:      "SetPublicJWK maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{execErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.SetPublicJWK",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				return repo.SetPublicJWK(context.Background(), id.NewKeyID("kid-public-jwk"), []byte(`{"kty":"EC"}`))
			},
		},
		{
			name: "SetPublicJWK maps missing key to not found",
			cfg: signingKeyRepoTestConfig{
				queryColumns: []string{"exists"},
				queryRows:    [][]driver.Value{{false}},
			},
			operation: "SigningKeyRepo.SetPublicJWK",
			kind:      storage.ErrorKindNotFound,
			call: func(repo *SigningKeyRepo) error {
				return repo.SetPublicJWK(context.Background(), id.NewKeyID("kid-public-jwk"), []byte(`{"kty":"EC"}`))
			},
		},
		{
			name:      "SetCurrent maps begin deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{beginErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.SetCurrent",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.SetCurrent(context.Background(), id.NewKeyID("kid-set-current"), time.Now())
				return err
			},
		},
		{
			name: "SetCurrent maps exec deadline exceeded to timeout",
			cfg: signingKeyRepoTestConfig{
				execErr:      context.DeadlineExceeded,
				queryColumns: []string{"kid", "is_current", "activates_at"},
				queryRows:    [][]driver.Value{{"kid-set-current", false, time.Now()}},
			},
			operation: "SigningKeyRepo.SetCurrent",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.SetCurrent(context.Background(), id.NewKeyID("kid-set-current"), time.Now())
				return err
			},
		},
		{
			name: "SetCurrent maps commit generic error to connection",
			cfg: signingKeyRepoTestConfig{
				commitErr: errors.New("commit failed"),
				queryResults: []signingKeyRepoTestQueryResult{
					{
						columns: []string{"kid", "is_current", "activates_at"},
						rows:    [][]driver.Value{{"kid-set-current", false, time.Now()}},
					},
					{
						columns: []string{"id", "kid", "algorithm", "private_key_encrypted", "public_jwk", "is_current", "activates_at", "created_at", "removed_at"},
						rows:    [][]driver.Value{{"signing-key-id", "kid-set-current", "ES256", []byte("ciphertext"), []byte(`{"kty":"EC"}`), true, time.Now(), time.Now(), nil}},
					},
				},
				rowsAffected: 1,
			},
			operation: "SigningKeyRepo.SetCurrent",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.SetCurrent(context.Background(), id.NewKeyID("kid-set-current"), time.Now())
				return err
			},
		},
		{
			name: "SetCurrent maps missing key to not found",
			cfg: signingKeyRepoTestConfig{
				queryColumns: []string{"kid", "is_current", "activates_at"},
			},
			operation: "SigningKeyRepo.SetCurrent",
			kind:      storage.ErrorKindNotFound,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.SetCurrent(context.Background(), id.NewKeyID("kid-set-current"), time.Now())
				return err
			},
		},
		{
			name:      "Delete maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.Delete",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				return repo.Delete(context.Background(), id.NewKeyID("kid-delete"))
			},
		},
		{
			name: "Delete maps generic exec error to connection",
			cfg: signingKeyRepoTestConfig{
				queryColumns: []string{"kid", "is_current", "activates_at"},
				queryRows: [][]driver.Value{
					{"kid-delete", false, time.Now()},
					{"other-kid", true, time.Now()},
				},
				execErr: errors.New("delete failed"),
			},
			operation: "SigningKeyRepo.Delete",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				return repo.Delete(context.Background(), id.NewKeyID("kid-delete"))
			},
		},
		{
			name: "Delete maps zero rows to invariant violation",
			cfg: signingKeyRepoTestConfig{
				queryColumns: []string{"kid", "is_current", "activates_at"},
				queryRows: [][]driver.Value{
					{"kid-delete", false, time.Now()},
					{"other-kid", true, time.Now()},
				},
				rowsAffected: 0,
			},
			operation:   "SigningKeyRepo.Delete",
			kind:        storage.ErrorKindUnknown,
			wantMessage: "invariant violation: locked signing key was not updated",
			call: func(repo *SigningKeyRepo) error {
				return repo.Delete(context.Background(), id.NewKeyID("kid-delete"))
			},
		},
		{
			name:      "CountActive maps deadline exceeded to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.DeadlineExceeded},
			operation: "SigningKeyRepo.CountActive",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.CountActive(context.Background())
				return err
			},
		},
		{
			name:      "CountActive maps context canceled to timeout",
			cfg:       signingKeyRepoTestConfig{queryErr: context.Canceled},
			operation: "SigningKeyRepo.CountActive",
			kind:      storage.ErrorKindTimeout,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.CountActive(context.Background())
				return err
			},
		},
		{
			name:      "CountActive maps generic query error to connection",
			cfg:       signingKeyRepoTestConfig{queryErr: errors.New("query failed")},
			operation: "SigningKeyRepo.CountActive",
			kind:      storage.ErrorKindConnection,
			call: func(repo *SigningKeyRepo) error {
				_, err := repo.CountActive(context.Background())
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newUnitTestSigningKeyRepo(t, tt.cfg)

			err := tt.call(repo)
			require.Error(t, err)
			assertStorageErrorKind(t, err, tt.operation, tt.kind)
			if tt.wantMessage != "" {
				var storageErr *storage.StorageError
				require.ErrorAs(t, err, &storageErr)
				assert.Equal(t, tt.wantMessage, storageErr.Message)
			}
		})
	}
}

func TestSigningKeyRepo_SetPublicJWKUsesConditionalUpdate(t *testing.T) {
	var execs []string
	repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
		rowsAffected:  1,
		recordedExecs: &execs,
	})

	err := repo.SetPublicJWK(context.Background(), id.NewKeyID("kid-public-jwk"), []byte(`{"kty":"EC"}`))
	require.NoError(t, err)
	require.Len(t, execs, 1)
	assert.Contains(t, execs[0], "public_jwk IS NULL")
}

func TestSigningKeyRepo_MutationPathsLockActiveKeysInDeterministicOrder(t *testing.T) {
	now := time.Now().UTC()

	t.Run("SetCurrent locks active keys in kid order before updates", func(t *testing.T) {
		var queries []string
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
			queryColumns: []string{"kid", "is_current", "activates_at"},
			queryRows: [][]driver.Value{
				{"kid-current", true, now},
				{"kid-target", false, now},
			},
			execErr:         context.Canceled,
			recordedQueries: &queries,
		})

		_, err := repo.SetCurrent(context.Background(), id.NewKeyID("kid-target"), time.Now())
		require.Error(t, err)
		require.NotEmpty(t, queries)
		assert.Contains(t, queries[0], "ORDER BY kid")
		assert.Contains(t, queries[0], "FOR UPDATE")
	})

	t.Run("Delete locks active keys in kid order before deletion", func(t *testing.T) {
		var queries []string
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
			queryColumns: []string{"kid", "is_current", "activates_at"},
			queryRows: [][]driver.Value{
				{"kid-delete", false, now},
				{"kid-other", true, now},
			},
			rowsAffected:    1,
			recordedQueries: &queries,
		})

		err := repo.Delete(context.Background(), id.NewKeyID("kid-delete"))
		require.NoError(t, err)
		require.NotEmpty(t, queries)
		assert.Contains(t, queries[0], "ORDER BY kid")
		assert.Contains(t, queries[0], "FOR UPDATE")
	})
}

func TestSigningKeyRepo_DeleteRejectsRemovingCurrentlyUsableFallbackKey(t *testing.T) {
	now := time.Now().UTC()
	repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
		queryColumns: []string{"kid", "is_current", "activates_at"},
		queryRows: [][]driver.Value{
			{"kid-fallback", false, now.Add(-time.Minute)},
			{"kid-future-current", true, now.Add(time.Hour)},
		},
	})

	err := repo.Delete(context.Background(), id.NewKeyID("kid-fallback"))
	require.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)
}

func TestSigningKeyRepo_WithBootstrapLock(t *testing.T) {
	t.Run("executes callback", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{})

		called := false
		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error {
			called = true
			return nil
		})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("maps begin deadline exceeded to timeout", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{beginErr: context.DeadlineExceeded})

		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error { return nil })
		require.Error(t, err)
		assertStorageErrorKind(t, err, "SigningKeyRepo.WithBootstrapLock", storage.ErrorKindTimeout)
	})

	t.Run("maps lock acquisition deadline exceeded to timeout", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{execErr: context.DeadlineExceeded})

		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error { return nil })
		require.Error(t, err)
		assertStorageErrorKind(t, err, "SigningKeyRepo.WithBootstrapLock", storage.ErrorKindTimeout)
	})

	t.Run("maps lock acquisition context canceled to timeout", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{execErr: context.Canceled})

		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error { return nil })
		require.Error(t, err)
		assertStorageErrorKind(t, err, "SigningKeyRepo.WithBootstrapLock", storage.ErrorKindTimeout)
	})

	t.Run("maps commit error to connection", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{commitErr: errors.New("commit failed")})

		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error { return nil })
		require.Error(t, err)
		assertStorageErrorKind(t, err, "SigningKeyRepo.WithBootstrapLock", storage.ErrorKindConnection)
	})

	t.Run("propagates callback error", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{})
		wantErr := errors.New("callback failed")

		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error { return wantErr })
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("allows bootstrap work to outlive generic write timeout", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepoWithTimeouts(t, signingKeyRepoTestConfig{}, ports.StorageTimeouts{
			Read:  time.Second,
			Write: 10 * time.Millisecond,
		})

		err := repo.WithBootstrapLock(context.Background(), func(context.Context) error {
			time.Sleep(25 * time.Millisecond)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("uses caller context deadline without a repository-specific timeout", func(t *testing.T) {
		repo := newUnitTestSigningKeyRepoWithTimeouts(t, signingKeyRepoTestConfig{}, ports.StorageTimeouts{
			Read:  time.Second,
			Write: time.Second,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := repo.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
			select {
			case <-lockCtx.Done():
				return lockCtx.Err()
			case <-time.After(50 * time.Millisecond):
				return errors.New("bootstrap context did not use caller timeout")
			}
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
