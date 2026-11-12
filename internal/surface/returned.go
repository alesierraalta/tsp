package surface

import "strings"

// This file is the rule the three languages share about a value an exported function builds and hands back:
// what kind of value it is, read from the bytes of the literal or the shape of the call that built it. The
// extraction stays per language; only the verdict is shared, so a Go row and a lexer's row mean the same thing.

// sqlWords are the words a statement carries, so a literal built from an input that mentions them is a query
// the caller will run.
var sqlWords = []string{"select ", "select *", "insert into", "update ", "delete from", "where ", "values (", "union ", "order by", "group by", "drop table", "alter table"}

// commandWords are the strings that open a shell command line, which is the other built value a caller runs.
var commandWords = []string{"sh -c", "bash -c", "cmd /c", "powershell", "/bin/sh", "/bin/bash", "/bin/zsh"}

// urlSchemes are the prefixes that make a built string a URL instead of a message.
var urlSchemes = []string{"http://", "https://", "ws://", "wss://", "ftp://", "file://", "ldap://", "gopher://"}

// builtFromLiteral answers which returned sink a written literal builds, by what the text itself says. A
// concatenation is markup because the bytes are a tag or an attribute, not because a name says so.
func builtFromLiteral(text string) builtValue {
	if strings.TrimSpace(text) == "" {
		return builtValue{}
	}
	lower := strings.ToLower(text)
	switch {
	case markupText(text):
		return builtValue{kind: SinkReturnedMarkup, call: clip(text, 60)}
	case sqlText(lower):
		return builtValue{kind: SinkReturnedQuery, call: clip(text, 60)}
	case commandText(lower):
		return builtValue{kind: SinkReturnedQuery, call: clip(text, 60)}
	case urlText(lower):
		return builtValue{kind: SinkReturnedURL, call: clip(text, 60)}
	}
	return builtValue{}
}

// markupText reports a literal that builds markup: an HTML tag, or an attribute written name="value". The
// attribute form matters because the escaper has to know the quote it is writing inside, which is the defect a
// shared text table hides.
func markupText(text string) bool {
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		next := text[i+1]
		if !(isMarkupStart(next) || next == '/') {
			continue
		}
		close := strings.IndexByte(text[i+1:], '>')
		if close < 0 {
			continue
		}
		// A tag name carries no space: `<that file>` is prose that happens to be bracketed, and a row on it is
		// a probe nobody can break. An attribute is the one place a space is written inside a tag.
		group := text[i+1 : i+1+close]
		if strings.ContainsRune(group, ' ') && !strings.ContainsRune(group, '=') {
			continue
		}
		return true
	}
	return attributeText(text)
}

// attributeText reports a literal that writes an attribute: a name, or a closed interpolation, followed by `=`
// and the quote or interpolation the value is written into.
func attributeText(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '=' {
			continue
		}
		// The name an attribute is written for can be an interpolation, so a literal that starts with the
		// assignment is still an attribute: `${name}="${value}"`.
		if i > 0 {
			before := text[i-1]
			if !(isIdentPart(before) || before == '"' || before == '\'' || before == '}' || before == ')') {
				continue
			}
		}
		if i+1 >= len(text) {
			continue
		}
		after := text[i+1]
		if after == '"' || after == '\'' || after == '{' || after == '$' {
			return true
		}
	}
	return false
}

// sqlText reports a literal that carries a statement, written as its own words.
func sqlText(lower string) bool {
	for _, word := range sqlWords {
		if containsWord(lower, word) {
			return true
		}
	}
	return false
}

// commandText reports a literal that opens a command line.
func commandText(lower string) bool {
	for _, word := range commandWords {
		if containsWord(lower, word) {
			return true
		}
	}
	return false
}

// containsWord reports a phrase that starts at a word boundary, so "anywhere else" is not a WHERE clause and
// "formatting" is not a format string.
func containsWord(text, phrase string) bool {
	for at := 0; at+len(phrase) <= len(text); at++ {
		index := strings.Index(text[at:], phrase)
		if index < 0 {
			return false
		}
		start := at + index
		if start == 0 || !isWordByte(text[start-1]) {
			return true
		}
		at = start + 1
	}
	return false
}

// isWordByte reports a byte that continues a word, which a phrase cannot start in the middle of.
func isWordByte(c byte) bool {
	return isIdentPart(c) || c == '-' || c == '#'
}

// urlText reports a literal that carries a scheme.
func urlText(lower string) bool {
	for _, scheme := range urlSchemes {
		if strings.Contains(lower, scheme) {
			return true
		}
	}
	return false
}

// clip shortens text to at most n characters, marking the cut so a reader knows something was removed.
func clip(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[:n]) + "…"
}

// firstBuilt answers the first built value among the candidates, which is how a concatenation reports the most
// specific thing one of its sides builds.
func firstBuilt(values ...builtValue) builtValue {
	for _, value := range values {
		if value.kind != "" {
			return value
		}
	}
	return builtValue{}
}

// presenceOnly reports a guard that only asks whether a value is there: a comparison against nil, an empty
// string, zero or nothing, or a bare negation of one name. A presence test says nothing about what the value
// contains, so it does not stand between an input and a sink.
func presenceOnly(toks []lexeme, part span) bool {
	if containsAny(toks, part, ">", "<", ">=", "<=", "includes", "startsWith", "endsWith", "indexOf", "test", "match", "isAbsolute", "in", "not in") {
		return false
	}
	for i := part.from; i < part.to && i < len(toks); i++ {
		if !anyOf(toks[i].text, []string{"==", "===", "!=", "!=="}) {
			continue
		}
		if emptyOperand(toks, i-1) || emptyOperand(toks, i+1) {
			return true
		}
	}
	return bareNegation(toks, part)
}

// emptyOperand reports the token that stands for nothing written: an empty string or a zero. A null, an
// undefined or a None is not one of these, because the error a parse returns is the verdict on the value.
func emptyOperand(toks []lexeme, at int) bool {
	if at < 0 || at >= len(toks) {
		return false
	}
	t := toks[at]
	switch t.kind {
	case tokString, tokText:
		return strings.TrimSpace(unquote(t.text)) == ""
	case tokNumber:
		return t.text == "0"
	}
	return false
}

// bareNegation reports a condition that is one negated name: `!value` asks whether the value is there.
func bareNegation(toks []lexeme, part span) bool {
	var meaningful []lexeme
	for i := part.from; i < part.to && i < len(toks); i++ {
		if anyOf(toks[i].text, []string{"(", ")"}) {
			continue
		}
		meaningful = append(meaningful, toks[i])
	}
	switch len(meaningful) {
	case 2:
		return meaningful[0].text == "!" && meaningful[1].kind == tokIdent
	case 1:
		return meaningful[0].kind == tokIdent
	}
	return false
}
