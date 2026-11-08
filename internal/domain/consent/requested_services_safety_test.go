package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestRequestedServiceSubmissionSecurityRecord(t *testing.T) {
	for _, reason := range []string{"unsatisfied", "binding_mismatch"} {
		t.Run(reason, func(t *testing.T) {
			f := newRequestedConsentFixture(t)
			var logs bytes.Buffer
			svc := f.svc
			service := NewService(svc.agentRepo, nil, svc.grantRepo, svc.sessionRepo, svc.refreshRepo, svc.psService, slog.New(slog.NewJSONHandler(&logs, nil))).
				WithRequestedServices(svc.requestedServices, svc.serviceEvaluator)
			req := f.request()
			if reason == "binding_mismatch" {
				req.ServiceScopeBindings[requestedGitHubScope] = id.NewServiceID()
			}
			_, err := service.GrantConsent(context.Background(), req)
			require.ErrorIs(t, err, ErrRequestedServiceNotSatisfied)
			assert.Zero(t, f.grants.createCalls+f.grants.updateCalls)
			var record map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record))
			assert.Equal(t, "consent.requested_service.submission_rejected", record["event"])
			assert.Equal(t, "WARN", record["level"])
			assert.Equal(t, reason, record["reason"])
			assert.Equal(t, req.Principal.LogValue().String(), record["principal"])
			assert.Equal(t, f.agentID.String(), record["agent_id"])
		})
	}
}
