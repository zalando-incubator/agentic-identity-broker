package ledger

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/google/uuid"
)

var errInvalidEvent = errors.New("invalid business event")

type eventRecipe struct {
	outcome model.BusinessEventOutcome
	reason  string
}

var catalogueRecipes = map[string]eventRecipe{
	"grant-created":           {model.BusinessEventSuccess, "A new user-to-agent delegation is committed."},
	"grant-updated":           {model.BusinessEventSuccess, "An existing delegation's effective permissions or validity changes."},
	"grant-revoked":           {model.BusinessEventSuccess, "A delegation is explicitly revoked."},
	"grant-expired":           {model.BusinessEventSuccess, "A delegation reaches expiry and the broker recognizes it."},
	"session-established":     {model.BusinessEventSuccess, "A usable third-party session is established."},
	"session-refreshed":       {model.BusinessEventSuccess, "Refreshed session state is accepted and committed."},
	"session-refresh-failed":  {model.BusinessEventFailure, "A third-party session refresh attempt fails."},
	"session-terminated":      {model.BusinessEventSuccess, "A third-party session is ended."},
	"authorization-requested": {model.BusinessEventPending, "The broker accepts an authorization request for processing."},
	"token-issued":            {model.BusinessEventSuccess, "The broker completes a non-exchange token issuance."},
	"token-request-failed":    {model.BusinessEventFailure, "A token request fails without a more specific exchange-denial classification."},
	"token-exchanged":         {model.BusinessEventSuccess, "The broker completes a token exchange for a receiving agent."},
	"token-exchange-denied":   {model.BusinessEventDenied, "An exchange is refused by authentication or authorization controls."},
	"impersonation-granted":   {model.BusinessEventSuccess, "The broker permits an impersonation decision."},
	"impersonation-denied":    {model.BusinessEventDenied, "The broker refuses an impersonation decision."},
	"approval-requested":      {model.BusinessEventPending, "A new pending approval is created."},
	"approval-approved":       {model.BusinessEventSuccess, "A pending approval is approved."},
	"approval-denied":         {model.BusinessEventDenied, "A pending approval is denied."},
	"approval-consumed":       {model.BusinessEventSuccess, "A single-use approval is consumed."},
	"approval-revoked":        {model.BusinessEventSuccess, "An existing approval is revoked."},
	"approval-expired":        {model.BusinessEventSuccess, "An approval reaches expiry and the broker recognizes it."},
	"agent-registered":        {model.BusinessEventSuccess, "An agent is registered."},
	"agent-updated":           {model.BusinessEventSuccess, "An agent's effective configuration changes."},
	"agent-deleted":           {model.BusinessEventSuccess, "An agent is deleted."},
	"credential-generated":    {model.BusinessEventSuccess, "An agent credential is generated."},
	"credential-rotated":      {model.BusinessEventSuccess, "An agent credential is rotated."},
	"credential-revoked":      {model.BusinessEventSuccess, "An agent credential is revoked."},
	"signing-key-promoted":    {model.BusinessEventSuccess, "A signing key becomes the active signing key."},
}

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
	if e.TraceID != "" && !validHexID(e.TraceID, 32) || e.SpanID != "" && (!validHexID(e.SpanID, 16) || e.TraceID == "") {
		return errInvalidEvent
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

func validHexID(value string, width int) bool {
	if len(value) != width {
		return false
	}
	nonzero := false
	for _, c := range value {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
		if c != '0' {
			nonzero = true
		}
	}
	return nonzero
}
