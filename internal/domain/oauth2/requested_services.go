package oauth2

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	descriptionServiceScopeNotPermitted = "requested service scope is not permitted"
	descriptionSessionInvalid           = "authorization session is invalid or expired"
	descriptionUsabilityUnverifiable    = "requested service usability cannot be verified; retry later"
)

// redirectError builds an error decision redirected to the already-validated redirect_uri.
func (s *AuthorizationService) redirectError(ctx context.Context, req *ports.AuthorizationRequest, code, description string) *ports.AuthorizationDecision {
	redirect, err := BuildErrorRedirectURL(req.RedirectURI, req.State, code, description)
	if err != nil && s.logger != nil {
		s.logger.ErrorContext(ctx, "error redirect build failed", "reason", reasonRedirectBuildFailed, "error_kind", errorKindInternal)
	}
	return &ports.AuthorizationDecision{Action: "error", ErrorCode: code, ErrorDesc: description, RedirectURL: redirect}
}

// isBrokerScope reports whether scope carries the configured virtual-scope prefix.
func (s *AuthorizationService) isBrokerScope(scope string) bool {
	return s.requestedServices != nil && servicescope.IsBrokerScope(scope, s.requestedServices.PublicURL())
}

// authorizationContinuation authenticates the protected post-consent continuation. The sealed
// expiry, principal, agent and original request must all match; it never falls back to an
// unbound fresh resolution.
func (s *AuthorizationService) authorizationContinuation(ctx context.Context, req *ports.AuthorizationRequest, principal id.Principal, agent *storage.Agent) (*ports.AuthorizationSession, *ports.AuthorizationDecision) {
	if req.SessionToken == "" {
		return nil, nil
	}
	session, err := s.sessionTokenService.ValidateAuthorizationSessionToken(req.SessionToken, agent.ID, principal)
	if err != nil || session.OriginalURL != withoutSessionToken(req.OriginalURL) {
		return nil, s.redirectError(ctx, req, "access_denied", descriptionSessionInvalid)
	}
	return session, nil
}

// withoutSessionToken removes only the continuation parameter, keeping every other query
// component byte for byte.
func withoutSessionToken(originalURL string) string {
	path, rawQuery, found := strings.Cut(originalURL, "?")
	if !found {
		return originalURL
	}
	var kept []string
	for _, part := range strings.Split(rawQuery, "&") {
		name, _, _ := strings.Cut(part, "=")
		if decoded, err := url.QueryUnescape(name); err == nil && decoded == "session_token" {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return path
	}
	return path + "?" + strings.Join(kept, "&")
}

// resolveRequestedServices resolves the request's virtual scopes. A continuation must resolve
// every virtual scope to its sealed binding.
func (s *AuthorizationService) resolveRequestedServices(ctx context.Context, req *ports.AuthorizationRequest, agent *storage.Agent, continuation *ports.AuthorizationSession) (*servicescope.RequestedServiceSet, *ports.AuthorizationDecision) {
	if s.requestedServices == nil {
		return &servicescope.RequestedServiceSet{}, nil
	}
	var bindings map[string]id.ServiceID
	if continuation != nil {
		bindings = map[string]id.ServiceID{}
		for scope, serviceID := range continuation.ServiceScopeBindings {
			bindings[scope] = serviceID
		}
	}
	requested, err := s.requestedServices.Resolve(ctx, req.Scope, agent, bindings)
	if err != nil {
		if servicescope.IsPermissionFailure(err) {
			if s.logger != nil {
				s.logger.WarnContext(ctx, "requested service scope rejected", "event", "oauth2.requested_service.scope_rejected",
					"agent_id", agent.ID, "reason", scopeRejectionReason(err), "error_kind", errorKindDefinitive)
			}
			return nil, s.redirectError(ctx, req, "invalid_scope", descriptionServiceScopeNotPermitted)
		}
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "requested service resolution failed", "event", "oauth2.requested_service.resolution_failed",
				"agent_id", agent.ID, "reason", reasonDependencyFailed, "error_kind", errorKindTransient)
		}
		return nil, s.redirectError(ctx, req, "server_error", "server error")
	}
	return requested, nil
}

// checkRequestedServices treats every requested service as required for this request only.
// It returns nil when all are satisfied.
func (s *AuthorizationService) checkRequestedServices(ctx context.Context, req *ports.AuthorizationRequest, principal id.Principal, agent *storage.Agent, grant *storage.UserGrant, requested *servicescope.RequestedServiceSet, consent func() *ports.AuthorizationDecision) *ports.AuthorizationDecision {
	if requested.Empty() {
		return nil
	}
	outcomes, err := s.serviceEvaluator.Evaluate(ctx, principal, agent, grant.GrantedPermissionSets, requested.ServiceIDs())
	if err != nil {
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "requested service evaluation failed", "event", "oauth2.requested_service.evaluation_failed",
				"agent_id", agent.ID, "principal", principal, "reason", reasonDependencyFailed, "error_kind", errorKindTransient)
		}
		return s.redirectError(ctx, req, "server_error", "server error")
	}
	if servicescope.AllSatisfied(outcomes) {
		return nil
	}
	if servicescope.AnyTransient(outcomes) {
		return s.redirectError(ctx, req, "temporarily_unavailable", descriptionUsabilityUnverifiable)
	}
	if s.logger != nil {
		for _, outcome := range outcomes {
			if !outcome.Satisfied {
				s.logger.InfoContext(ctx, "incremental authorization required", "event", "oauth2.authorization.incremental_required",
					"agent_id", agent.ID, "principal", principal, "service_id", outcome.ServiceID,
					"reason", unsatisfiedReason(outcome))
			}
		}
	}
	return consent()
}

type serviceLogReason string
type serviceLogErrorKind string

const (
	reasonMalformedScope          serviceLogReason    = "malformed_scope"
	reasonUnknownService          serviceLogReason    = "unknown_service"
	reasonServiceNotEligible      serviceLogReason    = "service_not_eligible"
	reasonBindingMismatch         serviceLogReason    = "binding_mismatch"
	reasonDependencyFailed        serviceLogReason    = "dependency_failed"
	reasonRedirectBuildFailed     serviceLogReason    = "redirect_build_failed"
	reasonNotSelected             serviceLogReason    = "not_selected"
	reasonRequiredScopesUncovered serviceLogReason    = "required_scopes_uncovered"
	errorKindDefinitive           serviceLogErrorKind = "definitive"
	errorKindTransient            serviceLogErrorKind = "transient"
	errorKindInternal             serviceLogErrorKind = "internal"
)

func scopeRejectionReason(err error) serviceLogReason {
	switch {
	case errors.Is(err, servicescope.ErrMalformedScope):
		return reasonMalformedScope
	case errors.Is(err, servicescope.ErrUnknownService):
		return reasonUnknownService
	case errors.Is(err, servicescope.ErrServiceNotEligible):
		return reasonServiceNotEligible
	default:
		return reasonBindingMismatch
	}
}

func unsatisfiedReason(outcome servicescope.Outcome) serviceLogReason {
	switch {
	case !outcome.Selected:
		return reasonNotSelected
	case !outcome.RequiredScopesCovered:
		return reasonRequiredScopesUncovered
	default:
		return serviceLogReason(outcome.Usability)
	}
}
