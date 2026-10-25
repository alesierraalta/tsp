package mutate

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeDeps answers the three boundaries a mutation run crosses without touching a repository, a
// network or a mutation engine: git, the engine lookup, and the engine itself.
type fakeDeps struct {
	changed string
	engine  string
	dry     string
	real    string
	calls   []string
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		Git: func(dir string, args ...string) (string, error) {
			switch args[0] {
			case "rev-parse":
				return "/repo\n", nil
			case "diff":
				return f.changed, nil
			}
			return "", nil
		},
		LookPath: func(name string) (string, error) {
			if f.engine == "" {
				return "", errors.New("not found")
			}
			return f.engine, nil
		},
		Run: func(ctx context.Context, dir, name string, args ...string) (string, error) {
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			body := f.real
			for _, a := range args {
				if a == "-d" {
					body = f.dry
				}
			}
			for i, a := range args {
				if a == "-o" && i+1 < len(args) {
					if err := os.WriteFile(args[i+1], []byte(body), 0o644); err != nil {
						return "", err
					}
				}
			}
			return body, nil
		},
	}
}

const dryOne = `{"files":[{"file_name":"mode.go","mutations":[{"type":"CONDITIONALS_NEGATION","status":"RUNNABLE","line":38,"column":16}]}],"mutants_total":1}`
const realLived = `{"files":[{"file_name":"mode.go","mutations":[{"type":"CONDITIONALS_NEGATION","status":"LIVED","line":38,"column":16}]}],"mutants_total":1,"mutants_killed":0,"mutants_lived":1}`

// A change that touched no Go source owes no mutation run: the tool says so and stays quiet.
func TestNoChangedGoSourceStaysQuiet(t *testing.T) {
	f := &fakeDeps{changed: "README.md\n", engine: "/bin/gremlins"}
	rep, err := Run(context.Background(), Options{Cwd: "/repo", Base: "main", Deps: f.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if rep.MutationCount != 0 || len(f.calls) != 0 {
		t.Fatalf("report %+v, want no engine call for a change with no Go source", rep)
	}
}

// A missing engine is never a pass: without it nothing was measured.
func TestMissingEngineIsUnusable(t *testing.T) {
	f := &fakeDeps{changed: "internal/mode/mode.go\n"}
	_, err := Run(context.Background(), Options{Cwd: "/repo", Base: "main", Deps: f.deps()})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "gremlins") {
		t.Fatalf("err = %v, want an unusable run naming the engine to install", err)
	}
}

// A mutant the tests do not kill is the finding: the change is not pinned by anything.
func TestSurvivorsBlockDelivery(t *testing.T) {
	f := &fakeDeps{changed: "internal/mode/mode.go\n", engine: "/bin/gremlins", dry: dryOne, real: realLived}
	rep, err := Run(context.Background(), Options{Cwd: "/repo", Base: "main", Deps: f.deps()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if rep.Survived != 1 || len(rep.Survivors) != 1 {
		t.Fatalf("report %+v, want one survivor", rep)
	}
	if got := rep.Survivors[0]; got.File != "internal/mode/mode.go" || got.Line != 38 || got.Operator != "CONDITIONALS_NEGATION" {
		t.Fatalf("survivor = %+v, want file, line and operator normalized", got)
	}
	if !rep.Blocking(0) {
		t.Fatal("a survivor must block at threshold 0")
	}
	if rep.Blocking(-1) {
		t.Fatal("a report-only threshold must not block")
	}
}

// The budget refuses a change wider than the operator agreed to pay for, before the expensive run.
func TestBudgetRefusesAWiderChange(t *testing.T) {
	dry := `{"files":[{"file_name":"a.go","mutations":[{"type":"X","status":"RUNNABLE","line":1},{"type":"X","status":"RUNNABLE","line":2},{"type":"X","status":"RUNNABLE","line":3}]}],"mutants_total":3}`
	f := &fakeDeps{changed: "a.go\n", engine: "/bin/gremlins", dry: dry, real: realLived}
	_, err := Run(context.Background(), Options{Cwd: "/repo", Base: "main", MaxMutants: 2, Deps: f.deps()})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("err = %v, want the budget to refuse the run", err)
	}
	for _, call := range f.calls {
		if !strings.Contains(call, "-d") {
			t.Fatalf("the engine ran past its budget: %s", call)
		}
	}
}

// A dry run counts mutants, and the live run reports what happened to each one. When the engine
// reports fewer statuses than it promised, the gap is a number, not a silent kill.
func TestUnreportedMutantsAreCounted(t *testing.T) {
	report := Report{MutationCount: 21, Killed: 2, NotCovered: 1}
	if got := report.Unreported(); got != 18 {
		t.Fatalf("unreported = %d, want 18", got)
	}
	report.NotViable = 18
	if got := report.Unreported(); got != 0 {
		t.Fatalf("unreported = %d, want 0 once every mutant has a status", got)
	}
}

// A run where every mutant timed out judged nothing. Reporting zero survivors there would call
// an unmeasured change clean, so the run is unusable instead.
func TestEveryMutantTimedOutIsUnusable(t *testing.T) {
	dry := `{"files":[{"file_name":"a.go","mutations":[{"type":"X","status":"RUNNABLE","line":1},{"type":"X","status":"RUNNABLE","line":2}]}],"mutants_total":2}`
	real := `{"files":[{"file_name":"a.go","mutations":[{"type":"X","status":"TIMED_OUT","line":1},{"type":"X","status":"TIMED_OUT","line":2}]}],"mutants_total":2}`
	f := &fakeDeps{changed: "a.go\n", engine: "/bin/gremlins", dry: dry, real: real}
	rep, err := Run(context.Background(), Options{Cwd: "/repo", Base: "main", MaxMutants: 10, Deps: f.deps()})
	if !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable", err)
	}
	if rep.TimedOut != 2 {
		t.Fatalf("timed out = %d, want 2", rep.TimedOut)
	}
	if !strings.Contains(err.Error(), "no mutant was judged") {
		t.Fatalf("error does not name the cause: %v", err)
	}
}

// The coefficient reaches the engine: without it the engine's default budget is what produced
// timeouts on a slow package.
func TestTimeoutCoefficientReachesTheEngine(t *testing.T) {
	args := unleashArgs("./internal/manifest", Options{TimeoutCoefficient: 8}, "", "out.json")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--timeout-coefficient 8") {
		t.Fatalf("coefficient missing from %q", joined)
	}
}
