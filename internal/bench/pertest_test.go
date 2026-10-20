package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPerTestDoesNotWritePythonCache(t *testing.T) {
	if testing.Short() {
		t.Skip("real Python runner")
	}
	t.Setenv("TSP_HOME", t.TempDir())
	t.Setenv("PYTHONDONTWRITEBYTECODE", "")
	dir := t.TempDir()
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	if err := os.WriteFile(filepath.Join(dir, "test_sample.py"), []byte("def test_sample():\n    assert True\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outcomes, code, out, err := PerTest(dir, "python3 -m pytest test_sample.py", 30*time.Second)
	if err != nil || code != 0 || !outcomes["test_sample.py::test_sample"] {
		t.Fatalf("per-test outcome: %v %d %v %s", outcomes, code, err, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "test_sample.py" {
		t.Fatalf("runner wrote into repository: %v %v", entries, err)
	}
}

func TestParseTAP(t *testing.T) {
	out := "TAP version 13\n# Subtest: escapes tags\nok 1 - escapes tags\n  ---\n  duration_ms: 0.8\n  ...\nnot ok 2 - escapes single quotes\n  ---\n  error: 'x'\n  ...\n# Subtest: file\n    ok 1 - nested case\nnot ok 3 - file\n1..3\n"
	got := parseTAP(out)
	want := map[string]bool{"escapes tags": true, "escapes single quotes": false, "file/nested case": true, "file": false}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if p, ok := got[k]; !ok || p != v {
			t.Fatalf("%s: got %v/%v, want %v (%v)", k, p, ok, v, got)
		}
	}
}

func TestParseGoJSON(t *testing.T) {
	out := `{"Action":"run","Package":"p","Test":"TestA"}
{"Action":"pass","Package":"p","Test":"TestA","Elapsed":0}
{"Action":"fail","Package":"p","Test":"TestB","Elapsed":0}
{"Action":"pass","Package":"p/sub","Test":"TestA/case_1","Elapsed":0}
{"Action":"fail","Package":"p","Elapsed":0.1}
`
	got := parseGoJSON(out)
	want := map[string]bool{"p.TestA": true, "p.TestB": false, "p/sub.TestA/case_1": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %v want %v", k, got[k], v)
		}
	}
}

func TestPerTestCommand(t *testing.T) {
	cases := map[string][]string{
		"node --test":             {"node", "--test", "--test-reporter=tap"},
		"go test ./...":           {"go", "test", "-json", "./..."},
		"go test -race ./...":     {"go", "test", "-json", "-race", "./..."},
		"sh run-tests.sh":         {"sh", "run-tests.sh"},
		"python3 -m pytest tests": {"python3", "-m", "pytest", "-rA", "-p", "no:cacheprovider", "tests"},
		"pytest -q":               {"pytest", "-rA", "-p", "no:cacheprovider", "-q"},
	}
	for suite, want := range cases {
		got, _, err := perTestCommand(suite)
		if err != nil {
			t.Fatalf("%q: %v", suite, err)
		}
		if len(got) != len(want) {
			t.Fatalf("%q: %v", suite, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%q: %v, want %v", suite, got, want)
			}
		}
	}
}

// A test that goes green once the bug is back asserts the defective behaviour instead of demanding the
// fix, and a test that is red everywhere proves nothing: the two are counted apart, because the first is
// what a human has to be told about. Only a suite the splitter can read per test shows either.
func TestDiscriminateNamesTheTestThatPinsTheDefect(t *testing.T) {
	if testing.Short() {
		t.Skip("runs node")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	caseDir, key := nodeCase(t)
	ws := agentWorkspace(t, caseDir, map[string]string{
		"tests/agent.test.js": "const test = require('node:test'); const assert = require('node:assert'); const s = require('../src.js');\n" +
			"test('demands the fix', () => { assert.equal(s.d1(), 'ok'); });\n" +
			"test('pins the bug', () => { assert.equal(s.d1(), 'bug'); });\n" +
			"test('always broken', () => { assert.equal(1, 2); });\n",
	})
	got := Discriminate(caseDir, ws, key, 2*time.Minute)
	if !got.Checked {
		t.Fatalf("not checked: %v", got.Notes)
	}
	if !got.Caught["D1"] || got.Caught["D2"] {
		t.Fatalf("caught = %v notes=%v", got.Caught, got.Notes)
	}
	if got.InvertedTests != 1 {
		t.Fatalf("inverted tests = %d, want 1: %v", got.InvertedTests, got.InvertedBy)
	}
	if names := got.InvertedBy["D1"]; len(names) != 1 || !strings.Contains(names[0], "pins the bug") {
		t.Fatalf("inverted_by[D1] = %v, want the one test that pins the bug", got.InvertedBy)
	}
	if got.BrokenTests != 2 {
		t.Fatalf("broken tests = %d, want 2 (the inverted one and the one that fails everywhere)", got.BrokenTests)
	}
	var inverted, ignored bool
	for _, n := range got.Notes {
		if strings.Contains(n, "pin the defective behaviour") {
			inverted = true
		}
		if strings.Contains(n, "were ignored") {
			ignored = true
		}
	}
	if !inverted || !ignored {
		t.Fatalf("both notes must be reported, got %v", got.Notes)
	}
}

// A real node case: one broken agent test must not hide the test that catches D1.
func TestDiscriminatePerTestSurvivesABrokenTest(t *testing.T) {
	if testing.Short() {
		t.Skip("runs node")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	caseDir, key := nodeCase(t)
	ws := agentWorkspace(t, caseDir, map[string]string{
		"tests/agent.test.js": "const test = require('node:test'); const assert = require('node:assert'); const s = require('../src.js');\n" +
			"test('d1 is fixed', () => { assert.equal(s.d1(), 'ok'); });\n" +
			"test('always broken', () => { assert.equal(1, 2); });\n",
	})
	got := Discriminate(caseDir, ws, key, 2*time.Minute)
	if !got.Checked || got.AllGreen {
		t.Fatalf("checked=%v allGreen=%v notes=%v", got.Checked, got.AllGreen, got.Notes)
	}
	if !got.Caught["D1"] || got.Caught["D2"] {
		t.Fatalf("caught = %v notes=%v", got.Caught, got.Notes)
	}
	if got.BrokenTests != 1 {
		t.Fatalf("broken tests = %d, want 1", got.BrokenTests)
	}
	if by := got.CaughtBy["D1"]; len(by) != 1 || by[0] != "d1 is fixed" {
		t.Fatalf("caught by = %v", got.CaughtBy)
	}
}

// A test that pins the defective behaviour is not a catch: it is green while the defect is there
// and red once it is fixed, so it resists the fix instead of demanding it.
func TestDiscriminateReportsInvertedTests(t *testing.T) {
	if testing.Short() {
		t.Skip("runs node")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	caseDir, key := nodeCase(t)
	ws := agentWorkspace(t, caseDir, map[string]string{
		"tests/agent.test.js": "const test = require('node:test'); const assert = require('node:assert'); const s = require('../src.js');\n" +
			"test('pins the bug', () => { assert.equal(s.d1(), 'bug'); });\n" +
			"test('catches d2', () => { assert.equal(s.d2(), 'ok'); });\n" +
			"test('red everywhere', () => { assert.equal(s.d1(), 'neither'); });\n",
	})
	got := Discriminate(caseDir, ws, key, 2*time.Minute)
	if got.Caught["D1"] {
		t.Fatal("a test asserting the defective value must not count as catching it")
	}
	if !got.Caught["D2"] {
		t.Fatalf("the sound test must still count: %v", got.Notes)
	}
	if got.InvertedTests != 1 {
		t.Fatalf("inverted = %d, want 1 (%v)", got.InvertedTests, got.Notes)
	}
	if got.BrokenTests != 2 {
		t.Fatalf("broken = %d, want 2 (the inverted one and the one red everywhere)", got.BrokenTests)
	}
	if by := got.InvertedBy["D1"]; len(by) != 1 || by[0] != "pins the bug" {
		t.Fatalf("inverted by = %v", got.InvertedBy)
	}
	if by := got.InvertedBy["D2"]; len(by) != 0 {
		t.Fatalf("a test red everywhere is broken, not inverted: %v", got.InvertedBy)
	}
	if !strings.Contains(strings.Join(got.Notes, " "), "pin the defective behaviour") {
		t.Fatalf("notes must name the failure: %v", got.Notes)
	}
}

// A test that hangs while the defect stands is killed by the timeout before the runner names it, so the
// run reports no per-test line at all. It is green on the fixed code and cannot pass on the defective
// code: that is a catch, not a test that never ran.
func TestDiscriminateCountsATestThatHangsOnTheDefectAsCaught(t *testing.T) {
	if testing.Short() {
		t.Skip("runs node")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	caseDir, key := nodeCase(t)
	ws := agentWorkspace(t, caseDir, map[string]string{
		"tests/agent.test.js": "const test = require('node:test'); const assert = require('node:assert'); const s = require('../src.js');\n" +
			"test('d1 settles', () => { while (s.d1() !== 'ok') {} });\n" +
			"test('d2 is fixed', () => { assert.equal(s.d2(), 'ok'); });\n",
	})
	got := Discriminate(caseDir, ws, key, 10*time.Second)
	if !got.Checked || !got.AllGreen {
		t.Fatalf("checked=%v allGreen=%v notes=%v", got.Checked, got.AllGreen, got.Notes)
	}
	if !got.Caught["D1"] || !got.Caught["D2"] {
		t.Fatalf("caught = %v, want both: the hang on D1 is a catch (notes=%v)", got.Caught, got.Notes)
	}
	if by := got.CaughtBy["D2"]; len(by) != 1 || by[0] != "d2 is fixed" {
		t.Fatalf("caught by[D2] = %v, want only the test that named its failure", got.CaughtBy)
	}
	if !strings.Contains(strings.Join(got.Notes, " "), "D1: caught by a run that stopped before reporting") {
		t.Fatalf("notes must say how D1 was caught: %v", got.Notes)
	}
}

// A test absent from a run that failed nothing proves nothing: a green run that simply did not list a
// test is not a catch.
func TestDiscriminateDoesNotCreditAnAbsentTestOnAGreenRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs node")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	caseDir, key := nodeCase(t)
	ws := agentWorkspace(t, caseDir, map[string]string{
		"tests/agent.test.js": "const test = require('node:test'); const s = require('../src.js');\n" +
			"if (s.d1() === 'ok') test('only listed on the fix', () => {});\n" +
			"test('always green', () => {});\n",
	})
	got := Discriminate(caseDir, ws, key, 10*time.Second)
	if got.Caught["D1"] || got.Caught["D2"] {
		t.Fatalf("caught = %v, want none (notes=%v)", got.Caught, got.Notes)
	}
}

// nodeCase is a two-defect fixture whose suite reports one result per test.
func nodeCase(t *testing.T) (string, Key) {
	t.Helper()
	caseDir := t.TempDir()
	files := map[string]string{
		"fixture/src.js":              "module.exports = { d1: () => 'bug', d2: () => 'bug' };\n",
		"fixture/tests/happy.test.js": "const test = require('node:test'); const assert = require('node:assert'); const s = require('../src.js');\ntest('exports', () => { assert.equal(typeof s.d1, 'function'); });\n",
		"fix/all/src.js":              "module.exports = { d1: () => 'ok', d2: () => 'ok' };\n",
		"fix/keep-D1/src.js":          "module.exports = { d1: () => 'bug', d2: () => 'ok' };\n",
		"fix/keep-D2/src.js":          "module.exports = { d1: () => 'ok', d2: () => 'bug' };\n",
	}
	for p, c := range files {
		full := filepath.Join(caseDir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return caseDir, Key{ID: "node", Suite: "node --test", Defects: []Defect{{ID: "D1"}, {ID: "D2"}}}
}
