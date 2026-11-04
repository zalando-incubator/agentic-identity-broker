package server

import (
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

	"github.com/agentic-identity-broker/mock-oauth2-service/internal/config"
	"github.com/agentic-identity-broker/mock-oauth2-service/internal/handlers"
	"github.com/agentic-identity-broker/mock-oauth2-service/internal/jwt"
)

// responseCapture wraps http.ResponseWriter to capture what's written
type responseCapture struct {
	http.ResponseWriter
	statusCode int
	body       []byte
	written    bool
}

func (r *responseCapture) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.written = true
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	r.written = true
	return r.ResponseWriter.Write(b)
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

	slog.Info("Mock OAuth2 client configured",
		"client_id", cfg.OAuth2.ClientID,
		"access_token_ttl", cfg.OAuth2.AccessTokenTTL,
		"refresh_token_ttl", cfg.OAuth2.RefreshTokenTTL)

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
	const keyID = "third-party-oauth2-key-1"

	// Issue RS256 JWT access tokens so downstream consumers (e.g. MCP server) can inspect claims.
	manager.MapAccessGenerate(jwt.NewRSAAccessGenerator(privateKey, "http://third-party-oauth2:9000", "mcp-server", keyID))

	// Create and configure OAuth2 server
	oauth2Srv := server.NewDefaultServer(manager)

	// Configure allowed response and grant types
	oauth2Srv.SetAllowedResponseType(oauth2.Code)
	oauth2Srv.SetAllowGetAccessRequest(false)
	oauth2Srv.SetAllowedGrantType(oauth2.AuthorizationCode, oauth2.Refreshing)
	// Accept client_secret_basic and client_secret_post; the broker refreshes with form credentials
	oauth2Srv.SetClientInfoHandler(func(r *http.Request) (string, string, error) {
		if clientID, clientSecret, err := server.ClientBasicHandler(r); err == nil {
			return clientID, clientSecret, nil
		}
		return server.ClientFormHandler(r)
	})

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

		slog.Info("token request full details",
			"client_id_form", formClientID,
			"client_secret_form", formClientSecret,
			"client_id_basic_auth", basicAuthUser,
			"client_secret_basic_auth", basicAuthSecret,
			"grant_type", r.Form.Get("grant_type"),
			"code", code,
			"code_length", len(code),
			"code_verifier", verifier,
			"verifier_length", len(verifier),
			"redirect_uri", redirectURI,
			"auth_header_present", authHeader != "",
			"configured_client_id", cfg.OAuth2.ClientID,
			"configured_client_secret", cfg.OAuth2.ClientSecret)

		// Wrap response writer to capture what's being written
		captureWriter := &responseCapture{ResponseWriter: w}

		if err := oauth2Srv.HandleTokenRequest(captureWriter, r); err != nil {
			slog.Error("token request handler error",
				"error", err.Error(),
				"error_type", fmt.Sprintf("%T", err),
				"client_id_form", formClientID,
				"client_id_basic_auth", basicAuthUser)
			if !captureWriter.written {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
		}

		// Log what was written to response
		if len(captureWriter.body) > 0 {
			slog.Info("token response written",
				"body_length", len(captureWriter.body),
				"status", captureWriter.statusCode,
				"body", string(captureWriter.body))
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

// handleJWKS serves the RSA public key as a JSON Web Key Set.
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
	slog.Info("Starting OAuth2 mock server", "addr", addr)
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
