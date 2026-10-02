//go:build integration

package e2e_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/jmoiron/sqlx"
)

// agedAbsoluteHistory retains the verified HTTP-issued root and token identities
// in an isolated pre-proof fixture before the complete migration backfill.
type agedAbsoluteHistory struct {
	FirstIssuedAt          time.Time
	LastFreshAt            time.Time
	CurrentExpiresAt       time.Time
	OriginalTokenSignature string
	HasRoot                bool
}

type absoluteLegacyToken struct {
	Signature string       `db:"signature"`
	RequestID string       `db:"request_id"`
	AgentID   string       `db:"agent_id"`
	ClientID  string       `db:"client_id"`
	Principal string       `db:"principal"`
	Scope     string       `db:"scope"`
	CreatedAt time.Time    `db:"created_at"`
	UsedAt    sql.NullTime `db:"used_at"`
	ExpiresAt time.Time    `db:"expires_at"`
}

type absoluteNativeRoot struct {
	ID                     string         `db:"id"`
	OriginalGrantID        string         `db:"original_grant_id"`
	OriginalTokenSignature string         `db:"original_token_signature"`
	AgentID                string         `db:"agent_id"`
	ClientID               string         `db:"client_id"`
	Principal              string         `db:"principal"`
	Scope                  string         `db:"scope"`
	StartedAt              time.Time      `db:"started_at"`
	LastFreshAt            time.Time      `db:"last_fresh_at"`
	AbsoluteExpiresAt      sql.NullTime   `db:"absolute_expires_at"`
	InactivityExpiresAt    time.Time      `db:"inactivity_expires_at"`
	RetainUntil            time.Time      `db:"retain_until"`
	CurrentSignature       string         `db:"current_signature"`
	PreviousSignature      sql.NullString `db:"previous_signature"`
	PreviousConsumedAt     sql.NullTime   `db:"previous_consumed_at"`
	ReuseUntil             sql.NullTime   `db:"reuse_until"`
	RetryAccessExpiresAt   sql.NullTime   `db:"retry_access_expires_at"`
	RetryExpiresAt         sql.NullTime   `db:"retry_expires_at"`
	RetryCiphertext        []byte         `db:"retry_ciphertext"`
	TerminalReason         sql.NullString `db:"terminal_reason"`
}

type absoluteNativeToken struct {
	Signature string       `db:"signature"`
	SessionID string       `db:"session_id"`
	IssuedAt  time.Time    `db:"issued_at"`
	UsedAt    sql.NullTime `db:"used_at"`
	ExpiresAt time.Time    `db:"expires_at"`
}

// ageVerifiedAbsoluteHistory backdates real RT0 -> RT1 -> RT2 activity to
// -35d, -20d, -1d. All presented tokens came from actual HTTP issuance.
// Each consumed predecessor was used before its immutable 30-day expiry;
// the current token still has 29 days. A closed historical retry window has
// no live ciphertext, while actual JWT expiry metadata remains unchanged.
func ageVerifiedAbsoluteHistory(ctx context.Context, database *sqlx.DB, raw [3]string) (_ agedAbsoluteHistory, err error) {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return agedAbsoluteHistory{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var proofInstalled bool
	if err := tx.GetContext(ctx, &proofInstalled, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'refresh_sessions' AND column_name = 'lineage_valid')`); err != nil {
		return agedAbsoluteHistory{}, err
	}
	if proofInstalled {
		return agedAbsoluteHistory{}, fmt.Errorf("aging requires an isolated pre-proof refresh fixture")
	}

	var hasRoot bool
	if err := tx.GetContext(ctx, &hasRoot, "SELECT to_regclass('public.refresh_sessions') IS NOT NULL"); err != nil {
		return agedAbsoluteHistory{}, err
	}
	var now time.Time
	if err := tx.GetContext(ctx, &now, "SELECT clock_timestamp()"); err != nil {
		return agedAbsoluteHistory{}, err
	}
	const inactivity = 30 * 24 * time.Hour
	issued := [3]time.Time{now.Add(-35 * 24 * time.Hour), now.Add(-20 * 24 * time.Hour), now.Add(-24 * time.Hour)}
	used := [3]sql.NullTime{{Time: issued[1], Valid: true}, {Time: issued[2], Valid: true}, {}}
	var signatures [3]string
	var previous [3]absoluteLegacyToken
	for index, token := range raw {
		signatures[index] = helpers.RefreshSignature(token)
		if err := tx.GetContext(ctx, &previous[index], `SELECT signature, request_id, agent_id::text AS agent_id,
			client_id, principal, scope, created_at, used_at, expires_at
			FROM refresh_token_sessions WHERE signature = $1 FOR UPDATE`, signatures[index]); err != nil {
			return agedAbsoluteHistory{}, fmt.Errorf("load issued legacy token %d: %w", index, err)
		}
		if previous[index].Signature != signatures[index] || previous[index].UsedAt.Valid != (index < 2) ||
			previous[index].ExpiresAt.Sub(previous[index].CreatedAt) < inactivity-time.Minute ||
			previous[index].ExpiresAt.Sub(previous[index].CreatedAt) > inactivity+time.Minute {
			return agedAbsoluteHistory{}, fmt.Errorf("issued legacy token %d has inconsistent native state", index)
		}
		if index > 0 && (signatures[index] == signatures[index-1] ||
			previous[index].RequestID != previous[0].RequestID ||
			previous[index].AgentID != previous[0].AgentID ||
			previous[index].ClientID != previous[0].ClientID ||
			previous[index].Principal != previous[0].Principal ||
			previous[index].Scope != previous[0].Scope ||
			!previous[index].CreatedAt.After(previous[index-1].CreatedAt) ||
			!previous[index-1].UsedAt.Time.Before(previous[index-1].ExpiresAt)) {
			return agedAbsoluteHistory{}, fmt.Errorf("issued legacy lineage at token %d is inconsistent", index)
		}
	}

	var original absoluteNativeRoot
	if hasRoot {
		if err := tx.GetContext(ctx, &original, `SELECT id::text AS id, original_grant_id::text AS original_grant_id,
			original_token_signature, agent_id::text AS agent_id, client_id, principal, scope,
			started_at, last_fresh_at, absolute_expires_at, inactivity_expires_at, retain_until,
			current_signature, previous_signature, previous_consumed_at, reuse_until,
			retry_access_expires_at, retry_expires_at, retry_ciphertext, terminal_reason
			FROM refresh_sessions WHERE current_signature = $1 FOR UPDATE`, signatures[2]); err != nil {
			return agedAbsoluteHistory{}, fmt.Errorf("load issued root: %w", err)
		}
		originalScope := strings.Fields(previous[0].Scope)
		slices.Sort(originalScope)
		if original.ID == "" || original.OriginalGrantID == "" || original.OriginalTokenSignature != signatures[0] ||
			original.CurrentSignature != signatures[2] || !original.PreviousSignature.Valid ||
			original.PreviousSignature.String != signatures[1] || original.AbsoluteExpiresAt.Valid ||
			original.TerminalReason.Valid || original.AgentID != previous[0].AgentID ||
			original.ClientID != previous[0].ClientID || original.Principal != previous[0].Principal ||
			original.Scope != strings.Join(originalScope, " ") || original.ID != previous[0].RequestID ||
			!original.PreviousConsumedAt.Valid || !original.ReuseUntil.Valid ||
			!original.RetryExpiresAt.Valid || !original.RetryAccessExpiresAt.Valid {
			return agedAbsoluteHistory{}, fmt.Errorf("original issuer did not retain root ownership and complete lineage")
		}
		var children []absoluteNativeToken
		if err := tx.SelectContext(ctx, &children, `SELECT signature, session_id::text AS session_id,
			issued_at, used_at, expires_at FROM refresh_tokens WHERE session_id = $1 FOR UPDATE`, original.ID); err != nil {
			return agedAbsoluteHistory{}, err
		}
		if len(children) != len(signatures) {
			return agedAbsoluteHistory{}, fmt.Errorf("issued root has %d native tokens, expected three", len(children))
		}
		seen := make(map[string]bool, len(children))
		for _, child := range children {
			seen[child.Signature] = true
			for index, signature := range signatures {
				if child.Signature != signature {
					continue
				}
				if child.SessionID != original.ID || !child.IssuedAt.Equal(previous[index].CreatedAt) ||
					!child.ExpiresAt.Equal(previous[index].ExpiresAt) ||
					child.UsedAt.Valid != previous[index].UsedAt.Valid ||
					(child.UsedAt.Valid && !child.UsedAt.Time.Equal(previous[index].UsedAt.Time)) {
					return agedAbsoluteHistory{}, fmt.Errorf("native token %d does not match issued legacy mirror", index)
				}
			}
		}
		if !seen[signatures[0]] || !seen[signatures[1]] || !seen[signatures[2]] ||
			!original.StartedAt.Equal(previous[0].CreatedAt) ||
			!original.LastFreshAt.Equal(previous[2].CreatedAt) ||
			!original.PreviousConsumedAt.Time.Equal(previous[1].UsedAt.Time) {
			return agedAbsoluteHistory{}, fmt.Errorf("native root lost first issuance or last fresh activity")
		}
		for index, signature := range signatures {
			var anchor struct {
				SessionID            sql.NullString `db:"session_id"`
				PredecessorSignature sql.NullString `db:"predecessor_signature"`
			}
			if err := tx.GetContext(ctx, &anchor, `SELECT session_id::text AS session_id,
				predecessor_signature FROM refresh_token_sessions WHERE signature = $1`, signature); err != nil {
				return agedAbsoluteHistory{}, err
			}
			if !anchor.SessionID.Valid || anchor.SessionID.String != original.ID ||
				anchor.PredecessorSignature.Valid != (index != 0) ||
				(index != 0 && anchor.PredecessorSignature.String != signatures[index-1]) {
				return agedAbsoluteHistory{}, fmt.Errorf("legacy token %d lacks complete native ancestry", index)
			}
		}
	}

	for index, signature := range signatures {
		expires := issued[index].Add(inactivity)
		if used[index].Valid && (!used[index].Time.After(issued[index]) || !used[index].Time.Before(expires)) {
			return agedAbsoluteHistory{}, fmt.Errorf("historical rotation %d outlived its predecessor", index)
		}
		result, err := tx.ExecContext(ctx, `UPDATE refresh_token_sessions SET created_at = $2,
			used_at = $3, expires_at = $4 WHERE signature = $1`, signature, issued[index], used[index], expires)
		if err != nil {
			return agedAbsoluteHistory{}, err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return agedAbsoluteHistory{}, fmt.Errorf("legacy token %d update did not affect exactly one row: %v", index, err)
		}
		if hasRoot {
			result, err = tx.ExecContext(ctx, `UPDATE refresh_tokens SET issued_at = $2,
				used_at = $3, expires_at = $4 WHERE session_id = $5 AND signature = $1`,
				signature, issued[index], used[index], expires, original.ID)
			if err != nil {
				return agedAbsoluteHistory{}, err
			}
			if affected, err := result.RowsAffected(); err != nil || affected != 1 {
				return agedAbsoluteHistory{}, fmt.Errorf("native token %d update did not affect exactly one row: %v", index, err)
			}
		}
	}
	if hasRoot {
		reuseEnded := used[1].Time.Add(30 * time.Second)
		result, err := tx.ExecContext(ctx, `UPDATE refresh_sessions SET started_at = $2,
			last_fresh_at = $3, inactivity_expires_at = $4, previous_consumed_at = $3,
			reuse_until = $5, retry_expires_at = LEAST(retry_expires_at, $5), retry_ciphertext = NULL
			WHERE id = $1`, original.ID, issued[0], issued[2], issued[2].Add(inactivity), reuseEnded)
		if err != nil {
			return agedAbsoluteHistory{}, err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return agedAbsoluteHistory{}, fmt.Errorf("root history update did not affect exactly one row: %v", err)
		}
		var aged absoluteNativeRoot
		if err := tx.GetContext(ctx, &aged, `SELECT id::text AS id, original_grant_id::text AS original_grant_id,
			original_token_signature, agent_id::text AS agent_id, client_id, principal, scope,
			started_at, last_fresh_at, absolute_expires_at, inactivity_expires_at, retain_until,
			current_signature, previous_signature, previous_consumed_at, reuse_until,
			retry_access_expires_at, retry_expires_at, retry_ciphertext, terminal_reason
			FROM refresh_sessions WHERE id = $1`, original.ID); err != nil {
			return agedAbsoluteHistory{}, err
		}
		if aged.ID != original.ID || aged.OriginalGrantID != original.OriginalGrantID ||
			aged.OriginalTokenSignature != original.OriginalTokenSignature || aged.AgentID != original.AgentID ||
			aged.ClientID != original.ClientID || aged.Principal != original.Principal ||
			aged.Scope != original.Scope || aged.CurrentSignature != original.CurrentSignature ||
			aged.PreviousSignature != original.PreviousSignature || aged.AbsoluteExpiresAt.Valid ||
			aged.TerminalReason.Valid || !aged.StartedAt.Equal(issued[0]) ||
			!aged.LastFreshAt.Equal(issued[2]) || !aged.InactivityExpiresAt.Equal(issued[2].Add(inactivity)) ||
			!aged.PreviousConsumedAt.Time.Equal(used[1].Time) || !aged.ReuseUntil.Time.Equal(reuseEnded) ||
			!aged.RetryAccessExpiresAt.Time.Equal(original.RetryAccessExpiresAt.Time) ||
			aged.RetryExpiresAt.Time.After(reuseEnded) || aged.RetainUntil.Before(issued[2].Add(inactivity)) ||
			len(aged.RetryCiphertext) != 0 {
			return agedAbsoluteHistory{}, fmt.Errorf("aged root changed immutable authority or retained an open retry result")
		}
	}
	for index, signature := range signatures {
		var aged absoluteLegacyToken
		if err := tx.GetContext(ctx, &aged, `SELECT signature, request_id, agent_id::text AS agent_id,
			client_id, principal, scope, created_at, used_at, expires_at
			FROM refresh_token_sessions WHERE signature = $1`, signature); err != nil {
			return agedAbsoluteHistory{}, err
		}
		if aged.Signature != previous[index].Signature || aged.RequestID != previous[index].RequestID ||
			aged.AgentID != previous[index].AgentID || aged.ClientID != previous[index].ClientID ||
			aged.Principal != previous[index].Principal || aged.Scope != previous[index].Scope ||
			!aged.CreatedAt.Equal(issued[index]) || !aged.ExpiresAt.Equal(issued[index].Add(inactivity)) ||
			aged.UsedAt.Valid != used[index].Valid || (used[index].Valid && !aged.UsedAt.Time.Equal(used[index].Time)) {
			return agedAbsoluteHistory{}, fmt.Errorf("historical legacy token %d is inconsistent", index)
		}
	}
	if err := tx.Commit(); err != nil {
		return agedAbsoluteHistory{}, err
	}
	return agedAbsoluteHistory{
		FirstIssuedAt: issued[0], LastFreshAt: issued[2],
		CurrentExpiresAt: issued[2].Add(inactivity), OriginalTokenSignature: signatures[0], HasRoot: hasRoot,
	}, nil
}
