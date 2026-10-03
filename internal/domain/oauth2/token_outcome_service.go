package oauth2

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var errProxyClientMissing = errors.New("agent has no upstream client_id configured")

const tokenProxySuccessStatus = 200

type TokenOutcomeService struct {
	proxy    ports.OAuth2TokenProxy
	verifier ports.MultiAgentVerifier
	ledger   *ledger.Service
}

func NewTokenOutcomeService(proxy ports.OAuth2TokenProxy, verifier ports.MultiAgentVerifier, recorder *ledger.Service) *TokenOutcomeService {
	if recorder == nil {
		panic("token recorder is required")
	}
	return &TokenOutcomeService{proxy: proxy, verifier: verifier, ledger: recorder}
}

func (s *TokenOutcomeService) Proxy(ctx context.Context, form url.Values, contentType string, resolution *ports.TokenGrantResolution) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	if resolution.ClientID == nil {
		if err := s.RecordFailure(ctx, ports.TokenRequestInternalFailure, resolution.AgentID); err != nil {
			return nil, ports.TokenProxyRecordingFailed, err
		}
		return nil, ports.TokenProxyClientMissing, errProxyClientMissing
	}
	form.Set("client_id", resolution.ClientID.String())
	response, failure, err := s.proxy.Exchange(ctx, form.Encode(), contentType)
	if err != nil {
		if recordErr := s.RecordFailure(ctx, ports.TokenRequestUpstreamFailure, resolution.AgentID); recordErr != nil {
			return nil, ports.TokenProxyRecordingFailed, recordErr
		}
	}
	return response, failure, err
}

func (s *TokenOutcomeService) CompleteProxy(ctx context.Context, response ports.TokenProxyResponse, agentID id.AgentID) (ports.TokenProxyCompletion, ports.TokenProxyFailure, error) {
	body, err := response.ReadBody()
	if err != nil {
		if recordErr := s.RecordFailure(ctx, ports.TokenRequestUpstreamFailure, agentID); recordErr != nil {
			return ports.TokenProxyCompletion{}, ports.TokenProxyRecordingFailed, recordErr
		}
		return ports.TokenProxyCompletion{}, ports.TokenProxyResponseReadFailed, err
	}
	verified := false
	if s.verifier != nil && response.ResponseStatus() == tokenProxySuccessStatus {
		if err := s.verifier.VerifyAgentIDClaim(ctx, body, agentID); err != nil {
			if recordErr := s.RecordFailure(ctx, ports.TokenRequestUnauthenticated, agentID); recordErr != nil {
				return ports.TokenProxyCompletion{}, ports.TokenProxyRecordingFailed, recordErr
			}
			return ports.TokenProxyCompletion{}, ports.TokenProxyVerificationFailed, err
		}
		verified = true
	}
	if response.ResponseStatus() != tokenProxySuccessStatus {
		err = s.RecordFailure(ctx, ports.TokenRequestUpstreamFailure, agentID)
	} else {
		err = s.recordOutcome(ctx, "token-issued", agentID, map[string]any{})
	}
	if err != nil {
		return ports.TokenProxyCompletion{}, ports.TokenProxyRecordingFailed, err
	}
	return ports.TokenProxyCompletion{Body: body, Verified: verified}, ports.TokenProxyNoFailure, nil
}

func (s *TokenOutcomeService) RecordFailure(ctx context.Context, failure ports.TokenRequestFailure, agentID id.AgentID) error {
	var reason string
	switch failure {
	case ports.TokenRequestUnauthenticated:
		reason = "authentication_failed"
	case ports.TokenRequestUnauthorized:
		reason = "authorization_failed"
	case ports.TokenRequestUpstreamFailure:
		reason = "upstream_failed"
	case ports.TokenRequestInternalFailure:
		reason = "internal_failure"
	default:
		return errors.New("invalid token failure classification")
	}
	return s.recordOutcome(ctx, "token-request-failed", agentID, map[string]any{"reason_code": reason})
}

func (s *TokenOutcomeService) recordOutcome(ctx context.Context, eventType string, agentID id.AgentID, data map[string]any) error {
	event, err := s.ledger.NewEvent(ctx, model.BusinessEventTypePrefix+eventType, model.BusinessEvent{
		OccurredAt: time.Now().UTC(), AgentID: agentID, Actor: model.BusinessEventActor{Kind: "agent"}, Data: data,
	})
	if err != nil {
		return err
	}
	return s.ledger.Record(ctx, event)
}
