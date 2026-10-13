package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/require"
)

const (
	businessEventSchemaFixtureName   = "review-completed"
	businessEventSchemaFixtureType   = model.BusinessEventTypePrefix + businessEventSchemaFixtureName
	businessEventSchemaFixtureReason = "A reviewed fixture action was completed."
)

func businessEventSchemaStorage(t *testing.T) *storage.Adapter {
	t.Helper()
	adapter, err := storage.NewAdapter(&ports.StorageConfig{
		Backend:  "memory",
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close(context.Background())) })
	return adapter
}

func businessEventSchemaBuilder(adapter *storage.Adapter, sources ...fs.FS) *Builder {
	config := &ports.Config{
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
		},
		Storage: ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		},
		Encryption: ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{
			Mode:  "local",
			Local: ports.LocalModeConfig{TokenTTL: time.Hour},
		},
	}
	builder := NewBuilder().WithConfig(config).WithStorage(adapter).WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, source := range sources {
		builder.WithBusinessEventSchemas(source)
	}
	return builder
}

func businessEventSchemaApp(t *testing.T, adapter *storage.Adapter, sources ...fs.FS) *App {
	t.Helper()
	app, err := businessEventSchemaBuilder(adapter, sources...).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, app.Shutdown(context.Background())) })
	return app
}

func businessEventSchemaFixture() map[string]any {
	return map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         "urn:agentic-identity-broker:events:v1:" + businessEventSchemaFixtureName,
		"title":       businessEventSchemaFixtureName,
		"description": businessEventSchemaFixtureReason,
		"allOf": []any{
			map[string]any{"$ref": "urn:agentic-identity-broker:events:v1:envelope"},
			map[string]any{"properties": map[string]any{
				"type":    map[string]any{"const": businessEventSchemaFixtureType},
				"outcome": map[string]any{"const": "success"},
				"data": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"marker"},
					"properties": map[string]any{
						"marker": map[string]any{"type": "string", "minLength": 1},
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

func businessEventSchemaSource(t *testing.T, schema map[string]any) fs.FS {
	t.Helper()
	contents, err := json.Marshal(schema)
	require.NoError(t, err)
	return fstest.MapFS{businessEventSchemaFixtureName + ".schema.json": {Data: contents}}
}

func businessEventSchemaSubject() id.Principal {
	return id.NewPrincipal("schema-fixture-subject")
}

func businessEventSchemaOccurrence() time.Time {
	return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
}

func businessEventSchemaFixtureEvent() *model.BusinessEvent {
	subject := businessEventSchemaSubject()
	caller := subject.String()
	return &model.BusinessEvent{
		ID:         id.NewBusinessEventID(),
		Type:       businessEventSchemaFixtureType,
		Source:     model.BusinessEventSource,
		OccurredAt: businessEventSchemaOccurrence().Add(time.Minute),
		Subject:    &subject,
		Actor:      model.BusinessEventActor{Kind: "user", ID: &caller},
		Outcome:    model.BusinessEventSuccess,
		ReasonUser: businessEventSchemaFixtureReason, ReasonAdmin: businessEventSchemaFixtureReason,
		Data: map[string]any{
			"marker": "reviewed-fixture",
			"review": map[string]any{"decision": "approved"},
		},
	}
}

func businessEventSchemaGrant(t *testing.T, app *App) *model.BusinessEvent {
	t.Helper()
	subject := businessEventSchemaSubject()
	caller := subject.String()
	grant, err := app.LedgerService.NewEvent(context.Background(), model.BusinessEventTypePrefix+"grant-created", model.BusinessEvent{
		OccurredAt: businessEventSchemaOccurrence(), Subject: &subject,
		Actor:   model.BusinessEventActor{Kind: "user", ID: &caller},
		AgentID: id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		GrantID: id.MustParseGrantID("22222222-2222-4222-8222-222222222222"),
		Data:    map[string]any{},
	})
	require.NoError(t, err)
	return grant
}

func businessEventSchemaQuery(t *testing.T, app *App, eventType string) []*model.BusinessEvent {
	t.Helper()
	events, err := app.LedgerService.Query(context.Background(), model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: businessEventSchemaSubject()},
		Type:    eventType,
		Start:   businessEventSchemaOccurrence().Add(-time.Hour),
		End:     businessEventSchemaOccurrence().Add(time.Hour),
	})
	require.NoError(t, err)
	return events
}

func TestBuilderBusinessEventSchemasAddsOfflineTypeWithoutReinterpretingExistingRecords(t *testing.T) {
	adapter := businessEventSchemaStorage(t)
	original := businessEventSchemaApp(t, adapter)
	grant := businessEventSchemaGrant(t, original)
	require.NoError(t, original.LedgerService.Record(context.Background(), grant))
	before := businessEventSchemaQuery(t, original, "")
	require.Len(t, before, 1)
	require.Equal(t, "agentic-identity-broker.grant-created", before[0].Type)
	require.Equal(t, "A new user-to-agent delegation is committed.", before[0].ReasonUser)
	require.Equal(t, before[0].ReasonUser, before[0].ReasonAdmin)
	require.Equal(t, map[string]any{}, before[0].Data)

	fixture := businessEventSchemaFixtureEvent()
	require.Error(t, original.LedgerService.Record(context.Background(), fixture), "default builders must not admit fixture types")
	require.True(t, fixture.RecordedAt.IsZero(), "an unknown type must not reach storage")
	require.Equal(t, before, businessEventSchemaQuery(t, original, ""))

	// Reuse the same real memory adapter: registering a type must not require a storage migration.
	extended := businessEventSchemaApp(t, adapter, businessEventSchemaSource(t, businessEventSchemaFixture()))
	require.Equal(t, before, businessEventSchemaQuery(t, extended, ""))
	require.NoError(t, extended.LedgerService.Record(context.Background(), fixture))
	all := businessEventSchemaQuery(t, extended, "")
	require.Len(t, all, 2)
	require.Equal(t, before[0], all[0], "the original fact must retain its identity, type, references and meaning")
	require.Equal(t, fixture.ID, all[1].ID)
	require.Equal(t, businessEventSchemaFixtureType, all[1].Type)
	require.Equal(t, businessEventSchemaFixtureReason, all[1].ReasonUser)
	require.Equal(t, fixture.Data, all[1].Data)
	require.NotZero(t, all[1].RecordedAt)
	fixtureOnly := businessEventSchemaQuery(t, extended, businessEventSchemaFixtureType)
	require.Equal(t, []*model.BusinessEvent{all[1]}, fixtureOnly)
	retained, err := extended.LedgerService.Get(context.Background(), model.BusinessEventKey{
		ID: fixture.ID, RecordedAt: all[1].RecordedAt,
	})
	require.NoError(t, err)
	require.Equal(t, all[1], retained)
}

func TestBuilderBusinessEventSchemasRejectsUnsafeSourcesAtStartup(t *testing.T) {
	adapter := businessEventSchemaStorage(t)
	published, err := fs.ReadFile(eventschemas.Schemas, "grant-created.schema.json")
	require.NoError(t, err)

	var requests atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(remote.Close)

	for _, tc := range []struct {
		name   string
		source func(*testing.T) fs.FS
	}{
		{"existing type even when identical", func(*testing.T) fs.FS {
			return fstest.MapFS{"grant-created.schema.json": {Data: published}}
		}},
		{"different functional name", func(t *testing.T) fs.FS {
			schema := businessEventSchemaFixture()
			properties := schema["allOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
			properties["type"] = map[string]any{"const": "another-broker.review-completed"}
			return businessEventSchemaSource(t, schema)
		}},
		{"different source", func(t *testing.T) fs.FS {
			schema := businessEventSchemaFixture()
			properties := schema["allOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
			properties["source"] = map[string]any{"const": "urn:another-broker:broker"}
			return businessEventSchemaSource(t, schema)
		}},
		{"remote reference", func(t *testing.T) fs.FS {
			schema := businessEventSchemaFixture()
			schema["allOf"].([]any)[0].(map[string]any)["$ref"] = remote.URL + "/envelope.schema.json"
			return businessEventSchemaSource(t, schema)
		}},
		{"nested and literal names collide when flattened", func(t *testing.T) fs.FS {
			schema := businessEventSchemaFixture()
			properties := schema["allOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
			dataProperties := properties["data"].(map[string]any)["properties"].(map[string]any)
			dataProperties["review.decision"] = map[string]any{"type": "string"}
			return businessEventSchemaSource(t, schema)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, err := businessEventSchemaBuilder(adapter, tc.source(t)).Build()
			require.ErrorContains(t, err, "business event registry initialization failed")
			require.Nil(t, app)
		})
	}
	require.Zero(t, requests.Load(), "startup must not resolve schema references over the network")
}

func TestBuilderBusinessEventSchemasRejectsUnknownAndInvalidEventsBeforeStorage(t *testing.T) {
	adapter := businessEventSchemaStorage(t)
	app := businessEventSchemaApp(t, adapter, businessEventSchemaSource(t, businessEventSchemaFixture()))
	valid := businessEventSchemaFixtureEvent()
	prepared := *valid
	prepared.RecordedAt = prepared.OccurredAt
	require.NoError(t, app.LedgerService.Validate(&prepared), "the fixture must be registered before testing its closed payload")
	require.NoError(t, app.LedgerService.Record(context.Background(), businessEventSchemaGrant(t, app)))
	before := businessEventSchemaQuery(t, app, "")
	require.Len(t, before, 1, "the real memory ledger must accept a published fact")

	for _, tc := range []struct {
		name   string
		mutate func(*model.BusinessEvent)
	}{
		{"unknown type", func(event *model.BusinessEvent) {
			event.Type = model.BusinessEventTypePrefix + "unregistered-review"
		}},
		{"missing required marker", func(event *model.BusinessEvent) {
			event.Data = map[string]any{"review": map[string]any{"decision": "approved"}}
		}},
		{"unregistered credential field", func(event *model.BusinessEvent) {
			event.Data = map[string]any{"marker": "reviewed-fixture", "access_token": "credential-canary"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := businessEventSchemaFixtureEvent()
			tc.mutate(candidate)
			err := app.LedgerService.Record(context.Background(), candidate)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "credential-canary")
			require.True(t, candidate.RecordedAt.IsZero(), "invalid events must be rejected before storage prepares them")
			require.Equal(t, before, businessEventSchemaQuery(t, app, ""), "rejected facts must leave the real ledger unchanged")
		})
	}
}
