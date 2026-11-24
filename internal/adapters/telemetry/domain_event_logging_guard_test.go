package telemetry

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const domainEventTypePath = "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/domainevent.Definition"

func TestDomainEventDefinitionsUseEventKey(t *testing.T) {
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
			for _, offender := range miskeyedDomainEvents(pkg.TypesInfo, file) {
				offenders = append(offenders, pkg.Fset.Position(offender.Pos()).String()+": "+types.ExprString(offender))
			}
		}
	}
	assert.Empty(t, offenders, "log domainevent.Definition only under event (coding-standards/LOGGING.md section 2.2):\n%s", strings.Join(offenders, "\n"))
}

func miskeyedDomainEvents(info *types.Info, file *ast.File) []ast.Expr {
	initializers, appendArgs := domainEventGuardBindings(info, file)
	var offenders []ast.Expr
	checkPairs := func(exprs []ast.Expr, after token.Pos) {
		for len(exprs) > 0 {
			key := exprs[0]
			if len(exprs) > 1 && (types.Identical(info.TypeOf(key), types.Typ[types.String]) ||
				types.Identical(info.TypeOf(key), types.Typ[types.UntypedString])) {
				if exprs[1].Pos() > after && isDomainEventType(info.TypeOf(exprs[1])) && !isDomainEventKey(info, key) {
					offenders = append(offenders, exprs[1])
				}
				exprs = exprs[2:]
				continue
			}
			if key.Pos() > after && isDomainEventType(info.TypeOf(key)) {
				offenders = append(offenders, key)
			}
			exprs = exprs[1:]
		}
	}
	ast.PreorderStack(file, nil, func(node ast.Node, stack []ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if isAppendToAnySlice(info, n) {
				if args, known := appendArgs[n]; known {
					checkPairs(args, n.Args[0].End())
					break
				}
				// An append can start with the value of a key in the destination slice.
				for i, arg := range n.Args[1:] {
					if isDomainEventType(info.TypeOf(arg)) && (i == 0 || !isDomainEventKey(info, n.Args[i])) {
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
				checkPairs(args, token.NoPos)
			}
		case *ast.CompositeLit:
			if isAnySlice(info.TypeOf(n)) {
				checkPairs(n.Elts, token.NoPos)
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
			if name, ok := value.(*ast.Ident); ok {
				obj := info.ObjectOf(name)
				value = initializers[obj].valueAt(obj, name.Pos(), stack)
			}
			call, ok := value.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || !callsSlog(info, call) {
				break
			}
			if isDomainEventType(info.TypeOf(call.Args[0])) && !isDomainEventKey(info, key) {
				offenders = append(offenders, n)
			}
		}
		return true
	})
	return offenders
}

type domainEventGuardBinding struct {
	value        ast.Expr
	reassignedAt token.Pos
}

func (b domainEventGuardBinding) valueAt(obj types.Object, pos token.Pos, stack []ast.Node) ast.Expr {
	if b.value == nil || b.value.End() > pos {
		return nil
	}
	if !b.reassignedAt.IsValid() {
		return b.value
	}
	if pos >= b.reassignedAt {
		return nil
	}
	for _, node := range slices.Backward(stack) {
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			// Source order does not establish when a closure reads a captured variable.
			if obj.Pos() >= node.Pos() {
				return b.value
			}
			return nil
		}
	}
	return nil
}

func domainEventGuardBindings(info *types.Info, file *ast.File) (map[types.Object]domainEventGuardBinding, map[*ast.CallExpr][]ast.Expr) {
	initializers := make(map[types.Object]domainEventGuardBinding)
	appendArgs := make(map[*ast.CallExpr][]ast.Expr)
	bind := func(name ast.Expr, value ast.Expr, end token.Pos) {
		ident, ok := name.(*ast.Ident)
		if !ok {
			return
		}
		obj := info.ObjectOf(ident)
		binding, assigned := initializers[obj]
		if !assigned {
			initializers[obj] = domainEventGuardBinding{value: value}
		} else if !binding.reassignedAt.IsValid() {
			// A reassignment ends the known binding after its RHS has been evaluated.
			binding.reassignedAt = end
			initializers[obj] = binding
		}
	}
	for node := range ast.Preorder(file) {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for i, name := range n.Lhs {
				var value ast.Expr
				if len(n.Lhs) == len(n.Rhs) {
					value = n.Rhs[i]
				}
				bind(name, value, n.End())
			}
		case *ast.ValueSpec:
			if len(n.Names) == len(n.Values) {
				for i, value := range n.Values {
					bind(n.Names[i], value, n.End())
				}
			}
		case *ast.BlockStmt:
			collectDomainEventAppendArgs(info, n, appendArgs)
		}
	}
	return initializers, appendArgs
}

func collectDomainEventAppendArgs(info *types.Info, block *ast.BlockStmt, appendArgs map[*ast.CallExpr][]ast.Expr) {
	var name types.Object
	var args []ast.Expr
	// Track consecutive assignments to a fresh argument slice, not aliases or control flow.
	for _, stmt := range block.List {
		lhs, rhs := domainEventSliceBinding(stmt)
		if lhs == nil || !isAnySlice(info.TypeOf(lhs)) {
			name = nil
			continue
		}
		obj := info.ObjectOf(lhs)
		switch rhs := ast.Unparen(rhs).(type) {
		case nil:
			name, args = obj, nil
			continue
		case *ast.CompositeLit:
			if isAnySlice(info.TypeOf(rhs)) {
				name, args = obj, rhs.Elts
				continue
			}
		case *ast.CallExpr:
			if name != nil && obj == name && isAppendToAnySlice(info, rhs) && !rhs.Ellipsis.IsValid() {
				if dst, ok := rhs.Args[0].(*ast.Ident); ok && info.ObjectOf(dst) == name {
					args = append(args, rhs.Args[1:]...)
					appendArgs[rhs] = args
					continue
				}
			}
		}
		name = nil
	}
}

func domainEventSliceBinding(stmt ast.Stmt) (*ast.Ident, ast.Expr) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		if len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
			name, _ := stmt.Lhs[0].(*ast.Ident)
			return name, stmt.Rhs[0]
		}
	case *ast.DeclStmt:
		decl, ok := stmt.Decl.(*ast.GenDecl)
		if !ok || decl.Tok != token.VAR || len(decl.Specs) != 1 {
			break
		}
		spec := decl.Specs[0].(*ast.ValueSpec)
		if len(spec.Names) == 1 {
			if len(spec.Values) == 1 {
				return spec.Names[0], spec.Values[0]
			}
			return spec.Names[0], nil
		}
	}
	return nil, nil
}

func isDomainEventKey(info *types.Info, expr ast.Expr) bool {
	value := info.Types[expr].Value
	return value != nil && value.Kind() == constant.String && constant.StringVal(value) == "event"
}

func isDomainEventType(typ types.Type) bool {
	return typ != nil && types.Unalias(typ).String() == domainEventTypePath
}

type domainEventGuardImporter map[string]*types.Package

func (imports domainEventGuardImporter) Import(path string) (*types.Package, error) {
	return imports[path], nil
}

func TestDomainEventGuardDetectsWrongKeys(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: moduleRoot(t)}
	pkgs, err := packages.Load(cfg, "log/slog", "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/domainevent")
	require.NoError(t, err)
	require.Zero(t, packages.PrintErrors(pkgs))
	imports := domainEventGuardImporter{}
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
		{"local attr value", `value := slog.AnyValue(definition); _ = slog.Attr{Key: "action", Value: value}`, 1},
		{"correct local attr value", `value := slog.AnyValue(definition); _ = slog.Attr{Key: "event", Value: value}`, 0},
		{"assigned local attr value", `var value slog.Value; value = slog.AnyValue(definition); _ = slog.Attr{Key: "action", Value: value}`, 1},
		{"attr before local assignment", `var value slog.Value; _ = slog.Attr{Key: "action", Value: value}; value = slog.AnyValue(definition)`, 0},
		{"attr before reassignment", `value := slog.AnyValue(definition); _ = slog.Attr{Key: "action", Value: value}; value = slog.StringValue("ordinary")`, 1},
		{"attr after reassignment", `value := slog.AnyValue(definition); value = slog.StringValue("ordinary"); _ = slog.Attr{Key: "action", Value: value}`, 0},
		{"captured attr after reassignment", `
			value := slog.AnyValue(definition)
			attr := func() slog.Attr { return slog.Attr{Key: "action", Value: value} }
			value = slog.StringValue("ordinary")
			_ = attr()
		`, 0},
		{"captured attr without reassignment", `
			value := slog.AnyValue(definition)
			attr := func() slog.Attr { return slog.Attr{Key: "action", Value: value} }
			_ = attr()
		`, 1},
		{"closure-local attr before reassignment", `
			func() {
				value := slog.AnyValue(definition)
				_ = slog.Attr{Key: "action", Value: value}
				value = slog.StringValue("ordinary")
			}()
		`, 1},
		{"correct literal", `_ = slog.Attr{Key: "event", Value: slog.AnyValue(definition)}`, 0},
		{"slice", `args := []any{"action", definition}; logger.InfoContext(nil, "record", args...)`, 1},
		{"append", `args := []any{}; args = append(args, "action", definition); logger.InfoContext(nil, "record", args...)`, 1},
		{"correct append", `args := []any{}; args = append(args, "event", definition); logger.InfoContext(nil, "record", args...)`, 0},
		{"split append", `args := []any{"action"}; args = append(args, "a value", "event", definition); logger.InfoContext(nil, "record", args...)`, 0},
		{"operational string", `logger.InfoContext(nil, "record", "action", "operational.event.recorded")`, 0},
		{"appended attribute value is not a key", `args := []any{}; args = append(args, "action", "event", definition); logger.InfoContext(nil, "record", args...)`, 1},
		{"append after var declaration", `var args = []any{"action"}; args = append(args, "event", definition); logger.InfoContext(nil, "record", args...)`, 1},
		{"valid append after var declaration", `var args = []any{"event"}; args = append(args, definition); logger.InfoContext(nil, "record", args...)`, 0},
		{"append completes event pair", `args := []any{}; args = append(args, "event"); args = append(args, definition); logger.InfoContext(nil, "record", args...)`, 0},
		{"nonlog definition", `_ = definition`, 0},
		{"alias", `type Alias = domainevent.Definition; var alias Alias; logger.InfoContext(nil, "record", "action", alias)`, 1},
		{"different typed value", `logger.InfoContext(nil, "record", "action", 42)`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := `package fixture
import (
 "log/slog"
 "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/domainevent"
)
func fixture(logger *slog.Logger, definition domainevent.Definition) { ` + tc.statement + ` }
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture.go", source, 0)
			require.NoError(t, err)
			info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object)}
			checker := types.Config{Importer: imports}
			_, err = checker.Check("fixture", fset, []*ast.File{file}, info)
			require.NoError(t, err)
			assert.Len(t, miskeyedDomainEvents(info, file), tc.want)
		})
	}
}
