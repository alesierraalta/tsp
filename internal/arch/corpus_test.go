package arch

import (
	"strings"
	"testing"
)

// The corpus is the answer to "does this rule catch a real violation, and only that?". Each entry names the
// class it pins, the fixture, the diff a change would carry, and either the finding the probe must report
// or the silence it must keep. A rule that cannot show both halves is a rule nobody can trust.
type corpusCase struct {
	class   string
	files   map[string]string
	diff    string
	want    []wantFinding // empty means: no finding at all
	reason  string
	control bool
}

type wantFinding struct {
	rule string
	file string
	line int
	says string
}

func layersRules() string {
	return `{"layers":[
		{"name":"cmd","paths":["cmd/"],"imports":["example.com/m/cmd"]},
		{"name":"app","paths":["internal/app/"],"imports":["example.com/m/internal/app"]},
		{"name":"db","paths":["internal/db/"],"imports":["example.com/m/internal/db"]}],
		"forbid":[{"from":"app","to":"cmd","why":"a service must not reach into the command line"}]}`
}

func corpusBase(extra map[string]string) map[string]string {
	files := map[string]string{
		"go.mod":              "module example.com/m\n\ngo 1.24\n",
		".tsp-arch.json":      layersRules(),
		"cmd/root.go":         "package cmd\n\ntype Root struct{}\n",
		"internal/app/app.go": "package app\n\nimport \"example.com/m/internal/db\"\n\nvar _ = db.Open\n",
		"internal/db/db.go":   "package db\n\n// Open is a handle.\nfunc Open() string { return \"\" }\n",
	}
	for name, body := range extra {
		files[name] = body
	}
	return files
}

func corpus(t *testing.T) {
	t.Helper()
	cases := []corpusCase{
		{
			class: "forbidden-edge",
			files: corpusBase(map[string]string{
				"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/m/cmd\"\n\t\"example.com/m/internal/db\"\n)\n\nvar _ = db.Open\nvar _ = cmd.Root\n",
			}),
			diff:   "+++ b/internal/app/app.go\n@@ -3,0 +4,1 @@\n+\t\"example.com/m/cmd\"\n",
			want:   []wantFinding{{rule: RuleForbiddenEdge, file: "internal/app/app.go", line: 4, says: "a service must not reach into the command line"}},
			reason: "an edge the repository forbids by name, on the line the change adds",
		},
		{
			class: "two-forbidden-edges",
			files: corpusBase(map[string]string{
				"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/m/cmd\"\n\t\"example.com/m/cmd/sub\"\n\t\"example.com/m/internal/db\"\n)\n\nvar _ = db.Open\nvar _ = cmd.Root\n",
				"cmd/sub/sub.go":      "package sub\n\n// Root is a symbol.\nvar Root = 1\n",
			}),
			diff: "+++ b/internal/app/app.go\n@@ -3,0 +4,2 @@\n+\t\"example.com/m/cmd\"\n+\t\"example.com/m/cmd/sub\"\n",
			want: []wantFinding{
				{rule: RuleForbiddenEdge, file: "internal/app/app.go", line: 4, says: "a service must not reach into the command line"},
				{rule: RuleForbiddenEdge, file: "internal/app/app.go", line: 5, says: "cmd/sub"},
			},
			reason: "every refused edge is reported, one finding per line, in the order they appear",
		},
		{
			class: "control-untouched-line",
			files: corpusBase(map[string]string{
				"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/m/cmd\"\n\t\"example.com/m/internal/db\"\n)\n\nvar _ = db.Open\nvar _ = cmd.Root\n",
			}),
			diff:    "+++ b/internal/app/app.go\n@@ -8,0 +8,1 @@\n+var _ = cmd.Root\n",
			reason:  "a refused edge that already existed is history, not a comment on this change",
			control: true,
		},
		{
			class:   "control-declared-edge",
			files:   corpusBase(nil),
			diff:    "+++ b/internal/app/app.go\n@@ -3,0 +3,1 @@\n+\t\"example.com/m/internal/db\"\n",
			reason:  "the edge the layer already declares is the one everybody needs",
			control: true,
		},
		{
			class: "control-standard-library",
			files: corpusBase(map[string]string{
				"internal/app/app.go": "package app\n\nimport (\n\t\"sort\"\n\t\"example.com/m/internal/db\"\n)\n\nvar _ = db.Open\nvar _ = sort.Strings\n",
			}),
			diff:    "+++ b/internal/app/app.go\n@@ -3,0 +4,1 @@\n+\t\"sort\"\n",
			reason:  "the standard library is not a layer: a rule about this module must not judge it",
			control: true,
		},
		{
			class: "control-undeclared-package",
			files: corpusBase(map[string]string{
				"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/m/internal/db\"\n\t\"example.com/m/tools/x\"\n)\n\nvar _ = db.Open\nvar _ = x.Y\n",
				"tools/x/x.go":        "package x\n\n// Y is a symbol.\nvar Y = 1\n",
			}),
			diff:    "+++ b/internal/app/app.go\n@@ -3,0 +4,1 @@\n+\t\"example.com/m/tools/x\"\n",
			reason:  "a package that matches no declared layer is not judged, and the report says so",
			control: true,
		},
		{
			class: "javascript-relative-edge",
			files: map[string]string{
				".tsp-arch.json": `{"layers":[
					{"name":"web","paths":["web/"],"imports":["web/"]},
					{"name":"core","paths":["core/"],"imports":["core/"]}],
					"forbid":[{"from":"web","to":"core","why":"the browser bundle must not reach the server core"}]}`,
				"web/app.ts": "import { open } from \"../core/db\";\n\nexport const x = open;\n",
				"core/db.ts": "export function open() { return 1; }\n",
			},
			diff:   "+++ b/web/app.ts\n@@ -1,0 +1,1 @@\n+import { open } from \"../core/db\";\n",
			want:   []wantFinding{{rule: RuleForbiddenEdge, file: "web/app.ts", line: 1, says: "the browser bundle must not reach the server core"}},
			reason: "a relative import crossing a refused layer boundary is the same violation in TypeScript",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.class, func(t *testing.T) {
			r := newRepo(t, c.files)
			r.diff = c.diff
			report := r.run(t)
			if c.control {
				if len(report.Findings) != 0 {
					t.Fatalf("%s: findings = %+v, want none (%s)", c.class, report.Findings, c.reason)
				}
				return
			}
			if len(report.Findings) != len(c.want) {
				t.Fatalf("%s: findings = %+v, want %d (%s)", c.class, report.Findings, len(c.want), c.reason)
			}
			for i, want := range c.want {
				got := report.Findings[i]
				if got.Rule != want.rule || got.File != want.file || got.Line != want.line {
					t.Fatalf("%s: finding = %+v, want %s at %s:%d", c.class, got, want.rule, want.file, want.line)
				}
				if want.says != "" && !strings.Contains(got.Message, want.says) {
					t.Fatalf("%s: message %q does not say %q", c.class, got.Message, want.says)
				}
			}
		})
	}
}

func TestCorpus(t *testing.T) { corpus(t) }
