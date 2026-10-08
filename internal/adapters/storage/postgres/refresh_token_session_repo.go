package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface check
var _ ports.RefreshTokenSessionRepository = (*RefreshTokenSessionRepo)(nil)

// RefreshTokenSessionRepo implements RefreshTokenSessionRepository using PostgreSQL.
type RefreshTokenSessionRepo struct {
	adapter *Adapter
}

// NewRefreshTokenSessionRepo creates a new PostgreSQL refresh token session repository.
func NewRefreshTokenSessionRepo(adapter *Adapter) *RefreshTokenSessionRepo {
	return &RefreshTokenSessionRepo{adapter: adapter}
}

func (r *RefreshTokenSessionRepo) Create(ctx context.Context, session *storage.RefreshTokenSession) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.Create", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`INSERT INTO refresh_token_sessions (signature, request_id, agent_id, client_id, principal, scope, expires_at, used_at, created_at, email, display_name, grant_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		session.Signature, session.RequestID, session.AgentID, session.ClientID, session.Principal,
		session.Scope, session.ExpiresAt, session.UsedAt, session.CreatedAt, session.Email, session.DisplayName, session.GrantID,
	)
	if err != nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.Create", storage.ErrorKindUnknown, err, "failed to create refresh token session")
	}
	return nil
}

func (r *RefreshTokenSessionRepo) FindBySignature(ctx context.Context, signature string) (*storage.RefreshTokenSession, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("RefreshTokenSessionRepo.FindBySignature", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var session storage.RefreshTokenSession
	err := r.adapter.db.GetContext(queryCtx, &session,
		`SELECT signature, request_id, agent_id, client_id, principal, scope, expires_at, used_at, created_at, email, display_name, grant_id
		 FROM refresh_token_sessions WHERE signature = $1`, signature)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("RefreshTokenSessionRepo.FindBySignature", storage.ErrorKindNotFound, err, "refresh token session not found")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, storage.NewStorageError("RefreshTokenSessionRepo.FindBySignature", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("RefreshTokenSessionRepo.FindBySignature", storage.ErrorKindConnection, err, "failed to query refresh token session")
	}
	return &session, nil
}

func (r *RefreshTokenSessionRepo) MarkUsed(ctx context.Context, signature string) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.MarkUsed", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var locked int
	err := sqlx.GetContext(execCtx, r.adapter.storageExecutor(execCtx), &locked,
		`SELECT 1 FROM agents WHERE id = (SELECT agent_id FROM refresh_token_sessions WHERE signature = $1) FOR KEY SHARE`, signature)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.NewStorageError("RefreshTokenSessionRepo.MarkUsed", storage.ErrorKindNotFound, err, "refresh token session not found or already used")
	}
	if err != nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.MarkUsed", storage.ErrorKindUnknown, err, "failed to lock refresh token agent")
	}

	now := time.Now()
	result, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`UPDATE refresh_token_sessions SET used_at = $1 WHERE signature = $2 AND used_at IS NULL`, now, signature)
	if err != nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.MarkUsed", storage.ErrorKindUnknown, err, "failed to mark refresh token session as used")
	}
	return checkRowsAffected("RefreshTokenSessionRepo.MarkUsed", result, "refresh token session not found or already used")
}

func (r *RefreshTokenSessionRepo) RevokeByRequestID(ctx context.Context, requestID string) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.RevokeByRequestID", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	now := time.Now()
	_, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`UPDATE refresh_token_sessions SET used_at = $1 WHERE request_id = $2 AND used_at IS NULL`, now, requestID)
	if err != nil {
		return storage.NewStorageError("RefreshTokenSessionRepo.RevokeByRequestID", storage.ErrorKindUnknown, err, "failed to revoke refresh token sessions")
	}
	return nil
}

func (r *RefreshTokenSessionRepo) RevokeByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error) {
	return r.revokeByAgent(ctx, agentID, &principal)
}

func (r *RefreshTokenSessionRepo) RevokeByAgent(ctx context.Context, agentID id.AgentID) (int, error) {
	return r.revokeByAgent(ctx, agentID, nil)
}

func (r *RefreshTokenSessionRepo) revokeByAgent(ctx context.Context, agentID id.AgentID, principal *id.Principal) (int, error) {
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	txCtx, err := r.adapter.BeginTX(execCtx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.adapter.Rollback(txCtx) }()
	const operation = "RefreshTokenSessionRepo.Revoke"
	var locked int
	err = sqlx.GetContext(txCtx, r.adapter.storageExecutor(txCtx), &locked, `SELECT 1 FROM agents WHERE id = $1 FOR UPDATE`, agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "failed to lock refresh token agent")
	}
	result, err := r.adapter.storageExecutor(txCtx).ExecContext(txCtx,
		`UPDATE refresh_token_sessions SET used_at = $1 WHERE agent_id = $2 AND used_at IS NULL AND ($3::text IS NULL OR principal = $3)`,
		time.Now(), agentID, principal)
	if err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "failed to revoke refresh token sessions")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "failed to determine rows affected")
	}
	if err := r.adapter.Commit(txCtx); err != nil {
		return 0, err
	}
	return int(rows), nil
}

func (r *RefreshTokenSessionRepo) DeleteExpired(ctx context.Context) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("RefreshTokenSessionRepo.DeleteExpired", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	now := time.Now()
	result, err := r.adapter.db.ExecContext(execCtx,
		`DELETE FROM refresh_token_sessions WHERE expires_at < $1`, now)
	if err != nil {
		return 0, storage.NewStorageError("RefreshTokenSessionRepo.DeleteExpired", storage.ErrorKindUnknown, err, "failed to delete expired refresh token sessions")
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}
