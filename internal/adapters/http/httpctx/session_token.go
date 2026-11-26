package httpctx

import (
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func ParseContinuationParameter(query url.Values) model.ContinuationParameter {
	values, present := query["session_token"]
	if !present {
		return model.ContinuationParameter{}
	}
	if len(values) != 1 || values[0] == "" {
		return model.ContinuationParameter{Status: model.ContinuationInvalid}
	}
	return model.ContinuationParameter{Token: values[0], Status: model.ContinuationPresent}
}
