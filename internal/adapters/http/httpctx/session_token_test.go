package httpctx

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func TestParseContinuationParameter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rawQuery string
		want     model.ContinuationParameter
	}{
		{"absent", "client_id=agent&scope=openid", model.ContinuationParameter{}},
		{"empty query", "", model.ContinuationParameter{}},
		{"present", "client_id=agent&session_token=synthetic-JWE", model.ContinuationParameter{Token: "synthetic-JWE", Status: model.ContinuationPresent}},
		{"percent-encoded", "session_token=synthetic%2EJWE", model.ContinuationParameter{Token: "synthetic.JWE", Status: model.ContinuationPresent}},
		{"empty", "session_token=", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"no value", "session_token", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"repeated", "session_token=synthetic-JWE&session_token=synthetic-JWE", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"leading empty", "session_token=&session_token=synthetic-JWE", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"trailing empty", "session_token=synthetic-JWE&session_token=", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"malformed only", "session_token=%ZZ", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"valid then malformed repeat", "session_token=synthetic-JWE&session_token=%ZZ", model.ContinuationParameter{Status: model.ContinuationInvalid}},
		{"malformed unrelated parameter", "state=%ZZ&session_token=synthetic-JWE", model.ContinuationParameter{Token: "synthetic-JWE", Status: model.ContinuationPresent}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseContinuationParameter(tc.rawQuery))
		})
	}
}
