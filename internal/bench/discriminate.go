package bench

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FixDir holds the fixed versions of a case: fix/all with every defect fixed, and fix/keep-<ID>
// with every defect but <ID> fixed. Each holds only the files that differ from the fixture.
const FixDir = "fix"

// CatchResult says which planted defects the agent's tests actually distinguish. A defect is
// caught when the agent's test files are green on the fully fixed code and red on the code where
// only that defect remains; reporting the defect in the plan is a separate measure.
type CatchResult struct {
	Checked       bool                `json:"checked"`               // fix/all exists for the case
	TestFiles     []string            `json:"test_files,omitempty"`  // agent test files carried into the check
	AllGreen      bool                `json:"all_green"`             // every test passes on the fully fixed code
	BrokenTests   int                 `json:"broken_tests"`          // tests red on the fully fixed code; they prove nothing
	RedTests      []string            `json:"red_tests,omitempty"`   // those tests by name, so an outside verdict can be applied to them
	InvertedTests int                 `json:"inverted_tests"`        // of those, tests green while the defect is present: they pin the bug
	InvertedBy    map[string][]string `json:"inverted_by,omitempty"` // defect id -> tests that assert its defective behaviour
	Caught        map[string]bool     `json:"caught"`                // defect id -> caught
	CaughtBy      map[string][]string `json:"caught_by,omitempty"`   // defect id -> tests green on fix/all and red on keep-<id>
	Attempts      map[string]int      `json:"attempts,omitempty"`    // defect id -> how many times its variant was run
	Notes         []string            `json:"notes,omitempty"`
}

// Count returns how many defects were caught.
func (c CatchResult) Count() int {
	n := 0
	for _, v := range c.Caught {
		if v {
			n++
		}
	}
	return n
}

var (
	testPathRe    = regexp.MustCompile(`(^|/)(tests?|__tests__|spec)/|(_test\.go|\.(test|spec)\.[cm]?[jt]sx?|_spec\.rb|(^|/)test_[^/]*\.py|(^|/)test\.[cm]?js|[-_]test\.[cm]?js|(^|/)test-[^/]*\.[cm]?js)$`)
	ignoredPathRe = regexp.MustCompile(`(^|/)(\.git|node_modules|vendor|\.atl|\.claude|docs|testdata)(/|$)`)
)

// isTestFile recognises the test files of the suites the benchmark runs.
func isTestFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	return !ignoredPathRe.MatchString(rel) && testPathRe.MatchString(rel)
}

// Discriminate runs the check for one agent workspace against the case's fixed versions.
func Discriminate(caseDir, ws string, key Key, timeout time.Duration) CatchResult {
	res := CatchResult{Caught: map[string]bool{}}
	if len(key.Defects) == 0 {
		res.Notes = append(res.Notes, "case plants no defect; catch check has nothing to distinguish")
		return res
	}
	fixture := filepath.Join(caseDir, FixtureDir)
	all := filepath.Join(caseDir, FixDir, "all")
	if st, err := os.Stat(all); err != nil || !st.IsDir() {
		res.Notes = append(res.Notes, "no fix/all directory; check skipped")
		return res
	}
	res.Checked = true
	tests, err := agentTestFiles(fixture, ws)
	if err != nil {
		res.Notes = append(res.Notes, "listing agent tests: "+err.Error())
		return res
	}
	res.TestFiles = tests
	if len(tests) == 0 {
		res.Notes = append(res.Notes, "the agent added or changed no test file")
		return res
	}
	onAll, code, out, err := testsOn(fixture, all, ws, tests, key.Suite, timeout)
	if err != nil {
		res.Notes = append(res.Notes, "fix/all: "+err.Error())
		return res
	}
	res.AllGreen = code == 0
	var trusted, red []string // only tests green on the correct code can catch anything
	for name, passed := range onAll {
		if passed {
			trusted = append(trusted, name)
		} else {
			red = append(red, name)
			res.BrokenTests++
		}
	}
	sort.Strings(trusted)
	sort.Strings(red)
	res.RedTests = red
	if len(trusted) == 0 {
		// A test red on correct code is broken or written to a different API: it can prove nothing.
		res.Notes = append(res.Notes, "no agent test is green on the fully fixed code: "+tail(out, 400))
		return res
	}
	recordDefectOutcomes(&res, caseDir, fixture, ws, key, tests, trusted, red, timeout)
	res.InvertedTests = countDistinct(res.InvertedBy)
	if res.InvertedTests > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%d test(s) pin the defective behaviour: green with the defect, red once it is fixed", res.InvertedTests))
	}
	if other := res.BrokenTests - res.InvertedTests; other > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%d test(s) red on the fully fixed code were ignored", other))
	}
	return res
}

func recordDefectOutcomes(res *CatchResult, caseDir, fixture, ws string, key Key, tests, trusted, red []string, timeout time.Duration) {
	for _, d := range key.Defects {
		got := discriminateDefect(caseDir, fixture, ws, key, d, tests, trusted, red, timeout)
		if got.note != "" {
			res.Notes = append(res.Notes, got.note)
		}
		if got.attempts > 0 {
			if res.Attempts == nil {
				res.Attempts = map[string]int{}
			}
			res.Attempts[d.ID] = got.attempts
		}
		if !got.checked {
			continue
		}
		if len(got.caughtBy) > 0 {
			if res.CaughtBy == nil {
				res.CaughtBy = map[string][]string{}
			}
			res.CaughtBy[d.ID] = got.caughtBy
		}
		res.Caught[d.ID] = len(got.caughtBy) > 0
		if len(got.invertedBy) > 0 {
			if res.InvertedBy == nil {
				res.InvertedBy = map[string][]string{}
			}
			res.InvertedBy[d.ID] = got.invertedBy
		}
	}
}

// defectOutcome is what the case's code said about one defect when only that defect was left in place.
type defectOutcome struct {
	checked    bool     // the variant was observed at least once, so its verdict means something
	attempts   int      // how many times the variant ran; zero when it was never reachable
	caughtBy   []string // trusted tests that turn red once only this defect remains
	invertedBy []string // tests that failed on the fixed code and are green while this defect stands
	note       string   // the one thing worth recording, empty when nothing went wrong
}

// discriminateDefect runs the case with only d left defective and reports which of the agent's trusted
// tests turn red, and which tests that failed on the fixed code are green here. A defect whose variant
// directory is missing is not checkable, and a variant that never ran says so: "nothing distinguished it"
// and "nobody looked" are different answers.
func discriminateDefect(caseDir, fixture, ws string, key Key, d Defect, tests, trusted, red []string, timeout time.Duration) defectOutcome {
	overlay := filepath.Join(caseDir, FixDir, "keep-"+d.ID)
	if len(key.Defects) == 1 {
		overlay = "" // the pristine fixture is the code where the only defect remains
	} else if st, err := os.Stat(overlay); err != nil || !st.IsDir() {
		return defectOutcome{note: fmt.Sprintf("%s: no fix/keep-%s directory; not checkable", d.ID, d.ID)}
	}
	// A defect that only manifests on an unlucky interleaving is not observed on every run: one red
	// attempt proves the test can distinguish it, while a green run proves nothing.
	attempts := attemptsForDefect(d)
	redOnKeep, ranOnKeep := map[string]bool{}, map[string]bool{}
	var lastErr error
	for a := 0; a < attempts; a++ {
		outcomes, _, _, err := testsOn(fixture, overlay, ws, tests, key.Suite, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		for name, passed := range outcomes {
			ranOnKeep[name] = true
			if !passed {
				redOnKeep[name] = true
			}
		}
	}
	if len(ranOnKeep) == 0 {
		got := defectOutcome{attempts: attempts}
		if lastErr != nil {
			got.note = d.ID + ": " + lastErr.Error()
		}
		return got
	}
	onKeep := map[string]bool{}
	for name := range ranOnKeep {
		onKeep[name] = !redOnKeep[name]
	}
	got := defectOutcome{checked: true, attempts: attempts}
	// A run that failed but never named a trusted test stopped before reporting it: a test that hangs
	// on the defect is killed by the timeout, or its file is cancelled as a whole. Fixture and variant
	// differ only by this defect, so that test could not pass here and counts as red. A run that
	// failed nothing proves nothing about a test it did not list.
	var stopped []string
	for _, name := range trusted {
		passed, ran := onKeep[name]
		switch {
		case ran && !passed:
			got.caughtBy = append(got.caughtBy, name)
		case !ran && len(redOnKeep) > 0:
			stopped = append(stopped, name)
		}
	}
	if len(got.caughtBy) == 0 && len(stopped) > 0 {
		got.caughtBy = stopped
		got.note = fmt.Sprintf("%s: caught by a run that stopped before reporting %s (hang or cancelled file)", d.ID, strings.Join(stopped, ", "))
	}
	// A test red on the fix and green while the defect stands asserts the defective behaviour: it
	// resists the fix instead of demanding it.
	for _, name := range red {
		if passed, ran := onKeep[name]; ran && passed {
			got.invertedBy = append(got.invertedBy, name)
		}
	}
	return got
}

// countDistinct counts the distinct test names across every defect's list.
func countDistinct(byDefect map[string][]string) int {
	seen := map[string]bool{}
	for _, names := range byDefect {
		for _, n := range names {
			seen[n] = true
		}
	}
	return len(seen)
}

// agentTestFiles lists test files the agent added or changed, relative to the workspace.
func agentTestFiles(fixture, ws string) ([]string, error) {
	var out []string
	err := filepath.Walk(ws, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(ws, path)
		if info.IsDir() {
			if rel != "." && ignoredPathRe.MatchString(filepath.ToSlash(rel)+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTestFile(rel) {
			return nil
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if orig, err := os.ReadFile(filepath.Join(fixture, rel)); err == nil && bytes.Equal(orig, got) {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// suiteOn runs the suite on fixture + overlay + the agent's test files, in a fresh directory. A suite the
// shared splitter cannot read is reported instead of run, so the caller records the case as unreadable
// rather than as a suite failure the fixture never had.
func suiteOn(fixture, overlay, ws string, tests []string, suite string, timeout time.Duration) (green bool, output string, err error) {
	dir, err := stage(fixture, overlay, ws, tests)
	if err != nil {
		return false, "", err
	}
	defer os.RemoveAll(dir)
	r, err := RunSuite(dir, suite, timeout)
	if err != nil {
		return false, r.Output, err
	}
	return r.ExitCode == 0, r.Output, nil
}

// testsOn is suiteOn with one outcome per test.
func testsOn(fixture, overlay, ws string, tests []string, suite string, timeout time.Duration) (map[string]bool, int, string, error) {
	dir, err := stage(fixture, overlay, ws, tests)
	if err != nil {
		return nil, -1, "", err
	}
	defer os.RemoveAll(dir)
	return perTest(dir, suite, timeout, tests...)
}

// stage builds fixture + overlay + the agent's test files in a fresh directory.
func stage(fixture, overlay, ws string, tests []string) (string, error) {
	dir, err := os.MkdirTemp(stageRoot(), "tpp-catch-")
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) { os.RemoveAll(dir); return "", err }
	if err := copyTree(fixture, dir); err != nil {
		return fail(err)
	}
	if overlay != "" {
		if err := copyTree(overlay, dir); err != nil {
			return fail(err)
		}
	}
	for _, rel := range tests {
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fail(err)
		}
		data, err := os.ReadFile(filepath.Join(ws, filepath.FromSlash(rel)))
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fail(err)
		}
	}
	return dir, nil
}

// attemptsForDefect says how many times a defect's variant is run before concluding that no test
// distinguishes it. A race or a timing defect appears only on some interleavings.
func attemptsForDefect(d Defect) int {
	class := strings.ToLower(d.Class)
	for _, marker := range []string{"race", "concurren", "timing", "flak"} {
		if strings.Contains(class, marker) {
			return 3
		}
	}
	return 1
}
