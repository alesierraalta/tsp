package store

import (
	"errors"
	"example.com/archive/internal/checksum"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/idgen"
	"example.com/archive/internal/storepath"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Stats is the store's summary for the operator page.
type Stats struct {
	Objects int            `json:"objects"`
	Bytes   int64          `json:"bytes"`
	Pages   int            `json:"pages"`
	Kinds   map[string]int `json:"kinds"`
	Tenants map[string]int `json:"tenants"`
	Blobs   int            `json:"blobs"`
}

// Store is one open object store. The index is held in memory and rebuilt from the metadata files
// when the store is opened, so a restart serves the same objects.
type Store struct {
	root  string
	base  string
	lim   config.Limits
	clock clock.Clock
	mu    sync.RWMutex
	index map[string]Record
}

// Open prepares the store below root and loads the metadata already there. A root that is not
// absolute is refused for the same reason the staging tree refuses one.
func Open(root string, lim config.Limits, now clock.Clock) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("store: no root")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("store: root %q is not absolute", root)
	}
	clean := filepath.Clean(root)
	base := filepath.Join(clean, "objects")
	s := &Store{root: clean, base: base, lim: lim.Or(), clock: now, index: map[string]Record{}}
	for _, dir := range []string{"blobs", "meta", "text"} {
		target, err := s.path(dir)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return nil, fmt.Errorf("store: create %s: %w", target, err)
		}
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Root is the deployment root the store lives under.
func (s *Store) Root() string { return s.root }

// Limits are the caps the store enforces on a write.
func (s *Store) Limits() config.Limits { return s.lim }

// path resolves a name below the objects directory through the containment rule.
func (s *Store) path(name string) (string, error) {
	p, err := storepath.Resolve(s.base, name)
	if err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	return p, nil
}

// load rebuilds the index from the metadata directory. A metadata file that does not parse is
// reported rather than skipped: a half-read store would answer with the wrong objects.
func (s *Store) load() error {
	dir, err := s.path("meta")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("store: read meta: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.readMeta(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return err
		}
		s.index[record.ID] = record
	}
	return nil
}

// Put commits one object. The digest is recomputed from the bytes, so a record cannot claim content
// it does not carry, and an object id may be committed once.
func (s *Store) Put(record Record, container, text []byte) (Record, error) {
	if len(container) == 0 {
		return Record{}, fmt.Errorf("%w: %q has no bytes", ErrInvalid, record.Name)
	}
	if int64(len(container)) > s.lim.MaxUpload {
		return Record{}, fmt.Errorf("%w: %q carries %d bytes, more than %d", ErrInvalid, record.Name, len(container), s.lim.MaxUpload)
	}
	if record.Name == "" {
		return Record{}, fmt.Errorf("%w: no name", ErrInvalid)
	}
	if record.Tenant == "" {
		return Record{}, fmt.Errorf("%w: no tenant", ErrInvalid)
	}
	digest := checksum.Sum(container)
	if record.Digest != "" && record.Digest != digest {
		return Record{}, fmt.Errorf("%w: %q declares %s and carries %s", ErrInvalid, record.Name, record.Digest, digest)
	}
	record.Digest = digest
	record.Bytes = int64(len(container))
	if record.ID == "" {
		record.ID = idgen.New(idgen.Doc)
	}
	if !idgen.Valid(idgen.Doc, record.ID) {
		return Record{}, fmt.Errorf("%w: id %q", ErrInvalid, record.ID)
	}
	if record.Commit == "" {
		record.Commit = clock.Stamp(clock.Now(s.clock))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.index[record.ID]; exists {
		return Record{}, fmt.Errorf("%w: %s", ErrExists, record.ID)
	}
	if err := s.writeBlob(digest, container); err != nil {
		return Record{}, err
	}
	if err := s.writeText(record.ID, text); err != nil {
		return Record{}, err
	}
	if err := s.writeMeta(record); err != nil {
		return Record{}, err
	}
	s.index[record.ID] = record
	return record, nil
}

// Get returns one object's metadata.
func (s *Store) Get(id string) (Record, error) {
	if !idgen.Valid(idgen.Doc, id) {
		return Record{}, fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.index[id]
	if !ok {
		return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return record, nil
}

// Read returns one object's container bytes.
func (s *Store) Read(id string) ([]byte, error) {
	record, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return s.readBlob(record.Digest)
}

// ReadText returns the text extracted for one object, which may be empty.
func (s *Store) ReadText(id string) (string, error) {
	if _, err := s.Get(id); err != nil {
		return "", err
	}
	body, err := s.readTextFile(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(body), nil
}

// Delete removes one object's metadata and text. The blob stays while another object carries the same
// digest, so two entries of one batch do not delete each other's content.
func (s *Store) Delete(id string) error {
	record, err := s.Get(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metaPath, err := s.metaPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: remove meta: %w", err)
	}
	delete(s.index, id)
	textPath, err := s.textPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(textPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: remove text: %w", err)
	}
	shared := false
	for _, other := range s.index {
		if other.Digest == record.Digest {
			shared = true
			break
		}
	}
	if !shared {
		blob, err := s.blobPath(record.Digest)
		if err != nil {
			return err
		}
		if err := os.Remove(blob); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: remove blob: %w", err)
		}
	}
	return nil
}

// Stats summarizes the store for the operator page.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := Stats{Objects: len(s.index), Kinds: map[string]int{}, Tenants: map[string]int{}}
	blobs := map[string]bool{}
	for _, record := range s.index {
		stats.Bytes += record.Bytes
		stats.Pages += record.Pages
		stats.Kinds[record.Kind]++
		stats.Tenants[record.Tenant]++
		blobs[record.Digest] = true
	}
	stats.Blobs = len(blobs)
	return stats
}

// Count is the number of committed objects.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.index)
}

// All returns every record, sorted by commit instant, newest first. It is the listing the export and
// the retention sweep read.
func (s *Store) All() []Record {
	s.mu.RLock()
	records := make([]Record, 0, len(s.index))
	for _, record := range s.index {
		records = append(records, record)
	}
	s.mu.RUnlock()
	sort.Slice(records, func(i, j int) bool {
		if records[i].Commit == records[j].Commit {
			return records[i].ID < records[j].ID
		}
		return records[i].Commit > records[j].Commit
	})
	return records
}

// Tenants lists the tenants that own at least one object, sorted.
func (s *Store) Tenants() []string {
	seen := map[string]bool{}
	for _, record := range s.All() {
		seen[record.Tenant] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
