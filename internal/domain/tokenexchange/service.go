// Package tokenexchange provides domain types and services for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// TokenExchangeService orchestrates the RFC 8693 token exchange flow.
// It coordinates validation, authorization, and token retrieval across multiple components.
//
// The service implements the complete token exchange flow:
// 1. Validate token exchange request structure
// 2. Validate subject_token JWT signature and claims
// 3. Validate client_assertion JWT signature and claims
// 4. Extract user principal and agent client ID from tokens
// 5. Authorize privileged client via CEL expression evaluation
// 6. Lookup target service by resource URI
// 7. Verify user has granted agent access to service
// 8. Retrieve stored third-party tokens for user
// 9. Check if tokens are fully expired
// 10. Refresh expired access token if refresh token available
// 11. Return RFC 8693 compliant response
//
// Thread-safe: All methods are safe for concurrent use. The service maintains
// no mutable state, only dependency references.
type TokenExchangeService struct {
	// jwtValidator validates JWT signatures and claims
	jwtValidator *JWTValidator

	// celEvaluator extracts claims and evaluates authorization policies
	celEvaluator *CELEvaluator

	// providerService provides service discovery by protected resources
	providerService *thirdparty.ThirdpartyOAuth2ProviderService

	// oauth2SessionService handles OAuth2 session lifecycle including token refresh
	oauth2SessionService *oauth2session.OAuth2SessionService

	// consentService verifies user has granted agent access
	consentService *consent.Service

	// permissionSetService resolves permission sets with caching (Phase 6 - US4)
	permissionSetService *permissionset.Service

	// agentRepository resolves agent client_id to internal UUID
	agentRepository ports.AgentRepository

	// config provides token exchange configuration
	config *ports.TokenExchangeConfig
}

// NewTokenExchangeService creates a new token exchange service.
// All dependencies are required and must not be nil.
//
// Parameters:
//   - jwtValidator: Validates JWT signatures and claims
//   - celEvaluator: Extracts claims and evaluates authorization policies
//   - providerService: Looks up services by protected resource
//   - oauth2SessionService: Handles OAuth2 session lifecycle including token refresh
//   - consentService: Verifies user grants and agent access
//   - permissionSetService: Resolves permission sets with caching (Phase 6 - US4)
//   - config: Token exchange configuration
//
// Returns error if any dependency is nil.
func NewTokenExchangeService(
	jwtValidator *JWTValidator,
	celEvaluator *CELEvaluator,
	providerService *thirdparty.ThirdpartyOAuth2ProviderService,
	oauth2SessionService *oauth2session.OAuth2SessionService,
	consentService *consent.Service,
	permissionSetService *permissionset.Service,
	agentRepository ports.AgentRepository,
	config *ports.TokenExchangeConfig,
) (*TokenExchangeService, error) {
	if jwtValidator == nil {
		return nil, fmt.Errorf("jwtValidator cannot be nil")
	}
	if celEvaluator == nil {
		return nil, fmt.Errorf("celEvaluator cannot be nil")
	}
	if providerService == nil {
		return nil, fmt.Errorf("providerService cannot be nil")
	}
	if oauth2SessionService == nil {
		return nil, fmt.Errorf("oauth2SessionService cannot be nil")
	}
	if consentService == nil {
		return nil, fmt.Errorf("consentService cannot be nil")
	}
	if permissionSetService == nil {
		return nil, fmt.Errorf("permissionSetService cannot be nil")
	}
	if agentRepository == nil {
		return nil, fmt.Errorf("agentRepository cannot be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	return &TokenExchangeService{
		jwtValidator:         jwtValidator,
		celEvaluator:         celEvaluator,
		providerService:      providerService,
		oauth2SessionService: oauth2SessionService,
		consentService:       consentService,
		permissionSetService: permissionSetService,
		agentRepository:      agentRepository,
		config:               config,
	}, nil
}

// DelegationResolver returns the resolver for exchange steps 2–9a over this service's
// dependencies. It is stateless, so access evaluation and exchange resolve identically.
func (s *TokenExchangeService) DelegationResolver() *DelegationResolver {
	var consentBaseURL string
	if s.oauth2SessionService != nil {
		consentBaseURL = s.oauth2SessionService.GetCallbackBaseURL()
	}
	return &DelegationResolver{
		jwtValidator:         s.jwtValidator,
		celEvaluator:         s.celEvaluator,
		providerService:      s.providerService,
		consentService:       s.consentService,
		permissionSetService: s.permissionSetService,
		agentRepository:      s.agentRepository,
		consentBaseURL:       consentBaseURL,
	}
}

// Exchange processes a complete token exchange request.
// PHASE 5+: Full implementation deferred to Phase 5+.
// For Phase 4, resource-based service discovery is implemented in HTTP handler.
// This is the main orchestration method that will implement the full token exchange flow.
//
// Flow:
// 1. Validate request structure (grant_type, required parameters)
// 2. Validate subject_token JWT (signature, issuer, audience, expiration)
// 3. Validate client_assertion JWT (signature, issuer, audience, expiration)
// 4. Extract principal from subject_token via CEL
// 5. Extract agent_id from subject_token via CEL
// 6. Authorize privileged client via CEL expression evaluation
// 7. Normalize resource URI (remove trailing slashes)
// 8. Lookup service by resource URI
// 9. Verify user has granted agent access to service (BEFORE session check per T078)
// 10. Retrieve user's session and stored tokens for service
// 11. Check if tokens are fully expired (T076)
// 12. Refresh expired tokens if refresh_token available
// 13. Return RFC 8693 compliant response
//
// Per spec FR-048, token exchange endpoint detection happens in HTTP layer
// (checking grant_type parameter). This service assumes it's processing
// a token exchange request and will fail if given non-token-exchange requests.
//
// Error handling:
// - InvalidRequest: malformed request, missing required parameters
// - InvalidClient: invalid/missing client_assertion
// - InvalidGrant: invalid/expired subject_token, no session, or all tokens expired (T075, T076)
// - InvalidTarget: no service matches resource URI
// - AccessDenied: user has not granted agent access, or CEL authorization failed
// - ServerError: configuration/system errors
//
// Per SR-005 (Security Rule): Token values are never included in error messages
// or logs. Only metadata (issuer, audience) and request context.
//
// Per SR-058 (Audit Logging): Caller is responsible for logging token exchange
// success/failure with context (principal, service, agent). This method does not log.
//
// PHASE 8 NOTES (T075-T078):
// - T075: Return invalid_grant when no UserSession exists for principal+service
// - T076: Return invalid_grant when both access_token and refresh_token expired
// - T077: Include service_id and re-auth hint in error_description
// - T078: CRITICAL - Grant check MUST occur BEFORE session check to prevent information leakage
func (s *TokenExchangeService) Exchange(ctx context.Context, req *TokenExchangeRequest) (_ *TokenExchangeResponse, err error) {
	var serviceRef ServiceRef
	var authRef AuthorizationRef
	stage := StageRequestValidation
	defer func() {
		if err == nil {
			return
		}
		var tokenErr *TokenExchangeError
		if !errors.As(err, &tokenErr) {
			tokenErr = NewServerErrorWithCause("token exchange failed", err)
		}
		diagnostic := tokenErr.Diagnostic()
		if diagnostic.Detail() == DetailInternalUnclassified {
			diagnostic = NewDiagnostic(stage, DetailInternalUnclassified)
		}
		var sessionErr *oauth2session.OperationError
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) && !errors.As(err, &sessionErr) {
			diagnostic = NewDiagnostic(diagnostic.Stage(), DetailCallerCanceled)
		}
		err = tokenErr.WithObservation(diagnostic, serviceRef, authRef)
	}()
	if ctx.Err() != nil {
		return nil, NewServerErrorWithCause("token exchange canceled", ctx.Err()).WithDiagnostic(NewDiagnostic(stage, DetailCallerCanceled))
	}

	// Step 1: Validate request structure
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Steps 2–9a: authoritative delegation resolution shared with access evaluation.
	delegation, err := s.DelegationResolver().Resolve(ctx, DelegationRequest{
		SubjectToken:    req.SubjectToken,
		ClientAssertion: req.ClientAssertion,
		Resource:        req.Resource,
		GrantType:       req.GrantType,
		Scope:           req.Scope,
	})
	if err != nil {
		var tokenErr *TokenExchangeError
		if errors.As(err, &tokenErr) {
			stage = tokenErr.Diagnostic().Stage()
			serviceRef = tokenErr.Service()
			authRef = tokenErr.Authorization()
		}
		return nil, err
	}
	principal, agent, service, grant := delegation.Principal, delegation.Agent, delegation.Service, delegation.Grant
	serviceRef = ServiceRef{ID: service.ID}
	authRef = authorizationRef(agent.ID, grant)
	serviceScopes := delegation.EffectiveScopes

	// Step 10: Get valid access token with session metadata (with transparent refresh if needed)
	// This single call handles:
	// - Fetching the session from storage
	// - Checking if access token is expired
	// - Automatically refreshing if refresh token is available
	// - Decrypting and returning valid token along with session metadata
	//
	// Per T075: invalid_grant if session doesn't exist
	// Per T076: invalid_grant if both tokens are expired
	stage = StageSessionLookup
	sessionObj, accessToken, err := s.oauth2SessionService.GetValidAccessToken(ctx, id.Principal(principal), service.ID)
	if err != nil {
		diagnostic := sessionDiagnostic(err)
		if diagnostic.Outcome() == OutcomeReauthRequired {
			return nil, NewInvalidGrantError("provider session is missing or unusable; re-authentication required").
				WithCause(err).WithErrorURI(s.oauth2SessionService.ServiceAuthorizeURL(service.ID)).WithDiagnostic(diagnostic)
		}
		return nil, NewServerErrorWithCause("failed to get valid access token", err).WithDiagnostic(diagnostic)
	}

	// Step 11: Validate the session's scopes cover the effective scopes for the requested
	// service (FR-012, FR-013). Requires the session, so it runs after retrieval.
	stage = StageScopeValidation
	if len(serviceScopes) > 0 {
		sessionScopeSet := make(map[string]bool, len(sessionObj.Scope))
		for _, scope := range sessionObj.Scope {
			sessionScopeSet[scope] = true
		}
		var missingScopes []string
		for _, scope := range serviceScopes {
			if !sessionScopeSet[scope] {
				missingScopes = append(missingScopes, scope)
			}
		}
		if len(missingScopes) > 0 {
			return nil, NewInvalidGrantError("provider session does not cover required scopes; re-authentication required").
				WithErrorURI(s.oauth2SessionService.ServiceAuthorizeURL(service.ID)).WithDiagnostic(NewDiagnostic(stage, DetailSessionScopeInsufficient))
		}
	}

	// Step 12: Build and return RFC 8693 response with permission set IDs (Phase 6 - US4)
	// Calculate expires_in from session's current access token expiration
	var expiresIn *int64
	if sessionObj.AccessTokenExpiresAt != nil {
		remaining := max(int64(0), int64(time.Until(*sessionObj.AccessTokenExpiresAt).Seconds()))
		expiresIn = &remaining
	}

	response := NewTokenExchangeResponseFull(
		accessToken,
		sessionObj.TokenType,
		AccessTokenType,
		"",
		strings.Join(sessionObj.Scope, " "),
		expiresIn,
	)

	// T051: Include granted_permission_sets in response (FR-012)
	if len(grant.GrantedPermissionSets) > 0 {
		psMap := make(map[string][]string, len(grant.GrantedPermissionSets))
		for _, entry := range grant.GrantedPermissionSets {
			svcIDs := make([]string, len(entry.IncludedServiceIDs))
			for i, svcID := range entry.IncludedServiceIDs {
				svcIDs[i] = svcID.String()
			}
			psMap[entry.PermissionSetID.String()] = svcIDs
		}
		response.GrantedPermissionSets = psMap
	}
	response.Principal = principal
	response.AgentID = agent.ID.String()
	response.Service = serviceRef
	response.Authorization = authRef

	return response, nil
}
