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
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const maxProviderStateBytes = 6000

const maxTokenResponseBytes = 1 << 20

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
	providerService     *thirdparty.ThirdpartyOAuth2ProviderService // Domain service that handles encryption/decryption
	sessionRepo         ports.UserSessionRepository
	refreshRepo         ports.UserSessionRefreshRepository
	refreshGroup        singleflight.Group
	background          *backgroundRefresher
	grantRepo           ports.UserGrantRepository // For dependent agents
	agentRepo           ports.AgentRepository     // For agent display names
	encryption          ports.EncryptionPort
	httpClient          *http.Client // For upstream OAuth2 token endpoint calls
	jweTokenService     *domjwe.TokenService
	cimdAssertionSigner ports.CIMDClientAssertionSigner
	config              Config
	logger              *slog.Logger
}

// Config holds configuration for the OAuth2 session service.
type Config struct {
	CallbackBaseURL          string        // e.g., "https://broker.example.com"
	StateTokenTTL            time.Duration // Default: 10 minutes
	PKCEVerifierLength       int           // Default: 32 bytes
	MaxRetries               int           // Default: 3
	RetryBaseDelay           time.Duration // Default: 1 second
	RefreshStorageTimeout    time.Duration // Budget for provider lookup and session storage operations
	RefreshLookahead         time.Duration // Access-token lookahead for background and sweep refresh.
	BackgroundRefreshWorkers int           // Maximum in-flight background refreshes per replica.
	SweepDefaultPageSize     int           // Default keyset page size for an admin sweep.
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		StateTokenTTL:            10 * time.Minute,
		PKCEVerifierLength:       32,
		MaxRetries:               3,
		RetryBaseDelay:           time.Second,
		RefreshLookahead:         5 * time.Minute,
		BackgroundRefreshWorkers: 10,
		SweepDefaultPageSize:     100,
	}
}

// NewConfigFromPorts builds OAuth2SessionService Config from application-wide configuration.
// This ensures the service uses configuration from the application's ConfigPort instead of hardcoded defaults.
// Constitution Principle VII (Configuration-Driven Design) compliance.
func NewConfigFromPorts(portsCfg ports.ThirdPartyOAuth2Config, refreshCfg ports.TokenRefreshConfig, callbackBaseURL string) Config {
	cfg := DefaultConfig()
	cfg.CallbackBaseURL = strings.TrimRight(callbackBaseURL, "/")

	if portsCfg.StateTokenTTL > 0 {
		cfg.StateTokenTTL = portsCfg.StateTokenTTL
	}
	if portsCfg.PKCEVerifierLength > 0 {
		cfg.PKCEVerifierLength = portsCfg.PKCEVerifierLength
	}
	if refreshCfg.LookaheadDuration > 0 {
		cfg.RefreshLookahead = refreshCfg.LookaheadDuration
	}
	if refreshCfg.BackgroundWorkers > 0 {
		cfg.BackgroundRefreshWorkers = refreshCfg.BackgroundWorkers
	}
	if refreshCfg.Sweep.DefaultPageSize > 0 {
		cfg.SweepDefaultPageSize = refreshCfg.Sweep.DefaultPageSize
	}

	return cfg
}

// NewOAuth2SessionService creates a new OAuth2SessionService.
// Requires a ThirdpartyOAuth2ProviderService that conditionally decrypts confidential client
// secrets. Public providers retain an absent Secret and require no client credential.
func NewOAuth2SessionService(
	providerService *thirdparty.ThirdpartyOAuth2ProviderService,
	sessionRepo ports.UserSessionRepository,
	refreshRepo ports.UserSessionRefreshRepository,
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
	if config.RefreshLookahead == 0 {
		config.RefreshLookahead = 5 * time.Minute
	}
	if config.BackgroundRefreshWorkers == 0 {
		config.BackgroundRefreshWorkers = 10
	}
	if config.SweepDefaultPageSize == 0 {
		config.SweepDefaultPageSize = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "oauth2session")

	service := &OAuth2SessionService{
		providerService: providerService,
		sessionRepo:     sessionRepo,
		refreshRepo:     refreshRepo,
		grantRepo:       grantRepo,
		agentRepo:       agentRepo,
		encryption:      encryption,
		httpClient:      httpClient,
		jweTokenService: jweTokenService,
		config:          config,
		logger:          logger,
	}
	service.background = newBackgroundRefresher(service, config.BackgroundRefreshWorkers)
	return service
}

// Close stops accepting background refreshes and waits for admitted work to finish.
func (s *OAuth2SessionService) Close(ctx context.Context) error {
	return s.background.Close(ctx)
}

// WithCIMDAssertionSigner injects the narrow signer used only by CIMD confidential services.
func (s *OAuth2SessionService) WithCIMDAssertionSigner(signer ports.CIMDClientAssertionSigner) *OAuth2SessionService {
	s.cimdAssertionSigner = signer
	return s
}

func (s *OAuth2SessionService) auditCIMDTokenAcquisition(serviceID id.ServiceID, operation, outcome string) {
	s.logger.Info("CIMD client token acquisition", "service_id", serviceID, "operation", operation, "outcome", outcome)
}

// GetCallbackBaseURL returns the configured callback base URL.
// This is used for validating OAuth2 redirect URIs and constructing callback endpoints.
// Constitution Principle VII: Configuration-Driven Design
func (s *OAuth2SessionService) GetCallbackBaseURL() string {
	return s.config.CallbackBaseURL
}

// SessionRecoveryURL returns a browser entry point without OAuth2 redirect parameters.
func (s *OAuth2SessionService) SessionRecoveryURL() string {
	return strings.TrimRight(s.config.CallbackBaseURL, "/") + "/sessions"
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
	if err := entity.ValidateOutboundCredentials(); err != nil {
		return nil, fmt.Errorf("invalid outbound client authentication: %w", err)
	}

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
	if entity.IsPublicClient() || entity.IsCIMDConfidentialClient() {
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

// IsSafeOAuthErrorCode reports whether code is an RFC 6749 §5.2 token-endpoint error code that
// may be propagated from a third-party provider without exposing provider-controlled text.
func IsSafeOAuthErrorCode(code string) bool {
	switch code {
	case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type", "invalid_scope":
		return true
	default:
		return false
	}
}

func safeTokenExchangeError(err error) error {
	metadata := NewErrorMetadata(OperationCodeExchange, DetailProviderUnavailable)
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		status := 0
		if retrieveErr.Response != nil {
			status = retrieveErr.Response.StatusCode
		}
		detail := DetailProviderRejected
		switch {
		case status == http.StatusTooManyRequests || status >= 500 && status <= 599:
			detail = DetailProviderUnavailable
		case status >= 400 && status <= 499 && (retrieveErr.ErrorCode == "invalid_client" || retrieveErr.ErrorCode == "unauthorized_client"):
			detail = DetailProviderClientRejected
		}
		metadata = NewErrorMetadata(OperationCodeExchange, detail).WithProviderResponse(status, retrieveErr.ErrorCode)
	}
	return NewOperationError(metadata, errors.Join(ErrTokenExchange, err))
}

func tokenExchangeErrorIsPermanent(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	return errors.As(err, &retrieveErr) && IsSafeOAuthErrorCode(retrieveErr.ErrorCode)
}

func shouldRetryTokenExchange(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		if retrieveErr.ErrorCode == "invalid_grant" || retrieveErr.ErrorCode == "invalid_client" {
			return false
		}
		return retrieveErr.Response != nil &&
			retrieveErr.Response.StatusCode >= http.StatusInternalServerError && retrieveErr.Response.StatusCode < 600
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// exchangeCodeWithRetry exchanges authorization code for tokens with exponential backoff retry.
func (s *OAuth2SessionService) exchangeCodeWithRetry(
	ctx context.Context,
	config *oauth2.Config,
	service *model.ThirdpartyOAuth2ProviderEntity,
	code string,
	verifier string,
	authorizationParams map[string]string,
) (resultToken *oauth2.Token, resultErr error) {
	defer func() {
		if resultErr != nil && ctx.Err() != nil && ctx.Value(sharedRefreshContextKey{}) != true {
			resultErr = callerCanceledError(ctx, OperationCodeExchange, resultErr)
		}
	}()
	var lastErr error = NewOperationError(NewErrorMetadata(OperationCodeExchange, DetailConfiguration), ErrInvalidConfiguration)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)

	for attempt := range s.config.MaxRetries {
		// Try to exchange code for token
		params := url.Values{}
		addProviderAuthorizationParams(params, authorizationParams)
		opts := make([]oauth2.AuthCodeOption, 0, len(params)+3)
		opts = append(opts, oauth2.VerifierOption(verifier))
		for name, values := range params {
			opts = append(opts, oauth2.SetAuthURLParam(name, values[0]))
		}
		if service.IsCIMDConfidentialClient() {
			if s.cimdAssertionSigner == nil {
				return nil, NewOperationError(NewErrorMetadata(OperationCodeExchange, DetailConfiguration).WithDependency(DependencySigning), ports.ErrCIMDKeyUnavailable)
			}
			assertion, err := s.cimdAssertionSigner.SignClientAssertion(ctx, service.ClientID, service.Endpoints.TokenEndpoint)
			if err != nil {
				failure := sessionOperationError(ctx, OperationCodeExchange, DetailProviderUnavailable, errors.Join(ports.ErrCIMDKeyUnavailable, err))
				return nil, failure.WithMetadata(failure.Metadata().WithDependency(DependencySigning))
			}
			opts = append(opts,
				oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
				oauth2.SetAuthURLParam("client_assertion", assertion),
			)
		}
		token, err := config.Exchange(ctx, code, opts...)
		if err == nil {
			return token, nil
		}
		lastErr = safeTokenExchangeError(err)
		if service.IsCIMDConfidentialClient() && tokenExchangeErrorIsPermanent(err) {
			return nil, lastErr
		}

		if ctx.Err() != nil {
			return nil, sessionOperationError(ctx, OperationCodeExchange, DetailCallerCanceled, errors.Join(lastErr, ctx.Err()))
		}
		if attempt == s.config.MaxRetries-1 || !shouldRetryTokenExchange(err) {
			break
		}

		// Calculate exponential backoff delay
		delay := s.config.RetryBaseDelay * time.Duration(math.Pow(2, float64(attempt)))
		s.logger.Warn("token exchange failed, retrying",
			"attempt", attempt+1,
			"delay", delay,
			"reason", "token_exchange_failed",
			"service_id", service.ID,
			"oauth2_session", sessionFailureMetadata(lastErr))

		// Sleep with context cancellation support
		select {
		case <-time.After(delay):
			// Continue to next retry
		case <-ctx.Done():
			return nil, sessionOperationError(ctx, OperationCodeExchange, DetailCallerCanceled, errors.Join(lastErr, ctx.Err()))
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
	s.logger.Info("initiating OAuth2 flow", "service_id", serviceID)
	if len(redirectURI) >= maxProviderStateBytes {
		return nil, ErrStateTokenTooLarge
	}

	// Fetch the service (with decrypted client secret via service manager)
	service, err := s.providerService.GetForTokenAcquisition(ctx, serviceID)
	if err != nil {
		failure := tokenAcquisitionError(ctx, OperationCodeExchange, err)
		s.logger.ErrorContext(ctx, "failed to fetch service for authorization", "service_id", serviceID, "oauth2_session", failure.Metadata())
		return nil, failure
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
		s.logger.ErrorContext(ctx, "failed to create state token", "service_id", serviceID, "oauth2_session", NewErrorMetadata(OperationCodeExchange, DetailEncryptionFailed))
		return nil, fmt.Errorf("failed to create state token: %w", err)
	}
	if len(stateToken) >= maxProviderStateBytes {
		return nil, ErrStateTokenTooLarge
	}

	// Build OAuth2 config with callback URL
	callbackURL := s.config.CallbackBaseURL + "/api/third-party/" + serviceID.String() + "/oauth2/callback"
	cfg, err := s.buildOAuth2Config(service, callbackURL)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to build oauth2 config", "service_id", serviceID, "oauth2_session", NewErrorMetadata(OperationCodeExchange, DetailConfiguration))
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
		failure := sessionOperationError(ctx, OperationCodeExchange, DetailRepositoryUnavailable, err)
		s.logger.ErrorContext(ctx, "failed to query existing session", "service_id", serviceID, "oauth2_session", failure.Metadata())
		return nil, failure
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
		failure := sessionOperationError(ctx, OperationCodeExchange, DetailEncryptionFailed, errors.Join(ErrEncryptionFailed, err))
		s.logger.ErrorContext(ctx, "failed to encrypt access token", "service_id", serviceID, "oauth2_session", failure.Metadata())
		return nil, failure
	}

	// Encrypt refresh token if present
	if token.RefreshToken != "" {
		encryptedRefresh, err = s.encryption.Encrypt(ctx, []byte(token.RefreshToken), encryptionContext)
		if err != nil {
			failure := sessionOperationError(ctx, OperationCodeExchange, DetailEncryptionFailed, errors.Join(ErrEncryptionFailed, err))
			s.logger.ErrorContext(ctx, "failed to encrypt refresh token", "service_id", serviceID, "oauth2_session", failure.Metadata())
			return nil, failure
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
		failure := sessionOperationError(ctx, OperationCodeExchange, DetailPersistenceFailed, err)
		s.logger.ErrorContext(ctx, "failed to create session", "service_id", serviceID, "oauth2_session", failure.Metadata())
		return nil, failure
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
	s.logger.Debug("processing OAuth2 callback", "service_id", req.ServiceID)

	// Check for OAuth2 error response
	if req.Error != "" {
		metadata := NewErrorMetadata(OperationCodeExchange, DetailProviderRejected).WithProviderResponse(0, req.Error)
		s.logger.WarnContext(ctx, "OAuth2 authorization failed", "service_id", req.ServiceID, "oauth2_session", metadata)
		return nil, NewOperationError(metadata, fmt.Errorf("OAuth2 authorization failed: %s: %s", req.Error, req.ErrorDesc))
	}

	// Validate state token (checks expiration, principal mismatch, tampering)
	claims, err := s.ValidateStateToken(req.State, principal, req.ServiceID)
	if err != nil {
		s.logger.ErrorContext(ctx, "state token validation failed", "service_id", req.ServiceID, "oauth2_session", NewErrorMetadata(OperationCodeExchange, DetailConfiguration))
		return nil, fmt.Errorf("state token validation failed: %w", err)
	}

	// Fetch the service (with decrypted client secret via service manager)
	service, err := s.providerService.GetForTokenAcquisition(ctx, req.ServiceID)
	if err != nil {
		failure := tokenAcquisitionError(ctx, OperationCodeExchange, err)
		s.logger.ErrorContext(ctx, "service not found during callback", "service_id", req.ServiceID, "oauth2_session", failure.Metadata())
		return nil, failure
	}

	// Build OAuth2 config
	callbackURL := s.config.CallbackBaseURL + "/api/third-party/" + req.ServiceID.String() + "/oauth2/callback"
	cfg, err := s.buildOAuth2Config(service, callbackURL)
	if err != nil {
		failure := sessionOperationError(ctx, OperationCodeExchange, DetailConfiguration, errors.Join(ErrInvalidConfiguration, err))
		s.logger.ErrorContext(ctx, "failed to build oauth2 config during callback", "service_id", req.ServiceID, "oauth2_session", failure.Metadata())
		return nil, failure
	}

	// Exchange authorization code for tokens (with retry)
	s.logger.InfoContext(ctx, "exchanging authorization code for token", "service_id", req.ServiceID, "operation", OperationCodeExchange)
	token, err := s.exchangeCodeWithRetry(ctx, cfg, service, req.Code, claims.PKCEVerifier, service.AuthorizationParams)
	if err != nil {
		if service.IsCIMDConfidentialClient() {
			s.auditCIMDTokenAcquisition(service.ID, "code_exchange", "rejected")
		}
		// Audit log: PKCE validation failure (token exchange failure typically indicates PKCE error)
		s.logger.Error("oauth2_pkce_validation_failed",
			"event", "session.oauth2.pkce_validation_failed",
			"service_id", req.ServiceID,
			"public_client", service.IsPublicClient(),
			"reason", "token_exchange_failed",
			"oauth2_session", sessionFailureMetadata(err),
			"timestamp", time.Now().Unix())
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}
	if service.IsCIMDConfidentialClient() {
		s.auditCIMDTokenAcquisition(service.ID, "code_exchange", "success")
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
		s.logger.ErrorContext(ctx, "failed to create session from token", "service_id", req.ServiceID, "oauth2_session", sessionFailureMetadata(err))
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
) (resultToken *oauth2.Token, resultErr error) {
	defer func() {
		if resultErr != nil && ctx.Err() != nil && ctx.Value(sharedRefreshContextKey{}) != true {
			resultErr = callerCanceledError(ctx, OperationRefresh, resultErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, sessionOperationError(ctx, OperationRefresh, DetailProviderUnavailable, err)
	}
	if entity == nil {
		return nil, sessionOperationError(ctx, OperationRefresh, DetailConfiguration, errors.New("service entity cannot be nil"))
	}

	isCIMDClient := entity.IsCIMDConfidentialClient()
	if err := entity.ValidateOutboundCredentials(); err != nil {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, sessionOperationError(ctx, OperationRefresh, DetailConfiguration, fmt.Errorf("%w: %w", ErrInvalidConfiguration, err))
	}
	if refreshToken == "" {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, sessionOperationError(ctx, OperationRefresh, DetailRefreshUnavailable, ErrRefreshNotAvailable)
	}

	isPublicClient := entity.IsPublicClient()
	var clientSecret string
	if !isPublicClient && !isCIMDClient {
		var err error
		clientSecret, err = entity.Secret.GetPlaintext()
		if err != nil {
			return nil, sessionOperationError(ctx, OperationRefresh, DetailDecryptionFailed, err)
		}
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", entity.ClientID.String())
	if !isPublicClient && !isCIMDClient {
		data.Set("client_secret", clientSecret)
	}
	addProviderAuthorizationParams(data, entity.AuthorizationParams)
	if isCIMDClient {
		if s.cimdAssertionSigner == nil {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
			return nil, NewOperationError(NewErrorMetadata(OperationRefresh, DetailConfiguration).WithDependency(DependencySigning), ports.ErrCIMDKeyUnavailable)
		}
		assertion, err := s.cimdAssertionSigner.SignClientAssertion(ctx, entity.ClientID, entity.Endpoints.TokenEndpoint)
		if err != nil {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
			failure := sessionOperationError(ctx, OperationRefresh, DetailProviderUnavailable, errors.Join(ports.ErrCIMDKeyUnavailable, err))
			return nil, failure.WithMetadata(failure.Metadata().WithDependency(DependencySigning))
		}
		data.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		data.Set("client_assertion", assertion)
	}

	// Create POST request to token endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, entity.Endpoints.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, sessionOperationError(ctx, OperationRefresh, DetailConfiguration, err)
	}

	// Set standard OAuth2 headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Execute the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, sessionOperationError(ctx, OperationRefresh, DetailProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, thirdpartyRefreshError(resp)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token,omitempty"`
		Scope        string `json:"scope,omitempty"`
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxTokenResponseBytes + 1}
	if err := json.NewDecoder(limited).Decode(&tokenResp); err != nil {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, refreshResponseError(ctx, resp.StatusCode, err)
	}
	if _, err := io.Copy(io.Discard, limited); err != nil {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, refreshResponseError(ctx, resp.StatusCode, err)
	}
	if limited.N == 0 {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, refreshResponseError(ctx, resp.StatusCode, fmt.Errorf("upstream token response exceeds %d byte limit", maxTokenResponseBytes))
	}

	// Validate required fields in response
	if tokenResp.AccessToken == "" {
		if isCIMDClient {
			s.auditCIMDTokenAcquisition(entity.ID, "refresh", "rejected")
		}
		return nil, refreshResponseError(ctx, resp.StatusCode, errors.New("upstream token response missing access_token"))
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
	if isCIMDClient {
		s.auditCIMDTokenAcquisition(entity.ID, "refresh", "success")
	}

	s.logger.InfoContext(ctx, "access token refreshed", "service_id", entity.ID, "operation", OperationRefresh)

	return token, nil
}

const maxThirdpartyErrorBodyBytes = 64 << 10

// thirdpartyRefreshError keeps only the HTTP status and an allowlisted OAuth error code so that
// provider-controlled descriptions, URIs, headers, and bodies never leave this function.
func thirdpartyRefreshError(resp *http.Response) error {
	retrieveErr := &oauth2.RetrieveError{
		Response: &http.Response{
			StatusCode: resp.StatusCode,
			Status:     fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
		},
	}
	oauthCode := ""
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxThirdpartyErrorBodyBytes+1))
	if err == nil && len(body) <= maxThirdpartyErrorBodyBytes {
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &payload) == nil {
			oauthCode = payload.Error
			if IsSafeOAuthErrorCode(payload.Error) {
				retrieveErr.ErrorCode = payload.Error
			}
		}
	}
	detail := DetailProviderRejected
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 && resp.StatusCode <= 599:
		detail = DetailProviderUnavailable
	case err != nil:
		detail = DetailProviderUnavailable
	case resp.StatusCode >= 400 && resp.StatusCode <= 499 && retrieveErr.ErrorCode == "invalid_grant":
		detail = DetailRefreshRejected
	case resp.StatusCode >= 400 && resp.StatusCode <= 499 && (retrieveErr.ErrorCode == "invalid_client" || retrieveErr.ErrorCode == "unauthorized_client"):
		detail = DetailProviderClientRejected
	}
	metadata := NewErrorMetadata(OperationRefresh, detail).WithProviderResponse(resp.StatusCode, oauthCode)
	return NewOperationError(metadata, errors.Join(&RefreshRejectedError{StatusCode: resp.StatusCode, OAuthError: retrieveErr.ErrorCode}, retrieveErr, err))
}

func (s *OAuth2SessionService) setSessionTokens(ctx context.Context, session *storage.UserSession, newToken *oauth2.Token) error {
	// Build encryption context for this session - uses service_id only
	serviceSubject := domainencryption.NewServiceBranchKeySubject(session.ServiceID)
	encContext := serviceSubject.EncryptionContext()

	// Encrypt new access token
	encryptedAccess, err := s.encryption.Encrypt(ctx, []byte(newToken.AccessToken), encContext)
	if err != nil {
		return sessionOperationError(ctx, OperationRefresh, DetailEncryptionFailed, errors.Join(ErrEncryptionFailed, err))
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
			return sessionOperationError(ctx, OperationRefresh, DetailEncryptionFailed, errors.Join(ErrEncryptionFailed, err))
		}
		session.EncryptedRefreshToken = encryptedRefresh
	}

	// Update timestamp
	session.UpdatedAt = time.Now()

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
		s.logger.Error("failed to list sessions", "err", err)
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
		"service_id", serviceID)

	// Step 1: Fetch session to verify it exists and ownership
	session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		s.logger.Error("failed to fetch session", "service_id", serviceID, "err", err)
		return nil, fmt.Errorf("session details retrieval failed: %w", ErrSessionNotFound)
	}

	if session == nil {
		return nil, ErrSessionNotFound
	}

	// Step 2: Verify principal ownership (authorization)
	if session.Principal != principal {
		s.logger.Error("principal mismatch in GetSessionWithAgents",
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
//   - OperationError preserves retrieval, decryption, and persistence causes with bounded metadata
//
// Per SR-005 (Security Rule): Token values are never included in error messages.
// Only metadata (service, expiration times) is included.
func (s *OAuth2SessionService) GetValidAccessToken(
	ctx context.Context,
	principal id.Principal,
	serviceID id.ServiceID,
) (resultSession *storage.UserSession, resultToken string, resultErr error) {
	defer func() {
		if resultErr != nil && ctx.Err() != nil {
			resultErr = callerCanceledError(ctx, OperationSessionLookup, resultErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, "", sessionOperationError(ctx, OperationSessionLookup, DetailCallerCanceled, err)
	}
	session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			return nil, "", sessionOperationError(ctx, OperationSessionLookup, DetailSessionMissing, errors.Join(ErrSessionNotFound, err))
		}
		return nil, "", sessionOperationError(ctx, OperationSessionLookup, DetailRepositoryUnavailable, err)
	}
	if session == nil {
		return nil, "", sessionOperationError(ctx, OperationSessionLookup, DetailSessionMissing, ErrSessionNotFound)
	}
	if session.HasValidAccessToken() {
		accessToken, err := s.DecryptAccessToken(ctx, session)
		if err != nil {
			return nil, "", sessionOperationError(ctx, OperationSessionLookup, DetailDecryptionFailed, err)
		}
		if session.CanRefresh() && session.AccessTokenExpiresBy(time.Now().Add(s.config.RefreshLookahead)) {
			s.background.submit(ctx, session, s.config.RefreshLookahead)
		}
		return session, accessToken, nil
	}

	key := principal.String() + "|" + serviceID.String()
	if err := ctx.Err(); err != nil {
		return nil, "", sessionOperationError(ctx, OperationRefresh, DetailCallerCanceled, err)
	}
	resultCh := s.refreshGroup.DoChan(key, func() (any, error) {
		refreshCtx, cancel := s.refreshOperationContext(context.WithoutCancel(ctx))
		defer cancel()
		current, refreshed, provider, err := s.refreshDueSession(refreshCtx, principal, serviceID, time.Now(), RefreshTriggerOnDemand)
		flight := refreshFlightResult{session: current, refreshed: refreshed, trigger: RefreshTriggerOnDemand, client: clientInfoFromProvider(provider)}
		if err != nil {
			if !errors.Is(err, ErrSessionNotFound) {
				s.logRefreshFailure(refreshCtx, session.ID, serviceID, RefreshTriggerOnDemand, flight.client, err)
			}
			return flight, err
		}
		return flight, nil
	})
	select {
	case <-ctx.Done():
		return nil, "", sessionOperationError(ctx, OperationRefresh, DetailCallerCanceled, ctx.Err())
	case outcome := <-resultCh:
		if err := ctx.Err(); err != nil {
			return nil, "", sessionOperationError(ctx, OperationRefresh, DetailCallerCanceled, err)
		}
		flight := outcome.Val.(refreshFlightResult)
		if outcome.Err != nil {
			if flight.trigger == RefreshTriggerBackground && !errors.Is(outcome.Err, ErrSessionNotFound) {
				s.logRefreshFailure(ctx, session.ID, serviceID, RefreshTriggerOnDemand, flight.client, outcome.Err)
			}
			return nil, "", outcome.Err
		}
		current := flight.session
		token, err := s.DecryptAccessToken(ctx, current)
		if err != nil {
			return nil, "", sessionOperationError(ctx, OperationRefresh, DetailDecryptionFailed, err)
		}
		return current, token, nil
	}
}

type refreshClientInfo struct {
	known  bool
	public bool
}

func clientInfoFromProvider(provider *model.ThirdpartyOAuth2ProviderEntity) refreshClientInfo {
	if provider == nil {
		return refreshClientInfo{}
	}
	return refreshClientInfo{known: true, public: provider.IsPublicClient()}
}

func (s *OAuth2SessionService) refreshDueSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, threshold time.Time, trigger RefreshTrigger) (*storage.UserSession, bool, *model.ThirdpartyOAuth2ProviderEntity, error) {
	return s.refreshLockedSession(ctx, principal, serviceID, &threshold, trigger)
}

// A nil threshold forces renewal; otherwise the latest locked row must still be due.
func (s *OAuth2SessionService) refreshLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, threshold *time.Time, trigger RefreshTrigger) (*storage.UserSession, bool, *model.ThirdpartyOAuth2ProviderEntity, error) {
	provider, providerErr := s.getRefreshProvider(ctx, serviceID)
	refreshed := false
	current, err := s.refreshRepo.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, current *storage.UserSession) (bool, error) {
		if current == nil {
			return false, sessionOperationError(ctx, OperationSessionLookup, DetailSessionMissing, ErrSessionNotFound)
		}
		if threshold != nil && !current.AccessTokenExpiresBy(*threshold) {
			return false, nil
		}
		if !current.CanRefresh() {
			if len(current.EncryptedRefreshToken) > 0 {
				cause := error(ErrRefreshTokenExpired)
				if threshold == nil {
					cause = errors.Join(ErrRefreshNotAvailable, cause)
				} else {
					cause = errors.Join(ErrSessionExpired, cause)
				}
				return false, sessionOperationError(ctx, OperationRefresh, DetailRefreshTokenExpired, cause)
			}
			if threshold == nil {
				return false, sessionOperationError(ctx, OperationRefresh, DetailRefreshUnavailable, ErrRefreshNotAvailable)
			}
			detail := DetailRefreshUnavailable
			if current.AccessTokenExpiresAt != nil && !current.AccessTokenExpiresAt.After(time.Now()) {
				detail = DetailAccessTokenExpired
			}
			return false, sessionOperationError(ctx, OperationRefresh, detail, errors.Join(ErrSessionExpired, ErrRefreshNotAvailable))
		}
		if providerErr != nil {
			return false, providerErr
		}
		if err := s.refreshSessionTokens(ctx, current, provider); err != nil {
			return false, err
		}
		refreshed = true
		return true, nil
	})
	if err != nil {
		detail := DetailRepositoryUnavailable
		if refreshed {
			detail = DetailPersistenceFailed
		}
		return nil, false, provider, sessionRepositoryError(ctx, detail, err)
	}
	if current == nil {
		return nil, false, provider, sessionOperationError(ctx, OperationSessionLookup, DetailSessionMissing, ErrSessionNotFound)
	}
	if refreshed {
		s.logRefreshSuccess(ctx, current.ID, serviceID, trigger, provider)
	}
	return current, refreshed, provider, nil
}

func (s *OAuth2SessionService) refreshOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	upstreamTimeout := s.httpClient.Timeout
	if upstreamTimeout <= 0 {
		upstreamTimeout = 30 * time.Second
	}
	ctx = context.WithValue(ctx, sharedRefreshContextKey{}, true)
	return context.WithTimeout(ctx, upstreamTimeout+s.config.RefreshStorageTimeout+time.Minute)
}

func (s *OAuth2SessionService) getRefreshProvider(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	service, err := s.providerService.GetForTokenAcquisition(ctx, serviceID)
	if err != nil {
		failure := tokenAcquisitionError(ctx, OperationRefresh, err)
		s.logger.ErrorContext(ctx, "failed to fetch service for token refresh", "service_id", serviceID, "oauth2_session", failure.Metadata())
		return nil, failure
	}
	return service, nil
}

// refreshSessionTokens exchanges the stored refresh token and encrypts the result.
// The caller holds the session lock and persists the modified session atomically.
// Precondition: session.CanRefresh() is true.
func (s *OAuth2SessionService) refreshSessionTokens(ctx context.Context, session *storage.UserSession, service *model.ThirdpartyOAuth2ProviderEntity) error {
	serviceID := session.ServiceID

	refreshToken, err := s.DecryptRefreshToken(ctx, session)
	if err != nil {
		failure := sessionOperationError(ctx, OperationRefresh, DetailDecryptionFailed, err)
		s.logger.ErrorContext(ctx, "failed to decrypt refresh token", "service_id", serviceID, "oauth2_session", failure.Metadata())
		return failure
	}

	if refreshToken == "" {
		return sessionOperationError(ctx, OperationRefresh, DetailRefreshUnavailable, ErrRefreshNotAvailable)
	}

	newToken, err := s.RefreshAccessToken(ctx, service, refreshToken)
	if err != nil {
		return NewOperationError(sessionFailureMetadata(err), errors.Join(ErrRefreshFailed, err))
	}

	if newToken == nil {
		return sessionOperationError(ctx, OperationRefresh, DetailProviderResponseInvalid, errors.New("upstream provider returned nil token"))
	}

	if err := s.setSessionTokens(ctx, session, newToken); err != nil {
		s.logger.ErrorContext(ctx, "failed to update session with refreshed tokens", "service_id", serviceID, "oauth2_session", sessionFailureMetadata(err))
		return err
	}

	return nil
}

func (s *OAuth2SessionService) logRefreshSuccess(ctx context.Context, sessionID id.SessionID, serviceID id.ServiceID, trigger RefreshTrigger, service *model.ThirdpartyOAuth2ProviderEntity) {
	s.logger.InfoContext(ctx, "oauth2_token_refreshed",
		"event", "session.oauth2.token_refreshed",
		"session_id", sessionID.String(),
		"service_id", serviceID.String(),
		"triggered_by", trigger,
		"public_client", service.IsPublicClient(),
		"reason", "token_refresh_succeeded",
		"timestamp", time.Now().Unix())
}

func (s *OAuth2SessionService) logRefreshFailure(ctx context.Context, sessionID id.SessionID, serviceID id.ServiceID, trigger RefreshTrigger, client refreshClientInfo, err error) {
	attrs := []any{
		"event", "session.oauth2.refresh_failed",
		"session_id", sessionID.String(),
		"service_id", serviceID.String(),
		"triggered_by", trigger,
		"oauth2_session", sessionFailureMetadata(err),
		"timestamp", time.Now().Unix(),
	}
	if client.known {
		attrs = append(attrs, "public_client", client.public)
	}
	s.logger.ErrorContext(ctx, "oauth2_refresh_failed", attrs...)
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
) (summary *storage.UserSessionSummary, resultErr error) {
	callerCtx := ctx
	defer func() {
		if resultErr != nil && callerCtx.Err() != nil {
			resultErr = callerCanceledError(callerCtx, OperationRefresh, resultErr)
		}
	}()
	ctx, cancel := s.refreshOperationContext(ctx)
	defer cancel()
	existing, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	if ports.IsNotFoundErr(err) || existing == nil && err == nil {
		return nil, sessionOperationError(ctx, OperationSessionLookup, DetailSessionMissing, errors.Join(ErrSessionNotFound, err))
	}
	if err != nil {
		return nil, sessionOperationError(ctx, OperationRefresh, DetailRepositoryUnavailable, err)
	}
	session, _, service, err := s.refreshLockedSession(ctx, principal, serviceID, nil, RefreshTriggerOnDemand)
	if err != nil {
		if !errors.Is(err, ErrSessionNotFound) {
			s.logRefreshFailure(ctx, existing.ID, serviceID, RefreshTriggerOnDemand, clientInfoFromProvider(service), err)
		}
		return nil, err
	}

	agentCount, err := s.grantRepo.CountAgentsByPrincipalAndServiceID(ctx, principal, serviceID)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to count agents after refresh", "service_id", serviceID, "oauth2_session", NewErrorMetadata(OperationRefresh, DetailRepositoryUnavailable).WithDependency(DependencySessionRepository))
		agentCount = 0
	}
	return storage.NewUserSessionSummary(session, service.DisplayName, agentCount), nil
}

type sharedRefreshContextKey struct{}

func sessionOperationError(ctx context.Context, operation Operation, detail ErrorDetail, cause error) *OperationError {
	if detail == DetailProviderResponseInvalid && (errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded)) {
		detail = DetailProviderUnavailable
	}
	if ctx.Err() != nil && ctx.Value(sharedRefreshContextKey{}) != true {
		detail = DetailCallerCanceled
		cause = errors.Join(cause, ctx.Err())
	}
	return NewOperationError(NewErrorMetadata(operation, detail), cause)
}

func refreshResponseError(ctx context.Context, status int, cause error) *OperationError {
	failure := sessionOperationError(ctx, OperationRefresh, DetailProviderResponseInvalid, cause)
	return failure.WithMetadata(failure.Metadata().WithProviderResponse(status, ""))
}

func sessionFailureMetadata(err error) ErrorMetadata {
	var failure *OperationError
	if errors.As(err, &failure) {
		return failure.Metadata()
	}
	return NewErrorMetadata(OperationUnknown, DetailInternalUnclassified)
}

func callerCanceledError(ctx context.Context, fallback Operation, cause error) *OperationError {
	metadata := sessionFailureMetadata(cause)
	if metadata.Operation() == OperationUnknown {
		metadata.operation = fallback
	}
	metadata.detail, metadata.kind = DetailCallerCanceled, KindCanceled
	return NewOperationError(metadata, errors.Join(cause, ctx.Err()))
}

func sessionRepositoryError(ctx context.Context, detail ErrorDetail, err error) *OperationError {
	var failure *OperationError
	if errors.As(err, &failure) {
		return NewOperationError(failure.Metadata(), err)
	}
	if ports.IsNotFoundErr(err) {
		return sessionOperationError(ctx, OperationSessionLookup, DetailSessionMissing, errors.Join(ErrSessionNotFound, err))
	}
	return sessionOperationError(ctx, OperationRefresh, detail, err)
}

func tokenAcquisitionError(ctx context.Context, operation Operation, cause error) *OperationError {
	detail, dependency := DetailRepositoryUnavailable, DependencyProviderRepository
	if ports.IsNotFoundErr(cause) {
		detail = DetailConfiguration
		cause = errors.Join(ErrServiceNotFound, cause)
	}
	if errors.Is(cause, thirdparty.ErrProviderConfiguration) {
		detail = DetailConfiguration
	}
	if errors.Is(cause, thirdparty.ErrSecretDecryption) {
		detail, dependency = DetailDecryptionFailed, DependencyEncryption
	}
	failure := sessionOperationError(ctx, operation, detail, cause)
	if failure.Metadata().Detail() == DetailCallerCanceled {
		return failure
	}
	return failure.WithMetadata(failure.Metadata().WithDependency(dependency))
}
