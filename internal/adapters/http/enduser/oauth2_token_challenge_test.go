package enduser

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenExchangeConsentChallenge(t *testing.T) {
	const consentURL = "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"
	for _, tc := range []struct {
		name   string
		uri    string
		detail tokenexchange.FailureDetail
		kind   tokenexchange.ExchangeKind
		want   string
	}{
		{"human impersonation needs consent", consentURL, tokenexchange.DetailGrantMissing, tokenexchange.ExchangeImpersonation, `Bearer error="insufficient_scope", error_uri="` + consentURL + `"`},
		{"unrepresentable URI stays out of the header", consentURL + `"`, tokenexchange.DetailGrantMissing, tokenexchange.ExchangeImpersonation, `Bearer error="insufficient_scope"`},
		{"newline cannot inject response headers", consentURL + "\r\nX-Injected: true", tokenexchange.DetailConsentMandatorySessionMissing, tokenexchange.ExchangeImpersonation, `Bearer error="insufficient_scope"`},
		{"non-ASCII URI is excluded from challenge", consentURL + "/café", tokenexchange.DetailGrantExpired, tokenexchange.ExchangeImpersonation, `Bearer error="insufficient_scope"`},
		{"policy denial has no consent challenge", consentURL, tokenexchange.DetailClientPolicyDenied, tokenexchange.ExchangeImpersonation, ""},
		{"third party consent has no new challenge", consentURL, tokenexchange.DetailGrantMissing, tokenexchange.ExchangeThirdParty, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tokenexchange.NewAccessDeniedError("user authorization is required").
				WithErrorURI(tc.uri).
				WithDiagnostic(tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tc.detail).WithExchangeKind(tc.kind))
			response := httptest.NewRecorder()
			require.NoError(t, (&OAuth2TokenHandler{}).handleTokenExchangeError(response, err))
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Equal(t, tc.want, response.Header().Get("WWW-Authenticate"))
			assert.Contains(t, response.Body.String(), `"error":"access_denied"`)
			assert.Contains(t, response.Body.String(), `"error_uri":"`)
		})
	}
}
