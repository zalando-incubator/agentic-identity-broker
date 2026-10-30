package impersonation

import (
	"context"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
)

// CEL variable names available to impersonation authorization and extraction expressions
// (FR-007). subject_unverified is true only for the unsigned unverified subject path (FR-003d,
// ADR 20260825-120000) and false for a signed subject.
const (
	celClientAssertion   = "client_assertion"
	celActorToken        = "actor_token"
	celSubjectToken      = "subject_token"
	celSubjectUnverified = "subject_unverified"
	celRequest           = "request"
)

// defaultEvaluationTimeout is applied when a rule's authorization timeout is unset (CR-005).
const defaultEvaluationTimeout = 100 * time.Millisecond

// newImpersonationCELEnv builds the CEL environment shared by authorization and extraction
// expressions. It declares exactly the five impersonation variables (FR-007).
func newImpersonationCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable(celClientAssertion, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(celActorToken, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(celSubjectToken, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(celSubjectUnverified, cel.BoolType),
		cel.Variable(celRequest, cel.MapType(cel.StringType, cel.DynType)),
	)
}

// newExtractionCELEnv builds a CEL environment for one credential role. Extraction expressions
// receive only the validated claims for that role.
func newExtractionCELEnv(role ports.CredentialRole) (*cel.Env, error) {
	var variable string
	switch role {
	case ports.CredentialRoleClientAssertion:
		variable = celClientAssertion
	case ports.CredentialRoleActor:
		variable = celActorToken
	case ports.CredentialRoleSubject:
		variable = celSubjectToken
	default:
		return nil, fmt.Errorf("unknown credential role %q", role)
	}
	return cel.NewEnv(cel.Variable(variable, cel.MapType(cel.StringType, cel.DynType)))
}

// requestContext carries the impersonation request parameters exposed to CEL as `request`.
type requestContext struct {
	Resource  string
	GrantType string
	Scope     string
	AgentID   string
}

func (r requestContext) toMap() map[string]interface{} {
	return map[string]interface{}{
		"resource":   r.Resource,
		"grant_type": r.GrantType,
		"scope":      r.Scope,
		"agent_id":   r.AgentID,
	}
}

// authorizationProgram is a compiled rule authorization predicate. It retains the set of variable
// names the expression references (so the builder can enforce subject binding for rules that accept
// the unverified subject mode, FR-007a) and the set of "identifier.field" selections (so the service
// can enforce that a caller-controlled unverified-subject email is only minted when the predicate
// binds subject_token.email, FR-006a).
type authorizationProgram struct {
	program    cel.Program
	references map[string]struct{}
	binds      map[string]struct{}
	timeout    time.Duration
}

// compileAuthorization compiles a rule's authorization predicate at startup (fail-fast, CR-005).
// The expression MUST be non-empty and type-check to a boolean.
func compileAuthorization(expression string, timeout time.Duration) (*authorizationProgram, error) {
	if expression == "" {
		return nil, fmt.Errorf("authorization expression is required")
	}
	if timeout <= 0 {
		timeout = defaultEvaluationTimeout
	}
	env, err := newImpersonationCELEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("invalid authorization expression: %w", issues.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("authorization expression must return bool, got %s", ast.OutputType())
	}
	program, err := env.Program(ast, cel.InterruptCheckFrequency(1))
	if err != nil {
		return nil, fmt.Errorf("failed to compile authorization expression: %w", err)
	}
	return &authorizationProgram{
		program:    program,
		references: referencedVariables(ast),
		binds:      fieldSelections(ast),
		timeout:    timeout,
	}, nil
}

// References reports whether the compiled predicate references the given CEL variable name.
// Used by the builder to verify subject binding for unverified-subject rules (FR-007a, CR-005a).
func (p *authorizationProgram) References(variable string) bool {
	_, ok := p.references[variable]
	return ok
}

// Binds reports whether the compiled predicate selects the given field on the given variable, e.g.
// Binds("subject_token", "email"). It underpins the caller-controlled email minting control (FR-006a).
func (p *authorizationProgram) Binds(variable, field string) bool {
	_, ok := p.binds[variable+"."+field]
	return ok
}

// evaluate runs the predicate against the credential claims. It fails closed on evaluation
// error or timeout (FR-007).
func (p *authorizationProgram) evaluate(
	ctx context.Context,
	clientAssertion, actorToken, subjectToken map[string]interface{},
	subjectUnverified bool,
	request requestContext,
) (bool, error) {
	activation := map[string]interface{}{
		celClientAssertion:   orEmptyMap(clientAssertion),
		celActorToken:        orEmptyMap(actorToken),
		celSubjectToken:      orEmptyMap(subjectToken),
		celSubjectUnverified: subjectUnverified,
		celRequest:           request.toMap(),
	}
	out, err := evaluateWithTimeout(ctx, p.program, activation, p.timeout)
	if err != nil {
		return false, err
	}
	switch v := out.(type) {
	case bool:
		return v, nil
	case types.Bool:
		return bool(v), nil
	default:
		return false, fmt.Errorf("authorization expression did not return a bool")
	}
}

// extractionProgram is a compiled per-role identity/email extraction expression (FR-006).
type extractionProgram struct {
	program cel.Program
	varName string
	timeout time.Duration
}

// compileExtraction compiles a per-role extraction expression at startup. The expression MUST
// be non-empty and may reference only the validated claims for its role.
func compileExtraction(role ports.CredentialRole, expression string, timeout time.Duration) (*extractionProgram, error) {
	if expression == "" {
		return nil, fmt.Errorf("extraction expression is required")
	}
	if timeout <= 0 {
		timeout = defaultEvaluationTimeout
	}
	env, err := newExtractionCELEnv(role)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("invalid extraction expression: %w", issues.Err())
	}
	program, err := env.Program(ast, cel.InterruptCheckFrequency(1))
	if err != nil {
		return nil, fmt.Errorf("failed to compile extraction expression: %w", err)
	}
	return &extractionProgram{program: program, varName: extractionVariable(role), timeout: timeout}, nil
}

func extractionVariable(role ports.CredentialRole) string {
	switch role {
	case ports.CredentialRoleClientAssertion:
		return celClientAssertion
	case ports.CredentialRoleActor:
		return celActorToken
	case ports.CredentialRoleSubject:
		return celSubjectToken
	default:
		return ""
	}
}

// extract evaluates the expression and returns the resulting string (possibly empty). Only the
// validated claims for the expression's credential role are available. Non-string results and
// evaluation failures are errors (fail closed).
func (p *extractionProgram) extract(ctx context.Context, claims map[string]interface{}) (string, error) {
	activation := map[string]interface{}{p.varName: orEmptyMap(claims)}
	out, err := evaluateWithTimeout(ctx, p.program, activation, p.timeout)
	if err != nil {
		return "", err
	}
	switch v := out.(type) {
	case string:
		return v, nil
	case types.String:
		return string(v), nil
	default:
		return "", fmt.Errorf("extraction expression did not return a string")
	}
}

// evaluateWithTimeout runs a CEL program with a cancellation-aware deadline. CEL interruption
// is configured at compilation, so the evaluation stops when the request context is cancelled.
func evaluateWithTimeout(ctx context.Context, program cel.Program, activation map[string]interface{}, timeout time.Duration) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, _, err := program.ContextEval(ctx, activation)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return out.Value(), nil
}

// referencedVariables returns the set of identifier names referenced by a type-checked AST.
func referencedVariables(ast *cel.Ast) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, info := range ast.NativeRep().ReferenceMap() {
		if info != nil && info.Name != "" {
			refs[info.Name] = struct{}{}
		}
	}
	return refs
}

// fieldSelections returns the set of "identifier.field" selections in a type-checked AST, e.g.
// "subject_token.email". It walks the navigable AST for field-selection nodes whose operand is a
// bare identifier so the service can enforce the FR-006a unverified-subject email-binding control.
func fieldSelections(a *cel.Ast) map[string]struct{} {
	sels := make(map[string]struct{})
	collect := func(e celast.Expr) {
		if e.Kind() != celast.SelectKind {
			return
		}
		sel := e.AsSelect()
		if operand := sel.Operand(); operand.Kind() == celast.IdentKind {
			sels[operand.AsIdent()+"."+sel.FieldName()] = struct{}{}
		}
	}
	nav := celast.NavigateAST(a.NativeRep())
	collect(nav)
	for _, e := range celast.MatchDescendants(nav, celast.KindMatcher(celast.SelectKind)) {
		collect(e)
	}
	return sels
}

func orEmptyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}
