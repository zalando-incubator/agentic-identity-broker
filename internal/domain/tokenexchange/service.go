// Package tokenexchange provides domain types and services for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtclaims"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
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
func (s *TokenExchangeService) Exchange(ctx context.Context, req *TokenExchangeRequest) (*TokenExchangeResponse, error) {
	// Step 1: Validate request structure
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Step 2: Validate subject_token JWT
	subjectTokenJWT, err := s.jwtValidator.ValidateSubjectToken(ctx, req.SubjectToken)
	if err != nil {
		return nil, err
	}

	// Step 3: Validate client_assertion JWT
	clientAssertionJWT, err := s.jwtValidator.ValidateClientAssertion(ctx, req.ClientAssertion)
	if err != nil {
		return nil, err
	}

	// Step 4: Extract principal from subject_token via CEL
	subjectTokenClaims := jwtclaims.FromToken(subjectTokenJWT)
	principal, err := s.celEvaluator.ExtractPrincipal(subjectTokenClaims)
	if err != nil {
		return nil, err
	}

	// ADR 033 §3: finalize the request security context at the post-validation
	// token-exchange seam. The subject-token principal (Actor) and the validated
	// client_assertion subject (CallingPeer) are both known here. Finalizing before
	// the authorization decision guarantees a denied exchange still carries
	// actor/calling_peer on the security-sensitive failure audit path.
	callingPeer, _ := clientAssertionJWT.Subject()
	_, _ = security.FinalizeCaptureHolder(ctx, principal, callingPeer)

	// Step 5: Extract agent_id from subject_token via CEL
	agentID, err := s.celEvaluator.ExtractAgentID(subjectTokenClaims)
	if err != nil {
		return nil, err
	}

	// Step 6: Authorize privileged client via CEL expression evaluation
	clientAssertionClaims := jwtclaims.FromToken(clientAssertionJWT)
	requestContext := &CELRequestContext{
		Resource:  req.Resource,
		GrantType: req.GrantType,
		Scope:     req.Scope,
		Principal: principal,
		AgentID:   agentID,
	}
	authorized, err := s.celEvaluator.AuthorizePrivilegedClient(clientAssertionClaims, subjectTokenClaims, *requestContext)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, NewAccessDeniedError("privileged client authorization failed")
	}

	// Step 7: Normalize resource URI
	normalizedResource := Normalize(req.Resource)

	// Step 8: Lookup service by resource URI
	service, err := s.providerService.FindByProtectedResource(ctx, normalizedResource)
	if err != nil {
		if IsTokenExchangeError(err) {
			// InvalidTarget error from repository
			return nil, err
		}
		return nil, NewServerErrorWithCause("failed to lookup service by resource URI", err)
	}

	// Step 9: Verify user has granted agent access to service (T059-T065)
	// CRITICAL (T078): Grant verification MUST occur BEFORE session check
	// This prevents information leakage: if grant is missing, return access_denied (no permission).
	// Only if grant exists but session is missing do we return invalid_grant (no session).
	//
	// T059: Agent ID extracted from subject_token (already done in Step 5)
	// T060 (Feature 021): agentID is the broker-internal agent UUID (resolved by CEL).
	// Parse it as UUID and look up by primary key — no GetByClientID needed.
	parsedAgentID, parseErr := id.ParseAgentID(agentID)
	if parseErr != nil {
		return nil, NewInvalidRequestError(
			fmt.Sprintf("agent_id %q extracted from subject_token is not a valid agent UUID", agentID),
		)
	}
	agent, err := s.agentRepository.Get(ctx, parsedAgentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, NewAccessDeniedErrorWithDetails(
				"user has not granted permission for this agent to access the requested service",
				fmt.Sprintf("agent with id %q not found", agentID),
			)
		}
		return nil, NewServerErrorWithCause("failed to lookup agent by agent_id", err)
	}
	// T061-T065: Delegate grant verification to ConsentService using internal agent UUID
	// ConsentService.VerifyAgentAccess checks:
	// - T061: Query UserGrant by principal + agent UUID
	// - T062: Check grant status: active, not revoked, not expired
	// - T063: Return error for missing grant
	// - T064: Return error for revoked grant
	// - T065: Return error for expired grant
	grant, err := s.consentService.VerifyAgentAccess(ctx, id.Principal(principal), agent.ID)
	if err != nil {
		// Map ConsentService errors to TokenExchange errors
		if errors.Is(err, consent.ErrAgentAccessDenied) {
			// T063: User has not granted agent access
			return nil, NewAccessDeniedErrorWithDetails(
				"user has not granted permission for this agent to access the requested service",
				err.Error(),
			)
		}
		if errors.Is(err, consent.ErrGrantExpired) {
			// T065: User grant has expired
			return nil, NewAccessDeniedErrorWithDetails(
				"user grant has expired",
				err.Error(),
			)
		}
		// System/repository error
		return nil, NewServerErrorWithCause("failed to verify user grant", err)
	}

	// Step 9a: Authorize the requested service against the grant (FR-010, SR-007).
	// Fail closed for every agent: the grant must cover the requested service before any
	// session lookup or credential decryption. Agents that declare neither PermissionSets
	// nor ServiceRequirements are not exempt.
	if len(grant.GrantedPermissionSets) == 0 {
		return nil, NewInvalidGrantError("grant has no permission set entries; re-consent required")
	}
	effectiveScopes, err := s.resolveEffectiveScopes(ctx, grant, agent)
	if err != nil {
		return nil, err
	}
	serviceScopes, covered := effectiveScopes[service.ID]
	if !covered {
		return nil, NewInvalidGrantError(fmt.Sprintf(
			"service %s is not authorized by any permission set in the grant; re-consent required",
			service.ID,
		))
	}

	// Step 10: Get valid access token with session metadata (with transparent refresh if needed)
	// This single call handles:
	// - Fetching the session from storage
	// - Checking if access token is expired
	// - Automatically refreshing if refresh token is available
	// - Decrypting and returning valid token along with session metadata
	//
	// Per T075: invalid_grant if session doesn't exist
	// Per T076: invalid_grant if both tokens are expired
	sessionObj, accessToken, err := s.oauth2SessionService.GetValidAccessToken(ctx, id.Principal(principal), service.ID)
	if err != nil {
		// Map oauth2session errors to RFC 8693 token exchange errors
		if errors.Is(err, oauth2session.ErrSessionNotFound) {
			// T075: No session exists for this principal+service combination
			// T077: Include service info and re-auth hint in error_description
			description := fmt.Sprintf(
				"User has no active session with the requested service. Service: %s. Please re-authenticate to %s.",
				service.ID,
				service.DisplayName,
			)
			reAuthURL := s.oauth2SessionService.ServiceAuthorizeURL(service.ID)
			return nil, NewInvalidGrantError(description).WithErrorURI(reAuthURL)
		}
		if errors.Is(err, oauth2session.ErrSessionExpired) {
			// T076: Both access and refresh tokens are expired
			// T077: Include service info and re-auth hint in error_description
			description := fmt.Sprintf(
				"User session has expired. All tokens are no longer valid. Service: %s. Please re-authenticate to %s.",
				service.ID,
				service.DisplayName,
			)
			reAuthURL := s.oauth2SessionService.ServiceAuthorizeURL(service.ID)
			return nil, NewInvalidGrantError(description).WithErrorURI(reAuthURL)
		}
		// Other errors (refresh failed, decryption failed, etc)
		return nil, NewServerErrorWithCause("failed to get valid access token", err)
	}

	// Step 11: Validate the session's scopes cover the effective scopes for the requested
	// service (FR-012, FR-013). Requires the session, so it runs after retrieval.
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
			description := fmt.Sprintf(
				"User session does not cover all required scopes for service %s. Missing: %s. Please re-authenticate with the required scopes.",
				service.DisplayName,
				strings.Join(missingScopes, ", "),
			)
			reAuthURL := s.oauth2SessionService.ServiceAuthorizeURL(service.ID)
			return nil, NewInvalidGrantError(description).WithErrorURI(reAuthURL)
		}
	}

	// Step 12: Build and return RFC 8693 response with permission set IDs (Phase 6 - US4)
	// Calculate expires_in from session's current access token expiration
	now := time.Now().UTC()
	expiresIn := int64(0)
	if sessionObj.AccessTokenExpiresAt != nil && sessionObj.AccessTokenExpiresAt.After(now) {
		expiresIn = int64(sessionObj.AccessTokenExpiresAt.Sub(now).Seconds())
	}

	response := NewTokenExchangeResponseFull(
		accessToken,
		sessionObj.TokenType,
		AccessTokenType, // issued_token_type per RFC 8693
		"",              // refresh token typically not returned in exchange response per RFC 8693
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

	return response, nil
}

// resolveEffectiveScopes resolves permission sets from the grant and computes
// per-service effective scopes by taking the union across all permission sets
// and intersecting with the agent's service requirement scope ceiling (FR-013).
//
// Returns a map of service ID → effective scopes (only scopes present in both
// PS definitions and agent SR are included).
func (s *TokenExchangeService) resolveEffectiveScopes(
	ctx context.Context,
	grant *storage.UserGrant,
	agent *storage.Agent,
) (map[id.ServiceID][]string, error) {
	// Collect PS IDs from grant
	psIDs := make([]id.PermissionSetID, len(grant.GrantedPermissionSets))
	for i, entry := range grant.GrantedPermissionSets {
		psIDs[i] = entry.PermissionSetID
	}

	// Resolve permission sets (TTL cache transparent)
	resolvedSets, err := s.permissionSetService.GetByIDs(ctx, psIDs)
	if err != nil {
		return nil, NewServerErrorWithCause("failed to resolve permission sets", err)
	}

	// Index resolved PSets by ID for lookup
	psIndex := make(map[id.PermissionSetID]*storage.PermissionSet, len(resolvedSets))
	for _, ps := range resolvedSets {
		psIndex[ps.ID] = ps
	}

	// Build included service IDs index from grant entries
	grantIndex := make(map[id.PermissionSetID]map[id.ServiceID]bool, len(grant.GrantedPermissionSets))
	for _, entry := range grant.GrantedPermissionSets {
		included := make(map[id.ServiceID]bool, len(entry.IncludedServiceIDs))
		for _, svcID := range entry.IncludedServiceIDs {
			included[svcID] = true
		}
		grantIndex[entry.PermissionSetID] = included
	}

	// Compute per-service scope union across all permission sets,
	// filtering to only included_service_ids per grant entry
	perServiceScopes := make(map[id.ServiceID]map[string]bool)
	for _, entry := range grant.GrantedPermissionSets {
		ps, ok := psIndex[entry.PermissionSetID]
		if !ok {
			// Fail closed: any PS referenced by a stored grant must still exist.
			// Whether mandatory or optional, a missing PS means the grant is stale.
			return nil, NewInvalidGrantError(fmt.Sprintf(
				"permission set %s referenced in grant no longer exists; re-consent required",
				entry.PermissionSetID,
			))
		}
		included := grantIndex[entry.PermissionSetID]
		for _, ss := range ps.ServiceScopes {
			if !included[ss.ServiceID] {
				continue // Service not included in this grant entry
			}
			if perServiceScopes[ss.ServiceID] == nil {
				perServiceScopes[ss.ServiceID] = make(map[string]bool)
			}
			for _, scope := range ss.Scopes {
				perServiceScopes[ss.ServiceID][scope] = true
			}
		}
	}

	// Build per-service SR indexes: explicit-scope ceilings vs. all-scopes passthroughs.
	hasSR := len(agent.ServiceRequirements) > 0
	srCeiling := make(map[id.ServiceID]map[string]bool)
	srAllScopes := make(map[id.ServiceID]bool)
	for _, sr := range agent.ServiceRequirements {
		if sr.RequireAllScopes {
			srAllScopes[sr.ServiceID] = true
			continue
		}
		ceiling := make(map[string]bool, len(sr.RequiredScopes))
		for _, scope := range sr.RequiredScopes {
			ceiling[scope] = true
		}
		srCeiling[sr.ServiceID] = ceiling
	}

	// FR-013: when the agent declares service_requirements, PS-covered services outside the
	// SR set are ignored. A require_all_scopes service takes the full PS union; other SR
	// services are intersected with their required_scopes ceiling. Agents with no SR use
	// PS-derived scopes directly.
	effectiveScopes := make(map[id.ServiceID][]string, len(perServiceScopes))
	for svcID, scopeSet := range perServiceScopes {
		_, hasCeiling := srCeiling[svcID]
		allScopes := srAllScopes[svcID]
		if hasSR && !hasCeiling && !allScopes {
			continue // service not declared in agent service_requirements
		}
		var scopes []string
		if !hasSR || allScopes {
			for scope := range scopeSet {
				scopes = append(scopes, scope)
			}
		} else {
			ceiling := srCeiling[svcID]
			for scope := range scopeSet {
				if ceiling[scope] {
					scopes = append(scopes, scope)
				}
			}
		}
		sort.Strings(scopes)
		if len(scopes) == 0 && len(scopeSet) > 0 {
			continue // fully capped out — drop (fail-closed for a zero-scope ceiling)
		}
		effectiveScopes[svcID] = scopes
	}

	return effectiveScopes, nil
}
