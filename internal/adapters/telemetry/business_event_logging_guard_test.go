package telemetry

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const businessEventTypePath = "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/bizevent.Definition"

func TestBusinessEventDefinitionsUseEventKey(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the module")
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  moduleRoot(t),
	}
	pkgs, err := packages.Load(cfg, "./internal/...", "./cmd/...")
	require.NoError(t, err)
	require.Zero(t, packages.PrintErrors(pkgs), "packages failed to type-check")
	var offenders []string
	for _, pkg := range pkgs {
		if principalGuardExempt(pkg.PkgPath) {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, offender := range miskeyedBusinessEvents(pkg.TypesInfo, file) {
				offenders = append(offenders, pkg.Fset.Position(offender.Pos()).String()+": "+types.ExprString(offender))
			}
		}
	}
	assert.Empty(t, offenders, "log bizevent.Definition only under event (coding-standards/LOGGING.md section 2.2):\n%s", strings.Join(offenders, "\n"))
}

func miskeyedBusinessEvents(info *types.Info, file *ast.File) []ast.Expr {
	var offenders []ast.Expr
	checkPairs := func(exprs []ast.Expr) {
		for len(exprs) > 0 {
			key := exprs[0]
			if len(exprs) > 1 && (types.Identical(info.TypeOf(key), types.Typ[types.String]) ||
				types.Identical(info.TypeOf(key), types.Typ[types.UntypedString])) {
				if isBusinessEventType(info.TypeOf(exprs[1])) && !isBusinessEventKey(info, key) {
					offenders = append(offenders, exprs[1])
				}
				exprs = exprs[2:]
				continue
			}
			if isBusinessEventType(info.TypeOf(key)) {
				offenders = append(offenders, key)
			}
			exprs = exprs[1:]
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if isAppendToAnySlice(info, n) {
				// An append can start with the value of a key in the destination slice.
				for i, arg := range n.Args[1:] {
					if isBusinessEventType(info.TypeOf(arg)) && (i == 0 || !isBusinessEventKey(info, n.Args[i])) {
						offenders = append(offenders, arg)
					}
				}
			} else if callsSlog(info, n) && !n.Ellipsis.IsValid() {
				if function, ok := n.Fun.(*ast.SelectorExpr); ok && function.Sel.Name == "AnyValue" {
					break
				}
				args := n.Args
				if sig, ok := info.TypeOf(n.Fun).(*types.Signature); ok && sig.Variadic() {
					fixed := sig.Params().Len() - 1
					if len(args) < fixed {
						break // One tuple expression supplies multiple parameters.
					}
					args = args[fixed:]
				}
				checkPairs(args)
			}
		case *ast.CompositeLit:
			if isAnySlice(info.TypeOf(n)) {
				checkPairs(n.Elts)
			}
			if typ := info.TypeOf(n); typ == nil || typ.String() != "log/slog.Attr" {
				break
			}
			var key, value ast.Expr
			for i, element := range n.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					field, _ := pair.Key.(*ast.Ident)
					if field != nil && field.Name == "Key" {
						key = pair.Value
					}
					if field != nil && field.Name == "Value" {
						value = pair.Value
					}
				} else if i == 0 {
					key = element
				} else if i == 1 {
					value = element
				}
			}
			call, ok := value.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || !callsSlog(info, call) {
				break
			}
			if isBusinessEventType(info.TypeOf(call.Args[0])) && !isBusinessEventKey(info, key) {
				offenders = append(offenders, n)
			}
		}
		return true
	})
	return offenders
}

func isBusinessEventKey(info *types.Info, expr ast.Expr) bool {
	value := info.Types[expr].Value
	return value != nil && value.Kind() == constant.String && constant.StringVal(value) == "event"
}

func isBusinessEventType(typ types.Type) bool {
	return typ != nil && types.Unalias(typ).String() == businessEventTypePath
}

type businessEventGuardImporter map[string]*types.Package

func (imports businessEventGuardImporter) Import(path string) (*types.Package, error) {
	return imports[path], nil
}

func TestBusinessEventGuardDetectsWrongKeys(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: moduleRoot(t)}
	pkgs, err := packages.Load(cfg, "log/slog", "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/bizevent")
	require.NoError(t, err)
	require.Zero(t, packages.PrintErrors(pkgs))
	imports := businessEventGuardImporter{}
	for _, pkg := range pkgs {
		imports[pkg.PkgPath] = pkg.Types
	}
	cases := []struct {
		name, statement string
		want            int
	}{
		{"key value", `logger.InfoContext(nil, "record", "action", definition)`, 1},
		{"correct key", `logger.InfoContext(nil, "record", "event", definition)`, 0},
		{"message is not a key", `logger.InfoContext(nil, "event", definition)`, 1},
		{"attribute value is not a key", `logger.InfoContext(nil, "record", "action", "event", definition)`, 1},
		{"group name is not a key", `_ = slog.Group("event", definition)`, 1},
		{"constant key", `const key = "event"; logger.InfoContext(nil, "record", key, definition)`, 0},
		{"dynamic key", `key := "event"; logger.InfoContext(nil, "record", key, definition)`, 1},
		{"typed attr", `_ = slog.Any("action", definition)`, 1},
		{"correct attr", `_ = slog.Any("event", definition)`, 0},
		{"with", `_ = logger.With("action", definition)`, 1},
		{"group", `_ = slog.Group("details", "action", definition)`, 1},
		{"attr literal", `_ = slog.Attr{Key: "action", Value: slog.AnyValue(definition)}`, 1},
		{"positional literal", `_ = slog.Attr{"action", slog.AnyValue(definition)}`, 1},
		{"correct literal", `_ = slog.Attr{Key: "event", Value: slog.AnyValue(definition)}`, 0},
		{"slice", `args := []any{"action", definition}; logger.InfoContext(nil, "record", args...)`, 1},
		{"append", `args := []any{}; args = append(args, "action", definition); logger.InfoContext(nil, "record", args...)`, 1},
		{"correct append", `args := []any{}; args = append(args, "event", definition); logger.InfoContext(nil, "record", args...)`, 0},
		{"split append", `args := []any{"action"}; args = append(args, "a value", "event", definition); logger.InfoContext(nil, "record", args...)`, 0},
		{"operational string", `logger.InfoContext(nil, "record", "action", "operational.event.recorded")`, 0},
		{"nonlog definition", `_ = definition`, 0},
		{"alias", `type Alias = bizevent.Definition; var alias Alias; logger.InfoContext(nil, "record", "action", alias)`, 1},
		{"different typed value", `logger.InfoContext(nil, "record", "action", 42)`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := `package fixture
import (
 "log/slog"
 "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/bizevent"
)
func fixture(logger *slog.Logger, definition bizevent.Definition) { ` + tc.statement + ` }
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture.go", source, 0)
			require.NoError(t, err)
			info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Uses: make(map[*ast.Ident]types.Object)}
			checker := types.Config{Importer: imports}
			_, err = checker.Check("fixture", fset, []*ast.File{file}, info)
			require.NoError(t, err)
			assert.Len(t, miskeyedBusinessEvents(info, file), tc.want)
		})
	}
}
