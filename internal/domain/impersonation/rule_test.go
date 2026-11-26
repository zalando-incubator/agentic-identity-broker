package impersonation

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestRuleSubjectCategories(t *testing.T) {
	for _, tc := range []struct {
		categories []string
		valid      bool
	}{
		{nil, true}, {[]string{"human"}, true}, {[]string{"agent"}, true},
		{[]string{"human", "agent"}, true}, {[]string{"agent", "human"}, true},
		{[]string{}, false}, {[]string{"machine"}, false}, {[]string{"Human"}, false},
		{[]string{"human", "human"}, false}, {[]string{"human", "agent", "human"}, false},
	} {
		t.Run(fmt.Sprint(tc.categories), func(t *testing.T) {
			cfg := testImpersonationConfig("categories").Rules[0]
			cfg.AllowedSubjectCategories = tc.categories
			err := validateRuleInvariants(cfg)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "allowed_subject_categories")
			}
		})
	}
}

func TestValidateRuleInvariantsAcceptsExactlyOneEngine(t *testing.T) {
	base := func() ports.ImpersonationRuleConfig { return opaRule("governed") }
	require.NoError(t, validateRuleInvariants(base()))

	for name, tc := range map[string]struct {
		mutate func(*ports.ImpersonationRuleConfig)
		want   string
	}{
		"unknown engine":   {func(r *ports.ImpersonationRuleConfig) { r.Authorization.Type = "rego" }, `authorization.type: must be "cel" or "opa"`},
		"no engine":        {func(r *ports.ImpersonationRuleConfig) { r.Authorization.Type = "" }, `authorization.type: must be "cel" or "opa"`},
		"CEL block on OPA": {func(r *ports.ImpersonationRuleConfig) { r.Authorization.CEL.Expression = "true" }, `authorization.cel: must be empty when authorization.type is "opa"`},
		"OPA block on CEL": {func(r *ports.ImpersonationRuleConfig) {
			r.Authorization = celRule("x", "true").Authorization
			r.Authorization.OPA.Package = "p"
		}, `authorization.opa: must be empty when authorization.type is "cel"`},
		"invalid package":      {func(r *ports.ImpersonationRuleConfig) { r.Authorization.OPA.Package = "company..x" }, "authorization.opa.package"},
		"invalid decision":     {func(r *ports.ImpersonationRuleConfig) { r.Authorization.OPA.Decision = "1st" }, "authorization.opa.decision"},
		"timeout out of range": {func(r *ports.ImpersonationRuleConfig) { r.Authorization.OPA.EvaluationTimeout = 6 * time.Second }, "authorization.opa.evaluation_timeout"},
	} {
		t.Run(name, func(t *testing.T) {
			rule := base()
			tc.mutate(&rule)
			assert.ErrorContains(t, validateRuleInvariants(rule), tc.want)
		})
	}
}

func TestOPADecisionDefaultsApplyAtUse(t *testing.T) {
	assert.Equal(t, ports.ImpersonationPolicyDecisionRef{
		Rule: "governed", Package: "aib.broker.impersonation", Decision: "result", EvaluationTimeout: 100 * time.Millisecond,
	}, opaDecisionRef(opaRule("governed")))

	rule := opaRule("governed")
	rule.Authorization.OPA = ports.ImpersonationRuleOPADecision{Package: "company.impersonation", Decision: "allow", EvaluationTimeout: time.Second}
	assert.Equal(t, ports.ImpersonationPolicyDecisionRef{
		Rule: "governed", Package: "company.impersonation", Decision: "allow", EvaluationTimeout: time.Second,
	}, opaDecisionRef(rule))
}

func TestCompileRuleSkipsCELBindingChecksForOPARules(t *testing.T) {
	jwks := func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return stubJWKSProvider{}, nil
	}
	compiled, err := compileRule(audienceAbsent(unverified(opaRule("governed")), "actor"), jwks, time.Minute)
	require.NoError(t, err, "OPA rules have no static reference or binding check")
	require.NotNil(t, compiled.opa)
	assert.Nil(t, compiled.authz)
	assert.True(t, compiled.authorizationFor(nil, nil).bindsSubjectField("sub"))
	assert.Equal(t, "opa", compiled.authorizationFor(nil, nil).engine())
}
