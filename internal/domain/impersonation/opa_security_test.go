package impersonation

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestOPAUnsignedHumanMintsTheExtractedEmail(t *testing.T) {
	h := newOPAHarness(t)
	rev := allowingRevision("bundle=r1", "instance-1")
	h.source.set(rev)
	svc := h.mustService(unverified(opaRule("chat")))
	outcome, err := svc.Impersonate(context.Background(), h.request(unsignedSubjectJWT(t, map[string]any{"sub": "alice@example.com", "email": "alice@example.com"})), h.target)
	require.NoError(t, err)
	require.NotNil(t, h.issuer.input.Email)
	assert.Equal(t, "alice@example.com", *h.issuer.input.Email)
	assert.Equal(t, SubjectCategoryHuman, outcome.Audit.SubjectCategory)
	assert.Equal(t, 1, h.verifier.calls, "an unsigned human still needs delegation")
	assert.Equal(t, true, rev.lastInput("chat")["context"].(map[string]any)["subject_unverified"])

	denied := newOPAHarness(t)
	denied.source.set(decidingRevision("r", func(string) (ports.ImpersonationPolicyAction, error) { return ports.ImpersonationPolicyDeny, nil }))
	svc = denied.mustService(unverified(opaRule("chat")))
	_, err = svc.Impersonate(context.Background(), denied.request(unsignedSubjectJWT(t, map[string]any{"sub": "alice@example.com", "email": "alice@example.com"})), denied.target)
	assert.Equal(t, "access_denied", exchangeError(t, err).Code())
	assert.False(t, denied.issuer.called, "no email is minted without an allow")
}

func TestOPAUnsignedExactTargetIsAgentCategory(t *testing.T) {
	h := newOPAHarness(t)
	svc := h.mustService(unverified(opaRule("chat", "agent")))
	outcome, err := svc.Impersonate(context.Background(), h.request(unsignedSubjectJWT(t, map[string]any{"sub": h.target.Agent.ID.String()})), h.target)
	require.NoError(t, err)
	assert.Equal(t, SubjectCategoryAgent, outcome.Audit.SubjectCategory)
	assert.Equal(t, h.target.Agent.ID.String(), h.issuer.input.Subject)
	assert.Zero(t, h.verifier.calls)
}

func TestOPACrossAgentSubjectsAreRejected(t *testing.T) {
	for name, subject := range map[string]func(other string) map[string]any{
		"raw sub": func(other string) map[string]any { return map[string]any{"sub": other} },
		"extracted principal": func(other string) map[string]any {
			return map[string]any{"sub": "alice@example.com", "principal": other}
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newOPAHarness(t)
			other := &storage.Agent{ID: id.NewAgentID()}
			h.agents[other.ID] = other
			rule := unverified(opaRule("chat", "human", "agent"))
			subjectRole := rule.Roles["subject"]
			subjectRole.PrincipalExpression = "has(subject_token.principal) ? subject_token.principal : subject_token.sub"
			rule.Roles["subject"] = subjectRole
			svc := h.mustService(rule)
			_, err := svc.Impersonate(context.Background(), h.request(unsignedSubjectJWT(t, subject(other.ID.String()))), h.target)
			exchangeErr := exchangeError(t, err)
			assert.Equal(t, "access_denied", exchangeErr.Code())
			assert.Empty(t, exchangeErr.ErrorURI(), "no consent link")
			assert.Zero(t, h.verifier.calls, "no human fallback")
			assert.False(t, h.issuer.called)
		})
	}
}

func TestOPAAudienceAbsentRolesKeepEveryCredentialCheck(t *testing.T) {
	roleTokens := map[string]func(r *Request, token string){
		"client_assertion": func(r *Request, token string) { r.ClientAssertion = token },
		"actor":            func(r *Request, token string) { r.ActorToken = token },
		"subject":          func(r *Request, token string) { r.SubjectToken = token },
	}
	subjects := map[string]string{"client_assertion": "gw", "actor": "actor", "subject": "alice@example.com"}
	for _, roles := range [][]string{{"client_assertion"}, {"actor"}, {"subject"}, {"client_assertion", "actor"}, {"client_assertion", "actor", "subject"}} {
		t.Run(joinNames(roles), func(t *testing.T) {
			h := newOPAHarness(t)
			svc := h.mustService(audienceAbsent(opaRule("audienceless"), roles...))
			request := func() *Request {
				r := h.request(h.signed("alice@example.com", nil))
				for _, role := range roles {
					roleTokens[role](r, h.sign(subjects[role], nil, nil))
				}
				return r
			}
			_, err := svc.Impersonate(context.Background(), request(), h.target)
			require.NoError(t, err)

			for _, role := range roles {
				aud := "aud"
				otherKey, _ := signedValidationKey(t, "other")
				for variant, token := range map[string]string{
					"aud present":   h.sign(subjects[role], &aud, nil),
					"bad signature": signWith(t, otherKey, subjects[role], time.Now().Add(time.Hour)),
					"expired":       signWith(t, h.key, subjects[role], time.Now().Add(-time.Hour)),
				} {
					r := request()
					roleTokens[role](r, token)
					h.issuer.called = false
					_, err := svc.Impersonate(context.Background(), r, h.target)
					assert.Error(t, err, "%s on %s", variant, role)
					assert.False(t, h.issuer.called, "%s on %s even though OPA allows", variant, role)
				}
			}
		})
	}
}

func TestCELRulesKeepTheirBindingChecks(t *testing.T) {
	h := newOPAHarness(t)
	_, err := h.service(unverified(celRule("chat", `client_assertion.sub == "gw"`)))
	assert.ErrorContains(t, err, "requires the authorization predicate to reference subject_token (FR-007a)")

	_, err = h.service(audienceAbsent(celRule("audienceless", `subject_token.sub != ""`), "actor"))
	assert.ErrorContains(t, err, "(CR-009)")

	svc := h.mustService(unverified(celRule("chat", `!has(subject_token.nonce)`, "human", "agent")))
	outcome, err := svc.Impersonate(context.Background(), h.request(unsignedSubjectJWT(t, map[string]any{"sub": h.target.Agent.ID.String(), "email": "a@example.com"})), h.target)
	require.NoError(t, err)
	assert.Equal(t, SubjectCategoryHuman, outcome.Audit.SubjectCategory, "an unbound CEL predicate keeps the unsigned exact target human")
	assert.Nil(t, h.issuer.input.Email, "an unbound CEL predicate mints no caller-asserted email")
	assert.Equal(t, "cel", outcome.Audit.AuthorizationEngine)
	assert.Empty(t, outcome.Audit.PolicyAdmissionID)
}

func signWith(t *testing.T, key interface{}, sub string, expiry time.Time) string {
	t.Helper()
	token, err := jwt.NewBuilder().Issuer(opaTestIssuer).Subject(sub).Expiration(expiry).NotBefore(time.Now().Add(-2 * time.Hour)).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), key))
	require.NoError(t, err)
	return string(signed)
}

func joinNames(names []string) string {
	out := ""
	for i, name := range names {
		if i > 0 {
			out += "+"
		}
		out += name
	}
	return out
}
