package surface

import "strings"

// This file is the vocabulary the JavaScript and Python extractors share with each other, and with nothing
// else: a lexeme stream instead of a parser, and the small questions a name-based flow analysis asks of one.
// The masking mirrors internal/findings/locate.go's quotedEnd and commentEnd — a literal or a comment is a
// lexeme of its own, so a sink spelled inside one is never read as a call — but the tokens are kept instead of
// thrown away, because the flow analysis has to walk them.

// tokKind says what a lexeme is: a name the analysis reads, punctuation it matches, or a literal whose text is
// data.
type tokKind int

const (
	tokIdent tokKind = iota
	tokNumber
	tokPunct
	// tokString is a quoted literal: no call name and no flow can hide in its text.
	tokString
	// tokText is the literal part of a template literal or a prefixed string: markup can live here, which is
	// why it is kept apart from tokString.
	tokText
	tokRegex
)

// lexeme is one lexical unit of a source file that has no parser. Offsets are bytes, lines are 1-based, so a
// row can point at the line a person opens.
type lexeme struct {
	kind       tokKind
	text       string
	start, end int
	line       int
}

// span is a half-open range of lexeme indices.
type span struct{ from, to int }

// empty reports a span with nothing in it.
func (s span) empty() bool { return s.to <= s.from }

// opensGroup reports a lexeme that opens a group: a paren, a bracket, a brace, or a template interpolation.
// The interpolation is a group because its closing brace has to be matched to something, or a template breaks
// the block it is written in.
func opensGroup(text string) bool {
	return anyOf(text, []string{"(", "[", "{", "${"})
}

// closesGroup reports a lexeme that closes one.
func closesGroup(text string) bool { return anyOf(text, []string{")", "]", "}"}) }

// matchClose answers the index of the lexeme closing the group opened at at, or -1. Groups of every kind are
// counted together, because a call inside a call still ends where its own paren does.
func matchClose(toks []lexeme, at int) int {
	if !opensGroup(toks[at].text) {
		return -1
	}
	depth := 0
	for i := at; i < len(toks); i++ {
		switch {
		case opensGroup(toks[i].text):
			depth++
		case closesGroup(toks[i].text):
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// depths answers, per lexeme, how many groups enclose it. A closing lexeme is counted inside the group it
// closes, so a statement can be ended by the lexeme that also closes its block.
func depths(toks []lexeme) []int {
	found := make([]int, len(toks))
	depth := 0
	for i, t := range toks {
		if closesGroup(t.text) {
			if depth > 0 {
				depth--
			}
		}
		found[i] = depth
		if opensGroup(t.text) {
			depth++
		}
	}
	return found
}

// joins names the tokens a line break does not end a statement after: an operator still wanting its
// right-hand side continues onto the next line, and so does a keyword that opens a clause.
// A definition starts a line of its own: it never continues the statement written above it, which is what a
// `return` followed by a `def` would otherwise be read as.
func joins(text string) bool {
	switch text {
	case ",", "=", "+=", "-=", "*=", "/=", "%=", "|=", "&=", "+", "-", "*", "/", "%", "&&", "||", "??",
		"?", ":", "=>", "(", "[", "{", ".", "?.", "==", "===", "!=", "!==", "<", ">", "<=", ">=", "&", "|",
		"^", "!", "~", "->", "\\", "return", "await", "yield", "new", "typeof", "in", "of", "not", "and",
		"or", "else", "elif", "if", "for", "while", "with", "except", "assert", "raise", "lambda", "from",
		"import", "as", "@":
		return true
	}
	return false
}

// statements splits a lexeme stream into the statements a person reads one per line: a line ends at a
// semicolon, at a line break the previous line does not continue, or when the group it lives in closes. A
// JavaScript block is delimited by braces, so its interior is split as well; a Python block is delimited by
// indentation, so one line is already one statement and python only adds the colon that ends a block header.
func statements(toks []lexeme, python bool) []span {
	return statementRange(toks, 0, len(toks), python)
}

// statementRange splits one region of a token stream, descending into the blocks inside it.
func statementRange(toks []lexeme, from, to int, python bool) []span {
	var found []span
	depth := 0
	start := from
	end := func(at int) {
		if at > start {
			found = append(found, span{start, at})
		}
		start = at
	}
	for i := from; i < to; i++ {
		t := toks[i]
		if opensGroup(t.text) {
			if t.text == "{" && depth == 0 && !python && jsBlockOpen(toks, i, start) {
				end(i + 1)
				close := matchClose(toks, i)
				if close < 0 || close > to {
					close = to
				}
				found = append(found, statementRange(toks, i+1, close, python)...)
				start = close + 1
				i = close
				continue
			}
			depth++
			continue
		}
		if closesGroup(t.text) {
			if depth > 0 {
				depth--
			}
			if depth == 0 && (i+1 >= to || toks[i+1].line > t.line) {
				end(i + 1)
			}
			continue
		}
		if depth != 0 {
			continue
		}
		if t.text == ";" || (python && t.text == ":") {
			end(i + 1)
			continue
		}
		if i > start && t.line > toks[i-1].line && !joins(toks[i-1].text) && !joins(t.text) {
			end(i)
		}
	}
	end(to)
	return found
}

// jsBlockOpen reports a `{` that opens a block instead of an object literal, which decides whether its
// interior is a sequence of statements or one value.
func jsBlockOpen(toks []lexeme, at, start int) bool {
	if at == 0 {
		return true
	}
	switch toks[at-1].text {
	case ")", "=>", "else", "try", "do", "finally":
		return true
	}
	for i := start; i < at; i++ {
		if anyOf(toks[i].text, []string{"class", "switch", "function"}) {
			return true
		}
		if anyOf(toks[i].text, []string{"=", ",", "[", "return"}) {
			return false
		}
	}
	return false
}

// statementOf answers the statement the lexeme at index belongs to, so a comparison or a guard can be read
// over the whole sentence instead of one lexeme.
func statementOf(stmts []span, at int) span {
	for _, stmt := range stmts {
		if at >= stmt.from && at < stmt.to {
			return stmt
		}
		if at < stmt.from {
			break
		}
	}
	return span{at, at + 1}
}

// splitSpan splits a lexeme range by a separator at its own level, which is how a call's arguments and a
// declaration's declarators are read.
func splitSpan(toks []lexeme, part span, sep string) []span {
	var found []span
	from := part.from
	for i := part.from; i < part.to; i++ {
		if opensGroup(toks[i].text) {
			if close := matchClose(toks, i); close >= 0 && close < part.to {
				i = close
			}
			continue
		}
		if toks[i].text == sep {
			if i > from {
				found = append(found, span{from, i})
			}
			from = i + 1
		}
	}
	if part.to > from {
		found = append(found, span{from, part.to})
	}
	return found
}

// identsIn collects the identifier tokens a span mentions: the names a guard or a sink argument is about.
func identsIn(toks []lexeme, part span) []string {
	var names []string
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].kind == tokIdent {
			names = append(names, toks[i].text)
		}
	}
	return names
}

// mentions reports the set of identifiers a span mentions, which is the shape guard matching needs.
func mentions(toks []lexeme, part span, skip map[string]bool) map[string]bool {
	names := map[string]bool{}
	for _, name := range identsIn(toks, part) {
		if skip[name] {
			continue
		}
		names[name] = true
	}
	return names
}

// render prints a span as one line of source-ish text, which is what the report shows for a guard or a sink.
func render(toks []lexeme, part span) string {
	var parts []string
	for i := part.from; i < part.to && i < len(toks); i++ {
		text := toks[i].text
		if toks[i].kind == tokText {
			text = strings.TrimSpace(text)
		}
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

// trimEnd drops the punctuation a statement span keeps at its end, so `return joined;` answers the value it
// returns instead of the semicolon that ends it.
func trimEnd(toks []lexeme, part span, texts ...string) span {
	for part.to > part.from && part.to <= len(toks) && anyOf(toks[part.to-1].text, texts) {
		part.to--
	}
	return part
}

// containsAny reports whether a span mentions one of the wanted tokens.
func containsAny(toks []lexeme, part span, wanted ...string) bool {
	for i := part.from; i < part.to && i < len(toks); i++ {
		if anyOf(toks[i].text, wanted) {
			return true
		}
	}
	return false
}

// callee is the dotted name of a call, the segment a rule matches, and where its receiver starts — enough to
// say `fs.readFileSync` and `res.send` are different promises.
type callee struct {
	segments []string
	// from is the index of the first lexeme of the name, so the receiver ends just before it.
	from int
	// index is the lexeme holding the last segment.
	end int
}

// name answers the last segment of a callee.
func (c callee) name() string {
	if len(c.segments) == 0 {
		return ""
	}
	return c.segments[len(c.segments)-1]
}

// owner answers the module or object the name belongs to, when one was written.
func (c callee) owner() string {
	if len(c.segments) < 2 {
		return ""
	}
	return c.segments[0]
}

// calleeAt reads the callee of the call whose opening paren is at index open.
func calleeAt(toks []lexeme, open int) callee {
	var found callee
	i := open - 1
	if i >= 0 && toks[i].kind == tokIdent {
		found.segments = []string{toks[i].text}
		found.end = i
		found.from = i
		i--
		for i >= 1 && (toks[i].text == "." || toks[i].text == "?.") && toks[i-1].kind == tokIdent {
			found.segments = append([]string{toks[i-1].text}, found.segments...)
			found.from = i - 1
			i -= 2
		}
		return found
	}
	// The receiver is an expression, not a name: foo()[i].bar(...) is reported by its last segment only.
	if i >= 0 && (toks[i].text == ")" || toks[i].text == "]") {
		if start := matchOpen(toks, i); start >= 0 && start >= 2 && (toks[start-1].text == "." || toks[start-1].text == "?.") && toks[start-2].kind == tokIdent {
			found.segments = []string{toks[start-2].text}
			found.from = start - 2
			found.end = start - 2
		}
	}
	return found
}

// matchOpen answers the index of the lexeme opening the group closed at close, or -1.
func matchOpen(toks []lexeme, close int) int {
	depth := 0
	for i := close; i >= 0; i-- {
		switch {
		case closesGroup(toks[i].text):
			depth++
		case opensGroup(toks[i].text):
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// templateEnd answers the index of the backtick closing the template literal opened at at, or -1. An
// interpolation is skipped whole, so a template nested inside one does not close the outer one.
func templateEnd(toks []lexeme, at int) int {
	for i := at + 1; i < len(toks); i++ {
		switch toks[i].text {
		case "`":
			return i
		case "${":
			depth := 1
			for i++; i < len(toks) && depth > 0; i++ {
				switch toks[i].text {
				case "${":
					depth++
				case "}":
					depth--
				}
			}
			i--
		}
	}
	return -1
}

// hasMarkup reports whether a template's literal text carries an HTML tag: the shape that makes a built
// string output markup rather than a message.
func hasMarkup(toks []lexeme, part span) bool {
	for i := part.from; i < part.to && i < len(toks); i++ {
		if toks[i].kind != tokText {
			continue
		}
		text := toks[i].text
		for j := 0; j+1 < len(text); j++ {
			if text[j] == '<' && (isMarkupStart(text[j+1]) || text[j+1] == '/') && strings.ContainsRune(text[j+1:], '>') {
				return true
			}
		}
	}
	return false
}

// isMarkupStart reports the first byte of a tag or attribute that makes a `<` markup.
func isMarkupStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '!'
}

// isIdentStart reports the first byte of an identifier in either language.
func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// isIdentPart reports a byte that continues an identifier.
func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// quotedEnd answers the offset just past the quoted literal opened at at, mirroring locate.go: an escape
// consumes the byte after it, so a delimiter inside a string never ends it early. An unterminated literal
// ends at the line break, which keeps one bad quote from swallowing the file.
func quotedEnd(src []byte, at int, multiline bool) int {
	quote := src[at]
	for i := at + 1; i < len(src); i++ {
		if src[i] == '\\' {
			i++
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		if src[i] == '\n' && !multiline {
			return i
		}
	}
	return len(src)
}

// commentEnd answers the offset just past a `//` or `/*` comment, mirroring locate.go.
func commentEnd(src []byte, i int) int {
	if i+1 >= len(src) {
		return i
	}
	if src[i] == '/' && src[i+1] == '/' {
		for j := i; j < len(src); j++ {
			if src[j] == '\n' {
				return j
			}
		}
		return len(src)
	}
	if src[i] == '/' && src[i+1] == '*' {
		for j := i + 2; j+1 < len(src); j++ {
			if src[j] == '*' && src[j+1] == '/' {
				return j + 2
			}
		}
		return len(src)
	}
	return i
}

// heuristicValidatorPrefixes name the calls that clean a value: a function whose name starts with one of these is a
// guard a reviewer can argue with, in any of the languages read without a parser.
var heuristicValidatorPrefixes = []string{"validate", "sanitize", "sanitise", "escape", "quote", "allow", "check", "verify", "clean"}

// heuristicValidators are the calls that bound, escape or parse a value into something safe, in the languages read
// without a parser.
var heuristicValidators = []string{
	"parseInt", "parseFloat", "Number", "BigInt", "Boolean", "int", "float", "Decimal", "shlex.quote",
	"quote", "escape", "realpath", "abspath",
}

// isHeuristicValidator reports a call that cleans a value. decodeURIComponent and encodeURIComponent are
// deliberately absent: decoding is a rename, not a check, which is exactly the ordering defect a path guard hides
// between — `if (!userPath.includes("..")) return resolve(root, decodeURIComponent(userPath))` checks the raw
// value and hands on the decoded one.
func isHeuristicValidator(name string) bool {
	if anyOf(name, heuristicValidators) {
		return true
	}
	lower := strings.ToLower(name)
	for _, prefix := range heuristicValidatorPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// param is one parameter of a function read without a parser: the name the body uses, the label the report
// prints, and a written type when the language has one.
type param struct {
	name     string
	label    string
	typeName string
}

// callRule is one line of a lexer's sink catalog: a sink kind, the callee it answers to, and which argument
// carries the value. It is data on purpose — a reviewer can read it, and a language adds its rows without
// touching the flow analysis.
type callRule struct {
	Kind string
	// Names are the last segment of the callee: open, execute, readFileSync.
	Names []string
	// Owners pin a name to the module or object it belongs to when the name alone is too common. An empty
	// list means the name is enough; an owner "" means a bare call the module bindings did not resolve.
	Owners []string
	// Ctor requires the call to be a construction, which is how `new RegExp` is told from `x.RegExp`.
	Ctor bool
	// From is the first argument that carries the sensitive value.
	From int
	// Whole reports a sink the whole statement decides: a case folding only counts in a comparison, so its
	// arguments say nothing on their own.
	Whole bool
}

// matches reports whether a call answers to this rule, using the names the file imported from a module for a
// callee written without its owner.
func (r callRule) matches(c callee, isNew bool, bindings map[string]string) bool {
	if !anyOf(c.name(), r.Names) {
		return false
	}
	if r.Ctor && !isNew {
		return false
	}
	if len(r.Owners) == 0 {
		return true
	}
	owner := c.owner()
	if owner == "" {
		owner = bindings[c.name()]
	}
	return anyOf(owner, r.Owners)
}
