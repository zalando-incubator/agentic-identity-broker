package ledger_test

import (
	"context"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	storagefactory "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type forbiddenLifecycleRepository struct{ t *testing.T }

func (r forbiddenLifecycleRepository) EraseSubject(context.Context, id.Principal) (int64, error) {
	r.t.Fatal("invalid erasure selector reached storage")
	return 0, nil
}

func (r forbiddenLifecycleRepository) SetRetentionPolicy(context.Context, time.Duration) error {
	r.t.Fatal("invalid retention policy reached storage")
	return nil
}

func (r forbiddenLifecycleRepository) ApplyRetention(context.Context) error {
	r.t.Fatal("invalid lifecycle request reached storage")
	return nil
}

func TestLifecycleServiceRejectsEmptySubjectBeforeErasure(t *testing.T) {
	service := ledger.NewService(nil, nil, forbiddenLifecycleRepository{t}, nil, false)
	count, err := service.EraseSubject(t.Context(), id.NewPrincipal(""))
	require.Error(t, err)
	require.Zero(t, count)
}

func TestLifecycleServiceRejectsInvalidRetentionBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		retention time.Duration
	}{
		{name: "zero", retention: 0},
		{name: "negative nanosecond", retention: -time.Nanosecond},
		{name: "negative day", retention: -24 * time.Hour},
		{name: "ceil would overflow duration", retention: time.Duration(1<<63 - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := ledger.NewService(nil, nil, forbiddenLifecycleRepository{t}, nil, false)
			require.Error(t, service.SetRetentionPolicy(t.Context(), tc.retention))
		})
	}
}

func TestLifecycleServiceErasesOnlyCommittedExactSubjectWithoutReplacement(t *testing.T) {
	store, err := storagefactory.NewAdapter(&ports.StorageConfig{Backend: "memory"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close(context.Background())) })
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	service := ledger.NewService(registry, store.BusinessEvents(), store.BusinessEventLifecycle(), store, false)

	subject := id.NewPrincipal("subject-to-erase")
	other := id.NewPrincipal("subject-to-erase-extra")
	actor := subject.String()
	occurred := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	record := func(name string, selected *id.Principal, caller string) model.BusinessEventKey {
		t.Helper()
		facts := model.BusinessEvent{
			OccurredAt: occurred,
			Subject:    selected,
			Actor:      model.BusinessEventActor{Kind: "agent", ID: &caller},
			Data:       map[string]any{"reason_code": "invalid_request"},
		}
		if name == "grant-created" {
			facts.Actor.Kind = "user"
			facts.AgentID = id.MustParseAgentID("11111111-1111-4111-8111-111111111111")
			facts.GrantID = id.NewGrantID()
			facts.Data = map[string]any{}
		}
		event, err := service.NewEvent(t.Context(), model.BusinessEventTypePrefix+name, facts)
		require.NoError(t, err)
		require.NoError(t, service.Record(t.Context(), event))
		return model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
	}

	first := record("grant-created", &subject, "different-caller")
	second := record("token-request-failed", &subject, "another-caller")
	otherKey := record("token-request-failed", &other, actor)
	noSubjectKey := record("token-request-failed", nil, actor)

	removed, err := service.EraseSubject(t.Context(), subject)
	require.NoError(t, err)
	require.EqualValues(t, 2, removed, "count only committed matching event rows")
	for _, key := range []model.BusinessEventKey{first, second} {
		_, err := service.Get(t.Context(), key)
		require.ErrorIs(t, err, ports.ErrNotFound)
	}
	for _, key := range []model.BusinessEventKey{otherKey, noSubjectKey} {
		_, err := service.Get(t.Context(), key)
		require.NoError(t, err, "another subject or a null subject must survive even when its actor matches")
	}

	query := func(selector model.BusinessEventSubject) []*model.BusinessEvent {
		t.Helper()
		found, err := service.Query(t.Context(), model.BusinessEventQuery{
			Subject: selector,
			Start:   time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			End:     time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)
		return found
	}
	require.Empty(t, query(model.BusinessEventSubject{Principal: subject}), "erasure must not append a tombstone or replacement event")
	require.Len(t, query(model.BusinessEventSubject{Principal: other}), 1)
	require.Len(t, query(model.BusinessEventSubject{NoSubject: true}), 1)

	removed, err = service.EraseSubject(t.Context(), subject)
	require.NoError(t, err)
	require.Zero(t, removed, "completed erasure is idempotent")
	newOccurrence := record("token-request-failed", &subject, "later-caller")
	found := query(model.BusinessEventSubject{Principal: subject})
	require.Len(t, found, 1)
	require.Equal(t, newOccurrence.ID, found[0].ID, "later legitimate occurrences remain possible")
}
