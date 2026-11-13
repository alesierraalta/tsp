package intake

import (
	"context"
	"example.com/archive/internal/checksum"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resume continues a batch that was interrupted. A batch is interrupted when the process stopped
// between staging and commit: its manifest, containers and page images are still in the tree, its
// committed objects are still in the store, and the resume runs the stages again for the entries that
// did not commit.
//
// The container was verified when the batch was accepted, so a resume does not repeat the size and
// digest checks on it; it rebuilds what the interruption lost, which is the extracted text of every
// page, and commits the entries the store does not carry yet.
func (r *Runner) Resume(ctx context.Context, batchID string) (Result, error) {
	layout, err := r.staging.Layout(batchID)
	if err != nil {
		return Result{}, err
	}
	if !layout.Exists() {
		return Result{}, fmt.Errorf("%w: batch %s", staging.ErrMissing, batchID)
	}
	m, err := layout.ReadManifest()
	if err != nil {
		return Result{}, err
	}
	result := Result{Batch: batchID, Tenant: m.Tenant, Started: clock.Stamp(clock.Now(r.now))}
	committed := r.Committed(batchID)
	if at, done := layout.CommittedAt(); done {
		result.Committed = true
		result.Finished = clock.Stamp(at)
		for _, e := range m.Entries {
			if record, ok := committed[e.Name]; ok {
				result.Entries = append(result.Entries, EntryResult{
					Name:   record.Entry,
					Object: record.ID,
					Kind:   record.Kind,
					Pages:  record.Pages,
					Bytes:  record.Bytes,
					Digest: checksum.Short(record.Digest),
				})
				result.Pages += record.Pages
				result.Bytes += record.Bytes
			}
		}
		return result, nil
	}
	for _, e := range m.Entries {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("intake: resume %s cancelled: %w", batchID, err)
		}
		if record, ok := committed[e.Name]; ok {
			result.Entries = append(result.Entries, EntryResult{
				Name:   record.Entry,
				Object: record.ID,
				Kind:   record.Kind,
				Pages:  record.Pages,
				Bytes:  record.Bytes,
				Digest: checksum.Short(record.Digest),
				Warn:   "already committed",
			})
			result.Pages += record.Pages
			result.Bytes += record.Bytes
			continue
		}
		entry, err := r.resumeEntry(ctx, layout, m, e)
		if err != nil {
			result.Entries = append(result.Entries, EntryResult{Name: e.Name, Kind: e.KindOf(), Err: err.Error()})
			continue
		}
		result.Entries = append(result.Entries, entry)
		result.Pages += entry.Pages
		result.Bytes += entry.Bytes
	}
	if result.Failed() > 0 {
		return result, nil
	}
	if err := layout.MarkCommitted(clock.Now(r.now)); err != nil {
		return result, err
	}
	result.Committed = true
	result.Finished = clock.Stamp(clock.Now(r.now))
	return result, nil
}

// resumeEntry runs the stages of one entry of an interrupted batch.
func (r *Runner) resumeEntry(ctx context.Context, layout staging.Layout, m manifest.Manifest, e manifest.Entry) (EntryResult, error) {
	if err := ctx.Err(); err != nil {
		return EntryResult{}, err
	}
	container, err := layout.ReadEntry(e)
	if err != nil {
		return EntryResult{}, fmt.Errorf("intake: resume %q: %w", e.Name, err)
	}
	if e.KindOf() != manifest.KindScan {
		text, err := r.extractEntry(layout, e, nil)
		if err != nil {
			return EntryResult{}, err
		}
		if err := layout.WriteSidecar(e, []byte(text)); err != nil {
			return EntryResult{}, err
		}
		return r.commitEntry(layout, m, e, scanResult{Container: container, Bytes: int64(len(container))}, text)
	}
	pages, text, err := r.rebuildPages(layout, e)
	if err != nil {
		return EntryResult{}, err
	}
	if err := layout.WriteSidecar(e, []byte(text)); err != nil {
		return EntryResult{}, err
	}
	scanned := scanResult{Container: container, Pages: pages, Bytes: int64(len(container))}
	return r.commitEntry(layout, m, e, scanned, text)
}

// rebuildPages rebuilds the page list and the extracted text of a scan whose sidecar the interruption
// left behind. The page index names the pages relative to the entry's own directory, so the resume
// reads each one from the tree the scanner wrote and re-derives the digest the index carries.
func (r *Runner) rebuildPages(layout staging.Layout, e manifest.Entry) ([]staging.Page, string, error) {
	pages, err := layout.ReadPageIndex(e)
	if err != nil {
		return nil, "", fmt.Errorf("intake: resume %q: %w", e.Name, err)
	}
	if e.Pages > 0 && len(pages) != e.Pages {
		return nil, "", fmt.Errorf("intake: resume %q declares %d pages and its index names %d", e.Name, e.Pages, len(pages))
	}
	var b strings.Builder
	for _, p := range pages {
		target := filepath.Join(layout.Dir(), filepath.FromSlash(e.Dir()), filepath.FromSlash(p.File))
		body, err := os.ReadFile(target)
		if err != nil {
			return nil, "", fmt.Errorf("intake: resume %q page %d: %w", e.Name, p.Number, err)
		}
		if p.Digest != "" && !checksum.Match(p.Digest, body) {
			return nil, "", fmt.Errorf("intake: resume %q page %d does not match its digest", e.Name, p.Number)
		}
		p.Bytes = int64(len(body))
		b.WriteString(extractPage(p, body))
	}
	return pages, b.String(), nil
}
