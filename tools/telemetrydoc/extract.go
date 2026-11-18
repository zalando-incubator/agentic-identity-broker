package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const (
	otelPkg     = "go.opentelemetry.io/otel"
	tracePkg    = "go.opentelemetry.io/otel/trace"
	metricPkg   = "go.opentelemetry.io/otel/metric"
	attrPkg     = "go.opentelemetry.io/otel/attribute"
	codesPkg    = "go.opentelemetry.io/otel/codes"
	resourcePkg = "go.opentelemetry.io/otel/sdk/resource"
)

var attrBuilders = []string{
	"String", "Int", "Int64", "Bool", "Float64",
	"StringSlice", "IntSlice", "Int64Slice", "BoolSlice", "Float64Slice",
	"Stringer",
}

var instruments = map[string]bool{
	"Int64Counter": true, "Int64UpDownCounter": true, "Int64Histogram": true, "Int64Gauge": true,
	"Int64ObservableCounter": true, "Int64ObservableUpDownCounter": true, "Int64ObservableGauge": true,
	"Float64Counter": true, "Float64UpDownCounter": true, "Float64Histogram": true, "Float64Gauge": true,
	"Float64ObservableCounter": true, "Float64ObservableUpDownCounter": true, "Float64ObservableGauge": true,
}

// measurements are the metric API methods that record a value.
var measurements = []string{"Add", "Record", "Observe", "ObserveInt64", "ObserveFloat64"}

// Reference is everything the extractor found.
type Reference struct {
	Spans   map[string]*Span
	Metrics map[string]*Metric
	// Gaps are places that the extractor could not follow.
	Gaps []Gap
}

// EnumValue is one constant of an enumerated attribute type.
type EnumValue struct{ Value, Doc string }

// Attr is one attribute key and what the code can put in it.
type Attr struct {
	Key string
	// Types are the Go types of values that are not constants.
	Types map[string]bool
	// Values are constant values.
	Values map[string]bool
	// EnumType names a string type of this module whose constants are the values.
	EnumType string
	Enum     []EnumValue
}

// Span is one span name.
type Span struct {
	Name, Scope, Kind string
	// RenamedFrom is the span that SetName renames to this one.
	RenamedFrom string
	Files       map[string]bool
	ErrorStatus bool
	Attrs       map[string]*Attr
}

// Metric is one instrument.
type Metric struct {
	Name, Instrument, Unit, Desc, Scope, File string
	Attrs                                     map[string]*Attr
}

// Gap is a place the extractor could not follow.
type Gap struct{ Where, What string }

type fnCtx struct {
	decl *ast.FuncDecl
	pkg  *packages.Package
}

func (c fnCtx) info() *types.Info { return c.pkg.TypesInfo }

type pkgVar struct {
	expr ast.Expr
	pkg  *packages.Package
}

// source is an expression and the function that contains it.
type source struct {
	expr ast.Expr
	c    fnCtx
}

type callSite struct {
	call *ast.CallExpr
	c    fnCtx
}

type rename struct {
	from, to string
	at       token.Pos
}

// wrapper is a module function that starts a span with a name it receives.
type wrapper struct {
	start  *ast.CallExpr
	c      fnCtx
	nameAt int
}

type extractor struct {
	root       string
	modulePath string
	fset       *token.FileSet
	pkgs       []*packages.Package
	byTypes    map[*types.Package]*packages.Package
	funcs      map[*types.Func]fnCtx
	constDoc   map[types.Object]string
	pkgVars    map[types.Object]pkgVar
	fieldScope map[types.Object]string
	// fieldValues are the values assigned to struct fields that hold
	// attributes or a context.
	fieldValues map[types.Object][]source
	// starters records whether each module function returns a context with a
	// span that it starts.
	starters   map[*types.Func]bool
	instrObjs  map[types.Object][]*Metric
	spanFields map[types.Object][]*Span
	// funcUses counts the references to each module function, and
	// ifaceMethods names the methods that module interfaces declare.
	funcUses     map[*types.Func]int
	ifaceMethods map[string]bool
	wrappers     map[*types.Func]wrapper
	callSites    map[*types.Func][]callSite
	renames      []rename
	// reached holds each call that builds an attribute, changes a span or
	// records a measurement, and that the extractor followed to a span, a
	// metric or a resource. Every other such call is a gap.
	reached map[*ast.CallExpr]bool
	ref     *Reference
	current []Gap
}

// Extract loads patterns from the module at root and returns the reference.
func Extract(dir string, patterns ...string) (*Reference, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports | packages.NeedModule,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	var errs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("load packages:\n%s", strings.Join(errs, "\n"))
	}
	x := &extractor{
		root:         root,
		pkgs:         pkgs,
		byTypes:      map[*types.Package]*packages.Package{},
		funcs:        map[*types.Func]fnCtx{},
		constDoc:     map[types.Object]string{},
		pkgVars:      map[types.Object]pkgVar{},
		fieldScope:   map[types.Object]string{},
		fieldValues:  map[types.Object][]source{},
		starters:     map[*types.Func]bool{},
		instrObjs:    map[types.Object][]*Metric{},
		spanFields:   map[types.Object][]*Span{},
		funcUses:     map[*types.Func]int{},
		ifaceMethods: map[string]bool{},
		wrappers:     map[*types.Func]wrapper{},
		callSites:    map[*types.Func][]callSite{},
		reached:      map[*ast.CallExpr]bool{},
		ref:          &Reference{Spans: map[string]*Span{}, Metrics: map[string]*Metric{}},
	}
	if len(pkgs) > 0 {
		x.fset = pkgs[0].Fset
		if pkgs[0].Module != nil {
			x.modulePath = pkgs[0].Module.Path
		}
	}
	x.index()
	x.extract()
	return x.ref, nil
}

func (x *extractor) rel(p token.Pos) (string, int) {
	ps := x.fset.Position(p)
	rel, err := filepath.Rel(x.root, ps.Filename)
	if err != nil {
		rel = ps.Filename
	}
	return filepath.ToSlash(rel), ps.Line
}

func (x *extractor) pos(p token.Pos) string {
	f, l := x.rel(p)
	return fmt.Sprintf("%s:%d", f, l)
}

func (x *extractor) file(p token.Pos) string {
	f, _ := x.rel(p)
	return f
}

func (x *extractor) gap(p token.Pos, what string) {
	x.current = append(x.current, Gap{x.pos(p), what})
}

// quietly runs f and drops the gaps that it reports.
func (x *extractor) quietly(f func()) {
	n := len(x.current)
	f()
	x.current = x.current[:n]
}

// ---------- index ----------

func (x *extractor) index() {
	for _, p := range x.pkgs {
		x.byTypes[p.Types] = p
	}
	for _, p := range x.pkgs {
		for _, obj := range p.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok {
				x.funcUses[fn.Origin()]++
			}
		}
		for _, f := range p.Syntax {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if fn, ok := p.TypesInfo.Defs[d.Name].(*types.Func); ok {
						x.funcs[fn] = fnCtx{d, p}
					}
				case *ast.GenDecl:
					x.indexValues(d, p)
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if it, ok := n.(*ast.InterfaceType); ok {
					for _, m := range it.Methods.List {
						for _, name := range m.Names {
							x.ifaceMethods[name.Name] = true
						}
					}
				}
				return true
			})
		}
	}
	// Fields that hold a tracer, a meter or attributes.
	x.eachBody(func(body ast.Node, c fnCtx) {
		ast.Inspect(body, func(n ast.Node) bool {
			eachFieldValue(n, c.info(), func(f *types.Var, v ast.Expr) {
				if s := x.resolveScope(v, c, 0); s != "" {
					x.fieldScope[f] = s
				}
				if attrish(f.Type()) || isContext(f.Type()) {
					x.fieldValues[f] = append(x.fieldValues[f], source{v, c})
				}
			})
			return true
		})
	})
}

func (x *extractor) indexValues(d *ast.GenDecl, p *packages.Package) {
	for _, s := range d.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok {
			continue
		}
		doc := firstSentence(commentText(vs.Doc, vs.Comment))
		if doc == "" && len(d.Specs) == 1 {
			doc = firstSentence(commentText(d.Doc))
		}
		for i, n := range vs.Names {
			obj := p.TypesInfo.Defs[n]
			if obj == nil {
				continue
			}
			if d.Tok == token.CONST {
				x.constDoc[obj] = withoutName(doc, n.Name)
			}
			if d.Tok == token.VAR && len(vs.Values) == len(vs.Names) {
				x.pkgVars[obj] = pkgVar{vs.Values[i], p}
			}
		}
	}
}

func (x *extractor) eachFunc(f func(fnCtx)) {
	for _, p := range x.pkgs {
		for _, file := range p.Syntax {
			for _, d := range file.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
					f(fnCtx{fd, p})
				}
			}
		}
	}
}

// eachBody calls f for each function body and each package variable
// declaration.
func (x *extractor) eachBody(f func(ast.Node, fnCtx)) {
	for _, p := range x.pkgs {
		for _, file := range p.Syntax {
			for _, d := range file.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body != nil {
						f(d.Body, fnCtx{d, p})
					}
				case *ast.GenDecl:
					if d.Tok == token.VAR {
						f(d, pkgVarCtx(p))
					}
				}
			}
		}
	}
}

// eachFieldValue calls f for each struct field that n assigns, as in
// T{f: v}, T{v} or x.f = v.
func eachFieldValue(n ast.Node, info *types.Info, f func(*types.Var, ast.Expr)) {
	switch n := n.(type) {
	case *ast.CompositeLit:
		t := info.TypeOf(n)
		if t == nil {
			return
		}
		if p, ok := t.Underlying().(*types.Pointer); ok {
			t = p.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return
		}
		for i, el := range n.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok {
					if v, ok := info.Uses[id].(*types.Var); ok && v.IsField() {
						f(v, kv.Value)
					}
				}
			} else if i < st.NumFields() {
				f(st.Field(i), el)
			}
		}
	case *ast.AssignStmt:
		if len(n.Lhs) != len(n.Rhs) {
			return
		}
		for i, l := range n.Lhs {
			if sel, ok := ast.Unparen(l).(*ast.SelectorExpr); ok {
				if v, ok := info.Uses[sel.Sel].(*types.Var); ok && v.IsField() {
					f(v, n.Rhs[i])
				}
			}
		}
	}
}

// ---------- helpers ----------

func commentText(groups ...*ast.CommentGroup) string {
	var parts []string
	for _, g := range groups {
		if g == nil {
			continue
		}
		for _, line := range strings.Split(g.Text(), "\n") {
			// A linter directive is not documentation.
			if i := strings.Index(line, "#nosec"); i >= 0 {
				line = line[:i]
			}
			if line = strings.TrimSpace(line); line != "" {
				parts = append(parts, line)
			}
		}
	}
	return strings.Join(parts, " ")
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// withoutName drops the Go identifier that starts a doc comment, as in
// "OutcomeOK means the call worked." or "OutcomeOK: the call worked.".
func withoutName(doc, name string) string {
	rest, ok := strings.CutPrefix(doc, name)
	if !ok {
		return doc
	}
	for _, sep := range []string{": ", " means ", " is "} {
		if r, ok := strings.CutPrefix(rest, sep); ok && r != "" {
			return strings.ToUpper(r[:1]) + r[1:]
		}
	}
	return doc
}

func constString(info *types.Info, e ast.Expr) (string, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil {
		return "", false
	}
	if tv.Value.Kind() == constant.String {
		return constant.StringVal(tv.Value), true
	}
	return tv.Value.ExactString(), true
}

func callee(info *types.Info, c *ast.CallExpr) *types.Func {
	fn, _ := typeutil.Callee(info, c).(*types.Func)
	if fn != nil {
		fn = fn.Origin()
	}
	return fn
}

// builtinOrConversion reports whether c calls a builtin such as append, or
// converts a value to a type.
func builtinOrConversion(info *types.Info, c *ast.CallExpr) bool {
	tv, ok := info.Types[c.Fun]
	return ok && (tv.IsBuiltin() || tv.IsType())
}

func recvOf(fn *types.Func) *types.Var {
	return fn.Type().(*types.Signature).Recv()
}

// isMethod reports whether fn is a method in pkg, with one of names if any are given.
func isMethod(fn *types.Func, pkg string, names ...string) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != pkg || recvOf(fn) == nil {
		return false
	}
	return len(names) == 0 || contains(names, fn.Name())
}

func isFunc(fn *types.Func, pkg string, names ...string) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != pkg || recvOf(fn) != nil {
		return false
	}
	return contains(names, fn.Name())
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func isNamed(t types.Type, pkg, name string) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkg && n.Obj().Name() == name
}

func isSpanType(t types.Type) bool { return t != nil && isNamed(t, tracePkg, "Span") }

func isContext(t types.Type) bool { return t != nil && isNamed(t, "context", "Context") }

// element returns the element type of a slice, an array or a map, and t
// otherwise.
func element(t types.Type) types.Type {
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return u.Elem()
	case *types.Array:
		return u.Elem()
	case *types.Map:
		return u.Elem()
	}
	return t
}

// attrish reports whether values of t carry attributes or options.
func attrish(t types.Type) bool {
	if tu, ok := t.(*types.Tuple); ok {
		for i := 0; i < tu.Len(); i++ {
			if attrish(tu.At(i).Type()) {
				return true
			}
		}
		return false
	}
	if t == nil {
		return false
	}
	t = element(t)
	return isNamed(t, attrPkg, "KeyValue") || isNamed(t, attrPkg, "Set") ||
		isNamed(t, tracePkg, "SpanStartOption") || isNamed(t, tracePkg, "EventOption") ||
		isNamed(t, metricPkg, "AddOption") || isNamed(t, metricPkg, "RecordOption") ||
		isNamed(t, metricPkg, "ObserveOption") || isNamed(t, metricPkg, "MeasurementOption")
}

// holdsKeyValue reports whether t is an attribute, a collection of
// attributes, or a result list with one of them.
func holdsKeyValue(t types.Type) bool {
	if tu, ok := t.(*types.Tuple); ok {
		for i := 0; i < tu.Len(); i++ {
			if holdsKeyValue(tu.At(i).Type()) {
				return true
			}
		}
		return false
	}
	return t != nil && isNamed(element(t), attrPkg, "KeyValue")
}

func returnsAttrs(fn *types.Func) bool {
	return attrish(fn.Type().(*types.Signature).Results())
}

func objOf(info *types.Info, e ast.Expr) types.Object {
	switch e := e.(type) {
	case *ast.Ident:
		return info.ObjectOf(e)
	case *ast.SelectorExpr:
		return info.ObjectOf(e.Sel)
	case *ast.ParenExpr:
		return objOf(info, e.X)
	case *ast.StarExpr:
		return objOf(info, e.X)
	}
	return nil
}

// varOf returns the variable that e names.
func varOf(info *types.Info, e ast.Expr) (*types.Var, bool) {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident, *ast.SelectorExpr:
		v, ok := objOf(info, e).(*types.Var)
		return v, ok
	}
	return nil, false
}

func isLocal(obj types.Object, c fnCtx) bool {
	v, ok := obj.(*types.Var)
	return ok && !v.IsField() && obj.Pos() >= c.decl.Pos() && obj.Pos() < c.decl.End()
}

// isParam reports whether obj is a parameter or the receiver of c's function.
func isParam(obj types.Object, c fnCtx) bool {
	in := func(l *ast.FieldList) bool { return l != nil && obj.Pos() >= l.Pos() && obj.Pos() < l.End() }
	return in(c.decl.Type.Params) || in(c.decl.Recv)
}

func paramIndex(obj types.Object, c fnCtx) int {
	i := 0
	for _, f := range c.decl.Type.Params.List {
		for _, n := range f.Names {
			if c.info().Defs[n] == obj {
				return i
			}
			i++
		}
	}
	return -1
}

// isClosureParam reports whether v is a parameter of a function literal in c.
func isClosureParam(v *types.Var, c fnCtx) bool {
	found := false
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && v.Pos() >= lit.Type.Pos() && v.Pos() < lit.Type.End() {
			found = true
		}
		return !found
	})
	return found
}

// pkgVarCtx is a function context for the initializer of a package variable.
func pkgVarCtx(p *packages.Package) fnCtx {
	return fnCtx{&ast.FuncDecl{Body: &ast.BlockStmt{}, Type: &ast.FuncType{}}, p}
}

// sourcesOf returns the expressions assigned to the local variable obj in
// c: its initializer, its assignments, the operand of a range statement
// that sets it, and the map or slice that it is stored into. exact is false
// when obj can also hold something that the expressions do not show, as
// with a multi-value assignment, a range statement, or a pointer to obj.
func sourcesOf(obj types.Object, c fnCtx) (out []ast.Expr, exact bool) {
	info := c.info()
	exact = true
	is := func(e ast.Expr) bool {
		if ix, ok := ast.Unparen(e).(*ast.IndexExpr); ok && objOf(info, ix.X) == obj {
			exact = false
			return true
		}
		return objOf(info, e) == obj
	}
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, l := range n.Lhs {
				if !is(l) {
					continue
				}
				switch {
				case n.Tok != token.ASSIGN && n.Tok != token.DEFINE:
					exact = false
					out = append(out, n.Rhs[0])
				case len(n.Lhs) == len(n.Rhs):
					out = append(out, n.Rhs[i])
				default:
					exact = false
					out = append(out, n.Rhs[0])
				}
			}
		case *ast.ValueSpec:
			for i, nm := range n.Names {
				if info.Defs[nm] != obj || len(n.Values) == 0 {
					continue
				}
				if len(n.Values) == len(n.Names) {
					out = append(out, n.Values[i])
				} else {
					exact = false
					out = append(out, n.Values[0])
				}
			}
		case *ast.RangeStmt:
			if n.Key != nil && is(n.Key) || n.Value != nil && is(n.Value) {
				exact = false
				out = append(out, n.X)
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND && is(n.X) {
				exact = false
			}
		}
		return true
	})
	return out, exact
}

// callArgs returns the arguments that call passes for parameter i of fn.
func callArgs(fn *types.Func, i int, call *ast.CallExpr) ([]ast.Expr, bool) {
	sig := fn.Type().(*types.Signature)
	if sig.Variadic() && i == sig.Params().Len()-1 && i <= len(call.Args) {
		return call.Args[i:], true
	}
	if i >= len(call.Args) {
		return nil, false
	}
	return call.Args[i : i+1], true
}

// resolveScope finds the constant name passed to Tracer(...) or Meter(...).
func (x *extractor) resolveScope(e ast.Expr, c fnCtx, depth int) string {
	if depth > 4 || e == nil {
		return ""
	}
	info := c.info()
	switch e := e.(type) {
	case *ast.CallExpr:
		fn := callee(info, e)
		if fn != nil && fn.Pkg() != nil && (fn.Name() == "Tracer" || fn.Name() == "Meter") &&
			contains([]string{otelPkg, tracePkg, metricPkg}, fn.Pkg().Path()) && len(e.Args) > 0 {
			if s, ok := constString(info, e.Args[0]); ok {
				return s
			}
		}
	case *ast.ParenExpr:
		return x.resolveScope(e.X, c, depth+1)
	case *ast.Ident, *ast.SelectorExpr:
		obj := objOf(info, e)
		if obj == nil {
			return ""
		}
		if s, ok := x.fieldScope[obj]; ok {
			return s
		}
		if isLocal(obj, c) {
			srcs, _ := sourcesOf(obj, c)
			for _, rhs := range srcs {
				if s := x.resolveScope(rhs, c, depth+1); s != "" {
					return s
				}
			}
		}
		if pv, ok := x.pkgVars[obj]; ok {
			return x.resolveScope(pv.expr, pkgVarCtx(pv.pkg), depth+1)
		}
	}
	return ""
}

// onlyCalledDirectly reports whether every call of fn is a static call in
// the loaded packages: fn is never used as a value, and it cannot be called
// through an interface.
func (x *extractor) onlyCalledDirectly(fn *types.Func) bool {
	if x.funcUses[fn] != len(x.callSites[fn]) {
		return false
	}
	if recvOf(fn) == nil {
		return true
	}
	// An exported method can satisfy an interface of another module.
	return !fn.Exported() && !x.ifaceMethods[fn.Name()]
}

// typeName names a type of this module by its package directory, and other
// types by their package name.
func (x *extractor) typeName(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string {
		if x.modulePath != "" && strings.HasPrefix(p.Path(), x.modulePath+"/") {
			return strings.TrimPrefix(p.Path(), x.modulePath+"/")
		}
		return p.Name()
	})
}

// ---------- attributes ----------

// collector follows attributes, span kinds and attribute values back from
// where a span, a metric or a resource receives them.
type collector struct {
	x     *extractor
	attrs map[string]*Attr
	kinds map[string]bool
	// active holds what the collector is following, so that it does not
	// follow it into itself.
	active map[any]bool
	// visited holds the calls that pass a span to a module function, which
	// the collector followed.
	visited map[*ast.CallExpr]bool
	// bind gives, for each parameter of a function that the collector
	// follows from a call, the arguments of that call.
	bind map[types.Object][]source
}

func newCollector(x *extractor, attrs map[string]*Attr) *collector {
	return &collector{
		x: x, attrs: attrs, kinds: map[string]bool{}, active: map[any]bool{},
		visited: map[*ast.CallExpr]bool{}, bind: map[types.Object][]source{},
	}
}

func (k *collector) enter(key any) bool {
	if k.active[key] {
		return false
	}
	k.active[key] = true
	return true
}

func (k *collector) leave(key any) { delete(k.active, key) }

// kind returns the span kind that the start options set.
func (k *collector) kind(p token.Pos) string {
	kinds := sortedKeys(k.kinds)
	if len(kinds) > 1 {
		k.x.gap(p, "span start options set more than one kind")
	}
	if len(kinds) == 0 {
		return ""
	}
	return kinds[len(kinds)-1]
}

// bindCall binds the parameters of fn to the arguments of call in c, and
// returns a function that removes the binding.
func (k *collector) bindCall(fn *types.Func, call *ast.CallExpr, c fnCtx) func() {
	params := fn.Type().(*types.Signature).Params()
	saved := map[types.Object][]source{}
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		if old, ok := k.bind[p]; ok {
			saved[p] = old
		}
		args, ok := callArgs(fn, i, call)
		if !ok {
			delete(k.bind, p)
			continue
		}
		srcs := make([]source, len(args))
		for j, a := range args {
			srcs[j] = source{a, c}
		}
		k.bind[p] = srcs
	}
	return func() {
		for i := 0; i < params.Len(); i++ {
			p := params.At(i)
			if old, ok := saved[p]; ok {
				k.bind[p] = old
			} else {
				delete(k.bind, p)
			}
		}
	}
}

// argsFor returns the arguments for the parameter v of c's function: the
// arguments of the call being followed, or else the arguments of every
// call. It fails when the function can be called in a way that the
// extractor does not see.
func (k *collector) argsFor(v *types.Var, c fnCtx) ([]source, bool) {
	if b, ok := k.bind[v]; ok {
		return b, true
	}
	if c.decl.Name == nil || !isParam(v, c) {
		return nil, false
	}
	fn, _ := c.info().Defs[c.decl.Name].(*types.Func)
	i := paramIndex(v, c)
	if fn == nil || i < 0 || len(k.x.callSites[fn]) == 0 || !k.x.onlyCalledDirectly(fn) {
		return nil, false
	}
	var out []source
	for _, cs := range k.x.callSites[fn] {
		args, ok := callArgs(fn, i, cs.call)
		if !ok {
			return nil, false
		}
		for _, a := range args {
			out = append(out, source{a, cs.c})
		}
	}
	return out, true
}

// valueSources returns what the variable v in c can hold: the arguments of
// a parameter, or the values assigned to a local variable.
func (k *collector) valueSources(v *types.Var, c fnCtx) ([]source, bool) {
	_, bound := k.bind[v]
	switch {
	case v.IsField():
		return nil, false
	case bound || isParam(v, c):
		return k.argsFor(v, c)
	case isLocal(v, c):
		exprs, exact := sourcesOf(v, c)
		if !exact || len(exprs) == 0 {
			return nil, false
		}
		out := make([]source, len(exprs))
		for i, e := range exprs {
			out[i] = source{e, c}
		}
		return out, true
	}
	return nil, false
}

// constValues resolves e to the constants that it can hold.
func (k *collector) constValues(e ast.Expr, c fnCtx) ([]string, bool) {
	if s, ok := constString(c.info(), e); ok {
		return []string{s}, true
	}
	v, ok := varOf(c.info(), e)
	if !ok {
		return nil, false
	}
	if pv, ok := k.x.pkgVars[v]; ok {
		s, ok := constString(pv.pkg.TypesInfo, pv.expr)
		return []string{s}, ok
	}
	srcs, ok := k.valueSources(v, c)
	if !ok || !k.enter(v) {
		return nil, false
	}
	defer k.leave(v)
	var out []string
	for _, s := range srcs {
		vals, ok := k.constValues(s.expr, s.c)
		if !ok {
			return nil, false
		}
		out = append(out, vals...)
	}
	return out, true
}

func (k *collector) add(key string, value ast.Expr, c fnCtx) {
	a := k.attrs[key]
	if a == nil {
		a = &Attr{Key: key, Types: map[string]bool{}, Values: map[string]bool{}}
		k.attrs[key] = a
	}
	if value != nil {
		k.addValue(a, value, c)
	}
}

func (k *collector) addValue(a *Attr, value ast.Expr, c fnCtx) {
	info := c.info()
	if s, ok := constString(info, value); ok {
		// An empty constant usually reaches a branch that checks for it, as
		// in if v != "".
		if s != "" {
			a.Values[s] = true
		}
		return
	}
	// Enumerated values: string(x), x.String() or x, where x has a named
	// string type of this module.
	v := value
	if c, ok := v.(*ast.CallExpr); ok && len(c.Args) == 1 {
		if tv, ok := info.Types[c.Fun]; ok && tv.IsType() {
			v = c.Args[0]
		}
	}
	if c, ok := v.(*ast.CallExpr); ok && len(c.Args) == 0 {
		if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "String" {
			v = s.X
		}
	}
	if k.addEnum(a, info.TypeOf(v)) {
		return
	}
	// A variable: what its assignments or the callers put in it.
	if id, ok := ast.Unparen(value).(*ast.Ident); ok {
		if obj, ok := info.Uses[id].(*types.Var); ok {
			if srcs, ok := k.valueSources(obj, c); ok && k.enter(obj) {
				for _, s := range srcs {
					k.addValue(a, s.expr, s.c)
				}
				k.leave(obj)
				return
			}
		}
	}
	if t := info.TypeOf(value); t != nil {
		a.Types[k.x.typeName(t)] = true
	}
}

// addEnum records t as a's enumeration when t is a string type of this
// module that has constants.
func (k *collector) addEnum(a *Attr, t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	if b, ok := named.Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return false
	}
	if _, ours := k.x.byTypes[named.Obj().Pkg()]; !ours {
		return false
	}
	name := k.x.typeName(named)
	if a.EnumType == name {
		return true
	}
	var enum []EnumValue
	scope := named.Obj().Pkg().Scope()
	for _, n := range scope.Names() {
		cst, ok := scope.Lookup(n).(*types.Const)
		if !ok || !types.Identical(cst.Type(), named) {
			continue
		}
		enum = append(enum, EnumValue{constant.StringVal(cst.Val()), k.x.constDoc[cst]})
	}
	if len(enum) == 0 || a.EnumType != "" {
		// A named string type with no constants is not an enumeration, and
		// a second enumeration for the same key is listed as a type.
		return false
	}
	sort.Slice(enum, func(i, j int) bool { return enum[i].Value < enum[j].Value })
	a.EnumType, a.Enum = name, enum
	return true
}

// keysIn collects the attributes and span kinds in e. It follows variables,
// struct fields, parameters, and the functions and closures that return
// attributes or options.
func (k *collector) keysIn(e ast.Node, c fnCtx) {
	if e == nil {
		return
	}
	info := c.info()
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			// A function literal is followed where it is called.
			return false
		case *ast.KeyValueExpr:
			if _, ok := n.Key.(*ast.Ident); ok {
				k.keysIn(n.Value, c)
				return false
			}
		case *ast.CallExpr:
			return k.keysInCall(n, c)
		case *ast.Ident:
			if v, ok := info.Uses[n].(*types.Var); ok && attrish(v.Type()) {
				k.keysInVar(v, n, c)
			}
		}
		return true
	})
}

// keysInVar follows what the variable v can hold.
func (k *collector) keysInVar(v *types.Var, at ast.Node, c fnCtx) {
	if !k.enter(v) {
		return
	}
	defer k.leave(v)
	_, bound := k.bind[v]
	var srcs []source
	switch {
	case v.IsField():
		srcs = k.x.fieldValues[v]
	case bound || isParam(v, c):
		args, ok := k.argsFor(v, c)
		if !ok {
			k.x.gap(at.Pos(), "attributes reach the parameter "+v.Name()+" from a call that the generator does not see")
		}
		for _, s := range args {
			k.keysIn(s.expr, s.c)
		}
		return
	case isLocal(v, c):
		exprs, _ := sourcesOf(v, c)
		if len(exprs) == 0 && !isClosureParam(v, c) {
			// Declared and never assigned.
			return
		}
		for _, e := range exprs {
			srcs = append(srcs, source{e, c})
		}
	default:
		if pv, ok := k.x.pkgVars[v]; ok {
			srcs = []source{{pv.expr, pkgVarCtx(pv.pkg)}}
		}
	}
	if len(srcs) == 0 {
		k.x.gap(at.Pos(), "the generator cannot see what "+v.Name()+" holds")
		return
	}
	for _, s := range srcs {
		k.keysIn(s.expr, s.c)
	}
}

// keysInCall reads the attributes or the span kind that n builds. It
// reports whether keysIn should read the arguments of n too.
func (k *collector) keysInCall(n *ast.CallExpr, c fnCtx) bool {
	info := c.info()
	if builtinOrConversion(info, n) {
		return true
	}
	fn := callee(info, n)
	switch {
	case isFunc(fn, attrPkg, attrBuilders...) && len(n.Args) >= 1:
		k.x.reached[n] = true
		var v ast.Expr
		if len(n.Args) > 1 {
			v = n.Args[1]
		}
		k.addKeys(n, n.Args[0], v, c, "attribute key is not a constant")
	case isMethod(fn, attrPkg) && isNamed(recvOf(fn).Type(), attrPkg, "Key") && returnsAttrs(fn):
		k.x.reached[n] = true
		var v ast.Expr
		if len(n.Args) > 0 {
			v = n.Args[0]
		}
		k.addKeys(n, n.Fun.(*ast.SelectorExpr).X, v, c, "attribute.Key receiver is not a constant")
	case isFunc(fn, tracePkg, "WithSpanKind") && len(n.Args) == 1:
		kind, ok := spanKind(info, n.Args[0])
		if !ok {
			k.x.gap(n.Pos(), "span kind is not a trace.SpanKind constant")
		}
		k.kinds[kind] = true
	case fn != nil && returnsAttrs(fn):
		k.x.reached[n] = true
		fc, ours := k.x.funcs[fn]
		if !ours {
			if holdsKeyValue(info.TypeOf(n)) && fn.Pkg() != nil && fn.Pkg().Path() != attrPkg {
				k.x.gap(n.Pos(), "attribute from "+fn.Pkg().Name()+"."+fn.Name()+" is not resolved")
			}
			return true
		}
		k.followCall(fn, fc, n, c)
		return false
	case fn == nil && attrish(info.TypeOf(n)):
		k.x.reached[n] = true
		k.keysInFuncValue(n, c)
	}
	return true
}

func (k *collector) addKeys(call *ast.CallExpr, key, value ast.Expr, c fnCtx, gap string) {
	keys, ok := k.constValues(key, c)
	if !ok {
		k.x.gap(call.Pos(), gap)
		return
	}
	for _, key := range keys {
		k.add(key, value, c)
	}
}

// followCall reads what the module function fn returns to call.
func (k *collector) followCall(fn *types.Func, fc fnCtx, call *ast.CallExpr, c fnCtx) {
	if !k.enter(fn) {
		return
	}
	defer k.leave(fn)
	defer k.bindCall(fn, call, c)()
	k.keysInReturns(fc.decl.Body, fn.Type().(*types.Signature), fc)
}

// keysInReturns reads the attribute and option results that body returns.
func (k *collector) keysInReturns(body *ast.BlockStmt, sig *types.Signature, c fnCtx) {
	res := sig.Results()
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			switch {
			case len(n.Results) == res.Len():
				for i, r := range n.Results {
					if attrish(res.At(i).Type()) {
						k.keysIn(r, c)
					}
				}
			case len(n.Results) == 1:
				k.keysIn(n.Results[0], c)
			case len(n.Results) == 0:
				for i := 0; i < res.Len(); i++ {
					if v := res.At(i); attrish(v.Type()) {
						k.keysInVar(v, n, c)
					}
				}
			}
		}
		return true
	})
}

// keysInFuncValue reads what a call of a closure or a function value
// returns.
func (k *collector) keysInFuncValue(call *ast.CallExpr, c fnCtx) {
	var srcs []ast.Expr
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.FuncLit:
		srcs = append(srcs, f)
	case *ast.Ident:
		if v, ok := c.info().Uses[f].(*types.Var); ok && isLocal(v, c) && !isParam(v, c) {
			srcs, _ = sourcesOf(v, c)
		}
	}
	if len(srcs) == 0 {
		k.x.gap(call.Pos(), "attributes come from a function value that the generator does not follow")
		return
	}
	for _, s := range srcs {
		if fn, ok := objOf(c.info(), s).(*types.Func); ok {
			if fc, ours := k.x.funcs[fn.Origin()]; ours {
				k.followCall(fn.Origin(), fc, call, c)
				continue
			}
		}
		lit, ok := ast.Unparen(s).(*ast.FuncLit)
		if !ok {
			k.x.gap(call.Pos(), "attributes come from a function value that the generator does not follow")
			continue
		}
		if k.enter(lit) {
			k.keysInReturns(lit.Body, c.info().TypeOf(lit).(*types.Signature), c)
			k.leave(lit)
		}
	}
}

// spanKind returns the kind that a trace.SpanKind constant names. The
// internal kind is "".
func spanKind(info *types.Info, e ast.Expr) (string, bool) {
	o, ok := objOf(info, e).(*types.Const)
	if !ok || o.Pkg() == nil || o.Pkg().Path() != tracePkg || !strings.HasPrefix(o.Name(), "SpanKind") {
		return "", false
	}
	kind := strings.TrimPrefix(o.Name(), "SpanKind")
	if kind == "Internal" || kind == "Unspecified" {
		kind = ""
	}
	return kind, true
}

// ---------- spans ----------

func (x *extractor) span(name string) *Span {
	s := x.ref.Spans[name]
	if s == nil {
		s = &Span{Name: name, Files: map[string]bool{}, Attrs: map[string]*Attr{}}
		x.ref.Spans[name] = s
	}
	return s
}

// holdSpan records that the struct field f can hold the span s.
func (x *extractor) holdSpan(f *types.Var, s *Span) {
	for _, have := range x.spanFields[f] {
		if have == s {
			return
		}
	}
	x.spanFields[f] = append(x.spanFields[f], s)
}

// trackSpan collects what c does with the span values in objs.
func (x *extractor) trackSpan(objs map[types.Object]bool, c fnCtx, s *Span, k *collector) {
	for o := range objs {
		if v, ok := o.(*types.Var); ok && v.IsField() {
			x.holdSpan(v, s)
		}
	}
	x.spanCalls(objs, c, s, k, 0)
}

// spanCalls collects attributes and error status set on the span values in
// objs within c, following module functions that receive one of them.
func (x *extractor) spanCalls(objs map[types.Object]bool, c fnCtx, s *Span, k *collector, depth int) {
	info := c.info()
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		// A struct field that receives the span.
		eachFieldValue(n, info, func(f *types.Var, v ast.Expr) {
			if objs[objOf(info, v)] {
				x.holdSpan(f, s)
			}
		})
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := callee(info, call)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && isMethod(fn, tracePkg) && objs[objOf(info, sel.X)] {
			x.spanMethod(fn.Name(), call, c, s, k)
			return true
		}
		var at []int
		for i, a := range call.Args {
			if objs[objOf(info, a)] {
				at = append(at, i)
			}
		}
		if len(at) == 0 {
			return true
		}
		// A module function that receives the span.
		fc, ours := x.funcs[fn]
		switch {
		case !ours:
			x.gap(call.Pos(), "span "+s.Name+" is passed to a function that the generator does not follow")
			return true
		case depth >= 6:
			x.gap(call.Pos(), "span "+s.Name+" is passed through more functions than the generator follows")
			return true
		case k.visited[call]:
			return true
		}
		k.visited[call] = true
		params := fn.Type().(*types.Signature).Params()
		next := map[types.Object]bool{}
		for _, i := range at {
			if i < params.Len() {
				next[params.At(i)] = true
			}
		}
		unbind := k.bindCall(fn, call, c)
		x.spanCalls(next, fc, s, k, depth+1)
		unbind()
		return true
	})
}

func (x *extractor) spanMethod(name string, call *ast.CallExpr, c fnCtx, s *Span, k *collector) {
	info := c.info()
	x.reached[call] = true
	switch name {
	case "SetAttributes":
		for _, a := range call.Args {
			k.keysIn(a, c)
		}
	case "AddEvent", "RecordError":
		ev := *k
		ev.attrs = map[string]*Attr{}
		x.quietly(func() {
			for _, a := range call.Args[1:] {
				ev.keysIn(a, c)
			}
		})
		if len(ev.attrs) > 0 {
			x.gap(call.Pos(), "the reference does not list span events, so it cannot list these event attributes")
		}
	case "SetName":
		if len(call.Args) != 1 {
			return
		}
		to, ok := constString(info, call.Args[0])
		if !ok {
			x.gap(call.Pos(), "span renamed to a name that is not a constant")
			return
		}
		x.renames = append(x.renames, rename{s.Name, to, call.Pos()})
	case "SetStatus":
		if len(call.Args) == 0 {
			return
		}
		o, ok := objOf(info, call.Args[0]).(*types.Const)
		switch {
		case !ok || o.Pkg() == nil || o.Pkg().Path() != codesPkg:
			x.gap(call.Pos(), "span status code is not a codes constant")
		case o.Name() == "Error":
			s.ErrorStatus = true
		}
	}
}

// fieldSpanCalls collects attributes and error status set on a span through
// a struct field that holds it, as in b.span.SetAttributes(...).
func (x *extractor) fieldSpanCalls(c fnCtx) {
	info := c.info()
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		field, ok := sel.X.(*ast.SelectorExpr)
		if !ok || !isMethod(callee(info, call), tracePkg) {
			return true
		}
		for _, s := range x.spanFields[objOf(info, field)] {
			x.spanMethod(sel.Sel.Name, call, c, s, newCollector(x, s.Attrs))
		}
		return true
	})
}

// identify sets the scope and kind of s, and reports a second start site
// that disagrees with the first.
func (x *extractor) identify(s *Span, scope, kind string, p token.Pos) {
	if scope == "" {
		x.gap(p, "tracer scope is not resolved")
	}
	if len(s.Files) > 0 && (s.Scope != scope || s.Kind != kind) {
		x.gap(p, "span "+s.Name+" is started in two places with a different scope or kind")
	}
	s.Scope, s.Kind = scope, kind
	s.Files[x.file(p)] = true
}

// spanVars returns the trace.Span variables and fields that receive the
// result of call.
func spanVars(call *ast.CallExpr, c fnCtx) map[types.Object]bool {
	out := map[types.Object]bool{}
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 || as.Rhs[0] != call {
			return true
		}
		for _, l := range as.Lhs {
			if o := objOf(c.info(), l); o != nil && isSpanType(o.Type()) {
				out[o] = true
			}
		}
		return false
	})
	return out
}

// ctxWalk collects the spans that a context can carry.
type ctxWalk struct {
	names map[string]bool
	seen  map[any]bool
	// unresolved is set when the context can come from a place in the
	// module that the extractor does not follow.
	unresolved bool
}

// startsSpan reports whether call starts a span and returns the context
// that holds it: a Start call, a span wrapper, or a module function that
// returns such a context on every return.
func (x *extractor) startsSpan(call *ast.CallExpr, c fnCtx) bool {
	fn := callee(c.info(), call)
	if _, wrapped := x.wrappers[fn]; wrapped || isMethod(fn, tracePkg, "Start") {
		return true
	}
	fc, ours := x.funcs[fn]
	if !ours {
		return false
	}
	if v, ok := x.starters[fn]; ok {
		return v
	}
	x.starters[fn] = false
	res := fn.Type().(*types.Signature).Results()
	starts := false
	for ri := 0; ri < res.Len(); ri++ {
		if !isContext(res.At(ri).Type()) {
			continue
		}
		returns, all := 0, true
		ast.Inspect(fc.decl.Body, func(n ast.Node) bool {
			if _, lit := n.(*ast.FuncLit); lit {
				return false
			}
			if r, ok := n.(*ast.ReturnStmt); ok {
				returns++
				all = all && len(r.Results) == res.Len() && x.ctxStarted(r.Results[ri], fc)
			}
			return true
		})
		starts = returns > 0 && all
	}
	x.starters[fn] = starts
	return starts
}

// ctxStarted reports whether the context e is set from a call that starts a
// span.
func (x *extractor) ctxStarted(e ast.Expr, c fnCtx) bool {
	if call, ok := ast.Unparen(e).(*ast.CallExpr); ok {
		return x.startsSpan(call, c)
	}
	v, ok := varOf(c.info(), e)
	if !ok || !isLocal(v, c) {
		return false
	}
	srcs, _ := sourcesOf(v, c)
	for _, s := range srcs {
		if call, ok := ast.Unparen(s).(*ast.CallExpr); ok && x.startsSpan(call, c) {
			return true
		}
	}
	return false
}

// ctxSpans finds the names of spans that a context expression can carry.
func (x *extractor) ctxSpans(e ast.Expr, c fnCtx, w *ctxWalk, depth int) {
	if depth > 64 {
		w.unresolved = true
		return
	}
	info := c.info()
	if call, ok := ast.Unparen(e).(*ast.CallExpr); ok {
		x.ctxSpansFromCall(call, c, w, depth+1)
		return
	}
	obj, ok := varOf(info, e)
	if !ok {
		w.unresolved = true
		return
	}
	if w.seen[[2]any{obj, c.decl}] {
		return
	}
	w.seen[[2]any{obj, c.decl}] = true
	if obj.IsField() {
		// A struct field that holds a context.
		if len(x.fieldValues[obj]) == 0 {
			w.unresolved = true
		}
		for _, s := range x.fieldValues[obj] {
			x.ctxSpans(s.expr, s.c, w, depth+1)
		}
		return
	}
	if !isLocal(obj, c) && !isParam(obj, c) {
		w.unresolved = true
		return
	}
	srcs, _ := sourcesOf(obj, c)
	// A context that the function sets from a Start call carries that span.
	started := false
	for _, rhs := range srcs {
		if call, ok := ast.Unparen(rhs).(*ast.CallExpr); ok && x.startsSpan(call, c) {
			started = true
			x.ctxSpansFromCall(call, c, w, depth+1)
		}
	}
	if started {
		return
	}
	for _, rhs := range srcs {
		x.ctxSpans(rhs, c, w, depth+1)
	}
	if !isParam(obj, c) {
		if len(srcs) == 0 {
			w.unresolved = true
		}
		return
	}
	// The context that each caller passes.
	fn, _ := info.Defs[c.decl.Name].(*types.Func)
	i := paramIndex(obj, c)
	if fn == nil || i < 0 || len(x.callSites[fn]) == 0 || !x.onlyCalledDirectly(fn) {
		w.unresolved = true
		return
	}
	for _, cs := range x.callSites[fn] {
		if i >= len(cs.call.Args) {
			w.unresolved = true
			continue
		}
		x.ctxSpans(cs.call.Args[i], cs.c, w, depth+1)
	}
}

func (x *extractor) ctxSpansFromCall(call *ast.CallExpr, c fnCtx, w *ctxWalk, depth int) {
	info := c.info()
	fn := callee(info, call)
	if wr, ok := x.wrappers[fn]; ok {
		if wr.nameAt >= len(call.Args) {
			w.unresolved = true
		} else if n, ok := constString(info, call.Args[wr.nameAt]); ok {
			w.names[n] = true
		} else {
			w.unresolved = true
		}
		return
	}
	if isMethod(fn, tracePkg, "Start") && len(call.Args) >= 2 {
		if n, ok := constString(info, call.Args[1]); ok {
			w.names[n] = true
		} else {
			w.unresolved = true
		}
		return
	}
	// A module function that returns a context: follow what it returns.
	if fc, ours := x.funcs[fn]; ours {
		if w.seen[fn] {
			return
		}
		w.seen[fn] = true
		res := fn.Type().(*types.Signature).Results()
		found := false
		for ri := 0; ri < res.Len(); ri++ {
			if !isContext(res.At(ri).Type()) {
				continue
			}
			found = true
			ast.Inspect(fc.decl.Body, func(n ast.Node) bool {
				if _, lit := n.(*ast.FuncLit); lit {
					return false
				}
				if r, ok := n.(*ast.ReturnStmt); ok {
					if len(r.Results) == res.Len() {
						x.ctxSpans(r.Results[ri], fc, w, depth+1)
					} else {
						w.unresolved = true
					}
				}
				return true
			})
		}
		if !found {
			w.unresolved = true
		}
		return
	}
	switch {
	case isFunc(fn, "context", "Background", "TODO"),
		isFunc(fn, tracePkg, "ContextWithSpanContext", "ContextWithRemoteSpanContext"):
		// No span, or a span that does not record attributes.
		return
	case isFunc(fn, tracePkg, "ContextWithSpan"):
		w.unresolved = true
		return
	}
	// context.WithValue(ctx, ...) and similar keep the span of the first
	// context argument.
	for _, a := range call.Args {
		if isContext(info.TypeOf(a)) {
			x.ctxSpans(a, c, w, depth+1)
			return
		}
	}
	// A context from outside the module, such as r.Context(), carries no
	// span of this module.
}

// ---------- extraction ----------

func (x *extractor) extract() {
	var starts, fromCtx, creates, resources []callSite
	x.eachFunc(func(c fnCtx) {
		info := c.info()
		ast.Inspect(c.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn := callee(info, call)
			if _, ours := x.funcs[fn]; ours && !isMethodExpr(info, call) {
				x.callSites[fn] = append(x.callSites[fn], callSite{call, c})
			}
			switch {
			case isMethod(fn, tracePkg, "Start") && len(call.Args) >= 2:
				starts = append(starts, callSite{call, c})
			case isFunc(fn, tracePkg, "SpanFromContext"):
				fromCtx = append(fromCtx, callSite{call, c})
			case isMethod(fn, metricPkg) && instruments[fn.Name()] && len(call.Args) >= 1:
				creates = append(creates, callSite{call, c})
			case fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == resourcePkg && recvOf(fn) == nil:
				resources = append(resources, callSite{call, c})
			}
			return true
		})
	})

	for _, cr := range creates {
		x.metric(cr.call, cr.c, callee(cr.c.info(), cr.call).Name())
	}
	for _, st := range starts {
		x.start(st)
	}
	for fn, w := range x.wrappers {
		if !x.onlyCalledDirectly(fn) {
			x.gap(w.start.Pos(), "span wrapper "+fn.Name()+" is called in a way that the generator does not see")
		}
	}
	x.eachFunc(x.wrapperCalls)
	for _, st := range fromCtx {
		x.fromContext(st)
	}
	x.eachFunc(x.fieldSpanCalls)
	x.applyRenames()
	x.metricUses()
	// Resource attributes describe the process, and the page does not list
	// them.
	x.quietly(func() {
		for _, r := range resources {
			k := newCollector(x, map[string]*Attr{})
			for _, a := range r.call.Args {
				k.keysIn(a, r.c)
			}
		}
	})
	x.checkReached()
	x.ref.Gaps = append(x.ref.Gaps, x.current...)
}

func isMethodExpr(info *types.Info, call *ast.CallExpr) bool {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	s := info.Selections[sel]
	return s != nil && s.Kind() == types.MethodExpr
}

// start handles a direct Tracer.Start call.
func (x *extractor) start(st callSite) {
	info := st.c.info()
	name, ok := constString(info, st.call.Args[1])
	if !ok {
		if obj, ok := varOf(info, st.call.Args[1]); ok && st.c.decl.Name != nil && isParam(obj, st.c) {
			fn := info.Defs[st.c.decl.Name].(*types.Func)
			_, twice := x.wrappers[fn]
			if i := paramIndex(obj, st.c); i >= 0 && !twice {
				x.wrappers[fn] = wrapper{st.call, st.c, i}
				return
			}
		}
		x.gap(st.call.Pos(), "span name is not a constant")
		return
	}
	s := x.span(name)
	k := newCollector(x, s.Attrs)
	for _, a := range st.call.Args[2:] {
		k.keysIn(a, st.c)
	}
	x.identify(s, x.resolveScope(st.call.Fun.(*ast.SelectorExpr).X, st.c, 0), k.kind(st.call.Pos()), st.call.Pos())
	x.trackSpan(spanVars(st.call, st.c), st.c, s, k)
}

// wrapperCalls handles calls of span wrappers in c.
func (x *extractor) wrapperCalls(c fnCtx) {
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := callee(c.info(), call)
		w, ok := x.wrappers[fn]
		if !ok {
			return true
		}
		if w.nameAt >= len(call.Args) {
			x.gap(call.Pos(), "span name passed to "+fn.Name()+" is not resolved")
			return true
		}
		name, ok := constString(c.info(), call.Args[w.nameAt])
		if !ok {
			x.gap(call.Pos(), "span name passed to "+fn.Name()+" is not a constant")
			return true
		}
		s := x.span(name)
		k := newCollector(x, s.Attrs)
		unbind := k.bindCall(fn, call, c)
		for _, a := range w.start.Args[2:] {
			k.keysIn(a, w.c)
		}
		x.identify(s, x.resolveScope(w.start.Fun.(*ast.SelectorExpr).X, w.c, 0), k.kind(call.Pos()), call.Pos())
		x.trackSpan(spanVars(w.start, w.c), w.c, s, k)
		unbind()
		x.trackSpan(spanVars(call, c), c, s, k)
		return true
	})
}

// fromContext handles a span taken from a context: it follows the context
// back through static callers to the Start calls that put a span there.
// When it cannot follow every way, or finds no span, it collects nothing,
// and checkReached reports the calls that change the span.
func (x *extractor) fromContext(st callSite) {
	if len(st.call.Args) != 1 {
		return
	}
	w := &ctxWalk{names: map[string]bool{}, seen: map[any]bool{}}
	x.ctxSpans(st.call.Args[0], st.c, w, 0)
	if w.unresolved {
		return
	}
	for _, n := range sortedKeys(w.names) {
		s := x.ref.Spans[n]
		if s == nil {
			continue
		}
		k := newCollector(x, s.Attrs)
		x.trackSpan(spanVars(st.call, st.c), st.c, s, k)
		// trace.SpanFromContext(ctx).SetAttributes(...)
		ast.Inspect(st.c.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && ast.Unparen(sel.X) == st.call {
				x.spanMethod(sel.Sel.Name, call, st.c, s, k)
			}
			return true
		})
	}
}

func (x *extractor) applyRenames() {
	done := map[[2]string]bool{}
	for _, r := range x.renames {
		if done[[2]string{r.from, r.to}] {
			continue
		}
		done[[2]string{r.from, r.to}] = true
		if _, ok := x.ref.Spans[r.to]; ok {
			x.gap(r.at, "span renamed to "+r.to+", which is the name of another span")
			continue
		}
		from, to := x.ref.Spans[r.from], x.span(r.to)
		to.Kind, to.Scope, to.Attrs, to.ErrorStatus = from.Kind, from.Scope, from.Attrs, from.ErrorStatus
		to.RenamedFrom = r.from
		for f := range from.Files {
			to.Files[f] = true
		}
	}
}

// checkReached reports each call that builds an attribute, changes a span
// or records a measurement, and that the extractor did not follow to a
// span, a metric or a resource.
func (x *extractor) checkReached() {
	x.eachBody(func(body ast.Node, c fnCtx) {
		info := c.info()
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || x.reached[call] || builtinOrConversion(info, call) {
				return true
			}
			fn := callee(info, call)
			switch {
			case isMethod(fn, metricPkg, measurements...):
				x.gap(call.Pos(), "records a measurement on an instrument that the generator cannot trace to where it is created")
			case isMethod(fn, tracePkg, "SetAttributes", "SetName", "SetStatus") && isSpanType(recvOf(fn).Type()):
				if fn.Name() != "SetStatus" || setsError(info, call) {
					x.gap(call.Pos(), "changes a span that the generator cannot trace to its Start call")
				}
			case holdsKeyValue(info.TypeOf(call)):
				x.gap(call.Pos(), "builds an attribute that the generator cannot trace to a span or a metric")
			}
			return true
		})
	})
}

// setsError reports whether a SetStatus call can set the status Error.
func setsError(info *types.Info, call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	o, ok := objOf(info, call.Args[0]).(*types.Const)
	return !ok || o.Pkg() == nil || o.Pkg().Path() != codesPkg || o.Name() == "Error"
}

// ---------- metrics ----------

func (x *extractor) metric(call *ast.CallExpr, c fnCtx, instrument string) {
	info := c.info()
	name, ok := constString(info, call.Args[0])
	if !ok {
		x.gap(call.Pos(), "metric name is not a constant")
		return
	}
	m := &Metric{Name: name, Instrument: instrument, File: x.file(call.Pos()), Attrs: map[string]*Attr{}}
	if m.Scope = x.resolveScope(call.Fun.(*ast.SelectorExpr).X, c, 0); m.Scope == "" {
		x.gap(call.Pos(), "meter scope is not resolved")
	}
	k := newCollector(x, m.Attrs)
	for _, a := range call.Args[1:] {
		ast.Inspect(a, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				fn := callee(info, n)
				if isFunc(fn, metricPkg, "WithDescription", "WithUnit") && len(n.Args) == 1 {
					s, ok := constString(info, n.Args[0])
					if !ok {
						x.gap(n.Pos(), "metric description or unit is not a constant")
					}
					if fn.Name() == "WithUnit" {
						m.Unit = s
					} else {
						m.Desc = s
					}
				}
			// Callbacks: a func literal or a module function value.
			case *ast.FuncLit:
				x.observeCalls(n.Body, c, k)
				return false
			case *ast.SelectorExpr, *ast.Ident:
				if fn, ok := objOf(info, n.(ast.Expr)).(*types.Func); ok {
					if fc, ours := x.funcs[fn.Origin()]; ours {
						x.observeCalls(fc.decl.Body, fc, k)
					}
					return false
				}
			}
			return true
		})
	}
	if prev, dup := x.ref.Metrics[name]; dup {
		x.gap(call.Pos(), "metric "+name+" is created in more than one place, also in "+prev.File)
	} else {
		x.ref.Metrics[name] = m
	}
	// The variable or field that receives the instrument.
	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 || as.Rhs[0] != call {
			return true
		}
		if o := objOf(info, as.Lhs[0]); o != nil {
			x.instrObjs[o] = append(x.instrObjs[o], m)
		}
		return false
	})
}

// observeCalls reads the attributes of the observations in the body of an
// instrument callback.
func (x *extractor) observeCalls(body ast.Node, c fnCtx, k *collector) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && isMethod(callee(c.info(), call), metricPkg, "Observe") {
			x.reached[call] = true
			for _, a := range call.Args[1:] {
				k.keysIn(a, c)
			}
		}
		return true
	})
}

func (x *extractor) metricUses() {
	// Follow instruments into struct fields and variables.
	for changed := true; changed; {
		changed = false
		follow := func(c fnCtx, dst types.Object, src ast.Expr) {
			if dst == nil {
				return
			}
			for _, m := range x.instrObjs[objOf(c.info(), src)] {
				if !containsMetric(x.instrObjs[dst], m) {
					x.instrObjs[dst] = append(x.instrObjs[dst], m)
					changed = true
				}
			}
		}
		x.eachBody(func(body ast.Node, c fnCtx) {
			ast.Inspect(body, func(n ast.Node) bool {
				eachFieldValue(n, c.info(), func(f *types.Var, v ast.Expr) { follow(c, f, v) })
				if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
					for i, l := range as.Lhs {
						if _, sel := l.(*ast.SelectorExpr); !sel {
							follow(c, objOf(c.info(), l), as.Rhs[i])
						}
					}
				}
				return true
			})
		})
	}
	x.eachBody(func(body ast.Node, c fnCtx) {
		info := c.info()
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn := callee(info, call)
			var instr ast.Expr
			var opts []ast.Expr
			switch {
			case isMethod(fn, metricPkg, "Add", "Record") && len(call.Args) >= 2:
				instr, opts = call.Fun.(*ast.SelectorExpr).X, call.Args[2:]
			case isMethod(fn, metricPkg, "ObserveInt64", "ObserveFloat64") && len(call.Args) >= 2:
				instr, opts = call.Args[0], call.Args[2:]
			default:
				return true
			}
			ms := x.instrObjs[objOf(info, instr)]
			if len(ms) > 0 {
				x.reached[call] = true
			}
			for _, m := range ms {
				k := newCollector(x, m.Attrs)
				for _, a := range opts {
					k.keysIn(a, c)
				}
			}
			return true
		})
	})
}

func containsMetric(list []*Metric, m *Metric) bool {
	for _, have := range list {
		if have == m {
			return true
		}
	}
	return false
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
