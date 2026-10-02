package ledger

import (
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/google/uuid"
)

var errInvalidQuery = errors.New("invalid business event query")

func (r *Registry) ValidateQuery(query model.BusinessEventQuery) error {
	if r == nil || query.Subject.NoSubject != query.Subject.Principal.IsZero() ||
		!isUTC(query.Start) || !isUTC(query.End) || !query.Start.Before(query.End) ||
		query.Limit < 0 || query.Limit > 1000 {
		return errInvalidQuery
	}
	if query.Outcome != "" && query.Outcome != model.BusinessEventSuccess && query.Outcome != model.BusinessEventFailure &&
		query.Outcome != model.BusinessEventDenied && query.Outcome != model.BusinessEventPending {
		return errInvalidQuery
	}
	if query.Type != "" {
		if _, exists := r.events[query.Type]; !exists {
			return errInvalidQuery
		}
	}
	if cursor := query.After; cursor != nil {
		if !isUTC(cursor.OccurredAt) || cursor.OccurredAt.Before(query.Start) || !cursor.OccurredAt.Before(query.End) ||
			cursor.ID.IsZero() || uuid.UUID(cursor.ID).Version() != 7 || uuid.UUID(cursor.ID).Variant() != uuid.RFC4122 {
			return errInvalidQuery
		}
	}
	return nil
}
