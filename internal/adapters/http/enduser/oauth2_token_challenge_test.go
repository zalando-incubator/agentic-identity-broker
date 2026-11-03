package enduser

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImpersonationConsentChallenge(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   tokenexchange.ExchangeKind
		detail tokenexchange.FailureDetail
		uri    string
		want   string
	}{
		{"consent required", tokenexchange.ExchangeImpersonation, tokenexchange.DetailConsentRequired, "https://broker.example.com/agents/agent-id", `Bearer error="insufficient_scope", error_uri="https://broker.example.com/agents/agent-id"`},
		{"missing grant", tokenexchange.ExchangeImpersonation, tokenexchange.DetailGrantMissing, "https://broker.example.com/agents/agent-id", `Bearer error="insufficient_scope", error_uri="https://broker.example.com/agents/agent-id"`},
		{"unsafe URI", tokenexchange.ExchangeImpersonation, tokenexchange.DetailGrantMissing, "https://broker.example.com/\r\nInjected: value", `Bearer error="insufficient_scope"`},
		{"quoted URI", tokenexchange.ExchangeImpersonation, tokenexchange.DetailGrantMissing, "https://broker.example.com/\"", `Bearer error="insufficient_scope"`},
		{"non-ASCII URI", tokenexchange.ExchangeImpersonation, tokenexchange.DetailGrantMissing, "https://broker.example.com/é", `Bearer error="insufficient_scope"`},
		{"policy denied", tokenexchange.ExchangeImpersonation, tokenexchange.DetailClientPolicyDenied, "", ""},
		{"third-party consent", tokenexchange.ExchangeThirdParty, tokenexchange.DetailGrantMissing, "https://broker.example.com/agents/agent-id", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tokenexchange.NewAccessDeniedError("denied").WithErrorURI(tc.uri).
				WithDiagnostic(tokenexchange.NewDiagnostic(tokenexchange.StageGrantAuthorization, tc.detail).WithExchangeKind(tc.kind))
			w := httptest.NewRecorder()
			require.NoError(t, (&OAuth2TokenHandler{}).handleTokenExchangeError(w, err))
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Equal(t, tc.want, w.Header().Get("WWW-Authenticate"))
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}
