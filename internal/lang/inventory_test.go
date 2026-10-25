package lang

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The inventory names the stacks the repository is made of, in a stable order, and only those.
func TestDetectNamesTheStacksTheRepositoryIsMadeOf(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go")
	write(t, root, "app/page.tsx")
	write(t, root, "scripts/tool.py")
	write(t, root, "README.md")

	got := Detect(root, Deps{LookPath: func(string) (string, error) { return "/bin/x", nil }}, nil)
	var names []string
	for _, language := range got.Languages {
		names = append(names, language.Name)
	}
	want := []string{"go", "javascript", "python"}
	if len(names) != len(want) {
		t.Fatalf("languages = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("languages = %v, want %v", names, want)
		}
	}
	if got.Languages[1].Files != 1 {
		t.Fatalf("javascript files = %d, want 1 (.tsx counted once)", got.Languages[1].Files)
	}
}

// A dependency tree is another project's code: counting it claims tools for files this repository does
// not own.
func TestDetectSkipsVendoredTrees(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go")
	write(t, root, "node_modules/left-pad/index.js")
	write(t, root, "backend/.venv/lib/site-packages/requests.py")

	got := Detect(root, Deps{LookPath: func(string) (string, error) { return "/bin/x", nil }}, nil)
	if len(got.Languages) != 1 || got.Languages[0].Name != "go" {
		t.Fatalf("languages = %+v, want only go", got.Languages)
	}
}

// A tool the machine cannot run is not a capability: the inventory says so and names the install line.
func TestDetectReportsTheToolsThatAreMissing(t *testing.T) {
	root := t.TempDir()
	write(t, root, "cmd/main.go")

	got := Detect(root, Deps{LookPath: func(name string) (string, error) {
		if name == "gremlins" {
			return "", errors.New("missing")
		}
		return "/bin/" + name, nil
	}}, nil)
	tools := got.Languages[0].Tools
	if len(tools) != 2 {
		t.Fatalf("tools = %+v, want tests and mutation", tools)
	}
	if !tools[0].Available || tools[1].Available {
		t.Fatalf("tools = %+v, want tests present and mutation missing", tools)
	}
	if tools[1].Install == "" {
		t.Fatal("a missing tool without an install line leaves the reader with nothing to do")
	}
}
