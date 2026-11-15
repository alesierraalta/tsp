package sqlrun

import (
	"fmt"
	"strconv"
	"strings"
)

// tokenKind is the class of a lexed token.
type tokenKind int

// The token classes the statement language carries.
const (
	tokEOF tokenKind = iota
	tokIdent
	tokNumber
	tokString
	tokComma
	tokStar
	tokLParen
	tokRParen
	tokOperator
	tokPlaceholder
	tokKeyword
)

// keywords are the words the language reserves. A column named after one of them is written with a
// leading underscore, and the name check in internal/query refuses a name that is one of these.
var keywords = map[string]bool{
	"select":   true,
	"from":     true,
	"where":    true,
	"order":    true,
	"by":       true,
	"limit":    true,
	"asc":      true,
	"desc":     true,
	"and":      true,
	"or":       true,
	"not":      true,
	"in":       true,
	"like":     true,
	"distinct": true,
}

// token is one lexed item with the position it started at, so a refusal can point at it.
type token struct {
	kind  tokenKind
	text  string
	pos   int
	label string
}

// String renders a token for an error message.
func (t token) String() string {
	if t.kind == tokEOF {
		return "end of statement"
	}
	return strconv.Quote(t.text)
}

// lexer reads one statement into tokens.
type lexer struct {
	input string
	pos   int
}

// lex turns a statement into its tokens. An unterminated string, a stray character and a malformed
// number are refusals: a statement the engine cannot read is not silently cut short, because a
// statement cut short would answer a different question than the one asked.
func lex(input string) ([]token, error) {
	l := &lexer{input: input}
	var out []token
	for {
		next, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, next)
		if next.kind == tokEOF {
			return out, nil
		}
	}
}

// next reads the token at the current position.
func (l *lexer) next() (token, error) {
	l.skipSpace()
	start := l.pos
	if l.pos >= len(l.input) {
		return token{kind: tokEOF, pos: start}, nil
	}
	c := l.input[l.pos]
	switch {
	case c == ',':
		l.pos++
		return token{kind: tokComma, text: ",", pos: start}, nil
	case c == '*':
		l.pos++
		return token{kind: tokStar, text: "*", pos: start}, nil
	case c == '(':
		l.pos++
		return token{kind: tokLParen, text: "(", pos: start}, nil
	case c == ')':
		l.pos++
		return token{kind: tokRParen, text: ")", pos: start}, nil
	case c == '?':
		l.pos++
		return token{kind: tokPlaceholder, text: "?", pos: start}, nil
	case c == '\'':
		return l.readString()
	case isDigit(c):
		return l.readNumber()
	case isIdentStart(c):
		return l.readWord()
	case c == '=':
		l.pos++
		return token{kind: tokOperator, text: "=", pos: start}, nil
	case c == '!' || c == '<' || c == '>':
		return l.readComparison()
	}
	return token{}, fmt.Errorf("sqlrun: unexpected character %q at %d", string(c), start)
}

// skipSpace advances past spaces, tabs and newlines.
func (l *lexer) skipSpace() {
	for l.pos < len(l.input) {
		switch l.input[l.pos] {
		case ' ', '\t', '\n', '\r':
			l.pos++
		default:
			return
		}
	}
}

// readString reads a quoted string. A doubled quote inside it is one quote, which is how a value
// carrying a quote is written.
func (l *lexer) readString() (token, error) {
	start := l.pos
	l.pos++
	var b strings.Builder
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == '\'' {
			if l.pos+1 < len(l.input) && l.input[l.pos+1] == '\'' {
				b.WriteByte('\'')
				l.pos += 2
				continue
			}
			l.pos++
			return token{kind: tokString, text: b.String(), pos: start}, nil
		}
		b.WriteByte(c)
		l.pos++
	}
	return token{}, fmt.Errorf("sqlrun: unterminated string starting at %d", start)
}

// readNumber reads an integer literal. A decimal point is not part of the language: a report that
// wants a fraction of a unit compares minor units.
func (l *lexer) readNumber() (token, error) {
	start := l.pos
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	text := l.input[start:l.pos]
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		return token{}, fmt.Errorf("sqlrun: %q carries a decimal point; the engine holds whole minor units", text+".")
	}
	if l.pos < len(l.input) && isIdentStart(l.input[l.pos]) {
		return token{}, fmt.Errorf("sqlrun: %q is not a number", l.input[start:l.pos+1])
	}
	return token{kind: tokNumber, text: text, pos: start}, nil
}

// readWord reads an identifier or a keyword.
func (l *lexer) readWord() (token, error) {
	start := l.pos
	for l.pos < len(l.input) && isIdentPart(l.input[l.pos]) {
		l.pos++
	}
	text := l.input[start:l.pos]
	if keywords[strings.ToLower(text)] {
		return token{kind: tokKeyword, text: strings.ToLower(text), pos: start}, nil
	}
	return token{kind: tokIdent, text: text, pos: start}, nil
}

// readComparison reads one of the comparison operators, or reports the one it cannot read.
func (l *lexer) readComparison() (token, error) {
	start := l.pos
	c := l.input[l.pos]
	l.pos++
	if l.pos < len(l.input) && l.input[l.pos] == '=' {
		l.pos++
		if c == '!' {
			return token{kind: tokOperator, text: "!=", pos: start}, nil
		}
		return token{kind: tokOperator, text: string(c) + "=", pos: start}, nil
	}
	switch c {
	case '<', '>':
		return token{kind: tokOperator, text: string(c), pos: start}, nil
	}
	return token{}, fmt.Errorf("sqlrun: %q is not an operator", string(c))
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isIdentStart reports whether c may start an identifier.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isIdentPart reports whether c may continue an identifier.
func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }
