package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

type recordingTokenProxy struct {
	forms []url.Values
}

func (p *recordingTokenProxy) Exchange(_ context.Context, encodedForm, _ string) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	form, err := url.ParseQuery(encodedForm)
	if err != nil {
		return nil, ports.TokenProxyNoFailure, err
	}
	p.forms = append(p.forms, form)
	return nil, ports.TokenProxyNoFailure, nil
}

// Proxy refresh stripping from specs/055-incremental-service-authorization (FR-013, FR-021; DEC-002).
func TestTokenOutcomeServiceStripsVirtualScopes(t *testing.T) {
	proxy := &recordingTokenProxy{}
	f := newRequestedFixture(t, nil, true)
	agents := NewMockAgentRepository()
	require.NoError(t, agents.Create(context.Background(), f.agent))
	svc := NewTokenOutcomeService(proxy, nil, slog.New(slog.DiscardHandler)).WithServiceScopeStripping(requestedPublicURL).
		WithServiceScopeValidation(agents, servicescope.NewResolver(f.services, requestedPermissionSets{}, requestedPublicURL))
	resolution, err := ports.NewTokenGrantResolution(f.agent.ID, ptr.To(id.ClientID("upstream-client")), storage.ProxyClient)
	require.NoError(t, err)

	for _, tc := range []struct {
		scope     string
		want      string
		wantScope bool
	}{
		{"openid https://broker.example.com/sessions/github profile", "openid profile", true},
		{"https://broker.example.com/sessions/github", "", false},
		{"openid https://other.example.com/sessions/github", "openid https://other.example.com/sessions/github", true},
	} {
		form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"upstream-refresh"}, "scope": {tc.scope}, "extension": {"kept"}}
		_, _, err := svc.Proxy(context.Background(), form, "application/x-www-form-urlencoded", resolution)
		require.NoError(t, err)
		sent := proxy.forms[len(proxy.forms)-1]
		assert.Equal(t, tc.wantScope, sent.Has("scope"), tc.scope)
		assert.Equal(t, tc.want, sent.Get("scope"))
		assert.Equal(t, "kept", sent.Get("extension"))
		assert.Equal(t, "upstream-refresh", sent.Get("refresh_token"))
	}

	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"upstream-refresh"}}
	_, _, err = svc.Proxy(context.Background(), form, "application/x-www-form-urlencoded", resolution)
	require.NoError(t, err)
	assert.False(t, proxy.forms[len(proxy.forms)-1].Has("scope"), "an absent scope stays absent")
}

type tokenScopeAgents struct {
	agent *storage.Agent
	err   error
	calls int
}

func (r *tokenScopeAgents) Get(_ context.Context, agentID id.AgentID) (*storage.Agent, error) {
	r.calls++
	if r.agent != nil && r.agent.ID != agentID {
		return nil, ports.ErrNotFound
	}
	return r.agent, r.err
}

type tokenScopePermissions struct {
	err   error
	calls int
}

func (r *tokenScopePermissions) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return requestedPermissionSets{}.GetByIDs(ctx, ids)
}

// FR-002/FR-021: proxy validation never reads grants or provider sessions.
func TestTokenOutcomeServiceValidatesBeforeForwarding(t *testing.T) {
	secrets := []string{"synthetic-refresh-token", "synthetic-access-token", "synthetic-subject-token", "synthetic-client-secret", "synthetic-client-assertion", "synthetic-code", "synthetic-verifier", "synthetic-state", "synthetic-sealed-jwe", "synthetic-rejected-scope"}
	for _, tc := range []struct {
		name, scope                         string
		agentErr, serviceErr, permissionErr bool
		noRequirements, ineligible          bool
		want                                ports.TokenProxyFailure
	}{
		{name: "malformed", scope: requestedGitHub + "/", want: ports.TokenProxyScopeNotPermitted},
		{name: "malformed before lookup", scope: "openid " + requestedGitHub + "/", agentErr: true, want: ports.TokenProxyScopeNotPermitted},
		{name: "unknown", scope: requestedPublicURL + "/sessions/unknown", want: ports.TokenProxyScopeNotPermitted},
		{name: "ineligible", scope: requestedGitHub, ineligible: true, want: ports.TokenProxyScopeNotPermitted},
		{name: "agent failure", scope: requestedGitHub, agentErr: true, want: ports.TokenProxyScopeLookupFailed},
		{name: "service failure", scope: requestedGitHub, serviceErr: true, want: ports.TokenProxyScopeLookupFailed},
		{name: "permission failure", scope: requestedGitHub, noRequirements: true, permissionErr: true, want: ports.TokenProxyScopeLookupFailed},
		{name: "mixed", scope: "openid " + requestedGitHub},
		{name: "virtual only", scope: requestedGitHub},
		{name: "duplicate forms", scope: requestedGitHub + " " + requestedGitHub + " " + requestedPublicURL + "/sessions/" + requestedGitHubID.String()},
		{name: "declared permission eligibility", scope: requestedGitHub, noRequirements: true},
		{name: "ordinary", scope: "openid profile", agentErr: true, serviceErr: true, permissionErr: true},
		{name: "external lookalike", scope: "https://other.example.com/sessions/github", agentErr: true, serviceErr: true, permissionErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRequestedFixture(t, nil, false)
			if tc.noRequirements {
				f.agent.ServiceRequirements = nil
			}
			if tc.ineligible {
				f.agent.ServiceRequirements[0].ServiceID = id.NewServiceID()
			}
			agents := &tokenScopeAgents{agent: f.agent}
			permissions := &tokenScopePermissions{}
			var services servicescope.ServiceLookup = f.services
			if tc.agentErr {
				agents.err = errors.New(strings.Join(secrets, " "))
			}
			if tc.permissionErr {
				permissions.err = errors.New(strings.Join(secrets, " "))
			}
			if tc.serviceErr {
				services = failingServices{}
			}
			proxy := &recordingTokenProxy{}
			var logs bytes.Buffer
			svc := NewTokenOutcomeService(proxy, nil, slog.New(slog.NewJSONHandler(&logs, nil))).WithServiceScopeStripping(requestedPublicURL).
				WithServiceScopeValidation(agents, servicescope.NewResolver(services, permissions, requestedPublicURL))
			resolution, err := ports.NewTokenGrantResolution(f.agent.ID, ptr.To(id.ClientID("upstream-client")), storage.ProxyClient)
			require.NoError(t, err)
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"synthetic-refresh"}, "scope": {tc.scope}, "extension": {"a & b", "second"}, "access_token": {secrets[1]}, "subject_token": {secrets[2]}, "client_secret": {secrets[3]}, "client_assertion": {secrets[4]}, "code": {secrets[5]}, "code_verifier": {secrets[6]}, "state": {secrets[7]}, "session_token": {secrets[8]}}
			_, failure, err := svc.Proxy(context.Background(), form, "application/x-www-form-urlencoded", resolution)
			assert.Equal(t, tc.want, failure)
			if tc.want != ports.TokenProxyNoFailure {
				require.Error(t, err)
				switch tc.name {
				case "malformed", "malformed before lookup":
					assert.ErrorIs(t, err, servicescope.ErrMalformedScope)
				case "unknown":
					assert.ErrorIs(t, err, servicescope.ErrUnknownService)
				case "ineligible":
					assert.ErrorIs(t, err, servicescope.ErrServiceNotEligible)
				}
				assert.Empty(t, proxy.forms, "refusal cannot consume upstream credentials")
				var record map[string]any
				require.NoError(t, json.Unmarshal(logs.Bytes(), &record), "exactly one domain audit record")
				assert.Equal(t, f.agent.ID.String(), record["agent_id"])
				if tc.want == ports.TokenProxyScopeNotPermitted {
					assert.Equal(t, "oauth2.requested_service.scope_rejected", record["event"])
					assert.Equal(t, "WARN", record["level"])
					assert.Equal(t, "definitive", record["error_kind"])
					reasons := map[string]string{"malformed": "malformed_scope", "malformed before lookup": "malformed_scope", "unknown": "unknown_service", "ineligible": "service_not_eligible"}
					assert.Equal(t, reasons[tc.name], record["reason"])
				} else {
					assert.Equal(t, "oauth2.requested_service.resolution_failed", record["event"])
					assert.Equal(t, "ERROR", record["level"])
					assert.Equal(t, "transient", record["error_kind"])
					assert.Equal(t, "dependency_failed", record["reason"])
				}
				for _, secret := range append(secrets, "synthetic-refresh") {
					assert.NotContains(t, logs.String(), secret)
				}
				logs.Reset()
				_, failure, err = svc.Proxy(context.Background(), url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"synthetic-refresh"}, "scope": {"openid"}}, "application/x-www-form-urlencoded", resolution)
				require.NoError(t, err)
				assert.Equal(t, ports.TokenProxyNoFailure, failure)
				assert.Equal(t, "synthetic-refresh", proxy.forms[0].Get("refresh_token"))
			} else {
				require.NoError(t, err)
				require.Len(t, proxy.forms, 1)
				assert.Equal(t, servicescope.StripVirtual(tc.scope, requestedPublicURL), proxy.forms[0].Get("scope"))
				assert.Equal(t, []string{"a & b", "second"}, proxy.forms[0]["extension"])
			}
			assert.Empty(t, logs.String(), "successful forwarding emits no refusal audit")
			if tc.name == "ordinary" || tc.name == "external lookalike" {
				assert.Zero(t, agents.calls)
				assert.Zero(t, f.services.calls)
				assert.Zero(t, permissions.calls)
			}
		})
	}
}

func TestTokenOutcomeServiceMissingAgentFailsClosed(t *testing.T) {
	for _, lookupErr := range []error{nil, ports.ErrNotFound} {
		agents := &tokenScopeAgents{err: lookupErr}
		services := &requestedServices{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}
		proxy := &recordingTokenProxy{}
		svc := NewTokenOutcomeService(proxy, nil, slog.New(slog.DiscardHandler)).WithServiceScopeStripping(requestedPublicURL).
			WithServiceScopeValidation(agents, servicescope.NewResolver(services, requestedPermissionSets{}, requestedPublicURL))
		resolution, err := ports.NewTokenGrantResolution(id.NewAgentID(), ptr.To(id.ClientID("upstream-client")), storage.ProxyClient)
		require.NoError(t, err)
		_, failure, err := svc.Proxy(context.Background(), url.Values{"grant_type": {"refresh_token"}, "scope": {requestedGitHub}}, "", resolution)
		require.Error(t, err)
		assert.Equal(t, ports.TokenProxyScopeLookupFailed, failure)
		assert.Empty(t, proxy.forms)
	}
}
