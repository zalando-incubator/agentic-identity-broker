//go:build integration
// +build integration

package postgres

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func setupPostgresBusinessEventAdapter(t *testing.T) (*Adapter, *BusinessEventRepository) {
	t.Helper()
	adapter, cleanup := setupMigratedAdapter(t)
	t.Cleanup(cleanup)
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	return adapter, NewBusinessEventRepository(adapter, registry)
}

func registeredAgentEvent(t *testing.T, agentID id.AgentID) *model.BusinessEvent {
	t.Helper()
	data, err := fs.ReadFile(eventschemas.Schemas, "examples.json")
	require.NoError(t, err)
	var examples []model.BusinessEvent
	require.NoError(t, json.Unmarshal(data, &examples))
	for i := range examples {
		if examples[i].Type == model.BusinessEventTypePrefix+"agent-registered" {
			event := examples[i]
			event.ID = id.NewBusinessEventID()
			event.AgentID = agentID
			event.OccurredAt = time.Now().UTC().Truncate(time.Microsecond)
			return &event
		}
	}
	t.Fatal("published agent-registered example is required")
	return nil
}

func TestPostgresBusinessEvent_AmbientAgentMutationAndReferenceRollback(t *testing.T) {
	adapter, repo := setupPostgresBusinessEventAdapter(t)
	ownerDB := adapter.db
	ctx := context.Background()
	permissionID := id.NewPermissionSetID()
	_, err := ownerDB.ExecContext(ctx, `INSERT INTO public.permission_sets(id,name,description)
		VALUES ($1,$2,'Transaction fixture')`, permissionID, "ledger-"+permissionID.String())
	require.NoError(t, err)

	for _, commit := range []bool{false, true} {
		agentID := id.NewAgentID()
		clientID := id.NewClientID("adapter-event-" + agentID.String())
		agent := &storage.Agent{
			ID: agentID, ClientID: &clientID, DisplayName: "Atomic agent", Description: "Agent and event share one transaction",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: permissionID, RequirementType: storage.RequirementTypeOptional}},
		}
		event := registeredAgentEvent(t, agentID)
		txContext, err := adapter.BeginTX(ctx)
		require.NoError(t, err)
		require.NoError(t, NewAgentRepository(adapter).Create(txContext, agent))
		require.NoError(t, repo.Append(txContext, event, true))
		var agentCount, eventCount, referenceCount int
		require.NoError(t, ownerDB.GetContext(ctx, &agentCount, `SELECT count(*) FROM public.agents WHERE id=$1`, agentID))
		require.NoError(t, ownerDB.GetContext(ctx, &eventCount, `SELECT count(*) FROM public.business_events WHERE id=$1`, event.ID))
		require.NoError(t, ownerDB.GetContext(ctx, &referenceCount, `SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1`, event.ID))
		require.Zero(t, agentCount, "independent connections cannot see an uncommitted business mutation")
		require.Zero(t, eventCount)
		require.Zero(t, referenceCount)
		if commit {
			require.NoError(t, adapter.Commit(txContext))
		} else {
			require.NoError(t, adapter.Rollback(txContext))
		}
		require.NoError(t, ownerDB.GetContext(ctx, &agentCount, `SELECT count(*) FROM public.agents WHERE id=$1`, agentID))
		require.NoError(t, ownerDB.GetContext(ctx, &eventCount, `SELECT count(*) FROM public.business_events WHERE id=$1`, event.ID))
		require.NoError(t, ownerDB.GetContext(ctx, &referenceCount, `SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1`, event.ID))
		want := 0
		if commit {
			want = 1
		}
		require.Equal(t, want, agentCount, "business mutation must share the ledger owner's commit or rollback")
		require.Equal(t, want, eventCount)
		require.Equal(t, want, referenceCount, "queued references are atomic with the business fact")
	}
}

func TestPostgresBusinessEvent_LifecycleAndSubjectGatesPrecedeBusinessRowLock(t *testing.T) {
	adapter, _ := setupPostgresBusinessEventAdapter(t)
	db := adapter.db
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agentID := id.NewAgentID()
	_, err := db.ExecContext(ctx, `INSERT INTO public.agents(id,client_id,display_name,description)
		VALUES ($1,$2,'Locked agent','Test row-lock ordering')`, agentID, "locked-"+agentID.String())
	require.NoError(t, err)
	rowOwner, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = rowOwner.Rollback() }()
	var lockedID id.AgentID
	require.NoError(t, rowOwner.QueryRowxContext(ctx, `SELECT id FROM public.agents WHERE id=$1 FOR UPDATE`, agentID).Scan(&lockedID))
	require.Equal(t, agentID, lockedID)

	subject := id.NewPrincipal("ledger-row-lock-order")
	owner, err := adapter.BeginTX(ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{
		Lifecycle: ports.StorageLifecycleShared,
		Subjects:  []ports.StorageSubjectGate{{Principal: subject}},
	}))
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	var gates int
	readGates := func() int {
		t.Helper()
		require.NoError(t, db.GetContext(ctx, &gates, `SELECT count(*) FROM pg_locks l
			JOIN pg_stat_activity a ON a.pid=l.pid
			WHERE a.datname=current_database() AND l.locktype='advisory'
				AND l.objsubid=2 AND l.mode='ShareLock' AND l.granted`))
		return gates
	}
	require.Equal(t, 2, readGates(), "begin must acquire lifecycle and subject gates before business-object locks")
	finished := make(chan error, 1)
	go func() { finished <- NewAgentRepository(adapter).Delete(owner, agentID) }()
	select {
	case err := <-finished:
		t.Fatalf("business-row mutation bypassed its held row lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.Equal(t, 2, readGates(), "lifecycle and subject gates remain held while business-row access waits")
	require.NoError(t, rowOwner.Commit())
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("business-row mutation did not resume after its row lock was released")
	}
	require.NoError(t, adapter.Rollback(owner))
	var agentCount int
	require.NoError(t, db.Get(&agentCount, `SELECT count(*) FROM public.agents WHERE id=$1`, agentID))
	require.Equal(t, 1, agentCount, "rolling back owner must undo the business-row deletion")
}

func TestPostgresBusinessEvent_MissingPartitionIsStorageFailure(t *testing.T) {
	adapter, repo := setupPostgresBusinessEventAdapter(t)
	ctx := context.Background()
	event := registeredAgentEvent(t, id.NewAgentID())
	require.NoError(t, repo.Append(ctx, event, false))
	var partition string
	require.NoError(t, adapter.db.QueryRowxContext(ctx, `SELECT tableoid::regclass::text
		FROM public.business_events WHERE recorded_at=$1 AND id=$2`, event.RecordedAt, event.ID).Scan(&partition))
	_, err := adapter.db.ExecContext(ctx, "DROP TABLE "+partition)
	require.NoError(t, err)
	missing := registeredAgentEvent(t, id.NewAgentID())
	err = repo.Append(ctx, missing, false)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	require.Equal(t, storage.ErrorKindConnection, storageErr.Kind, "valid input cannot repair absent storage partitions")
	if storageErr.Cause != nil {
		require.NotContains(t, storageErr.Cause.Error(), missing.ID.String())
	}
}
