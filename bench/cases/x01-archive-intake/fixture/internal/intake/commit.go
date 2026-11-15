package intake

import (
	"fmt"
	"strings"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
	"example.com/archive/internal/textutil"
)

// commitEntry stores one entry. The container is re-read from the tree, its digest is recomputed by
// the store, and the extraction text is committed next to it, so a record and its bytes cannot drift.
func (r *Runner) commitEntry(layout staging.Layout, m manifest.Manifest, e manifest.Entry, scanned scanResult, text string) (EntryResult, error) {
	container, err := layout.ReadEntry(e)
	if err != nil {
		return EntryResult{}, fmt.Errorf("intake: commit %q: %w", e.Name, err)
	}
	record := store.Record{
		Tenant:  m.Tenant,
		Name:    e.Name,
		Kind:    e.KindOf(),
		Title:   entryTitle(e),
		Tags:    deriveTags(e),
		Source:  e.Source,
		Note:    e.Note,
		Batch:   m.ID,
		Entry:   e.Name,
		Pages:   len(scanned.Pages),
		Created: m.Created,
		Commit:  clock.Stamp(clock.Now(r.now)),
	}
	if e.KindOf() == manifest.KindScan {
		record.Pages = len(scanned.Pages)
	}
	stored, err := r.store.Put(record, container, []byte(text))
	if err != nil {
		return EntryResult{}, fmt.Errorf("intake: commit %q: %w", e.Name, err)
	}
	return EntryResult{
		Name:   e.Name,
		Object: stored.ID,
		Kind:   stored.Kind,
		Title:  stored.Title,
		Pages:  stored.Pages,
		Bytes:  stored.Bytes,
		Digest: checksum.Short(stored.Digest),
		Text:   text,
	}, nil
}

// entryTitle is the title an entry is committed with: the declared one, or one derived from its name.
func entryTitle(e manifest.Entry) string {
	if strings.TrimSpace(e.Title) != "" {
		return textutil.Collapse(e.Title)
	}
	return DeriveTitle(e)
}

// deriveTags is the tag list an entry is committed with. The declared tags are kept in order, the kind
// is added in front, and the list is capped at the manifest's tag cap so a listing row stays short.
func deriveTags(e manifest.Entry) []string {
	tags := make([]string, 0, len(e.Tags)+1)
	tags = append(tags, e.KindOf())
	for _, tag := range e.Tags {
		dup := false
		for _, kept := range tags {
			if kept == tag {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		tags = append(tags, tag)
	}
	if len(tags) > manifest.MaxTags {
		tags = tags[:manifest.MaxTags]
	}
	return tags
}

// commitRecord rebuilds the record a committed object carries, for the report page that shows a batch
// after the fact. It reads the store, never the staging tree: the tree may be gone by then.
func commitRecord(record store.Record) map[string]string {
	return map[string]string{
		"id":      record.ID,
		"name":    record.Name,
		"kind":    record.Kind,
		"title":   record.Label(),
		"tenant":  record.Tenant,
		"batch":   record.Batch,
		"pages":   fmt.Sprintf("%d", record.Pages),
		"bytes":   fmt.Sprintf("%d", record.Bytes),
		"digest":  checksum.Short(record.Digest),
		"created": record.Created,
		"commit":  record.Commit,
	}
}
