package enduser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

type stagedProxyResponse struct {
	status int
}

func (r *stagedProxyResponse) ResponseStatus() int { return r.status }
func (*stagedProxyResponse) HeaderValues(name string) []string {
	switch name {
	case "Content-Type":
		return []string{"application/json"}
	case "Cache-Control":
		return []string{"no-store"}
	case "Pragma":
		return []string{"no-cache"}
	}
	return nil
}
func (*stagedProxyResponse) ReadBody() ([]byte, error) { panic("completion owns staging") }
func (*stagedProxyResponse) CloseBody() error          { return nil }

type stagedProxyOutcomes struct {
	response *stagedProxyResponse
	body     []byte
	err      error
}

func (s stagedProxyOutcomes) Proxy(context.Context, url.Values, string, *ports.TokenGrantResolution) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	return s.response, ports.TokenProxyNoFailure, nil
}
func (s stagedProxyOutcomes) CompleteProxy(context.Context, ports.TokenProxyResponse, id.AgentID) (ports.TokenProxyCompletion, ports.TokenProxyFailure, error) {
	return ports.TokenProxyCompletion{Body: s.body}, ports.TokenProxyNoFailure, s.err
}

func (s stagedProxyOutcomes) RecordFailure(context.Context, ports.TokenRequestFailure, id.AgentID) error {
	return s.err
}

func TestTokenGrantProxyAndHybridUseOnlyCommittedStagedResponse(t *testing.T) {
	for _, mode := range []string{"proxy", "hybrid"} {
		for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
			t.Run(mode+"/"+http.StatusText(status), func(t *testing.T) {
				response := &stagedProxyResponse{status: status}
				body := []byte(`{"access_token":"committed-token","error":"invalid_grant"}`)
				var strategy TokenGrantStrategy = NewProxyTokenGrantStrategy("https://upstream.example/token", stagedProxyOutcomes{response: response, body: body}, nil)
				if mode == "hybrid" {
					strategy = NewHybridTokenGrantStrategy(strategy, NewLocalGrantStrategy(fixedMinting(nil, errors.New("wrong route")), oauth2.NewTokenOutcomeService(nil, nil, ledgerfixture.NewRecorder()), nil), nil)
				}
				writer := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/oauth2/token", nil)
				strategy.HandleTokenGrant(writer, request, "client_credentials", url.Values{}, &ports.TokenGrantResolution{AgentID: id.NewAgentID(), ClientType: storage.ProxyClient})
				require.Equal(t, status, writer.Code)
				require.Equal(t, body, writer.Body.Bytes())
				require.Equal(t, "application/json", writer.Header().Get("Content-Type"))
				require.Equal(t, "no-store", writer.Header().Get("Cache-Control"))
			})
		}
	}
}

func TestTokenGrantProxyRecordingFailureCannotLeakBody(t *testing.T) {
	response := &stagedProxyResponse{status: http.StatusOK}
	strategy := NewProxyTokenGrantStrategy("https://upstream.example/token", stagedProxyOutcomes{response: response, body: []byte(`{"access_token":"credential-canary"}`), err: errors.New("ledger unavailable")}, nil)
	writer := httptest.NewRecorder()
	strategy.HandleTokenGrant(writer, httptest.NewRequest(http.MethodPost, "/oauth2/token", nil), "client_credentials", url.Values{}, &ports.TokenGrantResolution{AgentID: id.NewAgentID()})
	require.Equal(t, http.StatusInternalServerError, writer.Code)
	require.NotContains(t, writer.Body.String(), "credential-canary")
}
