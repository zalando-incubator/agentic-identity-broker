package helpers

import (
	"encoding/json"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// NormalizeLedgerEnvelope preserves all fields and null/absent distinctions,
// replacing only generated identities and instants that differ between journeys.
func NormalizeLedgerEnvelope(event *model.BusinessEvent) (map[string]any, error) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return nil, err
	}
	normalizeLedgerWireValues(envelope)
	return envelope, nil
}
