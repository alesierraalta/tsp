package search

import (
	"example.com/archive/internal/store"
	"strings"
	"sync"
)

// Match is one search result: the record, the score it ranked with, and the tokens that matched.
type Match struct {
	Record store.Record
	Score  int
	Tokens []string
}

// Index is an in-memory inverted index over the committed objects. It is rebuilt from the store when
// the service opens and updated as objects commit, so a search never reads the filesystem.
type Index struct {
	mu     sync.RWMutex
	byTerm map[string]map[string]int
	docs   map[string]store.Record
	terms  map[string][]string
}

// New returns an empty index.
func New() *Index {
	return &Index{byTerm: map[string]map[string]int{}, docs: map[string]store.Record{}, terms: map[string][]string{}}
}

// Add indexes one record. Indexing the same record twice replaces its terms, so a re-index cannot leave
// a stale term pointing at it.
func (i *Index) Add(record store.Record, text string) {
	terms := Sorted(strings.Join([]string{record.Name, record.Title, strings.Join(record.Tags, " "), text}, " "))
	i.mu.Lock()
	defer i.mu.Unlock()
	if previous, ok := i.terms[record.ID]; ok {
		for _, term := range previous {
			if bucket, ok := i.byTerm[term]; ok {
				delete(bucket, record.ID)
				if len(bucket) == 0 {
					delete(i.byTerm, term)
				}
			}
		}
	}
	i.docs[record.ID] = record
	i.terms[record.ID] = terms
	for _, term := range terms {
		bucket, ok := i.byTerm[term]
		if !ok {
			bucket = map[string]int{}
			i.byTerm[term] = bucket
		}
		bucket[record.ID]++
	}
}

// Remove drops one record from the index.
func (i *Index) Remove(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, term := range i.terms[id] {
		if bucket, ok := i.byTerm[term]; ok {
			delete(bucket, id)
			if len(bucket) == 0 {
				delete(i.byTerm, term)
			}
		}
	}
	delete(i.terms, id)
	delete(i.docs, id)
}

// Len is the number of indexed objects.
func (i *Index) Len() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.docs)
}
