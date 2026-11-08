package enduser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

type proxyScopeService struct {
	service *model.ThirdpartyOAuth2ProviderEntity
	err     error
}

func (s *proxyScopeService) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if s.err != nil {
		return nil, s.err
	}
	if serviceID != s.service.ID {
		return nil, ports.ErrNotFound
	}
	return s.service, nil
}

func (s *proxyScopeService) GetByCanonicalID(_ context.Context, name string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if s.err != nil {
		return nil, s.err
	}
	if name != *s.service.CanonicalID {
		return nil, ports.ErrNotFound
	}
	return s.service, nil
}

type proxyScopePermissions struct{}

func (proxyScopePermissions) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	panic("explicit requirements need no permission lookup")
}

func TestProxyTokenGrantRejectsInvalidServiceScopeBeforeUpstream(t *testing.T) {
	const publicURL = "https://broker.example.com"
	for _, tc := range []struct {
		name, scope               string
		lookupFailure, ineligible bool
		status                    int
	}{
		{"malformed", publicURL + "/sessions/github/", false, false, 400},
		{"unknown", publicURL + "/sessions/unknown", false, false, 400},
		{"ineligible", publicURL + "/sessions/github", false, true, 400},
		{"lookup failure", publicURL + "/sessions/github", true, false, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.NoError(t, r.ParseForm())
				require.Equal(t, "synthetic-refresh", r.Form.Get("refresh_token"))
				require.Equal(t, "openid", r.Form.Get("scope"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"synthetic-access","refresh_token":"rotated-refresh","token_type":"Bearer"}`)
			}))
			defer server.Close()
			service := &model.ThirdpartyOAuth2ProviderEntity{ID: id.NewServiceID(), CanonicalID: ptr.To("github")}
			services := &proxyScopeService{service: service}
			if tc.lookupFailure {
				services.err = errors.New("synthetic dependency failure")
			}
			agents := newStubAgentRepo(id.NewAgentID(), "upstream-client")
			agents.agent.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: service.ID}}
			if tc.ineligible {
				agents.agent.ServiceRequirements[0].ServiceID = id.NewServiceID()
			}
			outcomes := oauth2.NewTokenOutcomeService(NewOAuth2TokenProxy(server.URL, server.Client()), nil).
				WithServiceScopeStripping(publicURL).WithServiceScopeValidation(agents, servicescope.NewResolver(services, proxyScopePermissions{}, publicURL))
			strategy := NewProxyTokenGrantStrategy(server.URL, outcomes, nil)
			resolution, err := ports.NewTokenGrantResolution(agents.agent.ID, agents.agent.ClientID, storage.ProxyClient)
			require.NoError(t, err)
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"synthetic-refresh"}, "scope": {tc.scope}}
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", nil)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()
			strategy.HandleTokenGrant(recorder, request, "refresh_token", form, resolution)
			require.Equal(t, tc.status, recorder.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			if tc.status == 400 {
				require.Equal(t, "invalid_scope", body["error"])
				require.Equal(t, "requested service scope is not permitted", body["error_description"])
			} else {
				require.Equal(t, "server_error", body["error"])
			}
			require.Zero(t, calls)
			form.Set("scope", "openid")
			recorder = httptest.NewRecorder()
			strategy.HandleTokenGrant(recorder, request, "refresh_token", form, resolution)
			require.Equal(t, 200, recorder.Code)
			require.JSONEq(t, `{"access_token":"synthetic-access","refresh_token":"rotated-refresh","token_type":"Bearer"}`, recorder.Body.String())
			require.Equal(t, 1, calls)
		})
	}
}

func TestOAuth2TokenProxy_RejectsOversizedBufferedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token","token_type":"Bearer"}`)
		_, _ = io.WriteString(w, strings.Repeat(" ", 1<<20))
	}))
	defer server.Close()

	proxy := NewOAuth2TokenProxy(server.URL, server.Client())
	response, failure, err := proxy.Exchange(context.Background(), "grant_type=client_credentials", "application/x-www-form-urlencoded")
	require.NoError(t, err)
	require.Equal(t, ports.TokenProxyNoFailure, failure)
	defer func() { require.NoError(t, response.CloseBody()) }()

	body, err := response.ReadBody()
	require.ErrorContains(t, err, "exceeds")
	require.Nil(t, body)
}
