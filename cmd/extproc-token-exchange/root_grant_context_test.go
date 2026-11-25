package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/grantcontext"
)

const advertisedEvaluationPath = "/pdp/v2/evaluate"

type grantContextBrokerStub struct {
	discoveryBody func(baseURL string) string
	discoveryCode int
	// transientFailures makes the first discoveries fail, alternating 503 and invalid JSON.
	transientFailures   int32
	onDiscovery         func()
	discoveries         atomic.Int32
	evaluations         atomic.Int32
	advertisedEvaluated atomic.Int32
	authorizations      atomic.Int32
}

func (b *grantContextBrokerStub) start(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" && r.URL.Path != "/oauth/token" {
			b.authorizations.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"a","id_token":"assertion","token_type":"Bearer","expires_in":3600}`)
		case "/.well-known/authzen-configuration":
			n := b.discoveries.Add(1)
			if b.onDiscovery != nil {
				b.onDiscovery()
			}
			switch {
			case n <= b.transientFailures && n%2 == 1:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{}`)
			case n <= b.transientFailures:
				_, _ = io.WriteString(w, `{"policy_decision_point":`)
			default:
				w.WriteHeader(b.discoveryCode)
				_, _ = io.WriteString(w, b.discoveryBody(srv.URL))
			}
		case "/access/v1/evaluation":
			b.evaluations.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		case advertisedEvaluationPath:
			b.advertisedEvaluated.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func enableGrantContextRun(t *testing.T) {
	t.Helper()
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"allow\"}\n"), 0o600))
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", policyPath)
	t.Setenv("EXTPROC_GRANT_CONTEXT_ENABLED", "true")
}

func TestRun_GrantContextStartupFailures(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		body    func(string) string
		errText string
	}{
		{"PDP mismatch", http.StatusOK, func(string) string {
			return `{"policy_decision_point":"https://other.example.com","access_evaluation_endpoint":"https://other.example.com/access/v1/evaluation"}`
		}, "policy_decision_point mismatch"},
		{"invalid advertised endpoint", http.StatusOK, func(base string) string {
			return `{"policy_decision_point":"` + base + `","access_evaluation_endpoint":"/access/v1/evaluation"}`
		}, "invalid access_evaluation_endpoint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := &grantContextBrokerStub{discoveryCode: tt.code, discoveryBody: tt.body}
			srv := broker.start(t)
			// Occupy the gRPC port so a run that skipped discovery fails to listen instead of serving until a signal.
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, listener.Close()) })
			configureExtProcRun(t, srv.URL, listener.Addr().(*net.TCPAddr).Port, "secret")
			enableGrantContextRun(t)

			err = run(rootCmd, nil)

			require.ErrorContains(t, err, tt.errText)
			assert.NotContains(t, err.Error(), "failed to listen", "failed discovery must stop startup before the gRPC listener")
			assert.NotContains(t, err.Error(), "assertion")
			assert.Equal(t, int32(1), broker.discoveries.Load())
			assert.Zero(t, broker.evaluations.Load())
			assert.Zero(t, broker.advertisedEvaluated.Load())
			assert.Zero(t, broker.authorizations.Load(), "no credential is sent to discovery or a mismatched PDP")
		})
	}
}

func TestRun_GrantContextDiscoveryRetriesUntilTheBrokerRecovers(t *testing.T) {
	broker := &grantContextBrokerStub{transientFailures: 2, discoveryCode: http.StatusOK, discoveryBody: func(base string) string {
		return `{"policy_decision_point":"` + base + `","access_evaluation_endpoint":"` + base + `/access/v1/evaluation"}`
	}}
	srv := broker.start(t)
	// Occupy the gRPC port: reaching the listener proves discovery eventually succeeded.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	configureExtProcRun(t, srv.URL, listener.Addr().(*net.TCPAddr).Port, "secret")
	enableGrantContextRun(t)
	t.Setenv("EXTPROC_LOG_FORMAT", "json")

	records, err := runCapturingJSONLogs(t)

	require.ErrorContains(t, err, "failed to listen", "a transient discovery failure retries instead of stopping startup")
	assert.Equal(t, int32(3), broker.discoveries.Load())
	assert.Zero(t, broker.authorizations.Load(), "discovery retries send no credential")
	var retries []map[string]any
	enabled := false
	for _, record := range records {
		switch record["event"] {
		case "extproc.grant_context.discovery_failed":
			retries = append(retries, record)
		case "extproc.grant_context.enabled":
			enabled = true
		}
	}
	require.Len(t, retries, 2, "each failed attempt logs one retry record")
	for _, record := range retries {
		assert.Equal(t, "WARN", record["level"])
		assert.Contains(t, record["error"], "grant context discovery failed")
		assert.Contains(t, record, "retry_after")
	}
	assert.NotEqual(t, retries[0]["retry_after"], retries[1]["retry_after"], "the retry delay backs off")
	assert.True(t, enabled, "the recovered discovery emits the lifecycle event")
}

func TestRun_GrantContextLifecycleEvent(t *testing.T) {
	broker := &grantContextBrokerStub{discoveryCode: http.StatusOK, discoveryBody: func(base string) string {
		return `{"policy_decision_point":"` + base + `","access_evaluation_endpoint":"` + base + `/access/v1/evaluation"}`
	}}
	srv := broker.start(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	configureExtProcRun(t, srv.URL, listener.Addr().(*net.TCPAddr).Port, "secret")
	enableGrantContextRun(t)
	t.Setenv("EXTPROC_LOG_FORMAT", "json")

	records, err := runCapturingJSONLogs(t)
	require.ErrorContains(t, err, "failed to listen")

	found := false
	for _, record := range records {
		if record["event"] == "extproc.grant_context.enabled" {
			found = true
			assert.Equal(t, "INFO", record["level"])
		}
	}
	assert.True(t, found, "successful delegation discovery must emit a lifecycle event")
}

// runCapturingJSONLogs runs the command and returns the JSON log records it wrote to stdout.
func runCapturingJSONLogs(t *testing.T) ([]map[string]any, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = originalStdout })
	var logs bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&logs, reader)
		done <- err
	}()
	runErr := run(rootCmd, nil)
	os.Stdout = originalStdout
	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
	require.NoError(t, reader.Close())

	var records []map[string]any
	for _, line := range bytes.Split(logs.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		records = append(records, record)
	}
	return records, runErr
}

func TestRun_GrantContextRequiresAuthorization(t *testing.T) {
	broker := &grantContextBrokerStub{discoveryCode: http.StatusOK, discoveryBody: func(string) string { return `{}` }}
	srv := broker.start(t)
	configureExtProcRun(t, srv.URL, availableExtProcPort(t), "secret")
	t.Setenv("EXTPROC_GRANT_CONTEXT_ENABLED", "true")

	err := run(rootCmd, nil)

	require.ErrorContains(t, err, "grant_context.enabled requires authorization.enabled")
	assert.Zero(t, broker.discoveries.Load())
}

func TestNewPermitResolverFromConfig_UsesAdvertisedEndpointAndShutsDown(t *testing.T) {
	broker := &grantContextBrokerStub{discoveryCode: http.StatusOK, discoveryBody: func(base string) string {
		return `{"policy_decision_point":"` + base + `","access_evaluation_endpoint":"` + base + advertisedEvaluationPath + `"}`
	}}
	srv := broker.start(t)
	cfg := &extprocconfig.Config{
		OAuth2:       extprocconfig.OAuth2Config{TLS: extprocconfig.TLSConfig{AllowHTTP: true}},
		GrantContext: extprocconfig.GrantContextConfig{Enabled: true, PolicyDecisionPoint: srv.URL, RequestTimeout: time.Second, MaxStaleness: time.Minute},
	}

	resolver, err := grantcontext.NewPermitResolverFromConfig(context.Background(), cfg, staticAssertionProvider("assertion"), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	t.Cleanup(resolver.Shutdown)
	_, _, resolveErr := resolver.Resolve(context.Background(), "subject", "https://api.example.com")
	require.Error(t, resolveErr, "the stub endpoint rejects evaluation, which must fail closed")
	assert.Equal(t, int32(1), broker.advertisedEvaluated.Load(), "evaluation must use the advertised endpoint")
	assert.Zero(t, broker.evaluations.Load(), "evaluation must not fall back to the default path")
}

type staticAssertionProvider string

func (s staticAssertionProvider) ClientAssertion() (string, error) { return string(s), nil }

func TestRun_SignalDuringGrantContextDiscoveryIsNormalShutdown(t *testing.T) {
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	// Hold a second SIGTERM registration until run exits so the signal cannot end the test process.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(guard) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"a","id_token":"assertion","token_type":"Bearer","expires_in":3600}`)
		case "/.well-known/authzen-configuration":
			_ = process.Signal(syscall.SIGTERM)
			<-r.Context().Done()
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	// Occupy the gRPC port so a run that skipped discovery fails to listen instead of serving until a signal.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	configureExtProcRun(t, srv.URL, listener.Addr().(*net.TCPAddr).Port, "secret")
	enableGrantContextRun(t)
	t.Setenv("EXTPROC_GRANT_CONTEXT_REQUEST_TIMEOUT", "30s")
	t.Setenv("EXTPROC_LOG_FORMAT", "json")

	records, err := runCapturingJSONLogs(t)

	require.NoError(t, err, "a shutdown signal during discovery is a normal shutdown")
	found := false
	for _, record := range records {
		if record["event"] == "extproc.grant_context.discovery_interrupted" {
			found = true
			assert.Equal(t, "INFO", record["level"])
		}
	}
	assert.True(t, found, "an interrupted delegation discovery must emit a lifecycle event")
}

func TestRun_SignalDuringGrantContextDiscoveryRetryIsNormalShutdown(t *testing.T) {
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	// Hold a second SIGTERM registration until run exits so the signal cannot end the test process.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(guard) })
	// Signal after the failed response has been handled, so the run is waiting to retry; a run
	// that stopped on the first failure has already returned an error by then.
	signalled := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-signalled:
		case <-time.After(5 * time.Second):
		}
	})
	var once sync.Once
	broker := &grantContextBrokerStub{transientFailures: 1 << 30, onDiscovery: func() {
		once.Do(func() {
			time.AfterFunc(300*time.Millisecond, func() {
				_ = process.Signal(syscall.SIGTERM)
				close(signalled)
			})
		})
	}}
	srv := broker.start(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	configureExtProcRun(t, srv.URL, listener.Addr().(*net.TCPAddr).Port, "secret")
	enableGrantContextRun(t)
	t.Setenv("EXTPROC_LOG_FORMAT", "json")

	records, err := runCapturingJSONLogs(t)

	require.NoError(t, err, "a shutdown signal while waiting to retry discovery is a normal shutdown")
	assert.Equal(t, int32(1), broker.discoveries.Load(), "the signal ends the retry wait")
	found := false
	for _, record := range records {
		if record["event"] == "extproc.grant_context.discovery_interrupted" {
			found = true
		}
	}
	assert.True(t, found, "an interrupted discovery retry emits the lifecycle event")
}
