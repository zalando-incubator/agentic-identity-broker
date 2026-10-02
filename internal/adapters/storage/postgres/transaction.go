package postgres

import (
	"context"
	"database/sql"
	"errors"
	"hash/fnv"
	"slices"
	"strings"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jmoiron/sqlx"
)

const storageLifecycleLockClass int32 = 1095320140
const storageSubjectLockClass int32 = 1095320147

type storageTransactionKey struct{}

type storageTransactionOwner struct {
	mu               sync.Mutex
	adapter          *Adapter
	tx               *sqlx.Tx
	isolation        ports.StorageTransactionIsolation
	lifecycle        ports.StorageLifecycleGate
	subjects         map[string]bool
	lastSubject      string
	businessAccessed bool
	rollbackOnly     bool
	completed        bool
	afterCommit      []func()
}

type storageTransactionScope struct {
	*sqlx.Tx
	owner     *storageTransactionOwner
	ctx       context.Context
	owning    bool
	completed bool
}

type sqlTransactionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type storageSQLExecutor interface {
	sqlx.ExtContext
	sqlTransactionExecutor
	GetContext(context.Context, any, string, ...any) error
	SelectContext(context.Context, any, string, ...any) error
}

func (a *Adapter) BeginTX(ctx context.Context) (context.Context, error) {
	hints := ports.StorageTransactionHintsFromContext(ctx)
	if hints.Isolation > ports.StorageSerializable || hints.Lifecycle > ports.StorageLifecycleExclusive {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindValidation, nil, "invalid transaction hints")
	}
	if scope, ok := storageTransaction(ctx); ok {
		owner := scope.owner
		owner.mu.Lock()
		defer owner.mu.Unlock()
		if owner.adapter != a || owner.completed || scope.completed || owner.rollbackOnly {
			return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "transaction is not active")
		}
		if hints.Isolation > owner.isolation || hints.Lifecycle > owner.lifecycle {
			return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "joined scope cannot strengthen transaction protection")
		}
		if err := owner.lockSubjects(ctx, hints.Subjects); err != nil {
			owner.rollbackOnly = true
			return nil, err
		}
		joined := &storageTransactionScope{Tx: owner.tx, owner: owner}
		joined.ctx = context.WithValue(ctx, storageTransactionKey{}, joined)
		return joined.ctx, nil
	}
	if a == nil || a.db == nil {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConnection, nil, "database not initialized")
	}
	levels := [...]sql.IsolationLevel{sql.LevelReadCommitted, sql.LevelRepeatableRead, sql.LevelSerializable}
	tx, err := a.db.BeginTxx(ctx, &sql.TxOptions{Isolation: levels[hints.Isolation]})
	if err != nil {
		return nil, transactionStorageError("StorageTransaction.BeginTX", err, "failed to begin transaction")
	}
	return a.ownTransaction(ctx, tx, hints)
}

func (a *Adapter) ownTransaction(ctx context.Context, tx *sqlx.Tx, hints ports.StorageTransactionHints) (context.Context, error) {
	owner := &storageTransactionOwner{adapter: a, tx: tx, isolation: hints.Isolation, lifecycle: hints.Lifecycle, subjects: make(map[string]bool)}
	lock := "SELECT pg_advisory_xact_lock_shared($1,$2)"
	if hints.Lifecycle == ports.StorageLifecycleExclusive {
		lock = "SELECT pg_advisory_xact_lock($1,$2)"
	}
	if _, err := tx.ExecContext(ctx, lock, storageLifecycleLockClass, int32(0)); err != nil {
		_ = tx.Rollback()
		return nil, transactionStorageError("StorageTransaction.BeginTX", err, "failed to acquire lifecycle gate")
	}
	if err := owner.lockSubjects(ctx, hints.Subjects); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	scope := &storageTransactionScope{Tx: tx, owner: owner, owning: true}
	scope.ctx = ports.WithStorageTransactionEffects(context.WithValue(ctx, storageTransactionKey{}, scope), owner)
	return scope.ctx, nil
}

func (owner *storageTransactionOwner) lockSubjects(ctx context.Context, subjects []ports.StorageSubjectGate) error {
	slices.SortFunc(subjects, func(a, b ports.StorageSubjectGate) int {
		if order := strings.Compare(a.Principal.String(), b.Principal.String()); order != 0 {
			return order
		}
		if a.Exclusive == b.Exclusive {
			return 0
		}
		if a.Exclusive {
			return -1
		}
		return 1
	})
	for _, subject := range subjects {
		principal := subject.Principal.String()
		if strings.TrimSpace(principal) == "" {
			return storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindValidation, nil, "subject gate requires an exact principal")
		}
		if exclusive, held := owner.subjects[principal]; held {
			if subject.Exclusive && !exclusive {
				return storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "subject gate upgrade requires a fresh transaction")
			}
			continue
		}
		if owner.businessAccessed || principal < owner.lastSubject {
			return storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "subject discovery changed; restart before business locks")
		}
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(principal))
		lockKey := int64(hash.Sum32())
		if lockKey >= 1<<31 {
			lockKey -= 1 << 32
		}
		lock := "SELECT pg_advisory_xact_lock_shared($1,$2)"
		if subject.Exclusive {
			lock = "SELECT pg_advisory_xact_lock($1,$2)"
		}
		if _, err := owner.tx.ExecContext(ctx, lock, storageSubjectLockClass, lockKey); err != nil {
			return transactionStorageError("StorageTransaction.BeginTX", err, "failed to acquire subject gate")
		}
		owner.subjects[principal] = subject.Exclusive
		owner.lastSubject = principal
	}
	return nil
}

func (a *Adapter) Commit(ctx context.Context) error {
	scope, ok := storageTransaction(ctx)
	if !ok || scope.owner.adapter != a {
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindValidation, nil, "transaction missing from context")
	}
	owner := scope.owner
	owner.mu.Lock()
	if scope.completed || owner.completed {
		owner.mu.Unlock()
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindConflict, sql.ErrTxDone, "transaction is already complete")
	}
	scope.completed = true
	if !scope.owning {
		owner.mu.Unlock()
		return nil
	}
	owner.completed = true
	if owner.rollbackOnly {
		owner.afterCommit = nil
		owner.mu.Unlock()
		_ = owner.tx.Rollback()
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindConflict, nil, "transaction is rollback-only")
	}
	effects := owner.afterCommit
	owner.afterCommit = nil
	owner.mu.Unlock()
	if err := owner.tx.Commit(); err != nil {
		return transactionStorageError("StorageTransaction.Commit", err, "failed to commit transaction")
	}
	for _, effect := range effects {
		effect()
	}
	return nil
}

func (a *Adapter) Rollback(ctx context.Context) error {
	scope, ok := storageTransaction(ctx)
	if !ok || scope.owner.adapter != a {
		return storage.NewStorageError("StorageTransaction.Rollback", storage.ErrorKindValidation, nil, "transaction missing from context")
	}
	owner := scope.owner
	owner.mu.Lock()
	if scope.completed || owner.completed {
		owner.mu.Unlock()
		return nil
	}
	scope.completed = true
	owner.rollbackOnly = true
	if !scope.owning {
		owner.mu.Unlock()
		return nil
	}
	owner.completed = true
	owner.afterCommit = nil
	owner.mu.Unlock()
	if err := owner.tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return transactionStorageError("StorageTransaction.Rollback", err, "failed to roll back transaction")
	}
	return nil
}

func (owner *storageTransactionOwner) AfterCommit(effect func()) error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.completed || owner.rollbackOnly || effect == nil {
		return storage.NewStorageError("StorageTransaction.AfterCommit", storage.ErrorKindConflict, nil, "transaction cannot accept effects")
	}
	owner.afterCommit = append(owner.afterCommit, effect)
	return nil
}

func (scope *storageTransactionScope) Commit() error { return scope.owner.adapter.Commit(scope.ctx) }
func (scope *storageTransactionScope) Rollback() error {
	return scope.owner.adapter.Rollback(scope.ctx)
}

func storageTransaction(ctx context.Context) (*storageTransactionScope, bool) {
	scope, ok := ctx.Value(storageTransactionKey{}).(*storageTransactionScope)
	return scope, ok
}

func (a *Adapter) beginSQLTransaction(ctx context.Context, options *sql.TxOptions) (*storageTransactionScope, error) {
	if options != nil {
		hints := ports.StorageTransactionHintsFromContext(ctx)
		switch options.Isolation {
		case sql.LevelDefault, sql.LevelReadCommitted:
		case sql.LevelRepeatableRead:
			hints.Isolation = max(hints.Isolation, ports.StorageRepeatableRead)
		case sql.LevelSerializable:
			hints.Isolation = ports.StorageSerializable
		default:
			return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindValidation, nil, "unsupported transaction isolation")
		}
		ctx = ports.WithStorageTransactionHints(ctx, hints)
	}
	txCtx, err := a.BeginTX(ctx)
	if err != nil {
		return nil, err
	}
	scope, _ := storageTransaction(txCtx)
	scope.owner.mu.Lock()
	scope.owner.businessAccessed = true
	scope.owner.mu.Unlock()
	return scope, nil
}

func (a *Adapter) storageExecutor(ctx context.Context) storageSQLExecutor {
	if scope, ok := storageTransaction(ctx); ok {
		scope.owner.mu.Lock()
		scope.owner.businessAccessed = true
		scope.owner.mu.Unlock()
		return scope.Tx
	}
	return a.db
}

func transactionStorageError(operation string, cause error, message string) error {
	kind := storage.ErrorKindConnection
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		kind = storage.ErrorKindTimeout
	}
	return storage.NewStorageError(operation, kind, cause, message)
}
