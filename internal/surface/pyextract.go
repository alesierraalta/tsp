package surface

// init registers the Python extractor. Nothing here runs an interpreter: the file is read by indentation the
// way internal/findings/locate.go reads one, and its rows are marked heuristic because of it.
func init() { register(PyExtractor{}) }

// PyExtractor inventories a Python file with an indentation scan and a lexer.
type PyExtractor struct{}

// Language names the language this extractor reads.
func (PyExtractor) Language() string { return "python" }

// Fidelity says how the file was read: a scan, so a row is a promise to check rather than a verdict.
func (PyExtractor) Fidelity() string { return FidelityHeuristic }

// Extract inventories every def the indentation scan finds, methods included.
func (PyExtractor) Extract(src Source, want Want) ([]Function, error) {
	index := newPyIndex(src.Content)
	var functions []Function
	for _, fn := range index.functions {
		if !want.Any(fn.line, fn.last) {
			continue
		}
		functions = append(functions, Function{
			File: src.Path,
			Name: fn.name,
			Line: fn.line,
			Rows: pyInventory(index, src.Path, fn),
		})
	}
	return functions, nil
}

// pyIndex is one file read once: the tokens, the statements, the lines and their indentation, the names it
// imported, and the functions found in it.
type pyIndex struct {
	toks      []lexeme
	stmts     []span
	lines     []string
	indent    []int
	modules   map[string]string
	functions []pyFunction
}

// newPyIndex reads a file once for every function the inventory will need of it.
func newPyIndex(content []byte) *pyIndex {
	toks := lexPy(content)
	index := &pyIndex{
		toks:    toks,
		lines:   splitOnNewline(string(content)),
		modules: pyImports(toks),
	}
	index.stmts = statements(index.toks, true)
	index.indent = make([]int, len(index.lines))
	for i, line := range index.lines {
		index.indent[i] = pyIndent(line)
	}
	index.functions = pyFunctions(content, index.toks)
	return index
}

// splitOnNewline answers the file's lines, which is how a Python block is delimited, without the trailing
// empty line a final newline creates.
func splitOnNewline(body string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			lines = append(lines, body[start:i])
			start = i + 1
		}
	}
	if start < len(body) {
		lines = append(lines, body[start:])
	}
	return lines
}

// pyAnalysis is one function's inventory while it is being built.
type pyAnalysis struct {
	*flow
	index *pyIndex
	fn    pyFunction
	sizes map[string]bool
}

// pyInventory reads one function: the parameters settle first, then the flow, the guards and the sinks, so a
// check written after a use still guards the sink it precedes.
func pyInventory(index *pyIndex, path string, fn pyFunction) []Row {
	a := &pyAnalysis{flow: newFlow(path, fn.name, fn.line), index: index, fn: fn, sizes: map[string]bool{}}
	a.seed()
	for pass := 0; pass < 3; pass++ {
		a.propagate()
	}
	a.collectGuards()
	a.emit()
	return a.rows
}

// body answers the function's statements, which are the statements inside its token span.
func (a *pyAnalysis) body() []span {
	var found []span
	for _, stmt := range a.index.stmts {
		if stmt.from < a.fn.body.from || stmt.to > a.fn.body.to {
			continue
		}
		found = append(found, stmt)
	}
	return found
}

// seed makes every parameter an input, public unless the name says otherwise. A session, a cursor or a logger
// is a handle, not data, so it is not an input.
func (a *pyAnalysis) seed() {
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

// propagate follows assignments, unpacking, for-targets and with-targets until every name the input reached is
// known.
func (a *pyAnalysis) propagate() {
	for _, stmt := range a.body() {
		a.propagateStatement(stmt)
	}
}

// propagateStatement binds the names one statement assigns.
func (a *pyAnalysis) propagateStatement(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	switch {
	case first.kind == tokIdent && anyOf(first.text, []string{"for", "async"}):
		a.forStatement(stmt)
	case first.kind == tokIdent && first.text == "with":
		a.withStatement(stmt)
	}
	eq := a.assignAt(stmt)
	if eq < 0 {
		return
	}
	names := a.patternNames(span{stmt.from, eq})
	t := a.taintOf(span{eq + 1, stmt.to})
	if t == nil {
		return
	}
	for _, name := range names {
		if isVerdictName(name) {
			a.bindVerdict(name, t)
			continue
		}
		a.bind(name, t)
		a.noteSize(name, span{eq + 1, stmt.to})
	}
}

// assignAt answers the index of the assignment operator of a statement, or -1. A comparison is not an
// assignment, which is why `==` and `!=` are skipped.
func (a *pyAnalysis) assignAt(stmt span) int {
	for i := stmt.from; i < stmt.to; i++ {
		if anyOf(a.index.toks[i].text, []string{"=", ":="}) {
			return i
		}
	}
	return -1
}

// patternNames answers the names on the left of an assignment: one name, a tuple of them, or an annotated one.
func (a *pyAnalysis) patternNames(part span) []string {
	var names []string
	for i := part.from; i < part.to && i < len(a.index.toks); i++ {
		t := a.index.toks[i]
		if t.kind != tokIdent || pyKeywords[t.text] {
			continue
		}
		if i+1 < part.to && a.index.toks[i+1].text == "." {
			continue
		}
		names = append(names, t.text)
	}
	return names
}

// pyKeywords are the words a pattern mentions without binding anything.
var pyKeywords = map[string]bool{
	"self": true, "cls": true, "True": true, "False": true, "None": true, "and": true, "or": true,
	"not": true, "in": true, "is": true, "if": true, "else": true, "elif": true, "for": true, "while": true,
	"with": true, "as": true, "async": true, "await": true, "lambda": true, "return": true, "yield": true,
	"import": true, "from": true, "assert": true, "raise": true, "global": true, "nonlocal": true,
}

// forStatement binds the names a loop introduces. A range() counter is not tainted by the number that bounds it,
// because what the counter holds says nothing about how long the loop runs.
func (a *pyAnalysis) forStatement(stmt span) {
	toks := a.index.toks
	in := -1
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text == "in" {
			in = i
			break
		}
	}
	if in < 0 {
		return
	}
	value := span{in + 1, stmt.to}
	if a.isRange(value) {
		return
	}
	t := a.taintOf(value)
	if t == nil {
		return
	}
	for _, name := range a.patternNames(span{stmt.from + 1, in}) {
		a.bind(name, t)
	}
}

// withStatement follows `with open(path) as handle`, because what was read is an input in its own right.
func (a *pyAnalysis) withStatement(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].kind != tokIdent || toks[i].text != "as" || i+1 >= stmt.to || toks[i+1].kind != tokIdent {
			continue
		}
		name := toks[i+1].text
		value := span{stmt.from + 1, i}
		t := a.taintOf(value)
		if t == nil {
			continue
		}
		a.bind(name, a.newTaint(name, OriginReader, toks[i].line))
	}
}

// isRange reports a range() call, which is a bound rather than data.
func (a *pyAnalysis) isRange(part span) bool {
	toks := a.index.toks
	return part.to == part.from+2 && toks[part.from].text == "range"
}

// noteSize remembers a name holding a size the function computed from a collection.
func (a *pyAnalysis) noteSize(name string, value span) {
	if a.proportional(value) {
		a.sizes[name] = true
	}
}

// taintOf returns the input an expression carries, through names and the in-body sources.
func (a *pyAnalysis) taintOf(part span) *taint {
	for i := part.from; i < part.to && i < len(a.index.toks); i++ {
		t := a.index.toks[i]
		if t.kind == tokIdent && a.taints[t.text] != nil {
			return a.taints[t.text]
		}
	}
	return a.sourceOf(part)
}

// sourceOf names the inputs a Python function reads for itself: a request field, the command line, the
// environment, a decoded body, a file it opened.
func (a *pyAnalysis) sourceOf(part span) *taint {
	toks := a.index.toks
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].kind != tokIdent {
			continue
		}
		if i+2 < part.to && anyOf(toks[i+1].text, []string{".", "("}) {
			if origin := a.sourceOrigin(i, part); origin != "" {
				return a.newTaint(tight(toks, span{i, part.to}), origin, toks[i].line)
			}
		}
	}
	return nil
}

// sourceOrigin names where a property read or a call at index comes from.
func (a *pyAnalysis) sourceOrigin(at int, part span) string {
	toks := a.index.toks
	head := toks[at].text
	property := ""
	if toks[at+1].text == "." && at+2 < part.to && toks[at+2].kind == tokIdent {
		property = toks[at+2].text
	}
	switch {
	case anyOf(head, []string{"request", "req"}) && anyOf(property, []string{"args", "form", "json", "headers", "query_params", "path_params", "GET", "POST", "data", "body", "values", "cookies", "files", "raw_body"}):
		return OriginHTTP
	case head == "sys" && property == "argv":
		return OriginCLI
	case head == "os" && anyOf(property, []string{"environ", "getenv"}):
		return OriginEnv
	case head == "os" && property == "environ":
		return OriginEnv
	case head == "json" && property == "loads":
		return OriginDecode
	case head == "open" && toks[at+1].text == "(":
		return OriginFile
	case property == "read" || property == "read_text" || property == "read_bytes":
		return OriginReader
	}
	return ""
}

// collectGuards records every check the function contains.
func (a *pyAnalysis) collectGuards() {
	for _, stmt := range a.body() {
		a.guardStatement(stmt)
	}
}

// guardStatement records the two shapes a check takes: a condition whose block leaves the function, and a call
// that cleans the value it was handed.
func (a *pyAnalysis) guardStatement(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind == tokIdent && anyOf(first.text, []string{"if", "elif", "while"}) {
		if colon := a.colonOf(stmt); colon >= 0 {
			body := span{colon + 1, stmt.to}
			if body.empty() {
				body = a.blockAfter(stmt)
			}
			if a.leaves(body) {
				a.noteGuard(span{stmt.from + 1, colon}, first.line)
			}
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
		a.noteGuard(span{i + 1, close + 1}, toks[i].line)
	}
}

// colonOf answers the colon that ends a block header's condition, or -1.
func (a *pyAnalysis) colonOf(stmt span) int {
	for i := stmt.from; i < stmt.to; i++ {
		if a.index.toks[i].text == ":" {
			return i
		}
	}
	return -1
}

// blockAfter answers the tokens of the indented block a header starts, because that is where a Python check
// says what it does about the value.
func (a *pyAnalysis) blockAfter(header span) span {
	toks := a.index.toks
	line := toks[header.from].line - 1
	if line < 0 || line >= len(a.index.indent) {
		return span{header.to, header.to}
	}
	indent := a.index.indent[line]
	from, to := header.to, header.to
	started := false
	for i := header.to; i < len(toks); i++ {
		at := toks[i].line - 1
		if at < 0 || at >= len(a.index.indent) {
			continue
		}
		if a.index.indent[at] <= indent {
			to = i
			break
		}
		if !started {
			from, started = i, true
		}
		to = i + 1
	}
	return span{from, to}
}

// leaves reports a block that leaves the function: a raise, a return, an abort or an exit.
func (a *pyAnalysis) leaves(part span) bool {
	return containsAny(a.index.toks, part, "raise", "return", "abort", "exit", "continue", "break")
}

// noteGuard records one check and the names it mentions.
func (a *pyAnalysis) noteGuard(part span, line int) {
	names := mentions(a.index.toks, part, nil)
	if len(names) == 0 {
		return
	}
	a.guards = append(a.guards, guard{line: line, text: render(a.index.toks, part), names: names})
}

// emit finds the sinks each input reaches.
func (a *pyAnalysis) emit() {
	for _, stmt := range a.body() {
		a.callSinks(stmt)
		a.markupSink(stmt)
		a.loopSink(stmt)
		a.allocationSink(stmt)
		a.accumulationSink(stmt)
	}
}

// pySinkCatalog is the Python catalog: the callee a sink answers to, the module or object that owns it, and
// which argument carries the value.
var pySinkCatalog = []callRule{
	{Kind: SinkFile, Names: []string{"open"}},
	{Kind: SinkFile, Owners: []string{"io", "codecs", "gzip", "bz2", "lzma"}, Names: []string{"open"}},
	{Kind: SinkFile, Names: []string{"Path", "PurePath", "PosixPath", "WindowsPath"}},
	{Kind: SinkFile, Owners: []string{"os", "os.path", "shutil", "pathlib", "glob"}, Names: []string{"remove", "unlink", "rename", "replace", "mkdir", "makedirs", "rmdir", "removedirs", "stat", "listdir", "scandir", "walk", "rmtree", "copy", "copyfile", "copytree", "move", "readlink", "realpath", "abspath", "glob", "iglob", "open"}},
	{Kind: SinkFile, Names: []string{"read_text", "write_text", "read_bytes", "write_bytes", "iterdir"}},
	{Kind: SinkExec, Owners: []string{"os"}, Names: []string{"system", "popen", "spawnl", "spawnv", "execv", "execvp", "execl", "execle"}},
	{Kind: SinkExec, Owners: []string{"subprocess"}, Names: []string{"run", "call", "check_call", "check_output", "Popen", "getoutput", "getstatusoutput"}},
	{Kind: SinkExec, Owners: []string{"asyncio"}, Names: []string{"create_subprocess_exec", "create_subprocess_shell"}},
	{Kind: SinkExec, Names: []string{"eval", "exec", "compile"}},
	{Kind: SinkSQL, Names: []string{"execute", "executemany", "executescript", "query", "raw"}},
	{Kind: SinkSQL, Owners: []string{"sqlalchemy", ""}, Names: []string{"text"}},
	{Kind: SinkNetwork, Owners: []string{"requests", "httpx", "urllib", "urllib.request", "urllib2", "aiohttp", "session", "client", "s"}, Names: []string{"get", "post", "put", "patch", "delete", "head", "request", "urlopen", "open"}},
	{Kind: SinkNetwork, Names: []string{"urlopen"}},
	{Kind: SinkTemplate, Names: []string{"render_template_string", "mark_safe", "Markup", "HTML", "html"}},
	{Kind: SinkDecode, Owners: []string{"pickle", "cPickle", "dill", "marshal", "yaml", "jsonpickle"}, Names: []string{"loads", "load", "decode"}},
	{Kind: SinkRegex, Owners: []string{"re"}, Names: []string{"compile", "match", "search", "findall", "finditer", "sub", "subn", "fullmatch", "split"}},
	{Kind: SinkTimezone, Owners: []string{"pytz", "zoneinfo"}, Names: []string{"timezone", "ZoneInfo"}},
	{Kind: SinkTimezone, Owners: []string{"datetime"}, Names: []string{"now", "utcnow", "fromtimestamp", "utcfromtimestamp", "fromisoformat", "strptime", "combine"}},
	{Kind: SinkTimezone, Owners: []string{"time"}, Names: []string{"localtime", "gmtime", "mktime", "strptime", "asctime", "strftime"}},
	{Kind: SinkTimezone, Names: []string{"astimezone", "fromisoformat", "strptime"}},
	{Kind: SinkCase, Whole: true, Names: []string{"lower", "upper", "casefold", "title", "swapcase", "normalize", "translate"}},
	{Kind: SinkAllocation, Names: []string{"bytearray", "bytes", "list", "dict", "set", "array"}},
}

// callSinks reports every argument a sink rule considers dangerous and that carries an input.
func (a *pyAnalysis) callSinks(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text != "(" {
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
		args := splitSpan(toks, span{i + 1, close}, ",")
		for _, rule := range pySinkCatalog {
			if !rule.matches(callee, false, a.index.modules) {
				continue
			}
			if rule.Kind == SinkDecode && a.safeLoader(callee, args) {
				continue
			}
			if rule.Kind == SinkExec && callee.owner() == "subprocess" && !a.reachesShell(args) {
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

// reachesShell reports an exec call that actually reaches a shell: `shell=True`, or a command the function built
// as a string. A subprocess handed an argv list runs one program with the arguments it was given, so a value
// inside that list is not command injection and is not a row.
func (a *pyAnalysis) reachesShell(args []span) bool {
	toks := a.index.toks
	for _, arg := range args {
		if containsIdent(toks, arg, "shell") && containsIdent(toks, arg, "True") {
			return true
		}
	}
	if len(args) == 0 {
		return false
	}
	command := args[0]
	if containsAny(toks, command, "+", "%") || containsIdent(toks, command, "format") {
		return true
	}
	for i := command.from; i < command.to && i < len(toks); i++ {
		if toks[i].kind == tokText {
			return true
		}
	}
	return false
}

// safeLoader reports the yaml.load shape that asks for the safe constructor, which is not a deserialization
// sink: the whole point of the argument is that it refuses to build arbitrary objects.
func (a *pyAnalysis) safeLoader(c callee, args []span) bool {
	if c.name() != "load" && c.name() != "loads" {
		return false
	}
	for _, arg := range args {
		if containsIdent(a.index.toks, arg, "SafeLoader") || containsIdent(a.index.toks, arg, "CSafeLoader") {
			return true
		}
	}
	return false
}

// wholeSink reports a sink the whole statement decides, which is how a case folding only counts when a
// comparison turns it into an access decision.
func (a *pyAnalysis) wholeSink(stmt span, kind, call string) {
	if !containsAny(a.index.toks, stmt, "==", "!=", "in", "if", "while", "not", "sorted", "dict", "key") {
		return
	}
	for _, name := range identsIn(a.index.toks, stmt) {
		if t := a.taints[name]; t != nil {
			a.add(t, kind, call, a.index.toks[stmt.from].line)
		}
	}
}

// markupSink reports an f-string that builds markup out of an input, which is the shape a template injection
// reaches a page through.
func (a *pyAnalysis) markupSink(stmt span) {
	toks := a.index.toks
	if !hasMarkup(toks, stmt) {
		return
	}
	if t := a.taintOf(stmt); t != nil {
		a.add(t, SinkTemplate, "f\"...\"", toks[stmt.from].line)
	}
}

// loopSink reports a loop whose bound is a scalar input. A loop that walks a collection already in memory, or
// one bounded by its length, is proportional: the caller cannot make it run longer than the data it handed
// over.
func (a *pyAnalysis) loopSink(stmt span) {
	toks := a.index.toks
	first := toks[stmt.from]
	if first.kind != tokIdent || !anyOf(first.text, []string{"for", "while"}) {
		return
	}
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].kind != tokIdent || toks[i].text != "range" || i+1 >= stmt.to || toks[i+1].text != "(" {
			continue
		}
		close := matchClose(toks, i+1)
		if close < 0 {
			continue
		}
		inside := span{i + 1, close}
		for _, arg := range splitSpan(toks, inside, ",") {
			if a.proportional(arg) {
				continue
			}
			if t := a.taintOf(arg); t != nil {
				a.add(t, SinkLoop, "range", toks[i].line)
			}
		}
		return
	}
	if first.text == "while" {
		cond := span{stmt.from + 1, stmt.to}
		if colon := a.colonOf(stmt); colon >= 0 {
			cond = span{stmt.from + 1, colon}
		}
		if !containsAny(toks, cond, "len", "range", "of") && !a.proportional(cond) {
			if t := a.taintOf(cond); t != nil {
				a.add(t, SinkLoop, "while", first.line)
			}
		}
	}
}

// allocationSink reports a sequence multiplied by a scalar input, the memory-exhaustion shape Python writes as
// `[x] * n`.
func (a *pyAnalysis) allocationSink(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].text != "*" || i == stmt.from || i+1 >= stmt.to {
			continue
		}
		previous := toks[i-1]
		if previous.text != "]" && previous.kind != tokString && previous.kind != tokNumber {
			continue
		}
		value := span{i + 1, stmt.to}
		if a.proportional(value) {
			continue
		}
		if t := a.taintOf(value); t != nil {
			a.add(t, SinkAllocation, previous.text+" *", toks[i].line)
		}
	}
}

// proportional reports an expression bounded by the size of a collection the function already holds.
func (a *pyAnalysis) proportional(part span) bool {
	toks := a.index.toks
	if containsAny(toks, part, "len") {
		return true
	}
	found := false
	for _, name := range identsIn(toks, part) {
		t := a.taints[name]
		if t == nil {
			continue
		}
		if !a.sizes[name] {
			return false
		}
		found = true
	}
	return found
}

// accumulationSink reports money-like arithmetic fed by an input, the sink a rounding defect hides in.
func (a *pyAnalysis) accumulationSink(stmt span) {
	toks := a.index.toks
	for i := stmt.from; i < stmt.to; i++ {
		if toks[i].kind != tokIdent || i+1 >= stmt.to || !anyOf(toks[i+1].text, []string{"+=", "="}) {
			continue
		}
		if !moneyLike(toks[i].text) {
			continue
		}
		value := span{i + 2, stmt.to}
		if toks[i+1].text == "=" && !containsIdent(toks, value, toks[i].text) {
			continue
		}
		if t := a.taintOf(value); t != nil {
			a.add(t, SinkAccumulation, toks[i].text+" +=", toks[i].line)
		}
	}
}
