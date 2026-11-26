package opa

import (
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// clockBuiltins are nondeterministic only because they read the clock. Every other built-in OPA
// marks nondeterministic is removed, so a built-in added by an OPA upgrade fails closed.
var clockBuiltins = map[string]bool{"time.now_ns": true, "io.jwt.decode_verify": true}

// restrictedCapabilities returns this OPA version's capabilities without the nondeterministic
// built-ins other than clockBuiltins, and the names it removed.
func restrictedCapabilities() (*ast.Capabilities, map[string]bool) {
	caps := ast.CapabilitiesForThisVersion()
	removed := map[string]bool{}
	kept := make([]*ast.Builtin, 0, len(caps.Builtins))
	for _, builtin := range caps.Builtins {
		if builtin.Nondeterministic && !clockBuiltins[builtin.Name] {
			removed[builtin.Name] = true
			continue
		}
		kept = append(kept, builtin)
	}
	caps.Builtins = kept
	return caps, removed
}

// parseModules parses Rego sources keyed by bundle path as Rego v1.
func parseModules(sources map[string]string) (map[string]*ast.Module, *rejection) {
	modules := make(map[string]*ast.Module, len(sources))
	for path, source := range sources {
		module, err := ast.ParseModuleWithOpts(path, source, ast.ParserOptions{RegoVersion: ast.RegoV1})
		if err != nil {
			return nil, &rejection{reason: reasonCompileFailed, module: path}
		}
		modules[path] = module
	}
	return modules, nil
}

// candidateCompilation is one candidate's broker-owned compiler.
type candidateCompilation struct {
	compiler *ast.Compiler
}

// compileCandidate compiles every module with the restricted capabilities and print statements
// disabled. A call to a removed built-in in any module, reachable or not, is prohibited_builtin.
func compileCandidate(modules map[string]*ast.Module) (*candidateCompilation, *rejection) {
	caps, removed := restrictedCapabilities()
	compiler := ast.NewCompiler().WithCapabilities(caps).WithEnablePrintStatements(false)
	compiler.Compile(modules)
	if compiler.Failed() {
		return nil, classifyCompileErrors(compiler.Errors, removed)
	}
	return &candidateCompilation{compiler: compiler}, nil
}

func classifyCompileErrors(errs ast.Errors, removed map[string]bool) *rejection {
	first := &rejection{reason: reasonCompileFailed}
	for i, err := range errs {
		module := ""
		if err.Location != nil {
			module = err.Location.File
		}
		if i == 0 {
			first.module = module
		}
		name, ok := strings.CutPrefix(err.Message, "undefined function ")
		if ok && removed[name] {
			return &rejection{reason: reasonProhibitedBuiltin, builtin: name, module: module}
		}
	}
	return first
}

// decisionRef is the data reference of a rule's decision.
func decisionRef(ref ports.ImpersonationPolicyDecisionRef) (ast.Ref, error) {
	return ast.ParseRef("data." + ref.Package + "." + ref.Decision)
}

// decisionPresence reports, per rule, whether an exact rule lookup finds the decision. A value
// only in data documents or only rules nested below the reference do not count.
func (c *candidateCompilation) decisionPresence(decisions []ports.ImpersonationPolicyDecisionRef) map[string]bool {
	present := make(map[string]bool, len(decisions))
	for _, decision := range decisions {
		ref, err := decisionRef(decision)
		present[decision.Rule] = err == nil && len(c.compiler.GetRulesExact(ref)) > 0
	}
	return present
}
