package ledger

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

var errInvalidEvent = errors.New("invalid business event")

func validateEventValues(e *model.BusinessEvent) error {
	if e == nil || e.Source != model.BusinessEventSource || !strings.HasPrefix(e.Type, model.BusinessEventTypePrefix) || e.ID.IsZero() || uuid.UUID(e.ID).Version() != 7 || uuid.UUID(e.ID).Variant() != uuid.RFC4122 || !isUTC(e.OccurredAt) || !isUTC(e.RecordedAt) || e.Data == nil {
		return errInvalidEvent
	}
	if e.Subject != nil && e.Subject.IsZero() || e.Actor.ID != nil && *e.Actor.ID == "" || e.Actor.OnBehalfOf != nil && e.Actor.OnBehalfOf.IsZero() {
		return errInvalidEvent
	}
	switch e.Actor.Kind {
	case "user", "admin", "agent", "gateway", "policy":
	case "system":
		if e.Actor.ID == nil || *e.Actor.ID != "broker-lifecycle" {
			return errInvalidEvent
		}
	default:
		return errInvalidEvent
	}
	for _, reference := range e.PermissionSetIDs {
		if reference.IsZero() {
			return errInvalidEvent
		}
	}
	if e.TraceID != "" {
		if _, err := trace.TraceIDFromHex(e.TraceID); err != nil {
			return errInvalidEvent
		}
	}
	if e.SpanID != "" {
		if _, err := trace.SpanIDFromHex(e.SpanID); err != nil || e.TraceID == "" {
			return errInvalidEvent
		}
	}
	if e.Client != nil {
		if e.Client.IP != "" {
			ip, err := netip.ParseAddr(e.Client.IP)
			if err != nil || ip.Zone() != "" || ip.String() != e.Client.IP {
				return errInvalidEvent
			}
		}
		if e.Client.UserAgent != "" && !slices.Contains([]string{"Chrome", "Firefox", "Safari", "Edge", "curl", "Go-http-client"}, e.Client.UserAgent) {
			return errInvalidEvent
		}
	}
	return nil
}

func validPayloadUUID(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	parsed, err := uuid.Parse(text)
	return err == nil && parsed != uuid.Nil && parsed.String() == text
}

func isUTC(instant time.Time) bool {
	_, offset := instant.Zone()
	return !instant.IsZero() && offset == 0
}
