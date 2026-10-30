package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// AuthZEN Access Evaluation request values this broker accepts.
const (
	AuthZENSubjectTypeJWT          = "urn:ietf:params:oauth:token-type:jwt" // #nosec G101 -- RFC-defined token type URI, not a credential.
	AuthZENActionTokenExchange     = TokenExchangeGrantType
	AuthZENResourceTypeProtected   = "protected_resource"
	MaxDelegationContextAgeSeconds = 60
)

// AccessEvaluationRequest is a decoded AuthZEN Access Evaluation request. The bearer client
// assertion is passed separately and never travels in the body.
type AccessEvaluationRequest struct {
	SubjectType  string
	SubjectID    string
	ActionName   string
	ResourceType string
	ResourceID   string
	// AgentAnchor is the raw context.agent value; nil when absent.
	AgentAnchor *string
}

// DelegationContext is the credential-free projection of an authorized delegation (FR-007,
// FR-008). Optional members are omitted when unset, never null or synthesized.
type DelegationContext struct {
	Subject        DelegationSubject         `json:"subject"`
	Agent          DelegationEntityRef       `json:"agent"`
	Resource       DelegationResource        `json:"resource"`
	Grant          DelegationGrant           `json:"grant"`
	PermissionSets []DelegationPermissionSet `json:"permission_sets"`
}

type DelegationSubject struct {
	ID string `json:"id"`
}

type DelegationEntityRef struct {
	ID          string `json:"id"`
	CanonicalID string `json:"canonical_id,omitempty"`
}

type DelegationResource struct {
	ID          string   `json:"id"`
	CanonicalID string   `json:"canonical_id,omitempty"`
	ResourceURI string   `json:"resource_uri"`
	Scopes      []string `json:"scopes"`
}

type DelegationGrant struct {
	ID         string `json:"id"`
	UpdatedAt  string `json:"updated_at"`
	ValidUntil string `json:"valid_until,omitempty"`
}

type DelegationPermissionSet struct {
	ID          string              `json:"id"`
	CanonicalID string              `json:"canonical_id,omitempty"`
	Services    []DelegationService `json:"services"`
}

type DelegationService struct {
	ID          string   `json:"id"`
	CanonicalID string   `json:"canonical_id,omitempty"`
	Scopes      []string `json:"scopes"`
}

// DecisionKind classifies an evaluation outcome.
type DecisionKind int

const (
	DecisionPermit DecisionKind = iota + 1
	DecisionDeny
	DecisionRequestFailure
)

// DelegationDecision is the outcome of an access evaluation.
//
// A Permit carries Context and MaxAge. A Deny carries an allowlisted grant Detail and the
// consent URL. A RequestFailure carries the HTTP Status. Diagnostic, Service and
// Authorization are the credential-free observation for audit.
type DelegationDecision struct {
	Kind          DecisionKind
	Context       *DelegationContext
	MaxAge        int
	Detail        FailureDetail
	ConsentURL    string
	Status        int
	Diagnostic    Diagnostic
	Service       ServiceRef
	Authorization AuthorizationRef
}

// ServiceCanonicalIDReader batch-loads canonical service IDs.
type ServiceCanonicalIDReader interface {
	CanonicalIDs(ctx context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error)
}

// AccessEvaluationService answers AuthZEN access evaluations with the authoritative
// delegation resolution shared with token exchange. It reads no provider session.
type AccessEvaluationService struct {
	resolver     *DelegationResolver
	canonicalIDs ServiceCanonicalIDReader
	now          func() time.Time
}

// NewAccessEvaluationService creates the evaluation service.
func NewAccessEvaluationService(resolver *DelegationResolver, canonicalIDs ServiceCanonicalIDReader) (*AccessEvaluationService, error) {
	if resolver == nil {
		return nil, fmt.Errorf("resolver cannot be nil")
	}
	if canonicalIDs == nil {
		return nil, fmt.Errorf("canonicalIDs cannot be nil")
	}
	return &AccessEvaluationService{resolver: resolver, canonicalIDs: canonicalIDs, now: time.Now}, nil
}

var consentDenialDetails = map[FailureDetail]bool{
	DetailGrantMissing:                    true,
	DetailGrantExpired:                    true,
	DetailGrantEmpty:                      true,
	DetailGrantPermissionSetUndeclared:    true,
	DetailGrantPermissionSetMissing:       true,
	DetailGrantServiceOmitted:             true,
	DetailGrantServiceDefinitionMissing:   true,
	DetailGrantServiceRequirementExcluded: true,
	DetailGrantScopeIntersectionEmpty:     true,
}

// IsConsentDenialDetail reports whether detail is one of the nine grant-authorization
// details that an evaluation reports as a decision instead of an error.
func IsConsentDenialDetail(detail FailureDetail) bool {
	return consentDenialDetails[detail]
}

// Evaluate validates the request, resolves the delegation and classifies the result.
func (s *AccessEvaluationService) Evaluate(ctx context.Context, req AccessEvaluationRequest, clientAssertion string) DelegationDecision {
	anchor, err := validateEvaluationRequest(req)
	if err != nil {
		return requestFailure(err)
	}
	if ctx.Err() != nil {
		return requestFailure(NewServerErrorWithCause("access evaluation canceled", ctx.Err()).
			WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailCallerCanceled)))
	}
	delegation, err := s.resolver.Resolve(ctx, DelegationRequest{
		SubjectToken:    req.SubjectID,
		ClientAssertion: clientAssertion,
		Resource:        req.ResourceID,
		GrantType:       TokenExchangeGrantType,
		Scope:           "",
		ExpectedAgentID: anchor,
	})
	if err != nil {
		return s.classify(err)
	}

	now := s.now()
	grantRef := authorizationRef(delegation.Agent.ID, delegation.Grant)
	serviceRef := ServiceRef{ID: delegation.Service.ID}
	maxAge := MaxDelegationContextAgeSeconds
	if delegation.Grant.ValidUntil != nil {
		remaining := delegation.Grant.ValidUntil.Sub(now)
		if remaining <= 0 {
			return s.classify(NewAccessDeniedError("user grant has expired").
				WithErrorURI(s.resolver.ConsentURL(delegation.Agent.ID)).
				WithObservation(NewDiagnostic(StageGrantAuthorization, DetailGrantExpired), serviceRef, grantRef))
		}
		maxAge = min(int(remaining/time.Second), MaxDelegationContextAgeSeconds)
	}

	delegationContext, err := s.project(ctx, delegation)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return s.classify(NewServerErrorWithCause("access evaluation canceled", err).
				WithObservation(NewDiagnostic(StageResourceResolution, DetailCallerCanceled), serviceRef, grantRef))
		}
		return s.classify(NewServerErrorWithCause("failed to load canonical service identifiers", err).
			WithObservation(NewDiagnostic(StageResourceResolution, DetailResourceRepositoryUnavailable), serviceRef, grantRef))
	}
	return DelegationDecision{
		Kind:          DecisionPermit,
		Context:       delegationContext,
		MaxAge:        maxAge,
		Diagnostic:    SuccessDiagnostic(ExchangeThirdParty),
		Service:       serviceRef,
		Authorization: grantRef,
	}
}

func validateEvaluationRequest(req AccessEvaluationRequest) (*id.AgentID, error) {
	malformed := func(description string) error {
		return NewInvalidRequestError(description).WithDiagnostic(NewDiagnostic(StageRequestValidation, DetailRequestMalformed))
	}
	if req.SubjectType != AuthZENSubjectTypeJWT {
		return nil, malformed("unsupported subject type")
	}
	if req.SubjectID == "" {
		return nil, malformed("subject id is required")
	}
	if req.ActionName != AuthZENActionTokenExchange {
		return nil, malformed("unsupported action")
	}
	if req.ResourceType != AuthZENResourceTypeProtected {
		return nil, malformed("unsupported resource type")
	}
	if _, err := NewResourceURI(req.ResourceID); err != nil {
		return nil, malformed("resource id must be an absolute URI")
	}
	if req.AgentAnchor == nil {
		return nil, nil
	}
	anchor, err := id.ParseAgentID(*req.AgentAnchor)
	if err != nil {
		return nil, malformed("context.agent must be an agent UUID")
	}
	return &anchor, nil
}

func requestFailure(err error) DelegationDecision {
	var tokenErr *TokenExchangeError
	if !errors.As(err, &tokenErr) {
		tokenErr = NewServerErrorWithCause("access evaluation failed", err).WithDiagnostic(NewDiagnostic(StageExchangeRouting, DetailInternalUnclassified))
	}
	return DelegationDecision{Kind: DecisionRequestFailure, Status: evaluationStatus(tokenErr),
		Diagnostic: tokenErr.Diagnostic(), Service: tokenErr.Service(), Authorization: tokenErr.Authorization()}
}

func (s *AccessEvaluationService) classify(err error) DelegationDecision {
	var tokenErr *TokenExchangeError
	if !errors.As(err, &tokenErr) {
		return requestFailure(err)
	}
	diagnostic := tokenErr.Diagnostic()
	if tokenErr.Code() == "access_denied" && diagnostic.Stage() == StageGrantAuthorization && IsConsentDenialDetail(diagnostic.Detail()) {
		return DelegationDecision{
			Kind:          DecisionDeny,
			Detail:        diagnostic.Detail(),
			ConsentURL:    s.resolver.ConsentURL(tokenErr.Authorization().AgentID),
			Diagnostic:    diagnostic,
			Service:       tokenErr.Service(),
			Authorization: tokenErr.Authorization(),
		}
	}
	return requestFailure(tokenErr)
}

// evaluationStatus maps a non-denial failure to its AuthZEN HTTP status (research R6).
func evaluationStatus(err *TokenExchangeError) int {
	switch err.Code() {
	case "invalid_request", "invalid_grant", "invalid_target":
		return 400
	case "invalid_client":
		return 401
	case "access_denied":
		if err.Diagnostic().Detail() == DetailClientPolicyDenied {
			return 403
		}
	}
	return 500
}

func (s *AccessEvaluationService) project(ctx context.Context, d *Delegation) (*DelegationContext, error) {
	serviceIDs := make([]id.ServiceID, 0)
	seen := make(map[id.ServiceID]bool)
	for _, entry := range d.Grant.GrantedPermissionSets {
		for _, svcID := range entry.IncludedServiceIDs {
			if !seen[svcID] {
				seen[svcID] = true
				serviceIDs = append(serviceIDs, svcID)
			}
		}
	}
	canonicalIDs, err := s.canonicalIDs.CanonicalIDs(ctx, serviceIDs)
	if err != nil {
		return nil, err
	}

	result := &DelegationContext{
		Subject: DelegationSubject{ID: d.Principal},
		Agent:   DelegationEntityRef{ID: d.Agent.ID.String(), CanonicalID: deref(d.Agent.CanonicalID)},
		Resource: DelegationResource{
			ID:          d.Service.ID.String(),
			CanonicalID: deref(d.Service.CanonicalID),
			ResourceURI: d.ResourceURI,
			Scopes:      sortedCopy(d.EffectiveScopes),
		},
		Grant: DelegationGrant{
			ID:        d.Grant.ID.String(),
			UpdatedAt: d.Grant.UpdatedAt.UTC().Format(time.RFC3339),
		},
		PermissionSets: make([]DelegationPermissionSet, 0, len(d.Grant.GrantedPermissionSets)),
	}
	if d.Grant.ValidUntil != nil {
		result.Grant.ValidUntil = d.Grant.ValidUntil.UTC().Format(time.RFC3339)
	}
	for _, entry := range d.Grant.GrantedPermissionSets {
		ps := d.PermissionSets[entry.PermissionSetID]
		projected := DelegationPermissionSet{
			ID:       entry.PermissionSetID.String(),
			Services: make([]DelegationService, 0, len(entry.IncludedServiceIDs)),
		}
		contributions := make(map[id.ServiceID][]string)
		if ps != nil {
			projected.CanonicalID = deref(ps.CanonicalID)
			for _, ss := range ps.ServiceScopes {
				contributions[ss.ServiceID] = append(contributions[ss.ServiceID], ss.Scopes...)
			}
		}
		for _, svcID := range entry.IncludedServiceIDs {
			projected.Services = append(projected.Services, DelegationService{
				ID:          svcID.String(),
				CanonicalID: canonicalIDs[svcID],
				Scopes:      sortedCopy(contributions[svcID]),
			})
		}
		result.PermissionSets = append(result.PermissionSets, projected)
	}
	return result, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sortedCopy(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
