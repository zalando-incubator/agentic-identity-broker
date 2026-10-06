package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

var (
	_ ports.RefreshSessionRepository            = (*RefreshSessionRepo)(nil)
	_ ports.RefreshSessionRevocationRepository  = (*RefreshSessionRepo)(nil)
	_ ports.RefreshSessionMaintenanceRepository = (*RefreshSessionRepo)(nil)
	_ ports.RefreshTokenRepository              = (*RefreshTokenRepo)(nil)
)

type RefreshSessionRepo struct{ adapter *Adapter }
type RefreshTokenRepo struct{ adapter *Adapter }

func NewRefreshSessionRepo(adapter *Adapter) *RefreshSessionRepo {
	return &RefreshSessionRepo{adapter: adapter}
}
func NewRefreshTokenRepo(adapter *Adapter) *RefreshTokenRepo {
	return &RefreshTokenRepo{adapter: adapter}
}

type refreshSessionRecord struct {
	ID                                id.RefreshSessionID `db:"id"`
	OriginalGrantID                   id.GrantID          `db:"original_grant_id"`
	OriginalTokenSignature            string              `db:"original_token_signature"`
	AgentID                           id.AgentID          `db:"agent_id"`
	Principal                         id.Principal        `db:"principal"`
	ClientID                          id.ClientID         `db:"client_id"`
	Scope                             string              `db:"scope"`
	Email                             *string             `db:"email"`
	DisplayName                       string              `db:"display_name"`
	StartedAt                         time.Time           `db:"started_at"`
	LastFreshAt                       time.Time           `db:"last_fresh_at"`
	AbsoluteExpiresAt                 *time.Time          `db:"absolute_expires_at"`
	InactivityExpiresAt               time.Time           `db:"inactivity_expires_at"`
	RetainUntil                       time.Time           `db:"retain_until"`
	BranchKeyID                       string              `db:"branch_key_id"`
	CurrentSignature                  string              `db:"current_signature"`
	PreviousSignature                 *string             `db:"previous_signature"`
	PreviousConsumedAt                *time.Time          `db:"previous_consumed_at"`
	ReuseUntil                        *time.Time          `db:"reuse_until"`
	OriginalRequestedScope            *string             `db:"original_requested_scope"`
	OriginalRequestContextFingerprint *string             `db:"original_request_context_fingerprint"`
	RetryCiphertext                   []byte              `db:"retry_ciphertext"`
	RetryAccessExpiresAt              *time.Time          `db:"retry_access_expires_at"`
	RetryExpiresAt                    *time.Time          `db:"retry_expires_at"`
	RetryCount                        int                 `db:"retry_count"`
	RevokedAt                         *time.Time          `db:"revoked_at"`
	ExpiredAt                         *time.Time          `db:"expired_at"`
	TerminalReason                    *string             `db:"terminal_reason"`
	LineageValid                      bool                `db:"lineage_valid"`
}

func (r *refreshSessionRecord) model() *storage.RefreshSession {
	s := &storage.RefreshSession{
		ID: r.ID, OriginalGrantID: r.OriginalGrantID, OriginalTokenSignature: r.OriginalTokenSignature,
		AgentID: r.AgentID, Principal: r.Principal, ClientID: r.ClientID, Scope: r.Scope,
		Email: r.Email, DisplayName: r.DisplayName, StartedAt: r.StartedAt.UTC(), LastFreshAt: r.LastFreshAt.UTC(),
		AbsoluteExpiresAt: r.AbsoluteExpiresAt, InactivityExpiresAt: r.InactivityExpiresAt.UTC(),
		RetainUntil: r.RetainUntil.UTC(), BranchKeyID: r.BranchKeyID, CurrentSignature: r.CurrentSignature,
		PreviousSignature: r.PreviousSignature, PreviousConsumedAt: r.PreviousConsumedAt, ReuseUntil: r.ReuseUntil,
		OriginalRequestedScope: r.OriginalRequestedScope, OriginalRequestContextFingerprint: r.OriginalRequestContextFingerprint,
		RetryCiphertext: r.RetryCiphertext, RetryAccessExpiresAt: r.RetryAccessExpiresAt,
		RetryExpiresAt: r.RetryExpiresAt, RetryCount: r.RetryCount, RevokedAt: r.RevokedAt, ExpiredAt: r.ExpiredAt,
	}
	for _, at := range [...]*time.Time{s.AbsoluteExpiresAt, s.PreviousConsumedAt, s.ReuseUntil, s.RetryAccessExpiresAt, s.RetryExpiresAt, s.RevokedAt, s.ExpiredAt} {
		if at != nil {
			*at = at.UTC()
		}
	}
	if r.TerminalReason != nil {
		reason := storage.RefreshRevocationReason(*r.TerminalReason)
		s.TerminalReason = &reason
	}
	return s
}

func refreshSessionArgs(s *storage.RefreshSession) []any {
	return []any{s.ID, s.OriginalGrantID, s.OriginalTokenSignature, s.AgentID, s.Principal, s.ClientID,
		s.Scope, s.Email, s.DisplayName, s.StartedAt, s.LastFreshAt, s.AbsoluteExpiresAt,
		s.InactivityExpiresAt, s.RetainUntil, s.BranchKeyID, s.CurrentSignature, s.PreviousSignature,
		s.PreviousConsumedAt, s.ReuseUntil, s.OriginalRequestedScope, s.OriginalRequestContextFingerprint,
		s.RetryCiphertext, s.RetryAccessExpiresAt, s.RetryExpiresAt, s.RetryCount, s.RevokedAt,
		s.ExpiredAt, s.TerminalReason}
}

func refreshStoreError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return storage.NewStorageError(op, storage.ErrorKindNotFound, ports.ErrNotFound, "refresh record not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503") {
		return storage.NewStorageError(op, storage.ErrorKindConflict, err, "refresh record conflicts with existing state")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return storage.NewStorageError(op, storage.ErrorKindTimeout, err, "refresh storage deadline exceeded")
	}
	return storage.NewStorageError(op, storage.ErrorKindConnection, err, "refresh storage operation failed")
}

func refreshValidation(op string, err error) error {
	return storage.NewStorageError(op, storage.ErrorKindValidation, err, "invalid refresh session transition")
}

func (r *RefreshSessionRepo) Create(ctx context.Context, s *storage.RefreshSession) error {
	const op = "RefreshSession.Create"
	if err := s.Validate(); err != nil {
		return refreshValidation(op, err)
	}
	if err := requireAuthorizationAgent(ctx, s.AgentID); err != nil {
		return err
	}
	if s.PreviousSignature != nil || s.TerminalReason != nil || s.RetryCount != 0 {
		return refreshValidation(op, errors.New("first issuance requires an active root without predecessor"))
	}
	scope, _ := scopedAuthorization(ctx)
	if s.StartedAt.After(scope.latestTime) {
		return refreshValidation(op, errors.New("first issuance cannot be in the future of the shared clock"))
	}
	var grantValid bool
	err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_grants WHERE id = $1 AND agent_id = $2 AND principal = $3
			AND (valid_until IS NULL OR valid_until > $4))`,
		s.OriginalGrantID, s.AgentID, s.Principal, scope.latestTime).Scan(&grantValid)
	if err != nil {
		return refreshStoreError(op, err)
	}
	if !grantValid {
		return refreshValidation(op, errors.New("original active grant and owner not found"))
	}
	_, err = r.adapter.storageExecutor(ctx).ExecContext(ctx, `INSERT INTO refresh_sessions (
		id, original_grant_id, original_token_signature, agent_id, principal, client_id,
		scope, email, display_name, started_at, last_fresh_at, absolute_expires_at,
		inactivity_expires_at, retain_until, branch_key_id, current_signature, previous_signature,
		previous_consumed_at, reuse_until, original_requested_scope, original_request_context_fingerprint,
		retry_ciphertext, retry_access_expires_at, retry_expires_at, retry_count, revoked_at,
		expired_at, terminal_reason) VALUES (
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`,
		refreshSessionArgs(s)...)
	return refreshStoreError(op, err)
}

func loadRefreshSession(ctx context.Context, exec sqlx.ExtContext, sessionID id.RefreshSessionID, lock bool) (*storage.RefreshSession, error) {
	query := `SELECT * FROM refresh_sessions WHERE id = $1`
	args := []any{sessionID}
	if scope, ok := scopedAuthorization(ctx); ok {
		query += ` AND agent_id = $2`
		args = append(args, scope.agentID)
	}
	if lock {
		query += ` FOR UPDATE`
	}
	var record refreshSessionRecord
	if err := sqlx.GetContext(ctx, exec, &record, query, args...); err != nil {
		return nil, refreshStoreError("RefreshSession.FindByID", err)
	}
	return record.model(), nil
}

func (r *RefreshSessionRepo) FindByID(ctx context.Context, sessionID id.RefreshSessionID) (*storage.RefreshSession, error) {
	if r.adapter == nil || r.adapter.db == nil {
		return nil, refreshStoreError("RefreshSession.FindByID", errors.New("database not initialized"))
	}
	return loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), sessionID, false)
}

func (r *RefreshSessionRepo) Save(ctx context.Context, s *storage.RefreshSession) error {
	const op = "RefreshSession.Save"
	if err := s.Validate(); err != nil {
		return refreshValidation(op, err)
	}
	if err := requireAuthorizationAgent(ctx, s.AgentID); err != nil {
		return err
	}
	previous, err := loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), s.ID, false)
	if err != nil {
		return err
	}
	at, err := NewAuthorizationSessionCoordinator(r.adapter).Now(ctx)
	if err != nil {
		return err
	}
	if err := s.ValidateTransition(previous, at); err != nil {
		return refreshValidation(op, err)
	}
	if s.CurrentSignature != previous.CurrentSignature {
		var issuedAt time.Time
		var usedAt *time.Time
		err = r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
			`SELECT issued_at, used_at FROM refresh_tokens WHERE session_id = $1 AND signature = $2`,
			s.ID, s.CurrentSignature).Scan(&issuedAt, &usedAt)
		if err != nil || usedAt != nil || !issuedAt.Equal(s.LastFreshAt) {
			return refreshValidation(op, errors.New("successor must be unconsumed and issued at the fresh rotation"))
		}
		var consumedAt *time.Time
		err = r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
			`SELECT used_at FROM refresh_tokens WHERE session_id = $1 AND signature = $2`,
			s.ID, previous.CurrentSignature).Scan(&consumedAt)
		if err != nil || consumedAt == nil || !consumedAt.Equal(s.LastFreshAt) {
			return refreshValidation(op, errors.New("predecessor must be consumed at the successor issuance time"))
		}
		var childExpiresAt time.Time
		if err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
			`SELECT expires_at FROM refresh_tokens WHERE session_id = $1 AND signature = $2`,
			s.ID, s.CurrentSignature).Scan(&childExpiresAt); err != nil {
			return refreshStoreError(op, err)
		}
		if !s.InactivityExpiresAt.Equal(childExpiresAt) || s.RetainUntil.Before(childExpiresAt) {
			return refreshValidation(op, errors.New("fresh rotation must retain its successor through its issued expiry"))
		}
	}
	if s.TerminalReason == nil && (s.CurrentSignature != previous.CurrentSignature || s.RetryCount > previous.RetryCount) {
		if err := checkAnchoredCurrent(ctx, r.adapter.storageExecutor(ctx), s); err != nil {
			return err
		}
	}
	if s.RetryCount > previous.RetryCount {
		if len(s.RetryCiphertext) == 0 || s.RetryExpiresAt == nil || !at.Before(*s.RetryExpiresAt) ||
			s.RetryAccessExpiresAt == nil || !at.Before(*s.RetryAccessExpiresAt) ||
			s.ReuseUntil == nil || !at.Before(*s.ReuseUntil) {
			return refreshValidation(op, errors.New("stored retry result or its deadline is unavailable"))
		}
		var currentExpiry time.Time
		if err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
			`SELECT expires_at FROM refresh_tokens WHERE session_id = $1 AND signature = $2 AND used_at IS NULL`,
			s.ID, s.CurrentSignature).Scan(&currentExpiry); err != nil {
			return refreshStoreError(op, err)
		}
		if !at.Before(currentExpiry) {
			return refreshValidation(op, errors.New("retry successor has expired"))
		}
	}
	if s.TerminalReason != nil && previous.TerminalReason == nil {
		terminalAt := s.RevokedAt
		if terminalAt == nil {
			terminalAt = s.ExpiredAt
		}
		if terminalAt == nil {
			return refreshValidation(op, errors.New("terminal timestamp required"))
		}
		if err := writeRefreshReceipt(ctx, r.adapter.storageExecutor(ctx), s, *terminalAt); err != nil {
			return err
		}
		if err := invalidateRefreshLineage(ctx, r.adapter.storageExecutor(ctx), s, *terminalAt); err != nil {
			return err
		}
	}
	return updateRefreshSession(ctx, r.adapter.storageExecutor(ctx), s)
}

func updateRefreshSession(ctx context.Context, exec sqlx.ExtContext, s *storage.RefreshSession) error {
	const op = "RefreshSession.Save"
	result, err := exec.ExecContext(ctx, `UPDATE refresh_sessions SET
		last_fresh_at = $2, absolute_expires_at = $3, inactivity_expires_at = $4, retain_until = $5,
		current_signature = $6, previous_signature = $7, previous_consumed_at = $8, reuse_until = $9,
		original_requested_scope = $10, original_request_context_fingerprint = $11,
		retry_ciphertext = $12, retry_access_expires_at = $13, retry_expires_at = $14,
		retry_count = $15, revoked_at = $16, expired_at = $17, terminal_reason = $18 WHERE id = $1`,
		s.ID, s.LastFreshAt, s.AbsoluteExpiresAt, s.InactivityExpiresAt, s.RetainUntil, s.CurrentSignature,
		s.PreviousSignature, s.PreviousConsumedAt, s.ReuseUntil, s.OriginalRequestedScope,
		s.OriginalRequestContextFingerprint, s.RetryCiphertext, s.RetryAccessExpiresAt, s.RetryExpiresAt,
		s.RetryCount, s.RevokedAt, s.ExpiredAt, s.TerminalReason)
	if err != nil {
		return refreshStoreError(op, err)
	}
	return checkRowsAffected(op, result, "refresh session not found")
}

func writeRefreshReceipt(ctx context.Context, exec sqlx.ExtContext, s *storage.RefreshSession, at time.Time) error {
	if s.TerminalReason == nil {
		return refreshValidation("RefreshSession.Revoke", errors.New("revocation reason required"))
	}
	contextJSON, err := json.Marshal(security.RedactedAuditContext(ctx))
	if err != nil {
		return refreshStoreError("RefreshSession.Receipt", err)
	}
	_, err = exec.ExecContext(ctx, `INSERT INTO refresh_revocation_receipts
		(session_id, agent_id, principal, client_id, reason, "at", redacted_context)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (session_id, reason) DO NOTHING`,
		s.ID, s.AgentID, s.Principal, s.ClientID, *s.TerminalReason, at, contextJSON)
	return refreshStoreError("RefreshSession.Receipt", err)
}

func invalidateRefreshLineage(ctx context.Context, exec sqlx.ExtContext, s *storage.RefreshSession, at time.Time) error {
	// An old writer must consume its current row before inserting a descendant. Locking
	// every matching row waits for that writer, then a fresh statement sees its descendant.
	var legacy []struct {
		Signature string    `db:"signature"`
		ExpiresAt time.Time `db:"expires_at"`
	}
	err := sqlx.SelectContext(ctx, exec, &legacy, `SELECT signature, expires_at
		FROM refresh_token_sessions WHERE agent_id = $2 AND (session_id = $1 OR request_id = $3)
		ORDER BY signature FOR UPDATE`, s.ID, s.AgentID, s.ID.String())
	if err != nil {
		return refreshStoreError("RefreshSession.Revoke", err)
	}
	for _, row := range legacy {
		if row.ExpiresAt.After(s.RetainUntil) {
			s.RetainUntil = row.ExpiresAt.UTC()
		}
	}
	_, err = exec.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = $4
		WHERE agent_id = $2 AND (session_id = $1 OR request_id = $3) AND used_at IS NULL`,
		s.ID, s.AgentID, s.ID.String(), at)
	if err != nil {
		return refreshStoreError("RefreshSession.Revoke", err)
	}
	var latestExpiry *time.Time
	if err := exec.QueryRowxContext(ctx,
		`SELECT MAX(expires_at) FROM refresh_token_sessions
		WHERE agent_id = $2 AND (session_id = $1 OR request_id = $3)`,
		s.ID, s.AgentID, s.ID.String()).Scan(&latestExpiry); err != nil {
		return refreshStoreError("RefreshSession.Revoke", err)
	}
	if latestExpiry != nil && latestExpiry.After(s.RetainUntil) {
		s.RetainUntil = latestExpiry.UTC()
	}
	_, err = exec.ExecContext(ctx, `UPDATE refresh_tokens SET used_at = $2 WHERE session_id = $1 AND used_at IS NULL`, s.ID, at)
	return refreshStoreError("RefreshSession.Revoke", err)
}

func (r *RefreshSessionRepo) revoke(ctx context.Context, s *storage.RefreshSession, at time.Time, reason storage.RefreshRevocationReason) error {
	// The agent gate owns native root state. Fence mirrors before the root write
	// so an old writer can commit its deferred proof invalidation without a cycle.
	if s.TerminalReason != nil {
		if reason != storage.RefreshReasonRestoreInvalidation || len(s.RetryCiphertext) == 0 {
			return nil
		}
		s.RetryCiphertext = nil
		return updateRefreshSession(ctx, r.adapter.storageExecutor(ctx), s)
	}
	s.TerminalReason = &reason
	if reason == storage.RefreshReasonAbsoluteExpiry || reason == storage.RefreshReasonInactivityExpiry {
		s.ExpiredAt = &at
	} else {
		s.RevokedAt = &at
	}
	s.RetryCiphertext = nil
	if err := writeRefreshReceipt(ctx, r.adapter.storageExecutor(ctx), s, at); err != nil {
		return err
	}
	if err := invalidateRefreshLineage(ctx, r.adapter.storageExecutor(ctx), s, at); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return refreshValidation("RefreshSession.Revoke", err)
	}
	return updateRefreshSession(ctx, r.adapter.storageExecutor(ctx), s)
}

func (r *RefreshSessionRepo) ListActiveByAgent(ctx context.Context, agentID id.AgentID, principal *id.Principal) ([]storage.RefreshSessionAuditIdentity, error) {
	if err := requireAuthorizationAgent(ctx, agentID); err != nil {
		return nil, err
	}
	query := `SELECT id, agent_id, principal, client_id FROM refresh_sessions WHERE agent_id = $1 AND terminal_reason IS NULL`
	args := []any{agentID}
	if principal != nil {
		query += ` AND principal = $2`
		args = append(args, *principal)
	}
	query += ` ORDER BY id`
	rows, err := r.adapter.storageExecutor(ctx).QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, refreshStoreError("RefreshSession.ListActiveByAgent", err)
	}
	defer func() { _ = rows.Close() }()
	var identities []storage.RefreshSessionAuditIdentity
	for rows.Next() {
		var identity storage.RefreshSessionAuditIdentity
		if err := rows.Scan(&identity.ID, &identity.AgentID, &identity.Principal, &identity.ClientID); err != nil {
			return nil, refreshStoreError("RefreshSession.ListActiveByAgent", err)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, refreshStoreError("RefreshSession.ListActiveByAgent", err)
	}
	return identities, nil
}

func (r *RefreshSessionRepo) RevokeByID(ctx context.Context, sessionID id.RefreshSessionID, at time.Time, reason storage.RefreshRevocationReason) error {
	if _, scoped := scopedAuthorization(ctx); !scoped {
		return refreshValidation("RefreshSession.RevokeByID", errors.New("agent-scoped authorization transaction required"))
	}
	s, err := loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), sessionID, false)
	if err != nil {
		return err
	}
	if err := requireAuthorizationAgent(ctx, s.AgentID); err != nil {
		return err
	}
	if err := validateRefreshRevocationTime(ctx, at); err != nil {
		return err
	}
	return r.revoke(ctx, s, at, reason)
}

func validateRefreshRevocationTime(ctx context.Context, at time.Time) error {
	scope, ok := scopedAuthorization(ctx)
	if !ok || at.IsZero() || (!at.Equal(scope.latestTime) && !at.Equal(scope.decisionTime)) {
		return refreshValidation("RefreshSession.Revoke", errors.New("revocation requires shared authorization time"))
	}
	return nil
}

func (r *RefreshSessionRepo) RevokeByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	if err := requireAuthorizationAgent(ctx, agentID); err != nil {
		return err
	}
	if err := validateRefreshRevocationTime(ctx, at); err != nil {
		return err
	}
	return r.revokeMatching(ctx, agentID, &principal, at, reason)
}

func (r *RefreshSessionRepo) RevokeByAgent(ctx context.Context, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	if err := requireAuthorizationAgent(ctx, agentID); err != nil {
		return err
	}
	if err := validateRefreshRevocationTime(ctx, at); err != nil {
		return err
	}
	return r.revokeMatching(ctx, agentID, nil, at, reason)
}

func (r *RefreshSessionRepo) revokeMatching(ctx context.Context, agentID id.AgentID, principal *id.Principal, at time.Time, reason storage.RefreshRevocationReason) error {
	query := `SELECT * FROM refresh_sessions WHERE agent_id = $1`
	args := []any{agentID}
	if principal != nil {
		query += ` AND principal = $2`
		args = append(args, *principal)
	}
	query += ` ORDER BY id`
	var records []refreshSessionRecord
	if err := sqlx.SelectContext(ctx, r.adapter.storageExecutor(ctx), &records, query, args...); err != nil {
		return refreshStoreError("RefreshSession.Revoke", err)
	}
	for i := range records {
		if err := r.revoke(ctx, records[i].model(), at, reason); err != nil {
			return err
		}
	}
	query = `UPDATE refresh_token_sessions SET used_at = $2 WHERE agent_id = $1 AND used_at IS NULL`
	args = []any{agentID, at}
	if principal != nil {
		query += ` AND principal = $3`
		args = append(args, *principal)
	}
	_, err := r.adapter.storageExecutor(ctx).ExecContext(ctx, query, args...)
	return refreshStoreError("RefreshSession.RevokeLegacy", err)
}

func boundedRefreshLimit(limit int) error {
	if limit < 1 || limit > 1000 {
		return refreshValidation("RefreshSession.List", errors.New("limit must be between 1 and 1000"))
	}
	return nil
}

func (r *RefreshSessionRepo) ListAgentIDs(ctx context.Context, afterID id.AgentID, limit int) ([]id.AgentID, error) {
	if err := boundedRefreshLimit(limit); err != nil {
		return nil, err
	}
	var ids []id.AgentID
	err := sqlx.SelectContext(ctx, r.adapter.storageExecutor(ctx), &ids,
		`SELECT agent_id FROM (
			SELECT agent_id FROM refresh_sessions WHERE agent_id > $1
			UNION SELECT agent_id FROM refresh_token_sessions WHERE agent_id > $1
		) AS agents ORDER BY agent_id LIMIT $2`, afterID, limit)
	return ids, refreshStoreError("RefreshSession.ListAgentIDs", err)
}

func (r *RefreshSessionRepo) ListActive(ctx context.Context, afterID id.RefreshSessionID, limit int) ([]*storage.RefreshSession, error) {
	return r.listSessions(ctx, `WHERE id > $1 AND terminal_reason IS NULL ORDER BY id LIMIT $2`, afterID, limit)
}

func (r *RefreshSessionRepo) ListDue(ctx context.Context, at time.Time, limit int) ([]*storage.RefreshSession, error) {
	return r.listSessions(ctx, `WHERE terminal_reason IS NULL AND
		(inactivity_expires_at <= $1 OR absolute_expires_at <= $1 OR
		(retry_ciphertext IS NOT NULL AND retry_expires_at <= $1)) ORDER BY id LIMIT $2`, at, limit)
}

func (r *RefreshSessionRepo) HasRemainingAuthority(ctx context.Context) (bool, error) {
	var remaining bool
	err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM refresh_sessions WHERE terminal_reason IS NULL OR retry_ciphertext IS NOT NULL
		UNION ALL SELECT 1 FROM refresh_token_sessions WHERE used_at IS NULL
	)`).Scan(&remaining)
	return remaining, refreshStoreError("RefreshSession.HasRemainingAuthority", err)
}

func (r *RefreshSessionRepo) listSessions(ctx context.Context, condition string, value any, limit int) ([]*storage.RefreshSession, error) {
	if err := boundedRefreshLimit(limit); err != nil {
		return nil, err
	}
	var records []refreshSessionRecord
	err := sqlx.SelectContext(ctx, r.adapter.storageExecutor(ctx), &records,
		`SELECT * FROM refresh_sessions `+condition, value, limit)
	if err != nil {
		return nil, refreshStoreError("RefreshSession.List", err)
	}
	result := make([]*storage.RefreshSession, 0, len(records))
	for i := range records {
		s := records[i].model()
		if scope, ok := scopedAuthorization(ctx); ok && s.AgentID != scope.agentID {
			return nil, refreshValidation("RefreshSession.List", errors.New("refresh session belongs to another agent"))
		}
		result = append(result, s)
	}
	return result, nil
}

func (r *RefreshSessionRepo) DeleteTerminal(ctx context.Context, before time.Time, limit int) (int, error) {
	if err := boundedRefreshLimit(limit); err != nil {
		return 0, err
	}
	if _, scoped := scopedAuthorization(ctx); scoped {
		return 0, refreshValidation("RefreshSession.DeleteTerminal", errors.New("retention cleanup must not delete other agents inside a scoped transaction"))
	}
	var count int
	err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
		`WITH doomed AS (
			SELECT id, agent_id FROM refresh_sessions
			WHERE terminal_reason IS NOT NULL AND retain_until <= $1
			ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED
		), legacy AS (
			DELETE FROM refresh_token_sessions bridge USING doomed
			WHERE bridge.session_id IS NULL AND bridge.agent_id = doomed.agent_id
			AND bridge.request_id = doomed.id::text
		), rootless AS (
			SELECT signature FROM refresh_token_sessions bridge
			WHERE bridge.session_id IS NULL AND bridge.expires_at <= $1
			AND NOT EXISTS (SELECT 1 FROM refresh_sessions root
				WHERE root.id = CASE WHEN bridge.request_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
					THEN bridge.request_id::uuid END
				AND root.agent_id = bridge.agent_id AND root.id::text = bridge.request_id)
			ORDER BY expires_at, signature LIMIT $2 FOR UPDATE SKIP LOCKED
		), expired_legacy AS (
			DELETE FROM refresh_token_sessions bridge USING rootless
			WHERE bridge.signature = rootless.signature RETURNING bridge.signature
		), expired_roots AS (
			DELETE FROM refresh_sessions root USING doomed WHERE root.id = doomed.id RETURNING root.id
		)
		SELECT (SELECT count(*) FROM expired_roots) + (SELECT count(*) FROM expired_legacy)`, before, limit).Scan(&count)
	return count, refreshStoreError("RefreshSession.DeleteTerminal", err)
}

type refreshTokenRecord struct {
	Signature string              `db:"signature"`
	SessionID id.RefreshSessionID `db:"session_id"`
	IssuedAt  time.Time           `db:"issued_at"`
	ExpiresAt time.Time           `db:"expires_at"`
	UsedAt    *time.Time          `db:"used_at"`
}

func (r *RefreshTokenRepo) Create(ctx context.Context, token *storage.RefreshToken) error {
	const op = "RefreshToken.Create"
	if err := token.Validate(); err != nil {
		return refreshValidation(op, err)
	}
	root, err := loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), token.SessionID, false)
	if err != nil {
		return err
	}
	if err := requireAuthorizationAgent(ctx, root.AgentID); err != nil {
		return err
	}
	if root.TerminalReason != nil || token.UsedAt != nil || !token.ExpiresAt.After(token.IssuedAt) {
		return refreshValidation(op, errors.New("only an active, unconsumed child can be issued"))
	}
	var predecessor *string
	if token.Signature == root.OriginalTokenSignature {
		if root.CurrentSignature != token.Signature || !token.IssuedAt.Equal(root.StartedAt) || !token.ExpiresAt.Equal(root.RetainUntil) || !token.ExpiresAt.Equal(root.InactivityExpiresAt) {
			return refreshValidation(op, errors.New("first token must bind the original root and issuance time"))
		}
	} else {
		if !token.IssuedAt.After(root.LastFreshAt) {
			return refreshValidation(op, errors.New("successor issuance must advance fresh time"))
		}
		predecessor = &root.CurrentSignature
		var used *time.Time
		err = r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
			`SELECT used_at FROM refresh_tokens WHERE session_id = $1 AND signature = $2`,
			token.SessionID, *predecessor).Scan(&used)
		if err != nil || used == nil || !used.Equal(token.IssuedAt) {
			return refreshValidation(op, errors.New("successor requires the current predecessor consumed at issuance"))
		}
		var legacyUsed *time.Time
		err = r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
			`SELECT used_at FROM refresh_token_sessions WHERE signature = $1 AND session_id = $2
			AND request_id = $3 AND agent_id = $4 AND principal = $5 AND client_id = $6`,
			*predecessor, root.ID, root.ID.String(), root.AgentID, root.Principal, root.ClientID).Scan(&legacyUsed)
		if err != nil || legacyUsed == nil || !legacyUsed.Equal(token.IssuedAt) {
			return refreshValidation(op, errors.New("successor requires matching anchored predecessor consumption"))
		}
	}
	_, err = r.adapter.storageExecutor(ctx).ExecContext(ctx,
		`INSERT INTO refresh_tokens (signature, session_id, issued_at, expires_at, used_at) VALUES ($1,$2,$3,$4,$5)`,
		token.Signature, token.SessionID, token.IssuedAt, token.ExpiresAt, token.UsedAt)
	if err != nil {
		return refreshStoreError(op, err)
	}
	_, err = r.adapter.storageExecutor(ctx).ExecContext(ctx,
		`INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at, used_at, created_at,
		email, display_name, session_id, predecessor_signature)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULL,$8,$9,$10,$11,$12)`,
		token.Signature, root.ID.String(), root.AgentID, root.ClientID, root.Principal, root.Scope,
		token.ExpiresAt, token.IssuedAt, root.Email, root.DisplayName, root.ID, predecessor)
	return refreshStoreError(op, err)
}

func (r *RefreshTokenRepo) FindBySignature(ctx context.Context, signature string) (*storage.RefreshToken, error) {
	if r.adapter == nil || r.adapter.db == nil {
		return nil, refreshStoreError("RefreshToken.FindBySignature", errors.New("database not initialized"))
	}
	var record refreshTokenRecord
	if err := sqlx.GetContext(ctx, r.adapter.storageExecutor(ctx), &record,
		`SELECT signature, session_id, issued_at, expires_at, used_at FROM refresh_tokens WHERE signature = $1`, signature); err != nil {
		return nil, refreshStoreError("RefreshToken.FindBySignature", err)
	}
	if _, scoped := scopedAuthorization(ctx); scoped {
		root, err := loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), record.SessionID, false)
		if err != nil {
			return nil, err
		}
		if err := requireAuthorizationAgent(ctx, root.AgentID); err != nil {
			return nil, err
		}
	}
	if record.UsedAt != nil {
		*record.UsedAt = record.UsedAt.UTC()
	}
	return &storage.RefreshToken{Signature: record.Signature, SessionID: record.SessionID,
		IssuedAt: record.IssuedAt.UTC(), ExpiresAt: record.ExpiresAt.UTC(), UsedAt: record.UsedAt}, nil
}

func (r *RefreshTokenRepo) CheckCurrentLineage(ctx context.Context, sessionID id.RefreshSessionID) error {
	root, err := loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), sessionID, false)
	if err != nil {
		return err
	}
	if err := requireAuthorizationAgent(ctx, root.AgentID); err != nil {
		return err
	}
	return checkAnchoredCurrent(ctx, r.adapter.storageExecutor(ctx), root)
}

func checkAnchoredCurrent(ctx context.Context, exec sqlx.ExtContext, root *storage.RefreshSession) error {
	if err := root.Validate(); err != nil {
		return storage.NewStorageError("RefreshToken.LegacyOrigin", storage.ErrorKindNotFound, ports.ErrNotFound, "refresh root lacks valid lineage metadata")
	}
	var legacy struct {
		SessionID            *id.RefreshSessionID `db:"session_id"`
		RequestID            string               `db:"request_id"`
		AgentID              id.AgentID           `db:"agent_id"`
		Principal            id.Principal         `db:"principal"`
		ClientID             id.ClientID          `db:"client_id"`
		Scope                string               `db:"scope"`
		CreatedAt            time.Time            `db:"created_at"`
		ExpiresAt            time.Time            `db:"expires_at"`
		UsedAt               *time.Time           `db:"used_at"`
		PredecessorSignature *string              `db:"predecessor_signature"`
	}
	if err := sqlx.GetContext(ctx, exec, &legacy, `SELECT session_id, request_id, agent_id, principal,
		client_id, scope, created_at, expires_at, used_at, predecessor_signature FROM refresh_token_sessions
		WHERE signature = $1 FOR UPDATE`, root.CurrentSignature); err != nil {
		return refreshStoreError("RefreshToken.LegacyCurrent", err)
	}
	if legacy.SessionID == nil || *legacy.SessionID != root.ID || legacy.RequestID != root.ID.String() ||
		legacy.AgentID != root.AgentID || legacy.Principal != root.Principal || legacy.ClientID != root.ClientID ||
		legacy.Scope != root.Scope || legacy.UsedAt != nil ||
		(root.PreviousSignature == nil && legacy.PredecessorSignature != nil) ||
		(root.PreviousSignature != nil && (legacy.PredecessorSignature == nil || *legacy.PredecessorSignature != *root.PreviousSignature)) {
		return storage.NewStorageError("RefreshToken.LegacyCurrent", storage.ErrorKindNotFound, ports.ErrNotFound, "unsupported legacy refresh lineage")
	}
	var native refreshTokenRecord
	if err := sqlx.GetContext(ctx, exec, &native,
		`SELECT signature, session_id, issued_at, expires_at, used_at FROM refresh_tokens WHERE signature = $1`, root.CurrentSignature); err != nil {
		return refreshStoreError("RefreshToken.LegacyCurrent", err)
	}
	if native.SessionID != root.ID || native.UsedAt != nil || !legacy.CreatedAt.Equal(native.IssuedAt) ||
		!legacy.ExpiresAt.Equal(native.ExpiresAt) || !native.IssuedAt.Equal(root.LastFreshAt) ||
		native.ExpiresAt.After(root.RetainUntil) {
		return storage.NewStorageError("RefreshToken.LegacyCurrent", storage.ErrorKindNotFound, ports.ErrNotFound, "unsupported legacy refresh lineage")
	}
	// The durable proof is invalidated by database triggers for any unsupported
	// historical write. Lock it after the current mirror: an older writer holds
	// that row before its proof invalidation, so the reverse order deadlocks.
	var complete bool
	err := exec.QueryRowxContext(ctx, `SELECT r.lineage_valid AND r.original_token_signature = $2
		AND r.started_at = $3 AND r.agent_id = $4 AND r.principal = $5
		AND r.client_id = $6 AND r.scope = $7 AND r.original_grant_id = $8
		AND origin.session_id = r.id AND origin.issued_at = r.started_at
		AND origin.expires_at <= r.retain_until
		AND first_mirror.session_id = r.id AND first_mirror.request_id = r.id::text
		AND first_mirror.predecessor_signature IS NULL
		AND first_mirror.agent_id = r.agent_id AND first_mirror.principal = r.principal
		AND first_mirror.client_id = r.client_id AND first_mirror.scope = r.scope
		AND first_mirror.email IS NOT DISTINCT FROM r.email
		AND first_mirror.display_name = r.display_name
		AND first_mirror.created_at = origin.issued_at
		AND first_mirror.expires_at = origin.expires_at
		AND first_mirror.used_at IS NOT DISTINCT FROM origin.used_at
		FROM refresh_sessions r
		JOIN refresh_tokens origin ON origin.signature = r.original_token_signature
		JOIN refresh_token_sessions first_mirror ON first_mirror.signature = origin.signature
		WHERE r.id = $1 FOR UPDATE OF r`, root.ID, root.OriginalTokenSignature,
		root.StartedAt, root.AgentID, root.Principal, root.ClientID, root.Scope,
		root.OriginalGrantID).Scan(&complete)
	if err != nil {
		return refreshStoreError("RefreshToken.LegacyOrigin", err)
	}
	if !complete {
		return storage.NewStorageError("RefreshToken.LegacyOrigin", storage.ErrorKindNotFound, ports.ErrNotFound, "refresh ancestry lacks complete anchored evidence")
	}
	return nil
}

func (r *RefreshTokenRepo) MarkUsed(ctx context.Context, signature string, usedAt time.Time) error {
	const op = "RefreshToken.MarkUsed"
	var rootID id.RefreshSessionID
	if err := r.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
		`SELECT session_id FROM refresh_tokens WHERE signature = $1`, signature).Scan(&rootID); err != nil {
		return refreshStoreError(op, err)
	}
	root, err := loadRefreshSession(ctx, r.adapter.storageExecutor(ctx), rootID, false)
	if err != nil {
		return err
	}
	if err := requireAuthorizationAgent(ctx, root.AgentID); err != nil {
		return err
	}
	scope, _ := scopedAuthorization(ctx)
	if usedAt.After(scope.latestTime) || !usedAt.After(root.LastFreshAt) {
		return refreshValidation(op, errors.New("consumption must follow issuance and precede the final shared authorization time"))
	}
	if root.TerminalReason != nil || root.CurrentSignature != signature {
		return refreshValidation(op, errors.New("only the live current token can be consumed"))
	}
	if err := checkAnchoredCurrent(ctx, r.adapter.storageExecutor(ctx), root); err != nil {
		return err
	}
	result, err := r.adapter.storageExecutor(ctx).ExecContext(ctx,
		`UPDATE refresh_tokens SET used_at = $2 WHERE signature = $1 AND session_id = $3
			AND used_at IS NULL AND issued_at <= $2 AND expires_at > $2`,
		signature, usedAt, rootID)
	if err != nil {
		return refreshStoreError(op, err)
	}
	if err := checkRowsAffected(op, result, "refresh token already used or expired"); err != nil {
		return err
	}
	result, err = r.adapter.storageExecutor(ctx).ExecContext(ctx,
		`UPDATE refresh_token_sessions SET used_at = $2 WHERE signature = $1 AND session_id = $3
			AND used_at IS NULL AND agent_id = $4`, signature, usedAt, rootID, root.AgentID)
	if err != nil {
		return refreshStoreError(op, err)
	}
	if err := checkRowsAffected(op, result, "anchored legacy current was already consumed"); err != nil {
		return err
	}
	return nil
}
