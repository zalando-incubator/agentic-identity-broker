package telemetrylint

import (
	"go/ast"
	"go/constant"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

const (
	pkgOtel      = "go.opentelemetry.io/otel"
	pkgTrace     = "go.opentelemetry.io/otel/trace"
	pkgMetric    = "go.opentelemetry.io/otel/metric"
	pkgAttribute = "go.opentelemetry.io/otel/attribute"
	pkgSlog      = "log/slog"
	pkgFmt       = "fmt"
)

const doc = `check the OpenTelemetry and logging coding standards

telemetrylint enforces the rules of coding-standards/OPENTELEMETRY.md and
coding-standards/LOGGING.md that need type information:

  scope            otel.Tracer and otel.Meter take a constant scope from the section 2 allowlist
  span-name        span names are constants matching <scope>.<operation>[.<entity>]
  span-kind        each span start sets trace.WithSpanKind
  metric-name      instrument names are constants matching <scope>.<noun>[.<qualifier>]
  metric-metadata  each instrument sets metric.WithDescription and metric.WithUnit
  attribute-key    attribute keys are constants
  principal-key    keys ending in .principal take only id.Principal.SpanAttribute values
  principal-sink   an id.Principal never reaches metric attributes, span names, span events,
                   span status descriptions, or fmt.Errorf
  credential       credential types are never log or span attribute values

Test files are not checked. A span helper that forwards its name or options parameter to
Tracer.Start in the same package is checked at each of its call sites instead.`

var (
	segment        = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	telemetryName  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){1,2}$`)
	instrumentName = regexp.MustCompile(`^(Int64|Float64)(Counter|UpDownCounter|Histogram|Gauge|ObservableCounter|ObservableUpDownCounter|ObservableGauge)$`)
	attributeFuncs = map[string]bool{
		"Bool": true, "BoolSlice": true, "Int": true, "IntSlice": true, "Int64": true, "Int64Slice": true,
		"Float64": true, "Float64Slice": true, "String": true, "StringSlice": true, "Stringer": true,
	}
)

// NewAnalyzer returns the telemetrylint analyzer for the given settings.
func NewAnalyzer(s Settings) (*analysis.Analyzer, error) {
	cfg, err := newConfig(s)
	if err != nil {
		return nil, err
	}
	return &analysis.Analyzer{
		Name: "telemetrylint",
		Doc:  doc,
		Run: func(pass *analysis.Pass) (any, error) {
			newChecker(pass, cfg).run()
			return nil, nil
		},
	}, nil
}

// spanHelper describes a function that forwards its parameters to Tracer.Start. A field is the
// index of the forwarded parameter, or -1 when the helper sets that part itself.
type spanHelper struct {
	name  int
	opts  int
	scope string
}

type checker struct {
	pass    *analysis.Pass
	cfg     *config
	info    *types.Info
	helpers map[*types.Func]spanHelper
	pkgVars map[*types.Var]ast.Expr
	report  bool
	changed bool
}

// funcCtx is the innermost function around a call.
type funcCtx struct {
	sig  *types.Signature
	fn   *types.Func
	body ast.Node
}

func newChecker(pass *analysis.Pass, cfg *config) *checker {
	return &checker{
		pass:    pass,
		cfg:     cfg,
		info:    pass.TypesInfo,
		helpers: map[*types.Func]spanHelper{},
		pkgVars: map[*types.Var]ast.Expr{},
	}
}

func (c *checker) run() {
	var files []*ast.File
	for _, f := range c.pass.Files {
		name := c.pass.Fset.File(f.Pos()).Name()
		if strings.HasSuffix(name, "_test.go") || ast.IsGenerated(f) {
			continue
		}
		files = append(files, f)
		c.collectPackageVars(f)
	}

	// Find span helpers first, so that a call site checked before its helper's declaration is
	// still checked. A helper can call another helper, so iterate until nothing changes.
	for range 10 {
		c.changed = false
		for _, f := range files {
			c.walkFile(f)
		}
		if !c.changed {
			break
		}
	}
	c.report = true
	for _, f := range files {
		c.walkFile(f)
	}
}

func (c *checker) collectPackageVars(f *ast.File) {
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				if v, ok := c.info.Defs[name].(*types.Var); ok {
					c.pkgVars[v] = vs.Values[i]
				}
			}
		}
	}
}

func (c *checker) walkFile(f *ast.File) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil {
				continue
			}
			fn, _ := c.info.Defs[d.Name].(*types.Func)
			var sig *types.Signature
			if fn != nil {
				sig, _ = fn.Type().(*types.Signature)
			}
			c.walk(d.Body, &funcCtx{sig: sig, fn: fn, body: d.Body})
		case *ast.GenDecl:
			c.walk(d, &funcCtx{})
		}
	}
}

func (c *checker) walk(node ast.Node, ctx *funcCtx) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			sig, _ := c.info.TypeOf(n).(*types.Signature)
			c.walk(n.Body, &funcCtx{sig: sig, body: n.Body})
			return false
		case *ast.CallExpr:
			c.checkCall(ctx, n)
		}
		return true
	})
}

func (c *checker) reportf(node ast.Node, format string, args ...any) {
	if c.report {
		c.pass.Reportf(node.Pos(), format, args...)
	}
}

func (c *checker) checkCall(ctx *funcCtx, call *ast.CallExpr) {
	if tv, ok := c.info.Types[call.Fun]; ok && tv.IsType() {
		if isNamed(tv.Type, pkgAttribute, "Key") && len(call.Args) == 1 {
			c.checkAttributeKey(ctx, call.Args[0])
		}
		return
	}
	fn, _ := typeutil.Callee(c.info, call).(*types.Func)
	if fn == nil {
		return
	}
	if h, ok := c.helpers[fn]; ok {
		c.checkSpanStart(ctx, call, h.name, h.opts, h.scope)
	}
	if fn.Pkg() == nil {
		return
	}

	switch {
	case isFunc(fn, pkgOtel, "Tracer"), isFunc(fn, pkgOtel, "Meter"),
		isMethod(fn, pkgTrace, "TracerProvider", "Tracer"), isMethod(fn, pkgMetric, "MeterProvider", "Meter"):
		if len(call.Args) > 0 {
			c.checkScope(call.Args[0])
		}
	case isMethod(fn, pkgTrace, "Tracer", "Start"):
		c.checkSpanStart(ctx, call, 1, 2, c.resolveScope(ctx, receiver(call)))
	case isMethod(fn, pkgTrace, "Span", "SetName"):
		if len(call.Args) == 1 {
			c.checkSpanName(call.Args[0], "")
		}
	case isMethod(fn, pkgTrace, "Span", "AddEvent"):
		if len(call.Args) > 0 && c.derives(call.Args[0], c.cfg.principal) {
			c.reportf(call.Args[0], "principal-sink: a principal must not be in a span event (OPENTELEMETRY.md section 4.3)")
		}
	case isMethod(fn, pkgTrace, "Span", "SetStatus"):
		if len(call.Args) > 1 && c.derives(call.Args[1], c.cfg.principal) {
			c.reportf(call.Args[1], "principal-sink: a principal must not be in a span status description (OPENTELEMETRY.md section 4.3)")
		}
	case isMethod(fn, pkgMetric, "Meter", "") && instrumentName.MatchString(fn.Name()):
		c.checkInstrument(ctx, call)
	case isFunc(fn, pkgMetric, "WithAttributes"):
		c.checkMetricAttributes(call.Args)
	case isFunc(fn, pkgMetric, "WithAttributeSet"):
		for _, arg := range call.Args {
			if set, ok := ast.Unparen(arg).(*ast.CallExpr); ok {
				if f, _ := typeutil.Callee(c.info, set).(*types.Func); f != nil && isFunc(f, pkgAttribute, "NewSet") {
					c.checkMetricAttributes(set.Args)
				}
			}
		}
	case isFunc(fn, pkgAttribute, "") && attributeFuncs[fn.Name()]:
		if len(call.Args) == 2 {
			c.checkAttributeKey(ctx, call.Args[0])
			c.checkCredential(ctx, call.Args[1])
		}
	case isMethod(fn, pkgAttribute, "Key", ""):
		// A conversion receiver, attribute.Key("..."), is checked as a conversion.
		_, converted := ast.Unparen(receiver(call)).(*ast.CallExpr)
		if key, ok := c.constString(receiver(call)); ok && !converted && isPrincipalKey(key) && !c.mayBuildPrincipalAttribute(ctx) {
			c.reportPrincipalKey(call, key)
		}
		for _, arg := range call.Args {
			c.checkCredential(ctx, arg)
		}
	case c.isPrincipalMethod(fn, "SpanAttribute"):
		if len(call.Args) == 1 {
			if key, ok := c.constString(call.Args[0]); !ok || !isPrincipalKey(key) {
				c.reportf(call.Args[0], "principal-key: the SpanAttribute key must be a constant that ends in .principal (OPENTELEMETRY.md section 4.3)")
			}
		}
	case isFunc(fn, pkgFmt, "Errorf"):
		for _, arg := range call.Args[min(1, len(call.Args)):] {
			if c.derives(arg, c.cfg.principal) {
				c.reportf(arg, "principal-sink: a principal must not be in an error string (LOGGING.md section 4)")
			}
		}
	case fn.Pkg().Path() == pkgSlog:
		for _, arg := range call.Args {
			c.checkCredential(ctx, arg)
		}
	}
}

func (c *checker) checkScope(arg ast.Expr) {
	scope, ok := c.constString(arg)
	switch {
	case !ok:
		c.reportf(arg, "scope: the instrumentation scope must be a constant string (OPENTELEMETRY.md section 2)")
	case !c.cfg.scopes[scope]:
		c.reportf(arg, "scope: instrumentation scope %q is not in the allowlist of OPENTELEMETRY.md section 2", scope)
	}
}

// checkSpanStart checks a call to Tracer.Start or to a span helper. nameIdx and optsIdx are the
// argument indices of the span name and of the first span start option, or -1 when the callee
// sets that part itself.
func (c *checker) checkSpanStart(ctx *funcCtx, call *ast.CallExpr, nameIdx, optsIdx int, scope string) {
	if nameIdx >= 0 && nameIdx < len(call.Args) {
		arg := call.Args[nameIdx]
		if param := c.paramIndex(ctx, arg); param >= 0 && ctx.fn != nil {
			c.markHelper(ctx.fn, func(h *spanHelper) { h.name, h.scope = param, scope })
		} else {
			c.checkSpanName(arg, scope)
		}
	}
	if optsIdx < 0 {
		return
	}
	if c.optionsHave(ctx, call, optsIdx, func(f *types.Func) bool { return isFunc(f, pkgTrace, "WithSpanKind") }) {
		return
	}
	if call.Ellipsis.IsValid() && len(call.Args) > optsIdx && ctx.fn != nil && ctx.sig.Variadic() {
		if param := c.paramIndex(ctx, call.Args[len(call.Args)-1]); param == ctx.sig.Params().Len()-1 {
			c.markHelper(ctx.fn, func(h *spanHelper) { h.opts = param })
			return
		}
	}
	c.reportf(call, "span-kind: the span start does not set trace.WithSpanKind (OPENTELEMETRY.md section 3.3)")
}

func (c *checker) markHelper(fn *types.Func, update func(*spanHelper)) {
	h, ok := c.helpers[fn]
	if !ok {
		h = spanHelper{name: -1, opts: -1}
	}
	before := h
	update(&h)
	if !ok || h != before {
		c.helpers[fn] = h
		c.changed = true
	}
}

func (c *checker) checkSpanName(arg ast.Expr, scope string) {
	if c.derives(arg, c.cfg.principal) {
		c.reportf(arg, "principal-sink: a principal must not be in a span name (OPENTELEMETRY.md section 4.3)")
		return
	}
	name, ok := c.constString(arg)
	if !ok {
		c.reportf(arg, "span-name: the span name must be a constant string (OPENTELEMETRY.md section 3.2)")
		return
	}
	c.checkName(arg, "span-name", "span name", "<scope>.<operation>[.<entity>]", "tracer", name, scope)
}

func (c *checker) checkInstrument(ctx *funcCtx, call *ast.CallExpr) {
	if len(call.Args) == 0 {
		return
	}
	nameArg := call.Args[0]
	name, ok := c.constString(nameArg)
	if !ok {
		c.reportf(nameArg, "metric-name: the metric name must be a constant string (OPENTELEMETRY.md section 5.2)")
	} else {
		c.checkName(nameArg, "metric-name", "metric name", "<scope>.<noun>[.<qualifier>]", "meter", name, c.resolveScope(ctx, receiver(call)))
	}
	label := "the instrument"
	if ok {
		label = "instrument " + strconv.Quote(name)
	}
	if !c.optionsHave(ctx, call, 1, func(f *types.Func) bool { return isFunc(f, pkgMetric, "WithDescription") }) {
		c.reportf(call, "metric-metadata: %s does not set metric.WithDescription (OPENTELEMETRY.md section 5.1)", label)
	}
	if !c.optionsHave(ctx, call, 1, func(f *types.Func) bool { return isFunc(f, pkgMetric, "WithUnit") }) {
		c.reportf(call, "metric-metadata: %s does not set metric.WithUnit (OPENTELEMETRY.md section 5.1)", label)
	}
}

func (c *checker) checkName(node ast.Node, rule, what, grammar, provider, name, scope string) {
	if !telemetryName.MatchString(name) {
		c.reportf(node, "%s: %s %q does not match %s with lowercase snake_case segments", rule, what, name, grammar)
		return
	}
	first := name[:strings.IndexByte(name, '.')]
	switch {
	case !c.cfg.scopes[first]:
		c.reportf(node, "%s: %s %q does not start with a scope from OPENTELEMETRY.md section 2", rule, what, name)
	case scope != "" && first != scope:
		c.reportf(node, "%s: %s %q does not start with the %s scope %q", rule, what, name, provider, scope)
	}
}

func (c *checker) checkAttributeKey(ctx *funcCtx, arg ast.Expr) {
	if c.inPrincipalSpanAttribute(ctx) {
		return
	}
	key, ok := c.constString(arg)
	if !ok {
		c.reportf(arg, "attribute-key: the attribute key must be a constant string (OPENTELEMETRY.md section 4.1)")
		return
	}
	if isPrincipalKey(key) && !c.mayBuildPrincipalAttribute(ctx) {
		c.reportPrincipalKey(arg, key)
	}
}

func (c *checker) reportPrincipalKey(node ast.Node, key string) {
	c.reportf(node, "principal-key: key %q is reserved for principals; set it only with id.Principal.SpanAttribute (OPENTELEMETRY.md section 4.3)", key)
}

// mayBuildPrincipalAttribute reports whether code in this context may construct a principal
// attribute itself: the SpanAttribute method, and the packages exempted by configuration.
func (c *checker) mayBuildPrincipalAttribute(ctx *funcCtx) bool {
	if c.inPrincipalSpanAttribute(ctx) {
		return true
	}
	for _, re := range c.cfg.principalExempt {
		if re.MatchString(c.pass.Pkg.Path()) {
			return true
		}
	}
	return false
}

func (c *checker) inPrincipalSpanAttribute(ctx *funcCtx) bool {
	return ctx.fn != nil && c.isPrincipalMethod(ctx.fn, "SpanAttribute")
}

func (c *checker) checkMetricAttributes(args []ast.Expr) {
	for _, arg := range args {
		if c.attributeCarriesPrincipal(arg) {
			c.reportf(arg, "principal-sink: a principal must not be a metric attribute (OPENTELEMETRY.md section 5.3)")
		}
	}
}

func (c *checker) attributeCarriesPrincipal(expr ast.Expr) bool {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, _ := typeutil.Callee(c.info, call).(*types.Func)
	switch {
	case fn == nil:
		return false
	case c.isPrincipalMethod(fn, "SpanAttribute"):
		return true
	case isFunc(fn, pkgAttribute, "") && attributeFuncs[fn.Name()] && len(call.Args) == 2:
		return c.derives(call.Args[1], c.cfg.principal)
	case isMethod(fn, pkgAttribute, "Key", "") && len(call.Args) == 1:
		return c.derives(call.Args[0], c.cfg.principal)
	}
	return false
}

// checkCredential reports a credential value. The credential type's own methods, which implement
// its redacted slog.LogValuer form, are not checked.
func (c *checker) checkCredential(ctx *funcCtx, arg ast.Expr) {
	for _, credential := range c.cfg.credentials {
		if ctx.fn != nil && isMethod(ctx.fn, credential.path, credential.name, "") {
			continue
		}
		if c.derives(arg, credential) {
			c.reportf(arg, "credential: a %s value must not be a log or span attribute value (LOGGING.md section 6)", credential.name)
			return
		}
	}
}

// derives reports whether expr is a value of the target type, a conversion of one, the result of
// one of its methods, or a concatenation or fmt.Sprint* formatting that includes one.
func (c *checker) derives(expr ast.Expr, target qualifiedType) bool {
	expr = ast.Unparen(expr)
	if t := c.info.TypeOf(expr); t != nil && isNamed(t, target.path, target.name) {
		return true
	}
	if bin, ok := expr.(*ast.BinaryExpr); ok {
		return c.derives(bin.X, target) || c.derives(bin.Y, target)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if tv, ok := c.info.Types[call.Fun]; ok && tv.IsType() {
		return len(call.Args) == 1 && c.derives(call.Args[0], target)
	}
	if fn, _ := typeutil.Callee(c.info, call).(*types.Func); fn != nil && isFunc(fn, pkgFmt, "") && strings.HasPrefix(fn.Name(), "Sprint") {
		for _, arg := range call.Args {
			if c.derives(arg, target) {
				return true
			}
		}
		return false
	}
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		if s := c.info.Selections[sel]; s != nil && s.Kind() == types.MethodVal {
			return c.derives(sel.X, target)
		}
	}
	return false
}

// optionsHave reports whether the arguments from index start include a call to a function that
// matches want. A variable, spread or not, matches when its package-level initializer or an
// assignment to it in the enclosing function includes one.
func (c *checker) optionsHave(ctx *funcCtx, call *ast.CallExpr, start int, want func(*types.Func) bool) bool {
	for i := start; i < len(call.Args); i++ {
		if c.contains(ctx, call.Args[i], want, 3) {
			return true
		}
	}
	return false
}

func (c *checker) contains(ctx *funcCtx, node ast.Node, want func(*types.Func) bool, depth int) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.CallExpr:
			if fn, _ := typeutil.Callee(c.info, n).(*types.Func); fn != nil && want(fn) {
				found = true
			}
		case *ast.Ident:
			if depth > 0 {
				for _, value := range c.assignments(ctx, n) {
					if c.contains(ctx, value, want, depth-1) {
						found = true
						break
					}
				}
			}
		}
		return !found
	})
	return found
}

// assignments returns the values assigned to the variable expr in the enclosing function, or its
// package-level initializer.
func (c *checker) assignments(ctx *funcCtx, expr ast.Expr) []ast.Expr {
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := c.info.ObjectOf(ident).(*types.Var)
	if !ok {
		return nil
	}
	if init, ok := c.pkgVars[v]; ok {
		return []ast.Expr{init}
	}
	if ctx.body == nil {
		return nil
	}
	var values []ast.Expr
	ast.Inspect(ctx.body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && c.info.ObjectOf(id) == v {
					if len(n.Lhs) == len(n.Rhs) {
						values = append(values, n.Rhs[i])
					} else if len(n.Rhs) == 1 {
						values = append(values, n.Rhs[0])
					}
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if c.info.ObjectOf(name) == v && i < len(n.Values) {
					values = append(values, n.Values[i])
				}
			}
		}
		return true
	})
	return values
}

// resolveScope returns the constant scope of a tracer or meter expression, or "" when it is not
// known locally.
func (c *checker) resolveScope(ctx *funcCtx, expr ast.Expr) string {
	for range 5 {
		if expr == nil {
			return ""
		}
		expr = ast.Unparen(expr)
		if call, ok := expr.(*ast.CallExpr); ok {
			fn, _ := typeutil.Callee(c.info, call).(*types.Func)
			if fn == nil || len(call.Args) == 0 {
				return ""
			}
			if isFunc(fn, pkgOtel, "Tracer") || isFunc(fn, pkgOtel, "Meter") ||
				isMethod(fn, pkgTrace, "TracerProvider", "Tracer") || isMethod(fn, pkgMetric, "MeterProvider", "Meter") {
				scope, _ := c.constString(call.Args[0])
				return scope
			}
			return ""
		}
		values := c.assignments(ctx, expr)
		if len(values) != 1 {
			return ""
		}
		expr = values[0]
	}
	return ""
}

func (c *checker) paramIndex(ctx *funcCtx, expr ast.Expr) int {
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok || ctx.sig == nil {
		return -1
	}
	obj := c.info.Uses[ident]
	for i := range ctx.sig.Params().Len() {
		if ctx.sig.Params().At(i) == obj {
			return i
		}
	}
	return -1
}

func (c *checker) constString(expr ast.Expr) (string, bool) {
	if expr == nil {
		return "", false
	}
	tv, ok := c.info.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func (c *checker) isPrincipalMethod(fn *types.Func, name string) bool {
	return isMethod(fn, c.cfg.principal.path, c.cfg.principal.name, name)
}

func receiver(call *ast.CallExpr) ast.Expr {
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		return sel.X
	}
	return nil
}

func isPrincipalKey(key string) bool {
	return key == "principal" || strings.HasSuffix(key, ".principal")
}

// isFunc reports whether fn is the package-level function path.name. An empty name matches any.
func isFunc(fn *types.Func, path, name string) bool {
	sig, _ := fn.Type().(*types.Signature)
	return fn.Pkg() != nil && fn.Pkg().Path() == path && sig != nil && sig.Recv() == nil && (name == "" || fn.Name() == name)
}

// isMethod reports whether fn is the method name of the type path.typeName. An empty name
// matches any method.
func isMethod(fn *types.Func, path, typeName, name string) bool {
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil || (name != "" && fn.Name() != name) {
		return false
	}
	return isNamed(sig.Recv().Type(), path, typeName)
}

func isNamed(t types.Type, path, name string) bool {
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == path && obj.Name() == name
}
