package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ ports.AuthorizationSessionCoordinator = (*AuthorizationSessionCoordinator)(nil)
var _ ports.AuthorizationClock = (*AuthorizationSessionCoordinator)(nil)

type authorizationScopeKey struct{}

type authorizationScope struct {
	agentID      id.AgentID
	decisionTime time.Time
	latestTime   time.Time
	rollbackOnly atomic.Bool
}

func scopedAuthorization(ctx context.Context) (*authorizationScope, bool) {
	scope, ok := ctx.Value(authorizationScopeKey{}).(*authorizationScope)
	return scope, ok
}

func requireAuthorizationAgent(ctx context.Context, agentID id.AgentID) error {
	scope, ok := scopedAuthorization(ctx)
	if !ok || agentID.IsZero() || scope.agentID != agentID {
		return storage.NewStorageError("AuthorizationSession", storage.ErrorKindValidation, nil, "agent-scoped authorization transaction required")
	}
	return nil
}

type AuthorizationSessionCoordinator struct {
	adapter *Adapter
}

func NewAuthorizationSessionCoordinator(adapter *Adapter) *AuthorizationSessionCoordinator {
	return &AuthorizationSessionCoordinator{adapter: adapter}
}

func (c *AuthorizationSessionCoordinator) Now(ctx context.Context) (time.Time, error) {
	if c.adapter == nil || c.adapter.db == nil {
		return time.Time{}, storage.NewStorageError("AuthorizationClock.Now", storage.ErrorKindConnection, nil, "database not initialized")
	}
	var now time.Time
	if err := c.adapter.storageExecutor(ctx).QueryRowxContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return time.Time{}, storage.NewStorageError("AuthorizationClock.Now", storage.ErrorKindConnection, err, "failed to read database clock")
	}
	now = now.UTC()
	if scope, ok := scopedAuthorization(ctx); ok {
		scope.latestTime = now
	}
	return now, nil
}

func (c *AuthorizationSessionCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) (retErr error) {
	if c.adapter == nil || c.adapter.db == nil {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if agentID.IsZero() || operation == nil {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindValidation, nil, "agent and operation required")
	}
	if _, nested := scopedAuthorization(ctx); nested {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindValidation, nil, "authorization scope is non-reentrant")
	}
	if _, nested := storageTransaction(ctx); nested {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindValidation, nil, "authorization scope cannot borrow another transaction")
	}
	tx, err := c.adapter.db.BeginTxx(ctx, nil)
	if err != nil {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindConnection, err, "failed to begin authorization transaction")
	}
	committing := false
	defer func() {
		if !committing {
			if err := tx.Rollback(); err != nil {
				retErr = storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindConnection,
					errors.Join(retErr, err), "failed to confirm authorization rollback")
			}
		}
	}()
	var locked id.AgentID
	err = tx.QueryRowxContext(ctx, `SELECT id FROM agents WHERE id = $1 FOR UPDATE`, agentID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	if err != nil {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindConnection, err, "failed to lock agent")
	}
	scope := &authorizationScope{agentID: agentID}
	work := context.WithValue(context.WithValue(ctx, storageTransactionKey{}, tx), authorizationScopeKey{}, scope)
	at, err := c.Now(work)
	if err != nil {
		return err
	}
	scope.decisionTime = at
	if err := operation(work, at); err != nil {
		return err
	}
	if scope.rollbackOnly.Load() {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindValidation, nil, "borrowed transaction was rolled back")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	committing = true
	return commitAuthorizationTx(tx)
}

func commitAuthorizationTx(tx interface{ Commit() error }) error {
	if err := tx.Commit(); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) || errors.Is(err, sql.ErrTxDone) {
			return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindUnknown, err, "authorization transaction rejected")
		}
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindConnection,
			fmt.Errorf("%w: %w", ports.ErrCommitIndeterminate, err), "authorization commit acknowledgement lost")
	}
	return nil
}
