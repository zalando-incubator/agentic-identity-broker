package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"

	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	agentsservice "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJWKSPublisher is a minimal JWKS publisher stub for builder tests.
type mockJWKSPublisher struct{}

func (m *mockJWKSPublisher) PublishJWKS(_ context.Context) (jwk.Set, error) {
	return jwk.NewSet(), nil
}

type mockJWKSPublisherWithHealth struct {
	state ports.ComponentHealth
}

func (m *mockJWKSPublisherWithHealth) PublishJWKS(_ context.Context) (jwk.Set, error) {
	return jwk.NewSet(), nil
}

func (m *mockJWKSPublisherWithHealth) HealthState() ports.ComponentHealth {
	return m.state
}

func countHTTPRCGoroutines(t *testing.T) int {
	t.Helper()

	var dump bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&dump, 1); err != nil {
		t.Fatalf("pprof.Lookup(goroutine).WriteTo() failed: %v", err)
	}

	count := 0
	for _, block := range strings.Split(dump.String(), "\n\n") {
		if !strings.Contains(block, "github.com/lestrrat-go/httprc/v3.") {
			continue
		}
		var (
			groupCount int
			parsed     bool
		)
		for _, line := range strings.Split(block, "\n") {
			if _, err := fmt.Sscanf(line, "%d @", &groupCount); err == nil {
				parsed = true
				break
			}
		}
		if !parsed {
			t.Fatalf("failed to parse goroutine profile block:\n%s", block)
		}
		count += groupCount
	}
	return count
}

func waitForHTTPRCGoroutineCount(t *testing.T, want func(int) bool, failure string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	last := countHTTPRCGoroutines(t)
	for time.Now().Before(deadline) {
		last = countHTTPRCGoroutines(t)
		if want(last) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("%s (last count=%d)", failure, last)
}

// TestBuilderMinimalConfiguration validates that the builder can construct an application
// with in-memory storage and the minimum required configuration.
//
// This test documents the minimal setup needed to start the application:
// - Configuration with logging and server settings
// - In-memory storage adapter
// - Logger instance
//
// The test proves that the builder pattern works for dependency injection
// and that the application can be wired up with in-memory storage for testing.
func TestBuilderMinimalConfiguration(t *testing.T) {
	// Generate a valid JWE signing key for testing (32 bytes = 256 bits)
	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	// Create minimal configuration
	cfg := &ports.Config{
		Log: ports.LogConfig{
			Level:  ports.LogLevelInfo,
			Format: ports.LogFormatText,
		},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port:      8000,
				Bind:      "::1",
				PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{
						PrincipalHeaderName: "X-Remote-User",
					},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port:      14000,
				Bind:      "::1",
				PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{
						PrincipalHeaderName: "X-Remote-User",
					},
				},
			},
			Shutdown: ports.ShutdownConfig{
				Timeout: 30 * time.Second,
			},
		},
		Storage: ports.StorageConfig{
			Backend: "memory",
			Timeouts: ports.StorageTimeouts{
				Read:  5 * time.Second,
				Write: 5 * time.Second,
			},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: jweKey,
		},
		Encryption: ports.EncryptionConfig{
			Memory: &ports.MemoryConfig{
				RawKey: testutil.TestKEKBase64,
			},
		},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{
			Mode:  "local",
			Local: ports.LocalModeConfig{TokenTTL: time.Hour},
		},
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create in-memory storage
	adapter, err := storage.NewAdapter(&ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 5 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("failed to create storage adapter: %v", err)
	}

	// Build application with minimal configuration
	app, err := NewBuilder().
		WithConfig(cfg).
		WithStorage(adapter).
		WithLogger(logger).
		Build()

	if err != nil {
		t.Fatalf("failed to build application: %v", err)
	}

	// Verify application is wired correctly
	if app == nil {
		t.Fatal("expected non-nil application")
	}
	t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })

	// Verify required fields are set
	if app.Config == nil {
		t.Error("expected Config to be set")
	}

	if app.Storage == nil {
		t.Error("expected Storage to be set")
	}

	if app.Logger == nil {
		t.Error("expected Logger to be set")
	}

	// Verify handlers are created
	if app.AdminHandlers == nil {
		t.Error("expected AdminHandlers to be set")
	}

	if app.EnduserHandlers == nil {
		t.Error("expected EnduserHandlers to be set")
	}

	if app.EnduserHandlers.UserInfo == nil {
		t.Error("expected UserInfo handler to be created")
	}
}

// TestBuilderMissingRequiredDependency validates that the builder rejects invalid configurations.
func TestNewSigningKeyStartupContext(t *testing.T) {
	timeout := 3 * time.Second
	ctx, cancel := newSigningKeyStartupContext(timeout)
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok, "startup context should have a deadline")

	remaining := time.Until(deadline)
	assert.GreaterOrEqual(t, remaining, 2*time.Second)
	assert.LessOrEqual(t, remaining, timeout)
}

func TestBuilderMissingRequiredDependency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Try to build without configuration
	_, err := NewBuilder().
		WithStorage(nil).
		WithLogger(logger).
		Build()

	if err == nil {
		t.Error("expected error when building without configuration")
	}

	// Try to build without storage
	cfg := &ports.Config{}
	_, err = NewBuilder().
		WithConfig(cfg).
		WithLogger(logger).
		Build()

	if err == nil {
		t.Error("expected error when building without storage")
	}

	// Try to build without logger
	_, err = NewBuilder().
		WithConfig(cfg).
		WithStorage(nil).
		Build()

	if err == nil {
		t.Error("expected error when building without logger")
	}

	// Try to build with config + storage + logger but no encryption configured.
	// This specifically exercises the encryption guard at builder.go:130-132 which
	// fires after the nil-dependency checks. The test ensures that reordering or
	// removing that guard would cause a failure here, not silently pass.
	t.Run("missing encryption config", func(t *testing.T) {
		storageAdapter, err := storage.NewAdapter(&ports.StorageConfig{
			Backend: "memory",
			Timeouts: ports.StorageTimeouts{
				Read:  5 * time.Second,
				Write: 5 * time.Second,
			},
		})
		if err != nil {
			t.Fatalf("failed to create storage adapter: %v", err)
		}

		_, err = NewBuilder().
			WithConfig(&ports.Config{
				OAuth2AuthServer: ports.OAuth2AuthServerConfig{
					Mode: "proxy",
					Proxy: ports.ProxyModeConfig{
						UpstreamIssuerURI:         "https://auth.example.com",
						UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
						UpstreamTokenEndpoint:     "https://auth.example.com/token",
					},
				},
			}).
			WithStorage(storageAdapter).
			WithLogger(logger).
			Build()

		if err == nil {
			t.Fatal("expected error when building without encryption configuration")
		}
		if !strings.Contains(err.Error(), "encryption configuration required") {
			t.Errorf("expected error to contain %q, got: %v", "encryption configuration required", err)
		}
	})
}

// TestBuilderTokenExchangeExpectedAudience verifies that the builder correctly reads
// ExpectedAudience from config and passes it to the JWT validator, including the
// default fallback when no value is set.
//
// The test spins up a minimal httptest server that mimics the discovery and JWKS
// endpoints of a real upstream OAuth2 server, so the builder can complete
// JWKS-discovery and JWT-validator wiring without any external dependencies.
func TestBuilderTokenExchangeExpectedAudience(t *testing.T) {
	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	// Spin up a minimal mock upstream that serves OAuth2 discovery + empty JWKS.
	// We need discovery so the builder can find the jwks_uri, and we need
	// the JWKS endpoint so NewJWKSAdapter succeeds. The key set can be empty
	// because we are only testing wiring, not actual JWT verification here.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			baseURL := "http://" + r.Host
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 baseURL,
				"authorization_endpoint": baseURL + "/oauth/authorize",
				"token_endpoint":         baseURL + "/oauth/token",
				"jwks_uri":               baseURL + "/.well-known/jwks.json",
			})
		case "/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []interface{}{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	// buildAppWithTokenExchange constructs a complete App with token exchange enabled,
	// using the provided config modifier to adjust individual fields.
	buildAppWithTokenExchange := func(t *testing.T, modifyConfig func(*ports.Config)) (*App, error) {
		t.Helper()
		storageAdapter, err := storage.NewAdapter(&ports.StorageConfig{
			Backend: "memory",
			Timeouts: ports.StorageTimeouts{
				Read:  5 * time.Second,
				Write: 5 * time.Second,
			},
		})
		if err != nil {
			t.Fatalf("failed to create storage adapter: %v", err)
		}

		cfg := &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port:      8000,
					Bind:      "::1",
					PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port:      14000,
					Bind:      "::1",
					PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			OAuth2AuthServer: ports.OAuth2AuthServerConfig{
				Mode: "proxy",
				Proxy: ports.ProxyModeConfig{
					UpstreamIssuerURI:         upstream.URL,
					UpstreamAuthorizeEndpoint: upstream.URL + "/oauth/authorize",
					UpstreamTokenEndpoint:     upstream.URL + "/oauth/token",
					UpstreamTimeout:           5 * time.Second,
				},
			},
			TokenExchange: ports.TokenExchangeConfig{
				ClaimExtraction: ports.ClaimExtractionConfig{
					PrincipalExpression: "subject_token.sub",
					AgentIDExpression:   "subject_token.azp",
				},
				Authorization: ports.AuthorizationConfig{
					Type: "cel",
					CEL: ports.CELAuthorizationConfig{
						Expression:        "true",
						EvaluationTimeout: 100 * time.Millisecond,
					},
				},
			},
			Security: ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
		}
		modifyConfig(cfg)

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

		app, err := NewBuilder().
			WithConfig(cfg).
			WithStorage(storageAdapter).
			WithLogger(logger).
			Build()
		if err == nil {
			t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		}
		return app, err
	}

	t.Run("empty ExpectedAudience falls back to default and builds successfully", func(t *testing.T) {
		app, err := buildAppWithTokenExchange(t, func(cfg *ports.Config) {
			cfg.TokenExchange.ExpectedAudience = "" // use default "token-exchange-broker"
		})
		if err != nil {
			t.Fatalf("Build() with empty ExpectedAudience failed: %v", err)
		}
		if app.TokenExchangeService == nil {
			t.Error("expected TokenExchangeService to be set when token exchange is configured")
		}
		if app.ApprovalRequestAuthenticator == nil {
			t.Error("expected ApprovalRequestAuthenticator to be set when token exchange is configured")
		}
	})

	t.Run("custom ExpectedAudience is accepted and service is wired", func(t *testing.T) {
		app, err := buildAppWithTokenExchange(t, func(cfg *ports.Config) {
			cfg.TokenExchange.ExpectedAudience = "my-gateway"
		})
		if err != nil {
			t.Fatalf("Build() with custom ExpectedAudience failed: %v", err)
		}
		if app.TokenExchangeService == nil {
			t.Error("expected TokenExchangeService to be set when token exchange is configured")
		}
		if app.ApprovalRequestAuthenticator == nil {
			t.Error("expected ApprovalRequestAuthenticator to be set when token exchange is configured")
		}
	})

	t.Run("local mode wires token exchange with an external trust anchor", func(t *testing.T) {
		app, err := buildAppWithTokenExchange(t, func(cfg *ports.Config) {
			cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
				Mode:  "local",
				Local: ports.LocalModeConfig{TokenTTL: time.Hour},
			}
			cfg.TokenExchange.ClientAssertion.IssuerURI = upstream.URL
		})
		require.NoError(t, err)
		require.NotNil(t, app.TokenExchangeService)

		request := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		request.Header.Set("Authorization", "Bearer invalid")
		recorder := httptest.NewRecorder()
		httpmiddleware.RequireApprovalClientAssertion(app.ApprovalRequestAuthenticator)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("local mode rejects the broker issuer as trust anchor", func(t *testing.T) {
		_, err := buildAppWithTokenExchange(t, func(cfg *ports.Config) {
			cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
				Mode:  "local",
				Local: ports.LocalModeConfig{TokenTTL: time.Hour},
			}
			cfg.TokenExchange.ClientAssertion.IssuerURI = " HTTP://LOCALHOST:8000/ "
		})
		require.ErrorContains(t, err, "must be an external identity provider")
	})

	t.Run("wires approval authentication without token exchange service", func(t *testing.T) {
		app, err := buildAppWithTokenExchange(t, func(cfg *ports.Config) {
			cfg.TokenExchange.ClaimExtraction.PrincipalExpression = ""
			cfg.TokenExchange.Authorization.CEL.Expression = ""
			cfg.TokenExchange.ClaimExtraction.AgentIDExpression = "subject_token.azp"
		})
		require.NoError(t, err)
		require.Nil(t, app.TokenExchangeService)

		handler := httpmiddleware.RequireApprovalClientAssertion(app.ApprovalRequestAuthenticator)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("Authorization", "Bearer invalid")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// T056: Builder produces the correct strategy set for each OAuth2 server mode.
// Proxy → JWKS handler nil; Local/Hybrid → JWKS handler non-nil (signing keys served).
func TestBuilder_ModeStrategyWiring(t *testing.T) {
	baseConfig := func(jweKey string) *ports.Config {
		return &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		}
	}

	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("proxy mode — JWKS handler is non-nil", func(t *testing.T) {
		cfg := baseConfig(jweKey)
		cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
		}
		app, err := NewBuilder().WithConfig(cfg).WithStorage(newStorage(t)).WithLogger(logger).
			WithJWKSPublisher(&mockJWKSPublisher{}).Build()
		if err != nil {
			t.Fatalf("Build() in proxy mode failed: %v", err)
		}
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		if app.EnduserHandlers.JWKS == nil {
			t.Error("proxy mode must wire a JWKS handler")
		}
	})

	t.Run("proxy mode exposes upstream JWKS health", func(t *testing.T) {
		cfg := baseConfig(jweKey)
		cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
		}
		app, err := NewBuilder().WithConfig(cfg).WithStorage(newStorage(t)).WithLogger(logger).
			WithJWKSPublisher(&mockJWKSPublisherWithHealth{state: ports.ComponentHealthDegraded}).Build()
		if err != nil {
			t.Fatalf("Build() in proxy mode failed: %v", err)
		}
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		if got := app.EnduserHealthComponents(); got["upstream_jwks"] != "degraded" {
			t.Fatalf("EnduserHealthComponents()[\"upstream_jwks\"] = %q, want %q", got["upstream_jwks"], "degraded")
		}
		if app.AdminHandlers.SigningKeys != nil {
			t.Error("proxy mode must not wire signing key admin handlers")
		}
	})

	t.Run("local mode — JWKS handler is non-nil", func(t *testing.T) {
		cfg := baseConfig(jweKey)
		cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
			Mode:  "local",
			Local: ports.LocalModeConfig{TokenTTL: time.Hour},
		}
		app, err := NewBuilder().WithConfig(cfg).WithStorage(newStorage(t)).WithLogger(logger).Build()
		if err != nil {
			t.Fatalf("Build() in local mode failed: %v", err)
		}
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		if app.EnduserHandlers.JWKS == nil {
			t.Error("local mode must wire a JWKS handler")
		}
		if got := app.EnduserHealthComponents(); got != nil {
			t.Fatalf("EnduserHealthComponents() = %#v, want nil in local mode", got)
		}
	})

	t.Run("local mode with token exchange requires an external trust anchor", func(t *testing.T) {
		cfg := baseConfig(jweKey)
		cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
			Mode:  "local",
			Local: ports.LocalModeConfig{TokenTTL: time.Hour},
		}
		cfg.TokenExchange = ports.TokenExchangeConfig{
			ClaimExtraction: ports.ClaimExtractionConfig{
				PrincipalExpression: "subject_token.sub",
				AgentIDExpression:   "resolveAgentIdByClientId(subject_token.azp)",
			},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: "true"},
			},
		}
		_, err := NewBuilder().WithConfig(cfg).WithStorage(newStorage(t)).WithLogger(logger).Build()
		if err == nil || !strings.Contains(err.Error(), "no client-assertion trust anchor is set") {
			t.Fatalf("Build() error = %v, want missing client-assertion trust anchor", err)
		}
	})

	t.Run("hybrid mode — JWKS handler is non-nil", func(t *testing.T) {
		cfg := baseConfig(jweKey)
		cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
			Mode: "hybrid",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
			Local: ports.LocalModeConfig{TokenTTL: time.Hour},
		}
		app, err := NewBuilder().WithConfig(cfg).WithStorage(newStorage(t)).WithLogger(logger).
			WithJWKSPublisher(&mockJWKSPublisher{}).Build()
		if err != nil {
			t.Fatalf("Build() in hybrid mode failed: %v", err)
		}
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		if app.EnduserHandlers.JWKS == nil {
			t.Error("hybrid mode must wire a JWKS handler")
		}
	})
}

func TestNewTokenExchangeAgentIDResolver(t *testing.T) {
	newService := func(repo ports.AgentRepository) *agentsservice.Service {
		return agentsservice.NewService(repo, builderTestRequirementValidator{}, slog.Default(), false)
	}

	t.Run("resolves upstream client ID before any UUID fallback", func(t *testing.T) {
		agentID := id.NewAgentID()
		repo := &builderTestAgentRepo{
			byClientID: map[id.ClientID]*domstorage.Agent{
				id.ClientID("upstream-client"): {
					ID:          agentID,
					DisplayName: "Agent",
					Description: "desc",
				},
			},
		}

		resolve := newTokenExchangeAgentIDResolver(newService(repo), time.Second)
		got, err := resolve("upstream-client")
		require.NoError(t, err)
		assert.Equal(t, agentID.String(), got)
	})

	t.Run("falls back to agent UUID when client ID lookup is not found", func(t *testing.T) {
		agentID := id.NewAgentID()
		repo := &builderTestAgentRepo{
			byID: map[id.AgentID]*domstorage.Agent{
				agentID: {
					ID:          agentID,
					DisplayName: "Agent",
					Description: "desc",
				},
			},
		}

		resolve := newTokenExchangeAgentIDResolver(newService(repo), time.Second)
		got, err := resolve(agentID.String())
		require.NoError(t, err)
		assert.Equal(t, agentID.String(), got)
	})

	t.Run("ambiguous client ID still fails even when raw value parses as UUID", func(t *testing.T) {
		rawIdentifier := id.NewAgentID().String()
		firstAgentID := id.NewAgentID()
		repo := &builderTestAgentRepo{
			byClientID: map[id.ClientID]*domstorage.Agent{
				id.ClientID(rawIdentifier): {
					ID:          firstAgentID,
					DisplayName: "Agent A",
					Description: "desc",
				},
			},
			existsOtherWithClientID: true,
		}

		resolve := newTokenExchangeAgentIDResolver(newService(repo), time.Second)
		_, err := resolve(rawIdentifier)
		require.Error(t, err)
		assert.ErrorContains(t, err, "ambiguous")
	})
}

type builderTestRequirementValidator struct{}

func (builderTestRequirementValidator) ValidateServiceRequirements(context.Context, []domstorage.ServiceRequirement) error {
	return nil
}

type builderTestAgentRepo struct {
	byID                    map[id.AgentID]*domstorage.Agent
	byClientID              map[id.ClientID]*domstorage.Agent
	getErr                  error
	getByClientIDErr        error
	existsOtherWithClientID bool
	existsOtherErr          error
}

func (r *builderTestAgentRepo) Create(context.Context, *domstorage.Agent) error { return nil }

func (r *builderTestAgentRepo) Get(_ context.Context, agentID id.AgentID) (*domstorage.Agent, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if agent, ok := r.byID[agentID]; ok {
		return agent.Copy(), nil
	}
	return nil, ports.ErrNotFound
}

func (r *builderTestAgentRepo) Update(context.Context, *domstorage.Agent) error { return nil }

func (r *builderTestAgentRepo) Delete(context.Context, id.AgentID) error { return nil }

func (r *builderTestAgentRepo) List(context.Context) ([]*domstorage.Agent, error) { return nil, nil }

func (r *builderTestAgentRepo) GetByClientID(_ context.Context, clientID id.ClientID) (*domstorage.Agent, error) {
	if r.getByClientIDErr != nil {
		return nil, r.getByClientIDErr
	}
	if agent, ok := r.byClientID[clientID]; ok {
		return agent.Copy(), nil
	}
	return nil, ports.ErrNotFound
}

func (r *builderTestAgentRepo) ExistsOtherWithClientID(context.Context, id.ClientID, *id.AgentID) (bool, error) {
	return r.existsOtherWithClientID, r.existsOtherErr
}

func (r *builderTestAgentRepo) GetByClientURI(context.Context, string) (*domstorage.Agent, error) {
	return nil, ports.ErrNotFound
}

func TestBuilder_ProxyJWKSFailsWhenStartupMetadataDiscoveryFails(t *testing.T) {
	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	newConfig := func(jweKey string, upstreamURL string) *ports.Config {
		return &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			Security:         ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
			OAuth2AuthServer: ports.OAuth2AuthServerConfig{
				Mode: "proxy",
				Proxy: ports.ProxyModeConfig{
					UpstreamIssuerURI:         upstreamURL,
					UpstreamAuthorizeEndpoint: upstreamURL + "/authorize",
					UpstreamTokenEndpoint:     upstreamURL + "/token",
					UpstreamTimeout:           time.Second,
					UpstreamJWKSMinRefresh:    time.Second,
					UpstreamJWKSMaxRefresh:    time.Second,
				},
			},
		}
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	_, err := NewBuilder().
		WithConfig(newConfig(jweKey, upstream.URL)).
		WithStorage(newStorage(t)).
		WithLogger(logger).
		Build()
	if err == nil {
		t.Fatal("Build() in proxy mode succeeded, want metadata discovery startup error")
	}
	if !strings.Contains(err.Error(), "failed to discover OAuth2 server metadata") {
		t.Fatalf("Build() error = %v, want discovery failure", err)
	}
}

func TestBuilder_HybridJWKSWarnsWhenStartupProbeFails(t *testing.T) {
	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	newConfig := func(jweKey string, upstreamURL string) *ports.Config {
		return &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			Security:         ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
			OAuth2AuthServer: ports.OAuth2AuthServerConfig{
				Mode: "hybrid",
				Proxy: ports.ProxyModeConfig{
					UpstreamIssuerURI:         upstreamURL,
					UpstreamAuthorizeEndpoint: upstreamURL + "/authorize",
					UpstreamTokenEndpoint:     upstreamURL + "/token",
					UpstreamTimeout:           time.Second,
					UpstreamJWKSMinRefresh:    time.Second,
					UpstreamJWKSMaxRefresh:    time.Second,
				},
				Local: ports.LocalModeConfig{TokenTTL: time.Hour},
			},
		}
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			baseURL := "http://" + r.Host
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 baseURL,
				"authorization_endpoint": baseURL + "/authorize",
				"token_endpoint":         baseURL + "/token",
				"jwks_uri":               baseURL + "/.well-known/jwks.json",
			})
		case "/.well-known/jwks.json":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	app, err := NewBuilder().
		WithConfig(newConfig(jweKey, upstream.URL)).
		WithStorage(newStorage(t)).
		WithLogger(logger).
		Build()
	if err != nil {
		t.Fatalf("Build() in hybrid mode failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = app.Shutdown(shutdownCtx)
	}()

	if !strings.Contains(logs.String(), "startup upstream JWKS probe failed") {
		t.Fatalf("expected startup JWKS probe warning, got logs: %s", logs.String())
	}
}

func TestBuilder_ShutdownStopsUpstreamJWKSAdapterWorkers(t *testing.T) {
	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	newConfig := func(jweKey string, mode string, upstreamURL string) *ports.Config {
		cfg := &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			Security:         ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
		}

		switch mode {
		case "proxy":
			cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
				Mode: "proxy",
				Proxy: ports.ProxyModeConfig{
					UpstreamIssuerURI:         upstreamURL,
					UpstreamAuthorizeEndpoint: upstreamURL + "/authorize",
					UpstreamTokenEndpoint:     upstreamURL + "/token",
					UpstreamTimeout:           time.Second,
				},
			}
		case "hybrid":
			cfg.OAuth2AuthServer = ports.OAuth2AuthServerConfig{
				Mode: "hybrid",
				Proxy: ports.ProxyModeConfig{
					UpstreamIssuerURI:         upstreamURL,
					UpstreamAuthorizeEndpoint: upstreamURL + "/authorize",
					UpstreamTokenEndpoint:     upstreamURL + "/token",
					UpstreamTimeout:           time.Second,
				},
				Local: ports.LocalModeConfig{TokenTTL: time.Hour},
			}
		default:
			t.Fatalf("unsupported mode %q", mode)
		}

		return cfg
	}

	newUpstream := func(t *testing.T) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				baseURL := "http://" + r.Host
				_ = json.NewEncoder(w).Encode(map[string]string{
					"issuer":                 baseURL,
					"authorization_endpoint": baseURL + "/authorize",
					"token_endpoint":         baseURL + "/token",
					"jwks_uri":               baseURL + "/.well-known/jwks.json",
				})
			case "/.well-known/jwks.json":
				_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
			default:
				http.NotFound(w, r)
			}
		}))
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	for _, mode := range []string{"proxy", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			upstream := newUpstream(t)
			defer upstream.Close()

			baseline := countHTTPRCGoroutines(t)

			app, err := NewBuilder().
				WithConfig(newConfig(jweKey, mode, upstream.URL)).
				WithStorage(newStorage(t)).
				WithLogger(logger).
				Build()
			if err != nil {
				t.Fatalf("Build() in %s mode failed: %v", mode, err)
			}
			if app.Shutdown == nil {
				t.Fatalf("Build() in %s mode returned nil Shutdown", mode)
			}

			waitForHTTPRCGoroutineCount(t,
				func(count int) bool { return count > baseline },
				"expected upstream JWKS adapter workers to start")

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := app.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("Shutdown() in %s mode failed: %v", mode, err)
			}

			waitForHTTPRCGoroutineCount(t,
				func(count int) bool { return count <= baseline },
				"expected upstream JWKS adapter workers to stop after shutdown")
		})
	}
}

func TestBuilder_LocalModeSigningKeyReadiness(t *testing.T) {
	newConfig := func(jweKey string) *ports.Config {
		return &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			OAuth2AuthServer: ports.OAuth2AuthServerConfig{
				Mode:  "local",
				Local: ports.LocalModeConfig{TokenTTL: time.Hour},
			},
		}
	}

	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	t.Run("auto-generates an initial signing key when none exist", func(t *testing.T) {
		adapter := newStorage(t)

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		app, err := NewBuilder().WithConfig(newConfig(jweKey)).WithStorage(adapter).WithLogger(logger).Build()
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil app")
		}
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })

		count, err := adapter.SigningKeys().CountActive(context.Background())
		if err != nil {
			t.Fatalf("CountActive() failed: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected one auto-generated signing key, got %d", count)
		}

		current, err := adapter.SigningKeys().GetCurrent(context.Background())
		if err != nil {
			t.Fatalf("GetCurrent() failed: %v", err)
		}
		if current.ActivatesAt.After(time.Now().Add(time.Second)) {
			t.Fatalf("expected auto-generated signing key to be immediately active, activates_at=%s", current.ActivatesAt)
		}
		if !strings.Contains(logBuf.String(), "auto-generated initial signing key") {
			t.Fatalf("expected auto-generation log entry, got: %s", logBuf.String())
		}
	})

	t.Run("uses the local signing-key bootstrap timeout for both startup contexts", func(t *testing.T) {
		adapter := newStorage(t)

		originalNewSigningKeyStartupContext := newSigningKeyStartupContext
		t.Cleanup(func() {
			newSigningKeyStartupContext = originalNewSigningKeyStartupContext
		})

		calls := 0
		var requestedTimeouts []time.Duration
		newSigningKeyStartupContext = func(timeout time.Duration) (context.Context, context.CancelFunc) {
			calls++
			requestedTimeouts = append(requestedTimeouts, timeout)
			return context.WithCancel(context.Background())
		}

		cfg := newConfig(jweKey)
		cfg.OAuth2AuthServer.Local.SigningKeys.BootstrapTimeout = 12 * time.Second

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		app, err := NewBuilder().WithConfig(cfg).WithStorage(adapter).WithLogger(logger).Build()
		require.NoError(t, err)
		require.NotNil(t, app)
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		assert.Equal(t, 2, calls)
		require.Len(t, requestedTimeouts, 2)
		assert.Equal(t, cfg.OAuth2AuthServer.Local.SigningKeys.BootstrapTimeout, requestedTimeouts[0])
		assert.Equal(t, cfg.OAuth2AuthServer.Local.SigningKeys.BootstrapTimeout, requestedTimeouts[1])
	})

	t.Run("propagates EnsureInitialKey failure from bootstrap", func(t *testing.T) {
		adapter := newStorage(t)

		originalNewSigningKeyStartupContext := newSigningKeyStartupContext
		t.Cleanup(func() {
			newSigningKeyStartupContext = originalNewSigningKeyStartupContext
		})

		newSigningKeyStartupContext = func(time.Duration) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, func() {}
		}

		_, err := NewBuilder().WithConfig(newConfig(jweKey)).WithStorage(adapter).WithLogger(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))).Build()
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to ensure initial signing key")
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("warns when only a grace-period key exists", func(t *testing.T) {
		adapter := newStorage(t)
		err := adapter.SigningKeys().Create(context.Background(), &domstorage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 id.NewKeyID("550e8400-e29b-41d4-a716-446655440000"),
			Algorithm:           "ES256",
			PrivateKeyEncrypted: []byte("ciphertext"),
			IsCurrent:           true,
			ActivatesAt:         time.Now().Add(time.Hour),
			CreatedAt:           time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("seed signing key: %v", err)
		}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

		app, err := NewBuilder().WithConfig(newConfig(jweKey)).WithStorage(adapter).WithLogger(logger).Build()
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil app")
		}
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
		assert.Contains(t, logBuf.String(), `"level":"WARN"`)
		assert.Contains(t, logBuf.String(), "no currently-active signing key available")
		assert.Contains(t, logBuf.String(), "local token issuance is unavailable")
	})
}

func TestBuilder_HybridModeSigningKeyReadiness(t *testing.T) {
	newConfig := func(jweKey string, upstreamURL string) *ports.Config {
		return &ports.Config{
			Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
			Server: ports.ServerConfig{
				EndUser: ports.ServerInstanceConfig{
					Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Admin: ports.ServerInstanceConfig{
					Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
					Authentication: ports.AuthenticationConfig{
						Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
					},
				},
				Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
			},
			Storage: ports.StorageConfig{
				Backend:  "memory",
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			Security:         ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
			OAuth2AuthServer: ports.OAuth2AuthServerConfig{
				Mode: "hybrid",
				Proxy: ports.ProxyModeConfig{
					UpstreamIssuerURI:         upstreamURL,
					UpstreamAuthorizeEndpoint: upstreamURL + "/authorize",
					UpstreamTokenEndpoint:     upstreamURL + "/token",
				},
				Local: ports.LocalModeConfig{TokenTTL: time.Hour},
			},
		}
	}

	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	t.Run("auto-generates an initial signing key when none exist", func(t *testing.T) {
		adapter := newStorage(t)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				baseURL := "http://" + r.Host
				_ = json.NewEncoder(w).Encode(map[string]string{
					"issuer":                 baseURL,
					"authorization_endpoint": baseURL + "/authorize",
					"token_endpoint":         baseURL + "/token",
					"jwks_uri":               baseURL + "/.well-known/jwks.json",
				})
			case "/.well-known/jwks.json":
				_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
			default:
				http.NotFound(w, r)
			}
		}))
		defer upstream.Close()

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		app, err := NewBuilder().WithConfig(newConfig(jweKey, upstream.URL)).WithStorage(adapter).WithLogger(logger).Build()
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil app")
		}
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = app.Shutdown(shutdownCtx)
		}()

		count, err := adapter.SigningKeys().CountActive(context.Background())
		if err != nil {
			t.Fatalf("CountActive() failed: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected one auto-generated signing key, got %d", count)
		}

		current, err := adapter.SigningKeys().GetCurrent(context.Background())
		if err != nil {
			t.Fatalf("GetCurrent() failed: %v", err)
		}
		if current.ActivatesAt.After(time.Now().Add(time.Second)) {
			t.Fatalf("expected auto-generated signing key to be immediately active, activates_at=%s", current.ActivatesAt)
		}
		if !strings.Contains(logBuf.String(), "auto-generated initial signing key") {
			t.Fatalf("expected auto-generation log entry, got: %s", logBuf.String())
		}
	})
}

func TestBuilder_MissingOAuth2AuthServerConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	adapter, err := storage.NewAdapter(&ports.StorageConfig{
		Backend:  "memory",
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("failed to create storage adapter: %v", err)
	}

	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cfg := &ports.Config{
		Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
		},
		Storage: ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
		Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		// OAuth2AuthServer intentionally absent — must cause startup failure
	}

	_, err = NewBuilder().WithConfig(cfg).WithStorage(adapter).WithLogger(logger).Build()
	if err == nil {
		t.Fatal("Build() with absent OAuth2AuthServerConfig must fail")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("expected error to mention 'mode', got: %v", err)
	}
}

// TestBuilder_SharedUpstreamJWKSAdapter verifies that when both token exchange and
// JWKS publishing are enabled in proxy mode, the builder performs upstream JWKS URI
// discovery exactly once and creates a single shared adapter — not separate instances
// with independent refresh cycles.
func TestBuilder_SharedUpstreamJWKSAdapter(t *testing.T) {
	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	var discoveryCount atomic.Int32
	var jwksFetchCount atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			discoveryCount.Add(1)
			baseURL := "http://" + r.Host
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 baseURL,
				"authorization_endpoint": baseURL + "/authorize",
				"token_endpoint":         baseURL + "/token",
				"jwks_uri":               baseURL + "/.well-known/jwks.json",
			})
		case "/.well-known/jwks.json":
			jwksFetchCount.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	// Override the domain storage HTTP client so discovery calls use our test server.
	origClient := http.DefaultClient
	domstorage.SetHTTPClientForTesting(upstream.Client())
	defer domstorage.SetHTTPClientForTesting(origClient)

	newStorage := func(t *testing.T) *storage.Adapter {
		t.Helper()
		a, err := storage.NewAdapter(&ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		})
		if err != nil {
			t.Fatalf("storage.NewAdapter: %v", err)
		}
		return a
	}

	cfg := &ports.Config{
		Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
		},
		Storage: ports.StorageConfig{
			Backend:  "memory",
			Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
		Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		Security:         ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         upstream.URL,
				UpstreamAuthorizeEndpoint: upstream.URL + "/authorize",
				UpstreamTokenEndpoint:     upstream.URL + "/token",
				UpstreamTimeout:           5 * time.Second,
			},
			MultiAgentClient: ports.MultiAgentClientConfig{
				Enabled:          true,
				AgentIDParamName: "x_agent_id",
				AgentIDClaimName: "agent_id",
			},
		},
		TokenExchange: ports.TokenExchangeConfig{
			ClaimExtraction: ports.ClaimExtractionConfig{
				PrincipalExpression: "subject_token.sub",
				AgentIDExpression:   "subject_token.agent_id",
			},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL: ports.CELAuthorizationConfig{
					Expression:        "true",
					EvaluationTimeout: 100 * time.Millisecond,
				},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	app, err := NewBuilder().
		WithConfig(cfg).
		WithStorage(newStorage(t)).
		WithLogger(logger).
		Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	defer func() { _ = app.Shutdown(context.Background()) }()

	// With token exchange + multi-agent + JWKS publishing all active,
	// discovery should happen exactly once (shared adapter).
	got := discoveryCount.Load()
	if got != 1 {
		t.Errorf("upstream discovery calls = %d, want 1 (shared adapter)", got)
	}

	// JWKS fetch should come from a single httprc controller (one adapter).
	// At build time the adapter may or may not have completed its first fetch,
	// but there should be at most 1 outstanding fetch (not 3 from separate adapters).
	if fetches := jwksFetchCount.Load(); fetches > 1 {
		t.Errorf("initial JWKS fetches = %d, want at most 1 (single shared adapter)", fetches)
	}
}

func TestModeStrategyFor_PanicsOnUnknownMode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected modeStrategyFor to panic on unknown mode, but it did not")
		}
	}()
	modeStrategyFor("bogus")
}

func TestProxyOAuth2ConfigUpstreamTimeout(t *testing.T) {
	tests := []struct {
		configured time.Duration
		want       time.Duration
	}{
		{0, 30 * time.Second},                            // zero → application default
		{500 * time.Millisecond, 500 * time.Millisecond}, // sub-second timeout
		{5 * time.Second, 5 * time.Second},               // explicit proxy config
		{60 * time.Second, 60 * time.Second},             // custom timeout
	}
	for _, tt := range tests {
		cfg := ports.OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
				UpstreamTimeout:           tt.configured,
			},
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		p := resolved.(*ports.ProxyOAuth2Config)
		if p.UpstreamTimeout != tt.want {
			t.Errorf("UpstreamTimeout with configured=%v = %v, want %v", tt.configured, p.UpstreamTimeout, tt.want)
		}
	}
}
