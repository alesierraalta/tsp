package quality

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo writes a small tree and answers git with the hunks the test declares, so the probe's decisions
// are tested without a repository: what counts as the change is the input, not the fixture.
type repo struct {
	dir    string
	files  map[string]string
	hunks  map[string]string
	status string
	base   string
}

func (r *repo) deps() Deps {
	return Deps{
		Git: func(dir string, args ...string) (string, error) {
			switch args[0] {
			case "rev-parse":
				return r.dir + "\n", nil
			case "status":
				return r.status, nil
			case "diff":
				if strings.Contains(strings.Join(args, " "), "--name-only") {
					names := make([]string, 0, len(r.files))
					for name := range r.files {
						names = append(names, name)
					}
					return strings.Join(names, "\n"), nil
				}
				if len(r.hunks) == 1 {
					for _, hunk := range r.hunks {
						return hunk, nil
					}
				}
				return "", nil
			}
			return "", nil
		},
		ReadFile: func(path string) ([]byte, error) {
			rel, _ := filepath.Rel(r.dir, path)
			if body, ok := r.files[filepath.ToSlash(rel)]; ok {
				return []byte(body), nil
			}
			return os.ReadFile(path)
		},
	}
}

func newRepo(t *testing.T, files map[string]string) *repo {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &repo{dir: dir, files: files, hunks: map[string]string{}}
}

// The complexity rule follows the change: a sprawling function nobody touched is not this change's
// problem, and reporting it is how a tool becomes noise nobody runs twice.
func TestComplexityCountsOnlyWhatTheChangeTouches(t *testing.T) {
	body := `package p

func simple() int {
	return 1
}

func sprawling(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			if i%3 == 0 {
				total++
			}
			total++
		}
		switch n {
		case 1:
			total++
		case 2:
			total++
		}
	}
	if total > 3 && n > 2 || total < 0 {
		total = 0
	}
	return total
}
`
	body += "\nvar _ = []any{simple, sprawling}\n"
	r := newRepo(t, map[string]string{"p/p.go": body})
	r.base = "main"
	// The change touches the simple function only (lines 3-5).
	r.hunks["p/p.go"] = "diff --git a/p/p.go b/p/p.go\n--- a/p/p.go\n+++ b/p/p.go\n@@ -3,0 +3,1 @@\n+// touched\n"
	rep, err := Run(context.Background(), Options{Cwd: r.dir, Base: "main", MaxComplexity: 5, Deps: r.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v, want none: the complex function was not touched", rep.Findings)
	}

	// Now the change touches the sprawling body (lines 8-27).
	r.hunks["p/p.go"] = "diff --git a/p/p.go b/p/p.go\n--- a/p/p.go\n+++ b/p/p.go\n@@ -10,0 +10,1 @@\n+// touched\n"
	rep, err = Run(context.Background(), Options{Cwd: r.dir, Base: "main", MaxComplexity: 5, Deps: r.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Rule != RuleComplexity {
		t.Fatalf("findings = %+v, want one complexity finding for the touched function", rep.Findings)
	}
	if got := rep.Findings[0]; got.File != "p/p.go" || got.Line == 0 {
		t.Fatalf("finding = %+v, want the file and the function's line", got)
	}
}

// A block copied inside the change is duplication the author can still fix; the same block elsewhere in
// the repository is history, not a review comment.
func TestDuplicationCountsOnlyInsideTheChange(t *testing.T) {
	block := strings.Repeat("value := compute(input)\nstore(value)\n", 4)
	r := newRepo(t, map[string]string{"p/new.go": "package p\n\nfunc fresh() {\n" + block + block + "}\n\nvar _ = fresh\n"})
	r.status = "?? p/new.go\x00"
	rep, err := Run(context.Background(), Options{Cwd: r.dir, MaxComplexity: 99, DupWindow: 6, Deps: r.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Rule != RuleDuplication {
		t.Fatalf("findings = %+v, want one duplication finding in the new file", rep.Findings)
	}

	// The same duplicated block in a file the change did not touch is not reported.
	r2 := newRepo(t, map[string]string{"p/old.go": "package p\n\nfunc legacy() {\n" + block + block + "}\n\nvar _ = legacy\n"})
	r2.status = ""
	rep, err = Run(context.Background(), Options{Cwd: r2.dir, MaxComplexity: 99, DupWindow: 6, Deps: r2.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v, want none for a file outside the change", rep.Findings)
	}
}

// A language with no adapter is named instead of being passed over: silence there would read as clean.
func TestLanguageWithoutAnAdapterIsUnusable(t *testing.T) {
	r := newRepo(t, map[string]string{"app/main.py": "def main():\n    pass\n"})
	r.status = "?? app/main.py\x00"
	_, err := Run(context.Background(), Options{Cwd: r.dir, Deps: r.deps()})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "python") {
		t.Fatalf("err = %v, want an unusable probe naming the language without an adapter", err)
	}
}

// Nothing changed means nothing to probe, which is quiet rather than unusable.
func TestNoChangedFilesIsQuiet(t *testing.T) {
	r := newRepo(t, map[string]string{"p/p.go": "package p\n"})
	r.status = ""
	rep, err := Run(context.Background(), Options{Cwd: r.dir, Deps: r.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Measured) != 0 || len(rep.Findings) != 0 {
		t.Fatalf("report = %+v, want an empty probe", rep)
	}
}
