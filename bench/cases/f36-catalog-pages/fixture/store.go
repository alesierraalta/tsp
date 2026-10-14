package catalog

import (
	"errors"
	"sort"
)

type Store struct {
	records map[string]Record
}

func NewStore() *Store {
	return &Store{records: make(map[string]Record)}
}

func (s *Store) Put(r Record) error {
	if r.ID == "" || r.Title == "" {
		return errors.New("id and title required")
	}
	s.records[r.ID] = clone(r)
	return nil
}

func (s *Store) Get(id string) (Record, bool) {
	r, ok := s.records[id]
	return clone(r), ok
}

func (s *Store) Delete(id string) bool {
	_, ok := s.records[id]
	delete(s.records, id)
	return ok
}

func (s *Store) IDs() []string {
	ids := make([]string, 0, len(s.records))
	for id := range s.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *Store) Len() int {
	return len(s.records)
}
