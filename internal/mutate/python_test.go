package mutate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pyResults is what mutmut 3.8 prints for `results --all true` after a run on a two-branch function.
const pyResults = `    shop.x_price__mutmut_1: survived
    shop.x_price__mutmut_2: killed
    shop.x_price__mutmut_3: killed
    shop.x_price__mutmut_4: killed
    shop.x_price__mutmut_5: survived
    shop.x_price__mutmut_6: survived
`

// pythonRepo lays out a repository whose only stack is Python, with mutmut's scope declared.
func pythonRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "setup.cfg"), []byte("[mutmut]\nsource_paths=shop.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shop.py"), []byte("def price(units):\n    return units * 8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// pythonDeps answers the three engine calls from canned reports, so the adapter is judged without
// running mutmut.
func pythonDeps(t *testing.T, root, results string) Deps {
	t.Helper()
	return Deps{
		Git: func(_ string, args ...string) (string, error) {
			for _, a := range args {
				if a == "rev-parse" {
					return root + "\n", nil
				}
			}
			return "shop.py", nil
		},
		LookPath: func(name string) (string, error) {
			if name != "mutmut" {
				return "", errors.New("missing " + name)
			}
			return "/fake/mutmut", nil
		},
		Run: func(_ context.Context, _ string, _ string, args ...string) (string, error) {
			switch args[0] {
			case "run":
				return "", nil
			case "results":
				return results, nil
			case "show":
				return "@@ -2,1 +2,1 @@\n-    return units * 8\n+    return units * 9\n", nil
			}
			return "", nil
		},
	}
}

// A Python change is measured by mutmut and normalized: the counters come from the engine's own statuses,
// and each survivor carries the line its diff touches.
func TestPythonChangeIsMeasuredByMutmut(t *testing.T) {
	root := pythonRepo(t)
	report, err := Run(context.Background(), Options{Cwd: root, MaxMutants: 10, Deps: pythonDeps(t, root, pyResults)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Engine != "mutmut" || report.MutationCount != 6 || report.Killed != 3 || report.Survived != 3 {
		t.Fatalf("report = %+v, want mutmut 6 mutants, 3 killed, 3 survived", report)
	}
	if len(report.Survivors) != 3 {
		t.Fatalf("survivors = %+v, want three", report.Survivors)
	}
	first := report.Survivors[0]
	if first.File != "shop.py" || first.Line != 2 {
		t.Fatalf("survivor = %+v, want shop.py:2 from the diff", first)
	}
	if !report.Blocking(0) {
		t.Fatal("a survivor must block at a threshold of zero")
	}
}

// The budget is the operator's: a change that reaches more mutants than allowed is refused.
func TestPythonBudgetRefusesBeforeTheGate(t *testing.T) {
	root := pythonRepo(t)
	_, err := Run(context.Background(), Options{Cwd: root, MaxMutants: 3, Deps: pythonDeps(t, root, pyResults)})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "--max-mutants") {
		t.Fatalf("err = %v, want a budget refusal", err)
	}
}

// A status the adapter does not know is not guessed at, and a run where nothing was judged is unusable
// rather than clean.
func TestPythonRunThatJudgedNothingIsUnusable(t *testing.T) {
	root := pythonRepo(t)
	_, err := Run(context.Background(), Options{Cwd: root, MaxMutants: 10, Deps: pythonDeps(t, root, "    shop.x_price__mutmut_1: not checked\n")})
	if !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable", err)
	}
}

// Without mutmut's section the run says which file to edit instead of failing inside the engine.
func TestPythonWithoutConfigNamesTheFileToEdit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shop.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{Cwd: root, MaxMutants: 10, Deps: pythonDeps(t, root, pyResults)})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "source_paths") {
		t.Fatalf("err = %v, want the config instruction", err)
	}
}

// mutmut keeps a copy of the sources under mutants/: a second run must not measure that copy.
func TestPythonTargetsIgnoreTheEngineScratchTree(t *testing.T) {
	got := pythonTargets([]string{"shop.py", "mutants/shop.py", "tests/test_shop.py"})
	if len(got) != 1 || got[0] != "shop.py" {
		t.Fatalf("targets = %v, want only shop.py", got)
	}
}
