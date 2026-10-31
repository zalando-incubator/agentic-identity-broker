package tokenexchange

import (
	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"
)

// FieldSelections returns the "identifier.field" selections in a type-checked AST, e.g.
// "subject_token.email". It covers field selection, has() tests, and index access with a
// string literal (subject_token["email"]); only selections directly on a bare identifier count.
func FieldSelections(a *cel.Ast) map[string]struct{} {
	selections := make(map[string]struct{})
	collect := func(e celast.Expr) {
		switch e.Kind() {
		case celast.SelectKind:
			sel := e.AsSelect()
			if operand := sel.Operand(); operand.Kind() == celast.IdentKind {
				selections[operand.AsIdent()+"."+sel.FieldName()] = struct{}{}
			}
		case celast.CallKind:
			call := e.AsCall()
			if call.FunctionName() != operators.Index || len(call.Args()) != 2 {
				return
			}
			operand, key := call.Args()[0], call.Args()[1]
			if operand.Kind() != celast.IdentKind || key.Kind() != celast.LiteralKind {
				return
			}
			if field, ok := key.AsLiteral().(types.String); ok {
				selections[operand.AsIdent()+"."+string(field)] = struct{}{}
			}
		}
	}
	nav := celast.NavigateAST(a.NativeRep())
	collect(nav)
	for _, e := range celast.MatchDescendants(nav, func(e celast.NavigableExpr) bool {
		return e.Kind() == celast.SelectKind || e.Kind() == celast.CallKind
	}) {
		collect(e)
	}
	return selections
}
