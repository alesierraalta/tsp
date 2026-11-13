package archive_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/relay/internal/archive"
)

func TestSafeJoinKeepsNamesInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	path, err := archive.SafeJoin(root, "attachments/logo.png")
	if err != nil {
		t.Fatalf("safe join: %v", err)
	}
	if path != filepath.Join(root, "attachments", "logo.png") {
		t.Fatalf("path = %q", path)
	}
}

func TestSafeJoinRefusesEveryEscape(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"../escape.txt",
		"attachments/../../escape.txt",
		"/etc/passwd",
		"",
		"bad\x00name",
	} {
		if _, err := archive.SafeJoin(root, name); err == nil {
			t.Fatalf("name %q was accepted", name)
		}
	}
}

func TestWriteStoresAnEntryUnderTheRoot(t *testing.T) {
	root := t.TempDir()
	path, err := archive.Write(root, archive.Entry{Name: "attachments/logo.png", Body: []byte("png")})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "png" {
		t.Fatalf("read back: %q, %v", body, err)
	}
	if key, err := archive.Key(root, path); err != nil || key != "attachments/logo.png" {
		t.Fatalf("key = %q, %v", key, err)
	}
}

func tarStream(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatalf("header: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("body: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

func TestReadReturnsTheEntriesOfAStream(t *testing.T) {
	entries, err := archive.Read(bytes.NewReader(tarStream(t, map[string]string{
		"attachments/logo.png": "png",
		"manifest.txt":         "stream=daily\nrecords=1\n",
	})), archive.DefaultLimits())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if kind := archive.Kind("attachments/logo.png"); kind != "attachments" {
		t.Fatalf("kind = %q", kind)
	}
	if kind := archive.Kind("manifest.txt"); kind != "" {
		t.Fatalf("kind of a root entry = %q", kind)
	}
}

func TestReadEnforcesTheEntryLimit(t *testing.T) {
	stream := tarStream(t, map[string]string{
		"records/one.json": `{"id":"1","kind":"usage","account":"acme","amount":10}`,
		"records/two.json": `{"id":"2","kind":"usage","account":"acme","amount":20}`,
	})
	if _, err := archive.Read(bytes.NewReader(stream), archive.Limits{MaxEntries: 1, MaxEntry: 1 << 10, MaxTotal: 1 << 10}); err == nil {
		t.Fatal("a stream above the entry limit was read")
	}
	if _, err := archive.Read(bytes.NewReader(stream), archive.Limits{MaxEntries: 10, MaxEntry: 4, MaxTotal: 1 << 10}); !errors.Is(err, archive.ErrTooLarge) {
		t.Fatalf("entry limit: err = %v", err)
	}
	if _, err := archive.Read(bytes.NewReader(stream), archive.Limits{}); err == nil {
		t.Fatal("a stream with no limits was read")
	}
}

func TestParseManifest(t *testing.T) {
	manifest, err := archive.ParseManifest([]byte("# daily\nstream=daily\nrecords=3\nusage=300\n"))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if manifest.Stream != "daily" {
		t.Fatalf("stream = %q", manifest.Stream)
	}
	if n, ok := manifest.Declared("records"); !ok || n != 3 {
		t.Fatalf("records = %d, %v", n, ok)
	}
	if _, ok := manifest.Declared("nope"); ok {
		t.Fatal("an undeclared key was reported as declared")
	}
	if keys := manifest.Keys(); len(keys) != 2 || keys[0] != "records" {
		t.Fatalf("keys = %v", keys)
	}
	for _, body := range []string{"stream=daily\nbroken line\n", "stream=daily\nrecords=many\n", "records=1\n"} {
		if _, err := archive.ParseManifest([]byte(body)); err == nil {
			t.Fatalf("manifest %q was accepted", body)
		}
	}
}

func TestFindManifest(t *testing.T) {
	entries := []archive.Entry{
		{Name: "attachments/logo.png", Body: []byte("png")},
		{Name: "manifest.txt", Body: []byte("stream=daily\n")},
	}
	manifest, found, err := archive.FindManifest(entries)
	if err != nil || !found || manifest.Stream != "daily" {
		t.Fatalf("find = %+v, %v, %v", manifest, found, err)
	}
}
