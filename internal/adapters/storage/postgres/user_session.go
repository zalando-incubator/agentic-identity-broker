package postgres

import (
	"context"
	"database/sql"
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
	`

	_, err := r.adapter.db.ExecContext(ctx, query,
		session.ID, session.Principal, session.ServiceID,
		session.EncryptedAccessToken, session.EncryptedRefreshToken,
		session.TokenType, session.AccessTokenExpiresAt, session.RefreshTokenExpiresAt,
		pq.Array(session.Scope), session.EncryptionContext,
		session.InitiatedAt, session.CreatedAt, session.UpdatedAt,
	)

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

	err := r.adapter.db.GetContext(ctx, &rec, query, sessionID)
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

	err := r.adapter.db.GetContext(ctx, &rec, query, principal, serviceID)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is not an error
	}
	if err != nil {
		return nil, r.wrapError(err, "FindByPrincipalAndService")
	}
	return recordToSession(&rec), nil
}

// WithLockedSession re-reads and updates one session in a row-locked transaction.
func (r *PostgresUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("WithLockedSession", storage.ErrorKindConnection, nil, "database not initialized")
	}

	acquireCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	conn, err := r.adapter.db.Connx(acquireCtx)
	cancel()
	if err != nil {
		return nil, r.wrapError(err, "WithLockedSession")
	}
	defer conn.Close() //nolint:errcheck

	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return nil, r.wrapError(err, "WithLockedSession")
	}
	defer tx.Rollback() //nolint:errcheck

	var rec userSessionRecord
	readCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	err = tx.GetContext(readCtx, &rec, `SELECT * FROM user_sessions WHERE principal = $1 AND service_id = $2 FOR UPDATE`, principal, serviceID)
	cancel()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, r.wrapError(err, "WithLockedSession")
	}
	session := recordToSession(&rec)
	updated, err := refresh(ctx, session)
	if err != nil {
		return nil, err
	}
	if updated {
		if err := session.Validate(); err != nil {
			return nil, err
		}
		writeCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
		_, err = tx.ExecContext(writeCtx, `UPDATE user_sessions SET encrypted_access_token = $1, encrypted_refresh_token = $2,
			access_token_expires_at = $3, updated_at = $4 WHERE id = $5`,
			session.EncryptedAccessToken, session.EncryptedRefreshToken, session.AccessTokenExpiresAt, session.UpdatedAt, session.ID)
		cancel()
		if err != nil {
			return nil, r.wrapError(err, "WithLockedSession")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, r.wrapError(err, "WithLockedSession")
	}
	return session, nil
}

// ListByPrincipal retrieves all sessions for a principal, including expired ones.
// Used for user-visible session lists.
func (r *PostgresUserSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	var records []*userSessionRecord
	query := `SELECT * FROM user_sessions WHERE principal = $1 ORDER BY created_at DESC`

	err := r.adapter.db.SelectContext(ctx, &records, query, principal)
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

	err := r.adapter.db.SelectContext(ctx, &records, query, principal)
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
	_, err := r.adapter.db.ExecContext(ctx, query, sessionID)
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
	_, err := r.adapter.db.ExecContext(ctx, query, principal, serviceID)
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

	err := r.adapter.db.GetContext(ctx, &count, query, serviceID)
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
