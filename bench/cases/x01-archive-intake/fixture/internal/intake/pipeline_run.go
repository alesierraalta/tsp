package intake

import (
	"context"
	"example.com/archive/internal/checksum"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/idgen"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
	"fmt"
	"sort"
	"time"
)

// stageUpload writes what the delivery carried into the batch's tree. Every part is written through
// the layout, so a part whose name the layout refuses never reaches a file.
func (r *Runner) stageUpload(layout staging.Layout, m manifest.Manifest, upload Upload) error {
	for _, e := range m.Entries {
		container := upload.Containers[e.Name]
		if len(container) == 0 {
			return fmt.Errorf("intake: entry %q arrived with no container", e.Name)
		}
		if e.Bytes > 0 && int64(len(container)) != e.Bytes {
			return fmt.Errorf("intake: entry %q declares %d bytes and carries %d", e.Name, e.Bytes, len(container))
		}
		if e.Digest != "" && !checksum.Match(e.Digest, container) {
			return fmt.Errorf("intake: entry %q does not match the digest it declares", e.Name)
		}
		if err := layout.WriteEntry(e, container); err != nil {
			return err
		}
		if e.KindOf() != manifest.KindScan {
			if sidecar := upload.Sidecars[e.Name]; len(sidecar) > 0 {
				if err := layout.WriteSidecar(e, sidecar); err != nil {
					return err
				}
			}
			continue
		}
		pages, err := r.stagePages(layout, e, upload.Pages[e.Name])
		if err != nil {
			return err
		}
		if err := layout.WritePageIndex(e, pages); err != nil {
			return err
		}
	}
	return nil
}

// stagePages writes the page images of one scan in page order and returns the index the scanner would
// have written: number, file name and digest of each page.
func (r *Runner) stagePages(layout staging.Layout, e manifest.Entry, pages map[int][]byte) ([]staging.Page, error) {
	if len(pages) == 0 {
		return nil, fmt.Errorf("intake: scan %q arrived with no pages", e.Name)
	}
	if e.Pages > 0 && len(pages) != e.Pages {
		return nil, fmt.Errorf("intake: scan %q declares %d pages and carries %d", e.Name, e.Pages, len(pages))
	}
	numbers := make([]int, 0, len(pages))
	for number := range pages {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	index := make([]staging.Page, 0, len(numbers))
	for i, number := range numbers {
		if number != i+1 {
			return nil, fmt.Errorf("intake: scan %q starts at page %d", e.Name, numbers[0])
		}
		data := pages[number]
		if int64(len(data)) > r.lim.MaxUpload {
			return nil, fmt.Errorf("intake: scan %q page %d carries %d bytes", e.Name, number, len(data))
		}
		if err := layout.WritePage(e, number, data); err != nil {
			return nil, err
		}
		index = append(index, staging.Page{Number: number, File: e.PageFile(number), Digest: checksum.Sum(data), Bytes: int64(len(data))})
	}
	return index, nil
}

// run is the stage sequence a fresh batch and a resumed batch share.
func (r *Runner) run(ctx context.Context, layout staging.Layout, m manifest.Manifest) (Result, error) {
	if !r.begin(layout.Batch()) {
		return Result{}, fmt.Errorf("intake: batch %s is already running", layout.Batch())
	}
	defer r.end(layout.Batch())
	result := Result{Batch: layout.Batch(), Tenant: m.Tenant, Started: clock.Stamp(clock.Now(r.now))}
	for _, e := range m.Entries {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("intake: batch %s cancelled: %w", layout.Batch(), err)
		}
		scanned, err := r.scanEntry(layout, m, e)
		if err != nil {
			result.Entries = append(result.Entries, EntryResult{Name: e.Name, Kind: e.KindOf(), Err: err.Error()})
			continue
		}
		text, err := r.extractEntry(layout, e, scanned.Pages)
		if err != nil {
			result.Entries = append(result.Entries, EntryResult{Name: e.Name, Kind: e.KindOf(), Err: err.Error()})
			continue
		}
		if err := layout.WriteSidecar(e, []byte(text)); err != nil {
			return result, err
		}
		committed, err := r.commitEntry(layout, m, e, scanned, text)
		if err != nil {
			result.Entries = append(result.Entries, EntryResult{Name: e.Name, Kind: e.KindOf(), Err: err.Error()})
			continue
		}
		result.Entries = append(result.Entries, committed)
		result.Pages += committed.Pages
		result.Bytes += committed.Bytes
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

// begin marks a batch as running, refusing a second concurrent run over the same tree.
func (r *Runner) begin(batch string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active[batch] {
		return false
	}
	r.active[batch] = true
	return true
}

// end clears the running mark.
func (r *Runner) end(batch string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, batch)
}

// Committed lists the objects one batch has already committed, by entry name.
func (r *Runner) Committed(batch string) map[string]store.Record {
	out := map[string]store.Record{}
	for _, record := range r.store.UnderBatch(batch) {
		out[record.Entry] = record
	}
	return out
}

// Abort removes a batch's working tree. A batch that already committed keeps its objects: aborting is
// how an operator discards a delivery, not how they delete one.
func (r *Runner) Abort(batch string) error {
	layout, err := r.staging.Layout(batch)
	if err != nil {
		return err
	}
	if _, committed := layout.CommittedAt(); committed {
		return fmt.Errorf("%w: batch %s already committed", staging.ErrInvalid, batch)
	}
	return layout.Remove()
}

// BatchID mints an id a sender may use for a batch it is about to deliver.
func (r *Runner) BatchID() string { return idgen.New(idgen.Batch) }

// Now is the runner's clock reading, which the admin pages stamp their output with.
func (r *Runner) Now() time.Time { return clock.Now(r.now) }
