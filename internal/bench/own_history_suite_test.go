package bench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// corpusTmp returns a TMPDIR inside a git repository. Every process a reading starts gets a git ceiling at
// its workspace parent and at TMPDIR, so the temporary tree of a suite must live under a repository for the
// check to exercise that rule. A caller-provided TMPDIR under ~/bench-runs/tmp already is; otherwise make one.
func corpusTmp(t *testing.T) string {
	t.Helper()
	tmp := os.Getenv("TMPDIR")
	if tmp != "" {
		if st, err := os.Stat(tmp); err == nil && st.IsDir() {
			out, err := exec.Command("git", "-C", tmp, "rev-parse", "--show-toplevel").CombinedOutput()
			if err == nil && strings.TrimSpace(string(out)) != "" {
				return tmp
			}
		}
	}
	root := enclosingRepo(t)
	tmp = filepath.Join(root, "bench-runs", "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	return tmp
}

// Every o* case must survive the path a reading takes: SOURCE.json to a materialized case, scaffold to a
// fixture suite that is green, and the key's own trigger, which must distinguish the fixture from fix/all.
// An exclusion that breaks a suite or a trigger fails here, before a paid reading can spend a run on it.
func TestCorpusOwnHistoryFixtureSuitesUnderAnEnclosingRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("materializes own-history cases and runs their suites and triggers")
	}
	tmp := corpusTmp(t)
	t.Setenv("TMPDIR", tmp)
	runs := filepath.Join(tmp, "o-corpus-check")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	cases, err := filepath.Glob(filepath.Join(corpusDir(t), "o*"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no own-history cases: %v", err)
	}
	for _, dir := range cases {
		id := filepath.Base(dir)
		t.Run(id, func(t *testing.T) {
			built, err := MaterializeCase(dir, DefaultCaseCache())
			if err != nil {
				t.Fatal(err)
			}
			key, err := LoadKey(built)
			if err != nil {
				t.Fatal(err)
			}
			ws, err := os.MkdirTemp(runs, id+"-fixture-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = removeTree(ws) })
			res, err := Scaffold(built, ws, key, 10*time.Minute)
			if err != nil || !res.Green {
				msg := fmt.Sprintf("fixture suite not green: err=%v exit=%d\n%s", err, res.ExitCode, tail(res.Output, 2000))
				if strings.Contains(res.Output, "docs/testing/test-plan.md") {
					msg += "\nThe suite reads the shipped plan: drop docs/testing/test-plan.md from this case's SOURCE.json exclude (its plan rows stay visible)."
				}
				t.Fatal(msg)
			}
			fixed, err := os.MkdirTemp(runs, id+"-fixed-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = removeTree(fixed) })
			if err := copyTree(filepath.Join(built, FixtureDir), fixed); err != nil {
				t.Fatal(err)
			}
			if err := copyTree(filepath.Join(built, FixDir, "all"), fixed); err != nil {
				t.Fatal(err)
			}
			for _, defect := range key.Defects {
				for _, check := range []struct{ name, dir, want string }{
					{"trigger-red-on-fixture", ws, defect.Trigger.Actual},
					{"trigger-green-on-fix", fixed, defect.Trigger.Expected},
				} {
					t.Run(defect.ID+"-"+check.name, func(t *testing.T) {
						// A key trigger is a shell command: it pipes a base64 probe into a test file and then
						// runs go test. RunSuite takes command words, so hand the whole command to sh.
						out, err := RunSuite(check.dir, "sh -c "+shellQuote(defect.Trigger.Input), 10*time.Minute)
						if err != nil {
							t.Fatalf("trigger: %v", err)
						}
						if !strings.Contains(out.Output, check.want) {
							t.Fatalf("trigger = %q, want %q (exit %d)\n%s", tail(out.Output, 200), check.want, out.ExitCode, tail(out.Output, 2000))
						}
					})
				}
			}
		})
	}
}
