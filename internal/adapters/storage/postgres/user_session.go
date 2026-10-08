package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/lib/pq"
)

// userSessionRecord is an adapter-local database record struct for user_sessions.
// It maps directly to the table schema. The Scope field uses pq.StringArray to
// correctly handle PostgreSQL TEXT[] scanning via pgx/v5/stdlib — the driver
// returns TEXT[] as a string literal ("{val1,val2}") which []string cannot absorb.
type userSessionRecord struct {
	ID                    id.SessionID              `db:"id"`
	Principal             id.Principal              `db:"principal"`
	ServiceID             id.ServiceID              `db:"service_id"`
	EncryptedAccessToken  []byte                    `db:"encrypted_access_token"`
	EncryptedRefreshToken []byte                    `db:"encrypted_refresh_token"`
	TokenType             string                    `db:"token_type"`
	AccessTokenExpiresAt  *time.Time                `db:"access_token_expires_at"`
	RefreshTokenExpiresAt *time.Time                `db:"refresh_token_expires_at"`
	Scope                 pq.StringArray            `db:"scope"`
	EncryptionContext     storage.EncryptionContext `db:"encryption_context"`
	InitiatedAt           time.Time                 `db:"initiated_at"`
	CreatedAt             time.Time                 `db:"created_at"`
	UpdatedAt             time.Time                 `db:"updated_at"`
}

func recordToSession(r *userSessionRecord) *storage.UserSession {
	s := &storage.UserSession{
		ID:                    r.ID,
		Principal:             r.Principal,
		ServiceID:             r.ServiceID,
		EncryptedAccessToken:  r.EncryptedAccessToken,
		EncryptedRefreshToken: r.EncryptedRefreshToken,
		TokenType:             r.TokenType,
		AccessTokenExpiresAt:  r.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: r.RefreshTokenExpiresAt,
		Scope:                 []string(r.Scope),
		EncryptionContext:     r.EncryptionContext,
		InitiatedAt:           r.InitiatedAt,
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
	}
	if s.Scope == nil {
		s.Scope = []string{}
	}
	return s
}

// PostgresUserSessionRepository is a PostgreSQL implementation of UserSessionRepository.
type PostgresUserSessionRepository struct {
	adapter *Adapter
}

// NewUserSessionRepository creates a new PostgreSQL user session repository.
func NewUserSessionRepository(adapter *Adapter) *PostgresUserSessionRepository {
	return &PostgresUserSessionRepository{adapter: adapter}
}

// Create creates a new user session with upsert semantics.
func (r *PostgresUserSessionRepository) Create(ctx context.Context, session *storage.UserSession) error {
	if session == nil {
		return errors.New("session cannot be nil")
	}
	if err := session.Validate(); err != nil {
		return err
	}

	if session.ID.IsZero() {
		session.ID = id.NewSessionID()
	}

	query := `
		INSERT INTO user_sessions (
			id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
			token_type, access_token_expires_at, refresh_token_expires_at, scope,
			encryption_context, initiated_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
		ON CONFLICT (principal, service_id) DO UPDATE SET
			encrypted_access_token = EXCLUDED.encrypted_access_token,
			encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
			token_type = EXCLUDED.token_type,
			access_token_expires_at = EXCLUDED.access_token_expires_at,
			refresh_token_expires_at = EXCLUDED.refresh_token_expires_at,
			scope = EXCLUDED.scope,
			encryption_context = EXCLUDED.encryption_context,
			updated_at = NOW()
		RETURNING id, initiated_at, created_at, updated_at
	`

	err := r.adapter.storageExecutor(ctx).QueryRowContext(ctx, query,
		session.ID, session.Principal, session.ServiceID,
		session.EncryptedAccessToken, session.EncryptedRefreshToken,
		session.TokenType, session.AccessTokenExpiresAt, session.RefreshTokenExpiresAt,
		pq.Array(session.Scope), session.EncryptionContext,
		session.InitiatedAt, session.CreatedAt, session.UpdatedAt).
		Scan(&session.ID, &session.InitiatedAt, &session.CreatedAt, &session.UpdatedAt)

	if err != nil {
		return r.wrapError(err, "Create")
	}
	return nil
}

// Get retrieves a session by ID.
func (r *PostgresUserSessionRepository) Get(ctx context.Context, sessionID id.SessionID) (*storage.UserSession, error) {
	if sessionID.IsZero() {
		return nil, errors.New("session ID cannot be empty")
	}

	var rec userSessionRecord
	query := `SELECT * FROM user_sessions WHERE id = $1`

	err := r.adapter.storageExecutor(ctx).GetContext(ctx, &rec, query, sessionID)
	if err == sql.ErrNoRows {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, err, "session not found")
	}
	if err != nil {
		return nil, r.wrapError(err, "Get")
	}
	return recordToSession(&rec), nil
}

// FindByPrincipalAndService retrieves the session for a principal and service.
func (r *PostgresUserSessionRepository) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}

	var rec userSessionRecord
	query := `SELECT * FROM user_sessions WHERE principal = $1 AND service_id = $2`
	if _, ambient := storageTransaction(ctx); ambient {
		query += " FOR UPDATE"
	}
	err := r.adapter.storageExecutor(ctx).GetContext(ctx, &rec, query, principal, serviceID)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is not an error
	}
	if err != nil {
		return nil, r.wrapError(err, "FindByPrincipalAndService")
	}
	return recordToSession(&rec), nil
}

// WithLockedSession coordinates refreshes across processes on a connection,
// without holding ledger lifecycle/subject gates or a business row lock while
// the provider responds. Only the short conditional write starts a transaction.
func (r *PostgresUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) error) (session *storage.UserSession, resultErr error) {
	const operation = "WithLockedSession"
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	if _, ambient := storageTransaction(ctx); ambient {
		return nil, storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "refresh coordination must precede the business transaction")
	}
	if r.adapter == nil || r.adapter.db == nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	conn, err := r.adapter.db.Connx(ctx)
	if err != nil {
		return nil, r.wrapError(err, operation)
	}
	locked, discard := false, false
	defer func() {
		if locked {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), r.adapter.timeouts.Write)
			var unlocked bool
			unlockErr := conn.GetContext(cleanupCtx, &unlocked,
				`SELECT pg_advisory_unlock($1, hashtext($2 || '/' || $3))`, int32(1095320150), principal.String(), serviceID.String())
			cancel()
			if unlockErr != nil || !unlocked {
				discard = true
				resultErr = errors.Join(resultErr, storage.NewStorageError(operation, storage.ErrorKindConnection,
					unlockErr, "failed to release session coordination lock"))
			}
		}
		if discard {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		if err := conn.Close(); err != nil {
			resultErr = errors.Join(resultErr, r.wrapError(err, operation))
		}
	}()
	if _, err := conn.ExecContext(ctx,
		`SELECT pg_advisory_lock($1, hashtext($2 || '/' || $3))`, int32(1095320150), principal.String(), serviceID.String()); err != nil {
		discard = true // The server may have taken the lock just before cancellation.
		return nil, r.wrapError(err, operation)
	}
	locked = true
	readCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	var rec userSessionRecord
	err = conn.GetContext(readCtx, &rec, `SELECT * FROM user_sessions WHERE principal = $1 AND service_id = $2`, principal, serviceID)
	cancel()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, r.wrapError(err, operation)
	}
	previous := recordToSession(&rec)
	prepared := *previous
	prepared.EncryptedAccessToken = bytes.Clone(previous.EncryptedAccessToken)
	prepared.EncryptedRefreshToken = bytes.Clone(previous.EncryptedRefreshToken)
	session = &prepared
	coordinationCtx := context.WithValue(ctx, refreshConnectionKey{}, conn)
	if err := refresh(coordinationCtx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// UpdateRefreshedSession accepts only the exact pre-exchange session snapshot
// and joins the ledger owner, so the refreshed fact and tokens commit together.
func (r *PostgresUserSessionRepository) UpdateRefreshedSession(ctx context.Context, previous, current *storage.UserSession) error {
	const operation = "UpdateRefreshedSession"
	if previous == nil || current == nil || previous.ID != current.ID ||
		previous.Principal != current.Principal || previous.ServiceID != current.ServiceID {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "invalid refreshed session")
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if _, ambient := storageTransaction(ctx); !ambient {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "refresh requires an owning transaction")
	}
	writeCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	result, err := r.adapter.storageExecutor(writeCtx).ExecContext(writeCtx, `UPDATE user_sessions SET encrypted_access_token = $1, encrypted_refresh_token = $2,
		access_token_expires_at = $3, updated_at = $4 WHERE id = $5 AND principal = $6 AND service_id = $7
		AND encrypted_access_token IS NOT DISTINCT FROM $8 AND encrypted_refresh_token IS NOT DISTINCT FROM $9 AND updated_at = $10`,
		current.EncryptedAccessToken, current.EncryptedRefreshToken, current.AccessTokenExpiresAt, current.UpdatedAt,
		previous.ID, previous.Principal, previous.ServiceID, previous.EncryptedAccessToken, previous.EncryptedRefreshToken, previous.UpdatedAt)
	if err != nil {
		return r.wrapError(err, operation)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return r.wrapError(err, operation)
	}
	if count != 1 {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "session changed during refresh")
	}
	return nil
}

// ListByPrincipal retrieves all sessions for a principal, including expired ones.
// Used for user-visible session lists.
func (r *PostgresUserSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	var records []*userSessionRecord
	query := `SELECT * FROM user_sessions WHERE principal = $1 ORDER BY created_at DESC`

	err := r.adapter.storageExecutor(ctx).SelectContext(ctx, &records, query, principal)
	if err != nil && err != sql.ErrNoRows {
		return nil, r.wrapError(err, "ListByPrincipal")
	}
	sessions := make([]*storage.UserSession, len(records))
	for i, rec := range records {
		sessions[i] = recordToSession(rec)
	}
	return sessions, nil
}

// ListActiveByPrincipal retrieves only non-expired sessions for a principal.
// Used for FR-020 consent submission validation.
func (r *PostgresUserSessionRepository) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	var records []*userSessionRecord
	query := `SELECT * FROM user_sessions WHERE principal = $1 AND (refresh_token_expires_at IS NULL OR refresh_token_expires_at > NOW()) ORDER BY created_at DESC`

	err := r.adapter.storageExecutor(ctx).SelectContext(ctx, &records, query, principal)
	if err != nil && err != sql.ErrNoRows {
		return nil, r.wrapError(err, "ListActiveByPrincipal")
	}
	sessions := make([]*storage.UserSession, len(records))
	for i, rec := range records {
		sessions[i] = recordToSession(rec)
	}
	return sessions, nil
}

// Delete deletes a session by ID.
func (r *PostgresUserSessionRepository) Delete(ctx context.Context, sessionID id.SessionID) error {
	if sessionID.IsZero() {
		return errors.New("session ID cannot be empty")
	}

	query := `DELETE FROM user_sessions WHERE id = $1`
	_, err := r.adapter.storageExecutor(ctx).ExecContext(ctx, query, sessionID)
	if err != nil {
		return r.wrapError(err, "Delete")
	}
	return nil
}

// DeleteByPrincipalAndService deletes the session for a principal and service.
func (r *PostgresUserSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	if principal.IsZero() || serviceID.IsZero() {
		return errors.New("principal and serviceID required")
	}

	query := `DELETE FROM user_sessions WHERE principal = $1 AND service_id = $2`
	_, err := r.adapter.storageExecutor(ctx).ExecContext(ctx, query, principal, serviceID)
	if err != nil {
		return r.wrapError(err, "DeleteByPrincipalAndService")
	}
	return nil
}

// CountByService counts sessions referencing a service.
func (r *PostgresUserSessionRepository) CountByService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	if serviceID.IsZero() {
		return 0, errors.New("serviceID required")
	}

	var count int
	query := `SELECT COUNT(*) FROM user_sessions WHERE service_id = $1`

	err := r.adapter.storageExecutor(ctx).GetContext(ctx, &count, query, serviceID)
	if err != nil {
		return 0, r.wrapError(err, "CountByService")
	}
	return count, nil
}

// Helper function to wrap errors into StorageError
func (r *PostgresUserSessionRepository) wrapError(err error, operation string) error {
	if err == sql.ErrNoRows {
		return storage.NewStorageError(operation, storage.ErrorKindNotFound, err, "not found")
	}
	if err.Error() == "context deadline exceeded" {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "timeout")
	}
	return storage.NewStorageError(operation, storage.ErrorKindUnknown, err, err.Error())
}
