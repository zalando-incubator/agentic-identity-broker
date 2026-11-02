package integration_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentReconcileConcurrentAndReplayJourney(t *testing.T) {
	ctx := context.Background()
	logger := bootstrap.TestLogger(slog.LevelWarn)
	factory := bootstrap.NewStorageFactory(logger)
	st, err := factory.NewTestStorage()
	require.NoError(t, err)
	defer func() { require.NoError(t, factory.CloseStorage(st)) }()
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, st))
	application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(st)
	require.NoError(t, err)
	server, err := bootstrap.NewAdminTestServer(application, logger)
	require.NoError(t, err)
	defer server.Close()
	principal := fixtures.AdminPrincipal().String()
	key := id.ExternalID("registry")
	desired := map[string]any{"display_name": "Research", "description": "Definition", "permission_sets": []map[string]any{{"permission_set_id": fixtures.PlaceholderPermissionSetID.String(), "requirement_type": "mandatory"}}, "client_uris": []string{"https://agent.example/z.json", "https://agent.example/a.json"}}
	type result struct {
		status int
		body   map[string]any
		err    error
	}
	call := func() result {
		response, err := helpers.ReconcileAgent(server, principal, key, desired, "")
		if err != nil {
			return result{err: err}
		}
		defer func() { _ = response.Body.Close() }()
		var body map[string]any
		err = json.NewDecoder(response.Body).Decode(&body)
		return result{response.StatusCode, body, err}
	}
	results := make(chan result, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { results <- call() })
	}
	wg.Wait()
	close(results)
	created := 0
	var identity any
	for got := range results {
		require.NoError(t, got.err)
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, got.status, got.body)
		if got.status == http.StatusCreated {
			created++
		}
		if identity == nil {
			identity = got.body["id"]
		}
		require.Equal(t, identity, got.body["id"])
	}
	require.Equal(t, 1, created)
	found, err := st.Agents().ListByExternalID(ctx, key)
	require.NoError(t, err)
	require.Len(t, found, 1)
	before := found[0].Copy()
	for range 10 {
		got := call()
		require.NoError(t, got.err)
		require.Equal(t, http.StatusOK, got.status)
		require.Equal(t, []any{"https://agent.example/a.json", "https://agent.example/z.json"}, got.body["client_uris"])
	}
	actual, err := st.Agents().Get(ctx, before.ID)
	require.NoError(t, err)
	assert.Equal(t, before, actual)
	actual.UpdatedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	require.NoError(t, st.Agents().Update(ctx, actual))
	desired["display_name"] = "Changed"
	changed := call()
	require.NoError(t, changed.err)
	require.Equal(t, http.StatusOK, changed.status)
	assert.Equal(t, actual.UpdatedAt.Add(time.Microsecond).Format(time.RFC3339Nano), changed.body["updated_at"])
	assert.Equal(t, actual.CreatedAt.Format(time.RFC3339Nano), changed.body["created_at"])
	replay := call()
	require.NoError(t, replay.err)
	assert.Equal(t, changed, replay)
}
