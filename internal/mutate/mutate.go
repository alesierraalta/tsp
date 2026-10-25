// Package mutate runs mutation testing on the part of a change that has tests, and reports what those
// tests failed to kill.
//
// The harness owns the policy, not the mutation engine. Policy means four things: the scope is the
// production source the change touched, never the whole repository; the budget is the operator's
// (--max-mutants and a wall clock), and a change wider than the budget is refused before the
// expensive run instead of running for hours; the result is normalized into one shape whatever engine
// produced it; and the gate is the number of survivors, not the engine's own efficacy percentage. The
// engine stays external and per stack (gremlins for Go, Stryker for JavaScript, mutmut for Python)
// because a mutation engine is a project-specific investment this tool has no business reimplementing.
package mutate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/gitdiff"
)

// ErrUnusable marks a run that measured nothing: no engine, a change above the budget, or an engine
// that failed. It is never reported as a pass, because "no survivors" and "nothing ran" look alike
// from the outside and only one of them means the tests are worth something.
var ErrUnusable = errors.New("mutation run unusable")

// Mutant statuses the engines agree on, normalized into TSP's vocabulary.
const (
	StatusKilled     = "killed"
	StatusLived      = "lived"
	StatusNotCovered = "not_covered"
	StatusNotViable  = "not_viable"
	StatusTimedOut   = "timed_out"
)

// Mutation is one mutant, normalized: which file and line, which operator, and what happened to it.
type Mutation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Operator string `json:"operator"`
	Status   string `json:"status"`
}

// Report is what a mutation run measured.
type Report struct {
	Engine        string        `json:"engine"`
	Targets       []string      `json:"targets"`
	Budget        int           `json:"budget"`
	MutationCount int           `json:"mutation_count"`
	Killed        int           `json:"killed"`
	Survived      int           `json:"survived"`
	NotCovered    int           `json:"not_covered"`
	NotViable     int           `json:"not_viable"`
	TimedOut      int           `json:"timed_out"`
	Survivors     []Mutation    `json:"survivors,omitempty"`
	Elapsed       time.Duration `json:"elapsed"`
}

// Blocking reports whether the run fails delivery at this survivor threshold. A negative threshold is
// report-only: the survivor list is published and nothing is gated, which is how a repository discovers
// its current number without turning the tool off.
func (r Report) Blocking(threshold int) bool {
	return threshold >= 0 && r.Survived > threshold
}

// Unreported is the gap between the mutants the dry run counted and the statuses the live run
// reported back. Mortality is not a vote: a mutant nobody reported is one nobody judged, so the
// number is printed rather than left to look like a kill.
func (r Report) Unreported() int {
	accounted := r.Killed + r.Survived + r.NotCovered + r.NotViable + r.TimedOut
	if accounted >= r.MutationCount {
		return 0
	}
	return r.MutationCount - accounted
}

// Deps are the process boundaries, injected so the decision is testable without a repository.
type Deps struct {
	Git      func(dir string, args ...string) (string, error)
	LookPath func(name string) (string, error)
	Run      func(ctx context.Context, dir, name string, args ...string) (string, error)
}

// Options are one mutation run.
type Options struct {
	Cwd        string
	Base       string        // commit-ish the change is measured from; empty reads the working tree
	MaxMutants int           // 0 means no budget; a change above it is refused before the run
	Timeout    time.Duration // 0 means the engine's own default
	Workers    int           // 0 lets the engine decide
	// TimeoutCoefficient multiplies the engine's per-mutant test budget. Zero keeps its default,
	// which is short enough that a slow package reports timeouts instead of verdicts.
	TimeoutCoefficient int
	Deps               Deps
}

// Run measures the change and returns what the tests failed to kill.
func Run(ctx context.Context, opts Options) (Report, error) {
	d := engineDeps(opts.Deps)
	root, err := gitdiff.Root(opts.Cwd, d.Git)
	if err != nil {
		return Report{}, fmt.Errorf("%w: not a git repository: nothing to mutate", ErrUnusable)
	}
	changed, err := changedPaths(root, opts.Base, d)
	if err != nil {
		return Report{}, err
	}
	targets := goTargets(changed)
	report := Report{Targets: targets, Budget: opts.MaxMutants}
	if len(targets) == 0 {
		return report, nil
	}
	engine, err := findEngine(d)
	if err != nil {
		return report, err
	}
	report.Engine = "gremlins"
	work, err := workspace()
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(work)

	if err := countMutants(ctx, &report, opts, d, root, engine, work); err != nil {
		return report, err
	}
	if report.MutationCount == 0 {
		return report, nil
	}

	started := time.Now()
	if err := judgeMutants(ctx, &report, opts, d, root, engine, work); err != nil {
		return report, err
	}
	report.Elapsed = time.Since(started)
	if report.Killed+report.Survived == 0 && report.MutationCount > 0 {
		return report, fmt.Errorf("%w: no mutant was judged (%d timed out, %d not covered, %d not viable): the tests are slower than the engine's budget, so raise --timeout-coefficient or narrow the change", ErrUnusable, report.TimedOut, report.NotCovered, report.NotViable)
	}
	return report, nil
}

// engineDeps fills the three boundaries a run crosses, so a test can replace each one.
func engineDeps(d Deps) Deps {
	if d.Git == nil {
		d.Git = gitdiff.Default
	}
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	if d.Run == nil {
		d.Run = runAt
	}
	return d
}

// findEngine names the one engine this package drives, and refuses to measure instead of guessing
// when it is not installed.
func findEngine(d Deps) (string, error) {
	engine, err := d.LookPath("gremlins")
	if err != nil {
		return "", fmt.Errorf("%w: gremlins is not installed, so no mutant was measured: install it with \"go install github.com/go-gremlins/gremlins/cmd/gremlins@latest\"", ErrUnusable)
	}
	return engine, nil
}

// workspace is the scratch directory the engine writes its JSON into.
func workspace() (string, error) {
	work, err := os.MkdirTemp("", "tsp-mutate-")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	return work, nil
}

// countMutants is the budget's evidence: the dry run finds every mutant without running a test, so
// a change too wide to pay for is refused before anything expensive happens. Engines report paths
// relative to the package they analysed, so the packages are counted one at a time.
func countMutants(ctx context.Context, report *Report, opts Options, d Deps, root, engine, work string) error {
	for i, target := range report.Targets {
		dryPath := filepath.Join(work, fmt.Sprintf("dry-%d.json", i))
		if _, err := d.Run(ctx, root, engine, unleashArgs(target, opts, "-d", dryPath)...); err != nil {
			return fmt.Errorf("%w: gremlins could not analyse %s: %v", ErrUnusable, target, err)
		}
		dry, err := readGremlins(dryPath)
		if err != nil {
			return err
		}
		report.MutationCount += dry.total()
	}
	if opts.MaxMutants > 0 && report.MutationCount > opts.MaxMutants {
		return fmt.Errorf("%w: this change reaches %d mutants, above the --max-mutants budget of %d: narrow the change or raise the budget", ErrUnusable, report.MutationCount, opts.MaxMutants)
	}
	return nil
}

// judgeMutants runs the tests once per mutant and keeps the verdicts in the report.
func judgeMutants(ctx context.Context, report *Report, opts Options, d Deps, root, engine, work string) error {
	for i, target := range report.Targets {
		livePath := filepath.Join(work, fmt.Sprintf("live-%d.json", i))
		if _, err := d.Run(ctx, root, engine, unleashArgs(target, opts, "", livePath)...); err != nil {
			return fmt.Errorf("%w: gremlins could not run the mutants of %s: %v", ErrUnusable, target, err)
		}
		live, err := readGremlins(livePath)
		if err != nil {
			return err
		}
		normalize(report, packageDir(target), live)
	}
	return nil
}

// packageDir turns an engine target ("./internal/mode") into the prefix its reported paths carry.
func packageDir(target string) string {
	trimmed := strings.TrimPrefix(target, "./")
	if trimmed == "." || trimmed == "" {
		return ""
	}
	return path.Clean(trimmed)
}

// unleashArgs builds the engine command: the changed packages, the budget's worker count, the output
// file, and the dry-run flag when this is the counting pass.
func unleashArgs(target string, opts Options, dryFlag, out string) []string {
	args := []string{"unleash", target}
	args = append(args, "-o", out)
	if dryFlag != "" {
		args = append(args, dryFlag)
	}
	if opts.Workers > 0 {
		args = append(args, "--workers", fmt.Sprint(opts.Workers))
	}
	if opts.TimeoutCoefficient > 0 {
		args = append(args, "--timeout-coefficient", fmt.Sprint(opts.TimeoutCoefficient))
	}
	return args
}

// normalize keeps the survivors and the counts in TSP's vocabulary, with paths relative to the
// repository so a finding can be pointed at by a person.
func normalize(report *Report, pkg string, result gremlins) {
	for _, file := range result.Files {
		rel := path.Join(pkg, filepath.ToSlash(file.FileName))
		for _, m := range file.Mutations {
			switch normalizeStatus(m.Status) {
			case StatusKilled:
				report.Killed++
			case StatusLived:
				report.Survived++
				report.Survivors = append(report.Survivors, Mutation{File: rel, Line: m.Line, Operator: m.Type, Status: StatusLived})
			case StatusNotCovered:
				report.NotCovered++
			case StatusNotViable:
				report.NotViable++
			case StatusTimedOut:
				report.TimedOut++
			}
		}
	}
	sort.Slice(report.Survivors, func(i, j int) bool {
		if report.Survivors[i].File != report.Survivors[j].File {
			return report.Survivors[i].File < report.Survivors[j].File
		}
		return report.Survivors[i].Line < report.Survivors[j].Line
	})
}

// normalizeStatus maps the engine's words onto the ones this package reports.
func normalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "KILLED":
		return StatusKilled
	case "LIVED":
		return StatusLived
	case "NOT COVERED", "NOT_COVERED":
		return StatusNotCovered
	case "NOT VIABLE", "NOT_VIABLE":
		return StatusNotViable
	case "TIMED OUT", "TIMED_OUT":
		return StatusTimedOut
	}
	return ""
}

// gremlins is the machine-readable result gremlins writes: enough of it to normalize, nothing more.
type gremlins struct {
	Files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
		} `json:"mutations"`
	} `json:"files"`
	Total int `json:"mutants_total"`
}

func (g gremlins) total() int {
	if g.Total > 0 {
		return g.Total
	}
	count := 0
	for _, f := range g.Files {
		count += len(f.Mutations)
	}
	return count
}

func readGremlins(path string) (gremlins, error) {
	var g gremlins
	data, err := os.ReadFile(path)
	if err != nil {
		return g, fmt.Errorf("%w: the engine wrote no result: %v", ErrUnusable, err)
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return g, fmt.Errorf("%w: the engine's result could not be read: %v", ErrUnusable, err)
	}
	return g, nil
}

// changedPaths is the candidate's paths, read the one way every command reads them.
func changedPaths(root, base string, d Deps) ([]string, error) {
	paths, err := gitdiff.Changed(root, base, d.Git)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	return paths, nil
}

// goTargets keeps the Go packages the change touched: a test file is not source to mutate, and a
// dependency tree is nobody's change.
func goTargets(changed []string) []string {
	seen := map[string]bool{}
	var targets []string
	for _, p := range changed {
		p = strings.TrimSpace(filepath.ToSlash(p))
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			continue
		}
		if strings.HasPrefix(p, "vendor/") || strings.Contains(p, "/vendor/") || strings.HasPrefix(p, "testdata/") {
			continue
		}
		dir := path.Dir(p)
		if dir == "." {
			dir = "."
		}
		target := "./" + dir
		if dir == "." {
			target = "."
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func runAt(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
