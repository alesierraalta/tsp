package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"example.com/relay/internal/archive"
	"example.com/relay/internal/importer"
	"example.com/relay/internal/store"
)

func importerAt(t *testing.T) (*importer.Importer, string) {
	t.Helper()
	staging := t.TempDir()
	return importer.New(staging, store.New()), staging
}

func TestAttachmentsLandUnderTheStagingDirectory(t *testing.T) {
	im, staging := importerAt(t)
	report, err := im.Run([]archive.Entry{
		{Name: "attachments/logo.png", Body: []byte("png")},
		{Name: "attachments/terms.pdf", Body: []byte("pdf")},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Written != 2 || report.Entries != 2 {
		t.Fatalf("report = %+v", report)
	}
	for _, path := range report.Paths {
		if rel, err := filepath.Rel(staging, path); err != nil || rel == ".." {
			t.Fatalf("a write landed outside the staging directory: %q", path)
		}
		body, err := os.ReadFile(path)
		if err != nil || len(body) == 0 {
			t.Fatalf("read %q: %v", path, err)
		}
	}
	staged, err := importer.Staged(staging)
	if err != nil || len(staged) != 2 {
		t.Fatalf("staged = %v, %v", staged, err)
	}
}

func TestRecordsAreStoredAndTotaled(t *testing.T) {
	im, _ := importerAt(t)
	report, err := im.Run([]archive.Entry{
		{Name: "records/one.json", Body: []byte(`{"id":"1","kind":"usage","account":"acme","amount":100}`)},
		{Name: "records/two.json", Body: []byte(`{"id":"2","kind":"usage","account":"acme","amount":250}`)},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Records != 2 || len(report.IDs) != 2 {
		t.Fatalf("report = %+v", report)
	}
	if got := im.Store().Total(); got != 350 {
		t.Fatalf("total = %d, want 350 cents", got)
	}
}

func TestUnknownDirectoryStopsTheImport(t *testing.T) {
	im, staging := importerAt(t)
	if _, err := im.Run([]archive.Entry{{Name: "secrets/key.pem", Body: []byte("x")}}); err == nil {
		t.Fatal("an entry under an unknown directory was applied")
	}
	if _, err := os.Stat(filepath.Join(staging, "secrets")); err == nil {
		t.Fatal("the import created a directory for a refused entry")
	}
}

func TestInvalidRecordStopsTheImport(t *testing.T) {
	im, _ := importerAt(t)
	for _, body := range []string{
		`{"id":"","kind":"usage","account":"acme","amount":1}`,
		`{"id":"1","kind":"usage","account":"acme","amount":-1}`,
		`{"id":"1","kind":"usage","account":"acme","amount":1,"extra":true}`,
		`{"id":"1"} {"id":"2"}`,
	} {
		if _, err := im.Run([]archive.Entry{{Name: "records/one.json", Body: []byte(body)}}); err == nil {
			t.Fatalf("record %s was accepted", body)
		}
	}
}

func TestThumbnailStaging(t *testing.T) {
	dir := t.TempDir()
	path, err := importer.StageThumbnail(dir, archive.Entry{Name: "thumbnails/one.png", Body: []byte("png")})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if rel, err := filepath.Rel(dir, path); err != nil || rel == ".." {
		t.Fatalf("thumbnail landed outside the staging directory: %q", path)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "png" {
		t.Fatalf("read back: %q, %v", body, err)
	}
}

func TestSummarizeReportsTheWrittenKeys(t *testing.T) {
	im, staging := importerAt(t)
	entries := []archive.Entry{
		{Name: "attachments/logo.png", Body: []byte("png")},
		{Name: "records/one.json", Body: []byte(`{"id":"1","kind":"usage","account":"acme","amount":100}`)},
	}
	report, err := im.Run(entries)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	summary := importer.Summarize(report, entries)
	if summary.Kinds["attachments"] != 1 || summary.Kinds["records"] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.Entries != 2 || summary.Written != 1 || summary.Records != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.Keys) != 1 || summary.Keys[0] != "attachments/logo.png" {
		t.Fatalf("keys = %v", summary.Keys)
	}
	_ = staging
	if text := summary.Text(); text == "" {
		t.Fatal("the summary renders nothing")
	}
}
