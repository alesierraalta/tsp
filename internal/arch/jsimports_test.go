package arch

import (
	"strings"
	"testing"
)

// A TypeScript change that reaches from a declared layer into a refused one is a finding, on the line
// where the specifier is written.
func TestRunReportsAForbiddenEdgeInTypeScript(t *testing.T) {
	r := newRepo(t, map[string]string{
		".tsp-arch.json": `{"layers":[
			{"name":"web","paths":["web/"]},
			{"name":"api","paths":["api/"]}],
			"forbid":[{"from":"web","to":"api","why":"the page must not call the store directly"}]}`,
		"web/page.ts":  "import { load } from '../api/store'\n\nexport const page = load\n",
		"api/store.ts": "export const load = () => 'x'\n",
	})
	r.diff = "+++ b/web/page.ts\n@@ -0,0 +1,1 @@\n+import { load } from '../api/store'\n"
	report := r.run(t)
	if len(report.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the web -> api edge", report.Findings)
	}
	got := report.Findings[0]
	if got.File != "web/page.ts" || got.Line != 1 {
		t.Fatalf("finding = %+v, want web/page.ts:1", got)
	}
	if !strings.Contains(got.Message, "the page must not call the store directly") {
		t.Fatalf("message lost the repository's own reason: %q", got.Message)
	}
}

// A package from node_modules leaves the repository: it is not an edge between declared layers.
func TestTypeScriptPackageImportsAreNotEdges(t *testing.T) {
	r := newRepo(t, map[string]string{
		".tsp-arch.json": `{"layers":[{"name":"web","paths":["web/"]},{"name":"api","paths":["api/"]}],
			"forbid":[{"from":"web","to":"api","why":"reach around"}]}`,
		"web/page.ts":  "import { z } from 'zod'\n",
		"api/store.ts": "export const load = () => 'x'\n",
	})
	r.diff = "+++ b/web/page.ts\n@@ -0,0 +1,1 @@\n+import { z } from 'zod'\n"
	if report := r.run(t); len(report.Findings) != 0 {
		t.Fatalf("findings = %+v, want none for a package import", report.Findings)
	}
}
