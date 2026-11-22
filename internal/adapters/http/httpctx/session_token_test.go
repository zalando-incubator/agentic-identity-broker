package httpctx

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func TestParseContinuationParameter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values url.Values
		want   model.ContinuationParameter
	}{
		{"absent", url.Values{}, model.ContinuationParameter{}},
		{"present", url.Values{"session_token": {"synthetic-JWE"}}, model.ContinuationParameter{Token: "synthetic-JWE", Status: model.ContinuationPresent}},
		{"empty", url.Values{"session_token": {""}}, model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"no values", url.Values{"session_token": {}}, model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"repeated", url.Values{"session_token": {"synthetic-JWE", "synthetic-JWE"}}, model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"leading empty", url.Values{"session_token": {"", "synthetic-JWE"}}, model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"trailing empty", url.Values{"session_token": {"synthetic-JWE", ""}}, model.ContinuationParameter{Status: model.ContinuationInvalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseContinuationParameter(tc.values))
		})
	}
}
