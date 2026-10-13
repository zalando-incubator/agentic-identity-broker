package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

func configureExtProcRun(t *testing.T, issuerURL string, port int, secret string) {
	t.Helper()
	// A caller's ExtProc config file, policy, or exporter must not affect these tests.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "EXTPROC_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("EXTPROC_GRPC_BIND", "127.0.0.1")
	t.Setenv("EXTPROC_GRPC_PORT", strconv.Itoa(port))
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", issuerURL+"/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", issuerURL)
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "lifecycle-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", secret)
	t.Setenv("EXTPROC_OAUTH2_TLS_ALLOW_HTTP", "true")
	t.Setenv("EXTPROC_TELEMETRY_ENABLED", "false")
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "false")
	t.Setenv("EXTPROC_TOOL_APPROVALS_ENABLED", "false")
}

func availableExtProcPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func TestRun_InvalidConfigFailsBeforeTokenGrant(t *testing.T) {
	var requests atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer issuer.Close()

	configureExtProcRun(t, issuer.URL, 0, "secret")
	err := run(rootCmd, nil)
	require.ErrorContains(t, err, "configuration error")
	require.ErrorContains(t, err, "grpc.port")
	assert.Zero(t, requests.Load(), "invalid configuration must fail before contacting the issuer")
}

func TestRun_TokenGrantFailureStopsStartup(t *testing.T) {
	var requests atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/oauth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"temporarily_unavailable","error_description":"provider-description-secret-sentinel","error_uri":"https://recovery-url-secret-sentinel.invalid/private/path"}`)
	}))
	defer issuer.Close()

	port := availableExtProcPort(t)
	configureExtProcRun(t, issuer.URL, port, "secret")
	err := run(rootCmd, nil)
	require.ErrorContains(t, err, "failed to initialize token exchanger")
	var operationErr *extprocserver.OperationError
	require.ErrorAs(t, err, &operationErr)
	assert.Equal(t, extprocserver.OperationAssertionRefresh, operationErr.Metadata().Operation())
	assert.Equal(t, extprocserver.KindUnavailable, operationErr.Metadata().Kind())
	assert.Equal(t, extprocserver.DependencyOAuthProvider, operationErr.Metadata().Dependency())
	assert.Equal(t, http.StatusServiceUnavailable, operationErr.Metadata().StatusCode())
	assert.NotContains(t, err.Error(), "secret-sentinel", "startup error must not serialize provider text or URLs")
	assert.Equal(t, int32(1), requests.Load(), "startup must attempt the client credentials grant")
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	require.NoError(t, err, "failed token grant must not start the gRPC listener")
	require.NoError(t, listener.Close())
}

func TestRun_ServesExtProcAndGracefullyStopsOnSIGTERM(t *testing.T) {
	const secret = "extproc-lifecycle-secret-never-log-75391"
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.ParseForm() != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			if r.PostForm.Get("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"initial-access-token","id_token":"initial-id-token","token_type":"Bearer","expires_in":3600}`)
		case "/oauth2/token":
			if r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"exchanged-access-token","token_type":"Bearer","expires_in":3600}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer issuer.Close()

	port := availableExtProcPort(t)
	configureExtProcRun(t, issuer.URL, port, secret)
	t.Setenv("EXTPROC_LOG_FORMAT", "json")

	// Capture the logger actually created by run, rather than reproducing its message.
	logReader, logWriter, err := os.Pipe()
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = logWriter
	var stdout bytes.Buffer
	logReadDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&stdout, logReader)
		logReadDone <- err
	}()
	logsClosed := false
	closeLogs := func() {
		if logsClosed {
			return
		}
		logsClosed = true
		os.Stdout = originalStdout
		_ = logWriter.Close()
		require.NoError(t, <-logReadDone)
		require.NoError(t, logReader.Close())
	}
	t.Cleanup(closeLogs)

	// Hold a second SIGTERM registration until run exits. A signal sent before
	// run's NotifyContext unregisters can never take the test process down.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(guard) })
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	rootCmd.SetArgs([]string{})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	waited := make(chan error, 1)
	go func() { waited <- Execute() }()
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		_ = process.Signal(syscall.SIGTERM)
		select {
		case <-waited:
		case <-time.After(6 * time.Second):
			t.Error("ExtProc did not stop during test cleanup")
		}
	})

	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	readyCtx, cancelReady := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancelReady()
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(readyCtx, state) {
			select {
			case err := <-waited:
				stopped = true
				t.Fatalf("ExtProc exited before gRPC became ready: %v", err)
			default:
				t.Fatalf("ExtProc gRPC listener did not become ready: %v", readyCtx.Err())
			}
		}
	}

	rpcCtx, cancelRPC := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelRPC()
	stream, err := extprocv3.NewExternalProcessorClient(conn).Process(rpcCtx)
	require.NoError(t, err)
	metadata := &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
		"aib.tokenexchange": {Fields: map[string]*structpb.Value{
			"subject_token": structpb.NewStringValue("subject-from-proxy"),
			"resource_uri":  structpb.NewStringValue("https://resource.example.test/api"),
		}},
	}}
	require.NoError(t, stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: metadata,
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{}},
		},
	}))
	require.NoError(t, stream.CloseSend())
	response, err := stream.Recv()
	require.NoError(t, err, "the registered ExternalProcessor must serve a real request")
	require.NotNil(t, response.GetRequestHeaders(), "token exchange must return a headers mutation")
	headerMutations := response.GetRequestHeaders().GetResponse().GetHeaderMutation().GetSetHeaders()
	require.Len(t, headerMutations, 1)
	assert.Equal(t, "authorization", headerMutations[0].GetHeader().GetKey())
	assert.Equal(t, "Bearer "+"exchanged-access-token", string(headerMutations[0].GetHeader().GetRawValue()))
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)

	select {
	case err := <-waited:
		stopped = true
		t.Fatalf("ExtProc exited before shutdown signal: %v", err)
	default:
	}
	require.NoError(t, process.Signal(syscall.SIGTERM))
	select {
	case err := <-waited:
		stopped = true
		require.NoError(t, err, "ExtProc did not shut down gracefully")
	case <-time.After(6 * time.Second):
		t.Fatal("ExtProc did not shut down after SIGTERM")
	}
	closeLogs()

	assert.NotContains(t, stdout.String(), secret, "the real command must never log the client secret")
	assert.NotContains(t, stdout.String(), issuer.URL, "startup must not emit endpoint URLs")
	assert.NotContains(t, stdout.String(), "lifecycle-client")
	startupLogged := false
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal(line, &entry), "invalid structured command log: %s", line)
		if entry["msg"] == "ExtProc Token Exchange Service starting" {
			startupLogged = true
		}
		for _, field := range []string{"client_secret", "client_id", "token_endpoint", "issuer", "endpoint"} {
			assert.NotContains(t, entry, field, "credential-bearing configuration must be omitted")
		}
	}
	require.True(t, startupLogged, "the actual startup log must be exercised: %s", stdout.String())

	listener, err := net.Listen("tcp", address)
	require.NoError(t, err, "SIGTERM must release the gRPC listener")
	require.NoError(t, listener.Close())
}
