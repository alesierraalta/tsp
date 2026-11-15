package store

import (
	"errors"
	"example.com/archive/internal/clock"
	"time"
)

// Package store keeps the committed objects of the archive: a metadata record per object and one
// content-addressed blob per digest. It is the only package that writes below the objects directory,
// and it resolves every path through storepath.

// Errors the store returns. A handler maps ErrNotFound to 404 and ErrExists to 409.
var (
	ErrNotFound = errors.New("store: no such object")
	ErrExists   = errors.New("store: object already committed")
	ErrInvalid  = errors.New("store: invalid object")
)

// Record is the metadata of one committed object.
type Record struct {
	ID      string   `json:"id"`
	Tenant  string   `json:"tenant"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Source  string   `json:"source,omitempty"`
	Note    string   `json:"note,omitempty"`
	Batch   string   `json:"batch"`
	Entry   string   `json:"entry"`
	Bytes   int64    `json:"bytes"`
	Digest  string   `json:"digest"`
	Pages   int      `json:"pages"`
	Created string   `json:"created"`
	Commit  string   `json:"committed"`
}

// CreatedAt reads the creation instant the record carries.
func (r Record) CreatedAt() time.Time {
	t, err := clock.ParseRFC3339(r.Created)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CommittedAt reads the commit instant the record carries.
func (r Record) CommittedAt() time.Time {
	t, err := clock.ParseRFC3339(r.Commit)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Label is the value a page prints as the object's name: the title when it has one.
func (r Record) Label() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}
