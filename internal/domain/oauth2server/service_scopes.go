package oauth2server

import (
	"context"
	"maps"

	"github.com/ory/fosite"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const claimServiceScopeBindings = "service_scope_bindings"

const descriptionServiceScopeNotPermitted = "requested service scope is not permitted"

// WithServiceScopes enables Virtual Service Scopes (ADR 046): they bypass the per-client
// allow-list, must carry a Service-Scope Binding and are revalidated on code exchange.
func (p *Provider) WithServiceScopes(resolver *servicescope.Resolver) *Provider {
	p.serviceScopes = resolver
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
