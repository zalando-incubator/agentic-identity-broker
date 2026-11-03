package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

const BusinessEventTypePrefix = "agentic-identity-broker."
const BusinessEventSource = "urn:agentic-identity-broker:broker"

var errInvalidEventJSON = errors.New("invalid business event envelope")

type BusinessEventOutcome string

const (
	BusinessEventSuccess BusinessEventOutcome = "success"
	BusinessEventFailure BusinessEventOutcome = "failure"
	BusinessEventDenied  BusinessEventOutcome = "denied"
	BusinessEventPending BusinessEventOutcome = "pending"
)

type BusinessEventActor struct {
	Kind       string        `json:"kind"`
	ID         *string       `json:"id"`
	OnBehalfOf *id.Principal `json:"on_behalf_of"`
}

type BusinessEventClient struct {
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

type BusinessEvent struct {
	ID                    id.BusinessEventID   `json:"id"`
	Type                  string               `json:"type"`
	Source                string               `json:"source"`
	OccurredAt            time.Time            `json:"occurred_at"`
	RecordedAt            time.Time            `json:"recorded_at"`
	Subject               *id.Principal        `json:"subject"`
	Actor                 BusinessEventActor   `json:"actor"`
	AgentID               id.AgentID           `json:"agent_id,omitempty"`
	GatewayClientID       id.ClientID          `json:"gateway_client_id,omitempty"`
	ServiceID             id.ServiceID         `json:"service_id,omitempty"`
	PermissionSetIDs      []id.PermissionSetID `json:"permission_set_ids,omitempty"`
	GrantID               id.GrantID           `json:"grant_id,omitempty"`
	SessionID             id.SessionID         `json:"session_id,omitempty"`
	ApprovalID            id.ApprovalID        `json:"approval_id,omitempty"`
	MCPSessionID          string               `json:"mcp_session_id,omitempty"`
	AgentSessionID        string               `json:"agent_session_id,omitempty"`
	Outcome               BusinessEventOutcome `json:"outcome"`
	ReasonUser            string               `json:"reason_user"`
	ReasonAdmin           string               `json:"reason_admin"`
	TraceID               string               `json:"trace_id,omitempty"`
	SpanID                string               `json:"span_id,omitempty"`
	Client                *BusinessEventClient `json:"client,omitempty"`
	Data                  map[string]any       `json:"data"`
	trustedMCPSessionID   string
	trustedAgentSessionID string
	preparedKey           BusinessEventKey
}

// SetTrustedSessionCorrelations is for reviewed non-credential sources only.
// Request headers and ordinary JSON decoding do not establish that provenance.
func (e *BusinessEvent) SetTrustedSessionCorrelations(mcpSessionID, agentSessionID string) {
	e.MCPSessionID, e.trustedMCPSessionID = mcpSessionID, mcpSessionID
	e.AgentSessionID, e.trustedAgentSessionID = agentSessionID, agentSessionID
}

func (e *BusinessEvent) RecordingPrepared() bool {
	return !e.preparedKey.RecordedAt.IsZero() && e.ID == e.preparedKey.ID && e.RecordedAt.Equal(e.preparedKey.RecordedAt)
}

// PrepareRecording is called by storage once; retries retain the same composite key.
func (e *BusinessEvent) PrepareRecording(recordedAt time.Time) {
	e.RecordedAt = recordedAt.UTC().Truncate(time.Microsecond)
	e.OccurredAt = e.OccurredAt.Truncate(time.Microsecond)
	e.preparedKey = BusinessEventKey{RecordedAt: e.RecordedAt, ID: e.ID}
}

// Wire is the single primitive view used for validation and persistence.
func (e *BusinessEvent) Wire() map[string]any {
	var subject, actorID, onBehalfOf any
	if e.Subject != nil {
		subject = e.Subject.String()
	}
	if e.Actor.ID != nil {
		actorID = *e.Actor.ID
	}
	if e.Actor.OnBehalfOf != nil {
		onBehalfOf = e.Actor.OnBehalfOf.String()
	}
	data, _ := businessEventPayloadWire(e.Data)
	wire := map[string]any{
		"id": e.ID.String(), "type": e.Type, "source": e.Source,
		"occurred_at": e.OccurredAt.Format(time.RFC3339Nano), "recorded_at": e.RecordedAt.Format(time.RFC3339Nano),
		"subject": subject, "actor": map[string]any{"kind": e.Actor.Kind, "id": actorID, "on_behalf_of": onBehalfOf},
		"outcome": string(e.Outcome), "reason_user": e.ReasonUser, "reason_admin": e.ReasonAdmin, "data": data,
	}
	if !e.AgentID.IsZero() {
		wire["agent_id"] = e.AgentID.String()
	}
	if !e.GatewayClientID.IsZero() {
		wire["gateway_client_id"] = e.GatewayClientID.String()
	}
	if !e.ServiceID.IsZero() {
		wire["service_id"] = e.ServiceID.String()
	}
	if !e.GrantID.IsZero() {
		wire["grant_id"] = e.GrantID.String()
	}
	if !e.SessionID.IsZero() {
		wire["session_id"] = e.SessionID.String()
	}
	if !e.ApprovalID.IsZero() {
		wire["approval_id"] = e.ApprovalID.String()
	}
	if e.PermissionSetIDs != nil {
		ids := e.PermissionSetIDs
		compare := func(a, b id.PermissionSetID) int { return bytes.Compare(a[:], b[:]) }
		if !slices.IsSortedFunc(ids, compare) {
			ids = slices.Clone(ids)
			slices.SortFunc(ids, compare)
		}
		values := make([]string, 0, len(ids))
		for i, value := range ids {
			if i == 0 || value != ids[i-1] {
				values = append(values, value.String())
			}
		}
		wire["permission_set_ids"] = values
	}
	if e.MCPSessionID != "" && e.MCPSessionID == e.trustedMCPSessionID {
		wire["mcp_session_id"] = e.MCPSessionID
	}
	if e.AgentSessionID != "" && e.AgentSessionID == e.trustedAgentSessionID {
		wire["agent_session_id"] = e.AgentSessionID
	}
	if e.TraceID != "" {
		wire["trace_id"] = e.TraceID
	}
	if e.SpanID != "" {
		wire["span_id"] = e.SpanID
	}
	if e.Client != nil {
		client := map[string]any{}
		if e.Client.IP != "" {
			client["ip"] = e.Client.IP
		}
		if e.Client.UserAgent != "" {
			client["user_agent"] = e.Client.UserAgent
		}
		if len(client) != 0 {
			wire["client"] = client
		}
	}
	return wire
}

func (e *BusinessEvent) MarshalJSON() ([]byte, error) { return json.Marshal(e.Wire()) }

func (e *BusinessEvent) UnmarshalJSON(data []byte) error {
	type eventJSON BusinessEvent
	var event eventJSON
	decoded := struct {
		*eventJSON
		Subject json.RawMessage `json:"subject"`
	}{eventJSON: &event}
	if err := decodeEventJSON(data, &decoded); err != nil {
		return err
	}
	if len(decoded.Subject) == 0 {
		return errInvalidEventJSON
	}
	if string(decoded.Subject) != "null" {
		var subject id.Principal
		if err := json.Unmarshal(decoded.Subject, &subject); err != nil {
			return errInvalidEventJSON
		}
		event.Subject = &subject
	}
	*e = BusinessEvent(event)
	return nil
}

func (a *BusinessEventActor) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Kind       string          `json:"kind"`
		ID         json.RawMessage `json:"id"`
		OnBehalfOf json.RawMessage `json:"on_behalf_of"`
	}
	if err := decodeEventJSON(data, &decoded); err != nil {
		return err
	}
	if len(decoded.ID) == 0 || len(decoded.OnBehalfOf) == 0 {
		return errInvalidEventJSON
	}
	actor := BusinessEventActor{Kind: decoded.Kind}
	if string(decoded.ID) != "null" {
		var identity string
		if err := json.Unmarshal(decoded.ID, &identity); err != nil {
			return errInvalidEventJSON
		}
		actor.ID = &identity
	}
	if string(decoded.OnBehalfOf) != "null" {
		var principal id.Principal
		if err := json.Unmarshal(decoded.OnBehalfOf, &principal); err != nil {
			return errInvalidEventJSON
		}
		actor.OnBehalfOf = &principal
	}
	*a = actor
	return nil
}

func decodeEventJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errInvalidEventJSON
	}
	return nil
}

type BusinessEventKey struct {
	RecordedAt time.Time
	ID         id.BusinessEventID
}

type BusinessEventSubject struct {
	Principal id.Principal
	NoSubject bool
}

type BusinessEventCursor struct {
	OccurredAt time.Time
	ID         id.BusinessEventID
}

type BusinessEventQuery struct {
	Subject BusinessEventSubject
	Type    string
	Outcome BusinessEventOutcome
	Start   time.Time
	End     time.Time
	After   *BusinessEventCursor
	Limit   int
}
