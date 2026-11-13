package vault

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Doc is the metadata the service keeps for one object. The object bytes live on disk; everything
// the API renders about a document comes from this value.
type Doc struct {
	ID      string    `json:"id"`
	Key     string    `json:"key"`
	Tenant  string    `json:"tenant"`
	Title   string    `json:"title"`
	Kind    string    `json:"kind"`
	Owner   string    `json:"owner"`
	Tags    []string  `json:"tags"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// Kinds are the document kinds the API accepts. A kind decides how a document is rendered, not
// where it is stored.
var Kinds = []string{"article", "note", "invoice", "receipt"}

// IsKind reports whether kind is one the API accepts.
func IsKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Store is the in-memory metadata index. It is safe for concurrent use; the object tree it
// describes is not guarded here.
type Store struct {
	mu   sync.RWMutex
	docs map[string]Doc
}

// NewStore returns an empty metadata store.
func NewStore() *Store {
	return &Store{docs: make(map[string]Doc)}
}

// ErrNotFound is returned by the store for a key it does not carry.
var ErrNotFound = errors.New("vault: document not found")

// Put inserts or replaces the metadata of one key. A replacing write keeps the original creation
// time, which is what the audit trail compares against.
func (s *Store) Put(doc Doc) Doc {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev, ok := s.docs[doc.Key]; ok {
		doc.Created = prev.Created
	} else if doc.Created.IsZero() {
		doc.Created = time.Now().UTC()
	}
	if doc.Updated.IsZero() {
		doc.Updated = time.Now().UTC()
	}
	s.docs[doc.Key] = doc
	return doc
}

// Get returns the metadata of one key.
func (s *Store) Get(k string) (Doc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[k]
	if !ok {
		return Doc{}, fmt.Errorf("%w: %q", ErrNotFound, k)
	}
	return doc, nil
}

// ByID looks a document up by its public identifier.
func (s *Store) ByID(id string) (Doc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, doc := range s.docs {
		if doc.ID == id {
			return doc, nil
		}
	}
	return Doc{}, fmt.Errorf("%w: id %q", ErrNotFound, id)
}

// Delete drops the metadata of one key.
func (s *Store) Delete(k string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, k)
}

// Query selects documents of one tenant. Limit is applied after filtering, so a page never
// silently changes size with the number of documents the tenant has.
type Query struct {
	Tenant string
	Kind   string
	Tag    string
	Prefix string
	Limit  int
	Offset int
}

// List returns the documents a query selects, newest first.
func (s *Store) List(q Query) []Doc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Doc
	for _, doc := range s.docs {
		if q.Tenant != "" && doc.Tenant != q.Tenant {
			continue
		}
		if q.Kind != "" && doc.Kind != q.Kind {
			continue
		}
		if q.Tag != "" && !hasTag(doc.Tags, q.Tag) {
			continue
		}
		if q.Prefix != "" && !strings.HasPrefix(doc.Key, q.Prefix) {
			continue
		}
		out = append(out, doc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Updated.Equal(out[j].Updated) {
			return out[i].ID < out[j].ID
		}
		return out[i].Updated.After(out[j].Updated)
	})
	if q.Offset > 0 {
		if q.Offset >= len(out) {
			return nil
		}
		out = out[q.Offset:]
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out
}

// Count reports how many documents the store carries for a tenant.
func (s *Store) Count(tenant string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, doc := range s.docs {
		if tenant == "" || doc.Tenant == tenant {
			n++
		}
	}
	return n
}

// Tenants lists the tenants the store carries metadata for, sorted.
func (s *Store) Tenants() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, doc := range s.docs {
		seen[doc.Tenant] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
