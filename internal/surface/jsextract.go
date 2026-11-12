package surface

import (
	"sort"
	"strings"
)

// init registers the JavaScript extractor for both extensions the language ships under.
func init() {
	register(JSExtractor{})
	registerAs(JSExtractor{}, "typescript")
}

// JSExtractor inventories a JavaScript or TypeScript file with a lexer instead of a parser. It sees calls,
// assignments and literals; it does not see types, scopes or closures, which is what makes its rows
// heuristic.
type JSExtractor struct{}

// Language names the language this extractor reads.
func (JSExtractor) Language() string { return "javascript" }

// Fidelity says how the file was read: a lexer, so a row is a promise to check rather than a verdict.
func (JSExtractor) Fidelity() string { return FidelityHeuristic }

// Extract inventories every top-level function the file declares, the methods of its classes, and the named
// arrow functions bound at the top level.
func (JSExtractor) Extract(src Source, want Want) ([]Function, error) {
	toks := lexJS(src.Content)
	index := jsIndex{toks: toks, depth: depths(toks), stmts: statements(toks, false)}
	index.modules = jsModuleBindings(toks)
	index.classes = jsClassBodies(toks, index.depth)
	index.functions = jsFunctions(index)
	var functions []Function
	for _, fn := range index.functions {
		first := fn.line
		last := toks[fn.header.to-1].line
		if !want.Any(first, last) {
			continue
		}
		functions = append(functions, Function{
			File: src.Path,
			Name: fn.name,
			Line: first,
			Rows: jsInventory(&index, src.Path, fn),
		})
	}
	return functions, nil
}

// jsIndex is one file read once: the tokens, their nesting, the statements, the modules the file imports names
// from, the class bodies, and the functions found in it.
type jsIndex struct {
	toks      []lexeme
	depth     []int
	stmts     []span
	modules   map[string]string
	classes   []jsClass
	functions []jsRange
}

// jsClass is one class body and whether the class is exported, because a method of an exported class can be
// called from outside.
type jsClass struct {
	body     span
	exported bool
}

// jsRange is one function the lexer found: its name, where it starts, its parameters, and the token ranges of
// its body and of the whole declaration.
type jsRange struct {
	name     string
	line     int
	exported bool
	params   []param
	body     span
	header   span
}

// jsFunctions finds the functions of a file: declarations, named arrows and class methods at the top level, or
// directly inside a class body. A callback nobody named is folded into the function it lives in, because that
// function's inputs are the ones a reader can trace.
func jsFunctions(index jsIndex) []jsRange {
	var found []jsRange
	for i := 0; i < len(index.toks); i++ {
		t := index.toks[i]
		if !jsTopLevel(index, i) {
			continue
		}
		switch {
		case t.kind == tokIdent && t.text == "function":
			if fn, ok := jsDeclaredFunction(index, i); ok {
				found = append(found, fn)
			}
		case t.text == "=>":
			if fn, ok := jsArrowFunction(index, i); ok {
				found = append(found, fn)
			}
		}
	}
	found = append(found, jsMethods(index)...)
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].line != found[j].line {
			return found[i].line < found[j].line
		}
		return found[i].header.from < found[j].header.from
	})
	return found
}

// jsTopLevel reports a token that belongs to the file's top level or directly to a class body: the two places
// a function somebody can call from outside is written.
func jsTopLevel(index jsIndex, at int) bool {
	if index.depth[at] == 0 {
		return true
	}
	for _, class := range index.classes {
		if at > class.body.from && at < class.body.to && index.depth[at] == index.depth[class.body.from+1] {
			return true
		}
	}
	return false
}

// jsClassBodies finds every class body and whether the class is exported.
func jsClassBodies(toks []lexeme, depth []int) []jsClass {
	var found []jsClass
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tokIdent || toks[i].text != "class" || depth[i] != 0 {
			continue
		}
		open := -1
		for j := i + 1; j < len(toks); j++ {
			if toks[j].text == "{" {
				open = j
				break
			}
			if toks[j].text == ";" {
				break
			}
		}
		if open < 0 {
			continue
		}
		close := matchClose(toks, open)
		if close < 0 {
			continue
		}
		found = append(found, jsClass{body: span{open, close}, exported: jsExported(toks, i)})
		i = close
	}
	return found
}

// jsDeclaredFunction reads `function name(args) {body}` at the keyword, and the shapes that carry a name
// around it: `export function`, `const f = function`.
func jsDeclaredFunction(index jsIndex, at int) (jsRange, bool) {
	toks := index.toks
	j := at + 1
	if j < len(toks) && toks[j].text == "*" {
		j++
	}
	name := ""
	if j < len(toks) && toks[j].kind == tokIdent {
		name, j = toks[j].text, j+1
	}
	if j >= len(toks) || toks[j].text != "(" {
		return jsRange{}, false
	}
	open := j
	close := matchClose(toks, open)
	if close < 0 {
		return jsRange{}, false
	}
	bodyOpen := jsBodyOpen(toks, close+1)
	if bodyOpen < 0 {
		return jsRange{}, false
	}
	bodyClose := matchClose(toks, bodyOpen)
	if bodyClose < 0 {
		return jsRange{}, false
	}
	exported := jsExported(toks, at)
	if name == "" {
		name = jsAssignedName(toks, at)
	}
	return jsRange{
		name:     name,
		line:     toks[at].line,
		exported: exported,
		params:   jsParams(toks, span{open + 1, close}),
		body:     span{bodyOpen + 1, bodyClose},
		header:   span{at, bodyClose + 1},
	}, true
}

// jsArrowFunction reads `(args) => body` and `arg => body`, with or without a TypeScript return type. A body
// that is an expression is read to the end of the statement it starts.
func jsArrowFunction(index jsIndex, at int) (jsRange, bool) {
	toks := index.toks
	prev := at - 1
	if prev < 0 {
		return jsRange{}, false
	}
	var params span
	start := prev
	switch {
	case toks[prev].text == ")":
		open := matchOpen(toks, prev)
		if open < 0 {
			return jsRange{}, false
		}
		params, start = span{open + 1, prev}, open
	case toks[prev].kind == tokIdent && prev >= 2 && toks[prev-1].text == ":":
		// `(args): Type => body`: the parameters end at the paren before the annotation.
		open := -1
		for k := prev - 2; k >= 0; k-- {
			if toks[k].text == ")" {
				open = matchOpen(toks, k)
				if open >= 0 {
					params, start = span{open + 1, k}, open
				}
				break
			}
			if anyOf(toks[k].text, []string{"=", ";", "{", "}", "(", ","}) {
				break
			}
		}
		if open < 0 {
			return jsRange{}, false
		}
	case toks[prev].kind == tokIdent:
		params, start = span{prev, prev + 1}, prev
	default:
		return jsRange{}, false
	}
	var body span
	if at+1 < len(toks) && toks[at+1].text == "{" {
		close := matchClose(toks, at+1)
		if close < 0 {
			return jsRange{}, false
		}
		body = span{at + 2, close}
		at = close
	} else {
		body = span{at + 1, statementOf(index.stmts, at).to}
		at = body.to - 1
	}
	name := jsAssignedName(toks, start)
	exported := jsExported(toks, start)
	return jsRange{
		name:     name,
		line:     toks[start].line,
		exported: exported,
		params:   jsParams(toks, params),
		body:     body,
		header:   span{start, at + 1},
	}, true
}

// jsMethods reads the methods of every class body: a name followed by a parameter list and a body.
func jsMethods(index jsIndex) []jsRange {
	var found []jsRange
	toks := index.toks
	for _, class := range index.classes {
		for i := class.body.from + 1; i < class.body.to; i++ {
			private := toks[i].text == "#"
			if private {
				i++
			}
			for i < class.body.to && anyOf(toks[i].text, []string{"static", "async", "get", "set", "public", "private", "protected", "readonly", "override", "abstract", "*"}) {
				i++
			}
			if i >= class.body.to || toks[i].kind != tokIdent || i+1 >= class.body.to || toks[i+1].text != "(" {
				continue
			}
			open := i + 1
			close := matchClose(toks, open)
			if close < 0 {
				continue
			}
			bodyOpen := jsBodyOpen(toks, close+1)
			if bodyOpen < 0 {
				continue
			}
			bodyClose := matchClose(toks, bodyOpen)
			if bodyClose < 0 {
				continue
			}
			name := toks[i].text
			found = append(found, jsRange{
				name:     name,
				line:     toks[i].line,
				exported: class.exported && !private && !strings.HasPrefix(name, "_"),
				params:   jsParams(toks, span{open + 1, close}),
				body:     span{bodyOpen + 1, bodyClose},
				header:   span{i, bodyClose + 1},
			})
			i = bodyClose
		}
	}
	return found
}

// jsBodyOpen answers the index of the `{` that opens a function body, skipping a TypeScript return type.
func jsBodyOpen(toks []lexeme, from int) int {
	for i := from; i < len(toks); i++ {
		if toks[i].text == "{" {
			return i
		}
		if anyOf(toks[i].text, []string{";", "}"}) {
			return -1
		}
	}
	return -1
}

// jsExported reports whether the declaration at at is exported: `export function`, `export default function`,
// or a member of `exports`/`module.exports`.
func jsExported(toks []lexeme, at int) bool {
	j := at - 1
	for j >= 0 && anyOf(toks[j].text, []string{"async", "default"}) {
		j--
	}
	if j < 0 {
		return false
	}
	if toks[j].text == "export" {
		return true
	}
	if j >= 3 && toks[j].text == "=" && (toks[j-1].kind == tokIdent) && anyOf(toks[j-2].text, []string{".", "?."}) && anyOf(toks[j-3].text, []string{"exports", "module"}) {
		return true
	}
	return false
}

// jsAssignedName answers the name a function expression is bound to, which is the name a reader calls it by.
func jsAssignedName(toks []lexeme, at int) string {
	j := at - 1
	for j >= 0 && anyOf(toks[j].text, []string{"async", "default"}) {
		j--
	}
	if j < 0 || j >= len(toks) {
		return "anonymous"
	}
	name := ""
	switch {
	case toks[j].text == "=" && j >= 1 && toks[j-1].kind == tokIdent:
		name = toks[j-1].text
	case toks[j].text == ":" && j >= 1 && toks[j-1].kind == tokIdent:
		name = toks[j-1].text
	case toks[j].text == ":" && j >= 1 && toks[j-1].kind == tokString:
		name = strings.Trim(toks[j-1].text, `"'`)
	}
	if name == "" {
		return "anonymous"
	}
	return name
}

// jsParams reads a parameter list: a plain parameter is its name and, when written, its type; a destructured
// pattern contributes every name it binds.
func jsParams(toks []lexeme, inside span) []param {
	var params []param
	for _, part := range splitSpan(toks, inside, ",") {
		for part.from < part.to && toks[part.from].text == "..." {
			part.from++
		}
		if part.empty() {
			continue
		}
		if anyOf(toks[part.from].text, []string{"{", "["}) {
			for _, name := range identsIn(toks, part) {
				if jsReserved(name) {
					continue
				}
				params = append(params, param{name: name, label: name})
			}
			continue
		}
		if toks[part.from].kind != tokIdent {
			continue
		}
		name := toks[part.from].text
		typeName, label := "", name
		for k := part.from + 1; k < part.to; k++ {
			if toks[k].text == "=" {
				break
			}
			if toks[k].text != ":" {
				continue
			}
			stop := part.to
			for m := k + 1; m < part.to; m++ {
				if toks[m].text == "=" {
					stop = m
					break
				}
			}
			typeName = strings.TrimPrefix(render(toks, span{k + 1, stop}), "?")
			if typeName != "" {
				label = name + " " + typeName
			}
			break
		}
		params = append(params, param{name: name, label: label, typeName: typeName})
	}
	return params
}

// jsReservedWords names the words a destructuring pattern mentions without binding anything.
var jsReservedWords = map[string]bool{
	"true": true, "false": true, "null": true, "undefined": true, "this": true, "of": true, "in": true,
	"async": true, "await": true, "new": true, "typeof": true, "return": true,
}

// jsReserved reports one of them.
func jsReserved(name string) bool { return jsReservedWords[name] }

// jsModuleBindings records which local names came from which module, so `execSync(...)` is read as
// child_process.execSync when the file destructured it.
func jsModuleBindings(toks []lexeme) map[string]string {
	bindings := map[string]string{}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.kind == tokIdent && t.text == "require" && i+2 < len(toks) && toks[i+1].text == "(" && toks[i+2].kind == tokString:
			module := unquote(toks[i+2].text)
			if i >= 3 && toks[i-1].text == "=" && toks[i-2].kind == tokIdent {
				bindings[toks[i-2].text] = module
			}
		case t.kind == tokIdent && t.text == "from" && i+1 < len(toks) && toks[i+1].kind == tokString:
			module := unquote(toks[i+1].text)
			if i >= 1 && toks[i-1].kind == tokIdent {
				bindings[toks[i-1].text] = module
			}
			if i >= 1 && toks[i-1].text == "}" {
				if open := matchOpen(toks, i-1); open >= 0 {
					for k := open + 1; k < i-1; k++ {
						if toks[k].kind == tokIdent {
							bindings[toks[k].text] = module
						}
					}
				}
			}
		}
	}
	return bindings
}

// unquote strips the quotes a string token keeps so the report can print it.
func unquote(text string) string {
	return strings.Trim(text, "'\"`")
}

// jsAnalysis is one function's inventory while it is being built.
type jsAnalysis struct {
	*flow
	index  *jsIndex
	fn     jsRange
	sizes  map[string]bool
	nested []span
}

// jsInventory reads one function: the parameters settle first, then the flow, the guards and the sinks, in that
// order, so a check written after a use still guards the sink it precedes.
func jsInventory(index *jsIndex, path string, fn jsRange) []Row {
	a := &jsAnalysis{flow: newFlow(path, fn.name, fn.line), index: index, fn: fn, sizes: map[string]bool{}}
	a.nested = jsNested(index.functions, fn)
	a.seed()
	for pass := 0; pass < 3; pass++ {
		a.propagate()
	}
	a.collectGuards()
	a.emit()
	return a.rows
}

// jsNested names the functions declared inside this one, whose code belongs to them, not to the parent.
func jsNested(all []jsRange, fn jsRange) []span {
	var found []span
	for _, other := range all {
		if other.header.from >= fn.body.from && other.header.to <= fn.body.to {
			found = append(found, other.header)
		}
	}
	return found
}

// skipped reports a token inside a nested function, which its own inventory measures.
func (a *jsAnalysis) skipped(at int) bool {
	for _, part := range a.nested {
		if at >= part.from && at < part.to {
			return true
		}
	}
	return false
}

// body answers the function's statement spans that this inventory owns.
func (a *jsAnalysis) body() []span {
	var found []span
	for _, stmt := range a.index.stmts {
		if stmt.from < a.fn.body.from || stmt.to > a.fn.body.to || a.skipped(stmt.from) {
			continue
		}
		found = append(found, stmt)
	}
	return found
}

// seed makes every parameter an input: public when anyone can call the function, internal otherwise. A
// request, a session or a logger is a handle, not data, so it is not an input; what it is asked for is.
func (a *jsAnalysis) seed() {
	origin := OriginInternal
	if a.fn.exported {
		origin = OriginPublic
	}
	for _, param := range a.fn.params {
		if isHandle(param.name, param.typeName) {
			continue
		}
		a.bind(param.name, a.newTaint(param.label, origin, a.fn.line))
	}
}

// propagate follows declarations, assignments, destructuring and for-of bindings until every name the input
// reached is known. Three rounds, because a name can be assigned before the value that taints it is named.
func (a *jsAnalysis) propagate() {
	for _, stmt := range a.body() {
		a.propagateStatement(stmt)
	}
}

// propagateStatement binds the names one statement assigns.
func (a *jsAnalysis) propagateStatement(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind == tokIdent && anyOf(first.text, []string{"const", "let", "var"}) {
		a.declare(stmt)
		return
	}
	if first.kind == tokIdent && anyOf(first.text, []string{"for", "while"}) {
		a.forStatement(stmt)
		return
	}
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].kind != tokIdent || i+1 >= stmt.to {
			continue
		}
		if !anyOf(toks[i+1].text, []string{"=", "+=", "-="}) {
			continue
		}
		a.bindAssigned(toks[i].text, span{i + 2, stmt.to})
	}
}

// declare binds every declarator of a const/let/var statement.
func (a *jsAnalysis) declare(stmt span) {
	for _, part := range splitSpan(a.index.toks, span{stmt.from + 1, stmt.to}, ",") {
		a.declarator(part)
	}
}

// declarator binds one declarator's names to the taint its value carries.
func (a *jsAnalysis) declarator(part span) {
	if part.empty() {
		return
	}
	toks := a.index.toks
	eq := -1
	for i := part.from; i < part.to; i++ {
		if toks[i].text == "=" {
			eq = i
			break
		}
	}
	names := a.patternNames(span{part.from, cond(eq, part.to)})
	value := span{part.to, part.to}
	switch {
	case eq >= 0:
		value = span{eq + 1, part.to}
	default:
		for i := part.from; i < part.to; i++ {
			if anyOf(toks[i].text, []string{"of", "in"}) {
				value = span{i + 1, part.to}
				break
			}
		}
	}
	if value.empty() {
		return
	}
	t := a.taintOf(value)
	if t == nil {
		return
	}
	for _, name := range names {
		if isVerdictName(name) {
			a.bindVerdict(name, t)
			continue
		}
		a.bind(name, t)
		a.noteSize(name, value)
		a.noteBuilt(name, a.builtValue(value))
	}
}

// forStatement follows the bindings a loop header creates: the range a for-of walks and the counter a C-style
// loop starts from.
func (a *jsAnalysis) forStatement(stmt span) {
	toks := a.index.toks
	open := -1
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text == "(" {
			open = i
			break
		}
	}
	if open < 0 {
		return
	}
	close := matchClose(toks, open)
	if close < 0 {
		close = stmt.to
	}
	inside := span{open + 1, close}
	clauses := splitSpan(toks, inside, ";")
	if len(clauses) == 0 {
		return
	}
	init := clauses[0]
	if init.empty() {
		return
	}
	if toks[init.from].kind == tokIdent && anyOf(toks[init.from].text, []string{"const", "let", "var"}) {
		for _, part := range splitSpan(toks, span{init.from + 1, init.to}, ",") {
			a.declarator(part)
		}
		return
	}
	for i := init.from; i < init.to; i++ {
		if toks[i].kind == tokIdent && i+1 < init.to && anyOf(toks[i+1].text, []string{"=", "+=", "-="}) {
			a.bindAssigned(toks[i].text, span{i + 2, init.to})
		}
	}
}

// bindAssigned binds one assigned name, keeping a call's verdict out of the names a guard can speak for.
func (a *jsAnalysis) bindAssigned(name string, value span) {
	t := a.taintOf(value)
	if t == nil {
		return
	}
	if isVerdictName(name) {
		a.bindVerdict(name, t)
		return
	}
	a.bind(name, t)
	a.noteSize(name, value)
	a.noteBuilt(name, a.builtValue(value))
}

// patternNames answers the names a declaration pattern binds: the identifiers before the value, minus the
// words that bind nothing.
func (a *jsAnalysis) patternNames(part span) []string {
	var names []string
	for i := part.from; i < part.to && i < len(a.index.toks); i++ {
		t := a.index.toks[i]
		if t.kind != tokIdent || jsReserved(t.text) {
			continue
		}
		if i+1 < part.to && a.index.toks[i+1].text == "(" {
			continue
		}
		if i >= 1 && anyOf(a.index.toks[i-1].text, []string{".", "?."}) {
			continue
		}
		names = append(names, t.text)
	}
	return names
}

// noteSize remembers a name holding a size the function computed from a collection, because a loop or an
// allocation that size is proportional and not amplification.
func (a *jsAnalysis) noteSize(name string, value span) {
	if a.proportional(value) {
		a.sizes[name] = true
	}
}

// taintOf returns the input an expression carries, through names, property reads and the in-body sources.
func (a *jsAnalysis) taintOf(part span) *taint {
	for i := part.from; i < part.to && i < len(a.index.toks); i++ {
		t := a.index.toks[i]
		if t.kind == tokIdent && a.taints[t.text] != nil {
			return a.taints[t.text]
		}
	}
	return a.sourceOf(part)
}

// sourceOf names the inputs a function reads for itself: a request field, the process arguments, the
// environment, a decoded body, a file read from a variable path.
func (a *jsAnalysis) sourceOf(part span) *taint {
	toks := a.index.toks
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].kind != tokIdent {
			continue
		}
		if module := a.index.modules[toks[i].text]; module != "" && isFileModule(module) {
			if i+1 < part.to && toks[i+1].text == "(" && isFileRead(toks[i].text) {
				return a.source(tight(toks, span{i, part.to}), OriginFile, toks[i].line)
			}
		}
		if i+2 >= part.to || !anyOf(toks[i+1].text, []string{".", "?."}) || toks[i+2].kind != tokIdent {
			continue
		}
		head, property := toks[i].text, toks[i+2].text
		origin := ""
		switch {
		case anyOf(head, []string{"req", "request", "ctx", "event", "args"}):
			if anyOf(property, []string{"query", "params", "body", "headers", "url", "path", "cookies", "files", "data", "rawBody", "pathParameters", "queryStringParameters"}) {
				origin = OriginHTTP
			}
		case head == "process" && property == "argv":
			origin = OriginCLI
		case head == "process" && property == "env":
			origin = OriginEnv
		case head == "JSON" && property == "parse":
			origin = OriginDecode
		case anyOf(head, jsFileOwners) && isFileRead(property):
			origin = OriginFile
		}
		if origin != "" {
			return a.source(tight(toks, span{i, part.to}), origin, toks[i].line)
		}
	}
	return nil
}

// source records an in-body input with the expression as its label, which says more to a reader than the name
// it was assigned to.
func (a *jsAnalysis) source(label, origin string, line int) *taint {
	return a.newTaint(label, origin, line)
}

// tight renders a property chain without spaces, because `req.query.cmd` reads as the expression a reviewer
// would write.
func tight(toks []lexeme, part span) string {
	var built strings.Builder
	stop := part.to
	for i := part.from; i < part.to; i++ {
		if !anyOf(toks[i].text, []string{".", "?.", "(", ")", "[", "]", "`", "${", "}"}) && toks[i].kind != tokIdent && toks[i].kind != tokNumber {
			continue
		}
		if anyOf(toks[i].text, []string{"(", ")"}) {
			stop = i
			break
		}
		built.WriteString(toks[i].text)
	}
	if built.Len() == 0 {
		return render(toks, span{part.from, stop})
	}
	return built.String()
}

// collectGuards records every check the function contains, so a check written after a use still stands in
// front of the sink it precedes.
func (a *jsAnalysis) collectGuards() {
	for _, stmt := range a.body() {
		a.guardStatement(stmt)
	}
}

// guardStatement records the two shapes a check takes: a condition that leaves the function, and a call that
// cleans the value it was handed.
func (a *jsAnalysis) guardStatement(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind == tokIdent && anyOf(first.text, []string{"if", "while"}) && stmt.from+1 < stmt.to && toks[stmt.from+1].text == "(" {
		open := stmt.from + 1
		close := matchClose(toks, open)
		// A condition that only asks whether the value is there says nothing about what it contains, so it is
		// not a check the reader can attack.
		if close >= 0 && a.leaves(a.blockAfter(stmt, close)) && !presenceOnly(toks, span{open + 1, close}) {
			a.noteGuard(span{open + 1, close}, first.line)
		}
	}
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].kind != tokIdent || !isHeuristicValidator(toks[i].text) || i+1 >= stmt.to || toks[i+1].text != "(" {
			continue
		}
		close := matchClose(toks, i+1)
		if close < 0 {
			continue
		}
		a.noteGuard(span{i, close + 1}, toks[i].line)
	}
}

// blockAfter answers the tokens a header's check says what to do about: the block it opens, or the rest of the
// statement when the check is written on one line — `if (!inside(p)) throw ...` guards p exactly as loudly.
func (a *jsAnalysis) blockAfter(stmt span, headerClose int) span {
	if stmt.to > stmt.from && a.index.toks[stmt.to-1].text == "{" {
		if close := matchClose(a.index.toks, stmt.to-1); close >= 0 {
			return span{stmt.to, close}
		}
	}
	return span{headerClose + 1, stmt.to}
}

// leaves reports a body that leaves the function: a throw, a return, an abort, or a break that drops the
// caller out of the loop.
func (a *jsAnalysis) leaves(part span) bool {
	return containsAny(a.index.toks, part, "throw", "return", "abort", "exit", "break", "continue")
}

// noteGuard records one check and the names it mentions, with the body of the function it calls when this file
// defines it: the sentence a reader has to attack is what the check does.
func (a *jsAnalysis) noteGuard(part span, line int) {
	names := mentions(a.index.toks, part, nil)
	if len(names) == 0 {
		return
	}
	a.guards = append(a.guards, guard{line: line, text: render(a.index.toks, part), body: a.guardBody(part), names: names})
}

// guardBody answers the decisive expression of the first function this file defines that a guard calls. One call
// deep is the promise; a helper the guard calls in turn is not followed.
func (a *jsAnalysis) guardBody(part span) string {
	toks := a.index.toks
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].text != "(" {
			continue
		}
		c := calleeAt(toks, i)
		if c.name() == "" {
			continue
		}
		if body := a.decidedBy(c.name()); body != "" {
			return body
		}
	}
	return ""
}

// decidedBy answers the expression a named function of this file decides with: its first return value, or the
// condition of its first if. A name this file does not define answers nothing and the guard prints as written.
func (a *jsAnalysis) decidedBy(name string) string {
	toks := a.index.toks
	for _, fn := range a.index.functions {
		if fn.name != name {
			continue
		}
		for _, stmt := range a.index.stmts {
			if stmt.from < fn.body.from || stmt.to > fn.body.to {
				continue
			}
			first := toks[stmt.from]
			if first.kind != tokIdent {
				continue
			}
			switch first.text {
			case "return":
				return clip(strings.TrimSpace(strings.TrimSuffix(render(toks, span{stmt.from + 1, stmt.to}), ";")), 100)
			case "if":
				if condition := conditionOf(toks, stmt); !condition.empty() {
					return clip(render(toks, condition), 100)
				}
			}
		}
	}
	return ""
}

// conditionOf answers the parenthesised condition of an if statement.
func conditionOf(toks []lexeme, stmt span) span {
	for i := stmt.from; i < stmt.to && i < len(toks); i++ {
		if toks[i].text != "(" {
			continue
		}
		if close := matchClose(toks, i); close >= 0 && close < stmt.to {
			return span{i + 1, close}
		}
		break
	}
	return span{stmt.to, stmt.to}
}

// emit finds the sinks each input reaches.
func (a *jsAnalysis) emit() {
	for _, stmt := range a.body() {
		a.propertySink(stmt)
		a.callSinks(stmt)
		a.templateSinks(stmt)
		a.loopSink(stmt)
		a.accumulationSink(stmt)
		a.returnSink(stmt)
	}
}

// returnSink reports the sink a caller receives: an exported function that returns a path, markup, a query or a
// URL built from an input hands the dangerous value on, and the open happens somewhere this probe cannot see. A
// value handed back unchanged is not built here and is not a row.
func (a *jsAnalysis) returnSink(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind != tokIdent || first.text != "return" || !a.fn.exported {
		return
	}
	value := trimEnd(toks, span{stmt.from + 1, stmt.to}, ";")
	built := a.builtValue(value)
	if built.kind == "" {
		return
	}
	for _, t := range taintsOf(identsIn(toks, value), a.taints) {
		a.addReturned(t, built, first.line)
	}
}

// jsPathModules are the modules whose calls assemble or rewrite a path.
var jsPathModules = []string{"path", "node:path", "path/posix", "path/win32", "upath"}

// jsPathNames are the path operations whose result a caller treats as a location.
var jsPathNames = []string{"join", "resolve", "normalize", "dirname", "basename", "relative", "toNamespacedPath"}

// jsURLModules are the modules whose calls build a URL, and jsURLNames the operations that do.
var jsURLModules = []string{"url", "node:url"}

var jsURLNames = []string{"resolve", "format", "parse"}

// jsConversions are the calls that only rename a value, so a returned one keeps the kind it was built with.
var jsConversions = []string{"String", "Number", "Boolean", "toString", "toFixed", "trim"}

// builtValue answers how a returned expression was built: what its literal bytes say, the path or URL operation
// it performs, or the name it was assigned. A value handed to an arbitrary helper is not followed, because what
// that helper does to it is not in this function.
func (a *jsAnalysis) builtValue(part span) builtValue {
	toks := a.index.toks
	part = trimEnd(toks, trimParens(toks, part), ";")
	if part.empty() || part.from >= len(toks) {
		return builtValue{}
	}
	if part.to == part.from+1 && toks[part.from].kind == tokIdent {
		return a.built[toks[part.from].text]
	}
	if built := a.literalBuiltValue(part); built.kind != "" {
		return built
	}
	if built := a.callBuiltValue(part); built.kind != "" {
		return built
	}
	if a.convertedOnly(part) {
		return a.mentionedBuilt(part)
	}
	return builtValue{}
}

// callBuiltValue answers the path or URL call the returned expression is: the outermost one, because a path an
// argument was built from is not what the value is.
func (a *jsAnalysis) callBuiltValue(part span) builtValue {
	c, ok := outermostCall(a.index.toks, part)
	if !ok || c.name() == "" {
		return builtValue{}
	}
	owner := c.owner()
	if owner == "" {
		owner = a.index.modules[c.name()]
	}
	if anyOf(owner, jsPathModules) && anyOf(c.name(), jsPathNames) {
		return builtValue{kind: SinkReturnedPath, call: joined(c)}
	}
	if anyOf(owner, jsURLModules) && anyOf(c.name(), jsURLNames) {
		return builtValue{kind: SinkReturnedURL, call: joined(c)}
	}
	if anyOf(c.name(), []string{"URL", "URLSearchParams", "WebSocket"}) &&
		c.from >= 1 && a.index.toks[c.from-1].text == "new" {
		return builtValue{kind: SinkReturnedURL, call: joined(c)}
	}
	return builtValue{}
}

// literalBuiltValue answers what the strings and template text of a returned expression say they build.
func (a *jsAnalysis) literalBuiltValue(part span) builtValue {
	toks := a.index.toks
	for i := part.from; i < part.to && i < len(toks); i++ {
		switch toks[i].kind {
		case tokText:
			if built := builtFromLiteral(strings.TrimSpace(toks[i].text)); built.kind != "" {
				return built
			}
		case tokString:
			if built := builtFromLiteral(unquote(toks[i].text)); built.kind != "" {
				return built
			}
		}
	}
	return builtValue{}
}

// convertedOnly reports an expression that only renames or concatenates a value: a conversion call or an
// addition. Anything else is an operation this probe does not follow.
func (a *jsAnalysis) convertedOnly(part span) bool {
	toks := a.index.toks
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].text == "+" {
			return true
		}
		if toks[i].text == "(" {
			c := calleeAt(toks, i)
			if !anyOf(c.name(), jsConversions) {
				return false
			}
		}
	}
	return false
}

// mentionedBuilt answers the first built name a returned expression mentions.
func (a *jsAnalysis) mentionedBuilt(part span) builtValue {
	for _, name := range identsIn(a.index.toks, part) {
		if built := a.built[name]; built.kind != "" {
			return built
		}
	}
	return builtValue{}
}

// outermostCall answers the call whose result an expression is: the paren at the expression's own depth, closed
// at its end. A call inside an argument built a value this one consumed, not the value itself.
func outermostCall(toks []lexeme, part span) (callee, bool) {
	depth, at := 0, -1
	for i := part.from; i < part.to && i < len(toks); i++ {
		switch {
		case opensGroup(toks[i].text):
			if toks[i].text == "(" && depth == 0 {
				at = i
			}
			depth++
		case closesGroup(toks[i].text):
			if depth > 0 {
				depth--
			}
		}
	}
	if at < 0 || matchClose(toks, at) != part.to-1 {
		return callee{}, false
	}
	found := calleeAt(toks, at)
	return found, found.name() != ""
}

// trimParens drops the parentheses a returned expression is wrapped in.
func trimParens(toks []lexeme, part span) span {
	for part.to-part.from >= 2 && toks[part.from].text == "(" && matchClose(toks, part.from) == part.to-1 {
		part = span{part.from + 1, part.to - 1}
	}
	return part
}

// jsSinkCatalog is the JavaScript catalog: the callee a sink answers to, the object or module that owns it,
// and which argument carries the value. It is data for the same reason the Go catalog is: a reviewer can read
// it, and a rule is added without touching the flow analysis.
var jsSinkCatalog = []callRule{
	{Kind: SinkFile, Owners: append(jsFileOwners, "fs", "node:fs", "fs/promises", "node:fs/promises"), Names: jsFileNames},
	{Kind: SinkExec, Owners: []string{"child_process", "node:child_process", "childProcess", "cp"}, Names: []string{"exec", "execSync", "spawn", "spawnSync", "execFile", "execFileSync", "fork"}},
	{Kind: SinkSQL, Names: []string{"query", "execute", "executemany", "prepare", "raw"}},
	{Kind: SinkNetwork, Owners: []string{"http", "node:http", "https", "node:https", "axios", "got", "superagent", "undici", "request", "net", "dns", "tls"}, Names: []string{"get", "post", "put", "patch", "delete", "head", "request", "fetch"}},
	{Kind: SinkNetwork, Owners: []string{"", "window", "globalThis"}, Names: []string{"fetch"}},
	{Kind: SinkNetwork, Ctor: true, Names: []string{"URL", "URLSearchParams", "WebSocket"}},
	{Kind: SinkTemplate, Owners: []string{"res", "response", "reply", "replyRaw", "ctx"}, Names: []string{"send", "write", "end", "render", "sendFile", "html"}},
	{Kind: SinkTemplate, Owners: []string{"document"}, Names: []string{"write", "writeln"}},
	{Kind: SinkTemplate, Names: []string{"insertAdjacentHTML"}},
	{Kind: SinkDecode, Owners: []string{"Object", "Reflect"}, Names: []string{"assign", "defineProperty", "defineProperties", "setPrototypeOf"}},
	{Kind: SinkDecode, Owners: []string{"_", "lodash", "R", "deepmerge"}, Names: []string{"merge", "mergeAll", "mergeWith", "defaultsDeep", "extend", "assignIn", "set", "setWith", "update"}},
	{Kind: SinkRegex, Ctor: true, Names: []string{"RegExp"}},
	{Kind: SinkTimezone, Ctor: true, Names: []string{"Date"}},
	{Kind: SinkTimezone, Owners: []string{"Intl"}, Names: []string{"DateTimeFormat", "NumberFormat", "RelativeTimeFormat", "Locale"}},
	{Kind: SinkTimezone, Names: []string{"toLocaleDateString", "toLocaleTimeString", "toLocaleString"}},
	{Kind: SinkCase, Whole: true, Names: []string{"toLowerCase", "toUpperCase", "toLocaleLowerCase", "toLocaleUpperCase", "localeCompare", "normalize"}},
	{Kind: SinkAllocation, Ctor: true, Names: []string{"Array"}},
	{Kind: SinkAllocation, Names: []string{"Array", "repeat", "padStart", "padEnd", "alloc", "allocUnsafe", "fill"}},
	{Kind: SinkAllocation, Owners: []string{"Array"}, Names: []string{"from", "of"}},
}

// jsFileOwners are the names a filesystem read is written on.
var jsFileOwners = []string{"fs", "fsp", "fsPromises", "promises"}

// jsFileNames are the filesystem calls a path reaches.
var jsFileNames = []string{"readFileSync", "readFile", "writeFileSync", "writeFile", "appendFileSync", "appendFile",
	"openSync", "open", "unlinkSync", "unlink", "rmSync", "rm", "rmdirSync", "rmdir", "mkdirSync", "mkdir",
	"existsSync", "exists", "statSync", "stat", "lstatSync", "lstat", "readdirSync", "readdir",
	"createReadStream", "createWriteStream", "copyFileSync", "copyFile", "renameSync", "rename",
	"truncateSync", "truncate", "chmodSync", "chmod", "accessSync", "access", "realpathSync", "realpath",
	"readlinkSync", "readlink", "opendirSync", "opendir"}

// isFileModule reports a module that serves the filesystem.
func isFileModule(module string) bool {
	return anyOf(module, append(jsFileOwners, "fs", "node:fs", "fs/promises", "node:fs/promises"))
}

// isFileRead reports a filesystem call that reads or writes a path.
func isFileRead(name string) bool { return anyOf(name, jsFileNames) }

// callSinks reports every argument a sink rule considers dangerous and that carries an input.
func (a *jsAnalysis) callSinks(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text != "(" || a.skipped(i) {
			continue
		}
		close := matchClose(toks, i)
		if close < 0 {
			continue
		}
		callee := calleeAt(toks, i)
		if callee.name() == "" {
			continue
		}
		isNew := callee.from >= 1 && toks[callee.from-1].text == "new"
		args := splitSpan(toks, span{i + 1, close}, ",")
		for _, rule := range jsSinkCatalog {
			if !rule.matches(callee, isNew, a.index.modules) {
				continue
			}
			if rule.Whole {
				a.wholeSink(stmt, rule.Kind, joined(callee))
				continue
			}
			for _, arg := range args[min(rule.From, len(args)):] {
				if rule.Kind == SinkAllocation && a.proportional(arg) {
					continue
				}
				if t := a.taintOf(arg); t != nil {
					a.add(t, rule.Kind, joined(callee), toks[i].line)
				}
			}
		}
	}
}

// wholeSink reports a sink the whole statement decides, which is how a case folding only counts when a
// comparison turns it into an access decision.
func (a *jsAnalysis) wholeSink(stmt span, kind, call string) {
	if !containsAny(a.index.toks, stmt, "===", "==", "!==", "!=", "includes", "indexOf", "startsWith", "endsWith", "while", "if") {
		return
	}
	for _, name := range identsIn(a.index.toks, stmt) {
		if t := a.taints[name]; t != nil {
			a.add(t, kind, call, a.index.toks[stmt.from].line)
		}
	}
}

// joined renders a callee the way the report names it.
func joined(c callee) string { return strings.Join(c.segments, ".") }

// propertySink reports the sinks written as a property write: markup assigned to innerHTML, and a computed key
// written into an object, which is the prototype-pollution shape. A spread is deliberately not one of them:
// `{ ...input }` copies own properties and can never reach a prototype, so a row there is a probe nobody can
// break.
func (a *jsAnalysis) propertySink(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		switch toks[i].text {
		case ".":
			if i+2 < stmt.to && anyOf(toks[i+1].text, []string{"innerHTML", "outerHTML"}) && toks[i+2].text == "=" {
				if t := a.taintOf(span{i + 3, stmt.to}); t != nil {
					a.add(t, SinkTemplate, toks[i+1].text, toks[i].line)
				}
			}
		case "[":
			close := matchClose(toks, i)
			if close < 0 || close+1 >= stmt.to || toks[close+1].text != "=" {
				continue
			}
			if t := a.taintOf(span{i + 1, close}); t != nil {
				a.add(t, SinkDecode, "["+render(toks, span{i + 1, close})+"] =", toks[close].line)
			}
		}
	}
}

// templateSinks reports a template literal that builds markup out of an input. A template a function returns is
// the returned row's promise, so it is reported there instead of twice.
func (a *jsAnalysis) templateSinks(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind == tokIdent && first.text == "return" {
		return
	}
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text != "`" {
			continue
		}
		end := templateEnd(toks, i)
		if end < 0 {
			continue
		}
		part := span{i, end + 1}
		if !hasMarkup(toks, part) {
			i = end
			continue
		}
		if t := a.taintOf(part); t != nil {
			a.add(t, SinkTemplate, "`...`", toks[i].line)
		}
		i = end
	}
}

// loopSink reports a loop whose bound is a scalar input. A loop that walks a collection already in memory, or
// stops at its length, is proportional: the caller cannot make it run longer than the data it handed over.
func (a *jsAnalysis) loopSink(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind != tokIdent || !anyOf(first.text, []string{"for", "while"}) {
		return
	}
	open := -1
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text == "(" {
			open = i
			break
		}
	}
	if open < 0 {
		return
	}
	cond := a.loopCondition(span{open + 1, stmt.to})
	if cond.empty() || a.proportional(cond) {
		return
	}
	if containsAny(toks, span{open + 1, stmt.to}, "of", "in") {
		return
	}
	if t := a.taintOf(cond); t != nil {
		a.add(t, SinkLoop, first.text, first.line)
	}
}

// loopCondition answers the part of a loop header that decides when it stops: the middle clause of a C-style
// loop, or the whole condition of a while.
func (a *jsAnalysis) loopCondition(inside span) span {
	parts := splitSpan(a.index.toks, inside, ";")
	if len(parts) >= 2 {
		return parts[1]
	}
	return inside
}

// proportional reports an expression bounded by the size of a collection the function already holds, so a loop
// or an allocation sized by it cannot amplify.
func (a *jsAnalysis) proportional(part span) bool {
	toks := a.index.toks
	if containsAny(toks, part, "length", "size") {
		return true
	}
	for _, name := range identsIn(toks, part) {
		if a.taints[name] == nil || a.sizes[name] {
			if a.taints[name] == nil {
				return true
			}
			return true
		}
	}
	return false
}

// accumulationSink reports money-like arithmetic fed by an input, the sink a rounding defect hides in.
func (a *jsAnalysis) accumulationSink(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].kind != tokIdent || i+1 >= stmt.to || !anyOf(toks[i+1].text, []string{"+=", "="}) {
			continue
		}
		if !moneyLike(toks[i].text) {
			continue
		}
		value := span{i + 2, stmt.to}
		if toks[i+1].text == "=" {
			// `sum = sum + value`: only accumulation when the name is added to itself.
			if !containsIdent(toks, value, toks[i].text) {
				continue
			}
		}
		if t := a.taintOf(value); t != nil {
			a.add(t, SinkAccumulation, toks[i].text+" +=", toks[i].line)
		}
	}
}

// containsIdent reports whether a span mentions one identifier.
func containsIdent(toks []lexeme, part span, name string) bool {
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].kind == tokIdent && toks[i].text == name {
			return true
		}
	}
	return false
}

// cond answers the first of two indexes when it is set, so a missing separator reads as an empty pattern.
func cond(index, fallback int) int {
	if index >= 0 {
		return index
	}
	return fallback
}
