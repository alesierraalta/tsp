package quality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGoTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

func deadDeps() Deps { return Deps{ReadFile: os.ReadFile} }

func TestDeadCodeReportsAnUnexportedDeclarationNothingMentions(t *testing.T) {
	root := writeGoTree(t, map[string]string{
		"a.go": "package a\n\nvar _ = used\n\nfunc used() { helper() }\n\nfunc helper() {}\n\nfunc orphan() {}\n",
	})

	findings := deadCodeFindings(root, []string{"a.go"}, deadDeps())
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the orphan", findings)
	}
	if findings[0].Rule != "dead code" || findings[0].Line != 9 || !strings.Contains(findings[0].Message, "orphan is declared here") {
		t.Fatalf("finding = %+v, want a dead-code finding at a.go:7 naming orphan", findings[0])
	}
}

func TestDeadCodeStaysQuietAboutWhatItCannotJudge(t *testing.T) {
	root := writeGoTree(t, map[string]string{
		"a.go": "package a\n\ntype T struct{}\n\nfunc (t T) Method() {}\n\nfunc Exported() {}\n\nfunc main() {}\n\nfunc init() {}\n",
		"b.go": "package b\n\nfunc unusedToo() {}\n\nvar _ = unusedToo\n",
	})

	findings := deadCodeFindings(root, []string{"a.go", "b.go"}, deadDeps())
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none: a method, an exported symbol and the runtime hooks are out of scope", findings)
	}
}

func TestDeadCodeCountsAReferenceFromAnotherPackage(t *testing.T) {
	root := writeGoTree(t, map[string]string{
		"pkg/a.go": "package pkg\n\nfunc shared() {}\n",
		"pkg/b.go": "package pkg\n\nvar _ = shared\n",
	})

	if findings := deadCodeFindings(root, []string{"pkg/a.go"}, deadDeps()); len(findings) != 0 {
		t.Fatalf("findings = %+v: a declaration mentioned from another file is live", findings)
	}
}

func TestDeadCodeReportsOnlyTheChangedFiles(t *testing.T) {
	root := writeGoTree(t, map[string]string{
		"a.go": "package a\n\nfunc orphan() {}\n",
		"b.go": "package a\n\nfunc alsoOrphan() {}\n",
	})

	findings := deadCodeFindings(root, []string{"b.go"}, deadDeps())
	if len(findings) != 1 || findings[0].File != "b.go" {
		t.Fatalf("findings = %+v, want only b.go: the change is what is judged", findings)
	}
}
