package impersonation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestOPAFailuresAreTerminalWithoutFallthrough(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		detail tokenexchange.FailureDetail
	}{
		"invalid decision":  {fmt.Errorf("undefined: %w", ports.ErrPolicyDecisionInvalid), tokenexchange.DetailOPADecisionInvalid},
		"evaluation failed": {fmt.Errorf("timeout: %w", ports.ErrPolicyEvaluationFailed), tokenexchange.DetailOPAEvaluationFailed},
	} {
		t.Run(name, func(t *testing.T) {
			h := newOPAHarness(t)
			h.source.set(decidingRevision("bundle=r1", func(string) (ports.ImpersonationPolicyAction, error) { return "", tc.err }))
			svc := h.mustService(opaRule("governed"), celRule("permissive", "true"))
			outcome, err := svc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", nil)), h.target)
			exchangeErr := exchangeError(t, err)
			assert.Equal(t, "server_error", exchangeErr.Code())
			assert.Equal(t, "authorization evaluation failed", exchangeErr.Description())
			assert.Equal(t, tc.detail, exchangeErr.Diagnostic().Detail())
			assert.Equal(t, tokenexchange.StageClientAuthorization, exchangeErr.Diagnostic().Stage())
			assert.Equal(t, "governed", outcome.Audit.SelectedRule)
			assert.False(t, h.issuer.called, "the permissive later rule never runs")
			assert.Zero(t, h.verifier.calls)
		})
	}
}

func TestOPACallerCancellationIsTerminal(t *testing.T) {
	h := newOPAHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.source.set(decidingRevision("bundle=r1", func(string) (ports.ImpersonationPolicyAction, error) {
		cancel()
		return "", fmt.Errorf("canceled: %w: %w", ports.ErrPolicyEvaluationFailed, context.Canceled)
	}))
	svc := h.mustService(opaRule("governed"), celRule("permissive", "true"))
	_, err := svc.Impersonate(ctx, h.request(h.signed("alice@example.com", nil)), h.target)
	exchangeErr := exchangeError(t, err)
	require.Equal(t, "server_error", exchangeErr.Code())
	assert.Equal(t, tokenexchange.DetailCallerCanceled, exchangeErr.Diagnostic().Detail())
	assert.False(t, h.issuer.called)
}

func TestOPAFailureCarriesNoPolicyOrClaimText(t *testing.T) {
	const marker = "SYNTHETIC-POLICY-TEXT"
	h := newOPAHarness(t)
	h.source.set(decidingRevision("bundle=r1", func(string) (ports.ImpersonationPolicyAction, error) {
		return "", fmt.Errorf("%s: %w", marker, ports.ErrPolicyDecisionInvalid)
	}))
	svc := h.mustService(opaRule("governed"))
	_, err := svc.Impersonate(context.Background(), h.request(h.signed("alice@example.com", map[string]any{"email": marker})), h.target)
	exchangeErr := exchangeError(t, err)
	assert.NotContains(t, exchangeErr.Description(), marker)
}
