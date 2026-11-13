package search

import (
	"example.com/archive/internal/textutil"
	"fmt"
	"sort"
	"strings"
)

// Terms lists the indexed terms that appear in at least one object, sorted, capped at limit.
func (i *Index) Terms(limit int) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	terms := make([]string, 0, len(i.byTerm))
	for term := range i.byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	if limit > 0 && len(terms) > limit {
		terms = terms[:limit]
	}
	return terms
}

// Search answers a query: every token must match (the index is AND, not OR), and the ordering is by
// the number of matched tokens, then by how often the term occurs, then by object id so the answer is
// stable. A query with no tokens matches nothing.
func (i *Index) Search(query string, limit int) []Match {
	tokens := Unique(query)
	if len(tokens) == 0 {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	scores := map[string]int{}
	var first bool = true
	for _, token := range tokens {
		bucket := i.byTerm[token]
		if len(bucket) == 0 {
			return nil
		}
		if first {
			for id, count := range bucket {
				scores[id] = count + 1
			}
			first = false
			continue
		}
		for id := range scores {
			if count, ok := bucket[id]; ok {
				scores[id] += count + 1
				continue
			}
			delete(scores, id)
		}
	}
	matches := make([]Match, 0, len(scores))
	for id, score := range scores {
		record, ok := i.docs[id]
		if !ok {
			continue
		}
		matches = append(matches, Match{Record: record, Score: score, Tokens: matched(tokens, i.terms[id])})
	}
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].Score == matches[b].Score {
			return matches[a].Record.ID < matches[b].Record.ID
		}
		return matches[a].Score > matches[b].Score
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// matched lists the query tokens a document carries, in query order.
func matched(query, document []string) []string {
	have := map[string]bool{}
	for _, term := range document {
		have[term] = true
	}
	var out []string
	for _, term := range query {
		if have[term] {
			out = append(out, term)
		}
	}
	return out
}

// Describe renders one match as the line the console prints: score, label and the matched tokens.
func (m Match) Describe() string {
	return fmt.Sprintf("%d %s [%s]", m.Score, textutil.Label(m.Record.Label()), strings.Join(m.Tokens, " "))
}
