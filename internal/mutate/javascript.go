package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// jsReportPath is where Stryker's json reporter writes, relative to the repository root.
const jsReportPath = "reports/mutation/mutation.json"

// jsStatuses maps Stryker's words onto the vocabulary every engine reports in. An error is not a
// survivor and not a pass: a mutant that could not compile was never judged, so it is not viable.
var jsStatuses = map[string]string{
	"Killed":       StatusKilled,
	"Survived":     StatusLived,
	"NoCoverage":   StatusNotCovered,
	"Timeout":      StatusTimedOut,
	"CompileError": StatusNotViable,
	"RuntimeError": StatusNotViable,
}

// jsTestSuffixes are the files that catch a mutant, never the mutants themselves.
var jsTestSuffixes = []string{".test.js", ".test.jsx", ".test.ts", ".test.tsx", ".spec.js", ".spec.jsx", ".spec.ts", ".spec.tsx"}

// jsTargets keeps the changed JavaScript and TypeScript source, dropping the tests, the installed
// packages and the trees an engine writes.
func jsTargets(changed []string) []string {
	seen := map[string]bool{}
	var targets []string
	for _, path := range changed {
		slash := filepath.ToSlash(strings.TrimSpace(path))
		if !jsSource(slash) || seen[slash] {
			continue
		}
		seen[slash] = true
		targets = append(targets, slash)
	}
	return targets
}

// jsSource answers whether a changed path is JavaScript source an engine can mutate: not a test, not an
// installed package, not a tree an engine writes.
func jsSource(slash string) bool {
	if !jsExtensions(slash) {
		return false
	}
	for _, ignored := range []string{"node_modules/", "mutants/", "reports/", "/__tests__/"} {
		if strings.Contains(slash, ignored) {
			return false
		}
	}
	base := slash[strings.LastIndex(slash, "/")+1:]
	for _, suffix := range jsTestSuffixes {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}

// jsExtensions reports whether a path is one Stryker can mutate.
func jsExtensions(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx":
		return true
	}
	return false
}

// jsConfig finds the file where Stryker is told which runner and which plugins to use. Without it the
// engine has nothing to run, so the refusal names the file to write.
func jsConfig(root string) (string, error) {
	for _, candidate := range []string{"stryker.conf.json", "stryker.conf.js", "stryker.conf.mjs", "stryker.config.json", "stryker.config.mjs"} {
		if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
			return candidate, nil
		}
	}
	if body, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil && strings.Contains(string(body), `"stryker"`) {
		return "package.json", nil
	}
	return "", fmt.Errorf("%w: Stryker needs a config naming its test runner: write stryker.conf.json with testRunner, plugins and mutate, then run again", ErrUnusable)
}

// jsEngine finds a local Stryker first, so a repository pinning its own version is measured with it.
func jsEngine(root string, d Deps) (string, []string, error) {
	local := filepath.Join(root, "node_modules", ".bin", "stryker")
	if _, err := os.Stat(local); err == nil {
		return local, nil, nil
	}
	npx, err := d.LookPath("npx")
	if err != nil {
		return "", nil, fmt.Errorf("%w: neither a local Stryker nor npx is installed, so no mutant was measured: npm install --save-dev @stryker-mutator/core", ErrUnusable)
	}
	return npx, []string{"--yes", "@stryker-mutator/core"}, nil
}

// runJavaScript drives Stryker over the change and normalizes its report.
func runJavaScript(ctx context.Context, opts Options, d Deps, root string, targets []string) (Report, error) {
	report := Report{Targets: targets, Budget: opts.MaxMutants, Engine: "stryker"}
	if _, err := jsConfig(root); err != nil {
		return report, err
	}
	binary, prefix, err := jsEngine(root, d)
	if err != nil {
		return report, err
	}
	if err := runStryker(ctx, d, root, binary, prefix, targets); err != nil {
		return report, err
	}
	mutants, err := readJSReport(filepath.Join(root, jsReportPath), targets)
	if err != nil {
		return report, err
	}
	countJSMutants(&report, mutants)
	report.MutationCount = len(mutants)
	if report.MutationCount == 0 {
		return report, fmt.Errorf("%w: Stryker ran and reported no mutant: the change has no JavaScript source Stryker can change", ErrUnusable)
	}
	if opts.MaxMutants > 0 && report.MutationCount > opts.MaxMutants {
		return report, fmt.Errorf("%w: this change reaches %d mutants, above the --max-mutants budget of %d: narrow the change or raise the budget", ErrUnusable, report.MutationCount, opts.MaxMutants)
	}
	if report.Killed+report.Survived == 0 {
		return report, fmt.Errorf("%w: Stryker judged no mutant (%d not covered, %d not viable): the tests never import the changed code", ErrUnusable, report.NotCovered, report.NotViable)
	}
	sort.Slice(report.Survivors, func(i, j int) bool {
		if report.Survivors[i].File != report.Survivors[j].File {
			return report.Survivors[i].File < report.Survivors[j].File
		}
		return report.Survivors[i].Line < report.Survivors[j].Line
	})
	return report, nil
}

// strykerReport is the part of Stryker's json report this adapter reads.
type strykerReport struct {
	Files map[string]struct {
		Mutants []struct {
			MutatorName string `json:"mutatorName"`
			Status      string `json:"status"`
			Location    struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"location"`
		} `json:"mutants"`
	} `json:"files"`
}

// readJSReport normalizes the report, keeping only the files the change touched: a report is about a
// repository, the answer is about a change.
func readJSReport(path string, scope []string) ([]Mutation, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: Stryker wrote no report at %s: %v", ErrUnusable, jsReportPath, err)
	}
	var decoded strykerReport
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("%w: Stryker's report could not be read: %v", ErrUnusable, err)
	}
	keep := map[string]bool{}
	for _, path := range scope {
		keep[filepath.ToSlash(path)] = true
	}
	var out []Mutation
	for file, result := range decoded.Files {
		rel := filepath.ToSlash(strings.TrimPrefix(file, "./"))
		if !keep[rel] {
			continue
		}
		for _, mutant := range result.Mutants {
			status, known := jsStatuses[mutant.Status]
			if !known {
				continue
			}
			out = append(out, Mutation{File: rel, Line: mutant.Location.Start.Line, Operator: mutant.MutatorName, Status: status})
		}
	}
	return out, nil
}

// runStryker drives the engine. Stryker exits non-zero when mutants survive, which is a result and not a
// failure: the run only fails when no report was written at all.
func runStryker(ctx context.Context, d Deps, root, binary string, prefix, targets []string) error {
	args := append(append([]string{}, prefix...), "run", "--reporters", "json", "--mutate", strings.Join(targets, ","))
	if _, err := d.Run(ctx, root, binary, args...); err != nil {
		if _, statErr := os.Stat(filepath.Join(root, jsReportPath)); statErr != nil {
			return fmt.Errorf("%w: Stryker could not run: %v", ErrUnusable, err)
		}
	}
	return nil
}

// countJSMutants folds a report into the same counters every engine feeds.
func countJSMutants(report *Report, mutants []Mutation) {
	for _, mutant := range mutants {
		switch mutant.Status {
		case StatusKilled:
			report.Killed++
		case StatusLived:
			report.Survived++
			report.Survivors = append(report.Survivors, mutant)
		case StatusNotCovered:
			report.NotCovered++
		case StatusTimedOut:
			report.TimedOut++
		case StatusNotViable:
			report.NotViable++
		}
	}
}
