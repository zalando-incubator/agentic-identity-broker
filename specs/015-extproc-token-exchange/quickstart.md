# Quickstart: Implementing ExtProc Token Exchange Service

This guide provides step-by-step instructions for implementing the ExtProc token exchange service following a phased approach that ensures E2E tests compile at each stage.
> **Implementation note (superseded raw input):**
> [Feature 043](../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Prerequisites

- Go 1.24.0+
- Docker and Docker Compose
- Understanding of [Envoy ExtProc protocol](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto)
- Understanding of [RFC 8693 Token Exchange](https://datatracker.ietf.org/doc/html/rfc8693)
- Familiarity with [agentgateway ExtProc configuration](https://agentgateway.dev/docs/standalone/latest/configuration/traffic-management/extproc)

## Phase Overview

| Phase | Focus | Tests Status |
|-------|-------|--------------|
| 0 | Project scaffolding & dependencies | Compiles |
| 1 | Configuration & validation | Compiles, config tests pass |
| 2 | E2E test skeletons | Compiles, all failing (red) |
| 3 | ExtProc gRPC server core | Compiles, pass-through tests pass |
| 4 | Token exchange client | Compiles, exchange tests pass |
| 5 | Token caching | Compiles, cache tests pass |
| 6 | Docker compose & mocks | Full integration passes |
| 7 | Sample agent MCP button | End-to-end demo works |

---

## Phase 0: Project Scaffolding

### Step 0.1: Create cmd entry point

Create `cmd/extproc-token-exchange/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

Create `cmd/extproc-token-exchange/root.go` with Cobra root command wiring:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

var rootCmd = &cobra.Command{
	Use:   "extproc-token-exchange",
	Short: "Envoy ExtProc service for transparent OAuth2 token exchange",
	RunE:  run,
}

func Execute() error {
	return rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) error {
	// 1. Load and validate configuration
	cfg, err := extprocconfig.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// 2. Initialize logger
	logger := initLogger(cfg.Log)

	// 3. Create ExtProc server
	extprocServer, err := server.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating extproc server: %w", err)
	}

	// 4. Create gRPC server
	grpcServer := grpc.NewServer(grpc.MaxConcurrentStreams(uint32(cfg.GRPC.MaxConcurrentStreams)))
	extprocServer.Register(grpcServer)
	grpc_health_v1.RegisterHealthServer(grpcServer, extprocServer.HealthServer())

	// 5. Listen
	addr := fmt.Sprintf("%s:%d", cfg.GRPC.Bind, cfg.GRPC.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	logger.Info("starting ExtProc token exchange service",
		"bind", cfg.GRPC.Bind,
		"port", cfg.GRPC.Port,
		"token_endpoint", cfg.OAuth2.TokenEndpoint,
		"issuer", cfg.OAuth2.Issuer,
	)

	// 6. Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down gRPC server")
		grpcServer.GracefulStop()
		cancel()
	}()

	_ = ctx // used for shutdown coordination
	return grpcServer.Serve(lis)
}

func initLogger(cfg extprocconfig.LogConfig) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{}

	switch cfg.Level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
```

### Step 0.2: Add gRPC dependencies

```bash
go get google.golang.org/grpc
go get github.com/envoyproxy/go-control-plane
go get google.golang.org/grpc/health/grpc_health_v1
```

---

## Phase 1: Configuration

### Step 1.1: Create config types

Create `internal/extproc/config/config.go`:

```go
package config

import "time"

// Config is the root configuration for the ExtProc token exchange service.
type Config struct {
	GRPC  GRPCConfig  `yaml:"grpc"`
	OAuth2 OAuth2Config `yaml:"oauth2"`
	Cache CacheConfig  `yaml:"cache"`
	Log   LogConfig    `yaml:"log"`
}

// GRPCConfig configures the gRPC server.
type GRPCConfig struct {
	Bind string `yaml:"bind"`
	Port int    `yaml:"port"`
}

// OAuth2Config configures OAuth2 and token exchange endpoints.
type OAuth2Config struct {
	TokenEndpoint string `yaml:"token_endpoint"`
	Issuer        string `yaml:"issuer"`
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
}

// CacheConfig configures the token cache.
type CacheConfig struct {
	DefaultTTL time.Duration `yaml:"default_ttl"`
}

// LogConfig configures logging.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}
```

### Step 1.2: Create config loader

Create `internal/extproc/config/loader.go` using Viper/Cobra with `EXTPROC_` env prefix:

```go
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from file, environment variables, and defaults.
func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("grpc.bind", "0.0.0.0")
	v.SetDefault("grpc.port", 50051)
	v.SetDefault("cache.default_ttl", "5m")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")

	// Environment
	v.SetEnvPrefix("EXTPROC")
	v.AutomaticEnv()

	// Config file
	configPath := os.Getenv("EXTPROC_CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		// Config file is optional if all required values come from env
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading config file: %w", err)
			}
		}
	}

	// Parse duration manually since Viper doesn't handle durations well
	cfg := &Config{}
	cfg.GRPC.Bind = v.GetString("grpc.bind")
	cfg.GRPC.Port = v.GetInt("grpc.port")
	cfg.OAuth2.TokenEndpoint = v.GetString("oauth2.token_endpoint")
	cfg.OAuth2.Issuer = v.GetString("oauth2.issuer")
	cfg.OAuth2.ClientID = v.GetString("oauth2.client_id")
	cfg.OAuth2.ClientSecret = expandEnvVar(v.GetString("oauth2.client_secret"))
	cfg.Log.Level = v.GetString("log.level")
	cfg.Log.Format = v.GetString("log.format")

	ttlStr := v.GetString("cache.default_ttl")
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return nil, fmt.Errorf("parsing cache.default_ttl %q: %w", ttlStr, err)
	}
	cfg.Cache.DefaultTTL = ttl

	// Validate
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.GRPC.Port < 1 || cfg.GRPC.Port > 65535 {
		return fmt.Errorf("grpc.port must be between 1 and 65535, got %d", cfg.GRPC.Port)
	}
	if cfg.GRPC.Bind == "" {
		return fmt.Errorf("grpc.bind must not be empty")
	}
	if cfg.OAuth2.TokenEndpoint == "" {
		return fmt.Errorf("oauth2.token_endpoint must not be empty")
	}
	if _, err := url.ParseRequestURI(cfg.OAuth2.TokenEndpoint); err != nil {
		return fmt.Errorf("oauth2.token_endpoint must be a valid URL: %w", err)
	}
	if cfg.OAuth2.Issuer == "" {
		return fmt.Errorf("oauth2.issuer must not be empty")
	}
	if _, err := url.ParseRequestURI(cfg.OAuth2.Issuer); err != nil {
		return fmt.Errorf("oauth2.issuer must be a valid URL: %w", err)
	}
	if cfg.OAuth2.ClientID == "" {
		return fmt.Errorf("oauth2.client_id must not be empty")
	}
	if cfg.OAuth2.ClientSecret == "" {
		return fmt.Errorf("oauth2.client_secret must not be empty")
	}
	if cfg.Cache.DefaultTTL <= 0 {
		return fmt.Errorf("cache.default_ttl must be positive")
	}
	return nil
}

// expandEnvVar resolves ${VAR} patterns in a string.
func expandEnvVar(s string) string {
	return os.ExpandEnv(s)
}
```

---

## Phase 2: E2E Test Skeletons

### Step 2.1: Create separate E2E test suite

Create `tests/e2e/extproc/extproc_suite_test.go`:

```go
package extproc_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExtProcTokenExchange(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ExtProc Token Exchange E2E Suite")
}
```

### Step 2.2: Create test scenarios

Create `tests/e2e/extproc/token_exchange_test.go`:

```go
package extproc_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtProc Token Exchange", func() {
	// User Story 1: Transparent Token Exchange
	Describe("when processing request with Bearer token", func() {
		// Scenario 1.1 from specs/015-extproc-token-exchange/spec.md (User Story 1)
		It("should exchange the Bearer token using request URI as resource", func() {
			// Send request with Authorization: Bearer <token>
			// Verify ExtProc replaces the header with the exchanged token
		})

		// Scenario 1.2 from specs/015-extproc-token-exchange/spec.md (User Story 1)
		It("should replace the Authorization header with the exchanged token", func() {
			// Verify the outgoing Authorization header contains the new token
		})

		// Scenario 1.3 from specs/015-extproc-token-exchange/spec.md (User Story 1)
		It("should reject with failure response when token exchange fails", func() {
			// Verify ExtProc returns ImmediateResponse with error
		})
	})

	// User Story 2: Token Cache
	Describe("when processing repeated requests", func() {
		// Scenario 2.1 from specs/015-extproc-token-exchange/spec.md (User Story 2)
		It("should use cached token for same subject token and resource", func() {
			// Verify no new token exchange call for cached token
		})

		// Scenario 2.2 from specs/015-extproc-token-exchange/spec.md (User Story 2)
		It("should perform fresh exchange when cached token is expired", func() {
			// Verify new exchange after cache expiry
		})
	})

	// User Story 3: Configuration & Startup
	Describe("when starting the service", func() {
		// Scenario 3.1 from specs/015-extproc-token-exchange/spec.md (User Story 3)
		It("should bind to configured host/port and log startup summary", func() {
			// Verify gRPC server starts and logs correctly
		})

		// Scenario 3.2 from specs/015-extproc-token-exchange/spec.md (User Story 3)
		It("should exit with validation error on invalid configuration", func() {
			// Verify startup fails with clear error
		})
	})

	// Edge Cases
	Describe("when Authorization header is missing or not Bearer", func() {
		It("should pass through the request unchanged", func() {
			// Verify no modification when no Bearer token present
		})
	})

	Describe("when request URI is empty or invalid", func() {
		It("should reject with 503 response", func() {
			// Verify ImmediateResponse with 503
		})
	})

	Describe("when token exchange times out", func() {
		It("should return 500 response and log the failure", func() {
			// Verify ImmediateResponse with 500 and log entry
		})
	})

	Describe("when exchanged token has no expiry", func() {
		It("should use the default cache TTL", func() {
			// Verify token cached with configured default_ttl
		})
	})

	Describe("when multiple concurrent requests refresh the same expired token", func() {
		It("should perform only one token exchange via singleflight", func() {
			// Verify singleflight deduplication
		})
	})
})
```

---

## Phase 3: ExtProc gRPC Server Core

### Step 3.1: Create ExtProc server

Create `internal/extproc/server/server.go`:

```go
package server

import (
	"io"
	"log/slog"
	"strings"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Server implements the Envoy ExtProc gRPC service.
type Server struct {
	config    *extprocconfig.Config
	logger    *slog.Logger
	exchanger *TokenExchanger
}

// New creates a new ExtProc server.
func New(cfg *extprocconfig.Config, logger *slog.Logger) (*Server, error) {
	exchanger, err := NewTokenExchanger(cfg, logger)
	if err != nil {
		return nil, err
	}
	return &Server{
		config:    cfg,
		logger:    logger,
		exchanger: exchanger,
	}, nil
}

// Register registers the ExtProc server with a gRPC server.
func (s *Server) Register(grpcServer *grpc.Server) {
	ext_proc_v3.RegisterExternalProcessorServer(grpcServer, s)
}

// HealthServer returns a gRPC health server.
func (s *Server) HealthServer() grpc_health_v1.HealthServer {
	return &healthServer{}
}

// Process implements the ExtProc streaming RPC.
func (s *Server) Process(srv ext_proc_v3.ExternalProcessor_ProcessServer) error {
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "receive error: %v", err)
		}

		resp := &ext_proc_v3.ProcessingResponse{}

		switch v := req.Request.(type) {
		case *ext_proc_v3.ProcessingRequest_RequestHeaders:
			resp = s.processRequestHeaders(srv.Context(), v.RequestHeaders)

		case *ext_proc_v3.ProcessingRequest_RequestBody:
			// Pass through body unchanged (streaming mode)
			resp = &ext_proc_v3.ProcessingResponse{
				Response: &ext_proc_v3.ProcessingResponse_RequestBody{
					RequestBody: &ext_proc_v3.BodyResponse{
						Response: &ext_proc_v3.CommonResponse{
							BodyMutation: &ext_proc_v3.BodyMutation{
								Mutation: &ext_proc_v3.BodyMutation_StreamedResponse{
									StreamedResponse: &ext_proc_v3.StreamedBodyResponse{
										Body:        v.RequestBody.Body,
										EndOfStream: v.RequestBody.EndOfStream,
									},
								},
							},
						},
					},
				},
			}

		case *ext_proc_v3.ProcessingRequest_ResponseHeaders:
			resp.Response = &ext_proc_v3.ProcessingResponse_ResponseHeaders{}

		case *ext_proc_v3.ProcessingRequest_ResponseBody:
			resp = &ext_proc_v3.ProcessingResponse{
				Response: &ext_proc_v3.ProcessingResponse_ResponseBody{
					ResponseBody: &ext_proc_v3.BodyResponse{
						Response: &ext_proc_v3.CommonResponse{
							BodyMutation: &ext_proc_v3.BodyMutation{
								Mutation: &ext_proc_v3.BodyMutation_StreamedResponse{
									StreamedResponse: &ext_proc_v3.StreamedBodyResponse{
										Body:        v.ResponseBody.Body,
										EndOfStream: v.ResponseBody.EndOfStream,
									},
								},
							},
						},
					},
				},
			}

		case *ext_proc_v3.ProcessingRequest_RequestTrailers:
			resp.Response = &ext_proc_v3.ProcessingResponse_RequestTrailers{}

		case *ext_proc_v3.ProcessingRequest_ResponseTrailers:
			resp.Response = &ext_proc_v3.ProcessingResponse_ResponseTrailers{}

		default:
			s.logger.Warn("unknown request type")
		}

		if err := srv.Send(resp); err != nil {
			return err
		}
	}
}

// processRequestHeaders handles the token exchange logic.
func (s *Server) processRequestHeaders(ctx context.Context, headers *ext_proc_v3.HttpHeaders) *ext_proc_v3.ProcessingResponse {
	// Extract Authorization header
	bearerToken := extractBearerToken(headers)
	if bearerToken == "" {
		// No Bearer token — pass through unchanged
		return &ext_proc_v3.ProcessingResponse{
			Response: &ext_proc_v3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &ext_proc_v3.HeadersResponse{},
			},
		}
	}

	// Extract request URI
	requestURI := extractHeader(headers, ":path")
	if requestURI == "" {
		return immediateResponse(503, "invalid_resource", "request URI is empty or invalid")
	}

	// Perform token exchange (with caching and singleflight)
	exchangedToken, err := s.exchanger.Exchange(ctx, bearerToken, requestURI)
	if err != nil {
		s.logger.Error("token exchange failed", "error", err, "resource", requestURI)
		return immediateResponse(500, "token_exchange_failed", "token exchange request failed")
	}

	// Replace Authorization header
	return &ext_proc_v3.ProcessingResponse{
		Response: &ext_proc_v3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &ext_proc_v3.HeadersResponse{
				Response: &ext_proc_v3.CommonResponse{
					HeaderMutation: &ext_proc_v3.HeaderMutation{
						SetHeaders: []*core_v3.HeaderValueOption{
							{
								Header: &core_v3.HeaderValue{
									Key:      "authorization",
									RawValue: []byte("Bearer " + exchangedToken),
								},
							},
						},
					},
				},
			},
		},
	}
}

// Helper functions

func extractBearerToken(headers *ext_proc_v3.HttpHeaders) string {
	for _, h := range headers.Headers.Headers {
		if strings.EqualFold(h.Key, "authorization") {
			val := string(h.RawValue)
			if strings.HasPrefix(val, "Bearer ") || strings.HasPrefix(val, "bearer ") {
				return strings.TrimPrefix(strings.TrimPrefix(val, "Bearer "), "bearer ")
			}
		}
	}
	return ""
}

func extractHeader(headers *ext_proc_v3.HttpHeaders, key string) string {
	for _, h := range headers.Headers.Headers {
		if h.Key == key {
			return string(h.RawValue)
		}
	}
	return ""
}

func immediateResponse(statusCode int, errorCode, description string) *ext_proc_v3.ProcessingResponse {
	body := fmt.Sprintf(`{"error":"%s","error_description":"%s"}`, errorCode, description)
	return &ext_proc_v3.ProcessingResponse{
		Response: &ext_proc_v3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &ext_proc_v3.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode(statusCode),
				},
				Headers: &ext_proc_v3.HeaderMutation{
					SetHeaders: []*core_v3.HeaderValueOption{
						{
							Header: &core_v3.HeaderValue{
								Key:      "content-type",
								RawValue: []byte("application/json"),
							},
						},
					},
				},
				Body: body,
			},
		},
	}
}
```

### Step 3.2: Create token exchanger

Create `internal/extproc/server/exchanger.go`:

```go
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"golang.org/x/sync/singleflight"
)

// TokenExchanger handles token exchange with caching.
type TokenExchanger struct {
	config         *extprocconfig.Config
	logger         *slog.Logger
	httpClient     *http.Client
	cache          map[string]*cachedToken
	cacheMu        sync.RWMutex
	singleflight   singleflight.Group
	clientAssertion *clientAssertionEntry
	assertionMu    sync.RWMutex
	stopCh         chan struct{}
}

type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

type clientAssertionEntry struct {
	token     string
	expiresAt time.Time
}

type tokenExchangeResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int64  `json:"expires_in"`
}

// NewTokenExchanger creates a new TokenExchanger and acquires the initial
// client assertion at startup. If the initial acquisition fails, the service
// fails to start (fail-fast). A background goroutine refreshes the assertion
// before it expires.
func NewTokenExchanger(cfg *extprocconfig.Config, logger *slog.Logger) (*TokenExchanger, error) {
	te := &TokenExchanger{
		config:     cfg,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]*cachedToken),
		stopCh:     make(chan struct{}),
	}

	// Acquire initial client assertion at startup (fail-fast)
	if err := te.refreshClientAssertion(context.Background()); err != nil {
		return nil, fmt.Errorf("initial client assertion acquisition failed: %w", err)
	}

	// Start background refresh goroutine
	go te.backgroundRefreshLoop()

	return te, nil
}

// Exchange performs a token exchange, using cache when available.
func (te *TokenExchanger) Exchange(ctx context.Context, subjectToken, resourceURI string) (string, error) {
	// Check cache first
	cacheKey := te.cacheKey(subjectToken, resourceURI)

	te.cacheMu.RLock()
	if entry, ok := te.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		te.cacheMu.RUnlock()
		te.logger.Debug("cache hit", "resource", resourceURI)
		return entry.accessToken, nil
	}
	te.cacheMu.RUnlock()

	// Singleflight: one exchange per key
	result, err, _ := te.singleflight.Do(cacheKey, func() (interface{}, error) {
		return te.doExchange(ctx, subjectToken, resourceURI)
	})
	if err != nil {
		return "", err
	}

	token := result.(string)
	return token, nil
}

func (te *TokenExchanger) doExchange(ctx context.Context, subjectToken, resourceURI string) (string, error) {
	// 1. Read cached client assertion (always available; refreshed in background)
	te.assertionMu.RLock()
	clientAssertion := te.clientAssertion.token
	te.assertionMu.RUnlock()

	// 2. Perform RFC 8693 token exchange
	data := url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {subjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"resource":              {resourceURI},
		"client_assertion":      {clientAssertion},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, te.config.OAuth2.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := te.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenExchangeResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing token exchange response: %w", err)
	}

	// 3. Cache the result
	ttl := te.config.Cache.DefaultTTL
	if tokenResp.ExpiresIn > 0 {
		ttl = time.Duration(tokenResp.ExpiresIn) * time.Second
	}

	cacheKey := te.cacheKey(subjectToken, resourceURI)
	te.cacheMu.Lock()
	te.cache[cacheKey] = &cachedToken{
		accessToken: tokenResp.AccessToken,
		expiresAt:   time.Now().Add(ttl),
	}
	te.cacheMu.Unlock()

	te.logger.Info("token exchanged successfully",
		"resource", resourceURI,
		"expires_in", ttl.String(),
	)

	return tokenResp.AccessToken, nil
}

func (te *TokenExchanger) getClientAssertion(ctx context.Context) (string, error) {
	// Read the cached assertion (always available after startup)
	te.assertionMu.RLock()
	defer te.assertionMu.RUnlock()
	if te.clientAssertion != nil {
		return te.clientAssertion.token, nil
	}
	return "", fmt.Errorf("client assertion not available")
}

// refreshClientAssertion obtains a new client assertion via client_credentials grant.
func (te *TokenExchanger) refreshClientAssertion(ctx context.Context) error {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {te.config.OAuth2.ClientID},
		"client_secret": {te.config.OAuth2.ClientSecret},
		"scope":         {"openid"},
	}

	tokenURL := te.config.OAuth2.Issuer + "/oauth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating client credentials request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := te.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("client credentials request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("client credentials returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing client credentials response: %w", err)
	}

	// Cache the assertion (expire 30s before actual expiry to avoid edge cases)
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - 30*time.Second)

	te.assertionMu.Lock()
	te.clientAssertion = &clientAssertionEntry{
		token:     tokenResp.AccessToken,
		expiresAt: expiresAt,
	}
	te.assertionMu.Unlock()

	te.logger.Info("client assertion refreshed", "expires_at", expiresAt)
	return nil
}

// backgroundRefreshLoop refreshes the client assertion at 80% of TTL.
func (te *TokenExchanger) backgroundRefreshLoop() {
	for {
		te.assertionMu.RLock()
		expiresAt := te.clientAssertion.expiresAt
		te.assertionMu.RUnlock()

		// Refresh at 80% of remaining TTL
		remaining := time.Until(expiresAt)
		refreshIn := time.Duration(float64(remaining) * 0.8)
		if refreshIn < 10*time.Second {
			refreshIn = 10 * time.Second
		}

		select {
		case <-time.After(refreshIn):
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := te.refreshClientAssertion(ctx); err != nil {
				te.logger.Error("background client assertion refresh failed", "error", err)
				// Retry sooner on failure
				cancel()
				time.Sleep(5 * time.Second)
				continue
			}
			cancel()
		case <-te.stopCh:
			return
		}
	}
}

// Stop signals the background refresh goroutine to stop.
func (te *TokenExchanger) Stop() {
	close(te.stopCh)
}

func (te *TokenExchanger) cacheKey(subjectToken, resourceURI string) string {
	h := sha256.Sum256([]byte(subjectToken + "|" + resourceURI))
	return hex.EncodeToString(h[:])
}
```

---

## Phase 4: MCP Server Mock

### Step 4.1: Create MCP server mock

Create `mocks/mcp-server/cmd/mcp-server/main.go`:

A simple Go HTTP server that:
1. Listens on port 9003
2. Implements MCP Streamable HTTP transport at `/mcp`
3. Handles `initialize`, `tools/list`, and `tools/call` JSON-RPC methods
4. The `show_claims` tool decodes the Bearer token's JWT payload (base64) and returns the claims as JSON
5. Returns claims of the exchanged token to prove token exchange worked

### Step 4.2: Create mock configuration

Create `mocks/mcp-server/config.yaml`:

```yaml
server:
  port: 9003
  bind: "0.0.0.0"
```

---

## Phase 5: Docker Compose Integration

### Step 5.1: Add three new services to docker-compose.yml

Add `agentgateway`, `extproc-token-exchange`, and `mcp-server-mock` services.

### Step 5.2: Create agentgateway configuration

Create `mocks/agentgateway/config.yaml` with MCP backend and ExtProc policy.

### Step 5.3: Create ExtProc docker configuration

Create `configs/config.extproc.docker.yaml` in the project `configs/` directory:

```yaml
grpc:
  bind: "0.0.0.0"
  port: 50051
oauth2:
  token_endpoint: "http://identity-broker:8000/oauth2/token"
  issuer: "http://upstream-oauth2:9001"
  client_id: "extproc-gateway"
  client_secret: "${EXTPROC_CLIENT_SECRET}"
cache:
  default_ttl: "5m"
log:
  level: "debug"
  format: "text"
```

---

## Phase 6: Sample Agent MCP Button

### Step 6.1: Add MCP client handler to sample agent

Add a new route `/call-mcp` to the sample agent that:
1. Reads the user's access token from the session
2. Sends a JSON-RPC `initialize` request to agentgateway at `http://agentgateway:4000/mcp`
3. Sends a `tools/call` request for `tools_show_claims` tool
4. Renders the returned claims on the page

### Step 6.2: Add button to home page

Add a "Call MCP Tool (Token Exchange)" button to the sample agent's user info page that triggers `/call-mcp`.

---

## Phase 7: E2E Test Implementation

### Step 7.1: Implement E2E test helpers

Create in `tests/e2e/extproc/`:
- `bootstrap/` — ExtProc server setup, mock token exchange endpoint
- `helpers/` — gRPC client helpers, mock OAuth2 server
- `fixtures/` — test tokens, config objects

### Step 7.2: Complete test scenarios

Remove placeholder comments and implement actual assertions for all scenarios.

### Step 7.3: Run tests

```bash
# Run ExtProc E2E tests
cd tests/e2e/extproc && ginkgo -v ./...

# Or add justfile target
just test-e2e-extproc
```

---

## Configuration Example

```yaml
# examples/config/extproc-token-exchange.yaml
grpc:
  bind: "0.0.0.0"
  port: 50051

oauth2:
  token_endpoint: "https://identity-broker.example.com/oauth2/token"
  issuer: "https://auth.example.com"
  client_id: "extproc-gateway"
  client_secret: "${EXTPROC_CLIENT_SECRET}"

cache:
  default_ttl: "5m"

log:
  level: "info"
  format: "json"
```

---

## Troubleshooting

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| gRPC connection refused | ExtProc not started or wrong port | Check `grpc.port` config, verify container is running |
| 500 from ExtProc | Token exchange endpoint unreachable | Check `oauth2.token_endpoint`, verify identity-broker is healthy |
| 503 from ExtProc | Request URI empty/invalid | Check agentgateway routes and backend configuration |
| Stale tokens | Cache not expiring | Verify `cache.default_ttl` and check system clock |
| Client assertion failure | Wrong client_id/secret | Verify credentials match upstream OAuth2 server config |
| agentgateway ExtProc timeout | ExtProc service too slow | Check token exchange endpoint latency |
