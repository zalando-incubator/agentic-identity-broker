package enduser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The token endpoint is not an RFC 6750 protected resource: consent recovery travels only
// in the RFC 6749 §5.2 error_uri, and no response carries an authentication challenge.
func TestTokenExchangeConsentErrorHasNoChallenge(t *testing.T) {
	const consentURL = "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"
	for _, tc := range []struct {
		name       string
		err        *tokenexchange.TokenExchangeError
		kind       tokenexchange.ExchangeKind
		wantStatus int
		wantCode   string
	}{
		{"human impersonation needs consent", tokenexchange.NewInvalidGrantError("consent required").WithErrorURI(consentURL).
			WithDiagnostic(tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantMissing)),
			tokenexchange.ExchangeImpersonation, http.StatusBadRequest, "invalid_grant"},
		{"human impersonation needs a mandatory session", tokenexchange.NewInvalidGrantError("consent required").WithErrorURI(consentURL).
			WithDiagnostic(tokenexchange.NewDiagnostic(tokenexchange.StageSessionLookup, tokenexchange.DetailConsentMandatorySessionMissing)),
			tokenexchange.ExchangeImpersonation, http.StatusBadRequest, "invalid_grant"},
		{"impersonation access denial with consent recovery", tokenexchange.NewAccessDeniedError("consent required").WithErrorURI(consentURL).
			WithDiagnostic(tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantExpired)),
			tokenexchange.ExchangeImpersonation, http.StatusForbidden, "access_denied"},
		{"third party consent", tokenexchange.NewAccessDeniedError("consent required").WithErrorURI(consentURL).
			WithDiagnostic(tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantMissing)),
			tokenexchange.ExchangeThirdParty, http.StatusForbidden, "access_denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.err.WithDiagnostic(tc.err.Diagnostic().WithExchangeKind(tc.kind))
			response := httptest.NewRecorder()
			require.NoError(t, (&OAuth2TokenHandler{}).handleTokenExchangeError(response, err))
			assert.Equal(t, tc.wantStatus, response.Code)
			assert.Empty(t, response.Header().Values("WWW-Authenticate"))
			var body map[string]string
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, tc.wantCode, body["error"])
			assert.Equal(t, consentURL, body["error_uri"])
		})
	}
}
