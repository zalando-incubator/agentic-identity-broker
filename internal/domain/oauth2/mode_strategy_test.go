package oauth2

import (
	"context"
	"log/slog"
	"net/url"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeStrategy_Proxy(t *testing.T) {
	s := NewProxyModeStrategy()
	assert.Equal(t, servermode.Proxy, s.Mode())
	assert.True(t, s.AcceptsClientType(storage.ProxyClient))
	assert.False(t, s.AcceptsClientType(storage.CIMDClient))
	assert.False(t, s.AcceptsClientType(storage.LocalClient))
}

func TestModeStrategy_Local(t *testing.T) {
	s := NewLocalModeStrategy()
	assert.Equal(t, servermode.Local, s.Mode())
	assert.False(t, s.AcceptsClientType(storage.ProxyClient))
	assert.True(t, s.AcceptsClientType(storage.CIMDClient))
	assert.True(t, s.AcceptsClientType(storage.LocalClient))
}

func TestModeStrategy_Hybrid(t *testing.T) {
	s := NewHybridModeStrategy()
	assert.Equal(t, servermode.Hybrid, s.Mode())
	assert.True(t, s.AcceptsClientType(storage.ProxyClient))
	assert.True(t, s.AcceptsClientType(storage.CIMDClient))
	assert.True(t, s.AcceptsClientType(storage.LocalClient))
	assert.False(t, s.AcceptsClientType(storage.AmbiguousClient))
	assert.False(t, s.AcceptsClientType(storage.UnknownClient))
}

func TestHybridRequestedServicesFollowAgentMode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clientType storage.ClientType
	}{
		{name: "proxy", clientType: storage.ProxyClient},
		{name: "local", clientType: storage.LocalClient},
		{name: "CIMD", clientType: storage.CIMDClient},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientType := tc.clientType
			f := newRequestedFixture(t, &OAuth2Config{
				ModeStrategy: NewHybridModeStrategy(), UpstreamAuthorizeEndpoint: "https://idp.example.com/authorize",
			}, true)
			clientID := id.ClientID(f.agent.ID.String())
			switch clientType {
			case storage.ProxyClient:
				clientID := id.ClientID("upstream-client")
				f.agent.ClientID = &clientID
			case storage.CIMDClient:
				clientID = id.ClientID("https://client.example.com/metadata.json")
				f.agent.ClientURIs = []string{string(clientID)}
				agents := NewMockAgentRepository()
				require.NoError(t, agents.Create(context.Background(), f.agent))
				agents.RegisterURI(string(clientID), f.agent)
				metadata := &ports.CIMDFetchResult{Body: []byte(`{"client_id":"https://client.example.com/metadata.json","redirect_uris":["https://client.example.com/cb"]}`)}
				f.svc.clientResolver = NewAgentClientResolverWithCIMD(agents, cimdServiceForTest(t, metadata, nil), slog.New(slog.DiscardHandler))
			}
			request := func(scope string) *ports.AuthorizationRequest {
				req := f.request(scope)
				req.ClientID = clientID
				original, err := url.Parse(req.OriginalURL)
				require.NoError(t, err)
				query := original.Query()
				query.Set("client_id", string(clientID))
				original.RawQuery = query.Encode()
				req.OriginalURL = original.String()
				return req
			}
			decision := f.authorize(t, request("openid offline_access "+requestedGitHub))
			require.Equal(t, "proceed", decision.Action)
			assert.Equal(t, map[string]id.ServiceID{requestedGitHub: requestedGitHubID}, decision.ServiceScopeBindings)
			if clientType == storage.ProxyClient {
				upstream, err := url.Parse(decision.RedirectURL)
				require.NoError(t, err)
				assert.Equal(t, "idp.example.com", upstream.Host)
				assert.Equal(t, "openid offline_access", upstream.Query().Get("scope"))
			} else {
				assert.Empty(t, decision.RedirectURL)
			}

			f.snapshots.snapshot.Usability = "missing"
			consentToken(t, f.authorize(t, request("openid "+requestedGitHub)))
		})
	}
}

func TestAgent_ClientType(t *testing.T) {
	clientID := id.ClientID("upstream-client-id")

	cases := []struct {
		name  string
		agent storage.Agent
		want  storage.ClientType
	}{
		{
			name:  "agent with ClientID is ProxyClient",
			agent: storage.Agent{ClientID: &clientID},
			want:  storage.ProxyClient,
		},
		{
			name:  "agent with ClientURIs and no ClientID is CIMDClient",
			agent: storage.Agent{ClientURIs: []string{"https://example.com/.well-known/openid-configuration"}},
			want:  storage.CIMDClient,
		},
		{
			name:  "agent with neither is LocalClient",
			agent: storage.Agent{},
			want:  storage.LocalClient,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.agent.ClientType())
		})
	}
}
