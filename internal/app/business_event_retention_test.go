package app

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	storagefactory "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func retentionRecord(t *testing.T, store *storagefactory.Adapter, subject id.Principal) model.BusinessEventKey {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	service := ledger.NewService(registry, store.BusinessEvents(), store.BusinessEventLifecycle(), store, false)
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

func retentionWindowEnd(recordedAt time.Time) time.Time {
	return recordedAt.Truncate(6 * time.Hour).Add(6 * time.Hour)
}

func requireRetentionEvent(t *testing.T, store *storagefactory.Adapter, key model.BusinessEventKey, retained bool) {
	t.Helper()
	_, err := store.BusinessEvents().Get(t.Context(), key)
	if retained {
		require.NoError(t, err, "a record on the retained side of the six-hour boundary must remain")
	} else {
		require.ErrorIs(t, err, ports.ErrNotFound, "eligible history must actually be deleted")
	}
}

func retentionBuilder(store *storagefactory.Adapter, retention time.Duration, telemetry, logs, copyEnabled bool) *Builder {
	builder := businessEventSchemaBuilder(store)
	builder.config.BusinessEvents = ports.BusinessEventsConfig{Retention: retention, TelemetryCopyEnabled: copyEnabled}
	builder.config.Telemetry.Enabled = telemetry
	builder.config.Telemetry.Logs.Enabled = logs
	return builder.WithTracerProvider(sdktrace.NewTracerProvider())
}

func TestBuilderRetentionSweepsAtStartupUsingConfiguredPolicyRegardlessOfTelemetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		retention   time.Duration
		telemetry   bool
		logs        bool
		copyEnabled bool
	}{
		{name: "default policy with telemetry off", retention: ports.DefaultBusinessEventsConfig().Retention},
		{name: "changed policy with telemetry off", retention: 13 * time.Hour, copyEnabled: true},
		{name: "changed policy with logs off", retention: 13 * time.Hour, telemetry: true, copyEnabled: true},
		{name: "changed policy with ledger copies off", retention: 13 * time.Hour, telemetry: true, logs: true},
		{name: "changed policy with all telemetry enabled", retention: 13 * time.Hour, telemetry: true, logs: true, copyEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := businessEventSchemaStorage(t)
				old := retentionRecord(t, store, id.NewPrincipal("expired-at-startup"))
				time.Sleep(time.Until(retentionWindowEnd(old.RecordedAt).Add(tc.retention).Add(time.Second)))
				fresh := retentionRecord(t, store, id.NewPrincipal("fresh-at-startup"))

				application, err := retentionBuilder(store, tc.retention, tc.telemetry, tc.logs, tc.copyEnabled).Build()
				require.NoError(t, err)
				defer func() { require.NoError(t, application.Shutdown(context.Background())) }()
				synctest.Wait()
				requireRetentionEvent(t, store, old, false)
				requireRetentionEvent(t, store, fresh, true)
			})
		})
	}
}

func TestBuilderRetentionPolicyWaitsForActiveLifecycleTransaction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		const retention = 13 * time.Hour
		old := retentionRecord(t, store, id.NewPrincipal("startup-barrier-subject"))
		time.Sleep(time.Until(retentionWindowEnd(old.RecordedAt).Add(retention).Add(time.Second)))

		tx, err := store.BeginTX(t.Context())
		require.NoError(t, err)
		result := make(chan struct {
			application *App
			err         error
		}, 1)
		go func() {
			application, err := retentionBuilder(store, retention, false, false, false).Build()
			result <- struct {
				application *App
				err         error
			}{application, err}
		}()
		synctest.Wait()
		completedBeforeCommit := false
		var earlyApplication *App
		var earlyError error
		select {
		case early := <-result:
			completedBeforeCommit = true
			earlyApplication, earlyError = early.application, early.err
		default:
		}
		require.NoError(t, store.Commit(tx))
		if completedBeforeCommit {
			if earlyApplication != nil {
				require.NoError(t, earlyApplication.Shutdown(context.Background()))
			}
			t.Fatalf("startup policy and first sweep returned before the lifecycle predecessor committed: %v", earlyError)
		}
		built := <-result
		require.NoError(t, built.err)
		synctest.Wait()
		defer func() { require.NoError(t, built.application.Shutdown(context.Background())) }()
		requireRetentionEvent(t, store, old, false)
	})
}

func TestBuilderRetentionUsesMicrosecondCeilingAndFiveMinuteSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		started := time.Now()
		application, err := retentionBuilder(store, time.Nanosecond, false, false, false).Build()
		require.NoError(t, err)
		defer func() { require.NoError(t, application.Shutdown(context.Background())) }()
		synctest.Wait()

		key := retentionRecord(t, store, id.NewPrincipal("boundary-subject"))
		windowEnd := retentionWindowEnd(key.RecordedAt)
		time.Sleep(time.Until(windowEnd))
		synctest.Wait()
		requireRetentionEvent(t, store, key, true)

		time.Sleep(time.Microsecond)
		synctest.Wait()
		requireRetentionEvent(t, store, key, true)

		const cadence = 5 * time.Minute
		eligible := windowEnd.Add(time.Microsecond)
		elapsed := eligible.Sub(started)
		nextTick := started.Add(((elapsed + cadence - 1) / cadence) * cadence)
		time.Sleep(time.Until(nextTick.Add(-time.Nanosecond)))
		synctest.Wait()
		requireRetentionEvent(t, store, key, true)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		requireRetentionEvent(t, store, key, false)
	})
}

func TestMemoryRetentionWorkerHandlesStartupTicksAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		lifecycle := store.BusinessEventLifecycle()
		const retention = time.Hour
		require.NoError(t, lifecycle.SetRetentionPolicy(t.Context(), retention))
		old := retentionRecord(t, store, id.NewPrincipal("worker-startup"))
		time.Sleep(time.Until(retentionWindowEnd(old.RecordedAt).Add(retention).Add(time.Second)))
		fresh := retentionRecord(t, store, id.NewPrincipal("worker-fresh"))

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ticks := make(chan time.Time, 1)
		done := make(chan error, 1)
		go func() { done <- runMemoryBusinessEventRetention(ctx, lifecycle, ticks, make(chan error, 1)) }()
		synctest.Wait()
		requireRetentionEvent(t, store, old, false)
		requireRetentionEvent(t, store, fresh, true)

		time.Sleep(time.Until(retentionWindowEnd(fresh.RecordedAt).Add(retention).Add(time.Second)))
		requireRetentionEvent(t, store, fresh, true)
		ticks <- time.Now()
		synctest.Wait()
		requireRetentionEvent(t, store, fresh, false)

		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			require.NoError(t, err)
		default:
			t.Fatal("worker did not stop after cancellation")
		}

		afterShutdown := retentionRecord(t, store, id.NewPrincipal("after-worker-stopped"))
		time.Sleep(time.Until(retentionWindowEnd(afterShutdown.RecordedAt).Add(retention).Add(time.Second)))
		ticks <- time.Now()
		synctest.Wait()
		requireRetentionEvent(t, store, afterShutdown, true)
	})
}

func TestBuilderShutdownStopsRetentionMaintenance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		application, err := retentionBuilder(store, time.Hour, false, false, false).Build()
		require.NoError(t, err)
		key := retentionRecord(t, store, id.NewPrincipal("after-builder-shutdown"))
		require.NoError(t, application.Shutdown(context.Background()))
		time.Sleep(time.Until(retentionWindowEnd(key.RecordedAt).Add(time.Hour).Add(10 * time.Minute)))
		synctest.Wait()
		requireRetentionEvent(t, store, key, true)
		require.NoError(t, store.BusinessEventLifecycle().ApplyRetention(t.Context()))
		requireRetentionEvent(t, store, key, false)
	})
}

func TestBuilderShutdownCancelsSweepWaitingOnLifecycleBarrier(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		started := time.Now()
		application, err := retentionBuilder(store, time.Nanosecond, false, false, false).Build()
		require.NoError(t, err)
		defer func() { require.NoError(t, application.Shutdown(context.Background())) }()
		key := retentionRecord(t, store, id.NewPrincipal("blocked-sweep"))
		eligible := retentionWindowEnd(key.RecordedAt).Add(time.Microsecond)
		time.Sleep(time.Until(eligible))
		synctest.Wait()

		tx, err := store.BeginTX(t.Context())
		require.NoError(t, err)
		defer func() { _ = store.Rollback(tx) }()
		const cadence = 5 * time.Minute
		nextTick := started.Add(((eligible.Sub(started) + cadence - 1) / cadence) * cadence)
		time.Sleep(time.Until(nextTick))
		synctest.Wait()
		shutdown := make(chan error, 1)
		go func() { shutdown <- application.Shutdown(context.Background()) }()
		synctest.Wait()
		stoppedWhileLocked := false
		var shutdownErr error
		select {
		case shutdownErr = <-shutdown:
			stoppedWhileLocked = true
		default:
		}
		require.NoError(t, store.Rollback(tx))
		if !stoppedWhileLocked {
			shutdownErr = <-shutdown
		}
		require.NoError(t, shutdownErr)
		require.True(t, stoppedWhileLocked, "shutdown must cancel a blocked sweep instead of waiting for an unrelated transaction")
		requireRetentionEvent(t, store, key, true)
		require.NoError(t, store.BusinessEventLifecycle().ApplyRetention(t.Context()))
		requireRetentionEvent(t, store, key, false)
	})
}

func TestBuilderRetentionPolicyRoundsUpBeforeMemoryMaintenance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := businessEventSchemaStorage(t)
		application, err := retentionBuilder(store, 1001*time.Nanosecond, false, false, false).Build()
		require.NoError(t, err)
		defer func() { require.NoError(t, application.Shutdown(context.Background())) }()
		key := retentionRecord(t, store, id.NewPrincipal("fractional-microsecond-policy"))
		windowEnd := retentionWindowEnd(key.RecordedAt)

		time.Sleep(time.Until(windowEnd.Add(time.Microsecond)))
		synctest.Wait()
		require.NoError(t, store.BusinessEventLifecycle().ApplyRetention(t.Context()))
		requireRetentionEvent(t, store, key, true)

		time.Sleep(time.Microsecond)
		require.NoError(t, store.BusinessEventLifecycle().ApplyRetention(t.Context()))
		requireRetentionEvent(t, store, key, false)
	})
}
