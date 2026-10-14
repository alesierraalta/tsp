package catalog

import (
	"errors"
	"strings"
)

type Record struct {
	ID    string
	Title string
	Group string
	Tags  []string
}

func NewRecord(id, title, group string) (Record, error) {
	if strings.TrimSpace(id) == "" {
		return Record{}, errors.New("id required")
	}
	if strings.TrimSpace(title) == "" {
		return Record{}, errors.New("title required")
	}
	return Record{ID: id, Title: title, Group: group}, nil
}

func clone(r Record) Record {
	r.Tags = append([]string(nil), r.Tags...)
	return r
}

func (r Record) HasTag(tag string) bool {
	for _, candidate := range r.Tags {
		if candidate == tag {
			return true
		}
	}
	return false
}

func (r Record) WithTag(tag string) Record {
	r = clone(r)
	if tag != "" && !r.HasTag(tag) {
		r.Tags = append(r.Tags, tag)
	}
	return r
}
