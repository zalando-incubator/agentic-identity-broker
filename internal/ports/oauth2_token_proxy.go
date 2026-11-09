package ports

import (
	"context"
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type TokenProxyFailure uint8

const (
	TokenProxyNoFailure TokenProxyFailure = iota
	TokenProxyClientMissing
	TokenProxyRequestCreationFailed
	TokenProxyTransportFailed
	TokenProxyResponseReadFailed
	TokenProxyVerificationFailed
	TokenProxyRecordingFailed
)

// OAuth2TokenProxy performs transport only; the caller owns the response body.
type OAuth2TokenProxy interface {
	Exchange(ctx context.Context, encodedForm, contentType string) (TokenProxyResponse, TokenProxyFailure, error)
}

// TokenProxyResponse exposes the upstream response for staged domain completion.
type TokenProxyResponse interface {
	ResponseStatus() int
	HeaderValues(name string) []string
	ReadBody() ([]byte, error)
	CloseBody() error
}

type TokenProxyCompletion struct {
	Body     []byte
	Verified bool
}

type TokenRequestFailure uint8

const (
	TokenRequestUnauthenticated TokenRequestFailure = iota
	TokenRequestUnauthorized
	TokenRequestUpstreamFailure
	TokenRequestInternalFailure
)

type TokenFailureRecorder interface {
	RecordFailure(ctx context.Context, failure TokenRequestFailure, agentID id.AgentID) error
}

// TokenOutcomeService applies client association and response-verification policy.
type TokenOutcomeService interface {
	TokenFailureRecorder
	Proxy(ctx context.Context, form url.Values, contentType string, resolution *TokenGrantResolution) (TokenProxyResponse, TokenProxyFailure, error)
	CompleteProxy(ctx context.Context, response TokenProxyResponse, agentID id.AgentID) (TokenProxyCompletion, TokenProxyFailure, error)
}
