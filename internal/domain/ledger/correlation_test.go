package ledger

import (
	"encoding/json"
	"testing"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestRawSessionCorrelationsDoNotEnterValidatedWire(t *testing.T) {
	registry, err := NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	for _, field := range []string{"mcp_session_id", "agent_session_id"} {
		t.Run(field, func(t *testing.T) {
			event := validLedgerEvent()
			const credential = "raw-session-credential-canary"
			if field == "mcp_session_id" {
				event.MCPSessionID = credential
			} else {
				event.AgentSessionID = credential
			}
			wire, err := registry.Validate(event)
			require.NoError(t, err)
			require.NotContains(t, wire, field, "raw strings have no non-credential provenance")
			encoded, err := json.Marshal(wire)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), credential)
			encoded, err = json.Marshal(validLedgerEvent())
			require.NoError(t, err)
			var raw map[string]any
			require.NoError(t, json.Unmarshal(encoded, &raw))
			raw[field] = credential
			encoded, err = json.Marshal(raw)
			require.NoError(t, err)
			var decoded model.BusinessEvent
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			wire, err = registry.Validate(&decoded)
			require.NoError(t, err)
			require.NotContains(t, wire, field, "JSON decoding cannot attest provenance")
		})
	}
}

func TestTrustedSessionCorrelationsPreserveOpaqueValuesAndLoseTrustAfterMutation(t *testing.T) {
	registry, err := NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	event := validLedgerEvent()
	event.SetTrustedSessionCorrelations("mcp/opaque-session:reviewed", "agent opaque session 42")
	wire, err := registry.Validate(event)
	require.NoError(t, err)
	require.Equal(t, "mcp/opaque-session:reviewed", wire["mcp_session_id"])
	require.Equal(t, "agent opaque session 42", wire["agent_session_id"])
	event.MCPSessionID = "unattested credential replacement"
	wire, err = registry.Validate(event)
	require.NoError(t, err)
	require.NotContains(t, wire, "mcp_session_id")
	require.Equal(t, "agent opaque session 42", wire["agent_session_id"])
}
