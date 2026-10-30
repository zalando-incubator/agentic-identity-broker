package impersonation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestClassifySubject(t *testing.T) {
	target := id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000")
	require.Equal(t, SubjectCategoryHuman, classifySubject(false, map[string]interface{}{}, target))
	for _, tc := range []struct {
		sub      any
		unsigned bool
		want     SubjectCategory
	}{
		{target.String(), false, SubjectCategoryAgent}, {target.String(), true, SubjectCategoryHuman},
		{strings.ToUpper(target.String()), false, SubjectCategoryHuman}, {"{" + target.String() + "}", false, SubjectCategoryHuman},
		{" " + target.String(), false, SubjectCategoryHuman},
		{"research-agent", false, SubjectCategoryHuman}, {id.NewAgentID().String(), false, SubjectCategoryHuman},
		{"", false, SubjectCategoryHuman}, {nil, false, SubjectCategoryHuman}, {123, false, SubjectCategoryHuman},
	} {
		t.Run(fmt.Sprintf("%v/%v", tc.sub, tc.unsigned), func(t *testing.T) {
			require.Equal(t, tc.want, classifySubject(tc.unsigned, map[string]interface{}{"sub": tc.sub}, target))
		})
	}
}

func TestCompiledRuleSubjectEligibility(t *testing.T) {
	for _, categories := range [][]string{nil, {"human"}, {"agent"}, {"human", "agent"}, {"agent", "human"}} {
		t.Run(fmt.Sprint(categories), func(t *testing.T) {
			cfg := testImpersonationConfig()
			cfg.Rules[0].AllowedSubjectCategories = categories
			svc, _ := newTestService(t, cfg)
			rule := svc.rules[0]
			wantHuman, wantAgent := categories == nil, false
			for _, value := range categories {
				wantHuman = wantHuman || value == "human"
				wantAgent = wantAgent || value == "agent"
			}
			require.Equal(t, wantHuman, rule.allows(SubjectCategoryHuman))
			require.Equal(t, wantAgent, rule.allows(SubjectCategoryAgent))
			require.False(t, rule.allows(SubjectCategory("unknown")))
		})
	}
}

func TestImpersonateAgentSubject(t *testing.T) {
	for _, tc := range []struct {
		name                                                string
		categories                                          []string
		mapped, mappedTarget, badEmail, human, policyDenied bool
		wantToken                                           bool
		wantDelegation                                      int
	}{
		{name: "agent skips missing delegation", categories: []string{"agent"}, wantToken: true},
		{name: "agent target overrides successful extraction", categories: []string{"agent"}, mapped: true, wantToken: true},
		{name: "agent skips credential email evaluation", categories: []string{"agent"}, badEmail: true, wantToken: true},
		{name: "dual agent", categories: []string{"human", "agent"}, wantToken: true},
		{name: "omission rejects collision"},
		{name: "human-only rejects collision", categories: []string{"human"}},
		{name: "policy denial remains denial", categories: []string{"agent"}, policyDenied: true},
		{name: "human stays delegated", categories: []string{"human", "agent"}, human: true, wantDelegation: 1},
		{name: "extracted target does not classify raw human as agent", categories: []string{"human", "agent"}, human: true, mappedTarget: true, wantDelegation: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := testTarget()
			key, keyset := signedValidationKey(t, "subject-category")
			cfg := testImpersonationConfig()
			rule := &cfg.Rules[0]
			rule.AllowedSubjectCategories = tc.categories
			rule.TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
			subjectRole := rule.Roles["subject"]
			subjectRole.EmailExpression = "subject_token.email"
			if tc.mapped {
				subjectRole.PrincipalExpression = "'mapped-principal'"
			}
			if tc.mappedTarget {
				subjectRole.PrincipalExpression = "'" + target.Agent.ID.String() + "'"
			}
			if tc.badEmail {
				subjectRole.EmailExpression = "subject_token.absent_email"
			}
			rule.Roles["subject"] = subjectRole
			if tc.policyDenied {
				rule.Authorization.CEL.Expression = "false"
			} else {
				rule.Authorization.CEL.Expression = "subject_token.sub != '' && request.agent_id != ''"
			}
			issuer := &stubIssuer{}
			verifier := &recordingDelegationVerifier{status: ports.UserDelegationMissing}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return configurableJWKSProvider{set: keyset}, nil
			}, stubAgentRepository{}, issuer, 0, nil, verifier, "https://broker.example.com")
			require.NoError(t, err)
			subject := target.Agent.ID.String()
			if tc.human {
				subject = "alice@example.com"
			}
			sign := func(sub string) string {
				token, err := jwt.NewBuilder().Issuer("https://idp.example.com").Audience([]string{"aud"}).Subject(sub).Expiration(time.Now().Add(time.Hour)).Claim("email", "credential@example.com").Build()
				require.NoError(t, err)
				signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), key))
				require.NoError(t, err)
				return string(signed)
			}
			outcome, err := svc.Impersonate(context.Background(), &Request{ClientAssertion: sign("gateway"), ActorToken: sign("actor"), SubjectToken: sign(subject), SubjectTokenType: JWTTokenType}, target)
			require.Equal(t, tc.wantDelegation, verifier.calls)
			require.Equal(t, tc.wantToken, issuer.called)
			if tc.wantToken {
				require.NoError(t, err)
				require.Equal(t, target.Agent.ID.String(), issuer.input.Subject)
				require.Nil(t, issuer.input.Email)
				require.Equal(t, "actor", issuer.input.Actor)
				require.Equal(t, "https://idp.example.com", issuer.input.ActorIssuer)
				require.Equal(t, SubjectCategoryAgent, outcome.Audit.SubjectCategory)
				require.Equal(t, target.Agent.ID.String(), outcome.Audit.SubjectIdentity)
			} else {
				var exchangeErr *tokenexchange.TokenExchangeError
				require.ErrorAs(t, err, &exchangeErr)
				require.Equal(t, "access_denied", exchangeErr.Code())
				if !tc.human {
					require.Empty(t, exchangeErr.ErrorURI())
				}
			}
		})
	}
}
