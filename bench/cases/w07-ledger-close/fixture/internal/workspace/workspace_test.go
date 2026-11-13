package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/ledgerclose/internal/calendar"
)

func open(t *testing.T) *Workspace {
	t.Helper()
	workspace, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return workspace
}

func TestOpenRefusesTheFilesystemRoot(t *testing.T) {
	if _, err := Open("/"); !errors.Is(err, ErrBadName) {
		t.Fatalf("the filesystem root was opened: %v", err)
	}
	if _, err := Open(""); !errors.Is(err, ErrBadName) {
		t.Fatalf("an empty root was opened: %v", err)
	}
}

func TestResolveKeepsEveryNameInsideTheRoot(t *testing.T) {
	workspace := open(t)
	inside, err := workspace.Resolve("tenants/acme/statements/2024-05/statement.html")
	if err != nil {
		t.Fatalf("resolve a nested name: %v", err)
	}
	if !strings.HasPrefix(inside, workspace.Root()+string(filepath.Separator)) {
		t.Fatalf("%q is not under %q", inside, workspace.Root())
	}
	for _, name := range []string{"", "../escape.txt", "tenants/../../escape.txt", "/etc/passwd", `\windows\system32`, "c:escape.txt", "bad\x00name", "line\nbreak"} {
		if _, err := workspace.Resolve(name); err == nil {
			t.Fatalf("Resolve(%q) was accepted", name)
		}
	}
}

func TestResolveRefusesASiblingSharingTheRootName(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "close")
	sibling := filepath.Join(parent, "close-copy")
	for _, dir := range []string{root, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	workspace, err := Open(root)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := workspace.Resolve("../close-copy/statement.html"); !errors.Is(err, ErrOutside) {
		t.Fatalf("a sibling that shares the root's name was accepted: %v", err)
	}
}

func TestWriteAndReadAnObject(t *testing.T) {
	workspace := open(t)
	written, err := workspace.Write("tenants/acme/note.txt", []byte("hello"), 0o644)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	body, err := workspace.Read("tenants/acme/note.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("read %q", body)
	}
	entries, err := os.ReadDir(filepath.Dir(written))
	if err != nil {
		t.Fatalf("read the directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".write-") {
			t.Fatalf("the staged file %q was left behind", entry.Name())
		}
	}
}

func TestReadRefusesASymbolicLinkAndAMissingObject(t *testing.T) {
	workspace := open(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(workspace.Root(), "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := workspace.Read("link.txt"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("a symbolic link was read: %v", err)
	}
	if _, err := workspace.Read("missing.txt"); !errors.Is(err, ErrNoObject) {
		t.Fatalf("a missing object was read: %v", err)
	}
}

func TestStatementNameValidatesEveryPart(t *testing.T) {
	period, err := calendar.Parse("2024-05")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	name, err := StatementName("acme", period, "team-api", ".pdf")
	if err != nil {
		t.Fatalf("statement name: %v", err)
	}
	if name != "tenants/acme/statements/2024-05/team-api/statement.pdf" {
		t.Fatalf("statement name = %q", name)
	}
	if _, err := StatementName("../acme", period, "team-api", ".pdf"); err == nil {
		t.Fatal("a tenant that walks upwards was accepted")
	}
	if _, err := StatementName("acme", "2024-13", "team-api", ".pdf"); err == nil {
		t.Fatal("a month outside 1..12 was accepted")
	}
	if _, err := StatementName("acme", period, "Team API", ".pdf"); err == nil {
		t.Fatal("an account id with a space was accepted")
	}
	if _, err := StatementName("acme", period, "team-api", ".exe"); err == nil {
		t.Fatal("an extension the service does not write was accepted")
	}
}
