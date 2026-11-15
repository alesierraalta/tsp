package intake

import (
	"fmt"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
)

// scanResult is what the scan stage learned about one entry: the container bytes, the pages the index
// names, and the text the scanner itself delivered, if any.
type scanResult struct {
	Container []byte
	Pages     []staging.Page
	Sidecar   string
	Bytes     int64
}

// scanEntry verifies one entry against the manifest: the container is present and matches the digest
// the sender declared, the page index is dense, and every page the index names exists and matches its
// digest. A mismatch stops the entry; the other entries of the batch continue.
func (r *Runner) scanEntry(layout staging.Layout, m manifest.Manifest, e manifest.Entry) (scanResult, error) {
	container, err := layout.ReadEntry(e)
	if err != nil {
		return scanResult{}, fmt.Errorf("intake: read %q: %w", e.Name, err)
	}
	if int64(len(container)) > r.lim.MaxUpload {
		return scanResult{}, fmt.Errorf("intake: %q carried %d bytes, more than %d", e.Name, len(container), r.lim.MaxUpload)
	}
	if e.Bytes > 0 && int64(len(container)) != e.Bytes {
		return scanResult{}, fmt.Errorf("intake: %q declares %d bytes and carries %d", e.Name, e.Bytes, len(container))
	}
	if e.Digest != "" && !checksum.Match(e.Digest, container) {
		return scanResult{}, fmt.Errorf("intake: %q does not match the digest it declares", e.Name)
	}
	out := scanResult{Container: container, Bytes: int64(len(container))}
	if e.KindOf() != manifest.KindScan {
		if sidecar, err := layout.ReadSidecar(e); err == nil {
			out.Sidecar = string(sidecar)
		}
		return out, nil
	}
	pages, err := layout.ReadPageIndex(e)
	if err != nil {
		return out, fmt.Errorf("intake: %q: %w", e.Name, err)
	}
	if e.Pages > 0 && len(pages) != e.Pages {
		return out, fmt.Errorf("intake: %q declares %d pages and its index names %d", e.Name, e.Pages, len(pages))
	}
	if err := staging.PageCount(pages); err != nil {
		return out, fmt.Errorf("intake: %q: %w", e.Name, err)
	}
	// Every page is read through the layout, which is what keeps a file name from a page index inside
	// the batch's own tree.
	if err := layout.VerifyPages(e, pages); err != nil {
		return out, fmt.Errorf("intake: %q: %w", e.Name, err)
	}
	out.Pages = pages
	return out, nil
}

// ScanOnly runs the scan stage of one entry without extracting or committing. The admin surface uses
// it to answer "would this batch pass verification" before an operator resumes one.
func (r *Runner) ScanOnly(batch, entry string) (scanResult, error) {
	layout, err := r.staging.Layout(batch)
	if err != nil {
		return scanResult{}, err
	}
	m, err := layout.ReadManifest()
	if err != nil {
		return scanResult{}, err
	}
	e, ok := m.EntryByName(entry)
	if !ok {
		return scanResult{}, fmt.Errorf("%w: entry %q", staging.ErrMissing, entry)
	}
	return r.scanEntry(layout, m, e)
}

// Describe is the one-line summary of a scan the admin page prints.
func (s scanResult) Describe() string {
	return fmt.Sprintf("%d bytes, %d pages", s.Bytes, len(s.Pages))
}
