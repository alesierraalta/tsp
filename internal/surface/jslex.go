package surface

import "strings"

// lexJS turns JavaScript or TypeScript source into tokens. A literal or a comment becomes a token of its own,
// so a sink spelled inside one is never read as a call: the same masking internal/findings/locate.go does with
// quotedEnd and commentEnd, kept as a stream the flow analysis can walk. A template literal keeps its
// interpolations as code, because “ `${name}` “ is the shape a built string reaches a sink through.
func lexJS(src []byte) []lexeme {
	toks, _, _ := lexJSSpan(src, 0, 1, "")
	return toks
}

// lexJSSpan reads tokens from at until the end of the file, or until the `}` closing an interpolation when
// stop says so. It answers the tokens, the offset it stopped at, and the line it stopped on.
func lexJSSpan(src []byte, at, line int, stop string) ([]lexeme, int, int) {
	var toks []lexeme
	braces := 0
	i := at
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v' || c == 0:
			i++
		case c == '/' && (strings.HasPrefix(string(src[i:min(i+2, len(src))]), "//") || strings.HasPrefix(string(src[i:min(i+2, len(src))]), "/*")):
			end := commentEnd(src, i)
			line += countLines(src[i:end])
			i = end
		case c == '/' && jsRegexStart(toks):
			if end := jsRegexEnd(src, i); end > i {
				toks = append(toks, lexeme{kind: tokRegex, text: string(src[i:end]), start: i, end: end, line: line})
				i = end
				continue
			}
			toks = append(toks, lexeme{kind: tokPunct, text: "/", start: i, end: i + 1, line: line})
			i++
		case c == '\'' || c == '"':
			end := quotedEnd(src, i, false)
			toks = append(toks, lexeme{kind: tokString, text: string(src[i:end]), start: i, end: end, line: line})
			i = end
		case c == '`':
			var next int
			toks, next, line = lexJSChain(src, i, line, toks)
			i = next
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			toks = append(toks, lexeme{kind: tokIdent, text: string(src[start:i]), start: start, end: i, line: line})
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9'):
			start := i
			for i < len(src) && (isIdentPart(src[i]) || src[i] == '.' || ((src[i] == '+' || src[i] == '-') && i > start && (src[i-1] == 'e' || src[i-1] == 'E'))) {
				i++
			}
			toks = append(toks, lexeme{kind: tokNumber, text: string(src[start:i]), start: start, end: i, line: line})
		case stop == "}" && c == '}':
			return toks, i, line
		default:
			if c == '{' {
				braces++
			}
			if c == '}' && braces > 0 {
				braces--
			}
			op, width := jsOperator(src, i)
			toks = append(toks, lexeme{kind: tokPunct, text: op, start: i, end: i + width, line: line})
			i += width
		}
	}
	return toks, i, line
}

// lexJSChain reads one template literal starting at its backtick. The literal parts become tokText — the only
// place markup can live without being code — and each `${...}` is read as code, so a name inside one is a
// name the analysis sees.
func lexJSChain(src []byte, at, line int, toks []lexeme) ([]lexeme, int, int) {
	toks = append(toks, lexeme{kind: tokPunct, text: "`", start: at, end: at + 1, line: line})
	i := at + 1
	chunk := i
	chunkLine := line
	flush := func(end int) {
		if end <= chunk {
			return
		}
		toks = append(toks, lexeme{kind: tokText, text: string(src[chunk:end]), start: chunk, end: end, line: chunkLine})
	}
	for i < len(src) {
		switch src[i] {
		case '\\':
			if i+1 < len(src) && src[i+1] == '\n' {
				line++
			}
			i += 2
		case '\n':
			line++
			i++
		case '`':
			flush(i)
			toks = append(toks, lexeme{kind: tokPunct, text: "`", start: i, end: i + 1, line: line})
			return toks, i + 1, line
		case '$':
			if i+1 < len(src) && src[i+1] == '{' {
				flush(i)
				toks = append(toks, lexeme{kind: tokPunct, text: "${", start: i, end: i + 2, line: line})
				inner, next, nextLine := lexJSSpan(src, i+2, line, "}")
				toks = append(toks, inner...)
				if next < len(src) && src[next] == '}' {
					toks = append(toks, lexeme{kind: tokPunct, text: "}", start: next, end: next + 1, line: nextLine})
					next++
				}
				i, line, chunk, chunkLine = next, nextLine, next, nextLine
				continue
			}
			i++
		default:
			i++
		}
	}
	flush(i)
	return toks, i, line
}

// jsOperators are the multi-byte operators, longest first, so `===` is never read as two comparisons.
var jsOperators = []string{"===", "!==", "...", "**=", "??=", "||=", "&&=", "=>", "==", "!=", "&&", "||", "??", "?.", "+=", "-=", "*=", "/=", "%=", "**", "++", "--", "<=", ">=", "|=", "&=", "^=", "<<"}

// jsOperator answers the operator at an offset and how wide it is. `?.` before a digit is a ternary, not a
// member access.
func jsOperator(src []byte, at int) (string, int) {
	for _, op := range jsOperators {
		if !strings.HasPrefix(string(src[at:min(at+len(op), len(src))]), op) {
			continue
		}
		if op == "?." && at+2 < len(src) && src[at+2] >= '0' && src[at+2] <= '9' {
			continue
		}
		return op, len(op)
	}
	return string(src[at : at+1]), 1
}

// jsRegexStart reports whether a slash opens a regular expression instead of dividing: after a value it
// divides, after an operator or a keyword it starts a literal.
func jsRegexStart(toks []lexeme) bool {
	if len(toks) == 0 {
		return true
	}
	last := toks[len(toks)-1]
	if last.kind == tokIdent {
		switch last.text {
		case "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "do", "else", "yield", "await", "throw", "case", "default", "extends":
			return true
		}
		return false
	}
	if last.kind == tokPunct {
		switch last.text {
		case ")", "]", "}":
			return false
		}
		return true
	}
	return false
}

// jsRegexEnd answers the offset just past the regular expression literal at at, or 0 when the slash was not
// one: a literal cannot cross a line, so a newline ends the attempt and the slash is division after all.
func jsRegexEnd(src []byte, at int) int {
	inClass := false
	for i := at + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '\n':
			return 0
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			end := i + 1
			for end < len(src) && isIdentPart(src[end]) {
				end++
			}
			return end
		}
	}
	return 0
}

// countLines counts the newlines in a byte run, so a token read whole keeps the line numbers after it right.
func countLines(body []byte) int { return strings.Count(string(body), "\n") }

// min answers the smaller of two offsets.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
