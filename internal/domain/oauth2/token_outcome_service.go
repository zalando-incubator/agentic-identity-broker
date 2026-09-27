package oauth2

import (
	"context"
	"errors"
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var errProxyClientMissing = errors.New("agent has no upstream client_id configured")

const tokenProxySuccessStatus = 200

type TokenOutcomeService struct {
	proxy    ports.OAuth2TokenProxy
	verifier ports.MultiAgentVerifier
}

func NewTokenOutcomeService(proxy ports.OAuth2TokenProxy, verifier ports.MultiAgentVerifier) *TokenOutcomeService {
	return &TokenOutcomeService{proxy: proxy, verifier: verifier}
}

func (s *TokenOutcomeService) Proxy(ctx context.Context, form url.Values, contentType string, resolution *ports.TokenGrantResolution) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	if resolution.ClientID == nil {
		return nil, ports.TokenProxyClientMissing, errProxyClientMissing
	}
	form.Set("client_id", resolution.ClientID.String())
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
