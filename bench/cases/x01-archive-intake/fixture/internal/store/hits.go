package store

import (
	"example.com/archive/internal/storepath"
	"sort"
	"strings"
)

// Hit is one search result: the record and the score the search ordered it by.
type Hit struct {
	Record Record
	Score  int
	Field  string
}

// Search ranks the records that match a query. A name hit scores highest, then a title hit, then a
// tag; the score is the number of matched fields plus the position of the match, so an exact prefix
// ranks above a hit in the middle of a word.
func (s *Store) Search(query string, limit int) []Hit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var hits []Hit
	for _, record := range s.All() {
		score, field := 0, ""
		if idx := strings.Index(strings.ToLower(record.Name), q); idx >= 0 {
			score += 100 - idx
			field = "name"
		}
		if idx := strings.Index(strings.ToLower(record.Title), q); idx >= 0 {
			score += 60 - idx
			if field == "" {
				field = "title"
			}
		}
		for _, tag := range record.Tags {
			if idx := strings.Index(strings.ToLower(tag), q); idx >= 0 {
				score += 20 - idx
				if field == "" {
					field = "tag"
				}
			}
		}
		if strings.Contains(strings.ToLower(record.Note), q) {
			score += 5
			if field == "" {
				field = "note"
			}
		}
		if score > 0 {
			hits = append(hits, Hit{Record: record, Score: score, Field: field})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Record.ID < hits[j].Record.ID
		}
		return hits[i].Score > hits[j].Score
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// UnderBatch lists the objects committed from one batch. A batch id that is not the batch of any
// object is an empty listing, not an error.
func (s *Store) UnderBatch(batch string) []Record {
	records, _ := s.List(Filter{Batch: batch})
	return records
}

// KindOf returns the kind a name implies, using the same table the staging tree uses.
func KindOf(name string) string { return storepath.Kind(name) }
