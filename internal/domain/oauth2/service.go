package oauth2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/oidcscope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// OAuth2Config contains configuration for the OAuth2 service
type OAuth2Config struct {
	// Upstream OAuth2 server authorization endpoint
	UpstreamAuthorizeEndpoint string

	// Upstream OAuth2 server token endpoint
	UpstreamTokenEndpoint string

	// Broker's public URL (from enduser ServerInstanceConfig.PublicURL)
	PublicURL string

	// IssuerURI is the JWT iss claim for locally-minted tokens. Defaults to PublicURL when
	// local.issuer_uri is not set. GenerateMetadata uses this so the RFC 8414 discovery
	// document advertises the same issuer that tokens actually carry.
	IssuerURI string

	// Supported response types (default: ["code"])
	SupportedResponseTypes []string

	// Supported grant types (default: ["authorization_code", "refresh_token"])
	SupportedGrantTypes []string

	// Supported scopes advertised in RFC 8414 metadata.
	SupportedScopes []string

	// MultiAgentClient holds optional multi-agent client sharing configuration.
	// When Enabled, multiple agents may share a single upstream OAuth2 client ID.
	MultiAgentClient ports.MultiAgentClientConfig

	// CIMDEnabled indicates whether CIMD-based client_id resolution is enabled.
	// When true, client_id_metadata_document_supported is advertised in metadata.
	CIMDEnabled bool

	// TokenExchangeEnabled indicates whether the RFC 8693 token exchange service is wired.
	// When true, the token-exchange grant type is appended to GrantTypesSupported in metadata.
	TokenExchangeEnabled bool

	// ModeStrategy enforces which agent client modes are permitted and drives metadata output.
	// Always required — pass NewProxyModeStrategy(), NewLocalModeStrategy(), or NewHybridModeStrategy().
	ModeStrategy ModeStrategy
}

// AuthorizationService implements the OAuth2Service port.
type AuthorizationService struct {
	grantRepo           ports.UserGrantRepository
	sessionRepo         ports.UserSessionRepository
	sessionUsability    ports.SessionUsabilityEvaluator
	clientResolver      ports.ClientResolver
	sessionTokenService *sessiontoken.Service
	config              *OAuth2Config
	logger              *slog.Logger
}

// NewAuthorizationService creates an AuthorizationService.
// Panics if sessionTokenService, sessionRepo, or config.ModeStrategy is nil.
func NewAuthorizationService(
	grantRepo ports.UserGrantRepository,
	sessionRepo ports.UserSessionRepository,
	clientResolver ports.ClientResolver,
	config *OAuth2Config,
	logger *slog.Logger,
	sessionTokenService *sessiontoken.Service,
) *AuthorizationService {
	if sessionTokenService == nil {
		panic("oauth2.NewAuthorizationService: sessionTokenService must not be nil")
	}
	if sessionRepo == nil {
		panic("oauth2.NewAuthorizationService: sessionRepo must not be nil")
	}
	if config == nil || config.ModeStrategy == nil {
		panic("oauth2.NewAuthorizationService: config.ModeStrategy must not be nil")
	}
	return &AuthorizationService{
		grantRepo:           grantRepo,
		sessionRepo:         sessionRepo,
		clientResolver:      clientResolver,
		config:              config,
		logger:              logger,
		sessionTokenService: sessionTokenService,
	}
}

func (s *AuthorizationService) WithSessionUsabilityEvaluator(evaluator ports.SessionUsabilityEvaluator) *AuthorizationService {
	s.sessionUsability = evaluator
	return s
}

// ResolveForTokenGrant resolves the client_id, classifies the agent, and enforces
// mode boundaries for the token endpoint. Follows the same universal resolution as
// HandleAuthorization (FR-003, FR-004, FR-005) but without authorization-specific
// logic (redirect URI, scopes, consent).
func (s *AuthorizationService) ResolveForTokenGrant(ctx context.Context, clientID id.ClientID) (*ports.TokenGrantResolution, error) {
	resolution, resolveErr := s.clientResolver.ResolveClient(ctx, clientID)
	if resolveErr != nil {
		var clientErr *ports.ClientIDError
		if errors.As(resolveErr, &clientErr) {
			return nil, clientErr
		}
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "unexpected error from client resolver", "error", resolveErr)
		}
		return nil, &ports.ClientIDError{Code: "server_error", Desc: "client resolution failed"}
	}

	agent := resolution.Agent
	mode := agent.ClientType()

	if !s.config.ModeStrategy.AcceptsClientType(mode) {
		return nil, &ports.ClientIDError{
			Code: "unauthorized_client",
			Desc: fmt.Sprintf("client mode not supported in %s mode", s.config.ModeStrategy.Mode()),
		}
	}

	return ports.NewTokenGrantResolution(agent.ID, agent.ClientID, mode)
}

// HandleAuthorization processes an OAuth2 authorization request
// Returns an AuthorizationDecision with either:
// - proceed: Valid client with active grant — handler decides next step
// - redirect_to_consent: Valid client but no active grant
// - error: Invalid client or server error
func (s *AuthorizationService) HandleAuthorization(ctx context.Context, req *ports.AuthorizationRequest, principal id.Principal) (*ports.AuthorizationDecision, error) {
	// Resolve the client via the injected ClientResolver strategy.
	var agent *storage.Agent
	var cimdMeta *ports.CIMDMetadataDTO

	resolution, resolveErr := s.clientResolver.ResolveClient(ctx, req.ClientID)
	if resolveErr != nil {
		code := "invalid_client"
		desc := "Client not registered"
		var clientErr *ports.ClientIDError
		if errors.As(resolveErr, &clientErr) {
			code = clientErr.Code
			desc = clientErr.Desc
		}
		return ports.ErrorDecision(code, desc, ""), nil
	}
	agent = resolution.Agent
	cimdMeta = resolution.CIMDMetadata

	// Mode enforcement: reject agents whose ClientType is not permitted in this server mode.
	if !s.config.ModeStrategy.AcceptsClientType(agent.ClientType()) {
		return ports.ErrorDecision("unauthorized_client", fmt.Sprintf("client mode not supported in %s mode", s.config.ModeStrategy.Mode()), ""), nil
	}

	// Step 1b: Validate redirect_uri.
	// For CIMD clients, validate against the document's redirect_uris; private-use
	// callbacks must also be listed in the agent's redirect_uris.
	// For opaque clients, validate against the agent's registered redirect_uris.
	// Per RFC 6749 §4.1.2.1, MUST NOT redirect if redirect_uri is unverified.
	var allowedRedirectURIs []string
	if cimdMeta != nil {
		allowedRedirectURIs = cimdMeta.RedirectURIs
	} else {
		allowedRedirectURIs = agent.RedirectURIs
	}

	// CIMD clients: redirect_uri failures are invalid_request (the CIMD contract specifies this).
	// Opaque clients: use the standard invalid_redirect_uri error code.
	redirectURIErrCode := "invalid_redirect_uri"
	if cimdMeta != nil {
		redirectURIErrCode = "invalid_request"
	}

	if len(allowedRedirectURIs) == 0 {
		return ports.ErrorDecision(redirectURIErrCode, "redirect_uri not registered for this client", ""), nil
	}
	if !urivalidation.IsRegisteredRedirectURI(allowedRedirectURIs, agent.RedirectURIs, req.RedirectURI) {
		return ports.ErrorDecision(redirectURIErrCode, "redirect_uri not registered for this client", ""), nil
	}

	// Step 1b-runtime: Reject invalid web and private-use callbacks even on legacy data.
	// Write-time validation (Agent.Validate/ValidateForCreate) prevents new invalid
	// registrations, but this guard closes the gap for pre-existing stored URIs.
	if !urivalidation.IsValidRedirectURI(req.RedirectURI) && !urivalidation.IsValidPrivateUseRedirectURI(req.RedirectURI) {
		return ports.ErrorDecision("invalid_redirect_uri", "redirect_uri must be an https URI, an http URI on a loopback host, or a valid private-use scheme URI", ""), nil
	}

	// Step 1c: Validate requested scopes against agent's allowed scopes.
	// redirect_uri is validated above, so a redirect-with-error is now safe.
	if len(agent.AllowedScopes) > 0 && req.Scope != "" {
		allowedSet := make(map[string]bool, len(agent.AllowedScopes))
		for _, scope := range agent.AllowedScopes {
			allowedSet[scope] = true
		}
		for _, scope := range strings.Fields(req.Scope) {
			if oidcscope.IsReservedRefreshTokenScope(scope) {
				continue
			}
			if !allowedSet[scope] {
				errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "invalid_scope", "requested scope is not permitted")
				if buildURLErr != nil && s.logger != nil {
					s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
				}
				return &ports.AuthorizationDecision{
					Action:      "error",
					ErrorCode:   "invalid_scope",
					ErrorDesc:   "requested scope is not permitted",
					RedirectURL: errRedirect,
				}, nil
			}
		}
	}

	// Step 2: Check if user has active grant for this agent
	grant, err := s.grantRepo.FindByPrincipalAndAgent(ctx, principal, agent.ID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		// redirect_uri is validated above so a redirect-with-error is safe here.
		errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "server_error", "server error")
		if buildURLErr != nil && s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
		}
		return &ports.AuthorizationDecision{
			Action:      "error",
			ErrorCode:   "server_error",
			ErrorDesc:   "Failed to check grant",
			RedirectURL: errRedirect,
		}, nil
	}

	// Step 3: Determine action based on grant status
	needsConsent := grant == nil || !grant.IsActive()
	if !needsConsent {
		for _, entry := range grant.GrantedPermissionSets {
			if !slices.ContainsFunc(agent.PermissionSets, func(declared storage.AgentPermissionSetEntry) bool {
				return declared.PermissionSetID == entry.PermissionSetID
			}) {
				needsConsent = true
				break
			}
		}
	}
	if needsConsent {
		consentURL, buildErr := s.buildConsentURL(ctx, req, principal, agent, cimdMeta)
		if buildErr != nil {
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to build consent URL", "error", buildErr)
			}
			errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "server_error", "server error")
			if buildURLErr != nil && s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
			}
			return &ports.AuthorizationDecision{
				Action:      "error",
				ErrorCode:   "server_error",
				ErrorDesc:   "Failed to initiate consent session",
				RedirectURL: errRedirect,
			}, nil
		}
		return ports.ConsentDecision(consentURL), nil
	}

	// Step 4: Check included services and mandatory requirements together so each
	// relevant session is read and identity-evaluated only once.
	unusableServiceID, unusableReason, err := s.anyRequiredSessionUnusable(ctx, principal, grant.GrantedPermissionSets, agent.ServiceRequirements)
	if err != nil {
		s.auditUsabilityFailure(ctx, principal, err)
		errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "server_error", "server error")
		if buildURLErr != nil && s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
		}
		return &ports.AuthorizationDecision{
			Action:      "error",
			ErrorCode:   "server_error",
			ErrorDesc:   "Failed to check session status",
			RedirectURL: errRedirect,
		}, nil
	}
	if unusableReason != "" {
		if s.logger != nil {
			s.logger.WarnContext(ctx,
				"DelegatedSessionUnusable",
				"agent_id", agent.ID,
				"principal", principal,
				"service_id", unusableServiceID,
				"reason", unusableReason,
			)
		}
		consentURL, buildErr := s.buildConsentURL(ctx, req, principal, agent, cimdMeta)
		if buildErr != nil {
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to build consent URL", "error", buildErr)
			}
			errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "server_error", "server error")
			if buildURLErr != nil && s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
			}
			return &ports.AuthorizationDecision{
				Action:      "error",
				ErrorCode:   "server_error",
				ErrorDesc:   "Failed to initiate consent session",
				RedirectURL: errRedirect,
			}, nil
		}
		return ports.ConsentDecision(consentURL), nil
	}

	// Active grant exists and all mandatory requirements satisfied — proceed.
	// For ProxyClient agents with an upstream endpoint, build the redirect URL.
	// For CIMDClient/LocalClient agents (or when no upstream is configured), leave empty
	// so the proceed strategy issues a local authorization code.
	var upstreamURL string
	if s.config.UpstreamAuthorizeEndpoint != "" && agent.ClientID != nil {
		var urlErr error
		upstreamURL, urlErr = s.buildUpstreamAuthorizeURL(ctx, req, agent)
		if urlErr != nil {
			errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "server_error", "server error")
			if buildURLErr != nil && s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
			}
			return &ports.AuthorizationDecision{
				Action:      "error",
				ErrorCode:   "server_error",
				ErrorDesc:   "Failed to build upstream authorize URL",
				RedirectURL: errRedirect,
			}, nil
		}
	}
	if upstreamURL == "" && s.config.UpstreamAuthorizeEndpoint != "" && agent.ClientType() == storage.ProxyClient {
		errRedirect, buildURLErr := BuildErrorRedirectURL(req.RedirectURI, req.State, "server_error", "agent missing upstream client_id")
		if buildURLErr != nil && s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to build error redirect URL", "redirect_uri", req.RedirectURI, "error", buildURLErr)
		}
		return &ports.AuthorizationDecision{
			Action:      "error",
			ErrorCode:   "server_error",
			ErrorDesc:   "Agent is not configured for upstream proxy (missing client_id)",
			RedirectURL: errRedirect,
		}, nil
	}
	return ports.ProceedDecision(upstreamURL, agent.ClientType()), nil
}

// buildUpstreamAuthorizeURL constructs the upstream authorization endpoint URL
// preserving all OAuth2 parameters.
//
// Feature 021: uses agent.ClientID (upstream OAuth2 client ID) instead of req.ClientID
// (which is now the broker's internal agent UUID). When MultiAgentClient.Enabled,
// appends the agent's internal UUID as the configured AgentIDParamName query parameter.
func (s *AuthorizationService) buildUpstreamAuthorizeURL(ctx context.Context, req *ports.AuthorizationRequest, agent *storage.Agent) (string, error) {
	u, err := url.Parse(s.config.UpstreamAuthorizeEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid upstream authorize endpoint URL: %w", err)
	}
	q := u.Query()

	// Use agent.ClientID as upstream client_id (NOT the broker's internal agent UUID)
	if agent.ClientID == nil {
		return "", fmt.Errorf("agent %s has no upstream client_id configured", agent.ID)
	}
	q.Set("client_id", agent.ClientID.String())
	q.Set("redirect_uri", req.RedirectURI)
	q.Set("response_type", req.ResponseType)

	// Add optional parameters if present
	if req.Scope != "" {
		q.Set("scope", req.Scope)
	}
	if req.State != "" {
		q.Set("state", req.State)
	}
	if req.CodeChallenge != "" {
		q.Set("code_challenge", req.CodeChallenge)
	}
	if req.CodeChallengeMethod != "" {
		q.Set("code_challenge_method", req.CodeChallengeMethod)
	}

	// Feature 021 — multi-agent client sharing: inject agent UUID param so upstream
	// can embed it as a claim in the returned token (for MultiAgentTokenVerifier).
	if s.config.MultiAgentClient.Enabled {
		q.Set(s.config.MultiAgentClient.AgentIDParamName, agent.ID.String())
		if s.logger != nil {
			s.logger.InfoContext(ctx, "AgentIDParamInjected",
				"agent_id", agent.ID.String(),
				"param_name", s.config.MultiAgentClient.AgentIDParamName,
				"upstream_url", s.config.UpstreamAuthorizeEndpoint,
			)
		}
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// buildConsentURL builds the consent redirect URL for a given agent and request.
// ALL agent modes (local, proxy, CIMD) receive a JWE session_token sealing the
// authorization context (agent_id, principal, original_url, TTL). CIMD agents
// additionally embed cimd_metadata; for local/proxy agents cimd_metadata is nil.
func (s *AuthorizationService) buildConsentURL(_ context.Context, req *ports.AuthorizationRequest, principal id.Principal, agent *storage.Agent, cimdMeta *ports.CIMDMetadataDTO) (string, error) {
	var meta *ports.SessionCIMDMetadata
	if cimdMeta != nil {
		meta = &ports.SessionCIMDMetadata{
			ClientID:     cimdMeta.ClientID,
			ClientName:   cimdMeta.ClientName,
			LogoURI:      cimdMeta.LogoURI,
			RedirectURIs: cimdMeta.RedirectURIs,
		}
		if err := meta.Validate(); err != nil {
			return "", fmt.Errorf("invalid CIMD metadata: %w", err)
		}
	}
	claims, err := sessiontoken.NewAuthorizationSessionClaims(agent.ID, principal, req.OriginalURL, meta)
	if err != nil {
		return "", fmt.Errorf("failed to create authorization session claims: %w", err)
	}
	token, err := s.sessionTokenService.Create(claims)
	if err != nil {
		return "", fmt.Errorf("failed to create authorization session token: %w", err)
	}
	return fmt.Sprintf("%s/agents/%s?session_token=%s", s.config.PublicURL, agent.ID, url.QueryEscape(token)), nil
}

// GenerateMetadata returns RFC 8414 OAuth2 metadata for this broker.
// In local and hybrid mode, includes JWKS URI and code_challenge_methods.
func (s *AuthorizationService) GenerateMetadata(ctx context.Context) (*ports.MetadataResponse, error) {
	issuer := s.config.IssuerURI
	if issuer == "" {
		issuer = s.config.PublicURL
	}

	const tokenExchangeGrant = "urn:ietf:params:oauth:grant-type:token-exchange" // #nosec G101 -- RFC-defined grant type URI, not a credential.
	var grantTypes []string
	if s.config.TokenExchangeEnabled {
		if !slices.Contains(s.config.SupportedGrantTypes, tokenExchangeGrant) {
			grantTypes = append(slices.Clone(s.config.SupportedGrantTypes), tokenExchangeGrant)
		} else {
			grantTypes = s.config.SupportedGrantTypes
		}
	} else {
		grantTypes = slices.DeleteFunc(slices.Clone(s.config.SupportedGrantTypes), func(g string) bool {
			return g == tokenExchangeGrant
		})
		if len(grantTypes) == 0 {
			switch s.config.ModeStrategy.Mode() {
			case servermode.Local, servermode.Hybrid:
				grantTypes = []string{"authorization_code", "client_credentials", "refresh_token"}
			default:
				grantTypes = []string{"authorization_code"}
			}
		}
	}

	metadata := &ports.MetadataResponse{
		Issuer:                            issuer,
		AuthorizationEndpoint:             fmt.Sprintf("%s/oauth2/authorize", issuer),
		TokenEndpoint:                     fmt.Sprintf("%s/oauth2/token", issuer),
		ResponseTypesSupported:            s.config.SupportedResponseTypes,
		GrantTypesSupported:               grantTypes,
		ScopesSupported:                   s.config.SupportedScopes,
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "client_secret_basic"},
	}

	// JWKS URI is always advertised — all modes serve /oauth2/jwks.json.
	metadata.JWKSURI = fmt.Sprintf("%s/oauth2/jwks.json", issuer)

	// Code challenge methods and client auth overrides only apply in modes that issue codes.
	if mode := s.config.ModeStrategy.Mode(); mode == servermode.Local || mode == servermode.Hybrid {
		metadata.CodeChallengeMethodsSupported = []string{"S256"}
		// client_secret_post is always supported for confidential clients.
		// none is included when CIMD is enabled: CIMD agents are public clients with no pre-registered secret.
		methods := []string{"client_secret_post"}
		if s.config.CIMDEnabled {
			methods = append([]string{"none"}, methods...)
			methods = append(methods, "private_key_jwt")
			metadata.TokenEndpointAuthSigningAlgValuesSupported = []string{"RS256", "ES256"}
		}
		metadata.TokenEndpointAuthMethodsSupported = methods
	}

	if s.config.CIMDEnabled {
		t := true
		metadata.ClientIDMetadataDocumentSupported = &t
	}

	return metadata, nil
}

// anyRequiredSessionUnusable checks included services and mandatory requirements.
// Excluded optional services never participate in authorization gating.
func (s *AuthorizationService) anyRequiredSessionUnusable(ctx context.Context, principal id.Principal, grantEntries []storage.GrantedPermissionSetEntry, requirements []storage.ServiceRequirement) (id.ServiceID, string, error) {
	relevantServices := make(map[id.ServiceID][]string)
	for _, entry := range grantEntries {
		for _, svcID := range entry.IncludedServiceIDs {
			relevantServices[svcID] = nil
		}
	}
	for _, requirement := range requirements {
		if requirement.IsMandatory() {
			relevantServices[requirement.ServiceID] = requirement.RequiredScopes
		}
	}

	// Use per-service lookup so sessions that require reauthentication stay visible.
	// ListActiveByPrincipal filters those out; FindByPrincipalAndService returns them.
	sessions := make([]*storage.UserSession, 0, len(relevantServices))
	for svcID, requiredScopes := range relevantServices {
		session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, svcID)
		if err != nil {
			return id.ServiceID{}, "", fmt.Errorf("failed to find session for service %s: %w", svcID, err)
		}
		if session == nil {
			return svcID, "session_missing", nil
		}
		if session.RequiresReauthentication() {
			return svcID, "reauthentication_required", nil
		}
		if !s.hasRequiredScopes(session.Scope, requiredScopes) {
			return svcID, "scope_mismatch", nil
		}
		sessions = append(sessions, session)
	}
	return s.firstUnusableSession(ctx, sessions)
}

func (s *AuthorizationService) firstUnusableSession(ctx context.Context, sessions []*storage.UserSession) (id.ServiceID, string, error) {
	if len(sessions) == 0 {
		return id.ServiceID{}, "", nil
	}
	if s.sessionUsability == nil {
		return id.ServiceID{}, "", &ports.SessionUsabilityError{ServiceID: sessions[0].ServiceID, Err: model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)}
	}
	unusable, err := s.sessionUsability.RequiresReauthentication(ctx, sessions, ports.UsabilityGate)
	if err != nil {
		return id.ServiceID{}, "", err
	}
	for _, session := range sessions {
		if unusable[session.ServiceID] {
			return session.ServiceID, "reauthentication_required", nil
		}
	}
	return id.ServiceID{}, "", nil
}

type usabilityOperation string
type usabilityOutcome string

const (
	usabilityGateOperation usabilityOperation = "usability_gate"
	usabilityFailedOutcome usabilityOutcome   = "failed"
)

func (s *AuthorizationService) auditUsabilityFailure(ctx context.Context, principal id.Principal, err error) {
	var failure *ports.SessionUsabilityError
	if s.logger == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || !errors.As(err, &failure) {
		return
	}
	reason, level := failure.ReasonAndLevel()
	s.logger.Log(ctx, level, "oauth2 credential source unavailable",
		"event", "session.oauth2.credential_source_failed",
		"principal", principal,
		"service_id", failure.ServiceID,
		"operation", usabilityGateOperation,
		"outcome", usabilityFailedOutcome,
		"reason", reason,
		"oauth2_session", failure)
}

// hasRequiredScopes checks if sessionScopes is a superset of requiredScopes (case-sensitive).
// Returns true if sessionScopes contains all scopes in requiredScopes, allowing for extra scopes.
// Returns true if requiredScopes is empty or nil (no requirements).
func (s *AuthorizationService) hasRequiredScopes(sessionScopes []string, requiredScopes []string) bool {
	// If no required scopes, always pass
	if len(requiredScopes) == 0 {
		return true
	}

	// If required scopes exist but session has none, fail
	if len(sessionScopes) == 0 {
		return false
	}

	// Build map of session scopes for efficient lookup
	sessionScopeMap := make(map[string]bool)
	for _, scope := range sessionScopes {
		sessionScopeMap[scope] = true
	}

	// Check that all required scopes exist in session scopes
	for _, required := range requiredScopes {
		if !sessionScopeMap[required] {
			return false
		}
	}

	return true
}
