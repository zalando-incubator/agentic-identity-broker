package ledger

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validLedgerEvent() *model.BusinessEvent {
	principal := id.NewPrincipal("ledger-subject")
	caller := principal.String()
	instant := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	return &model.BusinessEvent{
		ID:   id.MustParseBusinessEventID("01997100-0000-7000-8000-000000000001"),
		Type: "agentic-identity-broker.grant-created", Source: "urn:agentic-identity-broker:broker",
		OccurredAt: instant, RecordedAt: instant.Add(time.Second), Subject: &principal,
		Actor:       model.BusinessEventActor{Kind: "user", ID: &caller},
		AgentID:     id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		GrantID:     id.MustParseGrantID("22222222-2222-4222-8222-222222222222"),
		Outcome:     model.BusinessEventSuccess,
		ReasonUser:  "A new user-to-agent delegation is committed.",
		ReasonAdmin: "A new user-to-agent delegation is committed.", Data: map[string]any{},
	}
}

func TestEventRejectsInvalidEnvelope(t *testing.T) {
	registry, err := NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	service := NewService(registry, nil, nil, nil, false)
	require.NoError(t, service.Validate(validLedgerEvent()))
	const sensitive = "credential-bearing-input-canary"
	cases := []struct {
		name   string
		mutate func(*model.BusinessEvent)
	}{
		{"unknown type", func(e *model.BusinessEvent) { e.Type = "agentic-identity-broker.unknown" }},
		{"configurable namespace", func(e *model.BusinessEvent) { e.Type = "deployment-name.grant-created" }},
		{"changed source", func(e *model.BusinessEvent) { e.Source = "https://untrusted.example/" + sensitive }},
		{"zero event ID", func(e *model.BusinessEvent) { e.ID = id.BusinessEventID{} }},
		{"UUIDv4 event ID", func(e *model.BusinessEvent) {
			e.ID = id.MustParseBusinessEventID("11111111-1111-4111-8111-111111111111")
		}},
		{"wrong outcome", func(e *model.BusinessEvent) { e.Outcome = model.BusinessEventDenied }},
		{"missing grant", func(e *model.BusinessEvent) { e.GrantID = id.GrantID{} }},
		{"missing agent", func(e *model.BusinessEvent) { e.AgentID = id.AgentID{} }},
		{"missing affected principal", func(e *model.BusinessEvent) { e.Subject = nil }},
		{"empty affected principal", func(e *model.BusinessEvent) { p := id.NewPrincipal(""); e.Subject = &p }},
		{"unknown actor category", func(e *model.BusinessEvent) { e.Actor.Kind = sensitive }},
		{"empty known actor", func(e *model.BusinessEvent) { e.Actor.ID = new(string) }},
		{"empty delegation", func(e *model.BusinessEvent) { p := id.NewPrincipal(""); e.Actor.OnBehalfOf = &p }},
		{"zero occurrence", func(e *model.BusinessEvent) { e.OccurredAt = time.Time{} }},
		{"non-UTC occurrence", func(e *model.BusinessEvent) { e.OccurredAt = e.OccurredAt.In(time.FixedZone("offset", 3600)) }},
		{"zero recording time", func(e *model.BusinessEvent) { e.RecordedAt = time.Time{} }},
		{"non-UTC recording time", func(e *model.BusinessEvent) { e.RecordedAt = e.RecordedAt.In(time.FixedZone("offset", -3600)) }},
		{"uncontrolled user reason", func(e *model.BusinessEvent) { e.ReasonUser = sensitive }},
		{"raw admin error", func(e *model.BusinessEvent) { e.ReasonAdmin = "upstream failed: " + sensitive }},
		{"null data", func(e *model.BusinessEvent) { e.Data = nil }},
		{"additional data", func(e *model.BusinessEvent) { e.Data = map[string]any{"unexpected": sensitive} }},
		{"nested additional data", func(e *model.BusinessEvent) {
			e.Data = map[string]any{"nested": map[string]any{"access_token": sensitive}}
		}},
		{"zero permission reference", func(e *model.BusinessEvent) { e.PermissionSetIDs = []id.PermissionSetID{{}} }},
		{"invalid IP", func(e *model.BusinessEvent) { e.Client = &model.BusinessEventClient{IP: "999.1.1.1"} }},
		{"IP with credentials", func(e *model.BusinessEvent) { e.Client = &model.BusinessEventClient{IP: sensitive} }},
		{"raw user agent", func(e *model.BusinessEvent) {
			e.Client = &model.BusinessEventClient{UserAgent: "curl/8.17 " + sensitive}
		}},
		{"short trace", func(e *model.BusinessEvent) { e.TraceID = "abcdef" }},
		{"nonhex trace", func(e *model.BusinessEvent) { e.TraceID = strings.Repeat("g", 32) }},
		{"zero trace", func(e *model.BusinessEvent) { e.TraceID = strings.Repeat("0", 32) }},
		{"span without trace", func(e *model.BusinessEvent) { e.SpanID = "7a3ce929d0e0e473" }},
		{"zero span", func(e *model.BusinessEvent) {
			e.TraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
			e.SpanID = strings.Repeat("0", 16)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := validLedgerEvent()
			tc.mutate(event)
			err := service.Validate(event)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), sensitive)
		})
	}
	t.Run("nil event", func(t *testing.T) { require.Error(t, service.Validate(nil)) })
}

func TestEventWireKeepsNullIdentitiesAndOmitsUnavailableReferences(t *testing.T) {
	event := validLedgerEvent()
	event.Subject, event.Actor.ID, event.Actor.OnBehalfOf = nil, nil, nil
	event.AgentID, event.GrantID = id.AgentID{}, id.GrantID{}
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	value, exists := wire["subject"]
	require.True(t, exists)
	require.Nil(t, value)
	actor := wire["actor"].(map[string]any)
	for _, field := range []string{"id", "on_behalf_of"} {
		value, exists := actor[field]
		require.True(t, exists)
		require.Nil(t, value)
	}
	for _, field := range []string{"agent_id", "gateway_client_id", "service_id", "grant_id", "session_id", "approval_id", "trace_id", "span_id", "client"} {
		require.NotContains(t, wire, field)
	}
}

func TestEventPermissionSetsSerializeAsAnIndependentSortedSet(t *testing.T) {
	first := id.MustParsePermissionSetID("11111111-1111-4111-8111-111111111111")
	second := id.MustParsePermissionSetID("22222222-2222-4222-8222-222222222222")
	event := validLedgerEvent()
	event.PermissionSetIDs = []id.PermissionSetID{second, first, second}
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Equal(t, []any{first.String(), second.String()}, wire["permission_set_ids"])
	require.Equal(t, []id.PermissionSetID{second, first, second}, event.PermissionSetIDs, "serialization must not mutate its caller")
}

func TestEventDecodeRejectsUnregisteredEnvelopeFields(t *testing.T) {
	encoded, err := json.Marshal(validLedgerEvent())
	require.NoError(t, err)
	for _, extra := range []string{`"unexpected":"credential-canary",`, `"actor":{"kind":"user","id":null,"on_behalf_of":null,"unexpected":"credential-canary"},`} {
		t.Run(extra, func(t *testing.T) {
			var decoded model.BusinessEvent
			require.Error(t, json.Unmarshal(append([]byte("{"+extra), encoded[1:]...), &decoded), "unknown fields must not silently disappear")
		})
	}
}
