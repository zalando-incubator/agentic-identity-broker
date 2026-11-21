package httpctx

import (
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func ParseAuthorizationContinuation(query url.Values) model.AuthorizationContinuation {
	values, present := query["session_token"]
	if !present {
		return model.AuthorizationContinuation{}
	}
	if len(values) != 1 || values[0] == "" {
		return model.AuthorizationContinuation{Status: model.ContinuationInvalid}
	}
	return model.AuthorizationContinuation{Token: values[0], Status: model.ContinuationPresent}
}
