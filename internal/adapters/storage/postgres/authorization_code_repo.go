package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface check
var _ ports.AuthorizationCodeRepository = (*AuthorizationCodeRepo)(nil)

// AuthorizationCodeRepo implements AuthorizationCodeRepository using PostgreSQL.
type AuthorizationCodeRepo struct {
	adapter *Adapter
}

// NewAuthorizationCodeRepo creates a new PostgreSQL authorization code repository.
func NewAuthorizationCodeRepo(adapter *Adapter) *AuthorizationCodeRepo {
	return &AuthorizationCodeRepo{adapter: adapter}
}

func (r *AuthorizationCodeRepo) Create(ctx context.Context, code *storage.AuthorizationCode) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("AuthorizationCodeRepo.Create", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err := r.adapter.oauth2Executor(execCtx).ExecContext(execCtx,
		`INSERT INTO authorization_codes (id, code_hash, agent_id, client_id, principal, redirect_uri, code_challenge, scope, expires_at, used_at, created_at, email, display_name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		code.ID, code.CodeHash, code.AgentID, code.ClientID, code.Principal,
		code.RedirectURI, code.CodeChallenge, code.Scope,
		code.ExpiresAt, code.UsedAt, code.CreatedAt, code.Email, code.DisplayName,
	)
	if err != nil {
		return storage.NewStorageError("AuthorizationCodeRepo.Create", storage.ErrorKindUnknown, err, "failed to create authorization code")
	}
	return nil
}

func (r *AuthorizationCodeRepo) FindByCodeHash(ctx context.Context, codeHash string) (*storage.AuthorizationCode, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("AuthorizationCodeRepo.FindByCodeHash", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var code storage.AuthorizationCode
	err := r.adapter.db.GetContext(queryCtx, &code,
		`SELECT id, code_hash, agent_id, client_id, principal, redirect_uri, code_challenge, scope, expires_at, used_at, created_at, email, display_name
		 FROM authorization_codes WHERE code_hash = $1 AND expires_at > NOW()`, codeHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.NewStorageError("AuthorizationCodeRepo.FindByCodeHash", storage.ErrorKindNotFound, err, "authorization code not found")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, storage.NewStorageError("AuthorizationCodeRepo.FindByCodeHash", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("AuthorizationCodeRepo.FindByCodeHash", storage.ErrorKindConnection, err, "failed to query authorization code")
	}
	return &code, nil
}

func (r *AuthorizationCodeRepo) MarkUsed(ctx context.Context, codeID id.AuthorizationCodeID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("AuthorizationCodeRepo.MarkUsed", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	now := time.Now()
	result, err := r.adapter.oauth2Executor(execCtx).ExecContext(execCtx,
		`UPDATE authorization_codes SET used_at = $1 WHERE id = $2 AND used_at IS NULL`, now, codeID)
	if err != nil {
		return storage.NewStorageError("AuthorizationCodeRepo.MarkUsed", storage.ErrorKindUnknown, err, "failed to mark code as used")
	}
	return checkRowsAffected("AuthorizationCodeRepo.MarkUsed", result, "authorization code not found or already used")
}

func (r *AuthorizationCodeRepo) DeleteExpired(ctx context.Context) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("AuthorizationCodeRepo.DeleteExpired", storage.ErrorKindConnection, nil, "database not initialized")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	now := time.Now()
	result, err := r.adapter.db.ExecContext(execCtx,
		`DELETE FROM authorization_codes WHERE expires_at < $1`, now)
	if err != nil {
		return 0, storage.NewStorageError("AuthorizationCodeRepo.DeleteExpired", storage.ErrorKindUnknown, err, "failed to delete expired codes")
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}
