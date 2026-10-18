package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// corpusDir is the shipped benchmark; tests here check the corpus itself, not the runner.
func corpusDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "bench", "cases")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no shipped corpus beside the module")
	}
	return dir
}

// vendoredCases are the shipped cases that carry their own fixture/ and fix/. A case shipped by reference
// (SOURCE.json) is built from its upstream on demand, so the offline checks below cannot read it; its own
// completeness is checked by TestCorpusSourceCasesAreComplete.
func vendoredCases(t *testing.T) []string {
	t.Helper()
	all, _ := filepath.Glob(filepath.Join(corpusDir(t), "*"))
	var cases []string
	for _, c := range all {
		if _, err := os.Stat(filepath.Join(c, SourceFile)); err != nil {
			cases = append(cases, c)
		}
	}
	return cases
}

// A case shipped by reference names a fetchable repository, a full commit and a patch that exists, and its
// key defects sit in files that patch changes: without that, no build could put the fix where the key says.
func TestCorpusSourceCasesAreComplete(t *testing.T) {
	all, _ := filepath.Glob(filepath.Join(corpusDir(t), "*", SourceFile))
	for _, f := range all {
		c := filepath.Dir(f)
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var src Source
		if err := json.Unmarshal(raw, &src); err != nil || src.Repo == "" || !fullCommit.MatchString(src.Commit) || src.Patch == "" {
			t.Errorf("%s: incomplete %s: %s", c, SourceFile, raw)
			continue
		}
		patch, err := os.ReadFile(filepath.Join(c, src.Patch))
		if err != nil {
			t.Errorf("%s: %v", c, err)
			continue
		}
		key, err := LoadKey(c)
		if err != nil {
			t.Errorf("%s: %v", c, err)
			continue
		}
		for _, d := range key.Defects {
			if !strings.Contains(string(patch), "+++ b/"+d.File+"\n") {
				t.Errorf("%s: defect %s sits in %s, which %s does not change", c, d.ID, d.File, src.Patch)
			}
		}
	}
}

// hiddenTriggerRe is a bench-internal hook a fixture must not carry: a TPP_ variable that switches a test on,
// or a test named after the trigger that proves a planted defect.
var hiddenTriggerRe = regexp.MustCompile(`TPP_[A-Z_]+|func Test\w*Trigger\b|test\(["'][^"']*trigger`)

// The fixture is what the agent sees. A trigger test shipped in it, even one skipped by default, names the
// defect and hands over its proof: the agent can run it instead of finding the defect, and the reading
// stops measuring discovery.
func TestCorpusFixturesCarryNoHiddenTriggers(t *testing.T) {
	cases := vendoredCases(t)
	for _, c := range cases {
		_ = filepath.Walk(filepath.Join(c, FixtureDir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if m := hiddenTriggerRe.Find(data); m != nil {
				t.Errorf("%s: the fixture carries a hidden trigger %q", path, m)
			}
			return nil
		})
	}
}

// Every case ships the fixed versions the discriminating check needs.
func TestCorpusShipsFixedVersions(t *testing.T) {
	cases := vendoredCases(t)
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil {
			t.Errorf("%s: %v", c, err)
			continue
		}
		if key.IsCleanControl() {
			if len(key.Defects) != 0 {
				t.Errorf("%s: clean control declares %d defects", filepath.Base(c), len(key.Defects))
			}
			continue
		}
		if st, err := os.Stat(filepath.Join(c, FixDir, "all")); err != nil || !st.IsDir() {
			t.Errorf("%s: no fix/all", filepath.Base(c))
		}
		if len(key.Defects) < 2 {
			continue
		}
		for _, d := range key.Defects {
			if st, err := os.Stat(filepath.Join(c, FixDir, "keep-"+d.ID)); err != nil || !st.IsDir() {
				t.Errorf("%s: no fix/keep-%s", filepath.Base(c), d.ID)
			}
		}
	}
}

// A defect's line is the statement fix/all corrects. A formatter that reflows the fixture after the key was
// written moves that statement and leaves the key pointing at a blank line or a signature, so a finding that
// cites the real line matches only by keyword. Checked for family-tagged cases, which are written to this rule.
func TestCorpusKeyLinesSitOnTheFixedStatement(t *testing.T) {
	cases := vendoredCases(t)
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil || key.Family == "" {
			continue
		}
		for _, d := range key.Defects {
			fixture, err := os.ReadFile(filepath.Join(c, FixtureDir, d.File))
			if err != nil {
				t.Errorf("%s %s: %v", filepath.Base(c), d.ID, err)
				continue
			}
			fixed, err := os.ReadFile(filepath.Join(c, FixDir, "all", d.File))
			if err != nil {
				t.Errorf("%s %s: fix/all does not correct %s", filepath.Base(c), d.ID, d.File)
				continue
			}
			lines := strings.Split(string(fixture), "\n")
			if d.Line > len(lines) {
				t.Errorf("%s %s: line %d is past the end of %s", filepath.Base(c), d.ID, d.Line, d.File)
				continue
			}
			cited := lines[d.Line-1]
			// The fix may rewrite the statement, delete one of its repeats, or move it (an ordering defect keeps
			// the statement and changes where it runs), so the test asks the diff: the cited line must sit on or
			// next to a line fix/all changes. A reflowed key points several lines away from any change.
			if strings.TrimSpace(cited) == "" || !nearChange(changedLines(lines, strings.Split(string(fixed), "\n")), d.Line-1) {
				t.Errorf("%s %s: %s:%d %q is not a statement fix/all changes", filepath.Base(c), d.ID, d.File, d.Line, strings.TrimSpace(cited))
			}
		}
	}
}

// changedLines marks the lines of a that a longest-common-subsequence diff against b does not keep, and the
// line of a that an insertion from b lands before.
func changedLines(a, b []string) []bool {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	changed := make([]bool, len(a))
	for i, j := 0, 0; i < len(a); {
		switch {
		case j < len(b) && a[i] == b[j]:
			i, j = i+1, j+1
		case j < len(b) && lcs[i][j+1] >= lcs[i+1][j]:
			changed[i] = true
			j++
		default:
			changed[i] = true
			i++
		}
	}
	return changed
}

// A fix that only inserts statements changes no fixture line, yet it corrects the place it is inserted: the
// fixture line it lands before counts as changed, so a key line on the statement above it sits next to a change.
func TestChangedLinesMarksTheLineAnInsertionLandsBefore(t *testing.T) {
	fixture := []string{"a", "b", "c"}
	fixed := []string{"a", "b", "inserted", "c"}
	changed := changedLines(fixture, fixed)
	if changed[0] || changed[1] || !changed[2] {
		t.Fatalf("changedLines = %v, want only the line after the insertion (c) marked", changed)
	}
	if !nearChange(changed, 1) {
		t.Fatalf("the statement right above the insertion must sit next to a change")
	}
}

// nearChange reports whether line i, or a line next to it, is changed: a swap of two adjacent statements
// leaves one of the pair in the common subsequence.
func nearChange(changed []bool, i int) bool {
	for k := i - 1; k <= i+1; k++ {
		if k >= 0 && k < len(changed) && changed[k] {
			return true
		}
	}
	return false
}

// A fix variant of a Go case is an overlay: it holds only the files the fix changes, so on its own it often does
// not compile (it uses types the fixture's other files declare). Without a module of its own it is a package of
// this repository's module, and `go test ./...` and `go vet ./...` at the root fail on it. Each variant carries
// its fixture's go.mod, byte for byte: that makes it a module of its own, and overlaying it changes nothing.
func TestCorpusGoFixVariantsAreModulesOfTheirOwn(t *testing.T) {
	cases := vendoredCases(t)
	for _, c := range cases {
		want, err := os.ReadFile(filepath.Join(c, FixtureDir, "go.mod"))
		if err != nil {
			continue // not a Go case
		}
		variants, _ := filepath.Glob(filepath.Join(c, FixDir, "*"))
		for _, v := range variants {
			goFiles, _ := filepath.Glob(filepath.Join(v, "**", "*.go"))
			top, _ := filepath.Glob(filepath.Join(v, "*.go"))
			if len(goFiles)+len(top) == 0 {
				continue
			}
			got, err := os.ReadFile(filepath.Join(v, "go.mod"))
			if err != nil {
				t.Errorf("%s: a Go fix variant without its own go.mod is a package of this module", v)
				continue
			}
			if string(got) != string(want) {
				t.Errorf("%s/go.mod differs from the fixture's: overlaying it would change the case", v)
			}
		}
	}
}

func TestCorpusShipsCleanControlsWithoutFixVersions(t *testing.T) {
	cases := vendoredCases(t)
	controls := 0
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil || !key.IsCleanControl() {
			continue
		}
		controls++
		if len(key.Defects) != 0 {
			t.Errorf("%s: clean control declares %d defects", filepath.Base(c), len(key.Defects))
		}
		if _, err := os.Stat(filepath.Join(c, FixDir)); !os.IsNotExist(err) {
			t.Errorf("%s: clean control ships a fix directory", filepath.Base(c))
		}
	}
	if controls == 0 {
		t.Fatal("no clean controls in shipped corpus")
	}
}

// The happy suite is green on the fixture and on every fixed variant: a variant that breaks the
// suite would make the check read "red" for the wrong reason.
func TestCorpusSuitesAreGreenOnEveryVariant(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the corpus suites")
	}
	cases := vendoredCases(t)
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil {
			continue
		}
		variants := []string{""}
		if entries, err := os.ReadDir(filepath.Join(c, FixDir)); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					variants = append(variants, filepath.Join(c, FixDir, e.Name()))
				}
			}
		}
		for _, v := range variants {
			green, out, err := suiteOn(filepath.Join(c, FixtureDir), v, c, nil, key.Suite, 5*time.Minute)
			if err != nil || !green {
				t.Errorf("%s variant %q: green=%v err=%v\n%s", filepath.Base(c), filepath.Base(v), green, err, tail(out, 600))
			}
		}
	}
}
