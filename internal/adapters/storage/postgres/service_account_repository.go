package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ServiceAccountRepository = (*ServiceAccountRepository)(nil)

const serviceAccountColumns = `id, service_id, canonical_id, kind, scopes, credential_encrypted, credential_summary,
	credential_expires_at, version, created_at, updated_at`

// ServiceAccountRepository stores service accounts in PostgreSQL. The credential column holds
// ciphertext only.
type ServiceAccountRepository struct {
	adapter *Adapter
}

// NewServiceAccountRepository creates a PostgreSQL service-account repository.
func NewServiceAccountRepository(adapter *Adapter) *ServiceAccountRepository {
	return &ServiceAccountRepository{adapter: adapter}
}

type serviceAccountRecord struct {
	ID          id.ServiceAccountID `db:"id"`
	ServiceID   id.ServiceID        `db:"service_id"`
	CanonicalID string              `db:"canonical_id"`
	Kind        string              `db:"kind"`
	Scopes      []byte              `db:"scopes"`
	Credential  []byte              `db:"credential_encrypted"`
	Summary     []byte              `db:"credential_summary"`
	ExpiresAt   sql.NullTime        `db:"credential_expires_at"`
	Version     int64               `db:"version"`
	CreatedAt   time.Time           `db:"created_at"`
	UpdatedAt   time.Time           `db:"updated_at"`
}

func serviceAccountToRecord(account *model.ServiceAccount) (serviceAccountRecord, error) {
	const operation = "ServiceAccountRepository.toRecord"
	ciphertext, err := account.Credential.GetCiphertext()
	if err != nil {
		return serviceAccountRecord{}, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "service account credential must be encrypted")
	}
	scopes, err := json.Marshal(account.Scopes)
	if err != nil {
		return serviceAccountRecord{}, storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid service account scopes")
	}
	summary, err := json.Marshal(account.Summary)
	if err != nil {
		return serviceAccountRecord{}, storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid service account summary")
	}
	record := serviceAccountRecord{
		ID: account.ID, ServiceID: account.ServiceID, CanonicalID: account.CanonicalID, Kind: string(account.Kind),
		Scopes: scopes, Credential: ciphertext, Summary: summary, Version: account.Version,
		CreatedAt: account.CreatedAt.UTC(), UpdatedAt: account.UpdatedAt.UTC(),
	}
	if account.CredentialExpiresAt != nil {
		record.ExpiresAt = sql.NullTime{Time: account.CredentialExpiresAt.UTC(), Valid: true}
	}
	return record, nil
}

func (r serviceAccountRecord) toEntity() (*model.ServiceAccount, error) {
	const operation = "ServiceAccountRepository.toEntity"
	account := &model.ServiceAccount{
		ID: r.ID, ServiceID: r.ServiceID, CanonicalID: r.CanonicalID, Kind: model.ServiceAccountKind(r.Kind),
		Credential: model.NewEncryptedSecret(r.Credential), Version: r.Version,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if err := json.Unmarshal(r.Scopes, &account.Scopes); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "stored service account scopes are invalid")
	}
	if err := json.Unmarshal(r.Summary, &account.Summary); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "stored service account summary is invalid")
	}
	if r.ExpiresAt.Valid {
		expiry := r.ExpiresAt.Time.UTC()
		account.CredentialExpiresAt = &expiry
	}
	return account, nil
}

// classifyServiceAccountError maps a driver error to a StorageError. A missing service on insert
// is not found; a taken name wraps the canonical-ID sentinel.
func classifyServiceAccountError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation exceeded timeout")
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
		return storage.NewStorageError(operation, storage.ErrorKindNotFound, fmt.Errorf("%w: %w", ports.ErrNotFound, err), "service account not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "uq_service_accounts_service_canonical_id" {
				return storage.NewStorageError(operation, storage.ErrorKindConflict, fmt.Errorf("%w: %w", ports.ErrServiceAccountCanonicalIDConflict, err), "service account canonical_id already exists on this service")
			}
			return storage.NewStorageError(operation, storage.ErrorKindConflict, err, "service account already exists")
		case "23503":
			return storage.NewStorageError(operation, storage.ErrorKindNotFound, fmt.Errorf("%w: %w", ports.ErrNotFound, err), "service not found")
		case "23514":
			return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "service account violates a table constraint")
		}
	}
	return storage.NewStorageError(operation, storage.ErrorKindConnection, err, "service account query failed")
}

func (r *ServiceAccountRepository) requireDB(operation string) error {
	if r.adapter == nil || r.adapter.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	return nil
}

func (r *ServiceAccountRepository) Create(ctx context.Context, account *model.ServiceAccount) (err error) {
	const operation = "ServiceAccountRepository.Create"
	ctx, endSpan := startStorageSpan(ctx, "storage.create.service_account", "Create")
	defer func() { endSpan(err) }()
	if err := r.requireDB(operation); err != nil {
		return err
	}
	record, err := serviceAccountToRecord(account)
	if err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	_, err = r.adapter.db.ExecContext(execCtx, `INSERT INTO service_accounts
		(id, service_id, canonical_id, kind, scopes, credential_encrypted, credential_summary,
		 credential_expires_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $10)`,
		record.ID, record.ServiceID, record.CanonicalID, record.Kind, record.Scopes, record.Credential, record.Summary,
		record.ExpiresAt, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return classifyServiceAccountError(operation, err)
	}
	account.Version = 1
	return nil
}

func (r *ServiceAccountRepository) getOne(ctx context.Context, operation, query string, args ...any) (*model.ServiceAccount, error) {
	if err := r.requireDB(operation); err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var record serviceAccountRecord
	if err := r.adapter.db.GetContext(queryCtx, &record, query, args...); err != nil {
		return nil, classifyServiceAccountError(operation, err)
	}
	return record.toEntity()
}

func (r *ServiceAccountRepository) Get(ctx context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (_ *model.ServiceAccount, err error) {
	ctx, endSpan := startStorageSpan(ctx, "storage.get.service_account", "Get")
	defer func() { endSpan(err) }()
	return r.getOne(ctx, "ServiceAccountRepository.Get",
		`SELECT `+serviceAccountColumns+` FROM service_accounts WHERE id = $1 AND service_id = $2`, accountID, serviceID)
}

func (r *ServiceAccountRepository) GetByCanonicalID(ctx context.Context, serviceID id.ServiceID, canonicalID string) (_ *model.ServiceAccount, err error) {
	ctx, endSpan := startStorageSpan(ctx, "storage.get_by_canonical_id.service_account", "GetByCanonicalID")
	defer func() { endSpan(err) }()
	return r.getOne(ctx, "ServiceAccountRepository.GetByCanonicalID",
		`SELECT `+serviceAccountColumns+` FROM service_accounts WHERE service_id = $1 AND canonical_id = $2`, serviceID, canonicalID)
}

func (r *ServiceAccountRepository) ListByService(ctx context.Context, serviceID id.ServiceID) (_ []*model.ServiceAccount, err error) {
	const operation = "ServiceAccountRepository.ListByService"
	ctx, endSpan := startStorageSpan(ctx, "storage.list.service_account", "ListByService")
	defer func() { endSpan(err) }()
	if err := r.requireDB(operation); err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var records []serviceAccountRecord
	if err := r.adapter.db.SelectContext(queryCtx, &records,
		`SELECT `+serviceAccountColumns+` FROM service_accounts WHERE service_id = $1 ORDER BY canonical_id COLLATE "C"`, serviceID); err != nil {
		return nil, classifyServiceAccountError(operation, err)
	}
	accounts := make([]*model.ServiceAccount, 0, len(records))
	for _, record := range records {
		account, err := record.toEntity()
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

// preconditionFailure tells a missing account from a stale version after a conditional write
// matched no row.
func (r *ServiceAccountRepository) preconditionFailure(ctx context.Context, operation string, serviceID id.ServiceID, accountID id.ServiceAccountID) error {
	var exists bool
	if err := r.adapter.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM service_accounts WHERE id = $1 AND service_id = $2)`, accountID, serviceID).Scan(&exists); err != nil {
		return classifyServiceAccountError(operation, err)
	}
	if !exists {
		return storage.NewStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "service account not found")
	}
	return storage.NewStorageError(operation, storage.ErrorKindConflict, ports.ErrServiceAccountVersionMismatch, "service account version mismatch")
}

func (r *ServiceAccountRepository) Update(ctx context.Context, account *model.ServiceAccount, expectedVersion *int64) (err error) {
	const operation = "ServiceAccountRepository.Update"
	ctx, endSpan := startStorageSpan(ctx, "storage.update.service_account", "Update")
	defer func() { endSpan(err) }()
	if err := r.requireDB(operation); err != nil {
		return err
	}
	record, err := serviceAccountToRecord(account)
	if err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	var version int64
	err = r.adapter.db.QueryRowContext(execCtx, `UPDATE service_accounts
		SET canonical_id = $3, scopes = $4, credential_encrypted = $5, credential_summary = $6,
		    credential_expires_at = $7, updated_at = $8, version = version + 1
		WHERE id = $1 AND service_id = $2 AND ($9::bigint IS NULL OR version = $9)
		RETURNING version`,
		record.ID, record.ServiceID, record.CanonicalID, record.Scopes, record.Credential, record.Summary,
		record.ExpiresAt, record.UpdatedAt, expectedVersion).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return r.preconditionFailure(execCtx, operation, account.ServiceID, account.ID)
	}
	if err != nil {
		return classifyServiceAccountError(operation, err)
	}
	account.Version = version
	return nil
}

func (r *ServiceAccountRepository) Delete(ctx context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID, expectedVersion *int64) (err error) {
	const operation = "ServiceAccountRepository.Delete"
	ctx, endSpan := startStorageSpan(ctx, "storage.delete.service_account", "Delete")
	defer func() { endSpan(err) }()
	if err := r.requireDB(operation); err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	result, err := r.adapter.db.ExecContext(execCtx,
		`DELETE FROM service_accounts WHERE id = $1 AND service_id = $2 AND ($3::bigint IS NULL OR version = $3)`,
		accountID, serviceID, expectedVersion)
	if err != nil {
		return classifyServiceAccountError(operation, err)
	}
	if rows, err := result.RowsAffected(); err != nil {
		return classifyServiceAccountError(operation, err)
	} else if rows == 0 {
		return r.preconditionFailure(execCtx, operation, serviceID, accountID)
	}
	return nil
}

func (r *ServiceAccountRepository) CountByService(ctx context.Context, serviceID id.ServiceID) (_ int, err error) {
	const operation = "ServiceAccountRepository.CountByService"
	ctx, endSpan := startStorageSpan(ctx, "storage.count.service_account", "CountByService")
	defer func() { endSpan(err) }()
	if err := r.requireDB(operation); err != nil {
		return 0, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var count int
	if err := r.adapter.db.GetContext(queryCtx, &count, `SELECT COUNT(*) FROM service_accounts WHERE service_id = $1`, serviceID); err != nil {
		return 0, classifyServiceAccountError(operation, err)
	}
	return count, nil
}
