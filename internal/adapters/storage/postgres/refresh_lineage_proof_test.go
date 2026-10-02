//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type lineageTraceKey struct{}

type lineageQuery struct {
	text string
	args []any
}

type lineageQueryTrace struct{ queries []lineageQuery }

func (trace *lineageQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if ctx.Value(lineageTraceKey{}) != nil && strings.Contains(data.SQL, "refresh_") && strings.Contains(data.SQL, "SELECT") {
		trace.queries = append(trace.queries, lineageQuery{text: data.SQL, args: append([]any(nil), data.Args...)})
	}
	return ctx
}

func (*lineageQueryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func lineageRowsVisited(t *testing.T, db *sqlx.DB, queries []lineageQuery) float64 {
	t.Helper()
	type node struct {
		ActualRows          float64 `json:"Actual Rows"`
		ActualLoops         float64 `json:"Actual Loops"`
		RowsRemovedByFilter float64 `json:"Rows Removed by Filter"`
		Plans               []node  `json:"Plans"`
	}
	var visit func(node) float64
	visit = func(n node) float64 {
		total := (n.ActualRows + n.RowsRemovedByFilter) * n.ActualLoops
		for _, child := range n.Plans {
			total += visit(child)
		}
		return total
	}
	var total float64
	for _, query := range queries {
		var planJSON []byte
		require.NoError(t, db.QueryRowx(`EXPLAIN (ANALYZE, FORMAT JSON) `+query.text, query.args...).Scan(&planJSON))
		var plan []struct {
			Plan node `json:"Plan"`
		}
		require.NoError(t, json.Unmarshal(planJSON, &plan))
		require.Len(t, plan, 1)
		total += visit(plan[0].Plan)
	}
	return total
}

func rotatePGHistory(t *testing.T, adapter *Adapter, rootID id.RefreshSessionID, count int) []string {
	t.Helper()
	ctx := context.Background()
	root, err := NewRefreshSessionRepo(adapter).FindByID(ctx, rootID)
	require.NoError(t, err)
	signatures := []string{root.OriginalTokenSignature}
	for n := range count {
		signature := pgRefreshSignature(fmt.Sprintf("lineage-%s-%d", rootID, n))
		require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
			return pgRotateRefresh(scope, NewRefreshSessionRepo(adapter), NewRefreshTokenRepo(adapter), rootID, signature, at)
		}))
		signatures = append(signatures, signature)
	}
	return signatures
}

func TestPGRefreshLineageLookupWorkDoesNotGrowWithHistory(t *testing.T) {
	for _, rotations := range []int{1, 48} {
		t.Run(fmt.Sprintf("rotations=%d", rotations), func(t *testing.T) {
			dsn, cleanup := setupMigratedConnString(t)
			defer cleanup()
			adapter, err := NewAdapter(testStorageConfig(dsn))
			require.NoError(t, err)
			require.NoError(t, adapter.Initialize(context.Background()))
			defer func() { require.NoError(t, adapter.Close(context.Background())) }()
			root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("bounded@example.test"))
			rotatePGHistory(t, adapter, root.ID, rotations)

			trace := &lineageQueryTrace{}
			cfg, err := pgx.ParseConfig(dsn)
			require.NoError(t, err)
			cfg.Tracer = trace
			tracedDB := sqlx.NewDb(stdlib.OpenDB(*cfg), "pgx")
			defer func() { require.NoError(t, tracedDB.Close()) }()
			tracedAdapter := &Adapter{db: tracedDB}
			ctx := context.WithValue(context.Background(), lineageTraceKey{}, true)
			require.NoError(t, NewAuthorizationSessionCoordinator(tracedAdapter).Run(ctx, root.AgentID, func(scope context.Context, _ time.Time) error {
				return NewRefreshTokenRepo(tracedAdapter).CheckCurrentLineage(scope, root.ID)
			}))
			require.NotEmpty(t, trace.queries, "measure the actual eligibility SQL, not a hand-written substitute")
			visited := lineageRowsVisited(t, tracedDB, trace.queries)
			require.LessOrEqual(t, visited, float64(35), "eligibility must visit a fixed number of indexed evidence rows, not the whole lineage")
		})
	}
}

func TestPGRefreshOlderEvidenceTamperingInvalidatesLineageIrreversibly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter string
		undo  string
	}{
		{"changed intermediate mirror owner", `UPDATE refresh_token_sessions SET principal = 'impostor' WHERE signature = $1`, `UPDATE refresh_token_sessions SET principal = 'owner@example.test' WHERE signature = $1`},
		{"changed intermediate mirror predecessor", `UPDATE refresh_token_sessions SET predecessor_signature = NULL WHERE signature = $1`, `UPDATE refresh_token_sessions SET predecessor_signature = $2 WHERE signature = $1`},
		{"changed intermediate native consumption", `UPDATE refresh_tokens SET used_at = used_at + INTERVAL '1 second' WHERE signature = $1`, `UPDATE refresh_tokens SET used_at = used_at - INTERVAL '1 second' WHERE signature = $1`},
		{"deleted intermediate mirror", `DELETE FROM refresh_token_sessions WHERE signature = $1`, ""},
		{"deleted intermediate token", `DELETE FROM refresh_tokens WHERE signature = $1`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, cleanup := setupMigratedAdapter(t)
			defer cleanup()
			ctx := context.Background()
			root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
			signatures := rotatePGHistory(t, adapter, root.ID, 4)
			older := signatures[1]
			var current string
			require.NoError(t, adapter.db.GetContext(ctx, &current, `SELECT current_signature FROM refresh_sessions WHERE id = $1`, root.ID))
			require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, _ time.Time) error {
				return NewRefreshTokenRepo(adapter).CheckCurrentLineage(scope, root.ID)
			}))
			_, err := adapter.db.ExecContext(ctx, tc.alter, older)
			require.NoError(t, err)
			if tc.undo != "" {
				args := []any{older}
				if strings.Contains(tc.undo, "$2") {
					args = append(args, signatures[0])
				}
				_, err = adapter.db.ExecContext(ctx, tc.undo, args...)
				require.NoError(t, err, "repair must not reauthorize a previously broken family")
			}
			err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
				tokens := NewRefreshTokenRepo(adapter)
				if err := tokens.CheckCurrentLineage(scope, root.ID); !ports.IsNotFoundErr(err) {
					return fmt.Errorf("corrupted history should be unsupported: %v", err)
				}
				if err := tokens.MarkUsed(scope, current, at); !ports.IsNotFoundErr(err) {
					return fmt.Errorf("corrupted history must not consume current token: %v", err)
				}
				return nil
			})
			require.NoError(t, err)
			var stillCurrent bool
			require.NoError(t, adapter.db.GetContext(ctx, &stillCurrent, `SELECT r.current_signature = $2 AND t.used_at IS NULL
				FROM refresh_sessions r JOIN refresh_tokens t ON t.signature = r.current_signature WHERE r.id = $1`, root.ID, current))
			require.True(t, stillCurrent, "rejection cannot mutate the viable-looking current token")
		})
	}
}

func TestPGRefreshExtraDisconnectedAnchoredEvidenceFailsClosed(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	rotatePGHistory(t, adapter, root.ID, 2)
	_, err := adapter.db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at, session_id, predecessor_signature)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, pgRefreshSignature("disconnected-"+root.ID.String()),
		root.ID.String(), root.AgentID, root.ClientID, root.Principal, root.Scope, root.RetainUntil, root.ID, root.OriginalTokenSignature)
	require.NoError(t, err)
	err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, _ time.Time) error {
		return NewRefreshTokenRepo(adapter).CheckCurrentLineage(scope, root.ID)
	})
	require.True(t, ports.IsNotFoundErr(err), "a detached anchored mirror cannot be ignored even if current/first remain intact")
}

func TestPGRefreshMovingIntermediatePoisonsBothOwners(t *testing.T) {
	for _, table := range []string{"refresh_tokens", "refresh_token_sessions"} {
		t.Run(table, func(t *testing.T) {
			adapter, cleanup := setupMigratedAdapter(t)
			defer cleanup()
			ctx := context.Background()
			owner, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
			neighbor, _ := createPGRefreshRoot(t, adapter, owner.AgentID, id.Principal("neighbor@example.test"))
			signatures := rotatePGHistory(t, adapter, owner.ID, 3)
			_, err := adapter.db.ExecContext(ctx, `UPDATE `+table+` SET session_id = $2 WHERE signature = $1`, signatures[1], neighbor.ID)
			require.NoError(t, err)
			require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, owner.AgentID, func(scope context.Context, _ time.Time) error {
				for _, rootID := range []id.RefreshSessionID{owner.ID, neighbor.ID} {
					if err := NewRefreshTokenRepo(adapter).CheckCurrentLineage(scope, rootID); !ports.IsNotFoundErr(err) {
						return fmt.Errorf("a moved %s row must invalidate both families: %s: %v", table, rootID, err)
					}
				}
				return nil
			}))
		})
	}
}

func TestPGRefreshOldOnlyCurrentConsumptionPoisonsProof(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	result, err := adapter.db.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = NOW()
		WHERE signature = $1 AND used_at IS NULL`, first.Signature)
	require.NoError(t, err, "the old writer must retain its conditional consume shape")
	changed, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, changed)
	var trusted bool
	require.NoError(t, adapter.db.GetContext(ctx, &trusted, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, root.ID))
	require.False(t, trusted, "unmatched first-token legacy consumption cannot leave a trusted proof")
	_, err = adapter.db.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = NULL WHERE signature = $1`, first.Signature)
	require.NoError(t, err)
	require.NoError(t, adapter.db.GetContext(ctx, &trusted, `SELECT lineage_valid FROM refresh_sessions WHERE id = $1`, root.ID))
	require.False(t, trusted, "undoing an old-only consumption cannot restore lineage authority")
	err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, _ time.Time) error {
		return NewRefreshTokenRepo(adapter).CheckCurrentLineage(scope, root.ID)
	})
	require.True(t, ports.IsNotFoundErr(err))
}
