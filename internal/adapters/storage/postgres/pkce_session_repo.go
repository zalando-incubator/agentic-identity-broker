package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface check
var _ ports.PKCESessionRepository = (*PKCESessionRepo)(nil)

// PKCESessionRepo implements PKCESessionRepository using PostgreSQL.
type PKCESessionRepo struct {
	adapter *Adapter
}

// NewPKCESessionRepo creates a new PostgreSQL PKCE session repository.
func NewPKCESessionRepo(adapter *Adapter) *PKCESessionRepo {
	return &PKCESessionRepo{adapter: adapter}
}

func (r *PKCESessionRepo) Create(ctx context.Context, session *storage.PKCESession) (err error) {
	ctx, endSpan := startStorageSpan(ctx, "storage.create.pkce_session", "PKCESessionRepo.Create")
	defer func() { endSpan(err) }()

	if r.adapter.db == nil {
		return storage.NewStorageError("PKCESessionRepo.Create", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err = r.adapter.db.ExecContext(execCtx,
		`INSERT INTO pkce_sessions (signature, code_challenge, code_challenge_method, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		session.Signature, session.CodeChallenge, session.CodeChallengeMethod,
		session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return storage.NewStorageError("PKCESessionRepo.Create", storage.ErrorKindUnknown, err, "failed to create PKCE session")
	}
	return nil
}

func (r *PKCESessionRepo) FindBySignature(ctx context.Context, signature string) (_ *storage.PKCESession, err error) {
	ctx, endSpan := startStorageSpan(ctx, "storage.find.pkce_session", "PKCESessionRepo.FindBySignature")
	defer func() { endSpan(err) }()

	if r.adapter.db == nil {
		return nil, storage.NewStorageError("PKCESessionRepo.FindBySignature", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var session storage.PKCESession
	err = r.adapter.db.GetContext(queryCtx, &session,
		`SELECT signature, code_challenge, code_challenge_method, expires_at, created_at
		 FROM pkce_sessions WHERE signature = $1`, signature)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("PKCESessionRepo.FindBySignature", storage.ErrorKindNotFound, err, "PKCE session not found")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, storage.NewStorageError("PKCESessionRepo.FindBySignature", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("PKCESessionRepo.FindBySignature", storage.ErrorKindConnection, err, "failed to query PKCE session")
	}
	return &session, nil
}

func (r *PKCESessionRepo) Delete(ctx context.Context, signature string) (err error) {
	ctx, endSpan := startStorageSpan(ctx, "storage.delete.pkce_session", "PKCESessionRepo.Delete")
	defer func() { endSpan(err) }()

	if r.adapter.db == nil {
		return storage.NewStorageError("PKCESessionRepo.Delete", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	result, err := r.adapter.db.ExecContext(execCtx,
		`DELETE FROM pkce_sessions WHERE signature = $1`, signature)
	if err != nil {
		return storage.NewStorageError("PKCESessionRepo.Delete", storage.ErrorKindUnknown, err, "failed to delete PKCE session")
	}
	return checkRowsAffected("PKCESessionRepo.Delete", result, "PKCE session not found")
}

func (r *PKCESessionRepo) DeleteExpired(ctx context.Context) (_ int, err error) {
	ctx, endSpan := startStorageSpan(ctx, "storage.delete_expired.pkce_session", "PKCESessionRepo.DeleteExpired")
	defer func() { endSpan(err) }()

	if r.adapter.db == nil {
		return 0, storage.NewStorageError("PKCESessionRepo.DeleteExpired", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	now := time.Now()
	result, err := r.adapter.db.ExecContext(execCtx,
		`DELETE FROM pkce_sessions WHERE expires_at < $1`, now)
	if err != nil {
		return 0, storage.NewStorageError("PKCESessionRepo.DeleteExpired", storage.ErrorKindUnknown, err, "failed to delete expired PKCE sessions")
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}
