package postgres

import (
	"context"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/jmoiron/sqlx"
)

type storageTransactionKey struct{}
type borrowedTransactionKey struct{}

func (a *Adapter) BeginTX(ctx context.Context) (context.Context, error) {
	if _, owned := scopedAuthorization(ctx); owned {
		if _, ok := storageTransaction(ctx); !ok {
			return nil, fmt.Errorf("StorageTransaction.BeginTX: authorization scope has no transaction")
		}
		return context.WithValue(ctx, borrowedTransactionKey{}, true), nil
	}
	if _, active := storageTransaction(ctx); active {
		return nil, fmt.Errorf("StorageTransaction.BeginTX: transaction already active")
	}
	if a.db == nil {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConnection, nil, "database not initialized")
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	return context.WithValue(ctx, storageTransactionKey{}, tx), nil
}

func (a *Adapter) Commit(ctx context.Context) error {
	if ctx.Value(borrowedTransactionKey{}) == true {
		return nil
	}
	if _, owned := scopedAuthorization(ctx); owned {
		return fmt.Errorf("StorageTransaction.Commit: coordinator owns the transaction")
	}
	tx, ok := storageTransaction(ctx)
	if !ok {
		return fmt.Errorf("StorageTransaction.Commit: transaction missing from context")
	}
	if err := commitAuthorizationTx(tx); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) Rollback(ctx context.Context) error {
	if ctx.Value(borrowedTransactionKey{}) == true {
		if scope, ok := scopedAuthorization(ctx); ok {
			scope.rollbackOnly.Store(true)
		}
		return nil
	}
	if _, owned := scopedAuthorization(ctx); owned {
		return fmt.Errorf("StorageTransaction.Rollback: coordinator owns the transaction")
	}
	tx, ok := storageTransaction(ctx)
	if !ok {
		return fmt.Errorf("StorageTransaction.Rollback: transaction missing from context")
	}
	if err := tx.Rollback(); err != nil {
		return storage.NewStorageError("StorageTransaction.Rollback", storage.ErrorKindConnection, err, "failed to roll back transaction")
	}
	return nil
}

func storageTransaction(ctx context.Context) (*sqlx.Tx, bool) {
	tx, ok := ctx.Value(storageTransactionKey{}).(*sqlx.Tx)
	return tx, ok
}

func (a *Adapter) storageExecutor(ctx context.Context) sqlx.ExtContext {
	if tx, ok := storageTransaction(ctx); ok {
		return tx
	}
	return a.db
}

// participantTx borrows the coordinator transaction, or starts a transaction
// for a standalone compound repository operation. Only the latter owns commit.
func (a *Adapter) participantTx(ctx context.Context) (*sqlx.Tx, bool, error) {
	if tx, ok := storageTransaction(ctx); ok {
		return tx, false, nil
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	return tx, true, err
}
