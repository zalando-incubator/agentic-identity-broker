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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
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

func TestRootCommandLoadsBusinessEventFlagsOverEnvironmentAndFile(t *testing.T) {
	for _, name := range []string{"business_events.retention", "business_events.telemetry_copy_enabled"} {
		flag := rootCmd.PersistentFlags().Lookup(name)
		require.NotNil(t, flag, "missing operator CLI flag %q", name)
		oldValue, oldChanged := flag.Value.String(), flag.Changed
		t.Cleanup(func() {
			_ = flag.Value.Set(oldValue)
			flag.Changed = oldChanged
		})
	}

	rootBusinessEventsConfigFixture(t)
	t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION", "36h")
	t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED", "true")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("business_events:\n  retention: 48h\n  telemetry_copy_enabled: true\n"), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)

	require.NoError(t, rootCmd.ParseFlags([]string{
		"--business_events.retention=12h",
		"--business_events.telemetry_copy_enabled=false",
	}))
	loader := config.NewLoader()
	loader.SetCommand(rootCmd)
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 12*time.Hour, cfg.BusinessEvents.Retention)
	require.False(t, cfg.BusinessEvents.TelemetryCopyEnabled)

	for _, source := range loader.GetSources() {
		if source.Type == ports.SourceTypeCLI {
			require.ElementsMatch(t, []string{
				"business_events.retention",
				"business_events.telemetry_copy_enabled",
			}, source.Keys)
			return
		}
	}
	t.Fatal("explicit business-event CLI flags missing from startup source metadata")
}

func TestRootCommandSelectsBusinessEventConfigurationFile(t *testing.T) {
	for _, tt := range []struct {
		name, environmentPath, cliPath, wantFile string
		wantRetention                            time.Duration
		wantCopy                                 bool
	}{
		{name: "CLI file overrides default", cliPath: "selected.yaml", wantFile: "selected.yaml", wantRetention: 12 * time.Hour},
		{name: "CLI file overrides environment path", environmentPath: "environment.yaml", cliPath: "selected.yaml", wantFile: "selected.yaml", wantRetention: 12 * time.Hour},
		{name: "unset CLI preserves environment path", environmentPath: "environment.yaml", wantFile: "environment.yaml", wantRetention: 24 * time.Hour, wantCopy: true},
		{name: "unset CLI and environment use default", wantFile: "config.yaml", wantRetention: 48 * time.Hour, wantCopy: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup("config")
			oldValue, oldChanged := flag.Value.String(), flag.Changed
			t.Cleanup(func() {
				_ = flag.Value.Set(oldValue)
				flag.Changed = oldChanged
			})
			require.NoError(t, flag.Value.Set(""))
			flag.Changed = false
			rootBusinessEventsConfigFixture(t)
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", tt.environmentPath)
			dir := t.TempDir()
			t.Chdir(dir)
			require.NoError(t, os.WriteFile("config.yaml", []byte("business_events:\n  retention: 48h\n  telemetry_copy_enabled: true\n"), 0o600))
			require.NoError(t, os.WriteFile("environment.yaml", []byte("business_events:\n  retention: 24h\n  telemetry_copy_enabled: true\n"), 0o600))
			require.NoError(t, os.WriteFile("selected.yaml", []byte("business_events:\n  retention: 12h\n  telemetry_copy_enabled: false\n"), 0o600))
			var args []string
			if tt.cliPath != "" {
				args = []string{"--config=" + tt.cliPath}
			}
			require.NoError(t, rootCmd.ParseFlags(args))
			loader := config.NewLoader()
			loader.SetCommand(rootCmd)
			cfg, err := loader.GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantRetention, cfg.BusinessEvents.Retention)
			require.Equal(t, tt.wantCopy, cfg.BusinessEvents.TelemetryCopyEnabled)
			var paths []string
			for _, source := range loader.GetSources() {
				if source.Type == ports.SourceTypeYAML {
					paths = append(paths, source.Path)
				}
			}
			require.Equal(t, []string{filepath.Join(dir, tt.wantFile)}, paths)
		})
	}
}

func rootBusinessEventsConfigFixture(t *testing.T) {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", key)
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", key)
	t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
	t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION", "")
	t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED", "")
}
