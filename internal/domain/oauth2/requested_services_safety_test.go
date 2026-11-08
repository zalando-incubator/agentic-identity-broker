package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type unsafeScopeServices struct{ cause error }

func (s unsafeScopeServices) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}
func (s unsafeScopeServices) GetByCanonicalID(context.Context, string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}

type unsafeScopePermissions struct{ cause error }

func (s unsafeScopePermissions) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	return nil, s.cause
}

// SR-005: dependency causes and authorization context are never operational diagnostics.
func TestRequestedServiceSecurityRecords(t *testing.T) {
	const secret = "synthetic-access-refresh-assertion-secret-code-verifier-state-JWE"
	for _, tc := range []struct {
		name, event, level, reason, kind string
	}{
		{"malformed", "oauth2.requested_service.scope_rejected", "WARN", "malformed_scope", "definitive"},
		{"unknown", "oauth2.requested_service.scope_rejected", "WARN", "unknown_service", "definitive"},
		{"ineligible", "oauth2.requested_service.scope_rejected", "WARN", "service_not_eligible", "definitive"},
		{"binding", "oauth2.requested_service.scope_rejected", "WARN", "binding_mismatch", "definitive"},
		{"resolution", "oauth2.requested_service.resolution_failed", "ERROR", "dependency_failed", "transient"},
		{"evaluation", "oauth2.requested_service.evaluation_failed", "ERROR", "dependency_failed", "transient"},
		{"focused consent", "oauth2.authorization.incremental_required", "INFO", "missing", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRequestedFixture(t, nil, true)
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			agents := NewMockAgentRepository()
			require.NoError(t, agents.Create(context.Background(), f.agent))
			var services servicescope.ServiceLookup = f.services
			var permissions servicescope.PermissionSetLookup = requestedPermissionSets{}
			req := f.request(requestedGitHub)
			switch tc.name {
			case "malformed":
				req = f.request(requestedGitHub + "/" + secret)
			case "unknown":
				req = f.request(requestedPublicURL + "/sessions/" + secret)
			case "ineligible":
				f.agent.ServiceRequirements[0].ServiceID = id.NewServiceID()
			case "binding":
				initial := f.authorize(t, f.request(requestedGitHub))
				assert.Equal(t, "proceed", initial.Action)
				f.snapshots.snapshot.Usability = servicescope.UsabilityMissing
				token := consentToken(t, f.authorize(t, req))
				req = f.continuation(req, token)
				f.services.services[requestedGitHubID].ID = id.NewServiceID()
			case "resolution":
				services = unsafeScopeServices{cause: errors.New(secret)}
			case "evaluation":
				permissions = unsafeScopePermissions{cause: errors.New(secret)}
			case "focused consent":
				f.snapshots.snapshot.Usability = servicescope.UsabilityMissing
			}
			svc := NewAuthorizationService(f.grants, newConnectedSessionRepository(), NewAgentClientResolver(agents, logger), f.svc.config, logger, f.svc.sessionTokenService).
				WithRequestedServices(servicescope.NewResolver(services, requestedPermissionSets{}, requestedPublicURL), servicescope.NewEvaluator(permissions, f.snapshots))
			decision, err := svc.HandleAuthorization(context.Background(), req, requestedPrincipal)
			require.NoError(t, err)
			assert.NotEqual(t, "proceed", decision.Action)
			assert.NotContains(t, buf.String(), secret)
			if req.SessionToken != "" {
				assert.NotContains(t, buf.String(), req.SessionToken)
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record))
			assert.Equal(t, tc.event, record["event"])
			assert.Equal(t, tc.level, record["level"])
			assert.Equal(t, tc.reason, record["reason"])
			if tc.kind != "" {
				assert.Equal(t, tc.kind, record["error_kind"])
			}
			if tc.name == "focused consent" {
				assert.Equal(t, requestedPrincipal.LogValue().String(), record["principal"])
			}
		})
	}
}

func TestContinuationRedirectBuildDiagnosticDoesNotLogURL(t *testing.T) {
	f := newRequestedFixture(t, nil, true)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	svc := NewAuthorizationService(f.grants, newConnectedSessionRepository(), f.svc.clientResolver, f.svc.config, logger, f.svc.sessionTokenService)
	req := f.request(requestedGitHub)
	req.RedirectURI = "%synthetic-redirect-secret?state=synthetic-state#synthetic-fragment"
	svc.redirectError(context.Background(), req, "access_denied", descriptionSessionInvalid)
	assert.NotContains(t, buf.String(), "synthetic-")
	var record map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record))
	assert.Equal(t, "ERROR", record["level"])
	assert.Equal(t, "redirect_build_failed", record["reason"])
	assert.Equal(t, "internal", record["error_kind"])
	assert.NotContains(t, record, "event")
}
