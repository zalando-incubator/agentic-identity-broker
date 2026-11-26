package impersonation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestMixedRulesFallThroughExplicitOPADenies(t *testing.T) {
	h := newOPAHarness(t)
	rev := decidingRevision("bundle=r1", func(rule string) (ports.ImpersonationPolicyAction, error) {
		return ports.ImpersonationPolicyDeny, nil
	})
	h.source.set(rev)
	svc := h.mustService(opaRule("first"), opaRule("second"), celRule("third", `client_assertion.sub == "gw"`))
	outcome, err := svc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
	require.NoError(t, err)
	assert.Equal(t, "third", outcome.Audit.SelectedRule)
	assert.Equal(t, "cel", outcome.Audit.AuthorizationEngine)
	assert.Equal(t, 1, rev.evaluations("first"))
	assert.Equal(t, 1, rev.evaluations("second"))
}

func TestMixedRulesUseOneRevisionPerRequest(t *testing.T) {
	h := newOPAHarness(t)
	first := decidingRevision("bundle=r1", func(rule string) (ports.ImpersonationPolicyAction, error) {
		if rule == "first" {
			return ports.ImpersonationPolicyDeny, nil
		}
		return ports.ImpersonationPolicyAllow, nil
	})
	second := decidingRevision("bundle=r2", func(string) (ports.ImpersonationPolicyAction, error) {
		return ports.ImpersonationPolicyDeny, nil
	})
	h.source.set(first)
	svc := h.mustService(opaRule("first"), opaRule("second"))
	h.source.afterGet = func() { h.source.afterGet = nil; h.source.set(second) }

	outcome, err := svc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
	require.NoError(t, err, "a swap during the request does not change later rules' revision")
	assert.Equal(t, "second", outcome.Audit.SelectedRule)
	assert.Equal(t, "bundle=r1", outcome.Audit.PolicyRevision)
	assert.Zero(t, second.evaluations("second"))

	outcome, err = svc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
	assert.Equal(t, "access_denied", exchangeError(t, err).Code(), "the next request reads the new revision")
	assert.Equal(t, "bundle=r2", outcome.Audit.PolicyRevision)
}

func TestMixedRulesKeepDelegationTerminal(t *testing.T) {
	h := newOPAHarness(t)
	h.verifier.status = ports.UserDelegationMissing
	svc := h.mustService(opaRule("governed"), celRule("permissive", "true", "human", "agent"))
	_, err := svc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
	assert.Contains(t, exchangeError(t, err).ErrorURI(), "/agents/")
	assert.Equal(t, 1, h.verifier.calls, "delegation denial is never retried by a later rule")
}

func TestMixedRulesMatchCELOutcomes(t *testing.T) {
	for _, allow := range []bool{true, false} {
		h := newOPAHarness(t)
		h.source.set(decidingRevision("r", func(string) (ports.ImpersonationPolicyAction, error) {
			if allow {
				return ports.ImpersonationPolicyAllow, nil
			}
			return ports.ImpersonationPolicyDeny, nil
		}))
		expression := "false"
		if allow {
			expression = "true"
		}
		opaSvc := h.mustService(opaRule("rule"))
		celSvc := h.mustService(celRule("rule", expression))
		_, opaErr := opaSvc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
		_, celErr := celSvc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
		if allow {
			assert.NoError(t, opaErr)
			assert.NoError(t, celErr)
			continue
		}
		assert.Equal(t, exchangeError(t, celErr).Code(), exchangeError(t, opaErr).Code())
		assert.Equal(t, exchangeError(t, celErr).Diagnostic().Detail(), exchangeError(t, opaErr).Diagnostic().Detail())
	}
}
