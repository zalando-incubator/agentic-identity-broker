package jwtauth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

const (
	// CELClaimsVariable is the name of the CEL variable that holds JWT claims.
	// All CEL expressions receive this variable as map(string, any).
	CELClaimsVariable = "claims"

	// DefaultEvaluationTimeoutMs is the default timeout for CEL expression evaluation.
	DefaultEvaluationTimeoutMs = 100
)

// CELEvaluatorConfig holds configuration for the JWT auth CEL evaluator.
type CELEvaluatorConfig struct {
	// PrincipalExpression is the CEL expression for extracting the principal.
	// Required. Default: "claims.sub"
	PrincipalExpression string

	// DisplayNameExpression is the CEL expression for extracting display name.
	// Optional. Empty string means not configured.
	DisplayNameExpression string

	// EmailExpression is the CEL expression for extracting email.
	// Optional. Empty string means not configured.
	EmailExpression string

	// PictureURLExpression is the CEL expression for extracting picture URL.
	// Optional. Empty string means not configured.
	PictureURLExpression string

	// EvaluationTimeout is the maximum time allowed for CEL expression evaluation.
	// Defaults to 100ms if zero.
	EvaluationTimeout time.Duration
}

// CELEvaluator evaluates CEL expressions to extract principal and optional profile
// attributes from JWT claims. Expressions are compiled at startup (fail-fast on
// syntax errors) and evaluated at runtime with timeout enforcement.
//
// The evaluator provides:
//   - Principal extraction (required, must produce non-empty string)
//   - Display name extraction (optional, nil on failure or non-string result)
//   - Email extraction (optional, nil on failure or non-string result)
//   - Picture URL extraction (optional, nil on failure or non-string result)
//
// This follows the pattern from internal/domain/tokenexchange/cel_evaluator.go (ADR 20260116-120100).
type CELEvaluator struct {
	// principalProgram is the compiled CEL program for principal extraction (required)
	principalProgram cel.Program

	// displayNameProgram is the compiled CEL program for display name extraction (optional)
	displayNameProgram cel.Program

	// emailProgram is the compiled CEL program for email extraction (optional)
	emailProgram cel.Program

	// pictureURLProgram is the compiled CEL program for picture URL extraction (optional)
	pictureURLProgram cel.Program

	// evaluationTimeout is the maximum time allowed for CEL expression evaluation
	evaluationTimeout time.Duration

	// logger for warnings on optional expression evaluation failures
	logger *slog.Logger
}

// NewCELEvaluator creates a new CEL evaluator with compiled expressions.
// Expressions are compiled at startup (fail-fast) to ensure they are valid before
// the application starts accepting requests.
//
// Returns error if any expression has invalid syntax or references undefined variables.
// The principal expression is always compiled. Optional expressions (display name,
// email, picture URL) are only compiled if configured (non-empty).
func NewCELEvaluator(config CELEvaluatorConfig, logger *slog.Logger) (*CELEvaluator, error) {
	if logger == nil {
		logger = slog.Default()
	}

	timeout := config.EvaluationTimeout
	if timeout == 0 {
		timeout = time.Duration(DefaultEvaluationTimeoutMs) * time.Millisecond
	}

	evaluator := &CELEvaluator{
		evaluationTimeout: timeout,
		logger:            logger,
	}

	// Compile principal expression (required)
	principalExpr := config.PrincipalExpression
	if principalExpr == "" {
		principalExpr = "claims.sub"
	}
	program, err := compileClaimsExpression(principalExpr)
	if err != nil {
		return nil, fmt.Errorf("invalid principal_expression %q: %w", principalExpr, err)
	}
	evaluator.principalProgram = program

	// Compile optional display name expression
	if config.DisplayNameExpression != "" {
		program, err := compileClaimsExpression(config.DisplayNameExpression)
		if err != nil {
			return nil, fmt.Errorf("invalid display_name_expression %q: %w", config.DisplayNameExpression, err)
		}
		evaluator.displayNameProgram = program
	}

	// Compile optional email expression
	if config.EmailExpression != "" {
		program, err := compileClaimsExpression(config.EmailExpression)
		if err != nil {
			return nil, fmt.Errorf("invalid email_expression %q: %w", config.EmailExpression, err)
		}
		evaluator.emailProgram = program
	}

	// Compile optional picture URL expression
	if config.PictureURLExpression != "" {
		program, err := compileClaimsExpression(config.PictureURLExpression)
		if err != nil {
			return nil, fmt.Errorf("invalid picture_url_expression %q: %w", config.PictureURLExpression, err)
		}
		evaluator.pictureURLProgram = program
	}

	return evaluator, nil
}

// ExtractClaims extracts principal and optional profile attributes from JWT claims.
// The principal must be a non-empty string; extraction failure is a hard error.
// Optional attributes that fail extraction or evaluate to non-string are returned as nil.
//
// Returns an AuthResult on success, or an error wrapping ErrClaimExtraction on failure.
func (e *CELEvaluator) ExtractClaims(claims map[string]interface{}) (*AuthResult, error) {
	result := &AuthResult{
		Claims: claims,
	}

	// Extract principal (required)
	principal, err := e.evaluateStringExpression(e.principalProgram, claims)
	if err != nil {
		return nil, fmt.Errorf("%w: principal: %v", ErrClaimExtraction, err)
	}
	if principal == "" {
		return nil, fmt.Errorf("%w: principal expression returned empty value", ErrClaimExtraction)
	}
	result.Principal = principal

	// Extract optional display name
	if e.displayNameProgram != nil {
		displayName, err := e.evaluateStringExpression(e.displayNameProgram, claims)
		if err != nil {
			e.logger.Warn("display_name CEL expression evaluation failed, treating as absent",
				"error", err)
		} else if displayName != "" {
			result.DisplayName = &displayName
		}
	}

	// Extract optional email
	if e.emailProgram != nil {
		email, err := e.evaluateStringExpression(e.emailProgram, claims)
		if err != nil {
			e.logger.Warn("email CEL expression evaluation failed, treating as absent",
				"error", err)
		} else if email != "" {
			result.Email = &email
		}
	}

	// Extract optional picture URL
	if e.pictureURLProgram != nil {
		pictureURL, err := e.evaluateStringExpression(e.pictureURLProgram, claims)
		if err != nil {
			e.logger.Warn("picture_url CEL expression evaluation failed, treating as absent",
				"error", err)
		} else if pictureURL != "" {
			result.PictureURL = &pictureURL
		}
	}

	return result, nil
}

// evaluateStringExpression evaluates a compiled CEL program with the given claims
// and returns the result as a string. Returns an error if the result is not a string
// or if evaluation times out.
func (e *CELEvaluator) evaluateStringExpression(program cel.Program, claims map[string]interface{}) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.evaluationTimeout)
	defer cancel()

	variables := map[string]interface{}{
		CELClaimsVariable: claims,
	}

	result, err := evaluateWithTimeout(ctx, program, variables)
	if err != nil {
		return "", fmt.Errorf("evaluation failed: %w", err)
	}

	// Convert result to string
	switch v := result.(type) {
	case string:
		return v, nil
	case types.String:
		return string(v), nil
	default:
		return "", fmt.Errorf("expression returned %T, expected string", result)
	}
}

// compileClaimsExpression compiles a CEL expression that operates on a "claims" variable.
// The claims variable is a map(string, any) representing JWT claims.
func compileClaimsExpression(expr string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Variable(CELClaimsVariable, cel.MapType(cel.StringType, cel.AnyType)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL parse error: %w", issues.Err())
	}

	checked, issues := env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL type check error: %w", issues.Err())
	}

	program, err := env.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("CEL compilation error: %w", err)
	}

	return program, nil
}

// evaluateWithTimeout evaluates a CEL program with timeout enforcement.
func evaluateWithTimeout(
	ctx context.Context,
	program cel.Program,
	variables map[string]interface{},
) (interface{}, error) {
	type evalResult struct {
		value interface{}
		err   error
	}
	resultChan := make(chan evalResult, 1)

	go func() {
		out, _, err := program.Eval(variables)
		resultChan <- evalResult{value: out, err: err}
	}()

	select {
	case result := <-resultChan:
		if result.err != nil {
			return nil, result.err
		}
		return result.value, nil
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("CEL evaluation timeout")
		}
		return nil, fmt.Errorf("CEL evaluation cancelled")
	}
}
