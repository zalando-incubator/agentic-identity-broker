package helpers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLedgerRawEnvelopePreservesAdjacentLargeIntegers(t *testing.T) {
	envelope := map[string]any{
		"id": "event", "type": "event-type", "source": "broker", "occurred_at": "now", "recorded_at": "now",
		"subject": nil, "actor": map[string]any{"kind": "system", "id": nil, "on_behalf_of": nil},
		"outcome": "success", "reason_user": "reason", "reason_admin": "reason",
		"data": map[string]any{"count": json.Number("9007199254740992")},
	}
	first, err := NormalizeLedgerRawEnvelope(envelope)
	require.NoError(t, err)
	envelope["data"].(map[string]any)["count"] = json.Number("9007199254740993")
	second, err := NormalizeLedgerRawEnvelope(envelope)
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740992"), first["data"].(map[string]any)["count"])
	require.Equal(t, json.Number("9007199254740993"), second["data"].(map[string]any)["count"])
	require.NotEqual(t, first, second)
}
