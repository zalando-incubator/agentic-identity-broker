// Package tokenexchange provides domain types and services for RFC 8693 OAuth 2.0 Token Exchange.
package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELEvaluatorConfig holds CEL-specific configuration for expression evaluation.
// This is extracted from ports.TokenExchangeConfig to avoid circular imports.
type CELEvaluatorConfig struct {
	// PrincipalExpression is a CEL expression for extracting the principal from subject_token
	PrincipalExpression string

	// AgentIDExpression is a CEL expression for extracting agent identifier from subject_token
	AgentIDExpression string

	// AuthorizationExpression is a CEL expression for privileged client authorization
	AuthorizationExpression string

	// EvaluationTimeout is the maximum time allowed for CEL expression evaluation
	EvaluationTimeout time.Duration

	// ResolveAgentIDByClientID is an optional function that maps an upstream OAuth2 client_id
	// to the broker's internal agent.id (UUID string).
	// Non-nil only when multi_agent_client.enabled = false.
	// When non-nil, the CEL function resolveAgentIdByClientId() is registered and available
	// in agent_id_expression.
	// When nil (feature enabled), the function is NOT registered.
	// Injected by app/builder.go from AgentRepository.GetByClientID.
	ResolveAgentIDByClientID func(clientID string) (agentID string, err error)
}

// CELEvaluator evaluates CEL expressions for claim extraction and privileged client authorization.
// Expressions are compiled at startup (fail-fast on syntax errors) and evaluated with
// timeouts to prevent blocking token exchange flows.
//
// The evaluator provides methods for:
// - Extracting principal (user identifier) from subject_token using configurable expressions
// - Extracting agent ID from subject_token using configurable expressions
// - Evaluating authorization policies to determine if a privileged client is permitted access
//
// All expression evaluation is sandboxed (no system access) and includes timeout enforcement
// per SC-005 (100ms default).
type CELEvaluator struct {
	// config contains token exchange configuration including CEL expressions
	config CELEvaluatorConfig

	// principalProgram is the compiled CEL program for principal extraction
	// Compiled at startup, evaluated at runtime
	principalProgram cel.Program

	// agentIDProgram is the compiled CEL program for agent ID extraction
	// Compiled at startup, evaluated at runtime
	agentIDProgram cel.Program

	// authorizationProgram is the compiled CEL program for privileged client authorization
	// Compiled at startup, evaluated at runtime
	authorizationProgram cel.Program

	// evaluationTimeout is the maximum time allowed for CEL expression evaluation
	// per SC-005. Defaults to 100ms, configurable via config.Authorization.CEL.EvaluationTimeout
	evaluationTimeout time.Duration
}

// NewCELEvaluator creates a new CEL evaluator with compiled expressions.
// Expressions are compiled at startup (fail-fast) to ensure they are valid before
// the application starts accepting requests.
//
// Returns error if any expression has invalid syntax or references undefined variables.
// Errors are wrapped with ServerError to indicate startup failures.
//
// Per FR-017 and SC-005, CEL expressions are validated at startup and evaluated
// with 100ms timeout enforcement.
func NewCELEvaluator(config CELEvaluatorConfig) (*CELEvaluator, error) {
	evaluator := &CELEvaluator{
		config:            config,
		evaluationTimeout: config.EvaluationTimeout,
	}

	// Default timeout if not configured
	if evaluator.evaluationTimeout == 0 {
		evaluator.evaluationTimeout = time.Duration(DefaultEvaluationTimeoutMs) * time.Millisecond
	}

	// Compile principal extraction expression
	if err := evaluator.compilePrincipalExpression(); err != nil {
		return nil, err
	}

	// Compile agent ID extraction expression
	if err := evaluator.compileAgentIDExpression(); err != nil {
		return nil, err
	}

	// Compile authorization expression
	if err := evaluator.compileAuthorizationExpression(); err != nil {
		return nil, err
	}

	return evaluator, nil
}

// compilePrincipalExpression compiles the principal extraction CEL expression.
// Returns ServerError on compilation failure.
func (e *CELEvaluator) compilePrincipalExpression() error {
	expr := e.config.PrincipalExpression
	if expr == "" {
		expr = DefaultPrincipalExpression
	}

	program, err := e.compileExpression(expr)
	if err != nil {
		return NewServerErrorWithCause("invalid principal extraction CEL expression at startup", err).
			WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration))
	}

	e.principalProgram = program
	return nil
}

// compileAgentIDExpression compiles the agent ID extraction CEL expression.
// Returns ServerError on compilation failure.
func (e *CELEvaluator) compileAgentIDExpression() error {
	expr := e.config.AgentIDExpression
	if expr == "" {
		expr = DefaultAgentIDExpression
	}

	program, err := e.compileExpression(expr)
	if err != nil {
		return NewServerErrorWithCause("invalid agent ID extraction CEL expression at startup", err).
			WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration))
	}

	e.agentIDProgram = program
	return nil
}

// compileAuthorizationExpression compiles the authorization CEL expression.
// Returns ServerError on compilation failure.
func (e *CELEvaluator) compileAuthorizationExpression() error {
	expr := e.config.AuthorizationExpression
	if expr == "" {
		expr = DefaultAuthorizationExpression
	}

	program, err := e.compileExpression(expr)
	if err != nil {
		return NewServerErrorWithCause("invalid authorization CEL expression at startup", err).
			WithDiagnostic(NewDiagnostic(StageClientAuthorization, DetailCELConfiguration))
	}

	e.authorizationProgram = program
	return nil
}

// compileExpression compiles a single CEL expression into a program.
// Returns error if the expression is invalid.
func (e *CELEvaluator) compileExpression(expr string) (cel.Program, error) {
	// Build CEL environment options: base variables + optional resolveAgentIdByClientId function
	envOpts := []cel.EnvOption{
		cel.Variable(CELSubjectTokenVariable, cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("client_assertion", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("request", cel.MapType(cel.StringType, cel.AnyType)),
	}

	// Feature 021 (T031): Register resolveAgentIdByClientId only when the resolver is configured.
	// When nil (multi-agent sharing enabled), the function is not registered and expressions
	// using it fail at compile time, surfacing the misconfiguration immediately.
	if e.config.ResolveAgentIDByClientID != nil {
		resolver := e.config.ResolveAgentIDByClientID
		envOpts = append(envOpts, cel.Function(
			"resolveAgentIdByClientId",
			cel.Overload(
				"resolveAgentIdByClientId_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					clientID, ok := arg.Value().(string)
					if !ok {
						return types.WrapErr(NewServerError("agent resolver argument must be a string").
							WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration)))
					}
					agentID, err := resolver(clientID)
					if err != nil {
						return types.WrapErr(err)
					}
					return types.String(agentID)
				}),
			),
		))
	}

	// Create CEL environment with sandbox restrictions
	env, err := cel.NewEnv(envOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Parse the expression
	ast, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL parse error: %w", issues.Err())
	}

	// Check the expression type
	checked, issues := env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL type check error: %w", issues.Err())
	}

	// Compile the expression
	program, err := env.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("CEL compilation error: %w", err)
	}

	return program, nil
}

// ExtractPrincipal extracts the user principal from subject_token JWT claims.
// Uses the configured principal_expression (default: "subject_token.sub").
//
// Returns the extracted principal string or ServerError on evaluation failure/timeout.
// Per T059, this is used to identify the authenticated user for grant verification.
func (e *CELEvaluator) ExtractPrincipal(subjectTokenClaims map[string]interface{}) (string, error) {
	// Create evaluation context
	ctx, cancel := context.WithTimeout(context.Background(), e.evaluationTimeout)
	defer cancel()

	// Prepare variables for evaluation
	variables := map[string]interface{}{
		CELSubjectTokenVariable: subjectTokenClaims,
	}

	// Evaluate the expression
	result, err := e.evaluateWithTimeout(ctx, e.principalProgram, variables)
	if err != nil {
		return "", celEvaluationError("failed to extract principal from subject_token", StageIdentityResolution, err)
	}

	// Convert result to string
	principal, ok := result.(string)
	if !ok {
		// Try to convert from CEL string type
		if val, ok := result.(types.String); ok {
			principal = string(val)
		} else {
			return "", NewServerError("principal extraction did not return a string").
				WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration))
		}
	}

	if principal == "" {
		return "", NewServerError("principal extraction returned empty value").
			WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration))
	}

	return principal, nil
}

// ExtractAgentID extracts the agent identifier from subject_token JWT claims.
// Uses the configured agent_id_expression (default: "subject_token.azp").
//
// Returns the extracted agent identifier string or ServerError on evaluation failure/timeout.
// Per T060, this is used to look up the agent for grant verification.
func (e *CELEvaluator) ExtractAgentID(subjectTokenClaims map[string]interface{}) (string, error) {
	// Create evaluation context
	ctx, cancel := context.WithTimeout(context.Background(), e.evaluationTimeout)
	defer cancel()

	// Prepare variables for evaluation
	variables := map[string]interface{}{
		CELSubjectTokenVariable: subjectTokenClaims,
	}

	// Evaluate the expression
	result, err := e.evaluateWithTimeout(ctx, e.agentIDProgram, variables)
	if err != nil {
		return "", celEvaluationError("failed to extract agent ID from subject_token", StageIdentityResolution, err)
	}

	// Convert result to string
	agentID, ok := result.(string)
	if !ok {
		// Try to convert from CEL string type
		if val, ok := result.(types.String); ok {
			agentID = string(val)
		} else {
			return "", NewServerError("agent ID extraction did not return a string").
				WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration))
		}
	}

	if agentID == "" {
		return "", NewServerError("agent ID extraction returned empty value").
			WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailCELConfiguration))
	}

	return agentID, nil
}

// AuthorizePrivilegedClient evaluates the authorization expression to determine if a privileged client
// is permitted to perform token exchange.
//
// The expression receives:
// - client_assertion: validated privileged client assertion JWT claims
// - subject_token: validated user subject token JWT claims
// - request: token exchange request context (resource, grant_type, scope)
//
// Returns true if authorization succeeds, false if denied.
// Returns AccessDeniedError if authorization evaluates to false.
// Returns ServerError if evaluation fails or times out (per SC-005 100ms timeout).
//
// Per T073, this implements CEL-based privileged client authorization policies.
func (e *CELEvaluator) AuthorizePrivilegedClient(
	clientAssertionClaims map[string]interface{},
	subjectTokenClaims map[string]interface{},
	request CELRequestContext,
) (bool, error) {
	// Create evaluation context with timeout (per SC-005)
	ctx, cancel := context.WithTimeout(context.Background(), e.evaluationTimeout)
	defer cancel()

	// Prepare variables for evaluation using typed context conversion
	variables := map[string]interface{}{
		"client_assertion":      clientAssertionClaims,
		CELSubjectTokenVariable: subjectTokenClaims,
		"request":               request.ToMap(), // Use typed conversion method
	}

	// Evaluate the expression
	result, err := e.evaluateWithTimeout(ctx, e.authorizationProgram, variables)
	if err != nil {
		return false, celEvaluationError("CEL authorization expression evaluation failed", StageClientAuthorization, err)
	}

	// Convert result to boolean
	authorized, ok := result.(bool)
	if !ok {
		// Try to convert from CEL boolean type
		if val, ok := result.(types.Bool); ok {
			authorized = bool(val)
		} else {
			return false, NewServerError("authorization expression did not return a boolean").
				WithDiagnostic(NewDiagnostic(StageClientAuthorization, DetailCELConfiguration))
		}
	}

	// If authorization fails, return access_denied
	if !authorized {
		return false, NewAccessDeniedError("privileged client authorization denied by CEL policy").
			WithDiagnostic(NewDiagnostic(StageClientAuthorization, DetailClientPolicyDenied))
	}

	return true, nil
}

// celEvaluationError retains resolver-origin diagnostics and all wrapped causes.
// An evaluator-owned deadline is an infrastructure failure, not caller cancellation.
func celEvaluationError(description string, stage FailureStage, cause error) *TokenExchangeError {
	diagnostic := NewDiagnostic(stage, DetailCELEvaluationFailed)
	var origin *TokenExchangeError
	if errors.As(cause, &origin) && origin.Diagnostic().Detail() != DetailInternalUnclassified {
		diagnostic = origin.Diagnostic()
	}
	return NewServerErrorWithCause(description, cause).WithDiagnostic(diagnostic)
}

// evaluateWithTimeout evaluates a CEL program with timeout enforcement.
// Returns ServerError if evaluation times out (per SC-005).
func (e *CELEvaluator) evaluateWithTimeout(
	ctx context.Context,
	program cel.Program,
	variables map[string]interface{},
) (interface{}, error) {
	// Create a channel to receive the result
	type evalResult struct {
		value interface{}
		err   error
	}
	resultChan := make(chan evalResult, 1)

	// Evaluate in a separate goroutine to enforce timeout
	go func() {
		out, _, err := program.Eval(variables)
		resultChan <- evalResult{value: out, err: err}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		if result.err != nil {
			return nil, result.err
		}
		return result.value, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CELRequestContext represents token exchange request context for CEL evaluation.
// This is passed to authorization expressions to make authorization decisions
// based on request parameters.
type CELRequestContext struct {
	// Resource is the target resource URI from the token exchange request
	Resource string

	// GrantType is the grant_type from the token exchange request
	GrantType string

	// Scope is the scope parameter from the token exchange request (optional)
	Scope string

	// Principal is the user principal extracted from subject_token
	Principal string

	// AgentID is the agent identifier extracted from subject_token
	AgentID string
}

// ToMap converts CELRequestContext to a map for CEL evaluation.
// This provides a typed and documented way to prepare request context for CEL expressions.
func (r CELRequestContext) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"resource":   r.Resource,
		"grant_type": r.GrantType,
		"scope":      r.Scope,
		"principal":  r.Principal,
		"agent_id":   r.AgentID,
	}
}
