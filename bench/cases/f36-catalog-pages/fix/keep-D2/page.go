package catalog

import "errors"

type Page struct {
	Records []Record
	Next    string
}

func List(s *Store, after string, limit int) (Page, error) {
	if limit < 1 || limit > 1000 {
		return Page{}, errors.New("limit must be between 1 and 1000")
	}
	index := BuildIndex(s)
	ids := index.From(after + "\x00")
	page := Page{Records: make([]Record, 0, limit)}
	for _, id := range ids {
		if len(page.Records) == limit {
			break
		}
		r, ok := s.Get(id)
		if ok {
			page.Records = append(page.Records, r)
		}
	}
	if len(ids) > len(page.Records) && len(page.Records) > 0 {
		page.Next = page.Records[len(page.Records)-1].ID
	}
	return page, nil
}

func (p Page) IDs() []string {
	ids := make([]string, len(p.Records))
	for i, r := range p.Records {
		ids[i] = r.ID
	}
	return ids
}

func (p Page) Empty() bool {
	return len(p.Records) == 0
}
