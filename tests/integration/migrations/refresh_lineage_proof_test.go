//go:build integration

package migrations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rotateMigrationProofRoot(t *testing.T, f *MigrationTestFramework, rootID, prior, next string, seconds int) {
	t.Helper()
	at := time.Date(2026, time.September, 1, 0, 0, seconds, 0, time.UTC)
	expires := time.Date(2026, time.November, 1, 0, 0, seconds, 0, time.UTC)
	tx, err := f.db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(t.Context(), `UPDATE refresh_tokens SET used_at = $2 WHERE signature = $1 AND used_at IS NULL`, prior, at)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), `UPDATE refresh_token_sessions SET used_at = $2 WHERE signature = $1 AND used_at IS NULL`, prior, at)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), `INSERT INTO refresh_tokens(signature,session_id,issued_at,expires_at) VALUES($1,$2,$3,$4)`, next, rootID, at, expires)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), `INSERT INTO refresh_token_sessions
		(signature,request_id,agent_id,client_id,principal,scope,created_at,expires_at,session_id,predecessor_signature)
		VALUES($1,$2,$3,'migration-owner','owner@example.test','openid profile',$4,$5,$7,$6)`,
		next, rootID, migrationAgentID, at, expires, prior, rootID)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), `UPDATE refresh_sessions SET current_signature=$2, previous_signature=$3,
		previous_consumed_at=$4, reuse_until=$5, original_requested_scope='', original_request_context_fingerprint=$6,
		retry_access_expires_at=$7, retry_expires_at=$5, last_fresh_at=$4,
		inactivity_expires_at=$8, retain_until=$8 WHERE id=$1`,
		rootID, next, prior, at, at.Add(30*time.Second), migrationSignature("redacted proof context"), at.Add(5*time.Minute), expires)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestMigration037BackfillsOnlyCompleteAnchoredEvidence(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	seedRefreshLegacyRows(t, f)
	require.NoError(t, f.Up(t, 36))
	intact := migrationSignature("intact before bounded proof")
	broken := migrationSignature("broken before bounded proof")
	insertMigrationRefreshRoot(t, f, migrationRootID, intact, migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	insertMigrationRefreshRoot(t, f, migrationLiveID, broken, migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	intactMiddle := migrationSignature("intact middle before bounded proof")
	brokenMiddle := migrationSignature("broken middle before bounded proof")
	rotateMigrationProofRoot(t, f, migrationRootID, intact, intactMiddle, 20)
	rotateMigrationProofRoot(t, f, migrationRootID, intactMiddle, migrationSignature("intact current before bounded proof"), 40)
	rotateMigrationProofRoot(t, f, migrationLiveID, broken, brokenMiddle, 20)
	rotateMigrationProofRoot(t, f, migrationLiveID, brokenMiddle, migrationSignature("broken current before bounded proof"), 40)
	migrationExec(t, f, `UPDATE refresh_token_sessions SET client_id = 'wrong-client' WHERE signature = $1`, brokenMiddle)

	require.NoError(t, f.Up(t, 37), "existing anchored sessions need a one-time complete proof, not fabricated origin")
	require.True(t, migrationBool(t, f, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, migrationRootID))
	require.False(t, migrationBool(t, f, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, migrationLiveID))
	migrationExec(t, f, `UPDATE refresh_token_sessions SET client_id = 'migration-owner' WHERE signature = $1`, brokenMiddle)
	require.False(t, migrationBool(t, f, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, migrationLiveID),
		"repairing a broken mirror cannot mint evidence for a family with missing issuance trust")
	// Old binaries must retain their seven-column insert and conditional-use shape.
	legacy := migrationSignature("old writer after bounded proof")
	migrationExec(t, f, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at)
		VALUES ($1,$2,$3,'migration-owner','owner@example.test','openid',TIMESTAMPTZ '2026-11-01 00:00:00+00')`,
		legacy, migrationLegacyRequestID, migrationAgentID)
	migrationExec(t, f, `UPDATE refresh_token_sessions SET used_at = NOW() WHERE signature = $1 AND used_at IS NULL`, legacy)
	require.True(t, migrationBool(t, f, `SELECT used_at IS NOT NULL AND session_id IS NULL FROM refresh_token_sessions WHERE signature = $1`, legacy))
}

func TestMigration037RejectsConsumedForkOutsideCurrentAncestry(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	require.NoError(t, f.Up(t, 36))
	first := migrationSignature("forked origin before bounded proof")
	middle := migrationSignature("forked middle before bounded proof")
	current := migrationSignature("forked current before bounded proof")
	fork := migrationSignature("consumed fork outside current ancestry")
	insertMigrationRefreshRoot(t, f, migrationRootID, first, migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	rotateMigrationProofRoot(t, f, migrationRootID, first, middle, 20)
	rotateMigrationProofRoot(t, f, migrationRootID, middle, current, 40)
	migrationExec(t, f, `INSERT INTO refresh_tokens(signature,session_id,issued_at,expires_at,used_at)
		SELECT $1,session_id,issued_at,expires_at,used_at FROM refresh_tokens WHERE signature=$2`, fork, middle)
	migrationExec(t, f, `INSERT INTO refresh_token_sessions
		(signature,request_id,agent_id,client_id,principal,scope,created_at,expires_at,used_at,session_id,predecessor_signature)
		SELECT $1,request_id,agent_id,client_id,principal,scope,created_at,expires_at,used_at,session_id,predecessor_signature
		FROM refresh_token_sessions WHERE signature=$2`, fork, middle)

	require.NoError(t, f.Up(t, 37))
	require.False(t, migrationBool(t, f, `SELECT lineage_valid FROM refresh_sessions WHERE id=$1`, migrationRootID),
		"a consumed fork outside the current ancestry cannot count as complete issuance evidence")
}

func TestMigration037DownAndReapplyCannotRecreateAnchoredAuthority(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	require.NoError(t, f.Up(t, 37))
	first := migrationSignature("bounded-proof root")
	insertMigrationRefreshRoot(t, f, migrationRootID, first, migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	require.True(t, migrationBool(t, f, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, migrationRootID))
	require.NoError(t, f.Down(t, 36))
	require.True(t, migrationBool(t, f, `SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1`, first),
		"removing a required proof must fence older binaries before dropping its guard")
	require.NoError(t, f.Up(t, 37))
	require.False(t, migrationBool(t, f, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, migrationRootID),
		"an unproven down/reapply cannot make formerly anchored authority renewable")
	require.NoError(t, f.Down(t, 35))
	require.True(t, migrationBool(t, f, `SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1`, first))
	require.NoError(t, f.Up(t, 37))
	require.True(t, migrationBool(t, f, `SELECT used_at IS NOT NULL AND session_id IS NULL FROM refresh_token_sessions WHERE signature = $1`, first))
}

func TestMigration037TerminalAuthorityCannotBeReopened(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	require.NoError(t, f.Up(t, 37))
	first := migrationSignature("permanently terminal root")
	insertMigrationRefreshRoot(t, f, migrationRootID, first, migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	migrationExec(t, f, `UPDATE refresh_sessions SET revoked_at = TIMESTAMPTZ '2026-09-02 00:00:00+00',
		terminal_reason = 'grant_deleted' WHERE id = $1`, migrationRootID)
	_, err := f.db.ExecContext(t.Context(), `UPDATE refresh_sessions SET revoked_at = NULL, terminal_reason = NULL
		WHERE id = $1`, migrationRootID)
	require.Error(t, err, "a terminal root cannot regain authority even if its rows are rewritten")
	require.True(t, migrationBool(t, f, `SELECT terminal_reason = 'grant_deleted' FROM refresh_sessions WHERE id = $1`, migrationRootID))
}
