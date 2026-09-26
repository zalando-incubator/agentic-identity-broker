package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface checks
var _ ports.SigningKeyRepository = (*SigningKeyRepo)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*SigningKeyRepo)(nil)

// SigningKeyRepo implements SigningKeyRepository using PostgreSQL.
type SigningKeyRepo struct {
	adapter *Adapter
}

// signingKeyBootstrapLockID is the advisory lock key guarding initial signing key bootstrap.
const signingKeyBootstrapLockID int64 = 0x53494b4253545250

type lockedSigningKeyRow struct {
	KID         id.KeyID  `db:"kid"`
	IsCurrent   bool      `db:"is_current"`
	ActivatesAt time.Time `db:"activates_at"`
}

// NewSigningKeyRepo creates a new PostgreSQL signing key repository.
func NewSigningKeyRepo(adapter *Adapter) *SigningKeyRepo {
	return &SigningKeyRepo{adapter: adapter}
}

func (r *SigningKeyRepo) lockActiveSigningKeys(ctx context.Context, tx *sqlx.Tx) ([]lockedSigningKeyRow, error) {
	var rows []lockedSigningKeyRow
	err := tx.SelectContext(ctx, &rows,
		`SELECT kid, is_current, activates_at
		 FROM signing_keys
		 WHERE removed_at IS NULL
		 ORDER BY kid
		 FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func currentUsableLockedSigningKey(rows []lockedSigningKeyRow, now time.Time) *lockedSigningKeyRow {
	var best *lockedSigningKeyRow
	for i := range rows {
		row := &rows[i]
		if row.ActivatesAt.After(now) {
			continue
		}
		if best == nil || (!best.IsCurrent && row.IsCurrent) || (best.IsCurrent == row.IsCurrent && row.ActivatesAt.After(best.ActivatesAt)) {
			best = row
		}
	}
	return best
}

func (r *SigningKeyRepo) Create(ctx context.Context, key *storage.SigningKey) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("SigningKeyRepo.Create", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err := r.adapter.db.ExecContext(execCtx,
		`INSERT INTO signing_keys (id, kid, algorithm, private_key_encrypted, public_jwk, is_current, activates_at, created_at, removed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		key.ID, key.KID, key.Algorithm, key.PrivateKeyEncrypted, key.PublicJWK,
		key.IsCurrent, key.ActivatesAt, key.CreatedAt, key.RemovedAt,
	)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.Create", err, "failed to create signing key")
	}
	return nil
}

func (r *SigningKeyRepo) CreateAndSetCurrent(ctx context.Context, key *storage.SigningKey) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("SigningKeyRepo.CreateAndSetCurrent", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTxx(execCtx, nil)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.CreateAndSetCurrent", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	// Demote all existing current keys before inserting the new one.
	_, err = tx.ExecContext(execCtx, `UPDATE signing_keys SET is_current = false WHERE is_current = true`)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.CreateAndSetCurrent", err, "failed to demote existing keys")
	}

	_, err = tx.ExecContext(execCtx,
		`INSERT INTO signing_keys (id, kid, algorithm, private_key_encrypted, public_jwk, is_current, activates_at, created_at, removed_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8)`,
		key.ID, key.KID, key.Algorithm, key.PrivateKeyEncrypted, key.PublicJWK,
		key.ActivatesAt, key.CreatedAt, key.RemovedAt,
	)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.CreateAndSetCurrent", err, "failed to insert signing key")
	}

	if err := tx.Commit(); err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.CreateAndSetCurrent", err, "failed to commit transaction")
	}
	return nil
}

func (r *SigningKeyRepo) GetByKID(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.GetByKID", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var key storage.SigningKey
	err := r.adapter.db.GetContext(queryCtx, &key,
		`SELECT id, kid, algorithm, private_key_encrypted, public_jwk, is_current, activates_at, created_at, removed_at
		 FROM signing_keys WHERE kid = $1 AND removed_at IS NULL`, kid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetByKID", storage.ErrorKindNotFound, err, "signing key not found")
		}
		if isContextTimeoutOrCanceled(err) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetByKID", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("SigningKeyRepo.GetByKID", storage.ErrorKindConnection, err, "failed to query signing key")
	}
	return &key, nil
}

// GetCurrent returns the signing key to use for token issuance. It prefers the key flagged
// as is_current provided its activates_at has passed. If the current key is still in its
// grace period, it falls back to the most recently activated key, keeping token issuance
// uninterrupted while JWKS caches learn about the new key.
func (r *SigningKeyRepo) GetCurrent(ctx context.Context) (*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.GetCurrent", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var key storage.SigningKey
	err := r.adapter.db.GetContext(queryCtx, &key,
		`SELECT id, kid, algorithm, private_key_encrypted, public_jwk, is_current, activates_at, created_at, removed_at
		 FROM signing_keys
		 WHERE removed_at IS NULL AND activates_at <= NOW()
		 ORDER BY
		   CASE WHEN is_current THEN 0 ELSE 1 END,
		   activates_at DESC
		 LIMIT 1`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetCurrent", storage.ErrorKindNotFound, err, "no current signing key")
		}
		if isContextTimeoutOrCanceled(err) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetCurrent", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("SigningKeyRepo.GetCurrent", storage.ErrorKindConnection, err, "failed to query current signing key")
	}
	return &key, nil
}

func (r *SigningKeyRepo) ListActive(ctx context.Context) ([]*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.ListActive", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var keys []*storage.SigningKey
	err := r.adapter.db.SelectContext(queryCtx, &keys,
		`SELECT id, kid, algorithm, private_key_encrypted, public_jwk, is_current, activates_at, created_at, removed_at
		 FROM signing_keys WHERE removed_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.ListActive", err, "failed to list signing keys")
	}
	return keys, nil
}

// SetCurrent promotes a key to be the current signing key using the domain-supplied
// activation timestamp.
func (r *SigningKeyRepo) SetCurrent(ctx context.Context, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.SetCurrent", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTxx(execCtx, nil)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrent", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	lockedRows, err := r.lockActiveSigningKeys(execCtx, tx)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrent", err, "failed to lock active signing keys")
	}

	foundTarget := false
	for _, row := range lockedRows {
		if row.KID == kid {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		return nil, storage.NewStorageError("SigningKeyRepo.SetCurrent", storage.ErrorKindNotFound, nil, "signing key not found")
	}

	// Demote all current keys.
	_, err = tx.ExecContext(execCtx, `UPDATE signing_keys SET is_current = false WHERE is_current = true`)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrent", err, "failed to demote keys")
	}

	// Promote the target key using the activation timestamp chosen by the domain service.
	var key storage.SigningKey
	err = tx.GetContext(execCtx, &key,
		`UPDATE signing_keys
		 SET is_current = true, activates_at = $2
		 WHERE kid = $1 AND removed_at IS NULL
		 RETURNING id, kid, algorithm, private_key_encrypted, public_jwk, is_current, activates_at, created_at, removed_at`, kid, activatesAt)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrent", err, "failed to promote key")
	}

	if err := tx.Commit(); err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrent", err, "failed to commit transaction")
	}
	return &key, nil
}

func (r *SigningKeyRepo) SetPublicJWK(ctx context.Context, kid id.KeyID, publicJWK []byte) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("SigningKeyRepo.SetPublicJWK", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	result, err := r.adapter.db.ExecContext(execCtx,
		`UPDATE signing_keys
		 SET public_jwk = $2
		 WHERE kid = $1 AND removed_at IS NULL AND public_jwk IS NULL`, kid, publicJWK)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.SetPublicJWK", err, "failed to backfill signing key public JWK")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError("SigningKeyRepo.SetPublicJWK", storage.ErrorKindUnknown, err, "failed to determine rows affected")
	}
	if rowsAffected != 0 {
		return nil
	}

	var exists bool
	err = r.adapter.db.GetContext(execCtx, &exists,
		`SELECT EXISTS(SELECT 1 FROM signing_keys WHERE kid = $1 AND removed_at IS NULL)`, kid)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.SetPublicJWK", err, "failed to query signing key")
	}
	if !exists {
		return storage.NewStorageError("SigningKeyRepo.SetPublicJWK", storage.ErrorKindNotFound, nil, "signing key not found")
	}
	return nil
}

func (r *SigningKeyRepo) Delete(ctx context.Context, kid id.KeyID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("SigningKeyRepo.Delete", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTxx(execCtx, nil)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.Delete", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	lockedRows, err := r.lockActiveSigningKeys(execCtx, tx)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.Delete", err, "failed to lock active signing keys")
	}

	activeCount := len(lockedRows)
	foundTarget := false
	targetIsCurrent := false
	for _, row := range lockedRows {
		if row.KID == kid {
			foundTarget = true
			targetIsCurrent = row.IsCurrent
			break
		}
	}

	if !foundTarget {
		return storage.NewStorageError("SigningKeyRepo.Delete", storage.ErrorKindNotFound, nil, "signing key not found")
	}
	if activeCount <= 1 {
		return ports.ErrLastActiveKey
	}
	if targetIsCurrent {
		return ports.ErrCurrentKey
	}

	now := time.Now()
	if current := currentUsableLockedSigningKey(lockedRows, now); current != nil && current.KID == kid {
		return ports.ErrEffectiveCurrentKey
	}
	result, err := tx.ExecContext(execCtx,
		`UPDATE signing_keys SET removed_at = $1 WHERE kid = $2 AND removed_at IS NULL`, now, kid)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.Delete", err, "failed to delete signing key")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError("SigningKeyRepo.Delete", storage.ErrorKindUnknown, err, "failed to determine rows affected")
	}
	if rowsAffected != 1 {
		return storage.NewStorageError("SigningKeyRepo.Delete", storage.ErrorKindUnknown, nil, "invariant violation: locked signing key was not updated")
	}
	if err := tx.Commit(); err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.Delete", err, "failed to commit transaction")
	}
	return nil
}

func (r *SigningKeyRepo) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("SigningKeyRepo.WithBootstrapLock", storage.ErrorKindConnection, nil, "database not initialized")
	}

	conn, err := r.adapter.db.Connx(ctx)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.WithBootstrapLock", err, "failed to acquire database connection")
	}
	defer conn.Close() //nolint:errcheck

	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.WithBootstrapLock", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, signingKeyBootstrapLockID); err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.WithBootstrapLock", err, "failed to acquire signing key bootstrap lock")
	}

	if err := fn(ctx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.WithBootstrapLock", err, "failed to commit transaction")
	}
	return nil
}

func (r *SigningKeyRepo) CountActive(ctx context.Context) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("SigningKeyRepo.CountActive", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var count int
	err := r.adapter.db.GetContext(queryCtx, &count,
		`SELECT COUNT(*) FROM signing_keys WHERE removed_at IS NULL`)
	if err != nil {
		if isContextTimeoutOrCanceled(err) {
			return 0, storage.NewStorageError("SigningKeyRepo.CountActive", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return 0, storage.NewStorageError("SigningKeyRepo.CountActive", storage.ErrorKindConnection, err, "failed to count signing keys")
	}
	return count, nil
}

func classifySigningKeyRepoError(operation string, err error, message string) error {
	if isContextTimeoutOrCanceled(err) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation exceeded timeout")
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
		return storage.NewStorageError(operation, storage.ErrorKindNotFound, err, "signing key not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return storage.NewStorageError(operation, storage.ErrorKindConflict, err, "signing key already exists")
		case "40001":
			return storage.NewStorageError(operation, storage.ErrorKindConflict, err, "transaction serialization failure")
		}
	}
	return storage.NewStorageError(operation, storage.ErrorKindConnection, err, message)
}

func isContextTimeoutOrCanceled(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
