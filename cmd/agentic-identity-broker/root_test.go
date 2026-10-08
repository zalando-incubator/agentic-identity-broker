package main

import (
	"context"
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

func TestBrokerClientNameFlagOverridesEnvironment(t *testing.T) {
	setBrokerConfigPath(t, writeBrokerConfig(t, freeBrokerPort(t), freeBrokerPort(t)))
	t.Setenv("IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CLIENT_NAME", "Environment Platform")

	flag := rootCmd.PersistentFlags().Lookup("third_party_oauth2.client_name")
	require.NotNil(t, flag, "broker CLI must expose the optional DCR client name")
	previousValue, previousChanged := flag.Value.String(), flag.Changed
	t.Cleanup(func() {
		if err := flag.Value.Set(previousValue); err != nil {
			t.Error(err)
		}
		flag.Changed = previousChanged
	})
	require.NoError(t, rootCmd.ParseFlags([]string{"--third_party_oauth2.client_name=CLI Platform"}))

	loader := config.NewLoader()
	loader.SetCommand(rootCmd)
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "CLI Platform", cfg.ThirdPartyOAuth2.ClientName)
}

func TestRunWithWhitespaceOnlyDCRClientNameReachesServerBind(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := occupied.Close(); err != nil {
			t.Error(err)
		}
	})
	adminPort := occupied.Addr().(*net.TCPAddr).Port
	enduserPort := freeBrokerPort(t)
	setBrokerConfigPath(t, writeBrokerConfig(t, enduserPort, adminPort))
	t.Setenv("IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CLIENT_NAME", "  \t  ")

	err = run(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "admin server bind failed") {
		t.Fatalf("whitespace-only DCR name should not prevent startup before port binding, got %v", err)
	}
}
