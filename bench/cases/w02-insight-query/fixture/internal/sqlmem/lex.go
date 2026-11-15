// Package sqlmem is the in-process statement engine the service ships with. It understands the
// subset of SQL the reports need: one SELECT over one table, a conjunction of comparisons, an
// ordering and a row limit.
package sqlmem

import (
	"fmt"
	"strings"
)

// Kind is the lexical class of a token.
type Kind int

// The token kinds the lexer produces.
const (
	Word   Kind = iota // an identifier or a bare number
	String             // a single-quoted literal
	Op                 // a comparison operator
	Comma              // the column separator
	Star               // *
	EOF                // end of input
)

// Token is one lexical unit. Literal is the decoded value for String and Word; Text is the source
// spelling, which the parser uses for errors.
type Token struct {
	Kind    Kind
	Text    string
	Literal string
	Pos     int
}

// Lex splits a statement into tokens. A single-quoted literal keeps ” as one quote, which is the
// only escape the engine accepts; an unterminated literal is an error rather than a guess.
func Lex(sql string) ([]Token, error) {
	var tokens []Token
	i := 0
	for i < len(sql) {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == ',':
			tokens = append(tokens, Token{Kind: Comma, Text: ",", Pos: i})
			i++
		case c == '*':
			tokens = append(tokens, Token{Kind: Star, Text: "*", Pos: i})
			i++
		case c == '=':
			tokens = append(tokens, Token{Kind: Op, Text: "=", Pos: i})
			i++
		case c == '!' || c == '<' || c == '>':
			start := i
			i++
			if i < len(sql) && sql[i] == '=' {
				i++
			}
			text := sql[start:i]
			switch text {
			case "!=", "<=", ">=", "<", ">":
				tokens = append(tokens, Token{Kind: Op, Text: text, Pos: start})
			default:
				return nil, fmt.Errorf("sqlmem: %q is not an operator", text)
			}
		case c == '\'':
			start := i
			i++
			var b strings.Builder
			closed := false
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						b.WriteByte('\'')
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				b.WriteByte(sql[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("sqlmem: unterminated literal at %d", start)
			}
			tokens = append(tokens, Token{Kind: String, Text: sql[start:i], Literal: b.String(), Pos: start})
		case isWordByte(c):
			start := i
			for i < len(sql) && isWordByte(sql[i]) {
				i++
			}
			text := sql[start:i]
			tokens = append(tokens, Token{Kind: Word, Text: text, Literal: text, Pos: start})
		default:
			return nil, fmt.Errorf("sqlmem: %q at %d is not part of a statement", string(c), i)
		}
	}
	tokens = append(tokens, Token{Kind: EOF, Pos: len(sql)})
	return tokens, nil
}

func isWordByte(c byte) bool {
	return c == '_' || c == '.' || c == '-' || c == ':' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// isNumber reports whether a word token is a bare number, which the evaluator compares as one.
func isNumber(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	digits := 0
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		digits++
	}
	return digits > 0
}
