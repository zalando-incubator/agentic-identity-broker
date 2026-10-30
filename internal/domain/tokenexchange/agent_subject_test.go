package tokenexchange

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentSubjectDetection(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		claims           map[string]interface{}
		want             bool
		failure          FailureDetail
	}{
		{"default equality", "", map[string]interface{}{"sub": "agent", "agent_id": "agent"}, true, ""},
		{"missing", "", map[string]interface{}{"sub": "human"}, false, ""},
		{"no sub", "", map[string]interface{}{"agent_id": "agent"}, false, ""},
		{"unequal", "", map[string]interface{}{"sub": "human", "agent_id": "agent"}, false, ""},
		{"type mismatch", "", map[string]interface{}{"sub": "1", "agent_id": 1}, false, ""},
		{"replacement", "false", map[string]interface{}{"sub": "agent", "agent_id": "agent"}, false, ""},
		{"external machine", "subject_token.kind == 'machine'", map[string]interface{}{"kind": "machine"}, true, ""},
		{"dynamic nonboolean", "subject_token.sub", map[string]interface{}{"sub": "agent"}, false, DetailCELConfiguration},
		{"evaluation error", "subject_token.missing == 'agent'", map[string]interface{}{"sub": "agent"}, false, DetailCELEvaluationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewCELEvaluator(CELEvaluatorConfig{AgentSubjectExpression: tc.expression})
			require.NoError(t, err)
			got, err := e.IsAgentSubject(tc.claims)
			if tc.failure != "" {
				var exchangeErr *TokenExchangeError
				require.ErrorAs(t, err, &exchangeErr)
				require.Equal(t, "server_error", exchangeErr.Code())
				require.Equal(t, StageIdentityResolution, exchangeErr.Diagnostic().Stage())
				require.Equal(t, tc.failure, exchangeErr.Diagnostic().Detail())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestAgentSubjectDetectionStartup(t *testing.T) {
	for _, expression := range []string{"(", "unknown.sub", "'agent'", "1", "[true]", "client_assertion.sub == 'agent'"} {
		t.Run(expression, func(t *testing.T) {
			_, err := NewCELEvaluator(CELEvaluatorConfig{AgentSubjectExpression: expression})
			require.Error(t, err)
		})
	}
}

func TestAgentSubjectDetectionTimeout(t *testing.T) {
	e, err := NewCELEvaluator(CELEvaluatorConfig{AgentSubjectExpression: "subject_token.values.all(x, x > 0)", EvaluationTimeout: time.Nanosecond})
	require.NoError(t, err)
	values := make([]int, 100000)
	for i := range values {
		values[i] = 1
	}
	claims := map[string]interface{}{"values": values}
	got, err := e.IsAgentSubject(claims)
	require.False(t, got)
	var exchangeErr *TokenExchangeError
	require.ErrorAs(t, err, &exchangeErr)
	require.Equal(t, DetailCELEvaluationFailed, exchangeErr.Diagnostic().Detail())
}
