package consent

import (
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func sessionTokenParameter(query url.Values) (string, error) {
	continuation := httpctx.ParseAuthorizationContinuation(query)
	if continuation.Status == model.ContinuationInvalid {
		return "", errInvalidToken
	}
	return continuation.Token, nil
}
