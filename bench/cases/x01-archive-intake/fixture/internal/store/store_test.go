package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/storepath"
)

// open returns a store over a fresh directory with a pinned clock.
func open(t *testing.T) *Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), "archive")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lim := config.Default().Limits()
	lim.MaxUpload = 1 << 20
	s, err := Open(root, lim, clock.Fixed(time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func record(name, kind string) Record {
	return Record{Tenant: "acme", Name: name, Kind: kind, Batch: "bat-00112233445566778899aabb", Entry: name}
}

func TestPutAndRead(t *testing.T) {
	s := open(t)
	stored, err := s.Put(record("scan-01", "scan"), []byte("container bytes"), []byte("page text"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if stored.ID == "" || stored.Digest == "" || stored.Bytes != 15 {
		t.Fatalf("stored = %+v", stored)
	}
	body, err := s.Read(stored.ID)
	if err != nil || string(body) != "container bytes" {
		t.Fatalf("read = %q err=%v", body, err)
	}
	text, err := s.ReadText(stored.ID)
	if err != nil || text != "page text" {
		t.Fatalf("text = %q err=%v", text, err)
	}
	if _, err := s.Put(stored, []byte("again"), nil); err == nil {
		t.Fatal("a second commit of the same id was accepted")
	}
}

func TestPutRefusesADigestItDoesNotCarry(t *testing.T) {
	s := open(t)
	rec := record("scan-02", "scan")
	rec.Digest = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := s.Put(rec, []byte("other bytes"), nil); err == nil {
		t.Fatal("a wrong declared digest was accepted")
	}
}

func TestListFiltersAndSorts(t *testing.T) {
	s := open(t)
	for i, name := range []string{"scan-01", "scan-02", "listing.csv"} {
		kind := "scan"
		if name == "listing.csv" {
			kind = "index"
		}
		rec := record(name, kind)
		rec.Title = name + " title"
		rec.Tags = []string{"intake"}
		rec.Pages = i
		if _, err := s.Put(rec, []byte("bytes of "+name), []byte("text of "+name)); err != nil {
			t.Fatalf("put %s: %v", name, err)
		}
	}
	all, err := s.List(Filter{})
	if err != nil || len(all) != 3 {
		t.Fatalf("list = %d err=%v", len(all), err)
	}
	scans, _ := s.List(Filter{Kind: "scan"})
	if len(scans) != 2 {
		t.Fatalf("scans = %d", len(scans))
	}
	index, _ := s.List(Filter{Query: "listing"})
	if len(index) != 1 || index[0].Name != "listing.csv" {
		t.Fatalf("query = %+v", index)
	}
	other, _ := s.List(Filter{Tenant: "globex"})
	if len(other) != 0 {
		t.Fatalf("another tenant sees %d objects", len(other))
	}
	if _, err := s.List(Filter{Query: "text of scan"}); err != nil {
		t.Fatalf("query over text: %v", err)
	}
}

func TestDigestIsSharedAndDeleteKeepsIt(t *testing.T) {
	s := open(t)
	first, err := s.Put(record("scan-01", "scan"), []byte("same bytes"), nil)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	second := record("scan-02", "scan")
	second.Entry = "scan-02"
	if _, err := s.Put(second, []byte("same bytes"), nil); err != nil {
		t.Fatalf("put second: %v", err)
	}
	if err := s.Delete(first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	kept, err := s.List(Filter{})
	if err != nil || len(kept) != 1 {
		t.Fatalf("kept = %d err=%v", len(kept), err)
	}
	if body, err := s.Read(kept[0].ID); err != nil || string(body) != "same bytes" {
		t.Fatalf("shared blob was removed: %q err=%v", body, err)
	}
}

func TestSearchRanksNameAboveNote(t *testing.T) {
	s := open(t)
	named := record("assessor-letter", "scan")
	note := record("scan-09", "scan")
	note.Note = "assessor"
	if _, err := s.Put(named, []byte("one"), nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := s.Put(note, []byte("two"), nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	hits := s.Search("assessor", 0)
	if len(hits) != 2 || hits[0].Record.Name != "assessor-letter" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestStatsAndTenants(t *testing.T) {
	s := open(t)
	if _, err := s.Put(record("scan-01", "scan"), []byte("bytes"), nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	stats := s.Stats()
	if stats.Objects != 1 || stats.Bytes != 5 || stats.Kinds["scan"] != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	if got := s.Tenants(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("tenants = %v", got)
	}
}

func TestVerifyReportsACorruptBlob(t *testing.T) {
	s := open(t)
	stored, err := s.Put(record("scan-01", "scan"), []byte("bytes"), nil)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	blob, err := s.BlobPath(stored.Digest)
	if err != nil {
		t.Fatalf("blob path: %v", err)
	}
	if err := os.WriteFile(blob, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	report, err := s.Verify()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.OK() || len(report.Corrupt) != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestStoreRefusesANameThatLeavesTheRoot(t *testing.T) {
	s := open(t)
	if _, err := s.metaPath("../../outside"); err == nil {
		t.Fatal("a traversing id was accepted")
	}
	if _, err := storepath.Resolve(s.Root(), "../../outside"); err == nil {
		t.Fatal("storepath accepted a traversing name")
	}
}
