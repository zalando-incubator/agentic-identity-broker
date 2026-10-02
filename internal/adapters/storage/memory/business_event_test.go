package memory_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	storagefactory "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func memoryLedgerStore(t *testing.T) *storagefactory.Adapter {
	t.Helper()
	store, err := storagefactory.NewAdapter(&ports.StorageConfig{Backend: "memory"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close(context.Background())) })
	return store
}

// Use reviewed per-type examples, replacing only their synthetic event ID. The
// example's old recorded_at is intentional: append must assign the storage time.
func memoryLedgerExample(t *testing.T, name string) *model.BusinessEvent {
	t.Helper()
	contents, err := fs.ReadFile(eventschemas.Schemas, "examples.json")
	require.NoError(t, err)
	var examples []*model.BusinessEvent
	require.NoError(t, json.Unmarshal(contents, &examples))
	for _, example := range examples {
		if example.Type == model.BusinessEventTypePrefix+name {
			example.ID = id.NewBusinessEventID()
			memoryLedgerValidate(t, example)
			return example
		}
	}
	t.Fatalf("no published business-event example for %q", name)
	return nil
}

func memoryLedgerValidate(t *testing.T, event *model.BusinessEvent) {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	_, err = registry.Validate(event)
	require.NoError(t, err)
}

func memoryLedgerAppend(t *testing.T, store *storagefactory.Adapter, ctx context.Context, event *model.BusinessEvent, queue bool) model.BusinessEventKey {
	t.Helper()
	before := time.Now().UTC()
	require.NoError(t, store.BusinessEvents().Append(ctx, event, queue))
	after := time.Now().UTC()
	require.False(t, event.RecordedAt.IsZero(), "append must assign the recorded time to the caller's event so it can be addressed by key")
	require.True(t, !event.RecordedAt.Before(before.Add(-time.Second)) && !event.RecordedAt.After(after.Add(time.Second)), "recorded_at must come from storage, not the published fixture")
	return model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
}

func memoryLedgerSubject(principal id.Principal) model.BusinessEventSubject {
	return model.BusinessEventSubject{Principal: principal}
}

func TestMemoryBusinessEventAppendGetAndIndependentValues(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	event := memoryLedgerExample(t, "approval-denied")
	permission1, permission2 := id.NewPermissionSetID(), id.NewPermissionSetID()
	event.PermissionSetIDs = []id.PermissionSetID{permission1, permission2}
	event.Client = &model.BusinessEventClient{IP: "192.0.2.10", UserAgent: "curl"}
	onBehalfOf := id.Principal("delegating-principal")
	event.Actor.OnBehalfOf = &onBehalfOf
	memoryLedgerValidate(t, event)

	key := memoryLedgerAppend(t, store, ctx, event, false)
	want, err := json.Marshal(event)
	require.NoError(t, err)

	event.ID = id.NewBusinessEventID()
	event.ApprovalID = id.NewApprovalID()
	event.RecordedAt = time.Time{}
	*event.Subject = "changed-input-subject"
	*event.Actor.ID = "changed-input-actor"
	*event.Actor.OnBehalfOf = "changed-input-delegator"
	event.PermissionSetIDs[0] = id.NewPermissionSetID()
	event.Client.IP = "198.51.100.12"
	event.Data["reason_code"] = "changed-input-reason"

	read, err := store.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, key.ID, read.ID)
	require.Equal(t, key.RecordedAt, read.RecordedAt)
	got, err := json.Marshal(read)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "retained event must not alias the append input")

	read.ID = id.NewBusinessEventID()
	read.ApprovalID = id.NewApprovalID()
	read.RecordedAt = time.Time{}
	*read.Subject = "changed-get-subject"
	*read.Actor.ID = "changed-get-actor"
	*read.Actor.OnBehalfOf = "changed-get-delegator"
	read.PermissionSetIDs[0] = id.NewPermissionSetID()
	read.Client.IP = "203.0.113.9"
	read.Data["reason_code"] = "changed-get-reason"

	second, err := store.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	got, err = json.Marshal(second)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "a Get result must not mutate the retained event")

	query := model.BusinessEventQuery{
		Subject: memoryLedgerSubject(*second.Subject),
		Start:   second.OccurredAt.Add(-time.Second),
		End:     second.OccurredAt.Add(time.Second),
	}
	rows, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, key.ID, rows[0].ID)
	rows[0].Data["reason_code"] = "changed-query-reason"
	rows[0].PermissionSetIDs[0] = id.NewPermissionSetID()
	*rows[0].Subject = "changed-query-subject"
	again, err := store.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	got, err = json.Marshal(again)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "a Query result must not mutate the retained event")

	_, err = store.BusinessEvents().Get(ctx, model.BusinessEventKey{RecordedAt: key.RecordedAt.Add(time.Microsecond), ID: key.ID})
	require.True(t, ports.IsNotFoundErr(err), "a different recorded_at must not retrieve an event merely by ID")
	_, err = store.BusinessEvents().Get(ctx, model.BusinessEventKey{RecordedAt: key.RecordedAt, ID: id.NewBusinessEventID()})
	require.True(t, ports.IsNotFoundErr(err), "a different ID must not retrieve an event merely by recorded_at")
}

func TestMemoryBusinessEventSurvivesReferencedGrantDeletion(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	event := memoryLedgerExample(t, "grant-created")
	grant := &storage.UserGrant{
		Principal: *event.Subject,
		AgentID:   event.AgentID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.UserGrants().Create(ctx, grant))
	event.GrantID = grant.ID
	memoryLedgerValidate(t, event)
	key := memoryLedgerAppend(t, store, ctx, event, false)
	require.NoError(t, store.UserGrants().Delete(ctx, grant.ID))
	_, err := store.UserGrants().Get(ctx, grant.ID)
	require.True(t, ports.IsNotFoundErr(err))

	retained, err := store.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, grant.ID, retained.GrantID)
	require.Equal(t, grant.Principal, *retained.Subject)
	rows, err := store.BusinessEvents().Query(ctx, model.BusinessEventQuery{
		Subject: memoryLedgerSubject(grant.Principal),
		Start:   event.OccurredAt.Add(-time.Second),
		End:     event.OccurredAt.Add(time.Second),
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, key.ID, rows[0].ID)
}

func TestMemoryBusinessEventQueryScopeRangeAndFilters(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	start := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	subject := id.Principal("principal-example")
	other := id.Principal("other-principal")
	created := memoryLedgerExample(t, "grant-created")
	created.OccurredAt = start
	memoryLedgerAppend(t, store, ctx, created, false)
	updated := memoryLedgerExample(t, "grant-updated")
	updated.OccurredAt = start.Add(time.Hour)
	memoryLedgerAppend(t, store, ctx, updated, false)
	denied := memoryLedgerExample(t, "token-exchange-denied")
	denied.OccurredAt = start.Add(2 * time.Hour)
	memoryLedgerAppend(t, store, ctx, denied, false)
	atEnd := memoryLedgerExample(t, "grant-created")
	atEnd.OccurredAt = start.Add(3 * time.Hour)
	atEndKey := memoryLedgerAppend(t, store, ctx, atEnd, false)
	atEndRow, err := store.BusinessEvents().Get(ctx, atEndKey)
	require.NoError(t, err)
	require.Equal(t, atEnd.ID, atEndRow.ID)
	elsewhere := memoryLedgerExample(t, "grant-created")
	elsewhere.OccurredAt = start.Add(time.Hour)
	elsewhere.Subject = &other
	memoryLedgerValidate(t, elsewhere)
	memoryLedgerAppend(t, store, ctx, elsewhere, false)
	noSubject := memoryLedgerExample(t, "agent-registered")
	noSubject.OccurredAt = start.Add(time.Hour)
	memoryLedgerAppend(t, store, ctx, noSubject, false)

	query := model.BusinessEventQuery{Subject: memoryLedgerSubject(subject), Start: start, End: start.Add(3 * time.Hour)}
	rows, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{created.ID, updated.ID, denied.ID}, memoryLedgerIDs(rows), "start is inclusive and end is exclusive; other subjects must not leak")
	query.Start = start.Add(time.Hour)
	query.End = start.Add(2 * time.Hour)
	query.Type = updated.Type
	query.Outcome = model.BusinessEventSuccess
	rows, err = store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{updated.ID}, memoryLedgerIDs(rows))
	query.Outcome = model.BusinessEventDenied
	rows, err = store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Empty(t, rows, "type and outcome filters must both apply")

	query = model.BusinessEventQuery{Subject: model.BusinessEventSubject{NoSubject: true}, Start: start, End: start.Add(3 * time.Hour)}
	rows, err = store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{noSubject.ID}, memoryLedgerIDs(rows), "no-subject is an exact NULL selector, not an unfiltered scan")
	require.Nil(t, rows[0].Subject)
	query.Subject = memoryLedgerSubject(other)
	rows, err = store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{elsewhere.ID}, memoryLedgerIDs(rows))
}

func TestMemoryBusinessEventQueryStrictTupleCursor(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	tiedAt := time.Date(2026, 9, 26, 0, 0, 0, 123456000, time.UTC)
	subject := id.Principal("principal-example")
	preceding := memoryLedgerExample(t, "grant-created")
	preceding.OccurredAt = tiedAt.Add(-time.Microsecond)
	memoryLedgerAppend(t, store, ctx, preceding, false)
	ties := make([]*model.BusinessEvent, 3)
	for i := range ties {
		ties[i] = memoryLedgerExample(t, "grant-created")
		ties[i].OccurredAt = tiedAt
	}
	for i := len(ties) - 1; i >= 0; i-- {
		memoryLedgerAppend(t, store, ctx, ties[i], false)
	}
	ordered := []id.BusinessEventID{ties[0].ID, ties[1].ID, ties[2].ID}
	slices.SortFunc(ordered, func(a, b id.BusinessEventID) int { return bytes.Compare(a[:], b[:]) })
	following := memoryLedgerExample(t, "grant-created")
	following.OccurredAt = tiedAt.Add(time.Microsecond)
	memoryLedgerAppend(t, store, ctx, following, false)

	query := model.BusinessEventQuery{
		Subject: memoryLedgerSubject(subject),
		Start:   tiedAt.Add(-time.Second), End: tiedAt.Add(time.Second), Limit: 2,
	}
	first, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{preceding.ID, ordered[0]}, memoryLedgerIDs(first))
	query.After = &model.BusinessEventCursor{OccurredAt: first[1].OccurredAt, ID: first[1].ID}
	second, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{ordered[1], ordered[2]}, memoryLedgerIDs(second), "continuation excludes exactly the cursor tuple, not all events at its time")
	query.After = &model.BusinessEventCursor{OccurredAt: second[1].OccurredAt, ID: second[1].ID}
	third, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []id.BusinessEventID{following.ID}, memoryLedgerIDs(third))
	query.After = &model.BusinessEventCursor{OccurredAt: third[0].OccurredAt, ID: third[0].ID}
	empty, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestMemoryBusinessEventQueryLimitsAndInvalidRanges(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	start := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	template := memoryLedgerExample(t, "grant-created")
	for i := range 1001 {
		event := *template
		event.ID = id.NewBusinessEventID()
		event.OccurredAt = start.Add(time.Duration(i) * time.Microsecond)
		memoryLedgerAppend(t, store, ctx, &event, false)
	}
	query := model.BusinessEventQuery{
		Subject: memoryLedgerSubject(*template.Subject),
		Start:   start, End: start.Add(time.Second),
	}
	defaultPage, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, defaultPage, 200)
	query.Limit = 201
	explicitPage, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, explicitPage, 201)
	require.Equal(t, defaultPage[199].ID, explicitPage[199].ID)
	query.Limit = 1000
	maximumPage, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, maximumPage, 1000)
	query.After = &model.BusinessEventCursor{OccurredAt: maximumPage[999].OccurredAt, ID: maximumPage[999].ID}
	remainder, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, remainder, 1, "cursor-based pagination must reach the row beyond the maximum page")

	query.After = nil
	for _, invalid := range []model.BusinessEventQuery{
		{Subject: query.Subject, Start: start, End: start, Limit: 1},
		{Subject: query.Subject, Start: start.Add(time.Second), End: start, Limit: 1},
		{Subject: query.Subject, Start: start.In(time.FixedZone("+0100", 3600)), End: query.End, Limit: 1},
		{Subject: query.Subject, Start: start, End: query.End, Limit: -1},
		{Subject: query.Subject, Start: start, End: query.End, Limit: 1001},
		{Start: start, End: query.End, Limit: 1},
		{Subject: model.BusinessEventSubject{Principal: *template.Subject, NoSubject: true}, Start: start, End: query.End, Limit: 1},
	} {
		rows, err := store.BusinessEvents().Query(ctx, invalid)
		require.Error(t, err, "invalid query must not silently broaden scope or weaken bounds: %+v", invalid)
		require.Empty(t, rows)
	}
}

func memoryLedgerIDs(rows []*model.BusinessEvent) []id.BusinessEventID {
	ids := make([]id.BusinessEventID, 0, len(rows))
	for _, event := range rows {
		ids = append(ids, event.ID)
	}
	return ids
}
