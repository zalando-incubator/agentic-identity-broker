package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"
)

// TestRunAdminJWTPreauthCreatesCIMDKey exercises the same server construction as the broker CLI.
func TestRunAdminJWTPreauthCreatesCIMDKey(t *testing.T) {
	for _, enduserJWKS := range []bool{false, true} {
		name := "admin JWT only"
		if enduserJWKS {
			name = "admin unsigned JWT and end-user signed JWT"
		}
		t.Run(name, func(t *testing.T) {
			enduserPort := freeBrokerPort(t)
			adminPort := freeBrokerPort(t)
			for adminPort == enduserPort {
				adminPort = freeBrokerPort(t)
			}

			configPath := writeBrokerConfig(t, enduserPort, adminPort)
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			adminJWT := `
      jwt:
        header_name: X-Userinfo
        verification: none
        claim_extraction:
          principal_expression: "claims.principal"`
			configText := strings.Replace(string(config), "        principal_header_name: X-Admin-User", "        principal_header_name: X-Admin-User"+adminJWT, 1)

			var enduserToken string
			if enduserJWKS {
				key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err)
				jwkKey, err := jwk.Import[jwk.Key](key)
				require.NoError(t, err)
				require.NoError(t, jwkKey.Set(jwk.KeyIDKey, "enduser-key"))
				require.NoError(t, jwkKey.Set(jwk.AlgorithmKey, jwa.ES256()))
				publicKey, err := jwkKey.PublicKey()
				require.NoError(t, err)
				keys := jwk.NewSet()
				require.NoError(t, keys.AddKey(publicKey))
				jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(keys); err != nil {
						t.Errorf("serve test JWKS: %v", err)
					}
				}))
				t.Cleanup(jwks.Close)
				configText = strings.Replace(configText, "        principal_header_name: X-Remote-User", fmt.Sprintf(`        principal_header_name: X-Remote-User
      jwt:
        header_name: X-Enduser-Identity
        verification: jwks
        jwks_uri: %s
        expected_issuer: https://identity.example.com
        claim_extraction:
          principal_expression: "claims.sub"`, jwks.URL), 1)
				configText += "\nsecurity:\n  skip_thirdparty_https_validation: true\n"
				token := jwt.New()
				require.NoError(t, token.Set(jwt.SubjectKey, "enduser@example.com"))
				require.NoError(t, token.Set(jwt.IssuerKey, "https://identity.example.com"))
				require.NoError(t, token.Set(jwt.ExpirationKey, time.Now().Add(time.Hour)))
				signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), jwkKey))
				require.NoError(t, err)
				enduserToken = string(signed)
			}
			require.NoError(t, os.WriteFile(configPath, []byte(configText), 0o600))
			setBrokerConfigPath(t, configPath)

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
					require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("broker did not stop after SIGTERM")
					}
				}
				signal.Stop(guard)
				require.NoError(t, runErr)
			})
			client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
			waitForBrokerHealth(t, done, &runErr, client, enduserPort)
			waitForBrokerHealth(t, done, &runErr, client, adminPort)

			// These requests model the gateway-to-broker hop, not a client allowed to set X-Userinfo.
			gatewayJWT := jwt.New()
			require.NoError(t, gatewayJWT.Set("principal", "operator@example.com"))
			require.NoError(t, gatewayJWT.Set(jwt.ExpirationKey, time.Now().Add(time.Hour)))
			unsigned, err := jwt.Sign(gatewayJWT, jwt.WithInsecureNoSignature())
			require.NoError(t, err)

			adminURL := fmt.Sprintf("http://127.0.0.1:%d/api/cimd-client-keys", adminPort)
			create := func(t *testing.T, jwtHeader, plainHeader string) (int, string) {
				t.Helper()
				req, err := http.NewRequest(http.MethodPost, adminURL, strings.NewReader(`{"algorithm":"ES256"}`))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer gateway-verified-elsewhere")
				if jwtHeader != "" {
					req.Header.Set("X-Userinfo", jwtHeader)
				}
				if plainHeader != "" {
					req.Header.Set("X-Admin-User", plainHeader)
				}
				resp, err := client.Do(req)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				return resp.StatusCode, string(body)
			}
			list := func(t *testing.T) []struct {
				KID string `json:"kid"`
			} {
				t.Helper()
				resp, err := client.Get(adminURL)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				require.Equal(t, http.StatusOK, resp.StatusCode)
				var body struct {
					Items []struct {
						KID string `json:"kid"`
					} `json:"items"`
				}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				return body.Items
			}
			status, body := create(t, string(unsigned), "")
			require.Equal(t, http.StatusCreated, status, body)
			var created struct {
				KID string `json:"kid"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &created))
			require.NotEmpty(t, created.KID)
			keysAfterCreate := list(t)
			require.Len(t, keysAfterCreate, 1)
			require.Equal(t, created.KID, keysAfterCreate[0].KID)

			missingClaim := jwt.New()
			require.NoError(t, missingClaim.Set(jwt.SubjectKey, "not-an-admin-operator"))
			withoutPrincipal, err := jwt.Sign(missingClaim, jwt.WithInsecureNoSignature())
			require.NoError(t, err)
			expired := jwt.New()
			require.NoError(t, expired.Set("principal", "operator@example.com"))
			require.NoError(t, expired.Set(jwt.ExpirationKey, time.Now().Add(-time.Hour)))
			expiredJWT, err := jwt.Sign(expired, jwt.WithInsecureNoSignature())
			require.NoError(t, err)

			for _, identity := range []struct{ name, jwtHeader, plainHeader string }{
				{"missing JWT", "", "impersonator@example.com"},
				{"malformed JWT", "malformed.jwt", "impersonator@example.com"},
				{"missing operator claim", string(withoutPrincipal), "impersonator@example.com"},
				{"expired JWT", string(expiredJWT), "impersonator@example.com"},
			} {
				t.Run(identity.name, func(t *testing.T) {
					status, body := create(t, identity.jwtHeader, identity.plainHeader)
					require.Equal(t, http.StatusInternalServerError, status, body)
					require.Contains(t, body, "operator principal required for mutating CIMD key operations")
					require.Len(t, list(t), 1, "rejected identity must not create a CIMD key")
				})
			}

			if enduserJWKS {
				meURL := fmt.Sprintf("http://127.0.0.1:%d/api/me", enduserPort)
				req, err := http.NewRequest(http.MethodGet, meURL, nil)
				require.NoError(t, err)
				req.Header.Set("X-Enduser-Identity", enduserToken)
				req.Header.Set("X-Remote-User", "impersonator@example.com")
				resp, err := client.Do(req)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				require.Equal(t, http.StatusOK, resp.StatusCode)
				var result struct {
					Data struct {
						Principal string `json:"principal"`
					} `json:"data"`
				}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
				require.Equal(t, "enduser@example.com", result.Data.Principal)

				unsignedReq, err := http.NewRequest(http.MethodGet, meURL, nil)
				require.NoError(t, err)
				unsignedReq.Header.Set("X-Enduser-Identity", string(unsigned))
				unsignedReq.Header.Set("X-Remote-User", "impersonator@example.com")
				rejected, err := client.Do(unsignedReq)
				require.NoError(t, err)
				defer func() { _ = rejected.Body.Close() }()
				require.Equal(t, http.StatusUnauthorized, rejected.StatusCode, "end-user JWKS must not accept admin unsigned JWT or plain-header fallback")
			}
		})
	}
}

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
