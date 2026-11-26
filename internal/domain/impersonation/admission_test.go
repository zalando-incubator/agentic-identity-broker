package impersonation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestOPAAdmission(t *testing.T) {
	admit := newOPAAdmission([]string{"first", "second", "third"})

	t.Run("admits a candidate defining every OPA rule's decision", func(t *testing.T) {
		assert.NoError(t, admit(map[string]bool{"first": true, "second": true, "third": true}))
	})

	t.Run("rejects the whole candidate at the first rule in rule order", func(t *testing.T) {
		err := admit(map[string]bool{"first": true, "second": false, "third": false})
		var admissionErr *ports.ImpersonationPolicyAdmissionError
		require.True(t, errors.As(err, &admissionErr))
		assert.Equal(t, "second", admissionErr.Rule)
		assert.ErrorIs(t, err, ErrPolicyDecisionAbsent)
	})

	t.Run("treats a missing flag as absent", func(t *testing.T) {
		err := admit(map[string]bool{"first": true, "second": true})
		var admissionErr *ports.ImpersonationPolicyAdmissionError
		require.True(t, errors.As(err, &admissionErr))
		assert.Equal(t, "third", admissionErr.Rule)
	})
}

func TestOPAAdmissionReceivesOnlyOPARules(t *testing.T) {
	cfg := testImpersonationConfig("cel-rule", "opa-rule")
	cfg.Rules[1].Authorization = ports.ImpersonationAuthorizationConfig{Type: "opa"}
	cfg.OPA = ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{Path: "/policy"}}

	var refs []ports.ImpersonationPolicyDecisionRef
	var admission ports.ImpersonationPolicyAdmission
	factory := func(_ ports.ImpersonationOPAEngineConfig, decisions []ports.ImpersonationPolicyDecisionRef, admit ports.ImpersonationPolicyAdmission) (ports.ImpersonationPolicySource, error) {
		refs, admission = decisions, admit
		return newFakePolicySource(), nil
	}
	_, err := newTestServiceWithPolicies(t, cfg, factory)
	require.NoError(t, err)

	require.Len(t, refs, 1)
	assert.Equal(t, "opa-rule", refs[0].Rule)
	assert.NoError(t, admission(map[string]bool{"opa-rule": true}))
	assert.Error(t, admission(map[string]bool{"cel-rule": true}))
}
