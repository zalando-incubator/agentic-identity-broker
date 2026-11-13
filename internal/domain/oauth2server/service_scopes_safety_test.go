package oauth2server

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

type unsafeRefreshRecheckGrant struct {
	grantVerifier
	cause error
	calls int
}

func (v *unsafeRefreshRecheckGrant) VerifyAgentAccess(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	v.calls++
	if v.calls == 2 {
		return nil, v.cause
	}
	return v.grantVerifier.VerifyAgentAccess(ctx, principal, agentID)
}

type unsafeRefreshScopePermissions struct{ cause error }

func (s unsafeRefreshScopePermissions) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	return nil, s.cause
}

type unsafeRefreshScopeServices struct{ cause error }

func (s unsafeRefreshScopeServices) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}
func (s unsafeRefreshScopeServices) GetByCanonicalID(context.Context, string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}

func TestRefreshScopeDependencyCauseNotLogged(t *testing.T) {
	const secret = "synthetic-provider-refresh-secret-body-JWE"
	var logs bytes.Buffer
	f := newServiceScopeFixture(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	code, err := f.authorize(t, "read offline_access "+githubScope, map[string]id.ServiceID{githubScope: scopeGitHubID})
	require.NoError(t, err)
	issued, err := f.exchange(code)
	require.NoError(t, err)
	logs.Reset()
	f.provider.WithServiceScopes(servicescope.NewResolver(unsafeRefreshScopeServices{cause: errors.New(secret)}, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(scopePermissionSets{}, f.snapshots))
	_, err = f.refresh(issued.RefreshToken, "")
	requireRFC6749(t, err, "server_error", "requested service usability cannot be verified")
	assert.NotContains(t, logs.String(), secret)
	assert.NotContains(t, logs.String(), issued.AccessToken)
	assert.NotContains(t, logs.String(), issued.RefreshToken)
	assert.Contains(t, logs.String(), `"event":"oauth2_server.service.recheck_failed"`)
	assert.Contains(t, logs.String(), `"level":"ERROR"`)
	assert.Contains(t, logs.String(), `"reason":"service_lookup_failed"`)
	assert.Contains(t, logs.String(), `"error_kind":"transient"`)
	f.provider.WithServiceScopes(servicescope.NewResolver(f.services, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(scopePermissionSets{}, f.snapshots))
	_, err = f.refresh(issued.RefreshToken, "")
	require.NoError(t, err, "dependency refusal must not consume the token")
}

func TestRefreshScopeSecurityRecordClassifications(t *testing.T) {
	secrets := []string{
		"synthetic-access-token-sentinel", "synthetic-refresh-token-sentinel", "synthetic-subject-token-sentinel",
		"synthetic-client-assertion-sentinel", "synthetic-client-secret-sentinel", "synthetic-code-sentinel",
		"synthetic-PKCE-verifier-sentinel", "synthetic-state-sentinel", "synthetic-sealed-JWE-sentinel",
		"synthetic-query-sentinel", "synthetic-fragment-sentinel",
		"https://redirect.invalid/?credential=synthetic-query-sentinel#synthetic-fragment-sentinel",
	}
	cause := errors.New(strings.Join(secrets, " "))
	for _, tc := range []struct {
		reason, level, kind, code string
	}{
		{"binding_unresolved", "WARN", "definitive", "invalid_scope"},
		{"not_selected", "WARN", "definitive", "invalid_scope"},
		{"required_scopes_uncovered", "WARN", "definitive", "invalid_scope"},
		{"missing", "WARN", "definitive", "invalid_scope"},
		{"unusable_definitive", "WARN", "definitive", "invalid_scope"},
		{"service_lookup_failed", "ERROR", "transient", "server_error"},
		{"grant_lookup_failed", "ERROR", "transient", "server_error"},
		{"permission_set_lookup_failed", "ERROR", "transient", "server_error"},
		{"usability_unverifiable", "ERROR", "transient", "server_error"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			var logs bytes.Buffer
			f := newServiceScopeFixture(t, slog.New(slog.NewJSONHandler(&logs, nil)))
			code, err := f.authorize(t, "read offline_access "+githubScope, map[string]id.ServiceID{githubScope: scopeGitHubID})
			require.NoError(t, err)
			issued, err := f.exchange(code)
			require.NoError(t, err)
			logs.Reset()
			var lookup servicescope.ServiceLookup = f.services
			var permissions servicescope.PermissionSetLookup = scopePermissionSets{}
			grants := f.provider.fositeStorage.verifier.(*testConsentVerifier).grants
			grant, err := grants.FindByPrincipalAndAgent(context.Background(), id.Principal("user@example.com"), f.agent.ID)
			require.NoError(t, err)
			switch tc.reason {
			case "binding_unresolved":
				f.services.services[scopeGitHubID].ID = id.NewServiceID()
			case "not_selected":
				grant.GrantedPermissionSets[0].IncludedServiceIDs = nil
				require.NoError(t, grants.Update(context.Background(), grant))
			case "required_scopes_uncovered":
				f.agent.ServiceRequirements[0].RequiredScopes = []string{"repo", "user:email"}
				require.NoError(t, f.agents.Update(context.Background(), f.agent))
			case "missing":
				f.snapshots.snapshot.Usability = servicescope.UsabilityMissing
			case "unusable_definitive":
				f.snapshots.snapshot.Usability = servicescope.UsabilityUnusableDefinitive
			case "service_lookup_failed":
				lookup = unsafeRefreshScopeServices{cause: cause}
			case "grant_lookup_failed":
				f.provider.fositeStorage.verifier = &unsafeRefreshRecheckGrant{grantVerifier: f.provider.fositeStorage.verifier, cause: cause}
			case "permission_set_lookup_failed":
				permissions = unsafeRefreshScopePermissions{cause: cause}
			case "usability_unverifiable":
				f.snapshots.snapshot.Usability = servicescope.UsabilityUnverifiableTransient
			}
			f.provider.WithServiceScopes(servicescope.NewResolver(lookup, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(permissions, f.snapshots))
			before := grant.Copy()
			refreshed, err := f.refresh(issued.RefreshToken, "")
			assert.Nil(t, refreshed, "a refusal issues no successor credential")
			assert.Equal(t, before, grant, "a refusal leaves the grant unchanged")
			description := "requested service usability cannot be verified"
			if tc.kind == "definitive" {
				description = "scope " + githubScope + " is no longer satisfied; interactive authorization required"
			}
			requireRFC6749(t, err, tc.code, description)
			var record map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record))
			assert.Equal(t, "oauth2_server.service.recheck_failed", record["event"])
			assert.Equal(t, tc.reason, record["reason"])
			assert.Equal(t, tc.level, record["level"])
			assert.Equal(t, tc.kind, record["error_kind"])
			assert.Equal(t, f.agent.ID.String(), record["agent_id"])
			assert.Equal(t, id.Principal("user@example.com").LogValue().String(), record["principal"])
			assert.NotContains(t, record, "error")
			assert.NotContains(t, record, "kind")
			if tc.kind == "definitive" {
				assert.Equal(t, scopeGitHubID.String(), record["service_id"])
			}
			for _, secret := range append(secrets, issued.AccessToken, issued.RefreshToken, f.secret, code, scopeVerifier) {
				assert.NotContains(t, logs.String(), secret)
			}
			f.services.services[scopeGitHubID].ID = scopeGitHubID
			f.agent.ServiceRequirements[0].RequiredScopes = []string{"repo"}
			require.NoError(t, f.agents.Update(context.Background(), f.agent))
			grant.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{scopeGitHubID}
			require.NoError(t, grants.Update(context.Background(), grant))
			f.snapshots.snapshot.Usability = servicescope.UsabilityUsable
			f.provider.WithServiceScopes(servicescope.NewResolver(f.services, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(scopePermissionSets{}, f.snapshots))
			_, err = f.refresh(issued.RefreshToken, "")
			require.NoError(t, err, "every refusal leaves the same credential valid for retry")
		})
	}
}
