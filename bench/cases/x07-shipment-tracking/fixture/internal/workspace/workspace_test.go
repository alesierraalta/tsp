package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/workspace"
)

func open(t *testing.T) *workspace.Workspace {
	t.Helper()
	root := t.TempDir()
	tree, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return tree
}

func TestResolveKeepsNamesInsideTheTree(t *testing.T) {
	tree := open(t)
	name, err := workspace.StatementName("shop", clock.Period{Year: 2024, Month: 5}, "html")
	if err != nil {
		t.Fatalf("name: %v", err)
	}
	if name != "partners/shop/statements/2024-05/statement.html" {
		t.Fatalf("name = %q", name)
	}
	path, err := tree.Resolve(name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.HasPrefix(path, tree.Root()+string(filepath.Separator)) {
		t.Fatalf("%q is not under %q", path, tree.Root())
	}
	// The containment test includes the separator, so a sibling directory whose name starts with the
	// root's name is outside the tree.
	sibling := tree.Root() + "-copy"
	if strings.HasPrefix(sibling, tree.Root()+string(filepath.Separator)) {
		t.Fatalf("%q must not count as inside %q", sibling, tree.Root())
	}
}

func TestResolveRefusesEscapes(t *testing.T) {
	tree := open(t)
	for _, name := range []string{"", "/etc/passwd", "../outside", "partners/../../etc/passwd", "c:/windows", "partners/shop\x00/x"} {
		if _, err := tree.Resolve(name); err == nil {
			t.Fatalf("Resolve(%q) accepted a name it must refuse", name)
		}
	}
}

func TestWriteAndReadAnObject(t *testing.T) {
	tree := open(t)
	payload := []byte("<html>statement</html>")
	path, err := tree.Write("partners/shop/statements/2024-05/statement.html", payload, 0o644)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := tree.Read("partners/shop/statements/2024-05/statement.html", 1024)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("read %q", got)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("stat: %v", err)
	}
	// A read above the bound refuses instead of truncating.
	if _, err := tree.Read("partners/shop/statements/2024-05/statement.html", 4); err == nil {
		t.Fatal("a read above the bound was accepted")
	}
	names, err := tree.List("partners/shop/statements/2024-05")
	if err != nil || len(names) != 1 || names[0] != "statement.html" {
		t.Fatalf("list = %v, %v", names, err)
	}
}

func TestReadRefusesASymbolicLink(t *testing.T) {
	tree := open(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("not for the service"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	target, err := tree.Resolve("partners/shop/link.txt")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("the platform does not create symbolic links: %v", err)
	}
	if _, err := tree.Read("partners/shop/link.txt", 1024); err == nil {
		t.Fatal("a symbolic link was read as an object")
	}
}

func TestNamesAreBuiltFromValidatedParts(t *testing.T) {
	for _, tc := range []struct{ partner, shipment, ext string }{
		{"../shop", "shp_00000000000000000000000000000000", "pdf"},
		{"shop", "../../etc", "pdf"},
		{"shop", "shp_00000000000000000000000000000000", "p/df"},
	} {
		if _, err := workspace.LabelName(tc.partner, tc.shipment, tc.ext); err == nil {
			t.Fatalf("LabelName(%q, %q, %q) was accepted", tc.partner, tc.shipment, tc.ext)
		}
	}
	name, err := workspace.LabelName("shop", "shp_00000000000000000000000000000000", "pdf")
	if err != nil {
		t.Fatalf("label name: %v", err)
	}
	if name != "partners/shop/labels/shp_00000000000000000000000000000000/label.pdf" {
		t.Fatalf("label name = %q", name)
	}
}
