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

func (r *SigningKeyRepo) lockActiveSigningKeysInDomain(ctx context.Context, tx *sqlx.Tx, domain storage.KeyDomain) ([]lockedSigningKeyRow, error) {
	var rows []lockedSigningKeyRow
	err := tx.SelectContext(ctx, &rows,
		`SELECT kid, is_current, activates_at
		 FROM signing_keys
		 WHERE key_domain = $1 AND removed_at IS NULL
		 ORDER BY kid
		 FOR UPDATE`, domain)
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
	if err := key.KeyDomain.Validate(); err != nil {
		return storage.NewStorageError("SigningKeyRepo.Create", storage.ErrorKindValidation, err, "key_domain is invalid")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err := r.adapter.db.ExecContext(execCtx,
		`INSERT INTO signing_keys (id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at, removed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		key.ID, key.KID, key.KeyDomain, key.Algorithm, key.PrivateKeyEncrypted,
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
	if err := key.KeyDomain.Validate(); err != nil {
		return storage.NewStorageError("SigningKeyRepo.CreateAndSetCurrent", storage.ErrorKindValidation, err, "key_domain is invalid")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTxx(execCtx, nil)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.CreateAndSetCurrent", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(execCtx, `UPDATE signing_keys SET is_current = false WHERE key_domain = $1 AND is_current = true`, key.KeyDomain)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.CreateAndSetCurrent", err, "failed to demote existing keys")
	}

	_, err = tx.ExecContext(execCtx,
		`INSERT INTO signing_keys (id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at, removed_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8)`,
		key.ID, key.KID, key.KeyDomain, key.Algorithm, key.PrivateKeyEncrypted,
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

func (r *SigningKeyRepo) GetByKIDInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID) (*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.GetByKIDInDomain", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var key storage.SigningKey
	err := r.adapter.db.GetContext(queryCtx, &key,
		`SELECT id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at, removed_at
		 FROM signing_keys WHERE kid = $1 AND key_domain = $2 AND removed_at IS NULL`, kid, domain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetByKIDInDomain", storage.ErrorKindNotFound, err, "signing key not found")
		}
		if isContextTimeoutOrCanceled(err) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetByKIDInDomain", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("SigningKeyRepo.GetByKIDInDomain", storage.ErrorKindConnection, err, "failed to query signing key")
	}
	return &key, nil
}

// GetCurrent returns the signing key to use for token issuance. It prefers the key flagged
// as is_current provided its activates_at has passed. If the current key is still in its
// grace period, it falls back to the most recently activated key, keeping token issuance
// uninterrupted while JWKS caches learn about the new key.

func (r *SigningKeyRepo) GetCurrentInDomain(ctx context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.GetCurrentInDomain", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var key storage.SigningKey
	err := r.adapter.db.GetContext(queryCtx, &key,
		`SELECT id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at, removed_at
		 FROM signing_keys
		 WHERE key_domain = $1 AND removed_at IS NULL AND activates_at <= NOW()
		 ORDER BY
		   CASE WHEN is_current THEN 0 ELSE 1 END,
		   activates_at DESC
		 LIMIT 1`, domain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetCurrentInDomain", storage.ErrorKindNotFound, err, "no current signing key")
		}
		if isContextTimeoutOrCanceled(err) {
			return nil, storage.NewStorageError("SigningKeyRepo.GetCurrentInDomain", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("SigningKeyRepo.GetCurrentInDomain", storage.ErrorKindConnection, err, "failed to query current signing key")
	}
	return &key, nil
}

func (r *SigningKeyRepo) ListActiveInDomain(ctx context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.ListActiveInDomain", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var keys []*storage.SigningKey
	err := r.adapter.db.SelectContext(queryCtx, &keys,
		`SELECT id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at, removed_at
		 FROM signing_keys WHERE key_domain = $1 AND removed_at IS NULL ORDER BY created_at DESC`, domain)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.ListActiveInDomain", err, "failed to list signing keys")
	}
	return keys, nil
}

// SetCurrent promotes a key to be the current signing key using the domain-supplied
// activation timestamp.

func (r *SigningKeyRepo) SetCurrentInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("SigningKeyRepo.SetCurrentInDomain", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTxx(execCtx, nil)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrentInDomain", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	lockedRows, err := r.lockActiveSigningKeysInDomain(execCtx, tx, domain)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrentInDomain", err, "failed to lock active signing keys")
	}

	foundTarget := false
	for _, row := range lockedRows {
		if row.KID == kid {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		return nil, storage.NewStorageError("SigningKeyRepo.SetCurrentInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
	}

	_, err = tx.ExecContext(execCtx, `UPDATE signing_keys SET is_current = false WHERE key_domain = $1 AND is_current = true`, domain)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrentInDomain", err, "failed to demote keys")
	}

	var key storage.SigningKey
	err = tx.GetContext(execCtx, &key,
		`UPDATE signing_keys
		 SET is_current = true, activates_at = $3
		 WHERE kid = $2 AND key_domain = $1 AND removed_at IS NULL
		 RETURNING id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at, removed_at`, domain, kid, activatesAt)
	if err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrentInDomain", err, "failed to promote key")
	}

	if err := tx.Commit(); err != nil {
		return nil, classifySigningKeyRepoError("SigningKeyRepo.SetCurrentInDomain", err, "failed to commit transaction")
	}
	return &key, nil
}

func (r *SigningKeyRepo) DeleteInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("SigningKeyRepo.DeleteInDomain", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTxx(execCtx, nil)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.DeleteInDomain", err, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	lockedRows, err := r.lockActiveSigningKeysInDomain(execCtx, tx, domain)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.DeleteInDomain", err, "failed to lock active signing keys")
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
		return storage.NewStorageError("SigningKeyRepo.DeleteInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
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
		`UPDATE signing_keys SET removed_at = $3 WHERE kid = $2 AND key_domain = $1 AND removed_at IS NULL`, domain, kid, now)
	if err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.DeleteInDomain", err, "failed to delete signing key")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError("SigningKeyRepo.DeleteInDomain", storage.ErrorKindUnknown, err, "failed to determine rows affected")
	}
	if rowsAffected != 1 {
		return storage.NewStorageError("SigningKeyRepo.DeleteInDomain", storage.ErrorKindUnknown, nil, "invariant violation: locked signing key was not updated")
	}
	if err := tx.Commit(); err != nil {
		return classifySigningKeyRepoError("SigningKeyRepo.DeleteInDomain", err, "failed to commit transaction")
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

func (r *SigningKeyRepo) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("SigningKeyRepo.CountActiveInDomain", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var count int
	err := r.adapter.db.GetContext(queryCtx, &count,
		`SELECT COUNT(*) FROM signing_keys WHERE key_domain = $1 AND removed_at IS NULL`, domain)
	if err != nil {
		if isContextTimeoutOrCanceled(err) {
			return 0, storage.NewStorageError("SigningKeyRepo.CountActiveInDomain", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return 0, storage.NewStorageError("SigningKeyRepo.CountActiveInDomain", storage.ErrorKindConnection, err, "failed to count signing keys")
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
