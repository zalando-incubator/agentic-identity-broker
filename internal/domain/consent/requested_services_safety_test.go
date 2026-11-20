package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type unsafeConsentServices struct{ cause error }

func (s unsafeConsentServices) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}

func (s unsafeConsentServices) GetByCanonicalID(context.Context, string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}

type unsafeConsentPermissions struct{ cause error }

func (s unsafeConsentPermissions) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	return nil, s.cause
}

func TestRequestedServiceSubmissionDependencySafety(t *testing.T) {
	for _, dependency := range []string{"resolver", "evaluator"} {
		t.Run(dependency, func(t *testing.T) {
			f := newRequestedConsentFixture(t)
			var logs bytes.Buffer
			cause := errors.New("synthetic-access-token synthetic-refresh-token synthetic-subject-token synthetic-client-assertion synthetic-client-secret synthetic-code synthetic-PKCE-verifier synthetic-state synthetic-sealed-JWE https://redirect.invalid/?credential=synthetic-query#synthetic-fragment")
			var lookup servicescope.ServiceLookup = f.services
			var permissions servicescope.PermissionSetLookup = f.svc.psService
			if dependency == "resolver" {
				lookup = unsafeConsentServices{cause: cause}
			} else {
				permissions = unsafeConsentPermissions{cause: cause}
			}
			svc := f.svc
			service := NewService(svc.agentRepo, nil, svc.grantRepo, svc.sessionRepo, svc.refreshRepo, svc.psService, slog.New(slog.NewJSONHandler(&logs, nil))).
				WithRequestedServices(servicescope.NewResolver(lookup, svc.psService, "https://broker.example.com"), servicescope.NewEvaluator(permissions, f.snapshots))
			before := f.grants.firstGrant().Copy()
			grant, err := service.GrantConsent(context.Background(), f.request(f.readSet))
			require.ErrorIs(t, err, cause)
			assert.Nil(t, grant)
			assert.Zero(t, f.grants.createCalls+f.grants.updateCalls)
			assert.Equal(t, before, f.grants.firstGrant())
			assert.Empty(t, logs.String(), "propagated dependency failures are logged only at their handling layer")
		})
	}
}

func TestRequestedServiceSubmissionSecurityRecord(t *testing.T) {
	secrets := []string{
		"synthetic-access-token", "synthetic-refresh-token", "synthetic-subject-token",
		"synthetic-client-assertion", "synthetic-client-secret", "synthetic-code",
		"synthetic-PKCE-verifier", "synthetic-state", "synthetic-sealed-JWE",
		"synthetic-query", "synthetic-fragment",
	}
	for _, name := range []string{"unsatisfied", "binding_mismatch", "binding_mismatch_with_untrusted_scope"} {
		t.Run(name, func(t *testing.T) {
			reason := name
			if name == "binding_mismatch_with_untrusted_scope" {
				reason = "binding_mismatch"
			}
			f := newRequestedConsentFixture(t)
			var logs bytes.Buffer
			svc := f.svc
			service := NewService(svc.agentRepo, nil, svc.grantRepo, svc.sessionRepo, svc.refreshRepo, svc.psService, slog.New(slog.NewJSONHandler(&logs, nil))).
				WithRequestedServices(svc.requestedServices, svc.serviceEvaluator)
			req := f.request()
			if reason == "binding_mismatch" {
				req.ServiceScopeBindings[requestedGitHubScope] = id.NewServiceID()
			}
			if name == "binding_mismatch_with_untrusted_scope" {
				req.ServiceScopeBindings = map[string]id.ServiceID{
					"https://redirect.invalid/?credential=" + strings.Join(secrets, "/") + "#synthetic-fragment": id.NewServiceID(),
				}
			}
			before := f.grants.firstGrant().Copy()
			grant, err := service.GrantConsent(context.Background(), req)
			require.ErrorIs(t, err, ErrRequestedServiceNotSatisfied)
			assert.Nil(t, grant)
			assert.Equal(t, before, f.grants.firstGrant())
			assert.Zero(t, f.grants.createCalls+f.grants.updateCalls)
			for _, secret := range secrets {
				assert.NotContains(t, logs.String(), secret)
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record))
			assert.Equal(t, "consent.requested_service.submission_rejected", record["event"])
			assert.Equal(t, "WARN", record["level"])
			assert.Equal(t, reason, record["reason"])
			assert.Equal(t, req.Principal.LogValue().String(), record["principal"])
			assert.Equal(t, f.agentID.String(), record["agent_id"])
			assert.Equal(t, []any{f.github.String()}, record["service_ids"])
			assert.NotContains(t, record, "error")
			assert.NotContains(t, record, "error_kind")
			assert.NotContains(t, record, "kind")
		})
	}
}
