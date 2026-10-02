package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type preflightBarrierCoordinator struct {
	base    ports.AuthorizationSessionCoordinator
	entered chan struct{}
	proceed chan struct{}
}

func (c preflightBarrierCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	c.entered <- struct{}{}
	<-c.proceed
	return c.base.Run(ctx, agentID, operation)
}

func TestRefreshAudit_OverlappingUnusedPreflightCountsStoredResult(t *testing.T) {
	f := newProviderRetryFixture(t)
	var logs bytes.Buffer
	f.provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	entered, proceed := make(chan struct{}, 2), make(chan struct{})
	coordinator := preflightBarrierCoordinator{base: f.provider.refreshDeps.Coordinator, entered: entered, proceed: proceed}
	f.provider.refreshDeps.Coordinator = coordinator
	f.provider.fositeStorage.refresh.Coordinator = coordinator
	type result struct {
		value *ports.TokenResponse
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			value, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read")
			results <- result{value, err}
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("both requests must preflight the unused predecessor")
		}
	}
	close(proceed)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	requireOriginalRetryResult(t, first.value, second.value)
	events := map[string]int{}
	decoder := json.NewDecoder(&logs)
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if event, ok := record["event"].(string); ok {
			events[event]++
		}
	}
	require.Equal(t, 1, events["RefreshRotated"], "one request commits fresh rotation")
	require.Equal(t, 1, events["RefreshRetryAccepted"], "the overlapping request commits a stored-result authorization")
}

func TestSessionCleanup_HeldAgentGateCannotKeepOverdueReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, before, _ := newCleanupRefreshFixture(t, 0)
		now := *before.PreviousConsumedAt
		cleanup := newCleanupWorker(f, &now, slog.Default())
		clock := cleanupAdvancingClock{from: now, started: time.Now()}
		cleanup.refresh.Clock = clock
		cleanup.refresh.Coordinator = cleanupClockCoordinator{base: f.provider.refreshDeps.Coordinator, clock: clock}
		require.NoError(t, cleanup.Reconcile(t.Context()))
		held, release, ownerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			ownerDone <- f.provider.refreshDeps.Coordinator.Run(t.Context(), f.agent.ID, func(context.Context, time.Time) error { close(held); <-release; return nil })
		}()
		<-held
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan struct{})
		go func() { defer close(finished); cleanup.Run(ctx) }()
		defer func() { cancel(); <-finished }()
		time.Sleep(before.RetryExpiresAt.Sub(now) + time.Second)
		synctest.Wait()
		readinessErr := cleanup.HealthCheck(ctx)
		close(release)
		require.NoError(t, <-ownerDone)
		require.Error(t, readinessErr, "held owner gate cannot preserve ready health past live erasure bound")
		time.Sleep(time.Second)
		synctest.Wait()
		after := cleanupRoot(t, f, before.ID)
		require.Empty(t, after.RetryCiphertext)
		require.NoError(t, cleanup.HealthCheck(ctx))
	})
}
