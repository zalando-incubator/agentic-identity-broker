package tokenexchange

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveHumanSubject(t *testing.T) {
	evaluator, err := NewCELEvaluator(CELEvaluatorConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp"})
	require.NoError(t, err)

	principal, agentID, err := evaluator.ResolveHumanSubject(map[string]interface{}{"sub": "alice", "azp": "agent-1"})
	require.NoError(t, err)
	require.Equal(t, "alice", principal)
	require.Equal(t, "agent-1", agentID)

	principal, agentID, err = evaluator.ResolveHumanSubject(map[string]interface{}{"sub": "agent-1", "agent_id": "agent-1", "azp": "agent-1"})
	var exchangeErr *TokenExchangeError
	require.ErrorAs(t, err, &exchangeErr)
	require.Equal(t, "access_denied", exchangeErr.Code())
	require.Equal(t, DetailSubjectCategoryUnsupported, exchangeErr.Diagnostic().Detail())
	require.ErrorIs(t, err, ErrAgentSubjectToken)
	require.True(t, IsTokenExchangeError(err))
	require.Equal(t, "agent-1", principal, "principal is returned for request security-context capture")
	require.Empty(t, agentID)

	_, _, err = evaluator.ResolveHumanSubject(map[string]interface{}{"azp": "agent-1"})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrAgentSubjectToken)

	failing, err := NewCELEvaluator(CELEvaluatorConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp", AgentSubjectExpression: "subject_token.sub == subject_token.missing"})
	require.NoError(t, err)
	principal, _, err = failing.ResolveHumanSubject(map[string]interface{}{"sub": "alice", "azp": "agent-1"})
	require.ErrorIs(t, err, ErrAgentSubjectDetection)
	require.True(t, IsTokenExchangeError(err))
	require.Equal(t, "server_error", err.(*TokenExchangeError).Code())
	require.Equal(t, "alice", principal)
}
