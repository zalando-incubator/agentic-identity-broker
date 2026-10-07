package tokenexchange

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCELEvaluatorSuccessfulCompilation tests successful evaluator creation.
func TestNewCELEvaluatorSuccessfulCompilation(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)

	evaluator, err := NewCELEvaluator(config)

	require.NoError(t, err)
	assert.NotNil(t, evaluator)
	assert.NotNil(t, evaluator.principalProgram)
	assert.NotNil(t, evaluator.agentIDProgram)
	assert.NotNil(t, evaluator.authorizationProgram)
}

// TestNewCELEvaluatorInvalidPrincipalExpression tests startup failure with invalid principal expression.
func TestNewCELEvaluatorInvalidPrincipalExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.PrincipalExpression = "invalid syntax !@#$"

	evaluator, err := NewCELEvaluator(config)

	assert.Nil(t, evaluator)
	assert.Error(t, err)
	assert.True(t, IsTokenExchangeError(err))
	tokExErr := err.(*TokenExchangeError)
	assert.Equal(t, "server_error", tokExErr.Code())
	assert.Equal(t, 500, tokExErr.HTTPStatus())
}

// TestNewCELEvaluatorInvalidAgentIDExpression tests startup failure with invalid agent_id expression.
func TestNewCELEvaluatorInvalidAgentIDExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AgentIDExpression = "invalid !@#$ syntax"

	evaluator, err := NewCELEvaluator(config)

	assert.Nil(t, evaluator)
	assert.Error(t, err)
	assert.True(t, IsTokenExchangeError(err))
}

// TestNewCELEvaluatorInvalidAuthorizationExpression tests startup failure with invalid authorization expression.
func TestNewCELEvaluatorInvalidAuthorizationExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = "not a boolean !@#$"

	evaluator, err := NewCELEvaluator(config)

	assert.Nil(t, evaluator)
	assert.Error(t, err)
	assert.True(t, IsTokenExchangeError(err))
}

// TestExtractPrincipalDefaultExpression tests principal extraction with default expression.
func TestExtractPrincipalDefaultExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"sub": "user123",
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	require.NoError(t, err)
	assert.Equal(t, "user123", principal)
}

// TestExtractPrincipalCustomExpression tests principal extraction with custom expression.
func TestExtractPrincipalCustomExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.PrincipalExpression = "subject_token.preferred_username"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"sub":                "user123",
		"preferred_username": "john.doe",
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	require.NoError(t, err)
	assert.Equal(t, "john.doe", principal)
}

// TestExtractPrincipalNestedClaim tests principal extraction from nested claim.
func TestExtractPrincipalNestedClaim(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.PrincipalExpression = "subject_token.claims.email"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"claims": map[string]any{
			"email": "user@example.com",
		},
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", principal)
}

// TestExtractPrincipalMissingClaim tests error when claim is missing.
func TestExtractPrincipalMissingClaim(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		// Missing 'sub' claim
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	assert.Error(t, err)
	assert.Empty(t, principal)
	assert.True(t, IsTokenExchangeError(err))
}

// TestExtractPrincipalEmptyValue tests error when extracted value is empty.
func TestExtractPrincipalEmptyValue(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"sub": "",
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	assert.Error(t, err)
	assert.Empty(t, principal)
	assert.True(t, IsTokenExchangeError(err))
}

// TestExtractPrincipalWrongType tests error when extracted value is not a string.
func TestExtractPrincipalWrongType(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.PrincipalExpression = "subject_token.exp"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"exp": int64(1234567890),
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	assert.Error(t, err)
	assert.Empty(t, principal)
	assert.True(t, IsTokenExchangeError(err))
}

// TestExtractAgentIDDefaultExpression tests agent_id extraction with default expression.
func TestExtractAgentIDDefaultExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"azp": "agent-app-1",
	}

	agentID, err := evaluator.ExtractAgentID(claims)

	require.NoError(t, err)
	assert.Equal(t, "agent-app-1", agentID)
}

// TestExtractAgentIDCustomExpression tests agent_id extraction with custom expression.
func TestExtractAgentIDCustomExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AgentIDExpression = "subject_token.client_id"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"client_id": "app-agent",
	}

	agentID, err := evaluator.ExtractAgentID(claims)

	require.NoError(t, err)
	assert.Equal(t, "app-agent", agentID)
}

// TestExtractAgentIDMissingClaim tests error when claim is missing.
func TestExtractAgentIDMissingClaim(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		// Missing 'azp' claim
	}

	agentID, err := evaluator.ExtractAgentID(claims)

	assert.Error(t, err)
	assert.Empty(t, agentID)
	assert.True(t, IsTokenExchangeError(err))
}

// TestAuthorizePrivilegedClientDefaultExpression tests privileged client authorization with default allow-all expression.
func TestAuthorizePrivilegedClientDefaultExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "privileged-client-1",
		"iss": "https://auth.example.com",
	}
	subjectToken := map[string]any{
		"sub": "user123",
	}
	request := CELRequestContext{
		Resource:  "https://api.example.com",
		GrantType: TokenExchangeGrantType,
		Scope:     "read",
		Principal: "user123",
		AgentID:   "agent-app-1",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	require.NoError(t, err)
	assert.True(t, authorized)
}

// TestAuthorizePrivilegedClientCustomExpressionAllow tests privileged client authorization with custom allow expression.
func TestAuthorizePrivilegedClientCustomExpressionAllow(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = `client_assertion.iss == "https://trusted.example.com"`
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "privileged-client-1",
		"iss": "https://trusted.example.com",
	}
	subjectToken := map[string]any{
		"sub": "user123",
	}
	request := CELRequestContext{
		Resource:  "https://api.example.com",
		GrantType: TokenExchangeGrantType,
		Principal: "user123",
		AgentID:   "agent-app-1",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	require.NoError(t, err)
	assert.True(t, authorized)
}

// TestAuthorizePrivilegedClientCustomExpressionDeny tests privileged client authorization with custom deny expression.
func TestAuthorizePrivilegedClientCustomExpressionDeny(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = `client_assertion.iss == "https://trusted.example.com"`
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "privileged-client-1",
		"iss": "https://untrusted.example.com", // Doesn't match trusted issuer
	}
	subjectToken := map[string]any{
		"sub": "user123",
	}
	request := CELRequestContext{
		Resource:  "https://api.example.com",
		GrantType: TokenExchangeGrantType,
		Principal: "user123",
		AgentID:   "agent-app-1",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	assert.Error(t, err)
	assert.False(t, authorized)
	assert.True(t, IsTokenExchangeError(err))
	tokExErr := err.(*TokenExchangeError)
	assert.Equal(t, "access_denied", tokExErr.Code())
}

// TestAuthorizePrivilegedClientComplexExpression tests authorization with complex multi-condition expression.
func TestAuthorizePrivilegedClientComplexExpression(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = `
		client_assertion.sub == "trusted-privileged-client" &&
		"admin" in subject_token.roles
	`
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "trusted-privileged-client",
	}
	subjectToken := map[string]any{
		"sub":   "user123",
		"roles": []any{"admin", "user"},
	}
	request := CELRequestContext{
		Resource:  "https://api.example.com",
		GrantType: TokenExchangeGrantType,
		Principal: "user123",
		AgentID:   "trusted-privileged-client",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	require.NoError(t, err)
	assert.True(t, authorized)
}

// TestAuthorizePrivilegedClientComplexExpressionFailsAdminCheck tests complex expression with failed admin check.
func TestAuthorizePrivilegedClientComplexExpressionFailsAdminCheck(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = `
		client_assertion.sub == "trusted-privileged-client" &&
		"admin" in subject_token.roles
	`
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "trusted-privileged-client",
	}
	subjectToken := map[string]any{
		"sub":   "user123",
		"roles": []any{"user"}, // Missing admin role
	}
	request := CELRequestContext{
		Resource:  "https://api.example.com",
		GrantType: TokenExchangeGrantType,
		Principal: "user123",
		AgentID:   "trusted-privileged-client",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	assert.Error(t, err)
	assert.False(t, authorized)
	assert.True(t, IsTokenExchangeError(err))
}

func TestCELEvaluatorFailureDiagnostics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		configure func(*CELEvaluatorConfig)
		evaluate  func(*CELEvaluator) error
		outcome   Outcome
		stage     FailureStage
		detail    FailureDetail
	}{
		{"principal syntax", func(c *CELEvaluatorConfig) { c.PrincipalExpression = "secret-expression-credential !" }, nil, OutcomeConfigurationError, StageIdentityResolution, DetailCELConfiguration},
		{"agent syntax", func(c *CELEvaluatorConfig) { c.AgentIDExpression = "secret-expression-credential !" }, nil, OutcomeConfigurationError, StageIdentityResolution, DetailCELConfiguration},
		{"authorization syntax", func(c *CELEvaluatorConfig) { c.AuthorizationExpression = "secret-expression-credential !" }, nil, OutcomeConfigurationError, StageClientAuthorization, DetailCELConfiguration},
		{"compile type", func(c *CELEvaluatorConfig) { c.AuthorizationExpression = "1 + true" }, nil, OutcomeConfigurationError, StageClientAuthorization, DetailCELConfiguration},
		{"principal result type", func(c *CELEvaluatorConfig) { c.PrincipalExpression = "subject_token.secret_claim" }, func(e *CELEvaluator) error {
			_, err := e.ExtractPrincipal(map[string]any{"secret_claim": int64(42)})
			return err
		}, OutcomeConfigurationError, StageIdentityResolution, DetailCELConfiguration},
		{"agent result type", func(c *CELEvaluatorConfig) { c.AgentIDExpression = "subject_token.secret_claim" }, func(e *CELEvaluator) error {
			_, err := e.ExtractAgentID(map[string]any{"secret_claim": int64(42)})
			return err
		}, OutcomeConfigurationError, StageIdentityResolution, DetailCELConfiguration},
		{"authorization result type", func(c *CELEvaluatorConfig) { c.AuthorizationExpression = "client_assertion.secret_claim" }, func(e *CELEvaluator) error {
			_, err := e.AuthorizePrivilegedClient(map[string]any{"secret_claim": "secret-claim-value"}, nil, CELRequestContext{})
			return err
		}, OutcomeConfigurationError, StageClientAuthorization, DetailCELConfiguration},
		{"empty principal", nil, func(e *CELEvaluator) error { _, err := e.ExtractPrincipal(map[string]any{"sub": ""}); return err }, OutcomeConfigurationError, StageIdentityResolution, DetailCELConfiguration},
		{"empty agent", nil, func(e *CELEvaluator) error { _, err := e.ExtractAgentID(map[string]any{"azp": ""}); return err }, OutcomeConfigurationError, StageIdentityResolution, DetailCELConfiguration},
		{"principal evaluation", func(c *CELEvaluatorConfig) { c.PrincipalExpression = "subject_token['secret-claim-key']" }, func(e *CELEvaluator) error { _, err := e.ExtractPrincipal(nil); return err }, OutcomeInfrastructureError, StageIdentityResolution, DetailCELEvaluationFailed},
		{"agent evaluation", func(c *CELEvaluatorConfig) { c.AgentIDExpression = "subject_token['secret-claim-key']" }, func(e *CELEvaluator) error { _, err := e.ExtractAgentID(nil); return err }, OutcomeInfrastructureError, StageIdentityResolution, DetailCELEvaluationFailed},
		{"authorization evaluation", func(c *CELEvaluatorConfig) {
			c.AuthorizationExpression = "client_assertion['secret-claim-key'] == 'secret-claim-value'"
		}, func(e *CELEvaluator) error {
			_, err := e.AuthorizePrivilegedClient(nil, nil, CELRequestContext{Resource: "https://secret-resource.example/private?credential=secret-url-value"})
			return err
		}, OutcomeInfrastructureError, StageClientAuthorization, DetailCELEvaluationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := validTokenExchangeConfig(t)
			if tc.configure != nil {
				tc.configure(&config)
			}
			evaluator, err := NewCELEvaluator(config)
			if tc.evaluate != nil {
				require.NoError(t, err)
				err = tc.evaluate(evaluator)
			} else {
				assert.Nil(t, evaluator)
			}
			tokenErr := assertOriginDiagnostic(t, err, "server_error", tc.outcome, tc.stage, tc.detail)
			if tc.evaluate == nil || tc.detail == DetailCELEvaluationFailed {
				assert.NotNil(t, tokenErr.Unwrap(), "compile and evaluation failures preserve their causes")
			}
			for _, secret := range []string{"secret-expression-credential", "secret-claim-key", "secret-claim-value", "secret-resource", "secret-url-value"} {
				assert.NotContains(t, fmt.Sprint(tokenErr, tokenErr.Description(), tokenErr.Details(), tokenErr.Diagnostic()), secret)
			}
		})
	}
}

func TestCELEvaluatorFalseIsPolicyDenial(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = "false"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)
	authorized, err := evaluator.AuthorizePrivilegedClient(nil, nil, CELRequestContext{})
	assert.False(t, authorized)
	tokenErr := assertOriginDiagnostic(t, err, "access_denied", OutcomeAuthorizationDenied, StageClientAuthorization, DetailClientPolicyDenied)
	assert.Equal(t, RecoveryNone, tokenErr.Diagnostic().RecoveryAction())
	assert.Equal(t, TargetNone, tokenErr.Diagnostic().RecoveryTarget())
}

func TestCELEvaluatorResolverPreservesCauses(t *testing.T) {
	t.Parallel()
	secretCause := &originTestError{value: "https://secret-resolver.example/private?credential=secret-resolver-token"}
	for _, tc := range []struct {
		name    string
		origin  error
		outcome Outcome
		detail  FailureDetail
	}{
		{"agent missing", NewServerErrorWithCause("agent lookup failed", secretCause).WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailAgentMissing)), OutcomeConfigurationError, DetailAgentMissing},
		{"agent invalid", NewServerErrorWithCause("agent configuration failed", secretCause).WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailAgentInvalid)), OutcomeConfigurationError, DetailAgentInvalid},
		{"repository unavailable", NewServerErrorWithCause("agent repository failed", secretCause).WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailAgentRepositoryUnavailable)), OutcomeInfrastructureError, DetailAgentRepositoryUnavailable},
		{"detached deadline", context.DeadlineExceeded, OutcomeInfrastructureError, DetailCELEvaluationFailed},
		{"unclassified dependency", secretCause, OutcomeInfrastructureError, DetailCELEvaluationFailed},
		{"unclassified typed dependency", NewServerErrorWithCause("unclassified resolver failure", secretCause), OutcomeInfrastructureError, DetailCELEvaluationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := validTokenExchangeConfig(t)
			config.AgentIDExpression = "resolveAgentIdByClientId(subject_token.azp)"
			config.ResolveAgentIDByClientID = func(string) (string, error) { return "", tc.origin }
			evaluator, err := NewCELEvaluator(config)
			require.NoError(t, err)
			_, err = evaluator.ExtractAgentID(map[string]any{"azp": "secret-client-id"})
			tokenErr := assertOriginDiagnostic(t, err, "server_error", tc.outcome, StageIdentityResolution, tc.detail)
			assert.ErrorIs(t, err, tc.origin)
			if tc.origin != context.DeadlineExceeded {
				var dependencyErr *originTestError
				require.ErrorAs(t, err, &dependencyErr)
				assert.Same(t, secretCause, dependencyErr)
			}
			for _, secret := range []string{"secret-resolver", "secret-client-id"} {
				assert.NotContains(t, fmt.Sprint(tokenErr, tokenErr.Description(), tokenErr.Details(), tokenErr.Diagnostic()), secret)
			}
		})
	}
}

func TestCELEvaluatorTimeoutIsInfrastructure(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	config := validTokenExchangeConfig(t)
	config.EvaluationTimeout = time.Second
	config.AuthorizationExpression = "resolveAgentIdByClientId(client_assertion.sub) == 'agent'"
	config.ResolveAgentIDByClientID = func(string) (string, error) {
		close(started)
		<-release
		close(finished)
		return "agent", nil
	}
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, err := evaluator.AuthorizePrivilegedClient(map[string]any{"sub": "secret-client-id"}, nil, CELRequestContext{})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("CEL resolver was not reached")
	}
	t.Cleanup(func() {
		close(release)
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("CEL resolver did not stop after release")
		}
	})
	select {
	case err := <-result:
		tokenErr := assertOriginDiagnostic(t, err, "server_error", OutcomeInfrastructureError, StageClientAuthorization, DetailCELEvaluationFailed)
		assert.NotContains(t, tokenErr.Error(), "secret-client-id")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("CEL evaluation did not return at its configured deadline")
	}
}

// TestExtractPrincipalWithComplexNestedClaims tests principal extraction from complex nested structure.
func TestExtractPrincipalWithComplexNestedClaims(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.PrincipalExpression = "subject_token.user.id"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"user": map[string]any{
			"id":   "user-456",
			"name": "John Doe",
		},
	}

	principal, err := evaluator.ExtractPrincipal(claims)

	require.NoError(t, err)
	assert.Equal(t, "user-456", principal)
}

// TestExtractAgentIDWithComplexNestedClaims tests agent_id extraction from complex nested structure.
func TestExtractAgentIDWithComplexNestedClaims(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AgentIDExpression = "subject_token.agent.client_id"
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	claims := map[string]any{
		"agent": map[string]any{
			"client_id": "agent-789",
			"name":      "Privileged Client Agent",
		},
	}

	agentID, err := evaluator.ExtractAgentID(claims)

	require.NoError(t, err)
	assert.Equal(t, "agent-789", agentID)
}

// TestAuthorizePrivilegedClientWithRequestContext tests authorization with request context variables.
func TestAuthorizePrivilegedClientWithRequestContext(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = `request.resource == "https://api.example.com"`
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "privileged-client-1",
	}
	subjectToken := map[string]any{
		"sub": "user123",
	}
	request := CELRequestContext{
		Resource:  "https://api.example.com",
		GrantType: TokenExchangeGrantType,
		Principal: "user123",
		AgentID:   "agent-app-1",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	require.NoError(t, err)
	assert.True(t, authorized)
}

// TestAuthorizePrivilegedClientWithRequestContextMismatch tests authorization with mismatched request context.
func TestAuthorizePrivilegedClientWithRequestContextMismatch(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.AuthorizationExpression = `request.resource == "https://api.example.com"`
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	clientAssertion := map[string]any{
		"sub": "privileged-client-1",
	}
	subjectToken := map[string]any{
		"sub": "user123",
	}
	request := CELRequestContext{
		Resource:  "https://other-api.example.com", // Different resource
		GrantType: TokenExchangeGrantType,
		Principal: "user123",
		AgentID:   "agent-app-1",
	}

	authorized, err := evaluator.AuthorizePrivilegedClient(clientAssertion, subjectToken, request)

	assert.Error(t, err)
	assert.False(t, authorized)
	assert.True(t, IsTokenExchangeError(err))
}

// TestNewCELEvaluatorWithDefaultTimeout tests evaluator initialization with default timeout.
func TestNewCELEvaluatorWithDefaultTimeout(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.EvaluationTimeout = 0 // Force default
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	assert.Equal(t, time.Duration(DefaultEvaluationTimeoutMs)*time.Millisecond, evaluator.evaluationTimeout)
}

// TestNewCELEvaluatorWithCustomTimeout tests evaluator initialization with custom timeout.
func TestNewCELEvaluatorWithCustomTimeout(t *testing.T) {
	t.Parallel()
	config := validTokenExchangeConfig(t)
	config.EvaluationTimeout = 250 * time.Millisecond
	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err)

	assert.Equal(t, 250*time.Millisecond, evaluator.evaluationTimeout)
}

func TestExtractAgentIDComposeHybridExpression(t *testing.T) {
	const (
		localAgentID  = "0a771543-779d-4be8-9967-2895f55f4ee1"
		upstreamAgent = "d7e68b7a-d7ec-46ce-84d2-8b708ed6eb4c"
	)

	tests := []struct {
		name             string
		claims           map[string]interface{}
		wantAgentID      string
		wantResolverCall bool
	}{
		{
			name: "local token uses its broker-issued agent ID",
			claims: map[string]interface{}{
				"agent_id": localAgentID,
			},
			wantAgentID: localAgentID,
		},
		{
			name: "upstream token resolves its authorized party",
			claims: map[string]interface{}{
				"azp": "upstream-client",
			},
			wantAgentID:      upstreamAgent,
			wantResolverCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolverCalls := 0
			config := validTokenExchangeConfig(t)
			config.AgentIDExpression = "has(subject_token.agent_id) ? subject_token.agent_id : resolveAgentIdByClientId(subject_token.azp)"
			config.ResolveAgentIDByClientID = func(clientID string) (string, error) {
				resolverCalls++
				if clientID != "upstream-client" {
					return "", fmt.Errorf("unknown client_id: %s", clientID)
				}
				return upstreamAgent, nil
			}
			evaluator, err := NewCELEvaluator(config)
			require.NoError(t, err)

			agentID, err := evaluator.ExtractAgentID(tt.claims)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAgentID, agentID)
			assert.Equal(t, tt.wantResolverCall, resolverCalls == 1)
		})
	}
}

// Helper function to create a valid CELEvaluatorConfig for testing
func validTokenExchangeConfig(_ *testing.T) CELEvaluatorConfig {
	return CELEvaluatorConfig{
		PrincipalExpression:     DefaultPrincipalExpression,
		AgentIDExpression:       DefaultAgentIDExpression,
		AuthorizationExpression: DefaultAuthorizationExpression,
		EvaluationTimeout:       time.Duration(DefaultEvaluationTimeoutMs) * time.Millisecond,
	}
}

// --- T028: resolveAgentIdByClientId CEL function tests (Feature 021 US2) ---

// TestCELEvaluator_ResolveAgentIdByClientId_CorrectResult verifies that when
// ResolveAgentIDByClientID is non-nil, the resolveAgentIdByClientId CEL function is
// registered and returns the agent UUID string returned by the resolver.
// [T028] Written before T031 implementation — must FAIL until T031 registers the function.
func TestCELEvaluator_ResolveAgentIdByClientId_CorrectResult(t *testing.T) {
	agentUUID := "550e8400-e29b-41d4-a716-446655440000"

	config := validTokenExchangeConfig(t)
	// Register a mock resolver that maps "upstream-client-1" → agentUUID
	config.ResolveAgentIDByClientID = func(clientID string) (string, error) {
		if clientID == "upstream-client-1" {
			return agentUUID, nil
		}
		return "", fmt.Errorf("unknown client_id: %s", clientID)
	}
	// Use the resolveAgentIdByClientId function in the expression
	config.AgentIDExpression = "resolveAgentIdByClientId(subject_token.azp)"

	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err, "NewCELEvaluator should succeed when resolver is registered")
	require.NotNil(t, evaluator)

	result, err := evaluator.ExtractAgentID(map[string]interface{}{
		"azp": "upstream-client-1",
	})
	require.NoError(t, err)
	assert.Equal(t, agentUUID, result)
}

// TestCELEvaluator_ResolveAgentIdByClientId_UnknownClientIDReturnsErr verifies that when
// the resolver returns an error (unknown clientID), ExtractAgentID returns an error.
// [T028] Written before T031 implementation — must FAIL until T031 registers the function.
func TestCELEvaluator_ResolveAgentIdByClientId_UnknownClientIDReturnsErr(t *testing.T) {
	config := validTokenExchangeConfig(t)
	config.ResolveAgentIDByClientID = func(clientID string) (string, error) {
		return "", fmt.Errorf("unknown client_id: %s", clientID)
	}
	config.AgentIDExpression = "resolveAgentIdByClientId(subject_token.azp)"

	evaluator, err := NewCELEvaluator(config)
	require.NoError(t, err, "NewCELEvaluator should succeed when resolver is registered")

	_, err = evaluator.ExtractAgentID(map[string]interface{}{
		"azp": "unknown-client",
	})
	assert.Error(t, err, "ExtractAgentID should return error when resolver fails")
}

// TestCELEvaluator_ResolveAgentIdByClientId_NotRegisteredWhenNil verifies that when
// ResolveAgentIDByClientID is nil, an expression using resolveAgentIdByClientId fails at
// compile time with an undeclared reference error (feature disabled — function absent).
// [T028] This test guards the contract that nil resolver → function NOT registered.
// It passes both before and after T031 (the function is only registered when non-nil).
func TestCELEvaluator_ResolveAgentIdByClientId_NotRegisteredWhenNil(t *testing.T) {
	config := validTokenExchangeConfig(t)
	config.ResolveAgentIDByClientID = nil // Feature disabled
	config.AgentIDExpression = "resolveAgentIdByClientId(subject_token.azp)"

	evaluator, err := NewCELEvaluator(config)
	assert.Error(t, err, "NewCELEvaluator should fail when nil resolver and expression uses resolveAgentIdByClientId")
	assert.Nil(t, evaluator)
	assert.True(t, IsTokenExchangeError(err), "error should be a TokenExchangeError")
}
