package mutate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// pythonExtension is the only source this adapter claims; anything else is another stack's business.
const pythonExtension = ".py"

// pythonResult matches one line of `mutmut results --all true`: the mutant's name and its status.
var pythonResult = regexp.MustCompile(`^\s+(\S+):\s+(\w[\w ]*)$`)

// pythonHunk matches the first line of a diff hunk, which is where a mutant changes the source.
var pythonHunk = regexp.MustCompile(`^@@ -(\d+)`)

// pythonStatuses maps mutmut's words onto the vocabulary every engine reports in. A status this map
// does not know is not guessed at: it is counted as unreported, and a run with nothing reported is
// unusable rather than clean.
var pythonStatuses = map[string]string{
	"killed":      StatusKilled,
	"survived":    StatusLived,
	"suspicious":  StatusLived,
	"timeout":     StatusTimedOut,
	"not checked": StatusNotCovered,
	"untested":    StatusNotCovered,
	"skipped":     StatusNotCovered,
}

// pythonTargets keeps the changed Python source, dropping the tests that are supposed to catch it.
func pythonTargets(changed []string) []string {
	seen := map[string]bool{}
	var targets []string
	for _, path := range changed {
		slash := filepath.ToSlash(strings.TrimSpace(path))
		if !strings.HasSuffix(slash, pythonExtension) {
			continue
		}
		base := slash[strings.LastIndex(slash, "/")+1:]
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || strings.Contains(slash, "/tests/") {
			continue
		}
		if strings.HasPrefix(slash, "vendor/") || strings.Contains(slash, "/.venv/") || strings.HasPrefix(slash, ".venv/") {
			continue
		}
		// mutmut keeps its own copy of the sources under mutants/: a second run must not measure the
		// engine's scratch tree as if it were part of the change.
		if strings.HasPrefix(slash, "mutants/") || strings.Contains(slash, "/mutants/") {
			continue
		}
		if seen[slash] {
			continue
		}
		seen[slash] = true
		targets = append(targets, slash)
	}
	sort.Strings(targets)
	return targets
}

// pythonConfig finds the section mutmut reads its scope from; without it mutmut itself refuses to say
// where the code is, so the run names the file to edit instead of failing somewhere inside the engine.
func pythonConfig(root string, d Deps) (string, error) {
	for _, candidate := range []string{"setup.cfg", "pyproject.toml", ".mutmut.toml"} {
		body, err := os.ReadFile(filepath.Join(root, candidate))
		if err != nil {
			continue
		}
		text := string(body)
		if strings.Contains(text, "[mutmut]") || strings.Contains(text, "[tool.mutmut]") {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: mutmut needs to know where the code is: add [mutmut]\nsource_paths=<dir> to setup.cfg (or [tool.mutmut] to pyproject.toml) and run again", ErrUnusable)
}

// runPython drives mutmut over the change and normalizes what it reports. It is the same contract as
// the Go engine: scope from the change, budget from the operator, statuses in one vocabulary.
func runPython(ctx context.Context, opts Options, d Deps, root string, targets []string) (Report, error) {
	report := Report{Targets: targets, Budget: opts.MaxMutants}
	// The engine comes first: telling a reader to write a config file for a tool they do not have is an
	// instruction they cannot follow.
	binary, err := d.LookPath("mutmut")
	if err != nil {
		return report, fmt.Errorf("%w: mutmut is not installed, so no mutant was measured: install it with `pipx install mutmut`", ErrUnusable)
	}
	if _, err := pythonConfig(root, d); err != nil {
		return report, err
	}
	report.Engine = "mutmut"
	if _, err := d.Run(ctx, root, binary, "run"); err != nil {
		return report, fmt.Errorf("%w: mutmut could not run: %v", ErrUnusable, err)
	}
	results, err := d.Run(ctx, root, binary, "results", "--all", "true")
	if err != nil {
		return report, fmt.Errorf("%w: mutmut ran but its results could not be read: %v", ErrUnusable, err)
	}
	mutants := parsePythonResults(results)
	report.MutationCount = len(mutants)
	if report.MutationCount == 0 {
		return report, fmt.Errorf("%w: mutmut ran and reported no mutant: the change has no Python source mutmut can change", ErrUnusable)
	}
	if opts.MaxMutants > 0 && report.MutationCount > opts.MaxMutants {
		return report, fmt.Errorf("%w: this change reaches %d mutants, above the --max-mutants budget of %d: narrow the change or raise the budget", ErrUnusable, report.MutationCount, opts.MaxMutants)
	}
	for _, mutant := range mutants {
		switch mutant.Status {
		case StatusKilled:
			report.Killed++
		case StatusLived:
			report.Survived++
		case StatusNotCovered:
			report.NotCovered++
		case StatusTimedOut:
			report.TimedOut++
		}
		if mutant.Status == StatusLived {
			mutant.Line = pythonSurvivorLine(ctx, root, binary, mutant.Operator, d)
			report.Survivors = append(report.Survivors, mutant)
		}
	}
	sort.Slice(report.Survivors, func(i, j int) bool {
		if report.Survivors[i].File != report.Survivors[j].File {
			return report.Survivors[i].File < report.Survivors[j].File
		}
		return report.Survivors[i].Line < report.Survivors[j].Line
	})
	if report.Killed+report.Survived == 0 {
		return report, fmt.Errorf("%w: mutmut judged no mutant (%d not covered, %d timed out): the tests never import the changed code", ErrUnusable, report.NotCovered, report.TimedOut)
	}
	return report, nil
}

// parsePythonResults turns mutmut's report into normalized mutants. The mutant's own name carries the
// module, which is how a finding names a file before its line is looked up.
func parsePythonResults(out string) []Mutation {
	var mutants []Mutation
	for _, line := range strings.Split(out, "\n") {
		match := pythonResult.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		status, known := pythonStatuses[strings.TrimSpace(match[2])]
		if !known {
			continue
		}
		mutants = append(mutants, Mutation{File: pythonModuleFile(match[1]), Operator: match[1], Status: status})
	}
	return mutants
}

// pythonModuleFile turns `pkg.mod.x_func__mutmut_3` into `pkg/mod.py`: the mutant names its module, and
// the module is the file the reader has to open.
func pythonModuleFile(name string) string {
	if index := strings.Index(name, "__mutmut_"); index > 0 {
		name = name[:index]
	}
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "/") + pythonExtension
}

// atoi reads the line number a diff hunk declares, or zero when it is not a number.
func atoi(text string) int {
	number, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return number
}

// pythonSurvivorLine asks mutmut where a mutant changes the source: `mutmut show` prints the diff, and
// the first hunk header carries the line. Zero means the line could not be read, which is reported as
// unknown rather than invented.
func pythonSurvivorLine(ctx context.Context, root, binary, name string, d Deps) int {
	diff, err := d.Run(ctx, root, binary, "show", name)
	if err != nil {
		return 0
	}
	number := 0
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		if match := pythonHunk.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			number, inHunk = atoi(match[1]), true
			continue
		}
		if !inHunk {
			continue
		}
		// The hunk starts at its first context line, so the changed line is the one the reader has to
		// open: a removed line is where the mutant replaced the source.
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			return number
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "-") {
			number++
		}
	}
	return 0
}
