package tokenexchange

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
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
		{"evaluation failure", "https://broker.example.com", "subject_token.sub == subject_token.missing", DetailCELEvaluationFailed, "server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, req, grants, sessions := observationFixture(t)
			store := &ledgerfixture.Store{}
			svc.ledger = store.Recorder(t)
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
			require.Len(t, store.Events, 1)
			event := store.Events[0]
			require.Nil(t, event.Subject)
			require.Nil(t, event.Actor.OnBehalfOf)
			require.True(t, event.AgentID.IsZero(), "agent detection precedes agent extraction")
			require.Equal(t, "gateway", *event.Actor.ID)
			if tc.wantCode == "access_denied" {
				require.Equal(t, "agentic-identity-broker.token-exchange-denied", event.Type)
				require.Equal(t, model.BusinessEventDenied, event.Outcome)
				require.Equal(t, "authorization_failed", event.Data["reason_code"])
			} else {
				require.Equal(t, "agentic-identity-broker.token-request-failed", event.Type)
				require.Equal(t, "internal_failure", event.Data["reason_code"])
			}
			require.Zero(t, grants.findByPrincipalAndAgentCalls)
			require.Zero(t, sessions.findByPrincipalAndServiceCalls)
			capture, ok := holder.Finalized()
			require.True(t, ok)
			require.Equal(t, id.NewPrincipal("same-text-human"), capture.Actor)
			require.Equal(t, "gateway", capture.CallingPeer)
		})
	}
}
