package oauth2server

import (
	"context"
	"maps"
	"net/http"
	"slices"

	"github.com/ory/fosite"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const claimServiceScopeBindings = "service_scope_bindings"

const descriptionServiceScopeNotPermitted = "requested service scope is not permitted"

// WithServiceScopes enables Virtual Service Scopes (ADR 049): they bypass the per-client
// allow-list, must carry a Service-Scope Binding and are revalidated on code exchange.
func (p *Provider) WithServiceScopes(resolver *servicescope.Resolver, evaluator *servicescope.Evaluator) *Provider {
	p.serviceScopes = resolver
	p.serviceEvaluator = evaluator
	publicURL := resolver.PublicURL()
	p.config.ScopeStrategy = func(allowedScopes []string, scope string) bool {
		return servicescope.IsBrokerScope(scope, publicURL) || oauth2.IsScopeAllowed(allowedScopes, scope)
	}
	return p
}

// isBrokerScope reports whether the configured public URL marks scope as a broker scope.
func (p *Provider) isBrokerScope(scope string) bool {
	return p.serviceScopes != nil && servicescope.IsBrokerScope(scope, p.serviceScopes.PublicURL())
}

// grantedBindings returns the bindings of the virtual scopes in scopes, failing closed when a
// broker scope is malformed or unbound.
func (p *Provider) grantedBindings(scopes fosite.Arguments, bindings map[string]id.ServiceID) (storage.ServiceScopeBindings, error) {
	var granted storage.ServiceScopeBindings
	for _, scope := range scopes {
		if !p.isBrokerScope(scope) {
			continue
		}
		serviceID, ok := bindings[scope]
		if _, kind := servicescope.Parse(scope, p.serviceScopes.PublicURL()); kind != servicescope.KindVirtual || !ok || serviceID.IsZero() {
			return nil, fosite.ErrInvalidScope.WithHint(descriptionServiceScopeNotPermitted)
		}
		if granted == nil {
			granted = storage.ServiceScopeBindings{}
		}
		granted[scope] = serviceID
	}
	return granted, nil
}

// HandleBoundAuthorize issues an authorization code whose virtual scopes are pinned to the
// service UUIDs the authorization service resolved.
func (p *Provider) HandleBoundAuthorize(
	ctx context.Context,
	clientID, redirectURI, responseType, scope, state, codeChallenge, codeChallengeMethod string,
	principal id.Principal,
	bindings map[string]id.ServiceID,
) (string, error) {
	return p.handleAuthorize(ctx, clientID, redirectURI, responseType, scope, state, codeChallenge, codeChallengeMethod, principal, bindings)
}

// verifyBoundServices re-resolves every binding against current service identity and agent
// eligibility. A rename, reuse or eligibility change fails closed with invalid_scope.
func (p *Provider) verifyBoundServices(ctx context.Context, bindings storage.ServiceScopeBindings, agent *storage.Agent) error {
	if len(bindings) == 0 {
		return nil
	}
	if p.serviceScopes == nil {
		return fosite.ErrInvalidScope.WithHint(descriptionServiceScopeNotPermitted)
	}
	if _, err := p.serviceScopes.ResolveBindings(ctx, bindings, agent); err != nil {
		if servicescope.IsPermissionFailure(err) {
			return fosite.ErrInvalidScope.WithHint(descriptionServiceScopeNotPermitted)
		}
		return fosite.ErrServerError.WithWrap(err)
	}
	return nil
}

func setSessionBindings(session fosite.Session, bindings storage.ServiceScopeBindings) {
	extra, ok := session.(fosite.ExtraClaimsSession)
	if !ok || len(bindings) == 0 {
		return
	}
	extra.GetExtraClaims()[claimServiceScopeBindings] = maps.Clone(bindings)
}

func sessionBindings(session fosite.Session) storage.ServiceScopeBindings {
	extra, ok := session.(fosite.ExtraClaimsSession)
	if !ok {
		return nil
	}
	bindings, _ := extra.GetExtraClaims()[claimServiceScopeBindings].(storage.ServiceScopeBindings)
	return bindings.Clone()
}

// narrowRefreshScope grants the requested subset of the predecessor's current scope; fosite
// itself always re-grants the full original scope. Any requested scope outside the current
// scope, including one removed by an earlier narrowing, is widening and is refused before the
// refresh token is rotated (FR-012).
func narrowRefreshScope(req *fosite.AccessRequest, scope string) error {
	requested := fosite.Arguments(oauth2.SplitScope(scope))
	if len(requested) == 0 {
		return nil
	}
	current := req.GetGrantedScopes()
	var narrowed fosite.Arguments
	for _, value := range requested {
		if !slices.Contains(current, value) {
			return NewRFC6749Error("invalid_scope", "scope "+value+" requires interactive authorization", http.StatusBadRequest, ErrInvalidScope)
		}
		if !slices.Contains(narrowed, value) {
			narrowed = append(narrowed, value)
		}
	}
	req.GrantedScope = narrowed
	return nil
}

// recheckBoundServices revalidates the complete immutable assurance binding set, including
// bindings whose scope was narrowed away, against current identity, eligibility, grant
// coverage and verified usability. It runs after fosite validation and before rotation, so
// every refusal leaves the refresh token unconsumed.
func (p *Provider) recheckBoundServices(ctx context.Context, req *fosite.AccessRequest) error {
	bindings := sessionBindings(req.GetSession())
	if len(bindings) == 0 {
		return nil
	}
	holder, ok := req.GetClient().(agentHolder)
	if !ok || p.serviceScopes == nil || p.serviceEvaluator == nil {
		return fosite.ErrServerError.WithDebug("service scope recheck unavailable")
	}
	agent := holder.getAgent()
	principal := id.Principal(req.GetSession().GetSubject())
	scopesByService := map[id.ServiceID]string{}
	for _, scope := range slices.Sorted(maps.Keys(bindings)) {
		if _, seen := scopesByService[bindings[scope]]; !seen {
			scopesByService[bindings[scope]] = scope
		}
	}
	firstScope := slices.Sorted(maps.Keys(bindings))[0]

	refuse := func(scope string, reason refreshScopeReason) error {
		p.logger.WarnContext(ctx, "refresh service recheck failed", "event", "oauth2_server.service.recheck_failed",
			"agent_id", agent.ID, "principal", principal,
			"service_id", bindings[scope], "reason", reason, "error_kind", refreshScopeDefinitive)
		return NewRFC6749Error("invalid_scope", "scope "+scope+" is no longer satisfied; interactive authorization required", http.StatusBadRequest, ErrInvalidScope)
	}
	transient := func(reason refreshScopeReason) error {
		p.logger.ErrorContext(ctx, "refresh service recheck failed", "event", "oauth2_server.service.recheck_failed",
			"agent_id", agent.ID, "principal", principal, "reason", reason, "error_kind", refreshScopeTransient)
		return NewRFC6749Error("server_error", "requested service usability cannot be verified", http.StatusInternalServerError, ErrServerError)
	}

	requested, err := p.serviceScopes.ResolveBindings(ctx, bindings, agent)
	if err != nil {
		if servicescope.IsPermissionFailure(err) {
			return refuse(firstScope, refreshScopeBindingUnresolved)
		}
		return transient(refreshScopeServiceLookupFailed)
	}
	grant, err := p.fositeStorage.verifier.VerifyAgentAccess(ctx, principal, agent.ID)
	if err != nil {
		return transient(refreshScopeGrantLookupFailed)
	}
	outcomes, err := p.serviceEvaluator.Evaluate(ctx, principal, agent, grant.GrantedPermissionSets, requested.ServiceIDs())
	if err != nil {
		return transient(refreshScopePermissionSetLookupFailed)
	}
	if servicescope.AnyTransient(outcomes) {
		return transient(refreshScopeUsabilityUnverifiable)
	}
	for _, outcome := range outcomes {
		if !outcome.Satisfied {
			return refuse(scopesByService[outcome.ServiceID], unsatisfiedRefreshReason(outcome))
		}
	}
	return nil
}

type refreshScopeReason string
type refreshScopeErrorKind string

const (
	refreshScopeBindingUnresolved         refreshScopeReason    = "binding_unresolved"
	refreshScopeServiceLookupFailed       refreshScopeReason    = "service_lookup_failed"
	refreshScopeGrantLookupFailed         refreshScopeReason    = "grant_lookup_failed"
	refreshScopePermissionSetLookupFailed refreshScopeReason    = "permission_set_lookup_failed"
	refreshScopeUsabilityUnverifiable     refreshScopeReason    = "usability_unverifiable"
	refreshScopeNotSelected               refreshScopeReason    = "not_selected"
	refreshScopeRequiredScopesUncovered   refreshScopeReason    = "required_scopes_uncovered"
	refreshScopeDefinitive                refreshScopeErrorKind = "definitive"
	refreshScopeTransient                 refreshScopeErrorKind = "transient"
)

func unsatisfiedRefreshReason(outcome servicescope.Outcome) refreshScopeReason {
	switch {
	case !outcome.Selected:
		return refreshScopeNotSelected
	case !outcome.RequiredScopesCovered:
		return refreshScopeRequiredScopesUncovered
	default:
		return refreshScopeReason(outcome.Usability)
	}
}

// rejectServiceScopesWithoutUser refuses virtual scopes on grants without a user.
func (p *Provider) rejectServiceScopesWithoutUser(scopes fosite.Arguments) error {
	for _, scope := range scopes {
		if p.isBrokerScope(scope) {
			return NewRFC6749Error("invalid_scope", "service scopes require a user authorization", http.StatusBadRequest, ErrInvalidScope)
		}
	}
	return nil
}
