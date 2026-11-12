package surface

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// init registers the Go extractor. The registry and the contract live in surface.go, so a later language
// adds its own file and nothing else.
func init() { register(GoExtractor{}) }

// GoExtractor inventories a Go file with go/ast.
type GoExtractor struct{}

// Language names the language this extractor reads.
func (GoExtractor) Language() string { return "go" }

// Extract inventories every function of the file that meets the change, methods included; a closure
// assigned inside an inventoried function is read as part of that function.
func (GoExtractor) Extract(src Source, want Want) ([]Function, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src.Path, src.Content, 0)
	if err != nil {
		return nil, err
	}
	var functions []Function
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		first := fset.Position(fn.Pos()).Line
		last := fset.Position(fn.End()).Line
		if !want.Any(first, last) {
			continue
		}
		functions = append(functions, Function{
			File: src.Path,
			Name: fn.Name.Name,
			Line: first,
			Rows: inventory(fset, src.Path, fn),
		})
	}
	return functions, nil
}

// taint is one input the function reads, and every name it was assigned to inside the function.
type taint struct {
	label  string
	origin string
	first  int
	names  map[string]bool
}

// guard is one check seen in the function and the names its condition mentions.
type guard struct {
	line  int
	text  string
	names map[string]bool
}

// goAnalysis is one function's inventory while it is being built.
type goAnalysis struct {
	fset   *token.FileSet
	path   string
	fn     *ast.FuncDecl
	fnLine int
	taints map[string]*taint
	guards []guard
	rows   []Row
}

// inventory reads one function: first the taints settle, then one pass collects the guards and another
// looks for sinks, so a check written after a use still guards the sink it precedes.
func inventory(fset *token.FileSet, path string, fn *ast.FuncDecl) []Row {
	a := &goAnalysis{fset: fset, path: path, fn: fn, fnLine: fset.Position(fn.Pos()).Line, taints: map[string]*taint{}}
	a.seed()
	// Three rounds because a name can be assigned before the value that taints it is named: the flow a
	// reader follows in one direction needs a couple of passes to settle in the other.
	for pass := 0; pass < 3; pass++ {
		a.propagate(fn.Body.List)
	}
	a.collect(fn.Body.List)
	a.emit(fn.Body.List)
	return a.rows
}

// seed makes every parameter an input. An exported function's parameter is public because anyone can call
// it; an unexported one's is internal. A receiver is not a parameter, and results are not inputs.
func (a *goAnalysis) seed() {
	origin := OriginInternal
	if ast.IsExported(a.fn.Name.Name) {
		origin = OriginPublic
	}
	for _, field := range a.fn.Type.Params.List {
		text := a.text(field.Type)
		for _, name := range field.Names {
			label := name.Name
			if text != "" {
				label = name.Name + " " + text
			}
			a.bind(name.Name, a.newTaint(label, origin, a.fnLine))
		}
	}
}

// newTaint records one input.
func (a *goAnalysis) newTaint(label, origin string, line int) *taint {
	return &taint{label: label, origin: origin, first: line, names: map[string]bool{}}
}

// bind says that name now carries the taint, which is how a derived value keeps its origin.
func (a *goAnalysis) bind(name string, t *taint) {
	if name == "" || name == "_" {
		return
	}
	a.taints[name] = t
	t.names[name] = true
}

// propagate follows assignments, declarations, range variables and nested closures until every name a
// tainted expression reached is known.
func (a *goAnalysis) propagate(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		a.propagateStmt(stmt)
	}
}

func (a *goAnalysis) propagateStmt(stmt ast.Stmt) {
	switch n := stmt.(type) {
	case nil:
		return
	case *ast.AssignStmt:
		a.assign(n)
	case *ast.DeclStmt:
		a.declare(n)
	case *ast.BlockStmt:
		a.propagate(n.List)
	case *ast.IfStmt:
		a.propagateStmt(n.Init)
		a.propagateStmt(n.Body)
		a.propagateStmt(n.Else)
	case *ast.ForStmt:
		a.propagateStmt(n.Init)
		a.propagateStmt(n.Body)
	case *ast.RangeStmt:
		if t := a.taintOf(n.X); t != nil {
			if key, ok := n.Key.(*ast.Ident); ok {
				a.bind(key.Name, t)
			}
			if value, ok := n.Value.(*ast.Ident); ok {
				a.bind(value.Name, t)
			}
		}
		a.propagateStmt(n.Body)
	case *ast.SwitchStmt:
		a.propagateStmt(n.Init)
		a.propagateStmt(n.Body)
	case *ast.TypeSwitchStmt:
		a.propagateStmt(n.Init)
		a.propagateStmt(n.Body)
	case *ast.SelectStmt:
		a.propagateStmt(n.Body)
	case *ast.CaseClause:
		a.propagate(n.Body)
	case *ast.CommClause:
		a.propagate(n.Body)
	case *ast.LabeledStmt:
		a.propagateStmt(n.Stmt)
	}
	// A closure assigned inside the function reads the same parameters, so its body is part of it.
	ast.Inspect(stmt, func(node ast.Node) bool {
		lit, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		a.propagate(lit.Body.List)
		return false
	})
}

// assign binds every left-hand name to the taint its right-hand side carries.
func (a *goAnalysis) assign(n *ast.AssignStmt) {
	for i, left := range n.Lhs {
		rhs := rightHandSide(n, i)
		if rhs == nil {
			continue
		}
		t := a.taintOf(rhs)
		if t == nil {
			continue
		}
		if id, ok := left.(*ast.Ident); ok {
			a.bind(id.Name, t)
		}
	}
}

// rightHandSide pairs a left-hand name with its value, including the n-returns-in-one-call shape.
func rightHandSide(n *ast.AssignStmt, i int) ast.Expr {
	switch {
	case len(n.Rhs) == len(n.Lhs):
		return n.Rhs[i]
	case len(n.Rhs) == 1:
		return n.Rhs[0]
	}
	return nil
}

// declare binds var and const names with a value.
func (a *goAnalysis) declare(n *ast.DeclStmt) {
	gen, ok := n.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range value.Names {
			if i >= len(value.Values) {
				continue
			}
			if t := a.taintOf(value.Values[i]); t != nil {
				a.bind(name.Name, t)
			}
		}
	}
}

// taintOf returns the input an expression carries, through assignments, concatenation, sprintf, joins,
// slicing, conversions and the string operations that pass a value along.
func (a *goAnalysis) taintOf(expr ast.Expr) *taint {
	switch e := expr.(type) {
	case nil:
		return nil
	case *ast.Ident:
		return a.taints[e.Name]
	case *ast.ParenExpr:
		return a.taintOf(e.X)
	case *ast.UnaryExpr:
		return a.taintOf(e.X)
	case *ast.StarExpr:
		return a.taintOf(e.X)
	case *ast.SliceExpr:
		return a.taintOf(e.X)
	case *ast.TypeAssertExpr:
		return a.taintOf(e.X)
	case *ast.BinaryExpr:
		if t := a.taintOf(e.X); t != nil {
			return t
		}
		return a.taintOf(e.Y)
	case *ast.IndexExpr:
		if t := a.taintOf(e.X); t != nil {
			return t
		}
		return a.sourceOf(expr)
	case *ast.SelectorExpr:
		if t := a.taintOf(e.X); t != nil {
			return t
		}
		return a.sourceOf(expr)
	case *ast.CompositeLit:
		for _, element := range e.Elts {
			if t := a.taintOf(element); t != nil {
				return t
			}
		}
	case *ast.CallExpr:
		if t := a.callTaint(e); t != nil {
			return t
		}
		for _, arg := range e.Args {
			if t := a.taintOf(arg); t != nil {
				return t
			}
		}
		return a.sourceOf(expr)
	}
	return nil
}

// callTaint reads the calls that either pass a taint along (a conversion, a builder) or introduce one
// (a source the caller of this function does not control).
func (a *goAnalysis) callTaint(call *ast.CallExpr) *taint {
	if isConversion(call) {
		for _, arg := range call.Args {
			if t := a.taintOf(arg); t != nil {
				return t
			}
		}
		return nil
	}
	return a.sourceOf(call)
}

// isConversion reports a type conversion such as string(b) or []byte(s), which only renames a value.
func isConversion(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.ArrayType, *ast.StarExpr, *ast.MapType, *ast.ChanType:
		return true
	case *ast.Ident:
		switch fun.Name {
		case "string", "byte", "rune", "bool", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
			return true
		}
	}
	return false
}

// sourceOf names the in-body sources: the process arguments, the environment, a request, a reader, a file
// read from a variable path, and a decoded message.
func (a *goAnalysis) sourceOf(expr ast.Expr) *taint {
	segments := chain(expr)
	if len(segments) == 0 {
		return nil
	}
	last := segments[len(segments)-1]
	switch {
	case strings.Join(segments, ".") == "os.Args":
		return a.source(expr, OriginCLI)
	case has(segments, "os") && (last == "Getenv" || last == "LookupEnv"):
		return a.source(expr, OriginEnv)
	case has(segments, "flag") && (last == "Args" || last == "Arg"):
		return a.source(expr, OriginCLI)
	case has(segments, "URL") || has(segments, "Form") || has(segments, "Header"):
		return a.source(expr, OriginHTTP)
	case last == "FormValue" || last == "PostFormValue" || last == "PathValue" || last == "Cookie":
		return a.source(expr, OriginHTTP)
	case last == "Body":
		return a.source(expr, OriginHTTP)
	case last == "ReadAll" || last == "ReadBytes" || last == "ReadString" || last == "ReadFull":
		if strings.Contains(a.text(expr), "Body") {
			return a.source(expr, OriginHTTP)
		}
		return a.source(expr, OriginReader)
	case has(segments, "os") && last == "ReadFile" && !isLiteral(callArg(expr, 0)):
		return a.source(expr, OriginFile)
	case last == "Unmarshal" || last == "Decode":
		return a.decoded(expr)
	}
	return nil
}

// source records an in-body input with the expression as its label: `r.URL.Query().Get("cmd")` says more
// to a reader than the name it was assigned to.
func (a *goAnalysis) source(expr ast.Expr, origin string) *taint {
	return a.newTaint(a.text(expr), origin, a.line(expr))
}

// decoded records the target of a decoder: the message the attacker shaped, now a value in this function.
func (a *goAnalysis) decoded(expr ast.Expr) *taint {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return nil
	}
	target := call.Args[len(call.Args)-1]
	if unary, ok := target.(*ast.UnaryExpr); ok {
		target = unary.X
	}
	if _, ok := target.(*ast.Ident); !ok {
		return nil
	}
	t := a.source(expr, OriginDecode)
	a.bind(a.text(target), t)
	return t
}

// collect records every guard the function contains. Guards are matched to sinks by line, so collecting
// them before the sinks are found keeps a check written after a use in front of the sink it precedes.
func (a *goAnalysis) collect(stmts []ast.Stmt) {
	ast.Inspect(&ast.BlockStmt{List: stmts}, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.IfStmt:
			a.noteIfGuard(n)
		case *ast.CallExpr:
			a.noteValidator(n)
		}
		return true
	})
}

// noteIfGuard records a condition whose body leaves the function: that is the shape of a check.
func (a *goAnalysis) noteIfGuard(n *ast.IfStmt) {
	if !terminates(n.Body) {
		return
	}
	names := a.mentioned(n.Cond)
	if len(names) == 0 {
		return
	}
	a.guards = append(a.guards, guard{line: a.line(n.Cond), text: a.text(n.Cond), names: names})
}

// noteValidator records a call that cleans a value. filepath.Clean alone is not a boundary — it turns
// "../../etc/passwd" into a path that is still outside the root — so the prefix check that makes it safe is
// the if-guard this records separately.
func (a *goAnalysis) noteValidator(call *ast.CallExpr) {
	segments := chain(call.Fun)
	if len(segments) == 0 {
		return
	}
	last := segments[len(segments)-1]
	if segments[0] == "filepath" && last == "Clean" {
		return
	}
	if !isValidator(last) {
		return
	}
	names := map[string]bool{}
	for _, arg := range call.Args {
		for name := range a.mentioned(arg) {
			names[name] = true
		}
	}
	if len(names) == 0 {
		return
	}
	a.guards = append(a.guards, guard{line: a.line(call), text: a.text(call), names: names})
}

// emit finds the sinks each input reaches, and the case-only comparisons that decide access.
func (a *goAnalysis) emit(stmts []ast.Stmt) {
	ast.Inspect(&ast.BlockStmt{List: stmts}, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			a.sinkCall(n)
		case *ast.AssignStmt:
			a.noteAccumulation(n)
		case *ast.ForStmt:
			a.noteLoop(n)
		case *ast.IfStmt:
			a.noteCaseComparison(n)
		}
		return true
	})
}

// sinkCall reports every argument a sink rule considers dangerous and that carries an input.
func (a *goAnalysis) sinkCall(call *ast.CallExpr) {
	segments := chain(call.Fun)
	if len(segments) == 0 {
		return
	}
	for _, rule := range sinkCatalog {
		if !rule.matches(segments, call, a.text) {
			continue
		}
		for i := rule.From; i < len(call.Args); i++ {
			t := a.taintOf(call.Args[i])
			if t == nil {
				continue
			}
			a.add(t, rule.Kind, a.text(call.Fun), a.line(call))
		}
	}
}

// noteLoop reports a loop whose bound is an input: the caller decides how long this runs.
func (a *goAnalysis) noteLoop(n *ast.ForStmt) {
	if n.Cond == nil {
		return
	}
	if t := a.taintOf(n.Cond); t != nil {
		a.add(t, SinkLoop, "for", a.line(n))
	}
}

// noteAccumulation reports money-like arithmetic fed by an input, the sink a rounding defect hides in.
func (a *goAnalysis) noteAccumulation(n *ast.AssignStmt) {
	if n.Tok != token.ADD_ASSIGN || len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return
	}
	target, ok := n.Lhs[0].(*ast.Ident)
	if !ok || !a.moneyLike(target.Name) {
		return
	}
	if t := a.taintOf(n.Rhs[0]); t != nil {
		a.add(t, SinkAccumulation, target.Name+" +=", a.line(n))
	}
}

// moneyLike reports a total whose float arithmetic drifts, by its declared type or by its name.
func (a *goAnalysis) moneyLike(name string) bool {
	lower := strings.ToLower(name)
	for _, word := range []string{"amount", "price", "total", "sum", "balance", "cost", "fee", "cents", "money", "budget", "charge", "credit", "debit"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return a.declaredFloat(name)
}

// declaredFloat reports a name declared float64 or float32 in this function.
func (a *goAnalysis) declaredFloat(name string) bool {
	found := false
	ast.Inspect(a.fn.Body, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		typeName := a.text(value.Type)
		if typeName != "float64" && typeName != "float32" {
			return true
		}
		for _, declared := range value.Names {
			if declared.Name == name {
				found = true
			}
		}
		return true
	})
	return found
}

// noteCaseComparison reports a comparison that only folds case: "Admin" == strings.ToLower(role) hides
// locale and Unicode assumptions behind a check that looks like validation.
func (a *goAnalysis) noteCaseComparison(n *ast.IfStmt) {
	ast.Inspect(n.Cond, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		segments := chain(call.Fun)
		if len(segments) == 0 || segments[0] != "strings" {
			return true
		}
		if !isCaseCall(segments[len(segments)-1]) {
			return true
		}
		for _, arg := range call.Args {
			if t := a.taintOf(arg); t != nil {
				a.add(t, SinkCase, a.text(call.Fun), a.line(call))
			}
		}
		return true
	})
}

// add records one row, with the guard that stands between the input's first use and this line.
func (a *goAnalysis) add(t *taint, kind, call string, line int) {
	seen := a.guardFor(t, line)
	a.rows = append(a.rows, Row{
		File:         a.path,
		Function:     a.fn.Name.Name,
		FunctionLine: a.fnLine,
		Input:        t.label,
		Origin:       t.origin,
		Sink:         kind,
		SinkCall:     call,
		SinkLine:     line,
		Guard:        seen,
		Guarded:      seen != "",
	})
}

// guardFor returns the guard text between an input's first use and a sink, preferring the closest one, or
// an empty string when nothing between them mentions the value.
func (a *goAnalysis) guardFor(t *taint, line int) string {
	text, at := "", -1
	for _, g := range a.guards {
		if g.line < t.first || g.line > line || g.line < at {
			continue
		}
		if !shares(g.names, t.names) {
			continue
		}
		text, at = g.text, g.line
	}
	return text
}

// mentioned names every identifier and selector root an expression mentions, so a check can be matched to
// the value it checks.
func (a *goAnalysis) mentioned(expr ast.Expr) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(expr, func(node ast.Node) bool {
		switch e := node.(type) {
		case *ast.Ident:
			names[e.Name] = true
		case *ast.SelectorExpr:
			if root, ok := e.X.(*ast.Ident); ok {
				names[root.Name] = true
			}
		}
		return true
	})
	return names
}

// chain renders a callee as the dotted segments a rule matches: os.Open, db.Query, r.URL.Query.Get.
// Arguments are dropped, because a rule cares which call it is, not what it was given.
func chain(expr ast.Expr) []string {
	switch e := expr.(type) {
	case *ast.Ident:
		return []string{e.Name}
	case *ast.ParenExpr:
		return chain(e.X)
	case *ast.SelectorExpr:
		base := chain(e.X)
		if len(base) == 0 {
			return nil
		}
		return append(base, e.Sel.Name)
	case *ast.IndexExpr:
		return chain(e.X)
	case *ast.CallExpr:
		return chain(e.Fun)
	}
	return nil
}

// text renders an expression the way the report prints it, on one line.
func (a *goAnalysis) text(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf strings.Builder
	if err := printer.Fprint(&buf, a.fset, expr); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

// line answers the 1-based line of a node.
func (a *goAnalysis) line(node ast.Node) int { return a.fset.Position(node.Pos()).Line }

// terminates reports a body that leaves the function: a return, a branch, a panic or a fatal exit.
func terminates(body *ast.BlockStmt) bool {
	stopped := false
	ast.Inspect(body, func(node ast.Node) bool {
		if stopped {
			return false
		}
		switch e := node.(type) {
		case *ast.ReturnStmt, *ast.BranchStmt:
			stopped = true
			return false
		case *ast.CallExpr:
			switch calleeName(e) {
			case "panic", "Exit", "Fatal", "Fatalf", "Fatalln", "Panic":
				stopped = true
				return false
			}
		}
		return true
	})
	return stopped
}

// calleeName answers the last name of a call's callee.
func calleeName(call *ast.CallExpr) string {
	segments := chain(call.Fun)
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

// callArg answers one argument of a call expression, or nil.
func callArg(expr ast.Expr, index int) ast.Expr {
	call, ok := expr.(*ast.CallExpr)
	if !ok || index >= len(call.Args) {
		return nil
	}
	return call.Args[index]
}

// isLiteral reports a string or number written in the source, which no caller decides.
func isLiteral(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.BasicLit:
		return true
	}
	return false
}

// shares reports whether two name sets meet.
func shares(a, b map[string]bool) bool {
	for name := range a {
		if b[name] {
			return true
		}
	}
	return false
}

// has reports whether a chain contains a segment.
func has(segments []string, want string) bool {
	for _, segment := range segments {
		if segment == want {
			return true
		}
	}
	return false
}
