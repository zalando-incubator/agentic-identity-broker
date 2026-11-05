package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestUserGrantRepositorySnapshotDeleteFailureResults(t *testing.T) {
	principal := id.Principal("snapshot-failure@example.com")
	agentID := id.NewAgentID()
	grantID := id.NewGrantID()
	now := time.Now().UTC()
	columns := []string{"id", "principal", "agent_id", "valid_until", "granted_permission_sets", "created_at", "updated_at"}
	row := []driver.Value{grantID.String(), string(principal), agentID.String(), nil, []byte(`[]`), now, now}
	badID := append([]driver.Value(nil), row...)
	badID[0] = "invalid-grant-id"
	badJSON := append([]driver.Value(nil), row...)
	badJSON[4] = []byte(`{}`)
	failure := errors.New("injected storage failure")
	cases := []struct {
		name        string
		cfg         signingKeyRepoTestConfig
		kind        storage.ErrorKind
		wantQueries int
	}{
		{name: "begin", cfg: signingKeyRepoTestConfig{beginErr: failure}, kind: storage.ErrorKindConnection},
		{name: "query", cfg: signingKeyRepoTestConfig{queryErr: failure}, kind: storage.ErrorKindConnection, wantQueries: 1},
		{name: "absence", cfg: signingKeyRepoTestConfig{queryColumns: columns}, kind: storage.ErrorKindNotFound, wantQueries: 1},
		{name: "scan", cfg: signingKeyRepoTestConfig{queryColumns: columns, queryRows: [][]driver.Value{badID}}, kind: storage.ErrorKindConnection, wantQueries: 1},
		{name: "decode", cfg: signingKeyRepoTestConfig{queryColumns: columns, queryRows: [][]driver.Value{badJSON}}, kind: storage.ErrorKindUnknown, wantQueries: 1},
		{name: "commit", cfg: signingKeyRepoTestConfig{queryColumns: columns, queryRows: [][]driver.Value{row}, commitErr: failure}, kind: storage.ErrorKindConnection, wantQueries: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var queries, execs []string
			tc.cfg.recordedQueries = &queries
			tc.cfg.recordedExecs = &execs
			adapter := newUnitTestSigningKeyRepo(t, tc.cfg).adapter
			repo := NewUserGrantRepository(adapter)
			deleted, err := repo.DeleteByPrincipalAndAgentID(context.Background(), principal, agentID)
			require.Error(t, err)
			assert.Nil(t, deleted)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, tc.kind, storageErr.Kind)
			if tc.kind == storage.ErrorKindNotFound {
				assert.ErrorIs(t, err, ports.ErrNotFound)
			}
			if tc.wantQueries == 0 {
				assert.Empty(t, execs)
			} else {
				assert.Equal(t, []string{"SELECT pg_advisory_xact_lock_shared($1,$2)"}, execs)
			}
			require.Len(t, queries, tc.wantQueries)
		})
	}
}

func TestUserGrantRepositoryAbsentDeleteCompletesJoinedScope(t *testing.T) {
	for _, failure := range []error{nil, errors.New("delete failed")} {
		t.Run(fmt.Sprintf("failure=%v", failure), func(t *testing.T) {
			adapter := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
				queryColumns: []string{"id", "principal", "agent_id", "valid_until", "granted_permission_sets", "created_at", "updated_at"},
				queryErr:     failure,
			}).adapter
			ctx, err := adapter.BeginTX(t.Context())
			require.NoError(t, err)
			defer func() { _ = adapter.Rollback(ctx) }()
			deleted, err := NewUserGrantRepository(adapter).DeleteByPrincipalAndAgentID(ctx, id.Principal("absent-user"), id.NewAgentID())
			require.Nil(t, deleted)
			if failure != nil {
				require.ErrorIs(t, err, failure)
				require.Error(t, adapter.Commit(ctx), "actual failures must leave the owner rollback-only")
				return
			}
			require.ErrorIs(t, err, ports.ErrNotFound)
			effectRan := false
			effects, ok := ports.StorageTransactionEffectsFromContext(ctx)
			require.True(t, ok)
			require.NoError(t, effects.AfterCommit(func() { effectRan = true }))
			require.NoError(t, adapter.Commit(ctx), "an absent deletion must not poison the owning transaction")
			require.True(t, effectRan, "the owner must still release its committed effects")
		})
	}
}

func TestUserGrantRepositoryFailedWritesLeaveMetadataUnchanged(t *testing.T) {
	failure := errors.New("injected storage failure")
	for _, update := range []bool{false, true} {
		operation := "create"
		columns := []string{"id", "created_at", "updated_at", "valid_until"}
		if update {
			operation = "update"
			columns = columns[1:]
		}
		t.Run(operation, func(t *testing.T) {
			now := time.Now().UTC()
			validUntil := now.Add(time.Hour)
			grant := &storage.UserGrant{
				ID:         id.NewGrantID(),
				Principal:  id.Principal("failed-write@example.com"),
				AgentID:    id.NewAgentID(),
				CreatedAt:  now,
				UpdatedAt:  now,
				ValidUntil: &validUntil,
			}
			returned := []driver.Value{now.Add(-time.Hour), now.Add(time.Minute), nil}
			if !update {
				returned = append([]driver.Value{id.NewGrantID().String()}, returned...)
			}
			badScan := append([]driver.Value(nil), returned...)
			badScan[len(badScan)-2] = "not-a-timestamp"
			cases := []struct {
				name string
				cfg  signingKeyRepoTestConfig
			}{
				{name: "begin", cfg: signingKeyRepoTestConfig{beginErr: failure}},
				{name: "query", cfg: signingKeyRepoTestConfig{queryErr: failure}},
				{name: "scan", cfg: signingKeyRepoTestConfig{queryColumns: columns, queryRows: [][]driver.Value{badScan}}},
				{name: "commit", cfg: signingKeyRepoTestConfig{queryColumns: columns, queryRows: [][]driver.Value{returned}, commitErr: failure}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					attempt := grant.Copy()
					before := attempt.Copy()
					validityPointer := attempt.ValidUntil
					adapter := newUnitTestSigningKeyRepo(t, tc.cfg).adapter
					repo := NewUserGrantRepository(adapter)
					var err error
					if update {
						err = repo.Update(context.Background(), attempt)
					} else {
						err = repo.Create(context.Background(), attempt)
					}
					require.Error(t, err)
					assert.Equal(t, before, attempt)
					assert.Same(t, validityPointer, attempt.ValidUntil)
				})
			}
		})
	}
}

func TestPermissionSetRepositoryDeleteRetryOutcomes(t *testing.T) {
	serialization := &pgconn.PgError{Code: "40001"}
	failure := errors.New("injected storage failure")
	cases := []struct {
		name        string
		cfg         signingKeyRepoTestConfig
		wantDeleted bool
		wantErr     error
		wantQueries int
		wantExecs   int
	}{
		{name: "existing", cfg: signingKeyRepoTestConfig{rowsAffected: 1}, wantDeleted: true, wantQueries: 2, wantExecs: 2},
		{name: "absent", wantQueries: 2, wantExecs: 2},
		{name: "commit failure", cfg: signingKeyRepoTestConfig{rowsAffected: 1, commitErr: failure}, wantErr: failure, wantQueries: 2, wantExecs: 2},
		{name: "serialization retries then commits", cfg: signingKeyRepoTestConfig{rowsAffected: 1, execErrs: []error{nil, serialization, nil, nil}}, wantDeleted: true, wantQueries: 4, wantExecs: 4},
		{name: "serialization exhausts three attempts", cfg: signingKeyRepoTestConfig{execErrs: []error{nil, serialization, nil, serialization, nil, serialization}}, wantErr: serialization, wantQueries: 6, wantExecs: 6},
		{name: "other delete failure does not retry", cfg: signingKeyRepoTestConfig{execErrs: []error{nil, failure}}, wantErr: failure, wantQueries: 2, wantExecs: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var queries, execs []string
			tc.cfg.queryColumns = []string{"count"}
			tc.cfg.queryRows = [][]driver.Value{{int64(0)}}
			tc.cfg.recordedQueries = &queries
			tc.cfg.recordedExecs = &execs
			adapter := newUnitTestSigningKeyRepo(t, tc.cfg).adapter
			repo := NewPermissionSetRepository(adapter)
			deleted, err := repo.Delete(context.Background(), id.NewPermissionSetID())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDeleted, deleted)
			assert.Len(t, queries, tc.wantQueries)
			assert.Len(t, execs, tc.wantExecs)
		})
	}
}

func TestAgentRepositoryRowIterationErrors(t *testing.T) {
	for _, operation := range []string{"ListAgents", "GetAgentsByIDs"} {
		t.Run(operation, func(t *testing.T) {
			cases := []struct {
				name string
				err  error
				kind storage.ErrorKind
			}{
				{name: "deadline", err: context.DeadlineExceeded, kind: storage.ErrorKindTimeout},
				{name: "wrapped deadline", err: fmt.Errorf("iteration: %w", context.DeadlineExceeded), kind: storage.ErrorKindTimeout},
				{name: "connection failure", err: errors.New("connection lost"), kind: storage.ErrorKindConnection},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					adapter := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{rowsErr: tc.err}).adapter
					repo := NewAgentRepository(adapter)
					var agents []*storage.Agent
					var err error
					if operation == "ListAgents" {
						agents, err = repo.List(context.Background())
					} else {
						agents, err = repo.GetByIDs(context.Background(), []id.AgentID{id.NewAgentID()})
					}
					require.ErrorIs(t, err, tc.err)
					assert.Nil(t, agents)
					var storageErr *storage.StorageError
					require.ErrorAs(t, err, &storageErr)
					assert.Equal(t, tc.kind, storageErr.Kind)
					assert.Equal(t, operation, storageErr.Operation)
				})
			}
		})
	}
}
