package arch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/gitdiff"
)

// repo lays out a module on disk and answers git with a diff the test wrote by hand, so the probe can
// be judged without a repository.
type repo struct {
	root  string
	diff  string
	files map[string]string
}

func newRepo(t *testing.T, files map[string]string) *repo {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &repo{root: root, files: files}
}

func (r *repo) deps() Deps {
	return Deps{
		Git: func(dir string, args ...string) (string, error) {
			switch args[0] {
			case "rev-parse":
				return r.root + "\n", nil
			case "diff":
				return r.diff, nil
			}
			return "", nil
		},
	}
}

func (r *repo) run(t *testing.T) Report {
	t.Helper()
	report, err := Run(context.Background(), Options{Cwd: r.root, Deps: r.deps()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return report
}

// oneModule is a three-layer module: cmd, internal/app and internal/db.
func oneModule(extra map[string]string) map[string]string {
	files := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.24\n",
		".tsp-arch.json": `{"layers":[
			{"name":"cmd","paths":["cmd/"],"imports":["example.com/m/cmd"]},
			{"name":"app","paths":["internal/app/"],"imports":["example.com/m/internal/app"]},
			{"name":"db","paths":["internal/db/"],"imports":["example.com/m/internal/db"]}],
			"forbid":[{"from":"app","to":"cmd","why":"a service must not reach into the command line"}]}`,
		"internal/app/app.go": "package app\n\nimport \"example.com/m/internal/db\"\n\nvar _ = db.Open\n",
		"internal/db/db.go":   "package db\n\n// Open is a handle.\nfunc Open() string { return \"\" }\n",
	}
	for name, body := range extra {
		files[name] = body
	}
	return files
}

// An import on a changed line that a declared rule refuses is a finding, located where the edge is.
func TestRunReportsAForbiddenEdgeOnAChangedLine(t *testing.T) {
	r := newRepo(t, oneModule(map[string]string{
		"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/m/cmd\"\n\t\"example.com/m/internal/db\"\n)\n\nvar _ = db.Open\nvar _ = cmd.Root\n",
		"cmd/root.go":         "package cmd\n\ntype Root struct{}\n",
	}))
	r.diff = "+++ b/internal/app/app.go\n@@ -3,0 +4,1 @@\n+\t\"example.com/m/cmd\"\n"
	report := r.run(t)
	if len(report.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the app -> cmd edge", report.Findings)
	}
	got := report.Findings[0]
	if got.Rule != RuleForbiddenEdge || got.File != "internal/app/app.go" || got.Line != 4 {
		t.Fatalf("finding = %+v, want the forbidden-edge at internal/app/app.go:4", got)
	}
	if !strings.Contains(got.Message, "a service must not reach into the command line") {
		t.Fatalf("message lost the repository's own reason: %q", got.Message)
	}
	if !report.Blocking() {
		t.Fatal("a finding must be blocking")
	}
}

// An edge the change did not touch is pre-existing: blaming it on the change would make every pull
// request fail forever.
func TestRunIgnoresAnEdgeThatWasAlreadyThere(t *testing.T) {
	r := newRepo(t, oneModule(map[string]string{
		"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/m/cmd\"\n\t\"example.com/m/internal/db\"\n)\n\nvar _ = db.Open\nvar _ = cmd.Root\n",
		"cmd/root.go":         "package cmd\n\ntype Root struct{}\n",
	}))
	r.diff = "+++ b/internal/app/app.go\n@@ -6,0 +7,1 @@\n+var _ = cmd.Root\n"
	report := r.run(t)
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %+v, want none: the import line is outside the change", report.Findings)
	}
}

// An edge no rule refuses stays quiet.
func TestRunAllowsAnEdgeNobodyForbade(t *testing.T) {
	r := newRepo(t, oneModule(map[string]string{
		"internal/app/app.go": "package app\n\nimport \"example.com/m/internal/db\"\n\nvar _ = db.Open\n",
	}))
	r.diff = "+++ b/internal/app/app.go\n@@ -3,0 +3,1 @@\n+import \"example.com/m/internal/db\"\n"
	report := r.run(t)
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", report.Findings)
	}
	if len(report.Layers) != 3 {
		t.Fatalf("layers = %v, want the declaration's three", report.Layers)
	}
}

// No declaration means nothing was measured, not that the architecture is clean.
func TestRunRefusesWithoutADeclaration(t *testing.T) {
	files := oneModule(nil)
	delete(files, ".tsp-arch.json")
	r := newRepo(t, files)
	r.diff = "+++ b/internal/app/app.go\n@@ -3,0 +3,1 @@\n+import \"example.com/m/internal/db\"\n"
	_, err := Run(context.Background(), Options{Cwd: r.root, Deps: r.deps()})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "no architecture rules") {
		t.Fatalf("err = %v, want ErrUnusable naming the missing declaration", err)
	}
}

// A rule naming a layer nobody declared is a typo that would disable a real rule, so it is refused.
func TestLoadRefusesARuleAboutAnUndeclaredLayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	body := `{"layers":[{"name":"app","paths":["internal/app/"]}],"forbid":[{"from":"app","to":"domain"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path, nil)
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("err = %v, want ErrUnusable naming the undeclared layer", err)
	}
}

// Importing another module is not a layer edge: the declaration only speaks about this repository.
func TestRunIgnoresImportsFromOtherModules(t *testing.T) {
	r := newRepo(t, oneModule(map[string]string{
		"internal/app/app.go": "package app\n\nimport \"github.com/other/thing\"\n\nvar _ = thing.Value\n",
	}))
	r.diff = "+++ b/internal/app/app.go\n@@ -3,0 +3,1 @@\n+import \"github.com/other/thing\"\n"
	report := r.run(t)
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %+v, want none for a third-party import", report.Findings)
	}
}

// A change with no Go file measured nothing, and saying "clean" would be a lie.
func TestRunRefusesAChangeWithNoGoFile(t *testing.T) {
	r := newRepo(t, oneModule(nil))
	r.diff = "+++ b/README.md\n@@ -1,0 +2,1 @@\n+text\n"
	_, err := Run(context.Background(), Options{Cwd: r.root, Deps: r.deps()})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "no Go file") {
		t.Fatalf("err = %v, want ErrUnusable naming the missing Go file", err)
	}
}

// NotChecked must name what the probe does not measure, including cycles, so a clean report cannot be
// read as a stronger claim.
func TestReportNamesWhatItDoesNotCheck(t *testing.T) {
	r := newRepo(t, oneModule(nil))
	r.diff = "+++ b/internal/app/app.go\n@@ -3,0 +3,1 @@\n+import \"example.com/m/internal/db\"\n"
	report := r.run(t)
	joined := strings.Join(report.NotChecked, " ")
	for _, want := range []string{"cycle", "layer"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("not_checked = %v, want it to mention %q", report.NotChecked, want)
		}
	}
}

var _ = gitdiff.Range{}
