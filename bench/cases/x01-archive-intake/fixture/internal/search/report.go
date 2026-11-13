package search

import (
	"fmt"
	"strings"

	"example.com/archive/internal/textutil"
)

// Answer is what one search reports: the query, the matches, and the suggestion line when it found
// nothing. The console renders it, and the JSON endpoint serves the same fields.
type Answer struct {
	Query       string
	Terms       []string
	Matches     []Match
	Suggestions []string
	Limit       int
}

// Build answers a query over an index and keeps only the matches of one tenant. A match of another tenant
// is dropped before the answer is built, so a page cannot leak one by counting.
func Build(index *Index, query Query, tenant string) Answer {
	answer := Answer{Query: query.Text, Terms: Unique(query.Text), Limit: query.Limit}
	all := index.Search(query.Text, 0)
	for _, match := range all {
		if match.Record.Tenant != tenant {
			continue
		}
		if query.Kind != "" && match.Record.Kind != query.Kind {
			continue
		}
		if query.PagesMin > 0 && match.Record.Pages < query.PagesMin {
			continue
		}
		if query.PagesMax > 0 && match.Record.Pages > query.PagesMax {
			continue
		}
		answer.Matches = append(answer.Matches, match)
		if query.Limit > 0 && len(answer.Matches) >= query.Limit {
			break
		}
	}
	if len(answer.Matches) == 0 {
		answer.Suggestions = Suggest(index, answer.Terms, 5)
	}
	return answer
}

// Suggest names the indexed terms that start like the query's terms, so a page can print "did you mean".
// The suggestion is a prefix match on the index's own vocabulary, never a read of an object.
func Suggest(index *Index, terms []string, limit int) []string {
	if len(terms) == 0 {
		return nil
	}
	vocabulary := index.Terms(0)
	var out []string
	for _, term := range terms {
		dash := len(term) / 2
		if dash < 2 {
			dash = 2
		}
		if dash > len(term) {
			dash = len(term)
		}
		prefix := term[:dash]
		for _, candidate := range vocabulary {
			if candidate == term || !strings.HasPrefix(candidate, prefix) {
				continue
			}
			out = append(out, candidate)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

// Summary renders the answer as the line the console prints above the results.
func (a Answer) Summary() string {
	if a.Query == "" {
		return "no query"
	}
	if len(a.Matches) == 0 {
		line := fmt.Sprintf("no match for %q", a.Query)
		if len(a.Suggestions) > 0 {
			line += ", did you mean " + strings.Join(a.Suggestions, ", ") + "?"
		}
		return line
	}
	return fmt.Sprintf("%d of at most %d matches for %q", len(a.Matches), a.Limit, a.Query)
}

// Lines renders one line per match, for the console's search block.
func (a Answer) Lines() []string {
	lines := make([]string, 0, len(a.Matches)+1)
	lines = append(lines, a.Summary())
	for _, match := range a.Matches {
		lines = append(lines, match.Describe())
	}
	for _, suggestion := range a.Suggestions {
		lines = append(lines, "did you mean "+suggestion)
	}
	return lines
}

// Explain renders the filter a query carried, so a page can show what it searched for and not only the
// words: "kind=scan, 2..8 pages".
func (q Query) Explain() string {
	var parts []string
	if q.Kind != "" {
		parts = append(parts, "kind="+q.Kind)
	}
	if q.Tenant != "" {
		parts = append(parts, "tenant="+q.Tenant)
	}
	if q.Batch != "" {
		parts = append(parts, "batch="+q.Batch)
	}
	if q.PagesMin > 0 || q.PagesMax > 0 {
		parts = append(parts, fmt.Sprintf("pages %d..%d", q.PagesMin, q.PagesMax))
	}
	if q.Limit > 0 {
		parts = append(parts, fmt.Sprintf("limit %d", q.Limit))
	}
	if len(parts) == 0 {
		return "no filter"
	}
	return textutil.Collapse(strings.Join(parts, ", "))
}
