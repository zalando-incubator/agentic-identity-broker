package tokenexchange

import (
	"testing"
	"time"

	"github.com/google/cel-go/cel"
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
		{"non-matching replacement", "has(subject_token.sub) && subject_token.sub == 'nobody'", map[string]interface{}{"sub": "agent", "agent_id": "agent"}, false, ""},
		{"external machine", "has(subject_token.sub) && subject_token.kind == 'machine'", map[string]interface{}{"sub": "svc", "kind": "machine"}, true, ""},
		{"dynamic nonboolean", "subject_token.sub", map[string]interface{}{"sub": "agent"}, false, DetailCELConfiguration},
		{"evaluation error", "subject_token.sub == subject_token.missing", map[string]interface{}{"sub": "agent"}, false, DetailCELEvaluationFailed},
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

func TestAgentSubjectDetectionRequiresSubReference(t *testing.T) {
	for _, tc := range []struct {
		expression string
		valid      bool
	}{
		{"", true},
		{DefaultAgentSubjectExpression, true},
		{`subject_token["sub"] == subject_token.agent_id`, true},
		{`subject_token.sub == subject_token.agent_id && subject_token.iss == "https://broker.example.com"`, true},
		{"(" + DefaultAgentSubjectExpression + `) || subject_token.iss == "https://issuer.example.com"`, true},
		{"false", false},
		{"true", false},
		{"subject_token.azp == subject_token.agent_id", false},
		{`subject_token["azp"] == subject_token.agent_id`, false},
		{`{"sub": "x"}.sub == subject_token.agent_id`, false},
	} {
		t.Run(tc.expression, func(t *testing.T) {
			_, err := NewCELEvaluator(CELEvaluatorConfig{AgentSubjectExpression: tc.expression})
			if tc.valid {
				require.NoError(t, err)
				return
			}
			var exchangeErr *TokenExchangeError
			require.ErrorAs(t, err, &exchangeErr)
			require.Equal(t, DetailCELConfiguration, exchangeErr.Diagnostic().Detail())
			require.Contains(t, err.Error(), "subject_token.sub")
		})
	}
}

func TestFieldSelections(t *testing.T) {
	env, err := cel.NewEnv(cel.Variable("subject_token", cel.MapType(cel.StringType, cel.DynType)))
	require.NoError(t, err)
	ast, issues := env.Compile(`has(subject_token.a) && subject_token["b"] == "x" && subject_token.c.d == "y" && {"e": 1}.e == 1`)
	require.NoError(t, issues.Err())
	got := FieldSelections(ast)
	require.Contains(t, got, "subject_token.a")
	require.Contains(t, got, "subject_token.b")
	require.Contains(t, got, "subject_token.c")
	require.NotContains(t, got, "subject_token.d")
	require.NotContains(t, got, "subject_token.e")
}

func TestAgentSubjectDetectionTimeout(t *testing.T) {
	e, err := NewCELEvaluator(CELEvaluatorConfig{AgentSubjectExpression: "has(subject_token.sub) || subject_token.values.all(x, x > 0)", EvaluationTimeout: time.Nanosecond})
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
