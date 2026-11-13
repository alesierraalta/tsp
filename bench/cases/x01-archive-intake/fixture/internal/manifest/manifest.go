package manifest

import (
	"example.com/archive/internal/config"
	"sort"
)

// Package manifest reads the description of one intake batch: which scans arrived, what they are,
// what they should contain. A manifest is input the service does not control, so every field is
// validated before the pipeline touches a file.

// Kinds an entry may declare. The kind decides which stage reads the object and how it is rendered.
const (
	KindScan    = "scan"
	KindPage    = "page"
	KindSidecar = "sidecar"
	KindIndex   = "index"
)

// Entry is one object a batch delivers.
type Entry struct {
	// Name is the entry's flat name. It is unique inside a batch and becomes a directory name in
	// the staging tree, which is why it is validated as one segment.
	Name string `json:"name"`
	// Kind is one of the kinds the service knows. An empty kind is derived from the name.
	Kind string `json:"kind,omitempty"`
	// Title is the human label the pages print.
	Title string `json:"title,omitempty"`
	// Tags are free labels the search index carries.
	Tags []string `json:"tags,omitempty"`
	// Pages is the number of page images the entry declares. A scan must declare at least one.
	Pages int `json:"pages,omitempty"`
	// Bytes is the size the sender declares for the container.
	Bytes int64 `json:"bytes,omitempty"`
	// Digest is the SHA-256 the container must have.
	Digest string `json:"digest,omitempty"`
	// Source is the provenance URI the report page shows.
	Source string `json:"source,omitempty"`
	// Note is free text the sender attached to the entry.
	Note string `json:"note,omitempty"`
}

// Manifest is the whole description of one batch.
type Manifest struct {
	// ID is the batch id. An empty id is minted when the batch is accepted.
	ID string `json:"id,omitempty"`
	// Tenant is the tenant the batch belongs to. It must be one of the configured tenants.
	Tenant string `json:"tenant"`
	// Operator is the subject who delivered the batch, for the audit trail.
	Operator string `json:"operator,omitempty"`
	// Created is the instant the sender says the batch was produced, RFC3339 in UTC.
	Created string `json:"created"`
	// Note is free text about the batch as a whole.
	Note string `json:"note,omitempty"`
	// Entries are the objects the batch delivers.
	Entries []Entry `json:"entries"`
}

// Limits is the cap set a manifest is validated against. It is the deployment's own limit set, so a
// manifest is never held to a number the configuration does not carry.
type Limits = config.Limits

// EntryByName returns the entry with the given name and whether it was found. Names are compared
// byte for byte, like every id in the service.
func (m Manifest) EntryByName(name string) (Entry, bool) {
	for _, e := range m.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Names lists the entry names in the order the manifest declared them.
func (m Manifest) Names() []string {
	names := make([]string, 0, len(m.Entries))
	for _, e := range m.Entries {
		names = append(names, e.Name)
	}
	return names
}

// SortedNames lists the entry names in sorted order, which is the order the report page prints them.
func (m Manifest) SortedNames() []string {
	names := m.Names()
	sort.Strings(names)
	return names
}

// TotalBytes is the size the manifest declares for the whole batch. The sum saturates instead of
// wrapping, so a manifest with absurd sizes cannot claim a small total.
func (m Manifest) TotalBytes() int64 {
	var total int64
	for _, e := range m.Entries {
		if e.Bytes > 0 && total > (1<<62)-e.Bytes {
			return 1 << 62
		}
		total += e.Bytes
	}
	return total
}

// Kinds counts the entries per kind, for the report's summary line.
func (m Manifest) Kinds() map[string]int {
	counts := map[string]int{}
	for _, e := range m.Entries {
		counts[e.KindOf()]++
	}
	return counts
}

// Pages is the number of page images the manifest declares across all entries.
func (m Manifest) Pages() int {
	total := 0
	for _, e := range m.Entries {
		total += e.Pages
	}
	return total
}
