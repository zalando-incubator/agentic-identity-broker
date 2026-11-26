package consent

import (
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func sessionTokenParameter(rawQuery string) (string, error) {
	continuation := httpctx.ParseContinuationParameter(rawQuery)
	if continuation.Status == model.ContinuationInvalid {
		return "", errInvalidToken
	}
	return continuation.Token, nil
}
