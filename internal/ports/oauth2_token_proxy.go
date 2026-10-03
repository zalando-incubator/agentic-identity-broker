package ports

import (
	"context"
	"io"
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
)

// OAuth2TokenProxy performs transport only; the caller owns the response body.
type OAuth2TokenProxy interface {
	Exchange(ctx context.Context, encodedForm, contentType string) (TokenProxyResponse, TokenProxyFailure, error)
}

// TokenProxyResponse keeps HTTP I/O behind the transport port without copying its body.
type TokenProxyResponse interface {
	ResponseStatus() int
	HeaderValues(name string) []string
	ReadBody() ([]byte, error)
	StreamBody(dst io.Writer) (int64, error)
	CloseBody() error
}

type TokenProxyCompletion struct {
	Body     []byte
	Verified bool
}

// TokenOutcomeService applies client association and response-verification policy.
type TokenOutcomeService interface {
	Proxy(ctx context.Context, form url.Values, contentType string, resolution *TokenGrantResolution) (TokenProxyResponse, TokenProxyFailure, error)
	CompleteProxy(ctx context.Context, response TokenProxyResponse, agentID id.AgentID) (TokenProxyCompletion, TokenProxyFailure, error)
}
