package oauth2

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
	secrets := []string{
		"synthetic-access-token-sentinel", "synthetic-refresh-token-sentinel", "synthetic-subject-token-sentinel",
		"synthetic-client-assertion-sentinel", "synthetic-client-secret-sentinel", "synthetic-code-sentinel",
		"synthetic-PKCE-verifier-sentinel", "synthetic-state-sentinel", "synthetic-sealed-JWE-sentinel",
		"synthetic-query-sentinel", "synthetic-fragment-sentinel",
		"https://redirect.invalid/?credential=synthetic-query-sentinel#synthetic-fragment-sentinel",
	}
	secret := strings.Join(secrets, " ")
	for _, tc := range []struct {
		name, event, level, reason, kind string
	}{
		{"malformed", "oauth2.requested_service.scope_rejected", "WARN", "malformed_scope", "definitive"},
		{"unknown", "oauth2.requested_service.scope_rejected", "WARN", "unknown_service", "definitive"},
		{"ineligible", "oauth2.requested_service.scope_rejected", "WARN", "service_not_eligible", "definitive"},
		{"binding", "oauth2.requested_service.scope_rejected", "WARN", "binding_mismatch", "definitive"},
		{"resolution", "oauth2.requested_service.resolution_failed", "ERROR", "dependency_failed", "transient"},
		{"evaluation", "oauth2.requested_service.evaluation_failed", "ERROR", "dependency_failed", "transient"},
		{"missing", "oauth2.authorization.incremental_required", "INFO", "missing", ""},
		{"not_selected", "oauth2.authorization.incremental_required", "INFO", "not_selected", ""},
		{"required_scopes_uncovered", "oauth2.authorization.incremental_required", "INFO", "required_scopes_uncovered", ""},
		{"unusable_definitive", "oauth2.authorization.incremental_required", "INFO", "unusable_definitive", ""},
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
			case "missing":
				f.snapshots.snapshot.Usability = servicescope.UsabilityMissing
			case "unusable_definitive":
				f.snapshots.snapshot.Usability = servicescope.UsabilityUnusableDefinitive
			case "not_selected", "required_scopes_uncovered":
				grant, err := f.grants.FindByPrincipalAndAgent(context.Background(), requestedPrincipal, f.agent.ID)
				require.NoError(t, err)
				if tc.name == "not_selected" {
					grant.GrantedPermissionSets[0].IncludedServiceIDs = nil
				} else {
					grant.GrantedPermissionSets[0].PermissionSetID = requestedTriageSet
				}
			}
			svc := NewAuthorizationService(f.grants, newConnectedSessionRepository(), NewAgentClientResolver(agents, logger), f.svc.config, logger, f.svc.sessionTokenService).
				WithRequestedServices(servicescope.NewResolver(services, requestedPermissionSets{}, requestedPublicURL), servicescope.NewEvaluator(permissions, f.snapshots))
			grant, err := f.grants.FindByPrincipalAndAgent(context.Background(), requestedPrincipal, f.agent.ID)
			require.NoError(t, err)
			before := grant.Copy()
			decision, err := svc.HandleAuthorization(context.Background(), req, requestedPrincipal)
			require.NoError(t, err)
			assert.Equal(t, before, grant, "a refusal must not mutate the grant")
			if tc.kind == "" {
				token := consentToken(t, decision)
				require.NotEmpty(t, token)
				assert.NotContains(t, buf.String(), token)
			} else {
				code, description := "invalid_scope", notPermitted
				if tc.kind == "transient" {
					code, description = "server_error", "server error"
				}
				requireAuthorizationError(t, decision, code, description)
			}
			for _, sentinel := range secrets {
				assert.NotContains(t, buf.String(), sentinel)
			}
			require.NotEmpty(t, req.State)
			assert.NotContains(t, buf.String(), req.State)
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
			assert.Equal(t, f.agent.ID.String(), record["agent_id"])
			assert.NotContains(t, record, "error")
			assert.NotContains(t, record, "kind")
			if tc.kind == "" || tc.name == "evaluation" {
				// The central principal policy is identity; the typed-call policy test
				// guards against bypassing it with a pre-rendered principal.
				assert.Equal(t, requestedPrincipal.LogValue().String(), record["principal"])
			}
			if tc.kind == "" {
				assert.Equal(t, requestedGitHubID.String(), record["service_id"])
				assert.NotContains(t, record, "error_kind")
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
	req.State = "synthetic-state"
	req.SessionToken = "synthetic-sealed-JWE"
	req.OriginalURL = "/oauth2/authorize?code=synthetic-code&code_verifier=synthetic-PKCE#synthetic-fragment"
	decision := svc.redirectError(context.Background(), req, "access_denied", descriptionSessionInvalid)
	assert.Equal(t, "error", decision.Action)
	assert.Equal(t, "access_denied", decision.ErrorCode)
	assert.Equal(t, descriptionSessionInvalid, decision.ErrorDesc)
	assert.Empty(t, decision.RedirectURL, "redirect-build failure preserves the existing fallback")
	assert.NotContains(t, buf.String(), "synthetic-")
	var record map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record))
	assert.Equal(t, "ERROR", record["level"])
	assert.Equal(t, "redirect_build_failed", record["reason"])
	assert.Equal(t, "internal", record["error_kind"])
	assert.NotContains(t, record, "event")
}
