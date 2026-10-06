package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/stretchr/testify/require"
)

const brokerTestKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func writeBrokerConfig(t *testing.T, enduserPort, adminPort int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "broker.yaml")
	config := fmt.Sprintf(`server:
  enduser:
    port: %d
    bind: 127.0.0.1
    public_url: http://127.0.0.1:%d
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    port: %d
    bind: 127.0.0.1
    public_url: http://127.0.0.1:%d
    authentication:
      preauth:
        principal_header_name: X-Admin-User
  shutdown:
    timeout: 2s
storage:
  backend: memory
third_party_oauth2:
  jwe_signing_key: %q
encryption:
  memory:
    raw_key: %q
oauth2_authorization_server:
  mode: local
`, enduserPort, enduserPort, adminPort, adminPort, brokerTestKey, brokerTestKey)
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func freeBrokerPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func setBrokerConfigPath(t *testing.T, path string) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "IDENTITY_BROKER_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
}

func TestRunRejectsMalformedConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broker.yaml")
	if err := os.WriteFile(path, []byte("server: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	setBrokerConfigPath(t, path)

	err := run(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to load configuration") || !strings.Contains(err.Error(), "valid YAML syntax") {
		t.Fatalf("expected malformed YAML to prevent startup, got %v", err)
	}
}

func TestRunRejectsInvalidServerPort(t *testing.T) {
	adminPort := freeBrokerPort(t)
	setBrokerConfigPath(t, writeBrokerConfig(t, -1, adminPort))

	err := run(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to load configuration") || !strings.Contains(err.Error(), "server.enduser.port") {
		t.Fatalf("expected invalid end-user port validation error, got %v", err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", adminPort))
	if err != nil {
		t.Fatalf("invalid configuration bound the admin port: %v", err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
}

func waitForBrokerHealth(t *testing.T, done <-chan struct{}, runErr *error, client *http.Client, port int) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var lastResult string
	for {
		response, err := client.Get(url)
		if err == nil {
			var health struct {
				Status string `json:"status"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&health)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && health.Status == "healthy" {
				return
			}
			lastResult = fmt.Sprintf("HTTP %d, health=%q, decode error=%v", response.StatusCode, health.Status, decodeErr)
		} else {
			lastResult = err.Error()
		}
		select {
		case <-done:
			t.Fatalf("broker exited before %s became healthy: %v", url, *runErr)
		case <-deadline.C:
			t.Fatalf("broker %s did not become healthy (%s)", url, lastResult)
		case <-ticker.C:
		}
	}
}

func TestRunServesBothPortsAndShutsDownOnSIGTERM(t *testing.T) {
	enduserPort := freeBrokerPort(t)
	adminPort := freeBrokerPort(t)
	for adminPort == enduserPort {
		adminPort = freeBrokerPort(t)
	}
	setBrokerConfigPath(t, writeBrokerConfig(t, enduserPort, adminPort))

	// Keep SIGTERM handled even if run exits unexpectedly before we send the signal.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	done := make(chan struct{})
	var runErr error
	go func() {
		runErr = run(rootCmd, nil)
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
				t.Errorf("signal broker during cleanup: %v", err)
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("broker did not exit during cleanup")
			}
		}
		signal.Stop(guard)
	})

	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	waitForBrokerHealth(t, done, &runErr, client, enduserPort)
	waitForBrokerHealth(t, done, &runErr, client, adminPort)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/me", enduserPort), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Remote-User", "alice@example.com")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var user struct {
		Data struct {
			Principal string `json:"principal"`
		} `json:"data"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&user)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || decodeErr != nil || user.Data.Principal != "alice@example.com" {
		t.Fatalf("end-user /api/me: HTTP %d, principal %q, decode error %v", response.StatusCode, user.Data.Principal, decodeErr)
	}

	response, err = client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/agents/", adminPort))
	if err != nil {
		t.Fatal(err)
	}
	var agents []json.RawMessage
	decodeErr = json.NewDecoder(response.Body).Decode(&agents)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || decodeErr != nil || agents == nil || len(agents) != 0 {
		t.Fatalf("admin /api/agents/: HTTP %d, agents %v, decode error %v", response.StatusCode, agents, decodeErr)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to broker: %v", err)
	}
	select {
	case <-done:
		if runErr != nil {
			t.Fatalf("broker failed to shut down on SIGTERM: %v", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("broker did not shut down after SIGTERM")
	}
	for name, port := range map[string]int{"end-user": enduserPort, "admin": adminPort} {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Errorf("%s port %d remained bound after shutdown: %v", name, port, err)
			continue
		}
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	}
}

func TestRunAtomicBindFailure(t *testing.T) {
	for _, occupiedServer := range []string{"enduser", "admin"} {
		t.Run(occupiedServer, func(t *testing.T) {
			occupied, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := occupied.Close(); err != nil {
					t.Error(err)
				}
			})
			occupiedPort := occupied.Addr().(*net.TCPAddr).Port
			peerPort := freeBrokerPort(t)
			enduserPort, adminPort := occupiedPort, peerPort
			if occupiedServer == "admin" {
				enduserPort, adminPort = peerPort, occupiedPort
			}
			setBrokerConfigPath(t, writeBrokerConfig(t, enduserPort, adminPort))
			err = run(rootCmd, nil)
			if err == nil || !strings.Contains(err.Error(), occupiedServer+" server bind failed") {
				t.Fatalf("expected %s bind error, got %v", occupiedServer, err)
			}
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", peerPort))
			if err != nil {
				t.Fatalf("peer port %d remained bound after %s startup failure: %v", peerPort, occupiedServer, err)
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRootCommandAdvertisesRefreshPolicyFlags(t *testing.T) {
	var help strings.Builder
	rootCmd.SetOut(&help)
	t.Cleanup(func() { rootCmd.SetOut(nil) })
	require.NoError(t, rootCmd.Help())
	for _, name := range []string{
		"--oauth2_authorization_server.local.refresh_token_reuse_interval",
		"--oauth2_authorization_server.local.absolute_session_lifetime",
		"--oauth2_authorization_server.local.refresh_token_ttl",
	} {
		require.Contains(t, help.String(), name)
	}
}

func TestRootRefreshPolicyFlagsControlRealRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`server:
  enduser:
    public_url: http://localhost:8000
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    authentication:
      preauth:
        principal_header_name: X-Remote-User
storage:
  backend: memory
oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 5s
    refresh_token_reuse_interval: 2s
    absolute_session_lifetime: 4s
    refresh_token_ttl: 4s
third_party_oauth2:
  jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
`), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "1s")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "1s")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "6s")
	flags := []string{
		"oauth2_authorization_server.local.refresh_token_reuse_interval",
		"oauth2_authorization_server.local.absolute_session_lifetime",
		"oauth2_authorization_server.local.refresh_token_ttl",
	}
	t.Cleanup(func() {
		for _, name := range flags {
			flag := rootCmd.Flags().Lookup(name)
			if flag != nil {
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
			}
		}
	})
	require.NoError(t, rootCmd.ParseFlags([]string{
		"--" + flags[0] + "=0s",
		"--" + flags[1] + "=0s",
		"--" + flags[2] + "=20s",
	}))
	loader := config.NewLoader()
	loader.SetCommand(rootCmd)
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	broker, err := bootstrap.NewRefreshEnvironment(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(broker.Close)
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, broker.Storage))
	client, err := broker.AddClient(ctx, id.Principal("cli-policy@example.test"), true)
	require.NoError(t, err)
	issued, err := helpers.AuthorizeRefreshSession(ctx, broker.Enduser.BaseURL(), *client, "read offline_access")
	require.NoError(t, err)
	first, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, first.Status, first.Error)

	// Environment's 1s absolute limit must not end a session when CLI selects 0s.
	time.Sleep(1200 * time.Millisecond)
	stillActive, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, stillActive.Status, stillActive.Error)

	strictClient, err := broker.AddClient(ctx, id.Principal("cli-reuse@example.test"), true)
	require.NoError(t, err)
	strictSession, err := helpers.AuthorizeRefreshSession(ctx, broker.Enduser.BaseURL(), *strictClient, "read offline_access")
	require.NoError(t, err)
	strictRotation, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *strictClient, strictSession.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, strictRotation.Status, strictRotation.Error)
	retry, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *strictClient, strictSession.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, "invalid_grant", retry.Error, "explicit CLI 0s must disable retry despite environment and YAML")

	require.NoError(t, rootCmd.ParseFlags([]string{"--" + flags[2] + "=2s"}))
	idleLoader := config.NewLoader()
	idleLoader.SetCommand(rootCmd)
	idleConfig, err := idleLoader.GetConfig(ctx)
	require.NoError(t, err)
	idleBroker, err := bootstrap.NewRefreshEnvironment(idleConfig, nil)
	require.NoError(t, err)
	t.Cleanup(idleBroker.Close)
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, idleBroker.Storage))
	second, err := idleBroker.AddClient(ctx, id.Principal("cli-idle@example.test"), true)
	require.NoError(t, err)
	newSession, err := helpers.AuthorizeRefreshSession(ctx, idleBroker.Enduser.BaseURL(), *second, "read offline_access")
	require.NoError(t, err)
	time.Sleep(2200 * time.Millisecond)
	expired, err := helpers.RotateRefreshSession(ctx, idleBroker.Enduser.BaseURL(), *second, newSession.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, "invalid_grant", expired.Error, "CLI inactivity must supersede environment and YAML")
}

func TestRootRefreshPolicyFlagsRejectBadValues(t *testing.T) {
	for _, tt := range []struct {
		flag, value, key string
	}{
		{"refresh_token_reuse_interval", "0", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "30", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "not-a-duration", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "-1s", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "2562048h", "refresh_token_reuse_interval"},
		{"absolute_session_lifetime", "0", "absolute_session_lifetime"},
		{"absolute_session_lifetime", "-1s", "absolute_session_lifetime"},
		{"absolute_session_lifetime", "2562048h", "absolute_session_lifetime"},
		{"refresh_token_ttl", "0", "refresh_token_ttl"},
		{"refresh_token_ttl", "-1s", "refresh_token_ttl"},
		{"refresh_token_ttl", "2562048h", "refresh_token_ttl"},
	} {
		t.Run(tt.flag+"="+tt.value, func(t *testing.T) {
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.yaml"))
			cmd := rootCmd
			name := "oauth2_authorization_server.local." + tt.flag
			flag := cmd.PersistentFlags().Lookup(name)
			require.NotNil(t, flag)
			t.Cleanup(func() {
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
			})
			require.NoError(t, cmd.ParseFlags([]string{"--" + name + "=" + tt.value}))
			t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
			loader := config.NewLoader()
			loader.SetCommand(cmd)
			_, err := loader.GetConfig(context.Background())
			require.ErrorContains(t, err, tt.key)
		})
	}
}

func TestRootRefreshPolicyFlagsRejectInvalidRelations(t *testing.T) {
	for _, tt := range []struct {
		name, flag, value, env, envValue, field string
	}{
		{"reuse equals access lifetime", "refresh_token_reuse_interval", "30s", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_TOKEN_TTL", "30s", "refresh_token_reuse_interval"},
		{"reuse equals inactivity lifetime", "refresh_token_reuse_interval", "2s", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "2s", "refresh_token_reuse_interval"},
		{"absolute equals reuse", "absolute_session_lifetime", "30s", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "30s", "absolute_session_lifetime"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name := "oauth2_authorization_server.local." + tt.flag
			flag := rootCmd.PersistentFlags().Lookup(name)
			require.NotNil(t, flag)
			t.Cleanup(func() {
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
			})
			require.NoError(t, rootCmd.ParseFlags([]string{"--" + name + "=" + tt.value}))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.yaml"))
			t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
			t.Setenv(tt.env, tt.envValue)
			loader := config.NewLoader()
			loader.SetCommand(rootCmd)
			_, err := loader.GetConfig(context.Background())
			require.ErrorContains(t, err, tt.field)
		})
	}
}
