package httpctx

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func TestParseAuthorizationContinuation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values url.Values
		want   model.AuthorizationContinuation
	}{
		{"absent", url.Values{}, model.AuthorizationContinuation{}},
		{"present", url.Values{"session_token": {"synthetic-JWE"}}, model.AuthorizationContinuation{Token: "synthetic-JWE", Status: model.ContinuationPresent}},
		{"empty", url.Values{"session_token": {""}}, model.AuthorizationContinuation{Status: model.ContinuationInvalid}},
		{"no values", url.Values{"session_token": {}}, model.AuthorizationContinuation{Status: model.ContinuationInvalid}},
		{"repeated", url.Values{"session_token": {"synthetic-JWE", "synthetic-JWE"}}, model.AuthorizationContinuation{Status: model.ContinuationInvalid}},
		{"leading empty", url.Values{"session_token": {"", "synthetic-JWE"}}, model.AuthorizationContinuation{Status: model.ContinuationInvalid}},
		{"trailing empty", url.Values{"session_token": {"synthetic-JWE", ""}}, model.AuthorizationContinuation{Status: model.ContinuationInvalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseAuthorizationContinuation(tc.values))
		})
	}
}
