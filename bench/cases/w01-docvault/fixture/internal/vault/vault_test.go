package vault_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/docvault/internal/vault"
)

func open(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return v, root
}

func TestRootMustBeADirectory(t *testing.T) {
	if _, err := vault.Open("/"); !errors.Is(err, vault.ErrInvalid) {
		t.Fatalf("opening the filesystem root: err = %v, want ErrInvalid", err)
	}
}

func TestResolveReturnsAPathInsideTheRoot(t *testing.T) {
	v, root := open(t)
	got, err := v.Resolve("documents/one.txt")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != filepath.Join(root, "documents", "one.txt") {
		t.Fatalf("resolve = %q, want it under %q", got, root)
	}
	if key, err := v.Key("documents/one.txt"); err != nil || key != "documents/one.txt" {
		t.Fatalf("key = %q, %v", key, err)
	}
}

func TestResolveRefusesNamesThatLeaveTheRoot(t *testing.T) {
	v, _ := open(t)
	for _, name := range []string{"../outside.txt", "documents/../../outside.txt", "/etc/passwd", "", "bad\x00name"} {
		if _, err := v.Resolve(name); err == nil {
			t.Fatalf("resolve(%q) was accepted", name)
		}
	}
	sibling := filepath.Base(v.Root()) + "-copy"
	if _, err := v.Resolve("../" + sibling + "/key.pem"); err == nil {
		t.Fatalf("a sibling directory sharing the root's name was accepted")
	}
}

func TestReadAndWriteStayInsideTheRoot(t *testing.T) {
	v, root := open(t)
	if err := v.Write("documents/one.txt", []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	body, err := v.Read("documents/one.txt")
	if err != nil || string(body) != "hello" {
		t.Fatalf("read = %q, %v", body, err)
	}
	if _, err := v.Read("documents/missing.txt"); !errors.Is(err, vault.ErrNoObject) {
		t.Fatalf("missing object: err = %v, want ErrNoObject", err)
	}
	if err := v.Write("../escape.txt", []byte("x")); err == nil {
		t.Fatal("write outside the root was accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); err == nil {
		t.Fatal("a write landed outside the root")
	}
}

func TestListRefusesAPrefixOutsideTheRoot(t *testing.T) {
	v, _ := open(t)
	if _, err := v.List("../secrets"); err == nil {
		t.Fatal("listing outside the root was accepted")
	}
}

func TestAttachmentsAreFlat(t *testing.T) {
	v, _ := open(t)
	if _, err := vault.AttachmentPath("logo.png"); err != nil {
		t.Fatalf("flat attachment: %v", err)
	}
	for _, name := range []string{"../logo.png", "sub/logo.png", "logo.exe", ""} {
		if _, err := vault.AttachmentPath(name); err == nil {
			t.Fatalf("attachment name %q was accepted", name)
		}
	}
	if _, err := v.WriteAttachment("logo.png", []byte("png")); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	items, err := v.Attachments()
	if err != nil || len(items) != 1 || items[0].Name != "logo.png" {
		t.Fatalf("attachments = %+v, %v", items, err)
	}
}

func TestParseRange(t *testing.T) {
	start, end, err := vault.ParseRange("", 100)
	if err != nil || start != 0 || end != 100 {
		t.Fatalf("absent range = %d,%d,%v", start, end, err)
	}
	start, end, err = vault.ParseRange("10-19", 100)
	if err != nil || start != 10 || end != 19 {
		t.Fatalf("explicit range = %d,%d,%v", start, end, err)
	}
	for _, header := range []string{"-5", "90-10", "abc-", "0-1000"} {
		if _, _, err := vault.ParseRange(header, 100); err == nil {
			t.Fatalf("range %q was accepted", header)
		}
	}
}

func TestNormalizeStripsTheFormsAClientMaySend(t *testing.T) {
	v, _ := open(t)
	for _, name := range []string{"./documents/a.txt", "documents/a.txt/", " documents/a.txt "} {
		if got := v.Normalize(name); got != "documents/a.txt" {
			t.Fatalf("normalize(%q) = %q", name, got)
		}
	}
}

func TestKeysCarryForwardSlashes(t *testing.T) {
	v, _ := open(t)
	if err := v.Write("documents/nested/one.txt", []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	keys, err := v.List("documents")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, k := range keys {
		if strings.Contains(k, "\\") {
			t.Fatalf("key %q carries a platform separator", k)
		}
	}
	if len(keys) != 1 || keys[0] != "documents/nested/one.txt" {
		t.Fatalf("list = %v", keys)
	}
}
