package opa

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func localRevision(t *testing.T, modules map[string]string, data map[string]any, decisions ...ports.ImpersonationPolicyDecisionRef) ports.ImpersonationPolicyRevision {
	t.Helper()
	s := &source{decisions: decisions, admit: admitAll, telemetry: newTelemetry(discardLogger()), instance: "test", admitted: make(chan struct{})}
	require.True(t, s.consider(context.Background(), candidateInput{
		modules: modules, data: data, label: "local=", configured: []string{"local"}, activated: []string{"local"},
	}), "rejected: %+v", s.lastRejection)
	return s.Current()
}

func TestRevisionDecisions(t *testing.T) {
	module := func(body string) map[string]string {
		return map[string]string{"/p.rego": "package company.impersonation\n\nimport rego.v1\n\n" + body + "\n"}
	}
	cases := map[string]struct {
		body   string
		want   ports.ImpersonationPolicyAction
		target error
	}{
		"allow with ignored members": {`result := {"action": "allow", "reasons": ["x"], "approval_context": {}}`, ports.ImpersonationPolicyAllow, nil},
		"deny":                       {`result := {"action": "deny", "reasons": ["governance"]}`, ports.ImpersonationPolicyDeny, nil},
		"undefined":                  {`result := {"action": "allow"} if input.never`, "", ports.ErrPolicyDecisionInvalid},
		"non-object":                 {`result := "allow"`, "", ports.ErrPolicyDecisionInvalid},
		"set of results":             {`result contains {"action": "allow"}`, "", ports.ErrPolicyDecisionInvalid},
		"missing action":             {`result := {"allow": true}`, "", ports.ErrPolicyDecisionInvalid},
		"non-string action":          {`result := {"action": true}`, "", ports.ErrPolicyDecisionInvalid},
		"approval required":          {`result := {"action": "approval_required"}`, "", ports.ErrPolicyDecisionInvalid},
		"ciba required":              {`result := {"action": "ciba_required"}`, "", ports.ErrPolicyDecisionInvalid},
		"strict built-in error":      {`result := {"action": "allow"} if to_number(input.text) > 0`, "", ports.ErrPolicyEvaluationFailed},
		"conflicting values":         {`result := {"action": "allow"} if true` + "\n\n" + `result := {"action": "deny"} if true`, "", ports.ErrPolicyEvaluationFailed},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rev := localRevision(t, module(tc.body), nil, decision("rule", "company.impersonation", "result"))
			action, err := rev.Evaluate(context.Background(), "rule", ports.ImpersonationPolicyInput{"text": "not-a-number"})
			if tc.target != nil {
				assert.ErrorIs(t, err, tc.target)
				assert.Empty(t, action)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, action)
		})
	}
}

func TestRevisionEvaluationReceivesTheTypedInput(t *testing.T) {
	rev := localRevision(t, map[string]string{"/p.rego": `package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.context.client_assertion.level == 3
	input.context.actor_token.groups == ["a", "b"]
	input.context.subject_token.profile.nested == {"k": true}
	data.governance.allowed
}
`}, governance(true), decision("rule", "company.impersonation", "result"))
	action, err := rev.Evaluate(context.Background(), "rule", ports.ImpersonationPolicyInput{"context": map[string]any{
		"client_assertion": map[string]any{"level": float64(3)},
		"actor_token":      map[string]any{"groups": []any{"a", "b"}},
		"subject_token":    map[string]any{"profile": map[string]any{"nested": map[string]any{"k": true}}},
	}})
	require.NoError(t, err)
	assert.Equal(t, ports.ImpersonationPolicyAllow, action)
}

func TestRevisionTimeoutAndCancellation(t *testing.T) {
	slow := map[string]string{"/p.rego": `package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	some i in numbers.range(1, 50000)
	some j in numbers.range(1, 50000)
	i + j < 0
}
`}
	ref := decision("rule", "company.impersonation", "result")
	ref.EvaluationTimeout = 20 * time.Millisecond
	rev := localRevision(t, slow, nil, ref)

	start := time.Now()
	_, err := rev.Evaluate(context.Background(), "rule", nil)
	assert.ErrorIs(t, err, ports.ErrPolicyEvaluationFailed)
	assert.Less(t, time.Since(start), 2*time.Second, "the evaluation timeout bounds the decision")

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	_, err = localRevision(t, slow, nil, decision("rule", "company.impersonation", "result")).Evaluate(ctx, "rule", nil)
	assert.ErrorIs(t, err, ports.ErrPolicyEvaluationFailed)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestRevisionUnknownRuleFailsClosed(t *testing.T) {
	rev := localRevision(t, map[string]string{"/p.rego": allowPolicy}, governance(true), decision("rule", "company.impersonation", "result"))
	_, err := rev.Evaluate(context.Background(), "other", nil)
	assert.ErrorIs(t, err, ports.ErrPolicyEvaluationFailed)
}
