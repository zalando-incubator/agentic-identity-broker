package impersonation

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestImpersonationCategoryRuleOrder(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		categories                                  [][]string
		human, unsigned, firstDenied, verifierError bool
		status                                      ports.UserDelegationStatus
		wantRule                                    string
		wantDelegation                              int
	}{
		{name: "ineligible first then independently eligible", categories: [][]string{{"human"}, {"agent"}}, wantRule: "second"},
		{name: "false policy then later agent", categories: [][]string{{"agent"}, {"agent"}}, firstDenied: true, wantRule: "second"},
		{name: "agent-only mismatch cannot fall back", categories: [][]string{{"agent"}}, human: true},
		{name: "explicit collision with active human delegation", categories: [][]string{{"human"}}, status: ports.UserDelegationActive},
		{name: "omitted collision with active human delegation", categories: [][]string{nil}, status: ports.UserDelegationActive},
		{name: "dual human active", categories: [][]string{{"human", "agent"}}, human: true, status: ports.UserDelegationActive, wantRule: "first", wantDelegation: 1},
		{name: "missing human is terminal", categories: [][]string{{"human", "agent"}, {"agent"}}, human: true, status: ports.UserDelegationMissing, wantDelegation: 1},
		{name: "expired human is terminal", categories: [][]string{{"human", "agent"}, {"human", "agent"}}, human: true, status: ports.UserDelegationExpired, wantDelegation: 1},
		{name: "verifier failure is terminal", categories: [][]string{{"human", "agent"}, {"human", "agent"}}, human: true, verifierError: true, wantDelegation: 1},
		{name: "unsigned exact text is delegated human", categories: [][]string{{"human", "agent"}}, unsigned: true, status: ports.UserDelegationActive, wantDelegation: 1, wantRule: "first"},
		{name: "unsigned exact text never agent", categories: [][]string{{"agent"}}, unsigned: true, status: ports.UserDelegationActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig("first", "second")
			cfg.Rules = cfg.Rules[:len(tc.categories)]
			for i, categories := range tc.categories {
				if tc.unsigned {
					cfg.Rules[i] = unverifiedSubjectConfig().Rules[0]
					cfg.Rules[i].Name = "first"
				}
				cfg.Rules[i].AllowedSubjectCategories = categories
				cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
				cfg.Rules[i].Authorization.CEL.Expression = "subject_token.sub != ''"
				if i == 0 && tc.firstDenied {
					cfg.Rules[i].Authorization.CEL.Expression = "false"
				}
			}
			key, keyset := signedValidationKey(t, "category-order")
			issuer := &stubIssuer{}
			verifier := &recordingDelegationVerifier{status: tc.status}
			if tc.verifierError {
				verifier.err = context.DeadlineExceeded
			}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return configurableJWKSProvider{set: keyset}, nil
			}, stubAgentRepository{}, issuer, 0, nil, verifier, "https://broker.example.com")
			require.NoError(t, err)
			target := testTarget()
			subject := target.Agent.ID.String()
			if tc.human {
				subject = "alice@example.com"
			}
			sign := func(sub string) string {
				return signedValidationTokenWithSubject(t, jwa.ES256(), key, "https://idp.example.com", "aud", sub, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
			}
			subjectToken := sign(subject)
			if tc.unsigned {
				subjectToken = unsignedSubjectJWT(t, map[string]any{"sub": subject})
			}
			outcome, err := svc.Impersonate(context.Background(), &Request{ClientAssertion: sign("gw"), ActorToken: sign("actor"), SubjectToken: subjectToken, SubjectTokenType: JWTTokenType}, target)
			require.Equal(t, tc.wantDelegation, verifier.calls)
			if tc.wantRule != "" {
				require.NoError(t, err)
				require.True(t, issuer.called)
				require.Equal(t, tc.wantRule, outcome.Audit.SelectedRule)
				if tc.unsigned {
					require.Equal(t, SubjectCategoryHuman, outcome.Audit.SubjectCategory)
				}
			} else {
				require.Error(t, err)
				require.False(t, issuer.called)
				var exchangeErr *tokenexchange.TokenExchangeError
				require.ErrorAs(t, err, &exchangeErr)
				if tc.verifierError {
					require.Equal(t, "server_error", exchangeErr.Code())
					require.Empty(t, exchangeErr.ErrorURI())
				} else {
					require.Equal(t, "access_denied", exchangeErr.Code())
					if tc.human && tc.wantDelegation > 0 {
						require.Contains(t, exchangeErr.ErrorURI(), "/agents/")
					} else {
						require.Empty(t, exchangeErr.ErrorURI())
					}
				}
			}
		})
	}
}
