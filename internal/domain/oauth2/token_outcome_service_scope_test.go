package oauth2

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
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
	svc := NewTokenOutcomeService(proxy, nil).WithServiceScopeStripping("https://broker.example.com")
	resolution, err := ports.NewTokenGrantResolution(id.NewAgentID(), ptr.To(id.ClientID("upstream-client")), storage.ProxyClient)
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
