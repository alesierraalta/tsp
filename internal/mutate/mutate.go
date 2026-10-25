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
	Survivors     []Mutation    `json:"survivors,omitempty"`
	Elapsed       time.Duration `json:"elapsed"`
}

// Blocking reports whether the run fails delivery at this survivor threshold. A negative threshold is
// report-only: the survivor list is published and nothing is gated, which is how a repository discovers
// its current number without turning the tool off.
func (r Report) Blocking(threshold int) bool {
	return threshold >= 0 && r.Survived > threshold
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
	Deps       Deps
}

// Run measures the change and returns what the tests failed to kill.
func Run(ctx context.Context, opts Options) (Report, error) {
	d := opts.Deps
	if d.Git == nil {
		d.Git = gitAt
	}
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	if d.Run == nil {
		d.Run = runAt
	}

	rootOut, err := d.Git(opts.Cwd, "rev-parse", "--show-toplevel")
	root := strings.TrimSpace(rootOut)
	if err != nil || root == "" {
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

	engine, err := d.LookPath("gremlins")
	if err != nil {
		return report, fmt.Errorf("%w: gremlins is not installed, so no mutant was measured: install it with \"go install github.com/go-gremlins/gremlins/cmd/gremlins@latest\"", ErrUnusable)
	}
	report.Engine = "gremlins"

	work, err := os.MkdirTemp("", "tsp-mutate-")
	if err != nil {
		return report, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	defer os.RemoveAll(work)

	// The dry run is the budget's evidence: it finds every mutant without running a test, so a change
	// too wide to pay for is refused before anything expensive happens. Engines report paths relative
	// to the package they analysed, so the packages are analysed one at a time and every mutant is
	// attributed to the package it came from.
	for i, target := range targets {
		dryPath := filepath.Join(work, fmt.Sprintf("dry-%d.json", i))
		if _, err := d.Run(ctx, root, engine, unleashArgs(target, opts, "-d", dryPath)...); err != nil {
			return report, fmt.Errorf("%w: gremlins could not analyse %s: %v", ErrUnusable, target, err)
		}
		dry, err := readGremlins(dryPath)
		if err != nil {
			return report, err
		}
		report.MutationCount += dry.total()
	}
	if opts.MaxMutants > 0 && report.MutationCount > opts.MaxMutants {
		return report, fmt.Errorf("%w: this change reaches %d mutants, above the --max-mutants budget of %d: narrow the change or raise the budget", ErrUnusable, report.MutationCount, opts.MaxMutants)
	}
	if report.MutationCount == 0 {
		return report, nil
	}

	started := time.Now()
	for i, target := range targets {
		livePath := filepath.Join(work, fmt.Sprintf("live-%d.json", i))
		if _, err := d.Run(ctx, root, engine, unleashArgs(target, opts, "", livePath)...); err != nil {
			return report, fmt.Errorf("%w: gremlins could not run the mutants of %s: %v", ErrUnusable, target, err)
		}
		live, err := readGremlins(livePath)
		if err != nil {
			return report, err
		}
		normalize(&report, packageDir(target), live)
	}
	report.Elapsed = time.Since(started)
	return report, nil
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

// changedPaths is the candidate's paths: the commit range when Base names one, the working tree otherwise.
func changedPaths(root, base string, d Deps) ([]string, error) {
	if base != "" {
		out, err := d.Git(root, "diff", "--name-only", "--no-renames", base+"...HEAD")
		if err != nil {
			return nil, fmt.Errorf("%w: git diff failed for --base %s", ErrUnusable, base)
		}
		return lines(out), nil
	}
	out, err := d.Git(root, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("%w: git status failed", ErrUnusable)
	}
	var names []string
	for _, entry := range strings.Split(out, "\x00") {
		if len(entry) < 4 {
			continue
		}
		names = append(names, entry[3:])
	}
	return names, nil
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

func lines(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

func gitAt(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

func runAt(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
