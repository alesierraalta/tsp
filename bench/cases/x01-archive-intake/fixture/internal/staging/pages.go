package staging

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/storepath"
)

// Page is one line of a scan's page index: the page number, the file the scanner wrote it to, and
// the digest the scanner computed for it. The file is relative to the entry's directory.
type Page struct {
	Number int
	File   string
	Digest string
	Bytes  int64
}

// RenderIndex renders pages as the page index sidecar the scanner writes: one tab-separated line per
// page, numbers zero-padded to three digits, in page order.
func RenderIndex(pages []Page) []byte {
	var b strings.Builder
	b.WriteString("# page\tfile\tdigest\n")
	sorted := append([]Page(nil), pages...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Number < sorted[j].Number })
	for _, p := range sorted {
		fmt.Fprintf(&b, "%03d\t%s\t%s\n", p.Number, p.File, p.Digest)
	}
	return []byte(b.String())
}

// ParseIndex reads a page index. It checks the numbers and the digests, and keeps the file column as
// the name it is: the file is opened through the layout, which is where the containment rule is
// applied. A line whose file column is not in the entry's own directory is refused by that open.
func ParseIndex(body []byte, maxPages int) ([]Page, error) {
	lines := strings.Split(string(body), "\n")
	pages := make([]Page, 0, len(lines))
	seen := map[int]bool{}
	for i, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("staging: page index line %d: %d fields, want 3", i+1, len(fields))
		}
		number, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || number < 1 {
			return nil, fmt.Errorf("staging: page index line %d: %q is not a page number", i+1, fields[0])
		}
		if seen[number] {
			return nil, fmt.Errorf("staging: page index line %d: page %d repeated", i+1, number)
		}
		seen[number] = true
		file := strings.TrimSpace(fields[1])
		if file == "" {
			return nil, fmt.Errorf("staging: page index line %d: no file", i+1)
		}
		if err := checksum.Validate(strings.TrimSpace(fields[2])); err != nil {
			return nil, fmt.Errorf("staging: page index line %d: %w", i+1, err)
		}
		if len(pages) >= maxPages {
			return nil, fmt.Errorf("staging: page index carries more than %d pages", maxPages)
		}
		pages = append(pages, Page{Number: number, File: file, Digest: strings.TrimSpace(fields[2])})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("staging: page index is empty")
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Number < pages[j].Number })
	return pages, nil
}

// ReadPageIndex reads and parses the page index of one entry. A scan whose index is absent is an
// error: the scanner always writes one, and guessing the page order from file names would hand the
// pipeline a different order than the scan declared.
func (l Layout) ReadPageIndex(e manifest.Entry) ([]Page, error) {
	body, err := l.read(e.Dir() + "/" + manifest.IndexFile)
	if err != nil {
		return nil, err
	}
	return ParseIndex(body, l.mgr.lim.MaxPages)
}

// VerifyPages reads every page the index names through OpenPage and compares its digest. It returns
// the pages it verified and refuses the first mismatch, naming the page.
func (l Layout) VerifyPages(e manifest.Entry, pages []Page) error {
	for _, p := range pages {
		body, err := l.OpenPage(e, p)
		if err != nil {
			return err
		}
		if p.Digest != "" && !checksum.Match(p.Digest, body) {
			return fmt.Errorf("staging: page %d of %q does not match its digest", p.Number, e.Name)
		}
	}
	return nil
}

// PageCount is the number of pages an index declares, refusing an index that is not dense from 1.
// A scanner that skips a page number is describing a different document than the one delivered.
func PageCount(pages []Page) error {
	for i, p := range pages {
		if p.Number != i+1 {
			return fmt.Errorf("staging: page index jumps from %d to %d", i, p.Number)
		}
	}
	return nil
}

// PageInDir reports whether the file the index names sits in the entry's own directory. It is the
// shape check the report prints when it explains why a name was refused.
func PageInDir(file string) bool {
	return storepath.Slash(file) == storepath.Base(file)
}
