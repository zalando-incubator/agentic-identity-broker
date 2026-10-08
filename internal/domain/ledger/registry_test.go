package ledger

import (
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

const (
	registryFixtureName   = "review-completed"
	registryFixtureReason = "A trusted review completes."
	registryEnvelopeURN   = "urn:agentic-identity-broker:events:v1:envelope"
)

func registryPublishedSchemas(t *testing.T) fs.FS {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	schemas := os.DirFS(filepath.Join(filepath.Dir(filename), "..", "..", "..", "api", "events", "v1"))
	_, err := fs.Stat(schemas, "envelope.schema.json")
	require.NoError(t, err)
	return schemas
}

func registryFixtureSchema(name string) map[string]any {
	return map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         "urn:agentic-identity-broker:events:v1:" + name,
		"title":       name,
		"description": registryFixtureReason,
		"allOf": []any{
			map[string]any{"$ref": registryEnvelopeURN},
			map[string]any{"properties": map[string]any{
				"type":    map[string]any{"const": model.BusinessEventTypePrefix + name},
				"outcome": map[string]any{"const": "success"},
				"data": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"review_stage": map[string]any{"type": "string"},
						"review": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]any{
								"decision": map[string]any{"enum": []string{"approved", "rejected"}},
							},
						},
					},
				},
			}},
		},
	}
}

func registryFixtureDataProperties(schema map[string]any) map[string]any {
	constraints := schema["allOf"].([]any)[1].(map[string]any)
	properties := constraints["properties"].(map[string]any)
	return properties["data"].(map[string]any)["properties"].(map[string]any)
}

func registryFixtureSource(t *testing.T, name string, schema map[string]any) fs.FS {
	t.Helper()
	contents, err := json.Marshal(schema)
	require.NoError(t, err)
	return fstest.MapFS{name + ".schema.json": &fstest.MapFile{Data: contents}}
}

func registryFixtureEvent(name string) *model.BusinessEvent {
	event := validLedgerEvent()
	event.Type = model.BusinessEventTypePrefix + name
	event.ReasonUser, event.ReasonAdmin = registryFixtureReason, registryFixtureReason
	event.Data = map[string]any{"review_stage": "complete", "review": map[string]any{"decision": "approved"}}
	return event
}

func TestRegistryComposesAdditionalOfflineTypeAndPreservesWireFields(t *testing.T) {
	base := registryPublishedSchemas(t)
	withoutAddition, err := NewRegistry(base)
	require.NoError(t, err)
	addition := registryFixtureSource(t, registryFixtureName, registryFixtureSchema(registryFixtureName))
	withAddition, err := NewRegistry(base, addition)
	require.NoError(t, err)

	event := registryFixtureEvent(registryFixtureName)
	_, err = withoutAddition.Validate(event)
	require.Error(t, err, "unknown types must not be accepted without registration")
	wire, err := withAddition.Validate(event)
	require.NoError(t, err)
	require.Equal(t, event.Type, wire["type"])
	require.Equal(t, registryFixtureReason, wire["reason_user"])
	require.Equal(t, map[string]any{
		"review_stage": "complete", "review": map[string]any{"decision": "approved"},
	}, wire["data"])
	earlier := registryFixtureEvent(registryFixtureName)
	earlier.Data = map[string]any{}
	earlierWire, err := withAddition.Validate(earlier)
	require.NoError(t, err, "new optional fields must not invalidate earlier records")
	require.Equal(t, map[string]any{}, earlierWire["data"])
	_, err = withAddition.Validate(validLedgerEvent())
	require.NoError(t, err, "the original catalogue remains available")

	for _, tc := range []struct {
		name   string
		mutate func(*model.BusinessEvent)
	}{
		{"unregistered top-level type", func(e *model.BusinessEvent) { e.Type += "-unregistered" }},
		{"unregistered data field", func(e *model.BusinessEvent) { e.Data["access_token"] = "credential-canary" }},
		{"unexpected nested field", func(e *model.BusinessEvent) {
			e.Data["review"] = map[string]any{"decision": "approved", "private_key": "credential-canary"}
		}},
		{"invalid selected enum", func(e *model.BusinessEvent) {
			e.Data["review"] = map[string]any{"decision": "ignored"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := registryFixtureEvent(registryFixtureName)
			tc.mutate(candidate)
			_, err := withAddition.Validate(candidate)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "credential-canary")
		})
	}
}

func TestRegistryAcceptsMultipleAdditiveOfflineSources(t *testing.T) {
	base := registryPublishedSchemas(t)
	first := registryFixtureSource(t, registryFixtureName, registryFixtureSchema(registryFixtureName))
	secondName := "audit-completed"
	second := registryFixtureSource(t, secondName, registryFixtureSchema(secondName))
	registry, err := NewRegistry(base, first, second)
	require.NoError(t, err)
	for _, name := range []string{registryFixtureName, secondName} {
		t.Run(name, func(t *testing.T) {
			wire, err := registry.Validate(registryFixtureEvent(name))
			require.NoError(t, err)
			require.Equal(t, model.BusinessEventTypePrefix+name, wire["type"])
		})
	}
	_, err = registry.Validate(validLedgerEvent())
	require.NoError(t, err)
}

func TestRegistryAcceptsDistinctFixedUserAndAdminReasonsFromOfflineSchema(t *testing.T) {
	const userReason = "The verified review is complete."
	const adminReason = "A verified review was committed."
	schema := registryFixtureSchema(registryFixtureName)
	properties := schema["allOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
	properties["reason_user"] = map[string]any{"const": userReason}
	properties["reason_admin"] = map[string]any{"const": adminReason}
	registry, err := NewRegistry(registryPublishedSchemas(t), registryFixtureSource(t, registryFixtureName, schema))
	require.NoError(t, err)

	event := registryFixtureEvent(registryFixtureName)
	event.ReasonUser, event.ReasonAdmin = userReason, adminReason
	wire, err := registry.Validate(event)
	require.NoError(t, err)
	require.Equal(t, userReason, wire["reason_user"])
	require.Equal(t, adminReason, wire["reason_admin"])

	const unsafe = "credential-canary"
	for _, tc := range []struct {
		name   string
		mutate func(*model.BusinessEvent)
	}{
		{"wrong user reason", func(e *model.BusinessEvent) { e.ReasonUser = adminReason }},
		{"interpolated admin reason", func(e *model.BusinessEvent) { e.ReasonAdmin += unsafe }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := *event
			tc.mutate(&candidate)
			_, err := registry.Validate(&candidate)
			require.Error(t, err)
			require.NotContains(t, err.Error(), unsafe)
		})
	}
}

func TestRegistryRejectsRedefinitionAndFixedNamespaceOrSourceChanges(t *testing.T) {
	base := registryPublishedSchemas(t)
	published, err := fs.ReadFile(base, "grant-created.schema.json")
	require.NoError(t, err)
	duplicatePublished := fstest.MapFS{"grant-created.schema.json": &fstest.MapFile{Data: published}}
	_, err = NewRegistry(base, duplicatePublished)
	require.Error(t, err, "even identical definitions cannot reassign an existing type")

	original := registryFixtureSource(t, registryFixtureName, registryFixtureSchema(registryFixtureName))
	_, err = NewRegistry(base, original, registryFixtureSource(t, registryFixtureName, registryFixtureSchema(registryFixtureName)))
	require.Error(t, err, "two added sources cannot redefine each other")

	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"functional name", func(schema map[string]any) {
			properties := schema["allOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
			properties["type"] = map[string]any{"const": "another-product.review-completed"}
		}},
		{"source", func(schema map[string]any) {
			properties := schema["allOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
			properties["source"] = map[string]any{"const": "urn:another-product:broker"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := registryFixtureSchema(registryFixtureName)
			tc.change(schema)
			_, err := NewRegistry(base, registryFixtureSource(t, registryFixtureName, schema))
			require.Error(t, err, "a schema incompatible with the fixed envelope must fail at registration")
		})
	}
}

func TestRegistryRejectsReservedAndCollidingFlattenedPropertyNames(t *testing.T) {
	base := registryPublishedSchemas(t)
	_, err := NewRegistry(base, registryFixtureSource(t, registryFixtureName, registryFixtureSchema(registryFixtureName)))
	require.NoError(t, err, "the same source without reserved names is valid")

	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"period in data property", func(schema map[string]any) {
			registryFixtureDataProperties(schema)["review.step"] = map[string]any{"type": "string"}
		}},
		{"reserved bookkeeping prefix", func(schema map[string]any) {
			registryFixtureDataProperties(schema)["_ledger_private"] = map[string]any{"type": "string"}
		}},
		{"reserved nested bookkeeping prefix", func(schema map[string]any) {
			review := registryFixtureDataProperties(schema)["review"].(map[string]any)
			review["properties"].(map[string]any)["_ledgerNull"] = map[string]any{"type": "string"}
		}},
		{"nested and literal paths flatten to the same key", func(schema map[string]any) {
			registryFixtureDataProperties(schema)["review.decision"] = map[string]any{"type": "string"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := registryFixtureSchema(registryFixtureName)
			tc.change(schema)
			_, err := NewRegistry(base, registryFixtureSource(t, registryFixtureName, schema))
			require.Error(t, err)
		})
	}
}

func TestRegistryRefusesRemoteSchemaReferenceBeforeOpeningConnection(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	schema := registryFixtureSchema(registryFixtureName)
	schema["allOf"].([]any)[0].(map[string]any)["$ref"] = server.URL + "/remote-envelope.schema.json"
	_, err := NewRegistry(registryPublishedSchemas(t), registryFixtureSource(t, registryFixtureName, schema))
	require.Error(t, err, "HTTPS references cannot be resolved from the network")
	require.Zero(t, connections.Load(), "offline registry must not contact even a local test HTTPS endpoint")
}

func TestRegistryChecksSemanticTimesAndNonzeroTypedIDs(t *testing.T) {
	registry, err := NewRegistry(registryPublishedSchemas(t))
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		make   func() *model.BusinessEvent
		mutate func(*model.BusinessEvent)
	}{
		{"UUIDv4 event ID", validLedgerEvent, func(e *model.BusinessEvent) {
			e.ID = id.MustParseBusinessEventID("11111111-1111-4111-8111-111111111111")
		}},
		{"non-UTC occurrence", validLedgerEvent, func(e *model.BusinessEvent) {
			e.OccurredAt = e.OccurredAt.In(time.FixedZone("offset", 3600))
		}},
		{"zero permission-set reference", validLedgerEvent, func(e *model.BusinessEvent) {
			e.PermissionSetIDs = []id.PermissionSetID{{}}
		}},
		{"zero required grant reference", validLedgerEvent, func(e *model.BusinessEvent) {
			e.GrantID = id.GrantID{}
		}},
		{"invalid calendar activation", func() *model.BusinessEvent {
			return registryPublishedExample(t, "signing-key-promoted")
		}, func(e *model.BusinessEvent) { e.Data["activates_at"] = "2026-02-30T12:00:00Z" }},
		{"zero signing-key reference", func() *model.BusinessEvent {
			return registryPublishedExample(t, "signing-key-promoted")
		}, func(e *model.BusinessEvent) { e.Data["signing_key_id"] = "00000000-0000-0000-0000-000000000000" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := tc.make()
			_, err := registry.Validate(event)
			require.NoError(t, err, "the original event must be accepted before mutation")
			tc.mutate(event)
			_, err = registry.Validate(event)
			require.Error(t, err)
		})
	}
}

func registryPublishedExample(t *testing.T, name string) *model.BusinessEvent {
	t.Helper()
	contents, err := fs.ReadFile(registryPublishedSchemas(t), "examples.json")
	require.NoError(t, err)
	var examples []model.BusinessEvent
	require.NoError(t, json.Unmarshal(contents, &examples))
	for i := range examples {
		if examples[i].Type == model.BusinessEventTypePrefix+name {
			return &examples[i]
		}
	}
	t.Fatalf("missing published example for %s", name)
	return nil
}

func TestRegistryPreservesNumericFixtureValues(t *testing.T) {
	schema := registryFixtureSchema(registryFixtureName)
	properties := registryFixtureDataProperties(schema)
	properties["scalar"] = map[string]any{"type": "number"}
	properties["values"] = map[string]any{"type": "array", "items": map[string]any{"type": "number"}}
	registry, err := NewRegistry(registryPublishedSchemas(t), registryFixtureSource(t, registryFixtureName, schema))
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		scalar any
		values any
		json   string
	}{
		{"large integer", int64(9007199254740993), []int64{9007199254740993, -9007199254740993}, `{"scalar":9007199254740993,"values":[9007199254740993,-9007199254740993]}`},
		{"retained integer", json.Number("9007199254740993"), []any{json.Number("9007199254740993"), json.Number("-9007199254740993")}, `{"scalar":9007199254740993,"values":[9007199254740993,-9007199254740993]}`},
		{"int32", int32(7), []int32{7, -7}, `{"scalar":7,"values":[7,-7]}`},
		{"float32", float32(0.1), []float32{0.1, -0.1}, `{"scalar":0.1,"values":[0.1,-0.1]}`},
		{"retained decimals", json.Number("0.1"), []any{json.Number("0.1"), json.Number("-0.1")}, `{"scalar":0.1,"values":[0.1,-0.1]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := registryFixtureEvent(registryFixtureName)
			event.Data = map[string]any{"scalar": tc.scalar, "values": tc.values}
			wire, err := registry.Validate(event)
			require.NoError(t, err)
			data, err := json.Marshal(wire["data"])
			require.NoError(t, err)
			require.Equal(t, tc.json, string(data))
		})
	}
}

func TestRegistryRejectsNumericValuesThatCannotReachTelemetryLosslessly(t *testing.T) {
	schema := registryFixtureSchema(registryFixtureName)
	properties := registryFixtureDataProperties(schema)
	properties["scalar"] = map[string]any{"type": "number"}
	properties["values"] = map[string]any{"type": "array", "items": map[string]any{"type": "number"}}
	registry, err := NewRegistry(registryPublishedSchemas(t), registryFixtureSource(t, registryFixtureName, schema))
	require.NoError(t, err)
	for _, data := range []map[string]any{
		{"scalar": uint64(18446744073709551615)},
		{"values": []uint64{18446744073709551615}},
		{"scalar": json.Number("9007199254740993.5")},
		{"values": []any{int64(9007199254740993), 0.5}},
		{"scalar": json.Number("01")},
	} {
		event := registryFixtureEvent(registryFixtureName)
		event.Data = data
		_, err := registry.Validate(event)
		require.Error(t, err, "unrepresentable payloads must not create undeliverable retained events")
	}
}
