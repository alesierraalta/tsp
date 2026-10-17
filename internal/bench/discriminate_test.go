package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A synthetic case whose "suite" is a shell script: src.txt holds one line per defect and each
// test under tests/ greps for the fixed line. No toolchain beyond sh is needed.
func shellCase(t *testing.T) (caseDir string, key Key) {
	t.Helper()
	caseDir = t.TempDir()
	files := map[string]string{
		"fixture/src.txt":        "D1=bug\nD2=bug\n",
		"fixture/run-tests.sh":   "for f in tests/*.sh; do [ -e \"$f\" ] || continue; sh \"$f\" || exit 1; done\nexit 0\n",
		"fixture/tests/happy.sh": "grep -q D1= src.txt\n",
		"fix/all/src.txt":        "D1=ok\nD2=ok\n",
		"fix/keep-D1/src.txt":    "D1=bug\nD2=ok\n",
		"fix/keep-D2/src.txt":    "D1=ok\nD2=bug\n",
	}
	for p, c := range files {
		full := filepath.Join(caseDir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	key = Key{ID: "fake", Suite: "sh run-tests.sh", Defects: []Defect{{ID: "D1", File: "src.txt", Line: 1}, {ID: "D2", File: "src.txt", Line: 2}}}
	return caseDir, key
}

// agentWorkspace copies the fixture and adds the agent's files on top.
func agentWorkspace(t *testing.T, caseDir string, agentFiles map[string]string) string {
	t.Helper()
	ws := t.TempDir()
	if err := copyTree(filepath.Join(caseDir, FixtureDir), ws); err != nil {
		t.Fatal(err)
	}
	for p, c := range agentFiles {
		full := filepath.Join(ws, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func TestDiscriminate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs sh")
	}
	caseDir, key := shellCase(t)
	cases := []struct {
		name       string
		agentFiles map[string]string
		wantAll    bool
		wantCaught map[string]bool
		wantTests  int
	}{
		{
			name:       "a test that is red only while D1 remains catches D1 and not D2",
			agentFiles: map[string]string{"tests/d1.sh": "grep -q D1=ok src.txt\n"},
			wantAll:    true, wantCaught: map[string]bool{"D1": true, "D2": false}, wantTests: 1,
		},
		{
			name:       "tests for both defects catch both",
			agentFiles: map[string]string{"tests/d1.sh": "grep -q D1=ok src.txt\n", "tests/d2.sh": "grep -q D2=ok src.txt\n"},
			wantAll:    true, wantCaught: map[string]bool{"D1": true, "D2": true}, wantTests: 2,
		},
		{
			name:       "a test that fails on the fixed code proves nothing",
			agentFiles: map[string]string{"tests/broken.sh": "exit 1\n"},
			wantAll:    false, wantCaught: map[string]bool{"D1": false, "D2": false}, wantTests: 1,
		},
		{
			name:       "no test files means nothing caught",
			agentFiles: map[string]string{"src.txt": "D1=ok\nD2=ok\n"},
			wantAll:    true, wantCaught: map[string]bool{"D1": false, "D2": false}, wantTests: 0,
		},
		{
			name:       "a source change by the agent is not carried into the check",
			agentFiles: map[string]string{"src.txt": "D1=ok\nD2=ok\n", "tests/d1.sh": "grep -q D1=ok src.txt\n"},
			wantAll:    true, wantCaught: map[string]bool{"D1": true, "D2": false}, wantTests: 1,
		},
		{
			name:       "a test that passes everywhere catches nothing",
			agentFiles: map[string]string{"tests/weak.sh": "grep -q D1= src.txt\n"},
			wantAll:    true, wantCaught: map[string]bool{"D1": false, "D2": false}, wantTests: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := agentWorkspace(t, caseDir, tc.agentFiles)
			got := Discriminate(caseDir, ws, key, 30*time.Second)
			if !got.Checked {
				t.Fatalf("not checked: %v", got.Notes)
			}
			if len(got.TestFiles) != tc.wantTests {
				t.Fatalf("test files = %v, want %d", got.TestFiles, tc.wantTests)
			}
			if tc.wantTests > 0 && got.AllGreen != tc.wantAll {
				t.Fatalf("all green = %v, want %v (%v)", got.AllGreen, tc.wantAll, got.Notes)
			}
			for id, want := range tc.wantCaught {
				if got.Caught[id] != want {
					t.Fatalf("caught[%s] = %v, want %v (%v)", id, got.Caught[id], want, got.Notes)
				}
			}
		})
	}
}

func TestDiscriminateWithoutFixedVersion(t *testing.T) {
	caseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(caseDir, FixtureDir), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Discriminate(caseDir, t.TempDir(), Key{ID: "x", Defects: []Defect{{ID: "D1"}}}, time.Second)
	if got.Checked {
		t.Fatal("checked without fix/all")
	}
}

func TestDiscriminateCleanControlSkipsCatchCheck(t *testing.T) {
	got := Discriminate(t.TempDir(), t.TempDir(), Key{ID: "clean", Control: ControlClean}, time.Second)
	if got.Checked {
		t.Fatal("clean control was checked")
	}
	notes := strings.Join(got.Notes, "; ")
	if !strings.Contains(notes, "case plants no defect") {
		t.Fatalf("notes = %q, want no-defect explanation", notes)
	}
	if strings.Contains(notes, "fix/all") {
		t.Fatalf("notes = %q, clean control must not report missing fix/all", notes)
	}
}

func TestDiscriminateSingleDefectUsesThePristineFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("runs sh")
	}
	caseDir, key := shellCase(t)
	// One defect, no keep-D1 directory: the pristine fixture is the variant where D1 remains.
	_ = os.RemoveAll(filepath.Join(caseDir, "fix", "keep-D1"))
	_ = os.RemoveAll(filepath.Join(caseDir, "fix", "keep-D2"))
	key.Defects = key.Defects[:1]
	ws := agentWorkspace(t, caseDir, map[string]string{"tests/d1.sh": "grep -q D1=ok src.txt\n"})
	got := Discriminate(caseDir, ws, key, 30*time.Second)
	if !got.Caught["D1"] {
		t.Fatalf("D1 not caught: %v", got.Notes)
	}
}

func TestDiscriminateMultiDefectWithoutKeepVariantIsUncheckable(t *testing.T) {
	if testing.Short() {
		t.Skip("runs sh")
	}
	caseDir, key := shellCase(t)
	_ = os.RemoveAll(filepath.Join(caseDir, "fix", "keep-D2"))
	ws := agentWorkspace(t, caseDir, map[string]string{"tests/d2.sh": "grep -q D2=ok src.txt\n"})
	got := Discriminate(caseDir, ws, key, 30*time.Second)
	if got.Caught["D2"] {
		t.Fatal("D2 credited without a keep-D2 variant")
	}
	if len(got.Notes) == 0 {
		t.Fatal("missing variant not reported")
	}
}

func TestIsTestFile(t *testing.T) {
	yes := []string{"tests/a.sh", "test/x.js", "__tests__/y.tsx", "src/a.test.js", "src/a.spec.ts", "pkg/a_test.go", "spec/z_spec.rb", "tests/test_x.py",
		// node --test runs these by default, so a suite may keep its tests in them.
		"test.js", "pkg/test.mjs", "src/a-test.js", "src/a_test.cjs", "src/test-a.js"}
	no := []string{"src/a.js", "config.go", "docs/testing/test-plan.md", "node_modules/x/test/a.js", ".git/hooks/test", "testdata/a.txt",
		"src/latest.js", "src/contest.js", "src/testing.js"}
	for _, p := range yes {
		if !isTestFile(p) {
			t.Errorf("%s should be a test file", p)
		}
	}
	for _, p := range no {
		if isTestFile(p) {
			t.Errorf("%s should not be a test file", p)
		}
	}
}
