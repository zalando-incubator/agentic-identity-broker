package oauth2

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var errProxyClientMissing = errors.New("agent has no upstream client_id configured")

const tokenProxySuccessStatus = 200

type TokenOutcomeService struct {
	proxy     ports.OAuth2TokenProxy
	verifier  ports.MultiAgentVerifier
	publicURL string
	logger    *slog.Logger
	agents    tokenScopeAgentLookup
	resolver  *servicescope.Resolver
}

type tokenScopeAgentLookup interface {
	Get(context.Context, id.AgentID) (*storage.Agent, error)
}

func NewTokenOutcomeService(proxy ports.OAuth2TokenProxy, verifier ports.MultiAgentVerifier, logger *slog.Logger) *TokenOutcomeService {
	return &TokenOutcomeService{proxy: proxy, verifier: verifier, logger: logger}
}

func (s *TokenOutcomeService) Proxy(ctx context.Context, form url.Values, contentType string, resolution *ports.TokenGrantResolution) (_ ports.TokenProxyResponse, failure ports.TokenProxyFailure, err error) {
	defer func() {
		switch failure {
		case ports.TokenProxyScopeNotPermitted:
			s.logger.WarnContext(ctx, "requested service scope rejected", "event", "oauth2.requested_service.scope_rejected",
				"agent_id", resolution.AgentID, "reason", scopeRejectionReason(err), "error_kind", errorKindDefinitive)
		case ports.TokenProxyScopeLookupFailed:
			s.logger.ErrorContext(ctx, "requested service resolution failed", "event", "oauth2.requested_service.resolution_failed",
				"agent_id", resolution.AgentID, "reason", reasonDependencyFailed, "error_kind", errorKindTransient)
		}
	}()

	if resolution.ClientID == nil {
		return nil, ports.TokenProxyClientMissing, errProxyClientMissing
	}
	hasBrokerScope := false
	if s.publicURL != "" && form.Get("grant_type") == "refresh_token" {
		for _, token := range strings.Fields(form.Get("scope")) {
			_, kind := servicescope.Parse(token, s.publicURL)
			if kind == servicescope.KindMalformed {
				return nil, ports.TokenProxyScopeNotPermitted, servicescope.ErrMalformedScope
			}
			if kind == servicescope.KindVirtual {
				hasBrokerScope = true
			}
		}
	}
	if hasBrokerScope {
		if s.agents == nil || s.resolver == nil {
			return nil, ports.TokenProxyScopeLookupFailed, errors.New("service scope validation unavailable")
		}
		agent, err := s.agents.Get(ctx, resolution.AgentID)
		if err != nil || agent == nil {
			return nil, ports.TokenProxyScopeLookupFailed, errors.New("service scope agent lookup failed")
		}
		if _, err := s.resolver.Resolve(ctx, form.Get("scope"), agent, nil); err != nil {
			if servicescope.IsPermissionFailure(err) {
				return nil, ports.TokenProxyScopeNotPermitted, err
			}
			return nil, ports.TokenProxyScopeLookupFailed, errors.New("service scope lookup failed")
		}
	}
	form.Set("client_id", resolution.ClientID.String())
	if s.publicURL != "" && form.Has("scope") {
		if scope := servicescope.StripVirtual(form.Get("scope"), s.publicURL); scope != "" {
			form.Set("scope", scope)
		} else {
			form.Del("scope")
		}
	}
	return s.proxy.Exchange(ctx, form.Encode(), contentType)
}

func (s *TokenOutcomeService) CompleteProxy(ctx context.Context, response ports.TokenProxyResponse, agentID id.AgentID) (ports.TokenProxyCompletion, ports.TokenProxyFailure, error) {
	if s.verifier == nil || response.ResponseStatus() != tokenProxySuccessStatus {
		return ports.TokenProxyCompletion{}, ports.TokenProxyNoFailure, nil
	}

	body, err := response.ReadBody()
	if err != nil {
		return ports.TokenProxyCompletion{}, ports.TokenProxyResponseReadFailed, err
	}
	if err := s.verifier.VerifyAgentIDClaim(ctx, body, agentID); err != nil {
		return ports.TokenProxyCompletion{}, ports.TokenProxyVerificationFailed, err
	}
	return ports.TokenProxyCompletion{Body: body, Verified: true}, ports.TokenProxyNoFailure, nil
}

func (s *TokenOutcomeService) WithServiceScopeValidation(agents tokenScopeAgentLookup, resolver *servicescope.Resolver) *TokenOutcomeService {
	s.agents = agents
	s.resolver = resolver
	return s
}

// WithServiceScopeStripping removes broker Virtual Service Scopes from forwarded token
// requests. Proxy refresh never checks or attests broker service usability (DEC-002).
func (s *TokenOutcomeService) WithServiceScopeStripping(publicURL string) *TokenOutcomeService {
	s.publicURL = publicURL
	return s
}
