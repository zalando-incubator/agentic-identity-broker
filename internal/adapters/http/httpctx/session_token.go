package httpctx

import (
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// ParseContinuationParameter classifies the session_token parameter of a raw query. It reads
// the raw query because url.Values silently drops values that fail to decode, which would
// turn an invalid continuation into an absent or single valid one.
func ParseContinuationParameter(rawQuery string) model.ContinuationParameter {
	var values []string
	present, malformed := false, false
	for pair := range strings.SplitSeq(rawQuery, "&") {
		rawKey, rawValue, _ := strings.Cut(pair, "=")
		if key, err := url.QueryUnescape(rawKey); err != nil || key != "session_token" {
			continue
		}
		present = true
		value, err := url.QueryUnescape(rawValue)
		if err != nil {
			malformed = true
			continue
		}
		values = append(values, value)
	}
	if !present {
		return model.ContinuationParameter{}
	}
	if malformed || len(values) != 1 || values[0] == "" {
		return model.ContinuationParameter{Status: model.ContinuationInvalid}
	}
	return model.ContinuationParameter{Token: values[0], Status: model.ContinuationPresent}
}
