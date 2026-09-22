package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"

	"github.com/agentic-identity-broker/mock-upstream-oauth2-service/internal/config"
	"github.com/agentic-identity-broker/mock-upstream-oauth2-service/internal/handlers"
	"github.com/agentic-identity-broker/mock-upstream-oauth2-service/internal/jwt"
)

// bufferingResponseWriter buffers response without writing until explicitly flushed
type bufferingResponseWriter struct {
	underlying http.ResponseWriter
	statusCode int
	header     http.Header
	body       *bytes.Buffer
	written    bool
}

func newBufferingResponseWriter(w http.ResponseWriter) *bufferingResponseWriter {
	return &bufferingResponseWriter{
		underlying: w,
		statusCode: http.StatusOK,
		header:     http.Header{},
		body:       &bytes.Buffer{},
	}
}

func (w *bufferingResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.written = true
}

func (w *bufferingResponseWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.body.Write(b)
}

func (w *bufferingResponseWriter) Flush() error {
	// Write headers — assign slices directly to avoid duplicating headers
	// that may already be set on the underlying writer (e.g. by middleware).
	for k, vv := range w.header {
		w.underlying.Header()[k] = vv
	}
	// Write status code and body
	w.underlying.WriteHeader(w.statusCode)
	_, err := w.underlying.Write(w.body.Bytes())
	return err
}

// Server holds the OAuth2 server and router
type Server struct {
	router     *http.ServeMux
	oauth2Srv  *server.Server
	cfg        *config.Config
	httpServer *http.Server
	privateKey *rsa.PrivateKey
	keyID      string
}

// New creates and configures a new OAuth2 server
func New(cfg *config.Config) (*Server, error) {
	// Create manager
	manager := manage.NewDefaultManager()

	// Configure token expiration times
	manager.SetAuthorizeCodeTokenCfg(&manage.Config{
		AccessTokenExp:    cfg.OAuth2.AccessTokenTTL,
		RefreshTokenExp:   cfg.OAuth2.RefreshTokenTTL,
		IsGenerateRefresh: true,
	})

	// Create client store and add the mock client
	clientStore := store.NewClientStore()
	client := &models.Client{
		ID:     cfg.OAuth2.ClientID,
		Secret: cfg.OAuth2.ClientSecret,
		// Domain field is used by go-oauth2/oauth2 v4 for redirect URI validation
		// Setting it to localhost allows redirects to localhost:* ports
		Domain: "localhost",
	}
	if err := clientStore.Set(cfg.OAuth2.ClientID, client); err != nil {
		return nil, fmt.Errorf("failed to store client: %w", err)
	}

	slog.Info("Upstream OAuth2 client configured",
		"client_id", cfg.OAuth2.ClientID,
		"access_token_ttl", cfg.OAuth2.AccessTokenTTL,
		"refresh_token_ttl", cfg.OAuth2.RefreshTokenTTL)

	// Also register ExtProc client for token exchange service (Phase 7)
	extprocClient := &models.Client{ // #nosec G101 -- fixed development-only mock client credentials.
		ID:     "extproc-gateway",
		Secret: "extproc-dev-secret",
		Domain: "localhost",
	}
	if err := clientStore.Set("extproc-gateway", extprocClient); err != nil {
		return nil, fmt.Errorf("failed to store extproc client: %w", err)
	}
	slog.Info("ExtProc client registered",
		"client_id", "extproc-gateway")

	// Set client store
	manager.MapClientStorage(clientStore)

	// Create and set token store (in-memory for testing)
	// This is required for storing authorization codes and access tokens
	tokenStore, err := store.NewMemoryTokenStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create token store: %w", err)
	}
	manager.MapTokenStorage(tokenStore)

	// Generate RSA key pair for RS256 JWT signing
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}
	const keyID = "upstream-oauth2-key-1"

	// Set RS256 JWT access generator with proper iss/aud claims for token exchange validation
	manager.MapAccessGenerate(jwt.NewRSAAccessGenerator(privateKey, "http://upstream-oauth2:9001", "token-exchange-broker", keyID))

	// Set client authenticator for client credentials grant
	// This validates client_id and client_secret for client credentials flow
	manager.SetClientTokenCfg(&manage.Config{
		AccessTokenExp: cfg.OAuth2.AccessTokenTTL,
	})

	// Create and configure OAuth2 server
	oauth2Srv := server.NewDefaultServer(manager)

	// Configure allowed response and grant types
	oauth2Srv.SetAllowedResponseType(oauth2.Code)
	oauth2Srv.SetAllowGetAccessRequest(false)
	oauth2Srv.SetAllowedGrantType(oauth2.AuthorizationCode, oauth2.Refreshing, oauth2.ClientCredentials)
	oauth2Srv.SetClientInfoHandler(server.ClientFormHandler)

	// PKCE is enabled by default in v4

	// Set the user authorization handler (consent screen)
	oauth2Srv.SetUserAuthorizationHandler(handlers.NewUserAuthorizationHandler(cfg))

	// Create router and partial Server struct so method handlers can reference s.
	router := http.NewServeMux()

	s := &Server{
		router:     router,
		oauth2Srv:  oauth2Srv,
		cfg:        cfg,
		privateKey: privateKey,
		keyID:      keyID,
	}

	// Register handlers
	router.HandleFunc("/health", handlers.Health)
	router.HandleFunc("/.well-known/oauth-authorization-server", s.handleDiscovery)
	router.HandleFunc("/.well-known/jwks.json", s.handleJWKS)
	router.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		// Handle authorize requests with error recovery
		// The oauth2 library may panic or return errors that need logging
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic in authorize handler", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		if err := oauth2Srv.HandleAuthorizeRequest(w, r); err != nil {
			slog.Error("authorize request error", "error", err, "url", r.URL.String())
			// Don't override the response if already written
			if w.Header().Get("Content-Type") == "" {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
		}
	})
	router.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		// Log token request details for debugging
		_ = r.ParseForm()

		// Parse Authorization header if present
		authHeader := r.Header.Get("Authorization")
		basicAuthUser := ""
		basicAuthSecret := ""
		if authHeader != "" && strings.HasPrefix(authHeader, "Basic ") {
			encoded := authHeader[6:] // Remove "Basic " prefix
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
				parts := strings.Split(string(decoded), ":")
				if len(parts) >= 1 {
					basicAuthUser = parts[0]
				}
				if len(parts) >= 2 {
					basicAuthSecret = parts[1]
				}
			}
		}

		// Extract all request parameters for PKCE debugging
		code := r.Form.Get("code")
		verifier := r.Form.Get("code_verifier")
		redirectURI := r.Form.Get("redirect_uri")
		formClientID := r.Form.Get("client_id")
		formClientSecret := r.Form.Get("client_secret")
		grantType := r.Form.Get("grant_type")

		slog.Info("token request full details",
			"client_id_form", formClientID,
			"client_secret_form", formClientSecret,
			"client_id_basic_auth", basicAuthUser,
			"client_secret_basic_auth", basicAuthSecret,
			"grant_type", grantType,
			"code", code,
			"code_length", len(code),
			"code_verifier", verifier,
			"verifier_length", len(verifier),
			"redirect_uri", redirectURI,
			"auth_header_present", authHeader != "",
			"configured_client_id", cfg.OAuth2.ClientID,
			"configured_client_secret", cfg.OAuth2.ClientSecret)

		// Wrap response writer to buffer output
		captureWriter := newBufferingResponseWriter(w)

		if err := oauth2Srv.HandleTokenRequest(captureWriter, r); err != nil {
			slog.Error("token request handler error",
				"error", err.Error(),
				"error_type", fmt.Sprintf("%T", err),
				"client_id_form", formClientID,
				"client_id_basic_auth", basicAuthUser)
			if captureWriter.written {
				// The oauth2 library already wrote an error response — flush it.
				if flushErr := captureWriter.Flush(); flushErr != nil {
					slog.Error("error flushing error response", "error", flushErr)
				}
			} else {
				http.Error(w, "invalid token request", http.StatusBadRequest)
			}
			return
		}

		// Log what was written to response
		if captureWriter.body.Len() > 0 {
			slog.Info("token response written",
				"body_length", captureWriter.body.Len(),
				"status", captureWriter.statusCode,
				"body", captureWriter.body.String())
		}

		// Flush buffered response to actual http.ResponseWriter
		if err := captureWriter.Flush(); err != nil {
			slog.Error("error flushing response", "error", err)
		}
	})

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Bind, cfg.Server.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// handleDiscovery serves the RFC 8414 OAuth 2.0 Authorization Server Metadata document.
// The broker discovers the jwks_uri from this endpoint at startup.
func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	issuer := fmt.Sprintf("http://%s:%d", "upstream-oauth2", s.cfg.Server.Port)

	metadata := map[string]interface{}{
		"issuer":                 issuer,
		"authorization_endpoint": issuer + "/oauth/authorize",
		"token_endpoint":         issuer + "/oauth/token",
		"jwks_uri":               issuer + "/.well-known/jwks.json",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(metadata); err != nil {
		slog.Error("failed to encode discovery response", "error", err)
	}
}

// handleJWKS serves the RSA public key as a JSON Web Key Set so that token consumers
// can verify RS256-signed JWTs without a shared secret.
func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	pub := &s.privateKey.PublicKey

	nEncoded := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	eEncoded := base64.RawURLEncoding.EncodeToString(eBytes)

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": s.keyID,
				"n":   nEncoded,
				"e":   eEncoded,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jwks); err != nil {
		slog.Error("failed to encode JWKS response", "error", err)
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("http://%s:%d", s.cfg.Server.Bind, s.cfg.Server.Port)
	slog.Info("Starting Upstream OAuth2 mock server", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// GetOAuth2Server returns the underlying OAuth2 server
func (s *Server) GetOAuth2Server() *server.Server {
	return s.oauth2Srv
}

// GetConfig returns the server configuration
func (s *Server) GetConfig() *config.Config {
	return s.cfg
}
