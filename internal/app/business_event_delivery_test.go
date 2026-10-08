package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	storagefactory "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func deliveryTestReceiver(t *testing.T) *helpers.OTLPReceiver {
	t.Helper()
	receiver, err := helpers.NewOTLPReceiver()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, receiver.Close()) })
	return receiver
}

func deliveryTestBuilder(store *storagefactory.Adapter, receiver *helpers.OTLPReceiver, copyEnabled bool) *Builder {
	builder := businessEventSchemaBuilder(store)
	builder.config.BusinessEvents = ports.BusinessEventsConfig{
		Retention: ports.DefaultBusinessEventsConfig().Retention, TelemetryCopyEnabled: copyEnabled,
	}
	builder.config.Telemetry = ports.DefaultTelemetryConfig()
	builder.config.Telemetry.Enabled = true
	builder.config.Telemetry.Traces.Enabled = false
	builder.config.Telemetry.Metrics.Enabled = false
	builder.config.Telemetry.Logs.Enabled = true
	builder.config.Telemetry.Exporter.Protocol = ports.OTLPProtocolGRPC
	builder.config.Telemetry.Exporter.Endpoint = receiver.Endpoint()
	builder.config.Telemetry.Exporter.Insecure = true
	builder.config.Telemetry.Exporter.Timeout = time.Second
	// Keep the existing slog/tracing test override separate from the real ledger provider.
	return builder.WithTracerProvider(sdktrace.NewTracerProvider())
}

func deliveryTestRecord(t *testing.T, service *ledger.Service, subject id.Principal) model.BusinessEventKey {
	t.Helper()
	caller := subject.String()
	event, err := service.NewEvent(t.Context(), model.BusinessEventTypePrefix+"grant-created", model.BusinessEvent{
		OccurredAt: time.Now().UTC(), Subject: &subject,
		Actor:   model.BusinessEventActor{Kind: "user", ID: &caller},
		AgentID: id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		GrantID: id.NewGrantID(), Data: map[string]any{},
	})
	require.NoError(t, err)
	require.NoError(t, service.Record(t.Context(), event))
	return model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
}

func deliveryTestSeed(t *testing.T, store *storagefactory.Adapter) model.BusinessEventKey {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	service := ledger.NewService(registry, store.BusinessEvents(), store.BusinessEventLifecycle(), store, true)
	return deliveryTestRecord(t, service, id.NewPrincipal("pre-existing-delivery"))
}

func deliveryTestPending(t *testing.T, delivery ports.BusinessEventDeliveryRepository) []model.BusinessEventKey {
	t.Helper()
	keys, err := delivery.ListDue(t.Context(), 100)
	require.NoError(t, err)
	return keys
}

func deliveryTestCount(receiver *helpers.OTLPReceiver, key model.BusinessEventKey) int {
	count := 0
	for _, exported := range receiver.LedgerRecords() {
		for _, attr := range exported.Record.GetAttributes() {
			if attr.GetKey() == "id" && attr.GetValue().GetStringValue() == key.ID.String() {
				count++
			}
		}
	}
	return count
}

func deliveryTestWaitForCopy(t *testing.T, receiver *helpers.OTLPReceiver, key model.BusinessEventKey, copies int) {
	t.Helper()
	require.Eventually(t, func() bool { return deliveryTestCount(receiver, key) >= copies },
		35*time.Second, 10*time.Millisecond, "retained event %s must reach the real OTLP receiver", key.ID)
}

func deliveryTestWaitForAttempt(t *testing.T, receiver *helpers.OTLPReceiver, minimum int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, receiver.WaitForLedgerAttempts(ctx, minimum))
}

func deliveryTestStartWorker(t *testing.T, repository ports.BusinessEventDeliveryRepository, config *ports.Config) chan<- time.Time {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 1)
	done := make(chan error, 1)
	go func() {
		done <- runBusinessEventDelivery(ctx, repository, config, ticks, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Error("delivery worker did not stop after cancellation")
		}
	})
	return ticks
}

func TestBuilderBusinessEventDeliveryScansAtStartupAndAfterCommit(t *testing.T) {
	store := businessEventSchemaStorage(t)
	receiver := deliveryTestReceiver(t)
	queued := deliveryTestSeed(t, store)

	application, err := deliveryTestBuilder(store, receiver, true).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })
	require.Eventually(t, func() bool { return deliveryTestCount(receiver, queued) == 1 },
		3*time.Second, 10*time.Millisecond, "startup scan must export an already-committed reference")
	require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
		5*time.Second, 10*time.Millisecond, "startup scan must acknowledge the stored reference")
	require.Equal(t, model.BusinessEventTypePrefix+"grant-created", receiver.LedgerRecords()[0].Record.GetEventName())

	later := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("recorded-after-startup"))
	require.Eventually(t, func() bool { return deliveryTestCount(receiver, later) == 1 },
		3*time.Second, 10*time.Millisecond, "a one-second scan must deliver a newly committed fact")
	require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
		5*time.Second, 10*time.Millisecond, "the one-second scan must acknowledge newly committed work")
	for _, key := range []model.BusinessEventKey{queued, later} {
		stored, getErr := store.BusinessEvents().Get(t.Context(), key)
		require.NoError(t, getErr)
		require.Equal(t, key.ID, stored.ID, "successful telemetry must not remove the ledger fact")
		require.Equal(t, 1, deliveryTestCount(receiver, key))
	}
}

func TestBusinessEventDeliveryProcessesAtMostOneHundredCandidatesPerScan(t *testing.T) {
	store := businessEventSchemaStorage(t)
	receiver := deliveryTestReceiver(t)
	keys := make([]model.BusinessEventKey, 101)
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	service := ledger.NewService(registry, store.BusinessEvents(), store.BusinessEventLifecycle(), store, true)
	for i := range keys {
		keys[i] = deliveryTestRecord(t, service, id.NewPrincipal("bounded-scan"))
	}

	config := deliveryTestBuilder(store, receiver, true).config
	ticks := deliveryTestStartWorker(t, store.BusinessEventDelivery(), config)
	require.Eventually(t, func() bool {
		pending, listErr := store.BusinessEventDelivery().ListDue(t.Context(), 100)
		return listErr == nil && len(receiver.LedgerRecords()) == 100 && len(pending) == 1
	}, 15*time.Second, 10*time.Millisecond, "a startup scan must export only 100 real refs, leaving one pending")
	remaining := deliveryTestPending(t, store.BusinessEventDelivery())
	require.Len(t, remaining, 1)
	require.True(t, slices.Contains(keys, remaining[0]))
	for _, key := range keys {
		require.LessOrEqual(t, deliveryTestCount(receiver, key), 1, "no candidate is exported twice in one scan")
	}

	ticks <- time.Now()
	deliveryTestWaitForCopy(t, receiver, remaining[0], 1)
	require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
		5*time.Second, 10*time.Millisecond)
	require.Len(t, receiver.LedgerRecords(), 101)
}

// The first storage read is intentionally delayed until the context expires. The
// next scan uses the underlying memory repository and must deliver the real ref.
type deadlineDeliveryRepository struct {
	ports.BusinessEventDeliveryRepository
	first   atomic.Bool
	entered chan time.Duration
}

func (r *deadlineDeliveryRepository) ListDue(ctx context.Context, limit int) ([]model.BusinessEventKey, error) {
	if !r.first.Swap(true) {
		deadline, ok := ctx.Deadline()
		if !ok {
			r.entered <- time.Hour
			return nil, errors.New("storage scan has no deadline")
		}
		r.entered <- time.Until(deadline)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return r.BusinessEventDeliveryRepository.ListDue(ctx, limit)
}

type deadlineDeliveryStorage struct {
	ports.StorageProvider
	delivery ports.BusinessEventDeliveryRepository
}

func (s *deadlineDeliveryStorage) BusinessEventDelivery() ports.BusinessEventDeliveryRepository {
	return s.delivery
}

func TestBuilderBusinessEventDeliveryBoundsStorageAndExporterDeadlines(t *testing.T) {
	t.Run("timed-out storage read resumes on a later scan", func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		receiver := deliveryTestReceiver(t)
		key := deliveryTestSeed(t, store)
		blocked := &deadlineDeliveryRepository{
			BusinessEventDeliveryRepository: store.BusinessEventDelivery(), entered: make(chan time.Duration, 1),
		}
		builder := deliveryTestBuilder(store, receiver, true).WithStorage(&deadlineDeliveryStorage{
			StorageProvider: store, delivery: blocked,
		})
		builder.config.Storage.Timeouts.Read = 80 * time.Millisecond
		application, err := builder.Build()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })
		select {
		case duration := <-blocked.entered:
			require.Greater(t, duration, time.Duration(0))
			require.LessOrEqual(t, duration, 150*time.Millisecond, "each ListDue call needs a bounded storage read deadline")
		case <-time.After(5 * time.Second):
			t.Fatal("startup scan did not attempt to read the pending reference")
		}
		deliveryTestWaitForCopy(t, receiver, key, 1)
		require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
			5*time.Second, 10*time.Millisecond)
	})

	t.Run("blocked collector cannot hold the deletion barrier indefinitely", func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		receiver := deliveryTestReceiver(t)
		release := receiver.PauseLedger()
		defer release()
		builder := deliveryTestBuilder(store, receiver, true)
		builder.config.Telemetry.Exporter.Timeout = 250 * time.Millisecond
		builder.config.Storage.Timeouts.Write = 700 * time.Millisecond
		application, err := builder.Build()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })
		key := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("deadline-subject"))
		deliveryTestWaitForAttempt(t, receiver, 1)
		// After the exporter deadline the real memory erasure must acquire the
		// lifecycle barrier despite the collector never returning success.
		time.Sleep(900 * time.Millisecond)
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()
		removed, err := store.BusinessEventLifecycle().EraseSubject(ctx, id.NewPrincipal("deadline-subject"))
		require.NoError(t, err)
		require.EqualValues(t, 1, removed)
		_, err = store.BusinessEvents().Get(t.Context(), key)
		require.ErrorIs(t, err, ports.ErrNotFound)
		release()
		time.Sleep(150 * time.Millisecond)
		require.Empty(t, receiver.LedgerRecords(), "timed-out export must never acknowledge a deleted fact")
	})
}

func TestBuilderBusinessEventDeliveryRecoversAfterProviderInitializationFailure(t *testing.T) {
	store := businessEventSchemaStorage(t)
	receiver := deliveryTestReceiver(t)
	queued := deliveryTestSeed(t, store)

	previous := newBusinessEventDeliveryProvider
	var attempts atomic.Int32
	newBusinessEventDeliveryProvider = func(ctx context.Context, cfg ports.TelemetryConfig) (*telemetry.BusinessEventProvider, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("temporary ledger exporter initialization failure")
		}
		return telemetry.NewBusinessEventProvider(ctx, cfg)
	}
	t.Cleanup(func() { newBusinessEventDeliveryProvider = previous })

	application, err := deliveryTestBuilder(store, receiver, true).Build()
	require.NoError(t, err, "the business ledger must remain available when its copy provider fails to start")
	t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })
	require.Eventually(t, func() bool { return attempts.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
	require.Contains(t, deliveryTestPending(t, store.BusinessEventDelivery()), queued)
	recordedDuringFailure := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("during-init-failure"))

	require.Eventually(t, func() bool {
		return deliveryTestCount(receiver, queued) == 1 && deliveryTestCount(receiver, recordedDuringFailure) == 1
	}, 4*time.Second, 10*time.Millisecond, "worker must retry initialization automatically rather than waiting for a restart")
	require.GreaterOrEqual(t, attempts.Load(), int32(2))
	require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
		5*time.Second, 10*time.Millisecond)
}

func TestBuilderBusinessEventCommitDoesNotWaitForCollector(t *testing.T) {
	store := businessEventSchemaStorage(t)
	receiver := deliveryTestReceiver(t)
	release := receiver.PauseLedger()
	defer release()
	application, err := deliveryTestBuilder(store, receiver, true).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })

	first := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("collector-blocked-first"))
	deliveryTestWaitForAttempt(t, receiver, 1)
	committed := make(chan model.BusinessEventKey, 1)
	go func() {
		// Unlike the collector, this business transaction has a short budget.
		caller := "collector-blocked-second"
		subject := id.NewPrincipal(caller)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		event, newErr := application.LedgerService.NewEvent(ctx, model.BusinessEventTypePrefix+"grant-created", model.BusinessEvent{
			OccurredAt: time.Now().UTC(), Subject: &subject,
			Actor:   model.BusinessEventActor{Kind: "user", ID: &caller},
			AgentID: id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
			GrantID: id.NewGrantID(), Data: map[string]any{},
		})
		if newErr != nil {
			committed <- model.BusinessEventKey{}
			return
		}
		if application.LedgerService.Record(ctx, event) != nil {
			committed <- model.BusinessEventKey{}
			return
		}
		committed <- model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
	}()
	var second model.BusinessEventKey
	select {
	case second = <-committed:
		require.False(t, second.ID.IsZero(), "business commit must succeed before the paused collector finishes")
	case <-time.After(time.Second):
		t.Fatal("business commit waited for ledger telemetry export")
	}
	_, err = store.BusinessEvents().Get(t.Context(), second)
	require.NoError(t, err)
	require.Empty(t, receiver.LedgerRecords(), "the receiver is still paused while both facts are durable")
	release()
	deliveryTestWaitForCopy(t, receiver, first, 1)
	deliveryTestWaitForCopy(t, receiver, second, 1)
}

func TestBuilderBusinessEventDeliveryRetriesExportAndPostExportAckWithSameID(t *testing.T) {
	t.Run("collector failure leaves the reference for a later attempt", func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		receiver := deliveryTestReceiver(t)
		receiver.SetUnavailable(true)
		builder := deliveryTestBuilder(store, receiver, true)
		builder.config.Telemetry.Exporter.Timeout = 150 * time.Millisecond
		application, err := builder.Build()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })
		key := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("receiver-outage"))
		deliveryTestWaitForAttempt(t, receiver, 1)
		time.Sleep(350 * time.Millisecond) // Let bounded SDK retries fail before restoring the receiver.
		require.Empty(t, receiver.LedgerRecords())
		receiver.SetUnavailable(false)
		deliveryTestWaitForCopy(t, receiver, key, 1)
		require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
			5*time.Second, 10*time.Millisecond)
		require.Equal(t, 1, deliveryTestCount(receiver, key))
	})

	t.Run("successful export followed by failed acknowledgement may duplicate the same ID", func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		receiver := deliveryTestReceiver(t)
		builder := deliveryTestBuilder(store, receiver, true)
		builder.config.Storage.Timeouts.Write = time.Second
		builder.config.Telemetry.Exporter.Timeout = 2 * time.Second
		application, err := builder.Build()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, application.Shutdown(context.Background())) })
		require.Eventually(t, func() bool {
			for _, exported := range receiver.LedgerRecords() {
				if exported.Record.GetEventName() == "agentic-identity-broker.signing-key-promoted" {
					return true
				}
			}
			return false
		}, 5*time.Second, 10*time.Millisecond, "bootstrap export must finish before target acknowledgement contention")
		attemptsBefore := receiver.LedgerAttempts()
		release := receiver.PauseLedger()
		defer release()
		key := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("ack-contention"))
		deliveryTestWaitForAttempt(t, receiver, attemptsBefore+1)

		// Memory releases its business visibility gate while exporting but
		// needs it again to acknowledge. A concurrent business transaction
		// holds the gate until the dispatch deadline expires after export.
		tx, err := store.BeginTX(t.Context())
		require.NoError(t, err)
		rolledBack := false
		defer func() {
			if !rolledBack {
				require.NoError(t, store.Rollback(tx))
			}
		}()
		release()
		deliveryTestWaitForCopy(t, receiver, key, 1)
		time.Sleep(1200 * time.Millisecond)
		require.NoError(t, store.Rollback(tx))
		rolledBack = true
		deliveryTestWaitForCopy(t, receiver, key, 2)
		require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
			5*time.Second, 10*time.Millisecond)
		require.Equal(t, 2, deliveryTestCount(receiver, key), "a retry must retain the original event identity")
	})
}

func TestBuilderBusinessEventDeliveryCopySwitchPausesWithoutBackfill(t *testing.T) {
	for _, tc := range []struct {
		name    string
		disable func(*ports.Config)
	}{
		{name: "ledger copy disabled", disable: func(cfg *ports.Config) { cfg.BusinessEvents.TelemetryCopyEnabled = false }},
		{name: "OTLP logs disabled", disable: func(cfg *ports.Config) { cfg.Telemetry.Logs.Enabled = false }},
		{name: "telemetry disabled", disable: func(cfg *ports.Config) { cfg.Telemetry.Enabled = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := businessEventSchemaStorage(t)
			receiver := deliveryTestReceiver(t)
			// Simulate a broker restart after commit but before delivery.
			queued := deliveryTestSeed(t, store)
			disabledBuilder := deliveryTestBuilder(store, receiver, true)
			tc.disable(disabledBuilder.config)
			disabled, err := disabledBuilder.Build()
			require.NoError(t, err)
			t.Cleanup(func() {
				if disabled != nil {
					require.NoError(t, disabled.Shutdown(context.Background()))
				}
			})
			disabledPeriod := deliveryTestRecord(t, disabled.LedgerService, id.NewPrincipal("disabled-period"))
			time.Sleep(1200 * time.Millisecond)
			require.Zero(t, receiver.LedgerAttempts(), "an existing pending ref must pause while any effective-copy switch is off")
			require.Equal(t, []model.BusinessEventKey{queued}, deliveryTestPending(t, store.BusinessEventDelivery()),
				"new disabled-period facts must not get a delivery reference")
			require.NoError(t, disabled.Shutdown(context.Background()))
			disabled = nil

			enabled, err := deliveryTestBuilder(store, receiver, true).Build()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, enabled.Shutdown(context.Background())) })
			deliveryTestWaitForCopy(t, receiver, queued, 1)
			require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
				5*time.Second, 10*time.Millisecond)
			time.Sleep(1200 * time.Millisecond) // A later scan must not discover the disabled-period fact.
			require.Zero(t, deliveryTestCount(receiver, disabledPeriod), "copy resume must not backfill disabled-period events")
			require.Len(t, receiver.LedgerRecords(), 1)
			stored, getErr := store.BusinessEvents().Get(t.Context(), disabledPeriod)
			require.NoError(t, getErr)
			require.Equal(t, disabledPeriod.ID, stored.ID, "the switch controls copies, not ledger writes")
		})
	}
}

func TestBuilderBusinessEventDeliveryShutdownCancelsExportAndPreservesUnfinishedReference(t *testing.T) {
	store := businessEventSchemaStorage(t)
	receiver := deliveryTestReceiver(t)
	release := receiver.PauseLedger()
	defer release()
	builder := deliveryTestBuilder(store, receiver, true)
	builder.config.Telemetry.Exporter.Timeout = 2 * time.Second
	// This case uses the real existing telemetry provider as well, so Shutdown
	// must stop ledger delivery before closing both provider pipelines.
	builder.WithTracerProvider(nil)
	application, err := builder.Build()
	require.NoError(t, err)
	var stopping chan error
	t.Cleanup(func() {
		if stopping != nil {
			release()
			select {
			case err := <-stopping:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Error("delivery shutdown did not finish after receiver release")
			}
		} else if application != nil {
			require.NoError(t, application.Shutdown(context.Background()))
		}
	})
	key := deliveryTestRecord(t, application.LedgerService, id.NewPrincipal("shutdown-pending"))
	deliveryTestWaitForAttempt(t, receiver, 1)

	stopping = make(chan error, 1)
	go func(active *App, done chan<- error) { done <- active.Shutdown(context.Background()) }(application, stopping)
	select {
	case err := <-stopping:
		stopping = nil
		application = nil
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("shutdown must cancel the active export before closing its provider")
	}
	// The stopped worker must not leave an SDK payload ready to export after shutdown.
	require.Empty(t, receiver.LedgerRecords(), "canceled work must not be acknowledged as delivered")
	stored, err := store.BusinessEvents().Get(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, key.ID, stored.ID)
	release()

	// The process can be restarted with the same store: only the still-pending
	// reference can produce this copy, and the canceled worker cannot export it.
	recovered, err := deliveryTestBuilder(store, receiver, true).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, recovered.Shutdown(context.Background())) })
	deliveryTestWaitForCopy(t, receiver, key, 1)
	require.Eventually(t, func() bool { return len(deliveryTestPending(t, store.BusinessEventDelivery())) == 0 },
		5*time.Second, 10*time.Millisecond)
	require.Equal(t, 1, deliveryTestCount(receiver, key))
}
