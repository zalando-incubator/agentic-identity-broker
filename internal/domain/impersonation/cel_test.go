package impersonation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextAwareProgram struct {
	contextCancelled chan<- struct{}
}

func (p contextAwareProgram) Eval(any) (ref.Val, *cel.EvalDetails, error) {
	return nil, nil, errors.New("Eval must not be called")
}

func (p contextAwareProgram) ContextEval(ctx context.Context, _ any) (ref.Val, *cel.EvalDetails, error) {
	<-ctx.Done()
	p.contextCancelled <- struct{}{}
	return nil, nil, ctx.Err()
}

func (p contextAwareProgram) ConcurrentEval(context.Context, any) <-chan cel.EvalResult {
	panic("ConcurrentEval must not be called")
}

func TestEvaluateWithTimeout_CancelsEvaluation(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	_, err := evaluateWithTimeout(context.Background(), contextAwareProgram{contextCancelled: cancelled}, map[string]interface{}{}, time.Nanosecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-cancelled:
	default:
		t.Fatal("CEL program did not receive cancellation")
	}
}

func TestCompileAuthorization_EvaluatesAndEnumeratesReferences(t *testing.T) {
	prog, err := compileAuthorization(`client_assertion.sub == "gw" && subject_token.sub != ""`, 100*time.Millisecond)
	require.NoError(t, err)

	// Reference enumeration underpins the (gated) unverified subject-binding check (FR-007a).
	assert.True(t, prog.References(celClientAssertion))
	assert.True(t, prog.References(celSubjectToken))
	assert.False(t, prog.References(celActorToken))

	ok, err := prog.evaluate(
		context.Background(),
		map[string]interface{}{"sub": "gw"},
		map[string]interface{}{"sub": "actor"},
		map[string]interface{}{"sub": "user"},
		false,
		requestContext{GrantType: GrantType},
	)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = prog.evaluate(
		context.Background(),
		map[string]interface{}{"sub": "other"},
		map[string]interface{}{"sub": "actor"},
		map[string]interface{}{"sub": "user"},
		false,
		requestContext{},
	)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCompileAuthorization_RejectsEmptyAndNonBool(t *testing.T) {
	_, err := compileAuthorization("", 0)
	require.Error(t, err)

	_, err = compileAuthorization(`client_assertion.sub`, 0)
	require.Error(t, err) // returns a string, not a bool
}

// FR-006a: field-level binding detection distinguishes a predicate that binds subject_token.email
// from one that only references subject_token.
func TestCompileAuthorization_BindsField(t *testing.T) {
	bound, err := compileAuthorization(`subject_token.email != "" && client_assertion.sub == "gw"`, 0)
	require.NoError(t, err)
	assert.True(t, bound.Binds(celSubjectToken, "email"))
	assert.False(t, bound.Binds(celSubjectToken, "sub"))

	unbound, err := compileAuthorization(`subject_token.sub != "" && client_assertion.sub == "gw"`, 0)
	require.NoError(t, err)
	assert.False(t, unbound.Binds(celSubjectToken, "email"))
}

func TestCompileExtraction_ScopesClaimsToRole(t *testing.T) {
	tests := []struct {
		name       string
		role       ports.CredentialRole
		expression string
		crossRole  []string
	}{
		{"client assertion", ports.CredentialRoleClientAssertion, `client_assertion.sub`, []string{`actor_token.sub`, `subject_token.sub`}},
		{"actor token", ports.CredentialRoleActor, `actor_token.sub`, []string{`client_assertion.sub`, `subject_token.sub`}},
		{"subject token", ports.CredentialRoleSubject, `subject_token.sub`, []string{`client_assertion.sub`, `actor_token.sub`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := compileExtraction(tc.role, tc.expression, 0)
			require.NoError(t, err)
			value, err := prog.extract(context.Background(), map[string]interface{}{"sub": "identity"})
			require.NoError(t, err)
			assert.Equal(t, "identity", value)

			for _, expression := range tc.crossRole {
				_, err := compileExtraction(tc.role, expression, 0)
				assert.Error(t, err, expression)
			}
		})
	}
}
