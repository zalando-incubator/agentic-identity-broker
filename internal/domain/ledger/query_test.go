package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestServiceQueryRejectsInvalidScopeAndFiltersBeforeRepository(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	start := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	principal := id.NewPrincipal("ledger-subject")
	cursor := model.BusinessEventCursor{
		OccurredAt: start.Add(time.Minute),
		ID:         id.MustParseBusinessEventID("01997100-0000-7000-8000-000000000001"),
	}
	base := model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: principal},
		Start:   start,
		End:     end,
	}
	for _, tc := range []struct {
		name   string
		change func(*model.BusinessEventQuery)
	}{
		{"missing subject selection", func(q *model.BusinessEventQuery) { q.Subject = model.BusinessEventSubject{} }},
		{"exact and no-subject together", func(q *model.BusinessEventQuery) { q.Subject.NoSubject = true }},
		{"unknown event type", func(q *model.BusinessEventQuery) {
			q.Type = "agentic-identity-broker.unregistered"
		}},
		{"untrusted namespace", func(q *model.BusinessEventQuery) {
			q.Type = "deployment-name.grant-created"
		}},
		{"unknown outcome", func(q *model.BusinessEventQuery) {
			q.Outcome = model.BusinessEventOutcome("credential-bearing-input-canary")
		}},
		{"outcome incompatible with registered type", func(q *model.BusinessEventQuery) {
			q.Type, q.Outcome = "agentic-identity-broker.grant-created", model.BusinessEventFailure
		}},
		{"missing start", func(q *model.BusinessEventQuery) { q.Start = time.Time{} }},
		{"missing end", func(q *model.BusinessEventQuery) { q.End = time.Time{} }},
		{"non-UTC start", func(q *model.BusinessEventQuery) {
			q.Start = start.In(time.FixedZone("untrusted start", 3600))
		}},
		{"non-UTC end", func(q *model.BusinessEventQuery) {
			q.End = end.In(time.FixedZone("untrusted end", -3600))
		}},
		{"empty interval", func(q *model.BusinessEventQuery) { q.End = q.Start }},
		{"reversed interval", func(q *model.BusinessEventQuery) { q.End = q.Start.Add(-time.Nanosecond) }},
		{"negative limit", func(q *model.BusinessEventQuery) { q.Limit = -1 }},
		{"limit over maximum", func(q *model.BusinessEventQuery) { q.Limit = 1001 }},
		{"cursor missing occurrence", func(q *model.BusinessEventQuery) {
			q.After = &model.BusinessEventCursor{ID: cursor.ID}
		}},
		{"cursor non-UTC occurrence", func(q *model.BusinessEventQuery) {
			q.After = &model.BusinessEventCursor{OccurredAt: cursor.OccurredAt.In(time.FixedZone("untrusted cursor", 3600)), ID: cursor.ID}
		}},
		{"cursor missing event ID", func(q *model.BusinessEventQuery) {
			q.After = &model.BusinessEventCursor{OccurredAt: cursor.OccurredAt}
		}},
		{"cursor non-event UUID version", func(q *model.BusinessEventQuery) {
			q.After = &model.BusinessEventCursor{
				OccurredAt: cursor.OccurredAt,
				ID:         id.MustParseBusinessEventID("11111111-1111-4111-8111-111111111111"),
			}
		}},
		{"cursor before interval", func(q *model.BusinessEventQuery) {
			q.After = &model.BusinessEventCursor{OccurredAt: start.Add(-time.Nanosecond), ID: cursor.ID}
		}},
		{"cursor at exclusive end", func(q *model.BusinessEventQuery) {
			q.After = &model.BusinessEventCursor{OccurredAt: end, ID: cursor.ID}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := base
			tc.change(&query)
			service := NewService(registry, ledgerServiceEvents{
				queryFn: func(context.Context, model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
					t.Fatal("forbidden investigation must never reach storage")
					return nil, nil
				},
			}, nil, nil, false)
			found, err := service.Query(context.Background(), query)
			require.Error(t, err)
			require.Nil(t, found)
			require.NotContains(t, err.Error(), "credential-bearing-input-canary")
		})
	}
}

func TestServiceQueryPermitsExactAndNoSubjectQueriesWithoutChangingFilters(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	start := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	principal := id.NewPrincipal("ledger-subject")
	cursor := model.BusinessEventCursor{
		OccurredAt: start,
		ID:         id.MustParseBusinessEventID("01997100-0000-7000-8000-000000000001"),
	}
	for _, tc := range []struct {
		name  string
		query model.BusinessEventQuery
	}{
		{"exact subject with type outcome and strict continuation", model.BusinessEventQuery{
			Subject: model.BusinessEventSubject{Principal: principal},
			Type:    "agentic-identity-broker.grant-created",
			Outcome: model.BusinessEventSuccess,
			Start:   start,
			End:     end,
			After:   &cursor,
			Limit:   37,
		}},
		{"explicit no-subject with repository default limit", model.BusinessEventQuery{
			Subject: model.BusinessEventSubject{NoSubject: true},
			Start:   start,
			End:     end,
		}},
		{"outcome without type at maximum bound", model.BusinessEventQuery{
			Subject: model.BusinessEventSubject{Principal: principal},
			Outcome: model.BusinessEventDenied,
			Start:   start,
			End:     end,
			Limit:   1000,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unavailable := errors.New("query storage unavailable")
			calls := 0
			service := NewService(registry, ledgerServiceEvents{
				queryFn: func(_ context.Context, got model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
					calls++
					require.Equal(t, tc.query, got, "validation must not widen, discard or reorder investigation filters")
					return nil, unavailable
				},
			}, nil, nil, false)
			found, err := service.Query(context.Background(), tc.query)
			require.ErrorIs(t, err, unavailable, "a valid investigation must reach its repository")
			require.Nil(t, found)
			require.Equal(t, 1, calls)
		})
	}
}
