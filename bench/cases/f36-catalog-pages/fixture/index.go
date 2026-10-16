package catalog

import "sort"

type Index struct {
	ids []string
}

func BuildIndex(s *Store) Index {
	return Index{ids: s.IDs()}
}

func (x Index) From(lower string) []string {
	start := sort.SearchStrings(x.ids, lower)
	return append([]string(nil), x.ids[start:]...)
}

func (x Index) Contains(id string) bool {
	n := sort.SearchStrings(x.ids, id)
	return n < len(x.ids) && x.ids[n] == id
}

func (x Index) Prefix(prefix string) []string {
	out := []string{}
	for _, id := range x.From(prefix) {
		if len(id) < len(prefix) || id[:len(prefix)] != prefix {
			break
		}
		out = append(out, id)
	}
	return out
}

func (x Index) Count() int {
	return len(x.ids)
}

func (x Index) First() string {
	if len(x.ids) == 0 {
		return ""
	}
	return x.ids[0]
}
