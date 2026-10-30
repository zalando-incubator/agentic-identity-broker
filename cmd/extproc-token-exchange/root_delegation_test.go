package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
)

type delegationBrokerStub struct {
	discoveryBody  func(baseURL string) string
	discoveryCode  int
	discoveries    atomic.Int32
	evaluations    atomic.Int32
	authorizations atomic.Int32
}

func (b *delegationBrokerStub) start(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" && r.URL.Path != "/oauth/token" {
			b.authorizations.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"a","id_token":"assertion","token_type":"Bearer","expires_in":3600}`)
		case "/.well-known/authzen-configuration":
			b.discoveries.Add(1)
			w.WriteHeader(b.discoveryCode)
			_, _ = io.WriteString(w, b.discoveryBody(srv.URL))
		case "/access/v1/evaluation":
			b.evaluations.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func enableDelegationRun(t *testing.T) {
	t.Helper()
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"allow\"}\n"), 0o600))
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", policyPath)
	t.Setenv("EXTPROC_DELEGATION_CONTEXT_ENABLED", "true")
}

func TestRun_DelegationContextStartupFailures(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		body    func(string) string
		errText string
	}{
		{"discovery unavailable", http.StatusServiceUnavailable, func(string) string { return `{}` }, "delegation context discovery failed"},
		{"discovery invalid", http.StatusOK, func(string) string { return `{"policy_decision_point":` }, "delegation context discovery failed"},
		{"PDP mismatch", http.StatusOK, func(string) string {
			return `{"policy_decision_point":"https://other.example.com","access_evaluation_endpoint":"https://other.example.com/access/v1/evaluation"}`
		}, "policy_decision_point mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := &delegationBrokerStub{discoveryCode: tt.code, discoveryBody: tt.body}
			srv := broker.start(t)
			port := availableExtProcPort(t)
			configureExtProcRun(t, srv.URL, port, "secret")
			enableDelegationRun(t)

			err := run(rootCmd, nil)

			require.ErrorContains(t, err, tt.errText)
			assert.NotContains(t, err.Error(), "assertion")
			assert.Equal(t, int32(1), broker.discoveries.Load())
			assert.Zero(t, broker.evaluations.Load())
			assert.Zero(t, broker.authorizations.Load(), "no credential is sent to discovery or a mismatched PDP")
			listener, listenErr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			require.NoError(t, listenErr, "failed discovery must not start the gRPC listener")
			require.NoError(t, listener.Close())
		})
	}
}

func TestRun_DelegationContextRequiresAuthorization(t *testing.T) {
	broker := &delegationBrokerStub{discoveryCode: http.StatusOK, discoveryBody: func(string) string { return `{}` }}
	srv := broker.start(t)
	configureExtProcRun(t, srv.URL, availableExtProcPort(t), "secret")
	t.Setenv("EXTPROC_DELEGATION_CONTEXT_ENABLED", "true")

	err := run(rootCmd, nil)

	require.ErrorContains(t, err, "delegation_context.enabled requires authorization.enabled")
	assert.Zero(t, broker.discoveries.Load())
}

func TestNewResolverFromConfig_UsesAdvertisedEndpointAndShutsDown(t *testing.T) {
	broker := &delegationBrokerStub{discoveryCode: http.StatusOK, discoveryBody: func(base string) string {
		return `{"policy_decision_point":"` + base + `","access_evaluation_endpoint":"` + base + `/access/v1/evaluation"}`
	}}
	srv := broker.start(t)
	cfg := &extprocconfig.Config{
		OAuth2:            extprocconfig.OAuth2Config{TLS: extprocconfig.TLSConfig{AllowHTTP: true}},
		DelegationContext: extprocconfig.DelegationContextConfig{Enabled: true, PolicyDecisionPoint: srv.URL, RequestTimeout: time.Second, MaxStaleness: time.Minute},
	}

	resolver, err := delegation.NewResolverFromConfig(context.Background(), cfg, staticAssertionProvider("assertion"))
	require.NoError(t, err)
	_, _, resolveErr := resolver.Resolve(context.Background(), "subject", "https://api.example.com")
	require.Error(t, resolveErr, "the stub endpoint rejects evaluation, which must fail closed")
	assert.Equal(t, int32(1), broker.evaluations.Load())
	resolver.Shutdown()
}

type staticAssertionProvider string

func (s staticAssertionProvider) ClientAssertion() (string, error) { return string(s), nil }
