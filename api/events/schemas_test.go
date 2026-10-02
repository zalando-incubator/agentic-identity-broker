package events_test

import (
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func publishedExamples(t *testing.T) ([]map[string]any, []model.BusinessEvent) {
	t.Helper()
	contents, err := os.ReadFile("v1/examples.json")
	require.NoError(t, err)
	var published []map[string]any
	require.NoError(t, json.Unmarshal(contents, &published))
	var events []model.BusinessEvent
	require.NoError(t, json.Unmarshal(contents, &events))
	require.Len(t, events, len(published))
	return published, events
}

func TestPublishedBusinessEventExamplesValidateOffline(t *testing.T) {
	registry, err := ledger.NewRegistry(os.DirFS("v1"))
	require.NoError(t, err)
	published, events := publishedExamples(t)
	require.Len(t, events, 28)
	seen := make(map[string]bool, len(events))
	for i := range events {
		event := &events[i]
		t.Run(strings.TrimPrefix(event.Type, model.BusinessEventTypePrefix), func(t *testing.T) {
			require.False(t, seen[event.Type], "duplicate published event type")
			seen[event.Type] = true
			wire, err := registry.Validate(event)
			require.NoError(t, err)
			require.Equal(t, published[i], wire, "the validated view must retain every published value")
		})
	}
}

func TestPublishedSchemaRejectsTypeOutcomeReferenceAndNestedPayloadMismatch(t *testing.T) {
	registry, err := ledger.NewRegistry(os.DirFS("v1"))
	require.NoError(t, err)
	_, examples := publishedExamples(t)
	byType := make(map[string]model.BusinessEvent, len(examples))
	for _, example := range examples {
		byType[example.Type] = example
	}
	const secret = "credential-secret-not-an-event-field"
	cases := []struct {
		name   string
		typeID string
		mutate func(*model.BusinessEvent)
	}{
		{"unknown type", "agentic-identity-broker.grant-created", func(e *model.BusinessEvent) {
			e.Type = "agentic-identity-broker.undocumented-grant"
		}},
		{"wrong outcome", "agentic-identity-broker.approval-denied", func(e *model.BusinessEvent) {
			e.Outcome = model.BusinessEventSuccess
		}},
		{"missing required reference", "agentic-identity-broker.token-exchanged", func(e *model.BusinessEvent) {
			e.AgentID = id.AgentID{}
		}},
		{"nested extra data", "agentic-identity-broker.approval-denied", func(e *model.BusinessEvent) {
			e.Data["claims"] = map[string]any{"access_token": secret}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, exists := byType[tc.typeID]
			require.True(t, exists)
			event.Data = maps.Clone(event.Data)
			_, err := registry.Validate(&event)
			require.NoError(t, err, "the published event must be accepted before mutation")
			tc.mutate(&event)
			_, err = registry.Validate(&event)
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
		})
	}
}

func TestPublishedEnvelopeDecoderRejectsUnregisteredFieldsWithoutDroppingThem(t *testing.T) {
	published, _ := publishedExamples(t)
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"envelope field", func(e map[string]any) { e["authorization_header"] = "Bearer private-canary" }},
		{"nested actor field", func(e map[string]any) {
			e["actor"].(map[string]any)["private_key"] = "private-canary"
		}},
		{"nested client field", func(e map[string]any) {
			e["client"] = map[string]any{"ip": "127.0.0.1", "access_token": "private-canary"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := make(map[string]any, len(published[0]))
			for key, value := range published[0] {
				candidate[key] = value
			}
			actor := make(map[string]any)
			for key, value := range published[0]["actor"].(map[string]any) {
				actor[key] = value
			}
			candidate["actor"] = actor
			tc.mutate(candidate)
			bytes, err := json.Marshal(candidate)
			require.NoError(t, err)
			var decoded model.BusinessEvent
			require.Error(t, json.Unmarshal(bytes, &decoded))
		})
	}
}
