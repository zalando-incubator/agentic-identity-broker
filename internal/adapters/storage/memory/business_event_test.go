package memory

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func memoryBusinessEvent() *model.BusinessEvent {
	return &model.BusinessEvent{
		ID:         id.NewBusinessEventID(),
		Event:      "test.ledger.recorded",
		OccurredAt: time.Date(2025, time.January, 2, 3, 4, 5, 123456789, time.UTC),
	}
}

func TestBusinessEventAppendStoresIsolatedCopy(t *testing.T) {
	adapter := NewAdapter()
	principal, actor := "policy:owner", "policy:operator"
	event := memoryBusinessEvent()
	event.Principal, event.Actor = &principal, &actor
	event.TraceID = "0123456789abcdef0123456789abcdef"
	event.RecordedAt = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	event.Attributes = map[string]any{
		"nested": map[string]any{"items": []any{map[string]any{"value": "original"}}},
		"array":  [1]map[string]any{{"items": []string{"original"}}},
	}
	before := time.Now()
	require.NoError(t, adapter.Append(context.Background(), event))
	after := time.Now()
	require.False(t, event.RecordedAt.IsZero(), "Append must return storage time")
	assert.False(t, event.RecordedAt.Before(before))
	assert.False(t, event.RecordedAt.After(after))
	require.Len(t, adapter.businessEvents, 1)
	stored := adapter.businessEvents[0]
	assert.Equal(t, event.ID, stored.ID)
	require.NotSame(t, event, stored)
	assert.True(t, event.RecordedAt.Equal(stored.RecordedAt))
	assert.Equal(t, event.TraceID, stored.TraceID)

	principal, actor = "changed owner", "changed actor"
	event.Attributes["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "changed"
	event.Attributes["array"].([1]map[string]any)[0]["items"].([]string)[0] = "changed"
	event.Event = "test.ledger.changed"
	assert.Equal(t, "policy:owner", *stored.Principal)
	assert.Equal(t, "policy:operator", *stored.Actor)
	assert.Equal(t, "original", stored.Attributes["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"])
	assert.Equal(t, "original", stored.Attributes["array"].([1]map[string]any)[0]["items"].([]string)[0])
	assert.Equal(t, "test.ledger.recorded", stored.Event)
}

func TestBusinessEventAppendOccurrencePrecision(t *testing.T) {
	for _, occurredAt := range []time.Time{
		time.Date(2025, time.January, 2, 3, 4, 5, 123456789, time.UTC),
		time.Date(2025, time.January, 2, 3, 4, 5, 999999999, time.FixedZone("offset", 5*3600+1800)),
	} {
		t.Run(occurredAt.Format(time.RFC3339Nano), func(t *testing.T) {
			adapter := NewAdapter()
			event := memoryBusinessEvent()
			event.OccurredAt = occurredAt
			require.NoError(t, adapter.Append(context.Background(), event))
			require.Len(t, adapter.businessEvents, 1)
			stored := adapter.businessEvents[0]
			assert.Equal(t, event.ID, stored.ID)
			assert.True(t, occurredAt.Equal(stored.OccurredAt), "memory must preserve every nanosecond")
			assert.Equal(t, occurredAt, event.OccurredAt, "Append must not normalize or truncate caller occurrence time")
		})
	}
}

func TestBusinessEventAppendNilAttributesAndDuplicate(t *testing.T) {
	adapter := NewAdapter()
	event := memoryBusinessEvent()
	require.NoError(t, adapter.Append(context.Background(), event))
	require.Len(t, adapter.businessEvents, 1)
	stored := adapter.businessEvents[0]
	assert.Equal(t, event.ID, stored.ID)
	assert.NotNil(t, stored.Attributes)
	assert.Empty(t, stored.Attributes)
	assert.Nil(t, stored.Principal)
	assert.Nil(t, stored.Actor)
	assert.Nil(t, event.Attributes, "normalization must not mutate caller attributes")
	recordedAt := stored.RecordedAt
	event.Event = "test.ledger.duplicate"
	var storageErr *storage.StorageError
	require.ErrorAs(t, adapter.Append(context.Background(), event), &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.Len(t, adapter.businessEvents, 1)
	assert.Equal(t, "test.ledger.recorded", stored.Event)
	assert.True(t, recordedAt.Equal(stored.RecordedAt))
}

func TestBusinessEventAppendPreservesNUL(t *testing.T) {
	adapter := NewAdapter()
	event := memoryBusinessEvent()
	event.Attributes = map[string]any{"nested": []any{"a\x00b"}}
	require.NoError(t, adapter.Append(t.Context(), event))
	require.Len(t, adapter.businessEvents, 1)
	assert.Equal(t, event.Attributes, adapter.businessEvents[0].Attributes)
}

func TestBusinessEventAppendRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*model.BusinessEvent) *model.BusinessEvent
	}{
		{"nil", func(*model.BusinessEvent) *model.BusinessEvent { return nil }},
		{"zero ID", func(event *model.BusinessEvent) *model.BusinessEvent { event.ID = id.BusinessEventID{}; return event }},
		{"empty event", func(event *model.BusinessEvent) *model.BusinessEvent { event.Event = ""; return event }},
		{"zero occurrence", func(event *model.BusinessEvent) *model.BusinessEvent { event.OccurredAt = time.Time{}; return event }},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := NewAdapter()
			var storageErr *storage.StorageError
			require.ErrorAs(t, adapter.Append(context.Background(), test.edit(memoryBusinessEvent())), &storageErr)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
			assert.Empty(t, adapter.businessEvents)
		})
	}
}

func TestBusinessEventDeleteRecordedBeforeOrdersAndLimits(t *testing.T) {
	adapter := NewAdapter()
	cutoff := time.Date(2025, time.March, 4, 5, 6, 7, 0, time.UTC)
	ids := []id.BusinessEventID{
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000001")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000002")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000003")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000004")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000005")),
	}
	// The highest ID is oldest. The next two tie and must be ordered by ID.
	for index, recordedAt := range []time.Time{cutoff.Add(-time.Hour), cutoff.Add(-time.Hour), cutoff, cutoff.Add(time.Hour), cutoff.Add(-2 * time.Hour)} {
		event := memoryBusinessEvent()
		event.ID, event.RecordedAt = ids[index], recordedAt
		adapter.businessEvents = append(adapter.businessEvents, event)
	}
	deleted, err := adapter.DeleteRecordedBefore(context.Background(), cutoff, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.NotContains(t, memoryBusinessEventIDs(adapter), ids[4])
	assert.Contains(t, memoryBusinessEventIDs(adapter), ids[0])

	deleted, err = adapter.DeleteRecordedBefore(context.Background(), cutoff, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.NotContains(t, memoryBusinessEventIDs(adapter), ids[0])
	assert.Contains(t, memoryBusinessEventIDs(adapter), ids[1], "equal timestamps must use ID order")

	deleted, err = adapter.DeleteRecordedBefore(context.Background(), cutoff, 1000)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.Contains(t, memoryBusinessEventIDs(adapter), ids[2], "cutoff is exclusive")
	assert.Contains(t, memoryBusinessEventIDs(adapter), ids[3])
	deleted, err = adapter.DeleteRecordedBefore(context.Background(), cutoff, 1000)
	require.NoError(t, err)
	assert.Zero(t, deleted)
}

func TestBusinessEventOperationsRejectExpiredContexts(t *testing.T) {
	adapter := NewAdapter()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	var storageErr *storage.StorageError
	require.ErrorAs(t, adapter.Append(ctx, memoryBusinessEvent()), &storageErr)
	assert.Equal(t, storage.ErrorKindTimeout, storageErr.Kind)
	assert.Empty(t, adapter.businessEvents)
	deleted, err := adapter.DeleteRecordedBefore(ctx, time.Now(), 1)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindTimeout, storageErr.Kind)
	assert.Zero(t, deleted)
}

func TestBusinessEventAppendTimesOutWhileStorageIsLocked(t *testing.T) {
	adapter := NewAdapter()
	require.True(t, adapter.ledgerLock.TryAcquire(1))
	defer adapter.ledgerLock.Release(1)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- adapter.Append(ctx, memoryBusinessEvent()) }()
	select {
	case err := <-result:
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindTimeout, storageErr.Kind)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("append waited for the lock after its deadline")
	}
}

func memoryBusinessEventIDs(adapter *Adapter) []id.BusinessEventID {
	ids := make([]id.BusinessEventID, len(adapter.businessEvents))
	for index, event := range adapter.businessEvents {
		ids[index] = event.ID
	}
	return ids
}
