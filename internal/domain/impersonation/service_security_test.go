package impersonation

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
)

type countingImpersonationJWKS struct {
	configurableJWKSProvider
	calls int
}

func (p *countingImpersonationJWKS) GetKeySet(ctx context.Context) (jwk.Set, error) {
	p.calls++
	return p.configurableJWKSProvider.GetKeySet(ctx)
}

func TestImpersonate_VerificationCacheRequiresSameJWKSProvider(t *testing.T) {
	signingKey, oldKeys := signedValidationKey(t, "old")
	_, rotatedKeys := signedValidationKey(t, "new")
	token := signedValidationToken(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	req := &Request{ClientAssertion: token, ActorToken: token, SubjectToken: token, SubjectTokenType: JWTTokenType}

	for _, tc := range []struct {
		name     string
		rotated  bool
		attempts int
	}{
		{name: "independently rotated provider rejects old key", rotated: true, attempts: 1},
		{name: "shared provider reuses verification per request", attempts: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig("deny", "allow")
			for i := range cfg.Rules {
				cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
				cfg.Rules[i].TrustedIssuers[0].JWKSURI = "https://idp.example.com/keys"
			}
			cfg.Rules[0].Authorization.CEL.Expression = "false"
			first := &countingImpersonationJWKS{configurableJWKSProvider: configurableJWKSProvider{set: oldKeys}}
			second := &countingImpersonationJWKS{configurableJWKSProvider: configurableJWKSProvider{set: rotatedKeys}}
			built := 0
			issuer := &stubIssuer{}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				built++
				if tc.rotated && built == 2 {
					return second, nil
				}
				return first, nil
			}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example.com", ledgerfixture.NewRecorder())
			require.NoError(t, err)

			for attempt := 1; attempt <= tc.attempts; attempt++ {
				outcome, err := svc.Impersonate(context.Background(), req, testTarget())
				if tc.rotated {
					require.Error(t, err)
					require.Equal(t, "access_denied", outcome.Audit.Outcome)
					require.Nil(t, outcome.Response)
					require.False(t, issuer.called)
					require.Equal(t, 1, second.calls)
				} else {
					require.NoError(t, err)
					require.NotNil(t, outcome.Response)
					require.Equal(t, "allow", outcome.Audit.SelectedRule)
					require.Equal(t, attempt, issuer.calls)
					require.Equal(t, 3*attempt, first.calls)
				}
			}
		})
	}
}
