package oauth2

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

type outcomeResponse struct {
	status    int
	body      []byte
	readError error
}

func (r outcomeResponse) ResponseStatus() int        { return r.status }
func (outcomeResponse) HeaderValues(string) []string { return nil }
func (r outcomeResponse) ReadBody() ([]byte, error)  { return r.body, r.readError }
func (outcomeResponse) CloseBody() error             { return nil }

type outcomeProxy struct {
	response ports.TokenProxyResponse
	failure  ports.TokenProxyFailure
	err      error
}

func (p outcomeProxy) Exchange(context.Context, string, string) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	return p.response, p.failure, p.err
}

type outcomeVerifier struct{ err error }

func (v outcomeVerifier) VerifyAgentIDClaim(context.Context, []byte, id.AgentID) error { return v.err }

func TestProxyLedgerStagesBodyAndRecordsOneOutcome(t *testing.T) {
	for _, status := range []int{200, 400} {
		t.Run(map[int]string{200: "issuance", 400: "upstream refusal"}[status], func(t *testing.T) {
			store := &ledgerfixture.Store{}
			svc := NewTokenOutcomeService(nil, nil, store.Recorder(t))
			agentID := id.NewAgentID()
			body := []byte(`{"access_token":"opaque-upstream-token-canary","error_description":"upstream-error-canary"}`)
			completion, _, err := svc.CompleteProxy(context.Background(), outcomeResponse{status: status, body: body}, agentID)
			require.NoError(t, err)
			require.Equal(t, body, completion.Body, "even the unverified path must stage the upstream body")
			require.False(t, completion.Verified)
			require.Len(t, store.Events, 1)
			name := "token-issued"
			if status != 200 {
				name = "token-request-failed"
			}
			require.Equal(t, model.BusinessEventTypePrefix+name, store.Events[0].Type)
			require.Equal(t, agentID, store.Events[0].AgentID)
			require.Nil(t, store.Events[0].Subject, "unverified upstream token claims cannot establish the principal")
		})
	}
}

func TestProxyLedgerRecordingFailureDoesNotReleaseStagedBody(t *testing.T) {
	store := &ledgerfixture.Store{AppendError: errors.New("ledger unavailable")}
	svc := NewTokenOutcomeService(nil, nil, store.Recorder(t))
	completion, _, err := svc.CompleteProxy(context.Background(), outcomeResponse{status: 200, body: []byte(`{"access_token":"upstream-token-canary"}`)}, id.NewAgentID())
	require.Error(t, err)
	require.Empty(t, completion.Body)
	require.Empty(t, store.Events)
}

func TestProxyLedgerTransportAndVerificationFailuresAreTerminalFacts(t *testing.T) {
	t.Run("transport", func(t *testing.T) {
		store := &ledgerfixture.Store{}
		failure := errors.New("upstream-error-canary")
		svc := NewTokenOutcomeService(outcomeProxy{failure: ports.TokenProxyTransportFailed, err: failure}, nil, store.Recorder(t))
		clientID := id.ClientID("upstream-client")
		_, classified, err := svc.Proxy(context.Background(), url.Values{"grant_type": {"client_credentials"}}, "application/x-www-form-urlencoded", &ports.TokenGrantResolution{AgentID: id.NewAgentID(), ClientID: &clientID})
		require.ErrorIs(t, err, failure)
		require.Equal(t, ports.TokenProxyTransportFailed, classified)
		require.Len(t, store.Events, 1)
		require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	})
	t.Run("verification", func(t *testing.T) {
		store := &ledgerfixture.Store{}
		svc := NewTokenOutcomeService(nil, outcomeVerifier{err: errors.New("upstream-token-canary")}, store.Recorder(t))
		completion, classified, err := svc.CompleteProxy(context.Background(), outcomeResponse{status: 200, body: []byte(`{"access_token":"unverified-token-canary"}`)}, id.NewAgentID())
		require.Error(t, err)
		require.Empty(t, completion.Body)
		require.Equal(t, ports.TokenProxyVerificationFailed, classified)
		require.Len(t, store.Events, 1)
		require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	})
}
