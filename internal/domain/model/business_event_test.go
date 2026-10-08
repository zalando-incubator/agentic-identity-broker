package model_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/fstest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventEnvelopePreservesExactNumericPayload(t *testing.T) {
	const schema = `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$id":"urn:agentic-identity-broker:events:v1:numeric-retention",
		"description":"A numeric event is retained.",
		"allOf":[
			{"$ref":"urn:agentic-identity-broker:events:v1:envelope"},
			{"properties":{
				"type":{"const":"agentic-identity-broker.numeric-retention"},
				"outcome":{"const":"success"},
				"data":{"type":"object","additionalProperties":false,
					"required":["scalar","values"],
					"properties":{
						"scalar":{"type":"integer"},
						"values":{"type":"array","items":{"type":"integer"}}
					}}
			}}
		]
	}`
	registry, err := ledger.NewRegistry(eventschemas.Schemas, fstest.MapFS{
		"numeric-retention.schema.json": &fstest.MapFile{Data: []byte(schema)},
	})
	require.NoError(t, err)

	const large = int64(9007199254740993)
	subject := id.NewPrincipal("numeric-retention-subject")
	actorID := subject.String()
	instant := time.Now().UTC().Truncate(time.Microsecond)
	event := &model.BusinessEvent{
		ID: id.NewBusinessEventID(), Type: model.BusinessEventTypePrefix + "numeric-retention",
		Source: model.BusinessEventSource, OccurredAt: instant, RecordedAt: instant,
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "user", ID: &actorID},
		Outcome: model.BusinessEventSuccess, ReasonUser: "A numeric event is retained.",
		ReasonAdmin: "A numeric event is retained.",
		Data:        map[string]any{"scalar": large, "values": []int64{7, large}},
	}
	_, err = registry.Validate(event)
	require.NoError(t, err, "the closed fixture schema must accept both integer shapes")
	encoded, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded model.BusinessEvent
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, event.ID, decoded.ID)
	requireExactNumericPayload(t, decoded.Data)
	_, err = registry.Validate(&decoded)
	require.NoError(t, err, "the decoded envelope must still satisfy its registered schema")
	decoded.Data["unreviewed"] = "not in the schema"
	_, err = registry.Validate(&decoded)
	require.Error(t, err, "numeric retention must not open the payload schema")
}

func requireExactNumericPayload(t *testing.T, data map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	var decoded struct {
		Scalar int64   `json:"scalar"`
		Values []int64 `json:"values"`
	}
	require.NoError(t, json.NewDecoder(bytes.NewReader(encoded)).Decode(&decoded))
	require.Equal(t, int64(9007199254740993), decoded.Scalar)
	require.Equal(t, []int64{7, 9007199254740993}, decoded.Values)
}
