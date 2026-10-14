package catalog

import "strings"

type Filter struct {
	Group string
	Tag   string
	Query string
}

func (f Filter) Match(r Record) bool {
	if f.Group != "" && r.Group != f.Group {
		return false
	}
	if f.Tag != "" && !r.HasTag(f.Tag) {
		return false
	}
	if f.Query != "" {
		title := strings.ToLower(r.Title)
		if !strings.Contains(title, strings.ToLower(f.Query)) {
			return false
		}
	}
	return true
}

func Search(s *Store, f Filter) []Record {
	out := []Record{}
	for _, id := range s.IDs() {
		r, _ := s.Get(id)
		if f.Match(r) {
			out = append(out, r)
		}
	}
	return out
}

func Groups(records []Record) map[string]int {
	counts := make(map[string]int)
	for _, r := range records {
		counts[GroupLabel(r)]++
	}
	return counts
}
