package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
)

type observedDelegationResolver struct {
	hit bool
	err error
}

func (r observedDelegationResolver) Resolve(context.Context, string, string) (delegation.Result, bool, error) {
	return delegation.Result{Deadline: time.Now().Add(time.Minute), Context: delegation.Context{Principal: "context-principal-marker"}}, r.hit, r.err
}
func (observedDelegationResolver) Shutdown() {}

func TestDelegationLogs_ContainOnlyBoundedMetadata(t *testing.T) {
	tests := []struct {
		name            string
		hit             bool
		err             error
		outcome, detail string
	}{
		{"permit miss", false, nil, "success", ""},
		{"permit hit", true, nil, "success", ""},
		{"deny", false, &delegation.DeniedError{Code: "grant_missing", ConsentURL: "https://broker.example/consent?private-marker"}, "authorization_denied", "delegation_denied"},
		{"unavailable", false, errors.New("dependency-cause-marker subject-token-marker assertion-marker provider-token-marker"), "infrastructure_error", "delegation_unavailable"},
		{"invalid response", false, &delegation.Error{Category: delegation.CategoryInvalidResponse}, "infrastructure_error", "delegation_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			s := &Server{delegationResolver: observedDelegationResolver{hit: tt.hit, err: tt.err}}
			_, err := s.resolveDelegation(context.Background(), logger, "subject-token-marker", "https://resource.example/private-marker")
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
			assert.Equal(t, "delegation_resolution", record["operation"])
			cache := "miss"
			if tt.hit {
				cache = "hit"
			}
			assert.Equal(t, cache, record["delegation.cache"])
			assert.Equal(t, tt.outcome, record["token_exchange.outcome"])
			if tt.detail != "" {
				assert.Equal(t, tt.detail, record["token_exchange.failure_detail"])
			}
			for _, sensitive := range []string{"subject-token-marker", "assertion-marker", "provider-token-marker", "context-principal-marker", "dependency-cause-marker", "private-marker"} {
				assert.NotContains(t, logs.String(), sensitive)
			}
		})
	}
}
