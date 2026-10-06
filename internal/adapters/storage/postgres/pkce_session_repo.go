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
	"github.com/jmoiron/sqlx"
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

func (r *PKCESessionRepo) checkScopedCode(ctx context.Context, signature string, creating bool) error {
	scope, ok := scopedAuthorization(ctx)
	if !ok {
		return nil
	}
	var agentID id.AgentID
	err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
		`SELECT agent_id FROM authorization_codes WHERE code_hash = $1`, signature).Scan(&agentID)
	if errors.Is(err, sql.ErrNoRows) && creating {
		return nil // Fosite can create PKCE before it writes the authorization code.
	}
	if err != nil {
		return refreshStoreError("PKCESessionRepo.AuthorizationOwner", err)
	}
	if scope.agentID != agentID {
		return refreshValidation("PKCESessionRepo.AuthorizationOwner", errors.New("PKCE code belongs to another agent"))
	}
	return nil
}

func (r *PKCESessionRepo) Create(ctx context.Context, session *storage.PKCESession) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("PKCESessionRepo.Create", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if session == nil {
		return refreshValidation("PKCESessionRepo.Create", errors.New("PKCE session required"))
	}
	if err := r.checkScopedCode(ctx, session.Signature, true); err != nil {
		return err
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
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

func (r *PKCESessionRepo) FindBySignature(ctx context.Context, signature string) (*storage.PKCESession, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("PKCESessionRepo.FindBySignature", storage.ErrorKindConnection, nil, "database not initialized")
	}

	if err := r.checkScopedCode(ctx, signature, false); err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var session storage.PKCESession
	err := sqlx.GetContext(queryCtx, r.adapter.storageExecutor(queryCtx), &session,
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

func (r *PKCESessionRepo) Delete(ctx context.Context, signature string) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("PKCESessionRepo.Delete", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if err := r.checkScopedCode(ctx, signature, false); err != nil {
		return err
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	result, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`DELETE FROM pkce_sessions WHERE signature = $1`, signature)
	if err != nil {
		return storage.NewStorageError("PKCESessionRepo.Delete", storage.ErrorKindUnknown, err, "failed to delete PKCE session")
	}
	return checkRowsAffected("PKCESessionRepo.Delete", result, "PKCE session not found")
}

func (r *PKCESessionRepo) DeleteExpired(ctx context.Context) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("PKCESessionRepo.DeleteExpired", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if _, scoped := scopedAuthorization(ctx); scoped {
		return 0, refreshValidation("PKCESessionRepo.DeleteExpired", errors.New("global cleanup cannot run inside an agent scope"))
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	now := time.Now()
	result, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`DELETE FROM pkce_sessions WHERE expires_at < $1`, now)
	if err != nil {
		return 0, storage.NewStorageError("PKCESessionRepo.DeleteExpired", storage.ErrorKindUnknown, err, "failed to delete expired PKCE sessions")
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}
