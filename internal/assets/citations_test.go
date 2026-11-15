package assets

import (
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// citedEmbeddedFile is a path into an embedded skill tree: a reference file (references/…), a shipped
// script or template (assets/…), with or without a leading ./, ~/.claude/skills/, or the name of the skill
// that owns it.
var citedEmbeddedFile = regexp.MustCompile(`(?:~?[\w.@-]*/)*(?:references|assets)/[\w.@-]+(?:\.[A-Za-z0-9_-]+)+`)

// TestEveryCitedSkillFileIsEmbedded pins that a pointer in an embedded document lands on a file the
// install carries. The skill tree no longer has a references/ directory, so a citation left behind sends
// the model that follows it to a file no host installs, and the promise it stood for is lost silently.
func TestEveryCitedSkillFileIsEmbedded(t *testing.T) {
	skills := Skills()
	embedded := map[string]bool{}
	for _, name := range SkillNames() {
		embedded[name] = true
	}
	var dangling []string
	err := fs.WalkDir(skills, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		data, err := fs.ReadFile(skills, p)
		if err != nil {
			return err
		}
		text := string(data)
		owner, _, _ := strings.Cut(p, "/")
		for _, m := range citedEmbeddedFile.FindAllStringIndex(text, -1) {
			// A citation preceded by / or : is a URL or an absolute host path, not a path into this tree.
			if start := m[0]; start > 0 && (text[start-1] == '/' || text[start-1] == ':') {
				continue
			}
			cite := text[m[0]:m[1]]
			resolved := resolveCitation(owner, cite, embedded)
			if _, err := fs.Stat(skills, resolved); err != nil {
				dangling = append(dangling, p+" cites `"+cite+"` but "+resolved+" is not embedded")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(dangling)
	for _, d := range dangling {
		t.Error(d)
	}
}

// resolveCitation maps a cited path onto the embedded skills tree: after skills/ when the citation names
// that root, from the tree root when it names an embedded skill, and relative to the citing skill
// otherwise.
func resolveCitation(owner, cite string, embedded map[string]bool) string {
	cite = strings.TrimPrefix(cite, "./")
	if i := strings.Index(cite, "skills/"); i >= 0 {
		return cite[i+len("skills/"):]
	}
	if dir, rest, ok := strings.Cut(cite, "/"); ok && embedded[dir] {
		return path.Join(dir, rest)
	}
	return path.Join(owner, cite)
}

// The walk above is only as good as this mapping: a resolver that sent every citation to an existing
// path, or to the wrong skill, would report a dead pointer as alive.
func TestCitedPathResolutionFollowsTheEmbeddedTree(t *testing.T) {
	embedded := map[string]bool{"test-strategy": true, "go-testing": true}
	for _, tc := range []struct{ owner, cite, want string }{
		{"go-testing", "references/examples.md", "go-testing/references/examples.md"},
		{"database-persistence-testing", "./assets/migration-idempotency-check.sh", "database-persistence-testing/assets/migration-idempotency-check.sh"},
		{"exploit-testing", "test-strategy/references/evidence.md", "test-strategy/references/evidence.md"},
		{"real-run-validation", "~/.claude/skills/test-strategy/SKILL.md", "test-strategy/SKILL.md"},
		{"appsec-adversarial-auditor", "~/.claude/skills/exploit-testing/references/safety-gate.md", "exploit-testing/references/safety-gate.md"},
	} {
		if got := resolveCitation(tc.owner, tc.cite, embedded); got != tc.want {
			t.Errorf("resolveCitation(%q, %q) = %q, want %q", tc.owner, tc.cite, got, tc.want)
		}
	}
}
