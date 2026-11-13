package staging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/idgen"
	"example.com/archive/internal/manifest"
)

func manager(t *testing.T) *Manager {
	t.Helper()
	root := filepath.Join(t.TempDir(), "archive")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lim := config.Default().Limits()
	lim.MaxUpload = 1 << 20
	m, err := Open(root, lim, clock.Fixed(time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return m
}

// staged writes one entry of a scan the way the pipeline does and returns its layout.
func staged(t *testing.T, m *Manager) (Layout, manifest.Manifest) {
	t.Helper()
	batch := idgen.New(idgen.Batch)
	layout, err := m.Layout(batch)
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	entry := manifest.Entry{Name: "scan-01", Kind: manifest.KindScan, Pages: 2}
	man := manifest.Manifest{
		ID:      batch,
		Tenant:  "acme",
		Created: "2024-04-30T09:00:00Z",
		Entries: []manifest.Entry{entry},
	}
	if err := layout.WriteManifest(man); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := layout.WriteEntry(entry, []byte("container")); err != nil {
		t.Fatalf("entry: %v", err)
	}
	pages := make([]Page, 0, 2)
	for _, number := range []int{1, 2} {
		body := []byte("page body " + string(rune('0'+number)))
		if err := layout.WritePage(entry, number, body); err != nil {
			t.Fatalf("page: %v", err)
		}
		pages = append(pages, Page{Number: number, File: entry.PageFile(number), Digest: checksum.Sum(body)})
	}
	if err := layout.WritePageIndex(entry, pages); err != nil {
		t.Fatalf("index: %v", err)
	}
	return layout, man
}

func TestLayoutRoundTrip(t *testing.T) {
	m := manager(t)
	layout, man := staged(t, m)
	read, err := layout.ReadManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if read.ID != man.ID || len(read.Entries) != 1 {
		t.Fatalf("manifest = %+v", read)
	}
	entry := man.Entries[0]
	pages, err := layout.ReadPageIndex(entry)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if len(pages) != 2 || pages[0].Number != 1 {
		t.Fatalf("pages = %+v", pages)
	}
	if err := layout.VerifyPages(entry, pages); err != nil {
		t.Fatalf("verify: %v", err)
	}
	body, err := layout.OpenPage(entry, pages[1])
	if err != nil || string(body) != "page body 2" {
		t.Fatalf("page = %q err=%v", body, err)
	}
}

func TestLayoutRefusesATraversingPageName(t *testing.T) {
	m := manager(t)
	layout, man := staged(t, m)
	entry := man.Entries[0]
	page := Page{Number: 3, File: "../../../outside.pgm"}
	if _, err := layout.OpenPage(entry, page); err == nil || !strings.Contains(err.Error(), "walks upwards") {
		t.Fatalf("err = %v", err)
	}
	if _, err := layout.read("../manifest.json"); err == nil {
		t.Fatal("a traversing name was read")
	}
}

func TestLayoutRefusesASecondManifest(t *testing.T) {
	m := manager(t)
	layout, man := staged(t, m)
	if err := layout.WriteManifest(man); err == nil {
		t.Fatal("a second manifest was written")
	}
}

func TestListingAndQuota(t *testing.T) {
	m := manager(t)
	layout, _ := staged(t, m)
	listing, err := layout.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listing.Files) < 3 {
		t.Fatalf("listing = %+v", listing)
	}
	if names := listing.Entries(); len(names) != 1 || names[0] != "scan-01" {
		t.Fatalf("entries = %v", names)
	}
	quota, err := layout.Quota()
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if quota.Used == 0 || quota.Limit == 0 {
		t.Fatalf("quota = %+v", quota)
	}
	if err := layout.Reserve(quota.Limit); err == nil {
		t.Fatal("a reservation past the cap was accepted")
	}
	if err := layout.MarkCommitted(time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if _, done := layout.CommittedAt(); !done {
		t.Fatal("the batch is not marked committed")
	}
	if got, _ := layout.CommittedAt(); got.IsZero() {
		t.Fatal("the commit instant is missing")
	}
}

func TestLayoutsAndRemove(t *testing.T) {
	m := manager(t)
	layout, _ := staged(t, m)
	layouts, err := m.Layouts()
	if err != nil || len(layouts) != 1 {
		t.Fatalf("layouts = %d err=%v", len(layouts), err)
	}
	if _, err := m.Layout("bat-not-an-id"); err == nil {
		t.Fatal("a malformed batch id was accepted")
	}
	if err := layout.Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	layouts, _ = m.Layouts()
	if len(layouts) != 0 {
		t.Fatalf("layouts after remove = %d", len(layouts))
	}
}

func TestParseIndexRefusesBrokenLines(t *testing.T) {
	digest := strings.Repeat("0", 64)
	good := []byte("# page\tfile\tdigest\n001\tpage-001.pgm\t" + digest + "\n")
	if pages, err := ParseIndex(good, 10); err != nil || len(pages) != 1 {
		t.Fatalf("pages = %+v err=%v", pages, err)
	}
	for _, body := range [][]byte{
		[]byte("001\tpage-001.pgm\n"),
		[]byte("000\tpage-000.pgm\t" + digest + "\n"),
		[]byte("001\tpage-001.pgm\tzzz\n"),
		[]byte("# page\tfile\tdigest\n"),
	} {
		if _, err := ParseIndex(body, 10); err == nil {
			t.Fatalf("ParseIndex(%q) was accepted", body)
		}
	}
}
