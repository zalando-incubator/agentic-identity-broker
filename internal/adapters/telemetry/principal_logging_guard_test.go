package telemetry

import (
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const principalTypePath = "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id.Principal"

// ExtProc must not import internal/domain/id (LOGGING.md §5.1 rule 10); its
// reserved-key strings rely on the policy handler safety net.
var principalGuardExemptPackages = []string{
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc",
	"github.com/agentic-identity-broker/agentic-identity-broker/cmd/extproc-token-exchange",
}

// TestLoggedPrincipalsAreTyped enforces LOGGING.md §5.1 rules 2 and 5: every
// value logged under a reserved principal key through log/slog, as a key-value
// argument or a slog.Attr literal, must be an id.Principal, never slog.String,
// .String(), string(...) or a plain string.
func TestLoggedPrincipalsAreTyped(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the module")
	}
	offenders := findUntypedLoggedPrincipals(t, "./internal/...", "./cmd/...")
	if len(offenders) > 0 {
		t.Fatalf("log principals as id.Principal under reserved keys %v (coding-standards/LOGGING.md §5.1):\n%s",
			sortedReservedKeys(), strings.Join(offenders, "\n"))
	}
}

func TestPrincipalGuardDetectsUntypedPrincipals(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the module")
	}
	offenders := findUntypedLoggedPrincipals(t, "./internal/adapters/telemetry/testdata/principalguard")
	want := []string{
		"slog.String(\"principal\", ...)",
		"p.String()",
		"string(p)",
		"raw",
		"operator",
		"actor",
		"slog.Any(\"principal\", ...)",
		"slog.Attr{Key: \"principal\"} with slog.StringValue(raw)",
		"slog.Attr{Key: \"actor\"} with slog.AnyValue(raw)",
		"slog.Attr{Key: \"operator_principal\"} with p.LogValue()",
	}
	if len(offenders) != len(want) {
		t.Fatalf("expected %d offenders, got %d:\n%s", len(want), len(offenders), strings.Join(offenders, "\n"))
	}
	for i, offender := range offenders {
		if !strings.Contains(offender, want[i]) {
			t.Errorf("offender %d = %q, want it to mention %q", i, offender, want[i])
		}
	}
}

func findUntypedLoggedPrincipals(t *testing.T, patterns ...string) []string {
	t.Helper()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  moduleRoot(t),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages failed to type-check")
	}

	var offenders []string
	for _, pkg := range pkgs {
		if principalGuardExempt(pkg.PkgPath) {
			continue
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if lit, ok := n.(*ast.CompositeLit); ok {
					if offender, found := untypedAttrLiteral(pkg.TypesInfo, lit); found {
						pos := pkg.Fset.Position(lit.Pos())
						offenders = append(offenders, pos.String()+": "+offender)
					}
					return true
				}
				call, ok := n.(*ast.CallExpr)
				if !ok || !callsSlog(pkg.TypesInfo, call) || call.Ellipsis.IsValid() {
					return true
				}
				for i := 0; i+1 < len(call.Args); i++ {
					key, ok := reservedKeyArg(pkg.TypesInfo, call.Args[i])
					if !ok {
						continue
					}
					value := call.Args[i+1]
					if isPrincipalType(pkg.TypesInfo.TypeOf(value)) {
						continue
					}
					pos := pkg.Fset.Position(value.Pos())
					offenders = append(offenders, pos.String()+": "+offenderExpr(call, key, value))
				}
				return true
			})
		}
	}
	return offenders
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func principalGuardExempt(pkgPath string) bool {
	for _, exempt := range principalGuardExemptPackages {
		if pkgPath == exempt || strings.HasPrefix(pkgPath, exempt+"/") {
			return true
		}
	}
	return false
}

func callsSlog(info *types.Info, call *ast.CallExpr) bool {
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		ident = fun.Sel
	case *ast.Ident:
		ident = fun
	default:
		return false
	}
	fn, ok := info.Uses[ident].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "log/slog"
}

// untypedAttrLiteral reports a slog.Attr literal whose reserved key holds a
// value other than slog.AnyValue(id.Principal).
func untypedAttrLiteral(info *types.Info, lit *ast.CompositeLit) (string, bool) {
	typ := info.TypeOf(lit)
	if typ == nil || typ.String() != "log/slog.Attr" {
		return "", false
	}
	var keyExpr, valueExpr ast.Expr
	for i, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			field, _ := kv.Key.(*ast.Ident)
			if field != nil && field.Name == "Key" {
				keyExpr = kv.Value
			}
			if field != nil && field.Name == "Value" {
				valueExpr = kv.Value
			}
			continue
		}
		switch i {
		case 0:
			keyExpr = elt
		case 1:
			valueExpr = elt
		}
	}
	if keyExpr == nil {
		return "", false
	}
	key, ok := reservedKeyArg(info, keyExpr)
	if !ok {
		return "", false
	}
	if valueExpr != nil && isPrincipalAnyValue(info, valueExpr) {
		return "", false
	}
	value := "no value"
	if valueExpr != nil {
		value = types.ExprString(valueExpr)
	}
	return "slog.Attr{Key: \"" + key + "\"} with " + value, true
}

func isPrincipalAnyValue(info *types.Info, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !callsSlog(info, call) {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "AnyValue" && isPrincipalType(info.TypeOf(call.Args[0]))
}

func reservedKeyArg(info *types.Info, arg ast.Expr) (string, bool) {
	tv, ok := info.Types[arg]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	key := constant.StringVal(tv.Value)
	_, reserved := reservedPrincipalKeys[key]
	return key, reserved
}

func isPrincipalType(typ types.Type) bool {
	return typ != nil && typ.String() == principalTypePath
}

func offenderExpr(call *ast.CallExpr, key string, value ast.Expr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "slog" {
			return "slog." + sel.Sel.Name + "(\"" + key + "\", ...) with " + types.ExprString(value)
		}
	}
	return "\"" + key + "\", " + types.ExprString(value)
}

func sortedReservedKeys() []string {
	keys := make([]string, 0, len(reservedPrincipalKeys))
	for key := range reservedPrincipalKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
