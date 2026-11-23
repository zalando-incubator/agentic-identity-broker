package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtclaims"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// GrantResolutionRequest is the input to the authoritative grant resolution shared by
// token exchange and AuthZEN access evaluation.
type GrantResolutionRequest struct {
	SubjectToken    string
	ClientAssertion string
	Resource        string
	GrantType       string
	Scope           string
	// ExpectedAgentID is an optional consistency anchor, compared after the agent is
	// loaded and before any grant is read. Exchange never sets it.
	ExpectedAgentID *id.AgentID
	// RequireCallingPeer rejects a client assertion without a non-blank sub, so every
	// finalized security context names the calling peer. Exchange never sets it.
	RequireCallingPeer bool
}

// ResolvedGrant is the outcome of a successful resolution: every invariant of exchange
// steps 2–9a holds. Resolution decrypts no credential, but Service is the full provider
// entity and still carries its encrypted client secret; only the GrantContext projection
// is credential-free.
type ResolvedGrant struct {
	Principal       id.Principal
	Agent           *storage.Agent
	Service         *model.ThirdpartyOAuth2ProviderEntity
	ResourceURI     string
	Grant           *storage.UserGrant
	PermissionSets  map[id.PermissionSetID]*storage.PermissionSet
	EffectiveScopes []string
}

// GrantResolver verifies the subject and caller, authorizes the caller, resolves the
// resource and agent, and authorizes the requested service against the active grant.
// It has no provider-session dependency, so resolution never reads provider credentials.
type GrantResolver struct {
	jwtValidator         *JWTValidator
	celEvaluator         *CELEvaluator
	providerService      *thirdparty.ThirdpartyOAuth2ProviderService
	consentService       *consent.Service
	permissionSetService *permissionset.Service
	agentRepository      ports.AgentRepository
	consentBaseURL       string
}

// NewGrantResolver creates a resolver. consentBaseURL is the end-user public base used
// for consent recovery URLs.
func NewGrantResolver(
	jwtValidator *JWTValidator,
	celEvaluator *CELEvaluator,
	providerService *thirdparty.ThirdpartyOAuth2ProviderService,
	consentService *consent.Service,
	permissionSetService *permissionset.Service,
	agentRepository ports.AgentRepository,
	consentBaseURL string,
) (*GrantResolver, error) {
	if jwtValidator == nil {
		return nil, fmt.Errorf("jwtValidator cannot be nil")
	}
	if celEvaluator == nil {
		return nil, fmt.Errorf("celEvaluator cannot be nil")
	}
	if providerService == nil {
		return nil, fmt.Errorf("providerService cannot be nil")
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
	return &GrantResolver{
		jwtValidator:         jwtValidator,
		celEvaluator:         celEvaluator,
		providerService:      providerService,
		consentService:       consentService,
		permissionSetService: permissionSetService,
		agentRepository:      agentRepository,
		consentBaseURL:       consentBaseURL,
	}, nil
}

// Resolve runs exchange steps 2–9a. Every error is a *TokenExchangeError that carries the
// failing stage's diagnostic and the service and authorization references observed so far.
func (r *GrantResolver) Resolve(ctx context.Context, req GrantResolutionRequest) (_ *ResolvedGrant, err error) {
	var serviceRef ServiceRef
	var resolvedPrincipal id.Principal
	var registeredAgentID id.AgentID
	var grant *storage.UserGrant
	stage := StageSubjectValidation
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
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			diagnostic = NewDiagnostic(diagnostic.Stage(), DetailCallerCanceled)
		}
		err = tokenErr.WithObservation(diagnostic, serviceRef, authorizationRef(resolvedPrincipal, registeredAgentID, grant))
	}()

	// Step 2: Validate subject_token JWT
	subjectTokenJWT, err := r.jwtValidator.ValidateSubjectToken(ctx, req.SubjectToken)
	if err != nil {
		return nil, err
	}

	// Step 3: Validate client_assertion JWT
	stage = StageClientValidation
	clientAssertionJWT, err := r.jwtValidator.ValidateClientAssertion(ctx, req.ClientAssertion)
	if err != nil {
		return nil, err
	}
	if req.RequireCallingPeer {
		if callingPeer, ok := clientAssertionJWT.Subject(); !ok || strings.TrimSpace(callingPeer) == "" {
			return nil, NewInvalidClientError("client_assertion subject is required").
				WithDiagnostic(NewDiagnostic(stage, DetailClientInvalid))
		}
	}

	// Steps 4–5: resolve the human principal and agent_id; agent-subject tokens are denied
	// before agent extraction and any grant or session lookup (ADR 045 decision 6).
	stage = StageIdentityResolution
	subjectTokenClaims := jwtclaims.FromToken(subjectTokenJWT)
	principal, agentID, err := r.celEvaluator.ResolveHumanSubject(subjectTokenClaims)

	// ADR 033 §3: finalize the request security context at the post-validation
	// token-exchange seam. The subject-token principal (Actor) and the validated
	// client_assertion subject (CallingPeer) are both known here. Finalizing before
	// the authorization decision guarantees a denied exchange still carries
	// actor/calling_peer on the security-sensitive failure audit path.
	if principal != "" {
		resolvedPrincipal = principal
		callingPeer, _ := clientAssertionJWT.Subject()
		_, _ = security.FinalizeCaptureHolder(ctx, principal, callingPeer)
	}
	if err != nil {
		return nil, err
	}

	// Step 6: Authorize privileged client via CEL expression evaluation
	stage = StageClientAuthorization
	clientAssertionClaims := jwtclaims.FromToken(clientAssertionJWT)
	requestContext := &CELRequestContext{
		Resource:  req.Resource,
		GrantType: req.GrantType,
		Scope:     req.Scope,
		Principal: principal,
		AgentID:   agentID,
	}
	authorized, err := r.celEvaluator.AuthorizePrivilegedClient(clientAssertionClaims, subjectTokenClaims, *requestContext)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, NewAccessDeniedError("privileged client authorization failed").WithDiagnostic(NewDiagnostic(stage, DetailClientPolicyDenied))
	}

	// Step 7: Normalize resource URI
	normalizedResource := Normalize(req.Resource)

	// Step 8: Lookup service by resource URI
	stage = StageResourceResolution
	service, err := r.providerService.FindByProtectedResource(ctx, normalizedResource)
	if err != nil {
		if IsTokenExchangeError(err) {
			// InvalidTarget error from repository
			return nil, err
		}
		return nil, NewServerErrorWithCause("failed to lookup service by resource URI", err).WithDiagnostic(NewDiagnostic(stage, DetailResourceRepositoryUnavailable))
	}
	serviceRef = ServiceRef{ID: service.ID}

	// Step 9: Verify user has granted agent access to service (T059-T065)
	// CRITICAL (T078): Grant verification MUST occur BEFORE session check
	// This prevents information leakage: if grant is missing, return access_denied (no permission).
	// Only if grant exists but session is missing do we return invalid_grant (no session).
	//
	// T060 (Feature 021): agentID is the broker-internal agent UUID (resolved by CEL).
	// Parse it as UUID and look up by primary key — no GetByClientID needed.
	stage = StageIdentityResolution
	parsedAgentID, parseErr := id.ParseAgentID(agentID)
	if parseErr != nil {
		return nil, NewServerErrorWithCause("agent identity configuration is invalid", parseErr).WithDiagnostic(NewDiagnostic(stage, DetailAgentInvalid))
	}
	agent, err := r.agentRepository.Get(ctx, parsedAgentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, NewServerErrorWithCause("agent is not configured", err).WithDiagnostic(NewDiagnostic(stage, DetailAgentMissing))
		}
		return nil, NewServerErrorWithCause("failed to lookup agent", err).WithDiagnostic(NewDiagnostic(stage, DetailAgentRepositoryUnavailable))
	}
	registeredAgentID = agent.ID
	if req.ExpectedAgentID != nil && *req.ExpectedAgentID != agent.ID {
		return nil, NewInvalidRequestError("context.agent does not match the agent resolved from the subject token").
			WithCause(ErrAgentAnchorMismatch).WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}
	// T061-T065: Delegate grant verification to ConsentService using internal agent UUID
	stage = StageGrantAuthorization
	grantCtx, grantSpan := otel.Tracer("tokenexchange").Start(ctx, "tokenexchange.verify_agent_access",
		trace.WithSpanKind(trace.SpanKindInternal))
	grant, err = r.consentService.VerifyAgentAccess(grantCtx, principal, agent.ID)
	if err != nil && !errors.Is(err, consent.ErrAgentAccessDenied) && !errors.Is(err, consent.ErrGrantExpired) &&
		(ctx.Err() == nil || !errors.Is(err, ctx.Err())) {
		grantSpan.SetAttributes(attribute.String("error.type", string(DetailGrantRepositoryUnavailable)))
		grantSpan.SetStatus(codes.Error, "grant verification failed")
	}
	grantSpan.End()
	if err != nil {
		if errors.Is(err, consent.ErrAgentAccessDenied) {
			return nil, NewAccessDeniedError("user has not granted permission for this agent to access the requested service").
				WithCause(err).WithErrorURI(r.agentConsentURL(agent.ID)).WithDiagnostic(NewDiagnostic(stage, DetailGrantMissing))
		}
		if errors.Is(err, consent.ErrGrantExpired) {
			return nil, NewAccessDeniedError("user grant has expired").WithCause(err).
				WithErrorURI(r.agentConsentURL(agent.ID)).WithDiagnostic(NewDiagnostic(stage, DetailGrantExpired))
		}
		return nil, NewServerErrorWithCause("failed to verify user grant", err).WithDiagnostic(NewDiagnostic(stage, DetailGrantRepositoryUnavailable))
	}

	// Step 9a: Authorize the requested service against the grant (FR-010, SR-007).
	// Fail closed for every agent: the grant must cover the requested service before any
	// session lookup or credential decryption. Agents that declare neither PermissionSets
	// nor ServiceRequirements are not exempt.
	if len(grant.GrantedPermissionSets) == 0 {
		return nil, NewAccessDeniedError("grant has no permission set entries; re-consent required").
			WithErrorURI(r.agentConsentURL(agent.ID)).WithDiagnostic(NewDiagnostic(stage, DetailGrantEmpty))
	}
	effectiveScopes, psIndex, coverageDetail, err := r.resolveGrantCoverage(ctx, grant, agent, service.ID)
	if err != nil {
		return nil, err
	}
	if coverageDetail != DetailNone {
		return nil, NewAccessDeniedError("service is not authorized by the grant; re-consent required").
			WithErrorURI(r.agentConsentURL(agent.ID)).WithDiagnostic(NewDiagnostic(stage, coverageDetail))
	}

	return &ResolvedGrant{
		Principal:       principal,
		Agent:           agent,
		Service:         service,
		ResourceURI:     normalizedResource,
		Grant:           grant,
		PermissionSets:  psIndex,
		EffectiveScopes: effectiveScopes[service.ID],
	}, nil
}

// ConsentURL returns the end-user consent page for the agent.
func (r *GrantResolver) ConsentURL(agentID id.AgentID) string {
	return r.agentConsentURL(agentID)
}

func (r *GrantResolver) agentConsentURL(agentID id.AgentID) string {
	return strings.TrimRight(r.consentBaseURL, "/") + "/agents/" + agentID.String()
}

func authorizationRef(principal id.Principal, agentID id.AgentID, grant *storage.UserGrant) AuthorizationRef {
	ref := AuthorizationRef{Principal: principal, AgentID: agentID}
	if grant == nil {
		return ref
	}
	ref.GrantID = grant.ID
	ref.GrantUpdatedAt = grant.UpdatedAt
	if grant.ValidUntil != nil {
		ref.GrantValidUntil = *grant.ValidUntil
		ref.GrantHasValidUntil = true
	}
	return ref
}

// resolveGrantCoverage resolves permission sets from the grant and computes
// per-service effective scopes by taking the union across all permission sets
// and intersecting with the agent's service requirement scope ceiling (FR-013).
//
// Coverage details describe the requested service; structural reference and repository
// failures retain their diagnostic on the error and return DetailNone separately.
func (r *GrantResolver) resolveGrantCoverage(
	ctx context.Context,
	grant *storage.UserGrant,
	agent *storage.Agent,
	requestedServiceID id.ServiceID,
) (map[id.ServiceID][]string, map[id.PermissionSetID]*storage.PermissionSet, FailureDetail, error) {
	declaredPS := make(map[id.PermissionSetID]bool, len(agent.PermissionSets))
	for _, aps := range agent.PermissionSets {
		declaredPS[aps.PermissionSetID] = true
	}

	// Collect PS IDs from grant
	psIDs := make([]id.PermissionSetID, len(grant.GrantedPermissionSets))
	for i, entry := range grant.GrantedPermissionSets {
		if !declaredPS[entry.PermissionSetID] {
			return nil, nil, DetailNone, NewAccessDeniedError("grant references a permission set not declared by the agent; re-consent required").
				WithErrorURI(r.agentConsentURL(agent.ID)).WithDiagnostic(NewDiagnostic(StageGrantAuthorization, DetailGrantPermissionSetUndeclared))
		}
		psIDs[i] = entry.PermissionSetID
	}

	// Resolve permission sets (TTL cache transparent)
	resolvedSets, err := r.permissionSetService.GetByIDs(ctx, psIDs)
	if err != nil {
		return nil, nil, DetailNone, NewServerErrorWithCause("failed to resolve permission sets", err).WithDiagnostic(NewDiagnostic(StageGrantAuthorization, DetailGrantRepositoryUnavailable))
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
	requestedServiceIncluded, requestedServiceDefined := false, false
	for _, entry := range grant.GrantedPermissionSets {
		ps, ok := psIndex[entry.PermissionSetID]
		if !ok {
			// Fail closed: any PS referenced by a stored grant must still exist.
			// Whether mandatory or optional, a missing PS means the grant is stale.
			return nil, nil, DetailNone, NewAccessDeniedError("grant references a missing permission set; re-consent required").
				WithErrorURI(r.agentConsentURL(agent.ID)).WithDiagnostic(NewDiagnostic(StageGrantAuthorization, DetailGrantPermissionSetMissing))
		}
		included := grantIndex[entry.PermissionSetID]
		if included[requestedServiceID] {
			requestedServiceIncluded = true
		}
		for _, ss := range ps.ServiceScopes {
			if !included[ss.ServiceID] {
				continue // Service not included in this grant entry
			}
			if ss.ServiceID == requestedServiceID {
				requestedServiceDefined = true
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
	coverageDetail := DetailNone
	for svcID, scopeSet := range perServiceScopes {
		_, hasCeiling := srCeiling[svcID]
		allScopes := srAllScopes[svcID]
		if hasSR && !hasCeiling && !allScopes {
			if svcID == requestedServiceID {
				coverageDetail = DetailGrantServiceRequirementExcluded
			}
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
			if svcID == requestedServiceID {
				coverageDetail = DetailGrantScopeIntersectionEmpty
			}
			continue // fully capped out — drop (fail-closed for a zero-scope ceiling)
		}
		effectiveScopes[svcID] = scopes
	}

	if !requestedServiceIncluded {
		return effectiveScopes, psIndex, DetailGrantServiceOmitted, nil
	}
	if !requestedServiceDefined {
		return effectiveScopes, psIndex, DetailGrantServiceDefinitionMissing, nil
	}
	return effectiveScopes, psIndex, coverageDetail, nil
}
