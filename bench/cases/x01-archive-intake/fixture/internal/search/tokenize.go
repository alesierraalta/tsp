// Package search is the archive's text index: the tokens an object's name, title, tags and text
// produce, and the ranking a query is answered with.
package search

import (
	"sort"
	"strings"
	"unicode"
)

// maxToken is the longest token the index keeps. A token longer than this is a base64 blob or a
// malformed word, and indexing it would make the index a copy of the object.
const maxToken = 48

// stopwords are the words the index drops: a query for one of them matches every object, which is the
// same as matching none.
var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true,
	"by": true, "for": true, "from": true, "in": true, "is": true, "it": true, "of": true,
	"on": true, "or": true, "that": true, "the": true, "to": true, "with": true,
}

// Tokens splits text into the lowercased tokens the index keeps. A run of letters or digits is one
// token; everything else separates. A token longer than maxToken is dropped rather than truncated,
// because a truncated token matches words it should not.
func Tokens(text string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		token := strings.ToLower(current.String())
		current.Reset()
		if len(token) > maxToken || len(token) < 2 {
			return
		}
		if stopwords[token] {
			return
		}
		tokens = append(tokens, token)
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

// Unique returns the tokens of text without repeats, in first-seen order.
func Unique(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range Tokens(text) {
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

// Sorted returns the unique tokens of text in sorted order, which is the order the index stores them.
func Sorted(text string) []string {
	tokens := Unique(text)
	sort.Strings(tokens)
	return tokens
}

// Prefix returns the tokens of text that start with prefix, for the "did you mean" line the search page
// prints when a query finds nothing.
func Prefix(text, prefix string) []string {
	lowered := strings.ToLower(prefix)
	var out []string
	for _, token := range Unique(text) {
		if strings.HasPrefix(token, lowered) {
			out = append(out, token)
		}
	}
	return out
}
