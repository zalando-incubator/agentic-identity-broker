// Package oauth2session manages OAuth2 authorization flows and session management.
//
// Encryption Integration:
//
// This service handles encryption and decryption of OAuth tokens using envelope encryption.
// Tokens are encrypted before storage and decrypted on retrieval using the EncryptionPort.
// The storage repository (UserSessionRepository) operates on opaque encrypted bytes and
// is unaware of encryption mechanics, maintaining hexagonal architecture purity.
//
// Key Methods:
// - storeSession(): Encrypts tokens before repository.Create()
// - DecryptAccessToken(): Decrypts access token for use
// - DecryptRefreshToken(): Decrypts refresh token for use
//
// Encryption Context:
// All encryption uses context binding: {"service_id": "<oauth2-service-id>"}
// This context is bound as Additional Authenticated Data (AAD) to prevent cross-service token reuse.
package oauth2session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const maxProviderStateBytes = 6000

func addProviderAuthorizationParams(values url.Values, params map[string]string) {
	for name, value := range params {
		if !model.IsReservedAuthorizationParamName(name) {
			values.Set(name, value)
		}
	}
}

// OAuth2SessionService orchestrates OAuth2 authorization flows and session management.
// It retrieves confidential services with client secrets decrypted when available. Public
// services intentionally retain an absent Secret, so their OAuth2 configurations omit a client
// secret.
type OAuth2SessionService struct {
	providerService *thirdparty.ThirdpartyOAuth2ProviderService // Domain service that handles encryption/decryption
	sessionRepo     ports.UserSessionRepository
	grantRepo       ports.UserGrantRepository // For dependent agents
	agentRepo       ports.AgentRepository     // For agent display names
	encryption      ports.EncryptionPort
	httpClient      *http.Client // For upstream OAuth2 token endpoint calls
	jweTokenService *domjwe.TokenService
	config          Config
	logger          *slog.Logger
}

// Config holds configuration for the OAuth2 session service.
type Config struct {
	CallbackBaseURL    string        // e.g., "https://broker.example.com"
	StateTokenTTL      time.Duration // Default: 10 minutes
	PKCEVerifierLength int           // Default: 32 bytes
	MaxRetries         int           // Default: 3
	RetryBaseDelay     time.Duration // Default: 1 second
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		StateTokenTTL:      10 * time.Minute,
		PKCEVerifierLength: 32,
		MaxRetries:         3,
		RetryBaseDelay:     time.Second,
	}
}

// NewConfigFromPorts builds OAuth2SessionService Config from application-wide configuration.
// This ensures the service uses configuration from the application's ConfigPort instead of hardcoded defaults.
// Constitution Principle VII (Configuration-Driven Design) compliance.
func NewConfigFromPorts(portsCfg ports.ThirdPartyOAuth2Config, callbackBaseURL string) Config {
	cfg := Config{
		CallbackBaseURL: callbackBaseURL,
		MaxRetries:      3,           // Not yet in ports config, use default
		RetryBaseDelay:  time.Second, // Not yet in ports config, use default
	}

	// Use configured values if provided, otherwise use defaults
	if portsCfg.StateTokenTTL > 0 {
		cfg.StateTokenTTL = portsCfg.StateTokenTTL
	} else {
		cfg.StateTokenTTL = 10 * time.Minute
	}

	if portsCfg.PKCEVerifierLength > 0 {
		cfg.PKCEVerifierLength = portsCfg.PKCEVerifierLength
	} else {
		cfg.PKCEVerifierLength = 32
	}

	return cfg
}

// NewOAuth2SessionService creates a new OAuth2SessionService.
// Requires a ThirdpartyOAuth2ProviderService that conditionally decrypts confidential client
// secrets. Public providers retain an absent Secret and require no client credential.
func NewOAuth2SessionService(
	providerService *thirdparty.ThirdpartyOAuth2ProviderService,
	sessionRepo ports.UserSessionRepository,
	grantRepo ports.UserGrantRepository,
	agentRepo ports.AgentRepository,
	encryption ports.EncryptionPort,
	httpClient *http.Client,
	jweTokenService *domjwe.TokenService,
	config Config,
	logger *slog.Logger,
) *OAuth2SessionService {
	if config.StateTokenTTL == 0 {
		config.StateTokenTTL = 10 * time.Minute
	}
	if config.PKCEVerifierLength == 0 {
		config.PKCEVerifierLength = 32
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBaseDelay == 0 {
		config.RetryBaseDelay = time.Second
	}

	return &OAuth2SessionService{
		providerService: providerService,
		sessionRepo:     sessionRepo,
		grantRepo:       grantRepo,
		agentRepo:       agentRepo,
		encryption:      encryption,
		httpClient:      httpClient,
		jweTokenService: jweTokenService,
		config:          config,
		logger:          logger,
	}
}

// GetCallbackBaseURL returns the configured callback base URL.
// This is used for validating OAuth2 redirect URIs and constructing callback endpoints.
// Constitution Principle VII: Configuration-Driven Design
func (s *OAuth2SessionService) GetCallbackBaseURL() string {
	return s.config.CallbackBaseURL
}

// ServiceAuthorizeURL returns the URL a user must visit to initiate (or re-initiate)
// OAuth2 authorization with the specified third-party service.
// The URL follows the same base-URL convention already used for OAuth2 callback URLs.
// It is included in RFC 6749 §5.2 error_uri fields when a user session is missing
// or has expired, giving clients an actionable re-authentication link.
func (s *OAuth2SessionService) ServiceAuthorizeURL(serviceID id.ServiceID) string {
	return s.config.CallbackBaseURL + "/api/third-party/" + serviceID.String() + "/oauth2/authorize"
}

// InitiateFlowResult contains the data needed to redirect user to authorization.
type InitiateFlowResult struct {
	AuthorizationURL string // Full URL to redirect user to
	StateToken       string // JWE-encrypted state token for storage/form hidden field
}

// HandleCallbackRequest contains parameters from the OAuth2 callback.
type HandleCallbackRequest struct {
	ServiceID id.ServiceID // From URL path
	Code      string       // Authorization code from query
	State     string       // JWE state token from query
	Error     string       // OAuth2 error code (optional)
	ErrorDesc string       // OAuth2 error description (optional)
}

// HandleCallbackResult contains the result of processing an OAuth2 callback.
type HandleCallbackResult struct {
	Session        *storage.UserSession
	RedirectURI    string // Original redirect_uri from state token claims
	ConsentStateID string
}

// AgentInfo represents an agent with display information.
type AgentInfo struct {
	ID          id.AgentID `json:"id"`           // Agent's unique identifier
	DisplayName string     `json:"display_name"` // Agent's display name for UI presentation
}

// SessionWithAgents represents a session with information about dependent agents.
type SessionWithAgents struct {
	Session         *storage.UserSession
	DependentAgents []AgentInfo // Agents that use this session with display names
}

// =============================================================================
// GROUP 1: Crypto Foundations
// =============================================================================

// CreateStateToken encrypts the state token claims into a JWE string.
func (s *OAuth2SessionService) CreateStateToken(claims *OAuth2StateTokenClaims) (string, error) {
	if err := claims.Validate(); err != nil {
		return "", fmt.Errorf("invalid state token claims: %w", err)
	}

	token, err := s.jweTokenService.Encrypt(claims)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt state token: %w", err)
	}

	return token, nil
}

// ValidateStateToken decrypts and validates a state token.
// The token must be a valid JWE compact serialization with A256GCMKW key wrapping
// and A256GCM content encryption. Any tampering with the token will cause decryption
// to fail due to GCM authentication tag verification.
func (s *OAuth2SessionService) ValidateStateToken(
	tokenString string,
	currentPrincipal id.Principal,
	expectedServiceID id.ServiceID,
) (*OAuth2StateTokenClaims, error) {
	// Input validation
	if tokenString == "" {
		return nil, fmt.Errorf("state token is empty: %w", ErrInvalidStateToken)
	}

	var claims OAuth2StateTokenClaims
	if err := s.jweTokenService.Decrypt(tokenString, &claims); err != nil {
		return nil, fmt.Errorf("failed to decrypt state token: %w", ErrInvalidStateToken)
	}

	// Validate claims structure
	if err := claims.Validate(); err != nil {
		return nil, fmt.Errorf("invalid state token claims: %w", err)
	}

	// Check expiration
	if claims.IsExpired() {
		// Audit log: state token expired
		s.logger.Warn("oauth2_state_token_expired",
			"event", "session.oauth2.state_expired",
			"principal", claims.Principal,
			"service_id", claims.ServiceID,
			"expired_at", claims.ExpiresAt.Unix(),
			"timestamp", time.Now().Unix())
		return nil, ErrStateTokenExpired
	}

	// Check principal match (CSRF protection)
	if claims.Principal != currentPrincipal {
		// Audit log: principal mismatch (CSRF attack detection)
		s.logger.Error("oauth2_state_validation_failed",
			"event", "session.oauth2.state_validation_failed",
			"reason", "principal_mismatch",
			"expected_principal", currentPrincipal,
			"actual_principal", claims.Principal,
			"service_id", expectedServiceID,
			"timestamp", time.Now().Unix())
		return nil, ErrPrincipalMismatch
	}

	// Check service ID match
	if claims.ServiceID != expectedServiceID {
		s.logger.Error("oauth2_state_validation_failed",
			"event", "session.oauth2.state_validation_failed",
			"reason", "service_id_mismatch",
			"expected_service_id", expectedServiceID,
			"actual_service_id", claims.ServiceID,
			"principal", currentPrincipal,
			"timestamp", time.Now().Unix())
		return nil, fmt.Errorf("state token validation failed: %w", ErrServiceIDMismatch)
	}

	return &claims, nil
}

// =============================================================================
// GROUP 2: OAuth2 Helpers
// =============================================================================

// buildOAuth2Config creates an oauth2.Config from a third-party provider entity.
// Confidential client secrets must be in plaintext state (decrypted by ThirdpartyOAuth2ProviderService.Get).
func (s *OAuth2SessionService) buildOAuth2Config(
	entity *model.ThirdpartyOAuth2ProviderEntity,
	callbackURL string,
) (*oauth2.Config, error) {
	// Extract scopes from entity
	scopes := make([]string, 0, len(entity.Scopes))
	for _, scope := range entity.Scopes {
		scopes = append(scopes, scope.ScopeValue)
	}

	config := &oauth2.Config{
		ClientID:     entity.ClientID.String(),
		ClientSecret: "",
		RedirectURL:  callbackURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  entity.Endpoints.AuthorizeEndpoint,
			TokenURL: entity.Endpoints.TokenEndpoint,
		},
	}
	if entity.IsPublicClient() {
		config.Endpoint.AuthStyle = oauth2.AuthStyleInParams
		return config, nil
	}

	clientSecret, err := entity.Secret.GetPlaintext()
	if err != nil {
		return nil, fmt.Errorf("provider secret not in plaintext state; ensure entity was fetched via ThirdpartyOAuth2ProviderService.Get: %w", err)
	}
	config.ClientSecret = clientSecret

	return config, nil
}

func safeTokenExchangeError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		switch retrieveErr.ErrorCode {
		case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type", "invalid_scope":
			return fmt.Errorf("%w: %s", ErrTokenExchange, retrieveErr.ErrorCode)
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return fmt.Errorf("%w: %w", ErrTokenExchange, urlErr.Err)
	}

	return fmt.Errorf("%w: upstream token request failed", ErrTokenExchange)
}

// exchangeCodeWithRetry exchanges authorization code for tokens with exponential backoff retry.
func (s *OAuth2SessionService) exchangeCodeWithRetry(
	ctx context.Context,
	config *oauth2.Config,
	code string,
	verifier string,
	authorizationParams map[string]string,
) (*oauth2.Token, error) {
	lastErr := ErrTokenExchange

	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		// Try to exchange code for token
		params := url.Values{}
		addProviderAuthorizationParams(params, authorizationParams)
		opts := make([]oauth2.AuthCodeOption, 0, len(params)+1)
		opts = append(opts, oauth2.VerifierOption(verifier))
		for name, values := range params {
			opts = append(opts, oauth2.SetAuthURLParam(name, values[0]))
		}
		token, err := config.Exchange(ctx, code, opts...)
		if err == nil {
			return token, nil
		}
		lastErr = safeTokenExchangeError(err)

		// If this was the last attempt, break
		if attempt == s.config.MaxRetries-1 {
			break
		}

		// Calculate exponential backoff delay
		delay := s.config.RetryBaseDelay * time.Duration(math.Pow(2, float64(attempt)))
		s.logger.Warn("token exchange failed, retrying",
			"attempt", attempt+1,
			"delay", delay,
			"reason", "token_exchange_failed",
			"error", lastErr)

		// Sleep with context cancellation support
		select {
		case <-time.After(delay):
			// Continue to next retry
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

// =============================================================================
// GROUP 3: Service Methods
// =============================================================================

// InitiateOAuth2Flow starts the OAuth2 authorization code flow.
// Creates PKCE verifier/challenge, generates JWE state token, returns auth URL.
func (s *OAuth2SessionService) InitiateOAuth2Flow(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
	redirectURI string,
) (*InitiateFlowResult, error) {
	return s.initiateOAuth2Flow(ctx, principal, serviceID, redirectURI, "")
}

// InitiateOAuth2FlowWithConsentState starts an OAuth2 flow with a sealed
// current-tab consent-selection reference.
func (s *OAuth2SessionService) InitiateOAuth2FlowWithConsentState(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
	redirectURI string,
	consentStateID string,
) (*InitiateFlowResult, error) {
	return s.initiateOAuth2Flow(ctx, principal, serviceID, redirectURI, consentStateID)
}

func (s *OAuth2SessionService) initiateOAuth2Flow(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
	redirectURI string,
	consentStateID string,
) (*InitiateFlowResult, error) {
	s.logger.Info("initiating OAuth2 flow", "principal", principal, "service_id", serviceID)
	if len(redirectURI) >= maxProviderStateBytes {
		return nil, ErrStateTokenTooLarge
	}

	// Fetch the service (with decrypted client secret via service manager)
	service, err := s.providerService.Get(ctx, serviceID)
	if err != nil {
		s.logger.Error("service not found", "service_id", serviceID, "err", err)
		return nil, fmt.Errorf("failed to initiate OAuth2 flow: %w", ErrServiceNotFound)
	}

	// Generate PKCE
	verifier := oauth2.GenerateVerifier()

	// Create state token claims
	now := time.Now()
	claims := &OAuth2StateTokenClaims{
		Principal:      principal,
		ServiceID:      serviceID,
		PKCEVerifier:   verifier,
		RedirectURI:    redirectURI,
		ConsentStateID: consentStateID,
		IssuedAt:       now,
		ExpiresAt:      now.Add(s.config.StateTokenTTL),
	}

	// Encrypt state token
	stateToken, err := s.CreateStateToken(claims)
	if err != nil {
		s.logger.Error("failed to create state token", "err", err)
		return nil, fmt.Errorf("failed to create state token: %w", err)
	}
	if len(stateToken) >= maxProviderStateBytes {
		return nil, ErrStateTokenTooLarge
	}

	// Build OAuth2 config with callback URL
	callbackURL := s.config.CallbackBaseURL + "/api/third-party/" + serviceID.String() + "/oauth2/callback"
	cfg, err := s.buildOAuth2Config(service, callbackURL)
	if err != nil {
		s.logger.Error("failed to build oauth2 config", "service_id", serviceID, "err", err)
		return nil, fmt.Errorf("failed to build oauth2 config: %w", err)
	}

	// Generate authorization URL with PKCE
	authURL := cfg.AuthCodeURL(stateToken, oauth2.S256ChallengeOption(verifier))
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse authorization URL: %w", err)
	}
	query := parsedURL.Query()
	if len(service.Scopes) == 0 {
		query.Del("scope")
	}
	addProviderAuthorizationParams(query, service.AuthorizationParams)
	parsedURL.RawQuery = query.Encode()
	authURL = parsedURL.String()

	// Audit log: OAuth2 flow initiated successfully
	s.logger.Info("oauth2_flow_initiated",
		"event", "session.oauth2.flow_initiated",
		"principal", principal,
		"service_id", serviceID,
		"public_client", service.IsPublicClient(),
		"timestamp", now.Unix())

	return &InitiateFlowResult{
		AuthorizationURL: authURL,
		StateToken:       stateToken,
	}, nil
}

// createSession creates and stores a new user session from OAuth2 tokens.
// Uses upsert semantics: if session exists for (principal, service_id), updates tokens while preserving CreatedAt.
func (s *OAuth2SessionService) createSession(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
	token *oauth2.Token,
	scope []string,
) (*storage.UserSession, error) {
	// Check if session already exists for this principal+service
	existingSession, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		s.logger.Error("failed to query existing session", "principal", principal, "service_id", serviceID, "err", err)
		return nil, fmt.Errorf("failed to query existing session: %w", err)
	}

	var encryptedAccess []byte
	var encryptedRefresh []byte

	// Create encryption context for this session bound to service (cryptographic service isolation)
	// This ensures tokens encrypted for one service cannot be decrypted with another service's context
	serviceSubject := domainencryption.NewServiceBranchKeySubject(serviceID)
	encryptionContext := serviceSubject.EncryptionContext()

	// Encrypt access token
	encryptedAccess, err = s.encryption.Encrypt(ctx, []byte(token.AccessToken), encryptionContext)
	if err != nil {
		s.logger.Error("failed to encrypt access token", "err", err)
		return nil, fmt.Errorf("failed to encrypt access token: %w", err)
	}

	// Encrypt refresh token if present
	if token.RefreshToken != "" {
		encryptedRefresh, err = s.encryption.Encrypt(ctx, []byte(token.RefreshToken), encryptionContext)
		if err != nil {
			s.logger.Error("failed to encrypt refresh token", "err", err)
			return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
		}
	}

	// Determine access token expiration
	var accessTokenExpiresAt *time.Time
	if !token.Expiry.IsZero() {
		accessTokenExpiresAt = &token.Expiry
	}

	// Determine token type (default to Bearer if not specified)
	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}

	// Create or update session
	now := time.Now()
	var sessionID id.SessionID
	var createdAt time.Time
	var initiatedAt time.Time

	if existingSession != nil {
		// Update existing session: preserve ID and CreatedAt
		sessionID = existingSession.ID
		createdAt = existingSession.CreatedAt
		initiatedAt = existingSession.InitiatedAt
	} else {
		// Create new session
		sessionID = id.NewSessionID()
		createdAt = now
		initiatedAt = now
	}

	session := &storage.UserSession{
		ID:                    sessionID,
		Principal:             principal,
		ServiceID:             serviceID,
		EncryptedAccessToken:  encryptedAccess,
		EncryptedRefreshToken: encryptedRefresh,
		TokenType:             tokenType,
		Scope:                 scope,
		AccessTokenExpiresAt:  accessTokenExpiresAt,
		InitiatedAt:           initiatedAt,
		CreatedAt:             createdAt,
		UpdatedAt:             now,
	}

	// Store session (will upsert if already exists)
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		s.logger.Error("failed to create session", "principal", principal, "service_id", serviceID, "err", err)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.Info("session created", "session_id", sessionID, "principal", principal, "service_id", serviceID)
	return session, nil
}

// HandleCallback processes the OAuth2 callback, exchanges code for tokens, stores session.
func (s *OAuth2SessionService) HandleCallback(
	ctx context.Context,
	principal id.Principal,
	req *HandleCallbackRequest,
) (*HandleCallbackResult, error) {
	s.logger.Debug("processing OAuth2 callback", "service_id", req.ServiceID, "principal", principal)

	// Check for OAuth2 error response
	if req.Error != "" {
		if req.ErrorDesc != "" {
			s.logger.Warn("OAuth2 authorization failed", "error", req.Error, "description", req.ErrorDesc)
			return nil, fmt.Errorf("OAuth2 authorization failed: %s: %s", req.Error, req.ErrorDesc)
		}
		s.logger.Warn("OAuth2 authorization failed", "error", req.Error)
		return nil, fmt.Errorf("OAuth2 authorization failed: %s", req.Error)
	}

	// Validate state token (checks expiration, principal mismatch, tampering)
	claims, err := s.ValidateStateToken(req.State, principal, req.ServiceID)
	if err != nil {
		s.logger.Error("state token validation failed", "err", err)
		return nil, fmt.Errorf("state token validation failed: %w", err)
	}

	// Fetch the service (with decrypted client secret via service manager)
	service, err := s.providerService.Get(ctx, req.ServiceID)
	if err != nil {
		s.logger.Error("service not found during callback", "service_id", req.ServiceID, "err", err)
		return nil, fmt.Errorf("service not found: %w", err)
	}

	// Build OAuth2 config
	callbackURL := s.config.CallbackBaseURL + "/api/third-party/" + req.ServiceID.String() + "/oauth2/callback"
	cfg, err := s.buildOAuth2Config(service, callbackURL)
	if err != nil {
		s.logger.Error("failed to build oauth2 config during callback", "service_id", req.ServiceID, "err", err)
		return nil, fmt.Errorf("failed to build oauth2 config: %w", err)
	}

	// Exchange authorization code for tokens (with retry)
	s.logger.Info("exchanging authorization code for token",
		"service_id", req.ServiceID,
		"callback_url", callbackURL,
		"token_endpoint", cfg.Endpoint.TokenURL,
		"client_id", cfg.ClientID)
	token, err := s.exchangeCodeWithRetry(ctx, cfg, req.Code, claims.PKCEVerifier, service.AuthorizationParams)
	if err != nil {
		// Audit log: PKCE validation failure (token exchange failure typically indicates PKCE error)
		s.logger.Error("oauth2_pkce_validation_failed",
			"event", "session.oauth2.pkce_validation_failed",
			"principal", principal,
			"service_id", req.ServiceID,
			"public_client", service.IsPublicClient(),
			"reason", "token_exchange_failed",
			"error", err,
			"timestamp", time.Now().Unix())
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	// Extract scopes from token response or fall back to service scopes.
	// GitHub returns scopes as comma-separated; use the flavor's separator.
	scopes := make([]string, 0)
	if scopeVal := token.Extra("scope"); scopeVal != nil {
		if scopeStr, ok := scopeVal.(string); ok && scopeStr != "" {
			separator := service.Flavor.ScopeSeparator()
			for _, s := range strings.Split(scopeStr, separator) {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					scopes = append(scopes, trimmed)
				}
			}
		}
	}
	if len(scopes) == 0 {
		// Fall back to service scopes
		for _, scope := range service.Scopes {
			scopes = append(scopes, scope.ScopeValue)
		}
	}

	// Create and store session
	session, err := s.createSession(ctx, principal, req.ServiceID, token, scopes)
	if err != nil {
		s.logger.Error("failed to create session from token", "err", err)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Audit log: OAuth2 session established successfully
	s.logger.Info("oauth2_session_established",
		"event", "session.oauth2.session_established",
		"principal", principal,
		"service_id", req.ServiceID,
		"public_client", service.IsPublicClient(),
		"session_id", session.ID,
		"timestamp", time.Now().Unix())

	return &HandleCallbackResult{
		Session:        session,
		RedirectURI:    claims.RedirectURI,
		ConsentStateID: claims.ConsentStateID,
	}, nil
}

// RefreshAccessToken calls the upstream OAuth2 service's token endpoint to refresh an expired access token.
// Uses the provided refresh token to obtain a new access token from the service.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - entity: The ThirdpartyOAuth2ProviderEntity configuration containing token endpoint and credentials
//   - refreshToken: The valid refresh token from the stored session
//
// Returns:
//   - *oauth2.Token with new access_token, optional refresh_token, and expiry
//   - error if the refresh request fails (network error, invalid response, or upstream error)
//
// Per RFC 6749 Section 6, sends a POST request to the token endpoint with:
//   - grant_type=refresh_token
//   - refresh_token=<the provided refresh token>
//   - client_id=<from service config>
//   - client_secret=<from service config, confidential clients only>
func (s *OAuth2SessionService) RefreshAccessToken(
	ctx context.Context,
	entity *model.ThirdpartyOAuth2ProviderEntity,
	refreshToken string,
) (*oauth2.Token, error) {
	if entity == nil {
		return nil, fmt.Errorf("service entity cannot be nil")
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token cannot be empty")
	}

	isPublicClient := entity.IsPublicClient()
	var clientSecret string
	if !isPublicClient {
		// Get plaintext secret (already decrypted by ThirdpartyOAuth2ProviderService).
		var err error
		clientSecret, err = entity.Secret.GetPlaintext()
		if err != nil {
			return nil, fmt.Errorf("provider secret not in plaintext state for refresh: %w", err)
		}
	}

	// Prepare refresh token request per RFC 6749 Section 6.
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", entity.ClientID.String())
	if !isPublicClient {
		data.Set("client_secret", clientSecret)
	}
	addProviderAuthorizationParams(data, entity.AuthorizationParams)

	// Create POST request to token endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, entity.Endpoints.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token request: %w", err)
	}

	// Set standard OAuth2 headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Execute the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call upstream token endpoint for refresh: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Decode response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token,omitempty"`
		Scope        string `json:"scope,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode upstream token response: %w", err)
	}

	// Check for HTTP error status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream token endpoint returned error status %d", resp.StatusCode)
	}

	// Validate required fields in response
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("upstream token response missing access_token")
	}

	// Determine token expiry
	var expiry time.Time
	if tokenResp.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	// Build oauth2.Token
	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
		Expiry:       expiry,
	}

	s.logger.Info("access token refreshed",
		"service_id", entity.ID,
		"token_endpoint", entity.Endpoints.TokenEndpoint)

	return token, nil
}

// UpdateSessionTokens updates an existing session with refreshed tokens.
// Encrypts tokens using encryption context binding and persists the updated session.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - principal: The user principal (for encryption context)
//   - session: The UserSession to update (will be modified in-place)
//   - newToken: The new oauth2.Token from refresh operation
//
// Returns:
//   - error if encryption or persistence fails
//
// The session is modified in-place and persisted with upsert semantics.
// Encryption context binds tokens to service_id per ADR 008 for performance optimization.
func (s *OAuth2SessionService) UpdateSessionTokens(
	ctx context.Context,
	principal id.Principal,
	session *storage.UserSession,
	newToken *oauth2.Token,
) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}

	if newToken == nil {
		return fmt.Errorf("new token cannot be nil")
	}

	// Build encryption context for this session - uses service_id only
	serviceSubject := domainencryption.NewServiceBranchKeySubject(session.ServiceID)
	encContext := serviceSubject.EncryptionContext()

	// Encrypt new access token
	encryptedAccess, err := s.encryption.Encrypt(ctx, []byte(newToken.AccessToken), encContext)
	if err != nil {
		return fmt.Errorf("failed to encrypt refreshed access token: %w", err)
	}

	// Update session with new access token
	session.EncryptedAccessToken = encryptedAccess

	// Update access token expiration time
	if !newToken.Expiry.IsZero() {
		session.AccessTokenExpiresAt = &newToken.Expiry
	} else {
		// If no expiry provided, assume token doesn't expire
		session.AccessTokenExpiresAt = nil
	}

	// Update refresh token if provided in response
	if newToken.RefreshToken != "" {
		encryptedRefresh, err := s.encryption.Encrypt(ctx, []byte(newToken.RefreshToken), encContext)
		if err != nil {
			return fmt.Errorf("failed to encrypt new refresh token: %w", err)
		}
		session.EncryptedRefreshToken = encryptedRefresh
	}

	// Update timestamp
	session.UpdatedAt = time.Now()

	// Persist updated session (upsert semantics)
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return fmt.Errorf("failed to update session with refreshed tokens: %w", err)
	}

	s.logger.Info("session tokens updated",
		"principal", principal,
		"service_id", session.ServiceID,
		"session_id", session.ID)

	return nil
}

// DecryptAccessToken retrieves and decrypts the stored access token from a session.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - principal: The user principal (for encryption context)
//   - session: The UserSession containing encrypted token
//
// Returns:
//   - string: The decrypted access token value
//   - error if decryption fails
//
// The encryption context uses service_id for verification.
func (s *OAuth2SessionService) DecryptAccessToken(
	ctx context.Context,
	session *storage.UserSession,
) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session cannot be nil")
	}

	serviceSubject := domainencryption.NewServiceBranchKeySubject(session.ServiceID)
	encContext := serviceSubject.EncryptionContext()

	accessToken, err := s.encryption.Decrypt(ctx, session.EncryptedAccessToken, encContext)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt access token: %w", err)
	}

	return string(accessToken), nil
}

// DecryptRefreshToken retrieves and decrypts the stored refresh token from a session.
// Returns empty string (not error) if no refresh token stored.
func (s *OAuth2SessionService) DecryptRefreshToken(
	ctx context.Context,
	session *storage.UserSession,
) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session cannot be nil")
	}

	// Return empty if no refresh token stored
	if len(session.EncryptedRefreshToken) == 0 {
		return "", nil
	}

	serviceSubject := domainencryption.NewServiceBranchKeySubject(session.ServiceID)
	encContext := serviceSubject.EncryptionContext()

	refreshToken, err := s.encryption.Decrypt(ctx, session.EncryptedRefreshToken, encContext)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	return string(refreshToken), nil
}

// ListUserSessions returns all sessions for a principal with summary info.
func (s *OAuth2SessionService) ListUserSessions(
	ctx context.Context,
	principal id.Principal,
) ([]*storage.UserSessionSummary, error) {
	// Fetch all sessions for principal
	sessions, err := s.sessionRepo.ListByPrincipal(ctx, principal)
	if err != nil {
		s.logger.Error("failed to list sessions", "principal", principal, "err", err)
		return nil, err
	}

	// Convert to summaries with agent counts
	summaries := make([]*storage.UserSessionSummary, 0, len(sessions))
	for _, session := range sessions {
		// Fetch service details (with decrypted client secret via service manager)
		service, err := s.providerService.Get(ctx, session.ServiceID)
		if err != nil {
			s.logger.Warn("service not found", "service_id", session.ServiceID, "err", err)
			continue
		}

		// Count dependent agents
		agentCount, err := s.grantRepo.CountAgentsByPrincipalAndServiceID(ctx, principal, session.ServiceID)
		if err != nil {
			s.logger.Warn("failed to count agents", "service_id", session.ServiceID, "err", err)
			agentCount = 0
		}

		summary := storage.NewUserSessionSummary(session, service.DisplayName, agentCount)
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// TerminateSession deletes a session and its encrypted tokens.
func (s *OAuth2SessionService) TerminateSession(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
) error {
	// Audit log: Session termination initiated
	s.logger.Info("oauth2_session_termination_initiated",
		"event", "session.oauth2.termination_initiated",
		"principal", principal,
		"service_id", serviceID,
		"timestamp", time.Now().Unix())

	// Step 1: Fetch session to verify ownership
	session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		// Audit log: Session not found
		s.logger.Warn("oauth2_session_termination_failed",
			"event", "session.oauth2.session_not_found",
			"principal", principal,
			"service_id", serviceID,
			"reason", "session_not_found",
			"error", err.Error(),
			"timestamp", time.Now().Unix())
		return fmt.Errorf("session not found: %w", ErrSessionNotFound)
	}

	if session == nil {
		// Audit log: Session not found (nil case)
		s.logger.Warn("oauth2_session_termination_failed",
			"event", "session.oauth2.session_not_found",
			"principal", principal,
			"service_id", serviceID,
			"reason", "session_not_found",
			"timestamp", time.Now().Unix())
		return ErrSessionNotFound
	}

	// Step 2: Verify principal ownership (authorization check)
	if session.Principal != principal {
		// Audit log: Unauthorized termination attempt (SECURITY INCIDENT)
		s.logger.Error("oauth2_session_termination_failed",
			"event", "session.oauth2.termination_unauthorized",
			"principal", principal,
			"expected_principal", session.Principal,
			"service_id", serviceID,
			"session_id", session.ID,
			"reason", "principal_mismatch",
			"timestamp", time.Now().Unix())
		return fmt.Errorf("termination denied: %w", ErrUnauthorized)
	}

	// Step 3: Delete session from repository
	err = s.sessionRepo.DeleteByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		// Audit log: Repository error during termination
		s.logger.Error("oauth2_session_termination_failed",
			"event", "session.oauth2.termination_repository_error",
			"principal", principal,
			"service_id", serviceID,
			"session_id", session.ID,
			"reason", "repository_error",
			"error", err.Error(),
			"timestamp", time.Now().Unix())
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Audit log: Session terminated successfully
	s.logger.Info("oauth2_session_terminated",
		"event", "session.oauth2.session_terminated",
		"principal", principal,
		"service_id", serviceID,
		"session_id", session.ID,
		"initiated_at", session.InitiatedAt,
		"timestamp", time.Now().Unix())

	return nil
}

// GetSessionWithAgents returns session details including list of dependent agents.
func (s *OAuth2SessionService) GetSessionWithAgents(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
) (*SessionWithAgents, error) {
	s.logger.Debug("retrieving session with agents",
		"principal", principal,
		"service_id", serviceID)

	// Step 1: Fetch session to verify it exists and ownership
	session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		s.logger.Error("failed to fetch session", "principal", principal, "service_id", serviceID, "err", err)
		return nil, fmt.Errorf("session details retrieval failed: %w", ErrSessionNotFound)
	}

	if session == nil {
		return nil, ErrSessionNotFound
	}

	// Step 2: Verify principal ownership (authorization)
	if session.Principal != principal {
		s.logger.Error("principal mismatch in GetSessionWithAgents",
			"expected_principal", principal,
			"session_principal", session.Principal,
			"service_id", serviceID)
		return nil, fmt.Errorf("session details access denied: %w", ErrUnauthorized)
	}

	// Step 3: Query dependent agent IDs using grant repository
	// Get list of actual agent IDs that have delegated tokens for this service
	agentIDs, err := s.grantRepo.ListByPrincipalAndServiceID(ctx, principal, serviceID)
	if err != nil {
		s.logger.Warn("failed to list dependent agent IDs", "service_id", serviceID, "err", err)
		agentIDs = []id.AgentID{} // Return empty list on error
	}

	// Step 4: Fetch agent display names for each dependent agent
	dependentAgents := make([]AgentInfo, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		agent, err := s.agentRepo.Get(ctx, agentID)
		if err != nil {
			s.logger.Warn("failed to fetch agent details", "agent_id", agentID, "err", err)
			// Fall back to using agent ID if agent not found
			dependentAgents = append(dependentAgents, AgentInfo{
				ID:          agentID,
				DisplayName: agentID.String(), // Use ID as fallback
			})
			continue
		}

		dependentAgents = append(dependentAgents, AgentInfo{
			ID:          agent.ID,
			DisplayName: agent.DisplayName,
		})
	}

	s.logger.Debug("retrieved session with agents",
		"principal", principal,
		"service_id", serviceID,
		"dependent_agent_count", len(dependentAgents))

	return &SessionWithAgents{
		Session:         session,
		DependentAgents: dependentAgents,
	}, nil
}

// GetValidAccessToken retrieves a valid, non-expired access token for a user at a service.
// This method transparently handles token refresh if the access token has expired but
// a valid refresh token is available.
//
// Usage pattern for callers:
//
//	session, token, err := s.GetValidAccessToken(ctx, principal, serviceID)
//	if err != nil {
//	    // Handle error (session not found, all tokens expired, etc)
//	}
//	// Use token - it's guaranteed valid and non-expired
//	// Use session - contains metadata like scope, token_type, expires_in
//
// Encapsulated logic:
// 1. Fetch session from repository
// 2. Check if access token is expired
// 3. If expired, refresh using refresh token (if available)
// 4. Update session with new tokens
// 5. Decrypt and return valid access token along with session metadata
//
// Error cases:
//   - ErrSessionNotFound: No session exists for principal+service (T075)
//   - ErrSessionExpired: Both tokens expired, user must re-authenticate (T076)
//   - ErrRefreshFailed: Upstream provider rejected refresh request
//   - fmt.Errorf wraps: Other retrieval/decryption/storage errors
//
// Per SR-005 (Security Rule): Token values are never included in error messages.
// Only metadata (service, expiration times) is included.
func (s *OAuth2SessionService) GetValidAccessToken(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
) (*storage.UserSession, string, error) {
	// Step 1: Fetch session from repository
	// This is the ONLY repository access in this flow.
	// All other operations use session aggregate methods.
	session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		if err == ports.ErrNotFound {
			// T075: Session doesn't exist
			return nil, "", fmt.Errorf("%w: principal=%s, service=%s", ErrSessionNotFound, principal, serviceID)
		}
		// Other repository errors (connection, timeout, etc)
		return nil, "", fmt.Errorf("failed to retrieve session: %w", err)
	}

	// Defensive: ensure session is not nil (should not happen given err is nil)
	if session == nil {
		return nil, "", fmt.Errorf("%w: session is nil (principal=%s, service=%s)", ErrSessionNotFound, principal, serviceID)
	}

	// Step 2: Check if access token has expired
	if session.HasValidAccessToken() {
		// Access token is still valid - just decrypt and return
		accessToken, err := s.DecryptAccessToken(ctx, session)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decrypt access token: %w", err)
		}
		return session, accessToken, nil
	}

	// Access token has expired - check if we can refresh

	// Step 3: Check if refresh token is available and valid
	if !session.CanRefresh() {
		// T076: Both tokens are expired
		return nil, "", fmt.Errorf("%w: principal=%s, service=%s", ErrSessionExpired, principal, serviceID)
	}

	if err := s.refreshSessionTokens(ctx, principal, session); err != nil {
		return nil, "", err
	}

	// Step 8: Decrypt and return the new access token along with updated session
	accessToken, err := s.DecryptAccessToken(ctx, session)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decrypt refreshed access token: %w", err)
	}

	return session, accessToken, nil
}

// refreshSessionTokens refreshes the session's access token against the upstream
// provider using the stored refresh token and persists the rotated tokens in place.
// Precondition: session.CanRefresh() is true. Returns ErrRefreshFailed (wrapped) on
// upstream rejection; other errors are wrapped with context.
func (s *OAuth2SessionService) refreshSessionTokens(
	ctx context.Context,
	principal id.Principal,
	session *storage.UserSession,
) error {
	serviceID := session.ServiceID

	service, err := s.providerService.Get(ctx, serviceID)
	if err != nil {
		s.logger.Error("failed to fetch service for token refresh",
			"principal", principal,
			"service_id", serviceID,
			"err", err)
		return fmt.Errorf("failed to fetch service for token refresh: %w", err)
	}

	if service == nil {
		return fmt.Errorf("service not found for refresh: service_id=%s", serviceID)
	}

	refreshToken, err := s.DecryptRefreshToken(ctx, session)
	if err != nil {
		s.logger.Error("failed to decrypt refresh token",
			"principal", principal,
			"service_id", serviceID,
			"err", err)
		return fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	if refreshToken == "" {
		return fmt.Errorf("refresh token is empty: principal=%s, service=%s", principal, serviceID)
	}

	newToken, err := s.RefreshAccessToken(ctx, service, refreshToken)
	if err != nil {
		s.logger.Error("oauth2_refresh_failed",
			"event", "session.oauth2.refresh_failed",
			"principal", principal,
			"service_id", serviceID,
			"public_client", service.IsPublicClient(),
			"reason", "token_refresh_failed",
			"error", err,
			"timestamp", time.Now().Unix())
		return fmt.Errorf("%w: %w", ErrRefreshFailed, err)
	}

	if newToken == nil {
		return fmt.Errorf("upstream provider returned nil token: principal=%s, service=%s", principal, serviceID)
	}

	if err := s.UpdateSessionTokens(ctx, principal, session, newToken); err != nil {
		s.logger.Error("failed to update session with refreshed tokens",
			"principal", principal,
			"service_id", serviceID,
			"err", err)
		return fmt.Errorf("failed to update session with refreshed tokens: %w", err)
	}

	s.logger.Info("oauth2_token_refreshed",
		"event", "session.oauth2.token_refreshed",
		"principal", principal,
		"service_id", serviceID,
		"public_client", service.IsPublicClient(),
		"reason", "token_refresh_succeeded",
		"timestamp", time.Now().Unix())

	return nil
}

// GetSessionWithValidToken retrieves session metadata plus a valid access token.
// This is useful when the caller needs to build a response that includes session
// metadata (scope, token_type, expires_in, etc) along with the token itself.
//
// Internally uses GetValidAccessToken to ensure token is valid (with auto-refresh).
//
// Usage pattern for RFC 8693 token exchange response:
//
//	session, token, err := s.GetSessionWithValidToken(ctx, principal, serviceID)
//	if err != nil {
//	    // Handle error
//	}
//	// Build response using both:
//	response.AccessToken = token
//	response.Scope = strings.Join(session.Scope, " ")
//	response.ExpiresIn = calculateExpiresIn(session)
//
// Returns:
//   - session: Current session state (may have been updated by refresh)
//   - token: Valid, non-expired access token
//   - error: Same error cases as GetValidAccessToken
//
// Note: The session returned here may have been modified by refresh operation.
// The session's AccessTokenExpiresAt and UpdatedAt fields reflect the latest state.
func (s *OAuth2SessionService) GetSessionWithValidToken(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
) (*storage.UserSession, string, error) {
	// Step 1: Get valid token with session metadata (this handles all refresh logic transparently)
	// GetValidAccessToken now returns both the session and token in a single call,
	// eliminating the need for a separate repository fetch
	session, token, err := s.GetValidAccessToken(ctx, principal, serviceID)
	if err != nil {
		// Return error as-is; no additional wrapping needed
		return nil, "", err
	}

	return session, token, nil
}

// ForceRefreshSession unconditionally refreshes the access token for a user's
// session using the stored refresh token, even if the current access token is
// still valid. Returns the updated session summary.
func (s *OAuth2SessionService) ForceRefreshSession(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
) (*storage.UserSessionSummary, error) {
	session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		if err == ports.ErrNotFound {
			return nil, fmt.Errorf("%w: principal=%s, service=%s", ErrSessionNotFound, principal, serviceID)
		}
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("%w: principal=%s, service=%s", ErrSessionNotFound, principal, serviceID)
	}
	if !session.CanRefresh() {
		return nil, fmt.Errorf("%w: principal=%s, service=%s", ErrRefreshNotAvailable, principal, serviceID)
	}

	if err := s.refreshSessionTokens(ctx, principal, session); err != nil {
		return nil, err
	}

	agentCount, err := s.grantRepo.CountAgentsByPrincipalAndServiceID(ctx, principal, serviceID)
	if err != nil {
		s.logger.Warn("failed to count agents after refresh", "service_id", serviceID, "err", err)
		agentCount = 0
	}

	service, err := s.providerService.Get(ctx, serviceID)
	if err != nil || service == nil {
		s.logger.Warn("failed to fetch service display name after refresh", "service_id", serviceID, "err", err)
		return storage.NewUserSessionSummary(session, "", agentCount), nil
	}

	return storage.NewUserSessionSummary(session, service.DisplayName, agentCount), nil
}
