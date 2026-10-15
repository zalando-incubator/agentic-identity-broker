package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lib/pq"
)

const userSessionColumns = `id, principal, service_id, callback_id, upstream_client_id,
encrypted_access_token, encrypted_refresh_token, token_type, access_token_expires_at,
refresh_token_expires_at, scope, encryption_context, initiated_at, created_at, updated_at`

const listExpiringSessionsFirstPageQuery = `SELECT ` + userSessionColumns + ` FROM user_sessions
WHERE access_token_expires_at IS NOT NULL
  AND access_token_expires_at <= $1
ORDER BY access_token_expires_at, id
LIMIT $2`

const listExpiringSessionsAfterCursorQuery = `SELECT ` + userSessionColumns + ` FROM user_sessions
WHERE access_token_expires_at IS NOT NULL
  AND access_token_expires_at <= $1
  AND (access_token_expires_at, id) > ($2, $3)
ORDER BY access_token_expires_at, id
LIMIT $4`

// SQL projection of storage.SessionIsUsable; both active-session reads share this predicate.
const usableSessionPredicate = `(access_token_expires_at IS NULL OR access_token_expires_at > NOW()
	OR (COALESCE(octet_length(encrypted_refresh_token), 0) > 0
		AND (refresh_token_expires_at IS NULL OR refresh_token_expires_at > NOW())))`

// userSessionRecord is an adapter-local database record struct for user_sessions.
// It maps directly to the table schema. The Scope field uses pq.StringArray to
// correctly handle PostgreSQL TEXT[] scanning via pgx/v5/stdlib — the driver
// returns TEXT[] as a string literal ("{val1,val2}") which []string cannot absorb.
type userSessionRecord struct {
	ID                    id.SessionID              `db:"id"`
	Principal             id.Principal              `db:"principal"`
	ServiceID             id.ServiceID              `db:"service_id"`
	CallbackID            string                    `db:"callback_id"`
	UpstreamClientID      *string                   `db:"upstream_client_id"`
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

type userSessionSummaryRecord struct {
	ID                    id.SessionID   `db:"id"`
	ServiceID             id.ServiceID   `db:"service_id"`
	TokenType             string         `db:"token_type"`
	Scope                 pq.StringArray `db:"scope"`
	InitiatedAt           time.Time      `db:"initiated_at"`
	AccessTokenExpiresAt  *time.Time     `db:"access_token_expires_at"`
	RefreshTokenExpiresAt *time.Time     `db:"refresh_token_expires_at"`
	HasRefreshToken       bool           `db:"has_refresh_token"`
}

func recordToSession(r *userSessionRecord) *storage.UserSession {
	s := &storage.UserSession{
		ID:                    r.ID,
		Principal:             r.Principal,
		ServiceID:             r.ServiceID,
		CallbackID:            r.CallbackID,
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
	if r.UpstreamClientID != nil {
		clientID := id.ClientID(*r.UpstreamClientID)
		s.UpstreamClientID = &clientID
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

var _ ports.UserSessionExpiryRepository = (*PostgresUserSessionRepository)(nil)

// NewUserSessionRepository creates a new PostgreSQL user session repository.
func NewUserSessionRepository(adapter *Adapter) *PostgresUserSessionRepository {
	return &PostgresUserSessionRepository{adapter: adapter}
}

// Create creates a new user session with upsert semantics.
func (r *PostgresUserSessionRepository) Create(ctx context.Context, session *storage.UserSession) error {
	_, err := r.create(ctx, session, true)
	return err
}

func (r *PostgresUserSessionRepository) CreateIfAbsent(ctx context.Context, session *storage.UserSession) (*storage.UserSession, error) {
	return r.create(ctx, session, false)
}

func (r *PostgresUserSessionRepository) create(ctx context.Context, session *storage.UserSession, replace bool) (*storage.UserSession, error) {
	if session == nil {
		return nil, errors.New("session cannot be nil")
	}
	if err := session.Validate(); err != nil {
		return nil, err
	}

	if session.ID.IsZero() {
		session.ID = id.NewSessionID()
	}
	var upstreamClientID *string
	if session.UpstreamClientID != nil {
		clientID := session.UpstreamClientID.String()
		upstreamClientID = &clientID
	}

	// The share lock serializes issuer and audience checks with provider updates,
	// which take the service row's exclusive lock before counting sessions.
	const insertQuery = `
		INSERT INTO user_sessions (
			id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
			token_type, access_token_expires_at, refresh_token_expires_at, scope,
			encryption_context, initiated_at, created_at, updated_at, callback_id, upstream_client_id
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		FROM thirdparty_oauth2_services AS service
		WHERE service.id = $3
			AND ((service.resource_url IS NULL AND $16::text = '' AND $17::text = '')
				OR (service.resource_url IS NOT NULL AND $16::text <> '' AND $17::text <> ''
					AND service.issuer_uri = $16::text AND service.authorization_params ->> 'resource' = $17::text))
		FOR SHARE OF service
		ON CONFLICT (principal, service_id)
	`
	query := insertQuery + `DO NOTHING`
	if replace {
		query = insertQuery + `DO UPDATE SET
			upstream_client_id = EXCLUDED.upstream_client_id,
			encrypted_access_token = EXCLUDED.encrypted_access_token,
			encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
			token_type = EXCLUDED.token_type,
			access_token_expires_at = EXCLUDED.access_token_expires_at,
			refresh_token_expires_at = EXCLUDED.refresh_token_expires_at,
			scope = EXCLUDED.scope,
			encryption_context = EXCLUDED.encryption_context,
			callback_id = EXCLUDED.callback_id,
			updated_at = NOW()
	`
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	result, err := r.adapter.db.ExecContext(execCtx, query,
		session.ID, session.Principal, session.ServiceID,
		session.EncryptedAccessToken, session.EncryptedRefreshToken,
		session.TokenType, session.AccessTokenExpiresAt, session.RefreshTokenExpiresAt,
		pq.Array(session.Scope), session.EncryptionContext,
		session.InitiatedAt, session.CreatedAt, session.UpdatedAt,
		session.CallbackID, upstreamClientID, session.ExpectedIssuerURI, session.ExpectedResource,
	)
	if err != nil {
		return nil, r.wrapError(err, "Create")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, r.wrapError(err, "Create")
	}
	if rows == 0 {
		operation := "CreateIfAbsent"
		if replace {
			operation = "Create"
		}
		// INSERT ... SELECT inserts nothing for a missing service, a stale provider
		// expectation, or (without replace) an existing session.
		var probe struct {
			ServiceExists   bool `db:"service_exists"`
			ProviderMatches bool `db:"provider_matches"`
		}
		readCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
		err = r.adapter.db.GetContext(readCtx, &probe, `SELECT
			EXISTS (SELECT 1 FROM thirdparty_oauth2_services WHERE id = $1) AS service_exists,
			EXISTS (
				SELECT 1 FROM thirdparty_oauth2_services
				WHERE id = $1 AND ((resource_url IS NULL AND $2::text = '' AND $3::text = '')
					OR (resource_url IS NOT NULL AND $2::text <> '' AND $3::text <> ''
						AND issuer_uri = $2::text AND authorization_params ->> 'resource' = $3::text))
			) AS provider_matches`, session.ServiceID, session.ExpectedIssuerURI, session.ExpectedResource)
		cancel()
		if err != nil {
			return nil, r.wrapError(err, operation)
		}
		if !probe.ServiceExists {
			return nil, storage.NewStorageError(operation, storage.ErrorKindNotFound, nil, "service not found")
		}
		if replace || !probe.ProviderMatches {
			return nil, storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "service issuer, audience, or source changed")
		}
		winner, err := r.FindByPrincipalAndService(ctx, session.Principal, session.ServiceID)
		if err != nil {
			return nil, err
		}
		if winner == nil {
			return nil, storage.NewStorageError("CreateIfAbsent", storage.ErrorKindConflict, nil, "session no longer exists")
		}
		return winner, nil
	}
	return session, nil
}

// Get retrieves a session by ID.
func (r *PostgresUserSessionRepository) Get(ctx context.Context, sessionID id.SessionID) (*storage.UserSession, error) {
	if sessionID.IsZero() {
		return nil, errors.New("session ID cannot be empty")
	}

	var rec userSessionRecord
	query := `SELECT ` + userSessionColumns + ` FROM user_sessions WHERE id = $1`

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	err := r.adapter.db.GetContext(queryCtx, &rec, query, sessionID)
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
	query := `SELECT ` + userSessionColumns + ` FROM user_sessions WHERE principal = $1 AND service_id = $2`

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	err := r.adapter.db.GetContext(queryCtx, &rec, query, principal, serviceID)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is not an error
	}
	if err != nil {
		return nil, r.wrapError(err, "FindByPrincipalAndService")
	}
	return recordToSession(&rec), nil
}

// WithLockedSession re-reads one session under a row lock and commits token changes atomically.
// A new callback generation may also replace the established upstream client identity.
func (r *PostgresUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("WithLockedSession", storage.ErrorKindConnection, nil, "database not initialized")
	}

	acquireCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	// Bound every trigger before it can occupy the shared pool, including retry waits.
	select {
	case r.adapter.lockedSessionSlots <- struct{}{}:
		defer func() { <-r.adapter.lockedSessionSlots }()
	case <-acquireCtx.Done():
		return nil, r.wrapError(acquireCtx.Err(), "WithLockedSession")
	}
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
	err = tx.GetContext(readCtx, &rec, `SELECT `+userSessionColumns+` FROM user_sessions WHERE principal = $1 AND service_id = $2 FOR UPDATE`, principal, serviceID)
	cancel()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, r.wrapError(err, "WithLockedSession")
	}
	session := recordToSession(&rec)
	upstreamClientID := session.UpstreamClientID
	updated, err := refresh(ctx, session)
	if err != nil {
		return nil, err
	}
	if session.CallbackID == rec.CallbackID {
		session.UpstreamClientID = upstreamClientID
		if upstreamClientID != nil {
			*upstreamClientID = id.ClientID(*rec.UpstreamClientID)
		}
	}
	if updated {
		if err := session.Validate(); err != nil {
			return nil, err
		}
		writeCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
		_, err = tx.ExecContext(writeCtx, `UPDATE user_sessions SET encrypted_access_token = $1, encrypted_refresh_token = $2,
			access_token_expires_at = $3, refresh_token_expires_at = $4, token_type = $5, scope = $6,
			encryption_context = $7, updated_at = $8, callback_id = $9, upstream_client_id = $10 WHERE id = $11`,
			session.EncryptedAccessToken, session.EncryptedRefreshToken, session.AccessTokenExpiresAt,
			session.RefreshTokenExpiresAt, session.TokenType, pq.Array(session.Scope), session.EncryptionContext,
			session.UpdatedAt, session.CallbackID, session.UpstreamClientID, session.ID)
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
	query := `SELECT ` + userSessionColumns + ` FROM user_sessions WHERE principal = $1 ORDER BY created_at DESC`

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	err := r.adapter.db.SelectContext(queryCtx, &records, query, principal)
	if err != nil && err != sql.ErrNoRows {
		return nil, r.wrapError(err, "ListByPrincipal")
	}
	sessions := make([]*storage.UserSession, len(records))
	for i, rec := range records {
		sessions[i] = recordToSession(rec)
	}
	return sessions, nil
}

func (r *PostgresUserSessionRepository) ListSummariesByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSessionSummary, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var records []userSessionSummaryRecord
	err := r.adapter.db.SelectContext(queryCtx, &records, `SELECT id, service_id, token_type, scope, initiated_at,
		access_token_expires_at, refresh_token_expires_at,
		COALESCE(octet_length(encrypted_refresh_token), 0) > 0 AS has_refresh_token
		FROM user_sessions WHERE principal = $1 ORDER BY created_at DESC`, principal)
	if err != nil {
		return nil, r.wrapError(err, "ListSummariesByPrincipal")
	}
	summaries := make([]*storage.UserSessionSummary, 0, len(records))
	for _, record := range records {
		session := storage.UserSession{
			ID: record.ID, ServiceID: record.ServiceID, TokenType: record.TokenType,
			Scope: []string(record.Scope), InitiatedAt: record.InitiatedAt,
			AccessTokenExpiresAt: record.AccessTokenExpiresAt, RefreshTokenExpiresAt: record.RefreshTokenExpiresAt,
		}
		if session.Scope == nil {
			session.Scope = []string{}
		}
		summary := storage.NewUserSessionSummary(&session, "", 0)
		summary.HasRefreshToken = record.HasRefreshToken
		summary.IsExpired = !storage.SessionIsUsable(record.AccessTokenExpiresAt, record.RefreshTokenExpiresAt, record.HasRefreshToken)
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (r *PostgresUserSessionRepository) ListActiveServiceIDsByPrincipal(ctx context.Context, principal id.Principal) ([]id.ServiceID, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	ids := make([]id.ServiceID, 0)
	err := r.adapter.db.SelectContext(queryCtx, &ids, `SELECT service_id FROM user_sessions WHERE principal = $1
		AND `+usableSessionPredicate+` ORDER BY created_at DESC`, principal)
	if err != nil {
		return nil, r.wrapError(err, "ListActiveServiceIDsByPrincipal")
	}
	return ids, nil
}

// ListActiveByPrincipal retrieves only non-expired sessions for a principal.
// Used for FR-020 consent submission validation.
func (r *PostgresUserSessionRepository) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	var records []*userSessionRecord
	query := `SELECT ` + userSessionColumns + ` FROM user_sessions WHERE principal = $1 AND ` + usableSessionPredicate + ` ORDER BY created_at DESC`

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	err := r.adapter.db.SelectContext(queryCtx, &records, query, principal)
	if err != nil && err != sql.ErrNoRows {
		return nil, r.wrapError(err, "ListActiveByPrincipal")
	}
	sessions := make([]*storage.UserSession, len(records))
	for i, rec := range records {
		sessions[i] = recordToSession(rec)
	}
	return sessions, nil
}

// ListExpiringSessions lists sessions due for the admin sweep.
func (r *PostgresUserSessionRepository) ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int) ([]*storage.UserSession, error) {
	if limit < 1 || limit > 1000 {
		return nil, storage.NewStorageError("ListExpiringSessions", storage.ErrorKindValidation, nil, "limit must be between 1 and 1000")
	}
	if cursor.ID.IsZero() && !cursor.AccessTokenExpiresAt.IsZero() {
		return nil, storage.NewStorageError("ListExpiringSessions", storage.ErrorKindValidation, nil, "cursor expiry requires a session ID")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var records []userSessionRecord
	var err error
	if cursor.ID.IsZero() {
		err = r.adapter.db.SelectContext(queryCtx, &records, listExpiringSessionsFirstPageQuery, threshold, limit)
	} else {
		err = r.adapter.db.SelectContext(queryCtx, &records, listExpiringSessionsAfterCursorQuery, threshold, cursor.AccessTokenExpiresAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, r.wrapError(err, "ListExpiringSessions")
	}

	sessions := make([]*storage.UserSession, len(records))
	for i := range records {
		sessions[i] = recordToSession(&records[i])
	}
	return sessions, nil
}

// Delete deletes a session by ID.
func (r *PostgresUserSessionRepository) Delete(ctx context.Context, sessionID id.SessionID) error {
	if sessionID.IsZero() {
		return errors.New("session ID cannot be empty")
	}

	query := `DELETE FROM user_sessions WHERE id = $1`
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	_, err := r.adapter.db.ExecContext(execCtx, query, sessionID)
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
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	_, err := r.adapter.db.ExecContext(execCtx, query, principal, serviceID)
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

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	err := r.adapter.db.GetContext(queryCtx, &count, query, serviceID)
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
	if errors.Is(err, context.DeadlineExceeded) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "timeout")
	}
	return storage.NewStorageError(operation, storage.ErrorKindUnknown, err, err.Error())
}
