package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/jmoiron/sqlx"
)

func QueryLedgerRawEnvelope(ctx context.Context, db *sqlx.DB, eventID id.BusinessEventID) (map[string]any, error) {
	if db == nil || eventID.IsZero() {
		return nil, errors.New("raw ledger lookup requires a database and event ID")
	}
	var encoded []byte
	if err := db.GetContext(ctx, &encoded, "SELECT envelope FROM public.business_events WHERE id=$1", eventID); err != nil {
		return nil, ledgerSQLError("read raw ledger envelope", err)
	}
	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, errors.New("invalid persisted envelope JSON")
	}
	if err := validateLedgerRawEnvelope(envelope); err != nil {
		return nil, err
	}
	return envelope, nil
}

func validateLedgerRawEnvelope(envelope map[string]any) error {
	allowed := map[string]bool{}
	for _, field := range []string{"id", "type", "source", "occurred_at", "recorded_at", "subject", "actor", "outcome", "reason_user", "reason_admin", "data", "agent_id", "gateway_client_id", "service_id", "permission_set_ids", "grant_id", "session_id", "approval_id", "mcp_session_id", "agent_session_id", "trace_id", "span_id", "client"} {
		allowed[field] = true
	}
	for field := range envelope {
		if !allowed[field] {
			return errors.New("unregistered persisted envelope field")
		}
	}
	for _, field := range []string{"id", "type", "source", "occurred_at", "recorded_at", "subject", "actor", "outcome", "reason_user", "reason_admin", "data"} {
		if _, exists := envelope[field]; !exists {
			return errors.New("missing required persisted envelope field")
		}
	}
	actor, ok := envelope["actor"].(map[string]any)
	if !ok {
		return errors.New("persisted actor is not an object")
	}
	for _, field := range []string{"kind", "id", "on_behalf_of"} {
		if _, exists := actor[field]; !exists {
			return errors.New("missing required nullable actor field")
		}
	}
	if len(actor) != 3 {
		return errors.New("unregistered persisted actor field")
	}
	for _, field := range []string{"agent_id", "gateway_client_id", "service_id", "permission_set_ids", "grant_id", "session_id", "approval_id", "mcp_session_id", "agent_session_id", "trace_id", "span_id", "client"} {
		if value, exists := envelope[field]; exists && value == nil {
			return errors.New("unavailable optional field must be absent, not null")
		}
	}
	return nil
}

func NormalizeLedgerRawEnvelope(envelope map[string]any) (map[string]any, error) {
	if err := validateLedgerRawEnvelope(envelope); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	normalizeLedgerWireValues(normalized)
	return normalized, nil
}

func normalizeLedgerWireValues(envelope map[string]any) {
	aliases := make(map[string]string)
	for _, field := range []string{"id", "agent_id", "service_id", "grant_id", "session_id", "approval_id", "trace_id", "span_id"} {
		if value, ok := envelope[field].(string); ok && value != "" {
			aliases[value] = "<" + field + ">"
			envelope[field] = aliases[value]
		}
	}
	for _, field := range []string{"occurred_at", "recorded_at"} {
		envelope[field] = "<" + field + ">"
	}
	if actor, ok := envelope["actor"].(map[string]any); ok {
		if value, ok := actor["id"].(string); ok {
			if alias, exists := aliases[value]; exists {
				actor["id"] = alias
			}
		}
	}
	if data, ok := envelope["data"].(map[string]any); ok {
		for _, field := range []string{"credential_id", "signing_key_id", "activates_at"} {
			if _, exists := data[field]; exists {
				data[field] = "<" + field + ">"
			}
		}
	}
}
