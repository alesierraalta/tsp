package surface

import (
	"bytes"
	"regexp"
	"strings"
)

// lexPy turns Python source into tokens. A string or a comment becomes a token of its own, so a sink spelled
// inside one is never read as a call. An f-string keeps its `{...}` interpolations as code, because
// `f"... {name}"` is the shape a built string reaches a sink through. No interpreter is involved: Python's
// blocks are read by indentation, the way internal/findings/locate.go's pythonBody reads one.
func lexPy(src []byte) []lexeme {
	var toks []lexeme
	line := 1
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '\'' || c == '"':
			var next int
			toks, next, line = lexPyString(src, i, line, toks, "")
			i = next
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			word := string(src[start:i])
			if pyStringPrefix(word) && i < len(src) && (src[i] == '\'' || src[i] == '"') {
				var next int
				toks, next, line = lexPyString(src, i, line, toks, word)
				i = next
				continue
			}
			toks = append(toks, lexeme{kind: tokIdent, text: word, start: start, end: i, line: line})
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9'):
			start := i
			for i < len(src) && (isIdentPart(src[i]) || src[i] == '.') {
				i++
			}
			toks = append(toks, lexeme{kind: tokNumber, text: string(src[start:i]), start: start, end: i, line: line})
		default:
			op, width := pyOperator(src, i)
			toks = append(toks, lexeme{kind: tokPunct, text: op, start: i, end: i + width, line: line})
			i += width
		}
	}
	return toks
}

// lexPyString reads one string literal, including a triple-quoted one and the interpolation of an f-string.
func lexPyString(src []byte, at, line int, toks []lexeme, prefix string) ([]lexeme, int, int) {
	quote := src[at]
	triple := at+2 < len(src) && src[at+1] == quote && src[at+2] == quote
	width := 1
	if triple {
		width = 3
	}
	delimiter := strings.Repeat(string(quote), width)
	interpolated := strings.ContainsAny(strings.ToLower(prefix), "f") || prefix == ""
	start := at
	i := at + width
	if !interpolated || !strings.ContainsAny(strings.ToLower(prefix), "f") {
		for i < len(src) {
			if src[i] == '\\' {
				i += 2
				continue
			}
			if bytes.HasPrefix(src[i:], []byte(delimiter)) {
				i += width
				break
			}
			if src[i] == '\n' {
				if !triple {
					break
				}
				line++
			}
			i++
		}
		text := string(src[start:min(i, len(src))])
		toks = append(toks, lexeme{kind: tokString, text: text, start: start, end: min(i, len(src)), line: line})
		return toks, min(i, len(src)), line
	}
	// An f-string: the literal parts are text, the interpolations are code.
	toks = append(toks, lexeme{kind: tokPunct, text: prefix + delimiter, start: start, end: i, line: line})
	chunk := i
	chunkLine := line
	flush := func(end int, atLine int) {
		if end <= chunk {
			return
		}
		toks = append(toks, lexeme{kind: tokText, text: string(src[chunk:end]), start: chunk, end: end, line: atLine})
	}
	for i < len(src) {
		switch {
		case src[i] == '\\':
			i += 2
		case bytes.HasPrefix(src[i:], []byte(delimiter)):
			flush(i, chunkLine)
			toks = append(toks, lexeme{kind: tokPunct, text: delimiter, start: i, end: i + width, line: line})
			return toks, i + width, line
		case src[i] == '{' && i+1 < len(src) && src[i+1] == '{':
			i += 2
		case src[i] == '{':
			flush(i, chunkLine)
			toks = append(toks, lexeme{kind: tokPunct, text: "{", start: i, end: i + 1, line: line})
			inner, next, nextLine := lexPyInterpolation(src, i+1, line)
			toks = append(toks, inner...)
			toks = append(toks, lexeme{kind: tokPunct, text: "}", start: next, end: next + 1, line: nextLine})
			i, line, chunk, chunkLine = next+1, nextLine, next+1, nextLine
		case src[i] == '\n':
			line++
			i++
		default:
			i++
		}
	}
	flush(i, chunkLine)
	return toks, i, line
}

// lexPyInterpolation reads the expression inside an f-string's braces, stopping at the brace that closes it.
func lexPyInterpolation(src []byte, at, line int) ([]lexeme, int, int) {
	depth := 1
	var toks []lexeme
	i := at
	for i < len(src) {
		c := src[i]
		switch {
		case c == '{':
			depth++
			i++
		case c == '}':
			depth--
			if depth == 0 {
				return toks, i, line
			}
			i++
		case c == '\'' || c == '"':
			var next int
			toks, next, line = lexPyString(src, i, line, toks, "")
			i = next
		case c == '\n':
			line++
			i++
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			toks = append(toks, lexeme{kind: tokIdent, text: string(src[start:i]), start: start, end: i, line: line})
		case c == ' ' || c == '\t':
			i++
		default:
			op, width := pyOperator(src, i)
			toks = append(toks, lexeme{kind: tokPunct, text: op, start: i, end: i + width, line: line})
			i += width
		}
	}
	return toks, i, line
}

// pyStringPrefix reports a run of letters that can prefix a Python string literal.
func pyStringPrefix(word string) bool {
	lower := strings.ToLower(word)
	if len(lower) > 2 {
		return false
	}
	for i := 0; i < len(lower); i++ {
		if !strings.ContainsRune("rbfu", rune(lower[i])) {
			return false
		}
	}
	return lower != ""
}

// pyOperators are the multi-byte operators, longest first.
var pyOperators = []string{"**=", "//=", ">>=", "<<=", "...", "**", "//", "==", "!=", "<=", ">=", "->", ":=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", ">>", "<<", "@=", "@"}

// pyOperator answers the operator at an offset and how wide it is.
func pyOperator(src []byte, at int) (string, int) {
	for _, op := range pyOperators {
		if strings.HasPrefix(string(src[at:min(at+len(op), len(src))]), op) {
			return op, len(op)
		}
	}
	return string(src[at : at+1]), 1
}

// pyDefinition and pyClass mirror locate.go's patterns, so a definition is read the same way wherever it is
// found.
var (
	pyDefinition = regexp.MustCompile(`^(?:async\s+)?def\s+([A-Za-z_][A-Za-z_0-9]*)\s*\(`)
	pyClass      = regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z_0-9]*)`)
)

// pyIndent answers how far a line is indented, in bytes, which is the only block delimiter Python has.
func pyIndent(line string) int { return len(line) - len(strings.TrimLeft(line, " \t")) }

// pyFunction is one function the indentation scan found.
type pyFunction struct {
	name     string
	line     int
	last     int
	exported bool
	method   bool
	params   []param
	body     span
	header   span
}

// pyFunctions reads the defs of a file: every function and every method, skipping a decorator line and
// reading a class body by indentation the way locate.go does.
func pyFunctions(src []byte, toks []lexeme) []pyFunction {
	lines := strings.Split(string(src), "\n")
	starts := pyLineStarts(toks, len(lines))
	class, classIndent := "", -1
	var found []pyFunction
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		indent := pyIndent(lines[i])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && indent <= classIndent {
			class, classIndent = "", -1
		}
		if match := pyClass.FindStringSubmatch(trimmed); match != nil {
			class, classIndent = match[1], indent
			continue
		}
		match := pyDefinition.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		last := i + 1
		for last < len(lines) {
			text := strings.TrimSpace(lines[last])
			if text != "" && pyIndent(lines[last]) <= indent {
				break
			}
			last++
		}
		from, ok := starts[i]
		if !ok {
			continue
		}
		to := len(toks)
		for j := from; j < len(toks); j++ {
			if toks[j].line > last {
				to = j
				break
			}
		}
		def := -1
		for j := from; j < to; j++ {
			if toks[j].kind == tokIdent && toks[j].text == "def" {
				def = j
				break
			}
		}
		if def < 0 {
			continue
		}
		open := -1
		for j := def + 1; j < to; j++ {
			if toks[j].text == "(" {
				open = j
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
		name := match[1]
		found = append(found, pyFunction{
			name:     name,
			line:     i + 1,
			last:     last,
			exported: !strings.HasPrefix(name, "_"),
			method:   class != "" && indent > classIndent,
			params:   pyParams(toks, span{open + 1, close}),
			body:     span{from, to},
			header:   span{from, to},
		})
		i = last - 1
	}
	return found
}

// pyLineStarts answers the first token index of every line, so a def line can be found from the line scan.
func pyLineStarts(toks []lexeme, lines int) map[int]int {
	starts := make(map[int]int, lines)
	for i, t := range toks {
		line := t.line - 1
		if _, seen := starts[line]; !seen {
			starts[line] = i
		}
	}
	return starts
}

// pyParams reads a parameter list: the name, a written annotation when there is one, and no `self`/`cls`,
// which the caller never sends.
func pyParams(toks []lexeme, inside span) []param {
	var params []param
	for _, part := range splitSpan(toks, inside, ",") {
		for part.from < part.to && anyOf(toks[part.from].text, []string{"*", "**", "***"}) {
			part.from++
		}
		if part.empty() || toks[part.from].kind != tokIdent {
			continue
		}
		name := toks[part.from].text
		if name == "self" || name == "cls" {
			continue
		}
		typeName, label := "", name
		if part.from+2 < part.to && toks[part.from+1].text == ":" {
			stop := part.to
			for m := part.from + 2; m < part.to; m++ {
				if toks[m].text == "=" {
					stop = m
					break
				}
			}
			typeName = render(toks, span{part.from + 2, stop})
			if typeName != "" {
				label = name + " " + typeName
			}
		}
		params = append(params, param{name: name, label: label, typeName: typeName})
	}
	return params
}

// pyImports records which local names came from which module, so `urlopen(...)` is read as urllib's.
func pyImports(toks []lexeme) map[string]string {
	bindings := map[string]string{}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.kind == tokIdent && t.text == "import":
			for j := i + 1; j < len(toks) && toks[j].line == t.line; j++ {
				if toks[j].kind != tokIdent {
					continue
				}
				bindings[toks[j].text] = toks[j].text
				if j+2 < len(toks) && toks[j+1].text == "." && toks[j+2].kind == tokIdent {
					bindings[toks[j+2].text] = toks[j].text + "." + toks[j+2].text
				}
			}
		case t.kind == tokIdent && t.text == "from":
			module := ""
			for j := i + 1; j < len(toks) && toks[j].line == t.line; j++ {
				if toks[j].kind != tokIdent || toks[j].text == "import" {
					continue
				}
				if module == "" {
					module = toks[j].text
					bindings[module] = module
					continue
				}
				bindings[toks[j].text] = module
			}
		}
	}
	return bindings
}
