package impersonation

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestAgentSubjectRequiresEveryPrincipalExtraction(t *testing.T) {
	for _, role := range []string{"client_assertion", "actor", "subject"} {
		for _, identity := range []any{nil, "", 123} {
			t.Run(role+"/"+typeName(identity), func(t *testing.T) {
				cfg := testImpersonationConfig()
				cfg.Rules[0].AllowedSubjectCategories = []string{"agent"}
				cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
				entry := cfg.Rules[0].Roles[role]
				variable := map[string]string{"client_assertion": "client_assertion", "actor": "actor_token", "subject": "subject_token"}[role]
				entry.PrincipalExpression = variable + ".identity"
				cfg.Rules[0].Roles[role] = entry
				key, keyset := signedValidationKey(t, "extraction")
				mint := &stubIssuer{}
				verifier := &recordingDelegationVerifier{status: ports.UserDelegationActive}
				svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
					return configurableJWKSProvider{set: keyset}, nil
				}, stubAgentRepository{}, mint, 0, nil, verifier, "https://broker.example.com")
				require.NoError(t, err)
				target := testTarget()
				builder := jwt.NewBuilder().Issuer("https://idp.example.com").Audience([]string{"aud"}).Subject(target.Agent.ID.String()).Expiration(time.Now().Add(time.Hour))
				if identity != nil {
					builder = builder.Claim("identity", identity)
				}
				token, err := builder.Build()
				require.NoError(t, err)
				signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), key))
				require.NoError(t, err)
				outcome, err := svc.Impersonate(context.Background(), &Request{ClientAssertion: string(signed), ActorToken: string(signed), SubjectToken: string(signed), SubjectTokenType: JWTTokenType}, target)
				require.Error(t, err)
				require.Nil(t, outcome.Response)
				require.False(t, mint.called)
				require.Zero(t, verifier.calls)
				require.Empty(t, outcome.Audit.SubjectCategory)
			})
		}
	}
}

func typeName(value any) string {
	switch value.(type) {
	case nil:
		return "missing"
	case string:
		return "empty"
	default:
		return "nonstring"
	}
}
