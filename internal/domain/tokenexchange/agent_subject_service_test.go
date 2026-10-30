package tokenexchange

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

func TestExchangeAgentSubjectRejection(t *testing.T) {
	for _, tc := range []struct {
		name, issuer, expression string
		wantDetail               FailureDetail
		wantCode                 string
	}{
		{"broker agent", "https://broker.example.com", "", DetailSubjectCategoryUnsupported, "access_denied"},
		{"external equal-claim token", "https://issuer.example.com", "", DetailSubjectCategoryUnsupported, "access_denied"},
		{"legacy broker human collision", "https://broker.example.com", "", DetailSubjectCategoryUnsupported, "access_denied"},
		{"mapped-principal broker human collision", "https://broker.example.com", "", DetailSubjectCategoryUnsupported, "access_denied"},
		{"unsigned-subject broker human output collision", "https://broker.example.com", "", DetailSubjectCategoryUnsupported, "access_denied"},
		{"compliant replacement", "https://issuer.example.com", "(has(subject_token.sub) && has(subject_token.agent_id) && subject_token.sub == subject_token.agent_id) || subject_token.kind == 'machine'", DetailSubjectCategoryUnsupported, "access_denied"},
		{"dynamic detector failure", "https://broker.example.com", "subject_token.sub", DetailCELConfiguration, "server_error"},
		{"evaluation failure", "https://broker.example.com", "subject_token.missing == 'agent'", DetailCELEvaluationFailed, "server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, req, grants, sessions := observationFixture(t)
			key, keyset := generateTestRSAKeySet(t)
			var err error
			svc.jwtValidator, err = NewJWTValidator(&MockJWKSProvider{keySet: keyset}, tc.issuer, "broker", 0)
			require.NoError(t, err)
			svc.celEvaluator, err = NewCELEvaluator(CELEvaluatorConfig{
				AgentSubjectExpression: tc.expression, AgentIDExpression: "subject_token.no_agent_id", AuthorizationExpression: "subject_token.no_policy == true",
			})
			require.NoError(t, err)
			claims := map[string]interface{}{"iss": tc.issuer, "aud": "broker", "sub": "same-text-human", "agent_id": "same-text-human", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
			req.SubjectToken = signServiceTestJWT(t, key, claims)
			claims["sub"] = "gateway"
			req.ClientAssertion = signServiceTestJWT(t, key, claims)
			holder := security.NewCaptureHolder(security.TransportCapture{})
			response, err := svc.Exchange(security.WithCaptureHolder(context.Background(), holder), req)
			require.Nil(t, response)
			var exchangeErr *TokenExchangeError
			require.ErrorAs(t, err, &exchangeErr)
			require.Equal(t, tc.wantCode, exchangeErr.Code())
			require.Equal(t, tc.wantDetail, exchangeErr.Diagnostic().Detail())
			require.Empty(t, exchangeErr.ErrorURI())
			require.Zero(t, grants.findByPrincipalAndAgentCalls)
			require.Zero(t, sessions.findByPrincipalAndServiceCalls)
			capture, ok := holder.Finalized()
			require.True(t, ok)
			require.Equal(t, "same-text-human", capture.Actor)
			require.Equal(t, "gateway", capture.CallingPeer)
		})
	}
}
