package enduser

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type failingTokenOutcomes struct {
	failure ports.TokenProxyFailure
	err     error
}

func (f failingTokenOutcomes) Proxy(context.Context, url.Values, string, *ports.TokenGrantResolution) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	return nil, f.failure, f.err
}

func (f failingTokenOutcomes) CompleteProxy(context.Context, ports.TokenProxyResponse, id.AgentID) (ports.TokenProxyCompletion, ports.TokenProxyFailure, error) {
	return ports.TokenProxyCompletion{}, f.failure, f.err
}

func TestProxyTokenGrantStrategy_UpstreamFailureLogsNoURLQueryOrCause(t *testing.T) {
	const upstreamURL = "https://idp.example.com/oauth2/token?tenant=query-secret"
	leaky := &url.Error{Op: "Post", URL: upstreamURL, Err: errors.New("dial tcp: connection refused")}

	for _, test := range []struct {
		name     string
		failure  ports.TokenProxyFailure
		wantKind string
	}{
		{name: "request creation", failure: ports.TokenProxyRequestCreationFailed, wantKind: security.OutboundErrorTransport},
		{name: "transport", failure: ports.TokenProxyNoFailure, wantKind: security.OutboundErrorTransport},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			strategy := NewProxyTokenGrantStrategy(upstreamURL, failingTokenOutcomes{failure: test.failure, err: leaky},
				slog.New(slog.NewJSONHandler(&logs, nil)))
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", nil)
			response := httptest.NewRecorder()

			strategy.HandleTokenGrant(response, request, "", url.Values{}, &ports.TokenGrantResolution{AgentID: id.NewAgentID()})

			require.GreaterOrEqual(t, response.Code, http.StatusInternalServerError)
			out := logs.String()
			assert.NotContains(t, out, "query-secret")
			assert.NotContains(t, out, "connection refused")
			assert.Contains(t, out, `"upstream_url":"https://idp.example.com/oauth2/token"`)
			assert.Contains(t, out, `"error_kind":"`+test.wantKind+`"`)
		})
	}
}
