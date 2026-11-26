package opa

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-policy-agent/opa/v1/plugins/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func writeToken(t *testing.T, path, token string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(token), 0o600))
}

func TestTokenPluginReadsTheTokenBeforeEveryRequest(t *testing.T) {
	var logs bytes.Buffer
	path := filepath.Join(t.TempDir(), "token")
	writeToken(t, path, "T1\n")
	plugin := &tokenPlugin{cfg: tokenFileConfig{path: path, scheme: "Bearer"}, telemetry: newTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))}

	prepare := func() (string, error) {
		req := httptest.NewRequest(http.MethodGet, "https://bundles.example.com/b.tar.gz", nil)
		err := plugin.Prepare(req)
		return req.Header.Get("Authorization"), err
	}
	header, err := prepare()
	require.NoError(t, err)
	assert.Equal(t, "Bearer T1", header)

	writeToken(t, path, "T2")
	header, err = prepare()
	require.NoError(t, err)
	assert.Equal(t, "Bearer T2", header, "rotation applies without a restart")

	writeToken(t, path, "  ")
	_, err = prepare()
	assert.ErrorIs(t, err, errTokenUnavailable)
	_, err = prepare()
	assert.ErrorIs(t, err, errTokenUnavailable)
	assert.Equal(t, 1, strings.Count(logs.String(), "oauth2_server.impersonation_policy.token_unavailable"), "rate-limited")
	assert.Contains(t, logs.String(), `"reason":"empty"`)
	assert.NotContains(t, logs.String(), "T2")
}

func TestTokenPluginRejectsALinkOutsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "other-credential")
	writeToken(t, outside, "OTHER-SECRET")
	path := filepath.Join(dir, "token")
	require.NoError(t, os.Symlink(outside, path))
	plugin := &tokenPlugin{cfg: tokenFileConfig{path: path, scheme: "Bearer"}, telemetry: newTelemetry(discardLogger())}
	req := httptest.NewRequest(http.MethodGet, "https://bundles.example.com/b.tar.gz", nil)
	assert.ErrorIs(t, plugin.Prepare(req), errTokenUnavailable)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestTokenPluginClientIsTraced(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	plugin := &tokenPlugin{cfg: tokenFileConfig{path: "/unused", scheme: "Bearer"}, telemetry: newTelemetry(discardLogger())}
	timeout := int64(5)
	client, err := plugin.NewClient(rest.Config{URL: server.URL, ResponseHeaderTimeoutSeconds: &timeout})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/bundles/b.tar.gz", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer SYNTHETIC-TOKEN")
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.NotEmpty(t, recorder.Ended())
	span := recorder.Ended()[0]
	assert.Equal(t, "http.request", span.Name())
	for _, attr := range span.Attributes() {
		assert.NotContains(t, attr.Value.String(), "SYNTHETIC-TOKEN")
	}
}

func TestManagedSourceResolvesTheTokenPluginThroughTheSDK(t *testing.T) {
	server := newBundleServer(t)
	server.set(func(s *bundleServer) { s.token = "T1" })
	server.publish(t, "impersonation", "r1", map[string]string{"/p.rego": allowPolicy}, governance(true))
	path := filepath.Join(t.TempDir(), "token")
	writeToken(t, path, "T1")
	bootstrap := server.bootstrap(t, "    credentials:\n      plugin: aib_bearer_token_file\nplugins:\n  aib_bearer_token_file:\n    path: "+path+"\n", "impersonation")
	src := startManaged(t, bootstrap, admitAll, governedDecision)
	require.NoError(t, awaitReady(t, src, 10*time.Second), "OPA resolves credentials.plugin through sdk.Options.Plugins")

	server.set(func(s *bundleServer) { s.token = "T2" })
	server.publish(t, "impersonation", "r2", map[string]string{"/p.rego": allowPolicy}, governance(false))
	time.Sleep(1500 * time.Millisecond)
	assert.Equal(t, "impersonation=r1", src.Current().Label(), "a rejected token keeps the active revision")
	writeToken(t, path, "T2")
	require.Eventually(t, func() bool { return src.Current().Label() == "impersonation=r2" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, ports.ImpersonationPolicyDeny, evaluate(t, src.Current()))
}
