package staging

import (
	"errors"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/storepath"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// clockTime is the duration a sweep compares a batch's timestamp to.
type clockTime = time.Duration

// Layout is the working tree of one batch. It is a value: handing it around copies the path, never
// the files.
type Layout struct {
	mgr   *Manager
	batch string
	dir   string
}

// Batch is the batch id the layout belongs to.
func (l Layout) Batch() string { return l.batch }

// Dir is the absolute directory of the batch. It is exported for the resume path, which rewrites the
// tree in place; every file inside it is still resolved through this package.
func (l Layout) Dir() string { return l.dir }

// Exists reports whether the batch has a directory.
func (l Layout) Exists() bool {
	info, err := os.Stat(l.dir)
	return err == nil && info.IsDir()
}

// Committed marks the batch as finished: the pipeline writes this file once the store holds every
// entry, so a resume knows there is nothing left to do.
const CommittedFile = "committed"

// ManifestFile is the name the delivered manifest is kept under.
const ManifestFile = "manifest.json"

// resolved returns the absolute path of a name inside the batch directory. Every read and write in
// this package goes through it, so a name that leaves the tree is refused before the filesystem is
// asked about it.
func (l Layout) resolved(name string) (string, error) {
	p, err := storepath.Resolve(l.dir, name)
	if err != nil {
		return "", err
	}
	return p, nil
}

// EntryDir is the directory of one entry: scans/<name> for a scan, sidecars/<name> for a sidecar.
func (l Layout) EntryDir(e manifest.Entry) string {
	return filepath.Join(l.dir, filepath.FromSlash(e.Dir()))
}

// WriteManifest stores the manifest the batch was accepted with. Writing it twice is an error: a
// second manifest would describe a different batch under the same id.
func (l Layout) WriteManifest(m manifest.Manifest) error {
	body, err := manifest.Encode(m)
	if err != nil {
		return err
	}
	target, err := l.resolved(ManifestFile)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, l.batch)
	}
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("staging: create %s: %w", l.dir, err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return fmt.Errorf("staging: write manifest: %w", err)
	}
	return nil
}

// ReadManifest reads the manifest back. A batch directory without one is ErrMissing, not an empty
// manifest: a pipeline that cannot find what it is importing must stop.
func (l Layout) ReadManifest() (manifest.Manifest, error) {
	target, err := l.resolved(ManifestFile)
	if err != nil {
		return manifest.Manifest{}, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return manifest.Manifest{}, fmt.Errorf("%w: %s", ErrMissing, ManifestFile)
		}
		return manifest.Manifest{}, fmt.Errorf("staging: read manifest: %w", err)
	}
	return manifest.Decode(body, l.mgr.lim)
}

// WriteEntry stores an entry's container. The container is the scan's PDF or the index CSV; the
// pages and the text sidecar are written separately.
func (l Layout) WriteEntry(e manifest.Entry, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("%w: entry %q is empty", ErrInvalid, e.Name)
	}
	if int64(len(data)) > l.mgr.lim.MaxUpload {
		return fmt.Errorf("%w: entry %q carries %d bytes", ErrInvalid, e.Name, len(data))
	}
	rel := e.Dir() + "/" + e.ContainerFile()
	return l.write(rel, data)
}

// ReadEntry reads an entry's container through the containment rule.
func (l Layout) ReadEntry(e manifest.Entry) ([]byte, error) {
	return l.read(e.Dir() + "/" + e.ContainerFile())
}

// EntryExists reports whether the entry's container is in the tree.
func (l Layout) EntryExists(e manifest.Entry) bool {
	_, err := l.ReadEntry(e)
	return err == nil
}

// WriteSidecar stores the extracted text of one entry. The pipeline writes it after extraction and
// the commit stage reads it, so a resume does not extract twice.
func (l Layout) WriteSidecar(e manifest.Entry, data []byte) error {
	return l.write(e.Dir()+"/text.txt", data)
}

// ReadSidecar reads the extracted text of an entry.
func (l Layout) ReadSidecar(e manifest.Entry) ([]byte, error) {
	body, err := l.read(e.Dir() + "/text.txt")
	if errors.Is(err, ErrMissing) {
		return nil, fmt.Errorf("%w: sidecar for %q", ErrMissing, e.Name)
	}
	return body, err
}

// WritePage stores one page image of an entry. The page number decides the file name; a caller never
// supplies the name.
func (l Layout) WritePage(e manifest.Entry, page int, data []byte) error {
	if page < 1 {
		return fmt.Errorf("%w: page %d", ErrInvalid, page)
	}
	if int64(len(data)) > l.mgr.lim.MaxUpload {
		return fmt.Errorf("%w: page %d carries %d bytes", ErrInvalid, page, len(data))
	}
	return l.write(e.Dir()+"/"+e.PageFile(page), data)
}

// OpenPage reads one page image through the containment rule. Every reader of a page goes through
// here, so a file name from a page index is checked before the filesystem is asked about it.
func (l Layout) OpenPage(e manifest.Entry, p Page) ([]byte, error) {
	if p.Number < 1 {
		return nil, fmt.Errorf("%w: page %d", ErrInvalid, p.Number)
	}
	return l.read(e.Dir() + "/" + p.File)
}

// WritePageIndex stores the page index the scanner wrote. The index is a sidecar of the scan; its
// lines name the page files of the entry.
func (l Layout) WritePageIndex(e manifest.Entry, pages []Page) error {
	return l.write(e.Dir()+"/"+manifest.IndexFile, RenderIndex(pages))
}

// PageFiles lists the page images the entry directory holds, sorted by name. It is the listing the
// scan stage uses when a manifest declares no page index of its own.
func (l Layout) PageFiles(e manifest.Entry) ([]Page, error) {
	dir, err := l.resolved(e.Dir())
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: entry %q", ErrMissing, e.Name)
		}
		return nil, fmt.Errorf("staging: list %q: %w", e.Name, err)
	}
	var pages []Page
	for _, entry := range entries {
		if entry.IsDir() || storepath.Kind(entry.Name()) != "page" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("staging: stat %q: %w", entry.Name(), err)
		}
		pages = append(pages, Page{File: entry.Name(), Bytes: info.Size()})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].File < pages[j].File })
	for i := range pages {
		pages[i].Number = i + 1
	}
	return pages, nil
}
