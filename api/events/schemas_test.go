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

func TestPublishedExchangeActorContracts(t *testing.T) {
	registry, err := ledger.NewRegistry(os.DirFS("v1"))
	require.NoError(t, err)
	_, examples := publishedExamples(t)
	for _, original := range examples {
		name := strings.TrimPrefix(original.Type, model.BusinessEventTypePrefix)
		switch name {
		case "token-exchanged", "impersonation-granted", "token-exchange-denied", "impersonation-denied":
		default:
			continue
		}
		t.Run(name, func(t *testing.T) {
			event := original
			event.Actor = model.BusinessEventActor{Kind: "gateway"}
			if name == "token-exchanged" || name == "impersonation-granted" {
				client := "verified-initiating-client"
				event.Actor.ID = &client
				event.Actor.OnBehalfOf = event.Subject
			}
			_, err := registry.Validate(&event)
			require.NoError(t, err)

			wrongKind := event
			wrongKind.Actor.Kind = "agent"
			_, err = registry.Validate(&wrongKind)
			require.Error(t, err, "receiving agents cannot be attributed as initiating callers")

			if name == "token-exchanged" || name == "impersonation-granted" {
				noVerifiedClient := event
				noVerifiedClient.Actor.ID = nil
				_, err = registry.Validate(&noVerifiedClient)
				require.Error(t, err, "a completed exchange requires a verified initiating client")
				noDelegation := event
				noDelegation.Actor.OnBehalfOf = nil
				_, err = registry.Validate(&noDelegation)
				require.Error(t, err, "a completed exchange requires established delegation")
			} else {
				event.Subject = nil
				_, err = registry.Validate(&event)
				require.NoError(t, err, "a denial before validation must preserve null identities")
			}
		})
	}
}

func TestPublishedApprovalConsumptionActorContract(t *testing.T) {
	registry, err := ledger.NewRegistry(os.DirFS("v1"))
	require.NoError(t, err)
	_, examples := publishedExamples(t)
	for _, event := range examples {
		if event.Type != model.BusinessEventTypePrefix+"approval-consumed" {
			continue
		}
		_, err := registry.Validate(&event)
		require.NoError(t, err)
		for _, tc := range []struct {
			name   string
			mutate func(*model.BusinessEvent)
		}{
			{"represented user is not consuming gateway", func(e *model.BusinessEvent) { e.Actor.Kind = "user" }},
			{"creator client cannot become consuming caller", func(e *model.BusinessEvent) {
				creator := "creation-time-gateway-client"
				e.Actor.ID = &creator
			}},
			{"consumption represents the approved owner", func(e *model.BusinessEvent) { e.Actor.OnBehalfOf = nil }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				candidate := event
				tc.mutate(&candidate)
				_, err := registry.Validate(&candidate)
				require.Error(t, err)
			})
		}
		return
	}
	t.Fatal("approval-consumed example missing from published catalogue")
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
