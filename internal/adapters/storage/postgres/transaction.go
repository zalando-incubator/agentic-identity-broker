package postgres

import (
	"context"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/jmoiron/sqlx"
)

type storageTransactionKey struct{}

func (a *Adapter) BeginTX(ctx context.Context) (context.Context, error) {
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
	tx, ok := storageTransaction(ctx)
	if !ok {
		return fmt.Errorf("StorageTransaction.Commit: transaction missing from context")
	}
	if err := tx.Commit(); err != nil {
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindConnection, err, "failed to commit transaction")
	}
	return nil
}

func (a *Adapter) Rollback(ctx context.Context) error {
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
