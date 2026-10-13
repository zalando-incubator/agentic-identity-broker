package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func unitLedgerEvent(t *testing.T) (*ledger.Registry, *model.BusinessEvent) {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	const canary = "business-event-sensitive-principal-canary"
	subject := id.NewPrincipal(canary)
	actorID := subject.String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := &model.BusinessEvent{
		ID: id.NewBusinessEventID(), Type: model.BusinessEventTypePrefix + "token-issued",
		Source: model.BusinessEventSource, OccurredAt: now, RecordedAt: now,
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "user", ID: &actorID},
		Outcome:     model.BusinessEventSuccess,
		ReasonUser:  "The broker completes a non-exchange token issuance.",
		ReasonAdmin: "The broker completes a non-exchange token issuance.", Data: map[string]any{},
	}
	_, err = registry.Validate(event)
	require.NoError(t, err, "unit fixture must satisfy the actual published schema")
	return registry, event
}

func assertSafeBusinessEventStorageError(t *testing.T, err error, kind storage.ErrorKind) {
	t.Helper()
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	require.Equal(t, kind, storageErr.Kind)
	require.NotContains(t, err.Error(), "business-event-sensitive-principal-canary")
	if storageErr.Cause != nil {
		require.NotContains(t, storageErr.Cause.Error(), "business-event-sensitive-principal-canary")
	}
}

func TestBusinessEventRepository_NilDatabaseMapsStorageErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		adapter *Adapter
	}{{name: "nil adapter"}, {name: "nil database", adapter: &Adapter{}}} {
		t.Run(tt.name, func(t *testing.T) {
			registry, event := unitLedgerEvent(t)
			repo := NewBusinessEventRepository(tt.adapter, registry)
			query := model.BusinessEventQuery{
				Subject: model.BusinessEventSubject{Principal: *event.Subject},
				Start:   event.OccurredAt.Add(-time.Minute), End: event.OccurredAt.Add(time.Minute),
			}
			t.Run("append", func(t *testing.T) {
				assertSafeBusinessEventStorageError(t, repo.Append(context.Background(), event, true), storage.ErrorKindConnection)
			})
			t.Run("query", func(t *testing.T) {
				_, err := repo.Query(context.Background(), query)
				assertSafeBusinessEventStorageError(t, err, storage.ErrorKindConnection)
			})
			t.Run("get", func(t *testing.T) {
				_, err := repo.Get(context.Background(), model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID})
				assertSafeBusinessEventStorageError(t, err, storage.ErrorKindConnection)
			})
		})
	}
}

func TestBusinessEventRepository_DatabaseDeadlineMapsTimeoutWithoutEventValues(t *testing.T) {
	registry, event := unitLedgerEvent(t)
	adapter := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
		beginErr: context.DeadlineExceeded,
		execErr:  context.DeadlineExceeded,
		queryErr: context.DeadlineExceeded,
	}).adapter
	repo := NewBusinessEventRepository(adapter, registry)
	query := model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: *event.Subject},
		Start:   event.OccurredAt.Add(-time.Minute), End: event.OccurredAt.Add(time.Minute),
	}
	t.Run("append", func(t *testing.T) {
		assertSafeBusinessEventStorageError(t, repo.Append(context.Background(), event, false), storage.ErrorKindTimeout)
	})
	t.Run("query", func(t *testing.T) {
		_, err := repo.Query(context.Background(), query)
		assertSafeBusinessEventStorageError(t, err, storage.ErrorKindTimeout)
	})
	t.Run("get", func(t *testing.T) {
		_, err := repo.Get(context.Background(), model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID})
		assertSafeBusinessEventStorageError(t, err, storage.ErrorKindTimeout)
	})
}

func TestBusinessEventRepository_DecodePreservesExactRegisteredIntegers(t *testing.T) {
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
	_, event := unitLedgerEvent(t)
	event.Type = model.BusinessEventTypePrefix + "numeric-retention"
	event.ReasonUser, event.ReasonAdmin = "A numeric event is retained.", "A numeric event is retained."
	const large = int64(9007199254740993)
	event.Data = map[string]any{"scalar": large, "values": []int64{7, large}}
	_, err = registry.Validate(event)
	require.NoError(t, err)
	envelope, err := json.Marshal(event)
	require.NoError(t, err)

	repo := NewBusinessEventRepository(nil, registry)
	decoded, err := repo.decodeEvent(envelope)
	require.NoError(t, err, "retained numeric JSON must remain valid under its registered schema")
	require.Equal(t, event.ID, decoded.ID)
	_, err = registry.Validate(decoded)
	require.NoError(t, err)
	payload, err := json.Marshal(decoded.Data)
	require.NoError(t, err)
	var numbers struct {
		Scalar int64   `json:"scalar"`
		Values []int64 `json:"values"`
	}
	require.NoError(t, json.NewDecoder(bytes.NewReader(payload)).Decode(&numbers))
	require.Equal(t, large, numbers.Scalar)
	require.Equal(t, []int64{7, large}, numbers.Values)
}
