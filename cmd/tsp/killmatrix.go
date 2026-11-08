package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// The kill matrix is pr-review's instrument, not this repository's. The harness owns the scope, the
// exit code and the JSON it reads back; the engine stays where its owner maintains it, because a
// vendored copy would be a second source of truth for a file that runs shell commands.
const (
	killMatrixEnv         = "TSP_KILL_MATRIX"
	killMatrixPythonFloor = "3.12"
)

// killMatrixReport is the part of the engine's JSON this command acts on; the rest of it is the
// engine's business and is passed through untouched.
type killMatrixReport struct {
	Uncovered []string   `json:"uncovered"`
	Vacuous   []string   `json:"vacuous"`
	Redundant [][]string `json:"redundant"`
	NotRun    []string   `json:"not_run"`
	Complete  bool       `json:"complete"`
}

// killMatrixExit is the contract the caller reads: 0 every hunk killed, 1 uncovered hunks, 3 the
// matrix did not finish. A budget that stopped it is not a pass, whatever the uncovered count says.
func killMatrixExit(report killMatrixReport) int {
	if len(report.NotRun) > 0 || !report.Complete {
		return 3
	}
	if len(report.Uncovered) > 0 {
		return 1
	}
	return 0
}

// stringList collects a flag that may be repeated, because the engine takes one --src per file.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runKillmatrix(args []string) int {
	flags := flag.NewFlagSet("killmatrix", flag.ContinueOnError)
	base := flags.String("base", "", "the merge-base commit the change is measured from")
	testCmd := flags.String("test-cmd", "", "the pytest command that runs the tests (the engine appends its own junit flags)")
	timeout := flags.Int("timeout", 600, "seconds one mutant run may take")
	jobs := flags.Int("jobs", 1, "mutants run in parallel")
	budget := flags.Float64("budget", 0, "minutes; hunks not reached are reported as not run")
	jsonPath := flags.String("json", "", "write the full matrix to this file")
	instrument := flags.String("instrument", "", "kill-matrix.py to run (default: the installed pr-review)")
	python := flags.String("python", "", "the Python interpreter (default: python3)")
	var srcs, tests stringList
	flags.Var(&srcs, "src", "production file the change touched (repeatable)")
	flags.Var(&tests, "tests", "test file to measure (repeatable)")
	if code, ok := parseFlags(flags, args); !ok {
		return code
	}
	switch {
	case *base == "":
		return killMatrixUsage("--base is required")
	case *testCmd == "":
		return killMatrixUsage("--test-cmd is required")
	case len(srcs) == 0:
		return killMatrixUsage("--src is required at least once")
	}
	for _, src := range srcs {
		if strings.TrimSpace(src) == "" {
			// An unset shell variable expands to an empty string, which is not a path: the
			// engine would run over nothing and report it as a finished matrix.
			return killMatrixUsage("--src names nothing: give the file or glob this change touches")
		}
	}

	engine, err := locateKillMatrix(*instrument)
	if err != nil {
		fmt.Fprintln(os.Stderr, "killmatrix:", err)
		return 2
	}
	interpreter, err := killMatrixPython(*python)
	if err != nil {
		fmt.Fprintln(os.Stderr, "killmatrix:", err)
		return 2
	}

	work, err := os.MkdirTemp("", "tsp-killmatrix-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "killmatrix:", err)
		return 2
	}
	defer os.RemoveAll(work)
	reportPath := filepath.Join(work, "matrix.json")

	argv := []string{engine, "--base", *base, "--test-cmd", *testCmd, "--timeout", strconv.Itoa(*timeout), "--jobs", strconv.Itoa(*jobs), "--json", reportPath}
	for _, src := range srcs {
		argv = append(argv, "--src", src)
	}
	if len(tests) > 0 {
		argv = append(argv, "--tests")
		argv = append(argv, tests...)
	}
	budgetGiven := false
	flags.Visit(func(f *flag.Flag) {
		budgetGiven = budgetGiven || f.Name == "budget"
	})
	if budgetGiven {
		// Forwarded even when it is not positive: the engine treats a budget it cannot spend as
		// already spent, and dropping the cap here would turn that refusal into a full run.
		argv = append(argv, "--budget", strconv.FormatFloat(*budget, 'g', -1, 64))
	}

	// The engine's own report is the human matrix: stream it instead of hiding it behind a summary.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the instrument and its flags are the operator's own arguments; nothing here comes from untrusted input.
	run := exec.Command(interpreter, argv...)
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	if err := run.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "killmatrix: the engine did not finish: %v\n", err)
		return 2
	}

	encoded, err := os.ReadFile(reportPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "killmatrix: the engine reported success without writing a matrix")
		return 2
	}
	var report killMatrixReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		fmt.Fprintf(os.Stderr, "killmatrix: the matrix it wrote is not readable: %v\n", err)
		return 2
	}
	if *jsonPath != "" {
		if err := os.WriteFile(*jsonPath, encoded, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "killmatrix:", err)
			return 2
		}
	}
	fmt.Fprintf(os.Stderr, "killmatrix: complete=%t uncovered=%d vacuous=%d redundant=%d\n",
		report.Complete && len(report.NotRun) == 0, len(report.Uncovered), len(report.Vacuous), len(report.Redundant))
	return killMatrixExit(report)
}

func killMatrixUsage(problem string) int {
	fmt.Fprintf(os.Stderr, "killmatrix: %s\nusage: tsp killmatrix --base <ref> --src <path> --test-cmd <cmd> [--tests <path>...] [--timeout <sec>] [--jobs <n>] [--budget <min>] [--json <path>] [--instrument <path>] [--python <exe>]\n", problem)
	return 2
}

// locateKillMatrix finds the engine without guessing twice: an explicit path wins, then the
// environment, then the place pr-review installs it. Nothing is vendored here on purpose.
func locateKillMatrix(explicit string) (string, error) {
	candidate := explicit
	if candidate == "" {
		candidate = os.Getenv(killMatrixEnv)
	}
	if candidate == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot locate kill-matrix.py: %w", err)
		}
		candidate = filepath.Join(home, ".claude", "skills", "pr-review", "assets", "kill-matrix.py")
	}
	if info, err := os.Stat(candidate); err != nil || info.IsDir() {
		return "", fmt.Errorf("kill-matrix.py is not at %s: pass --instrument <path>, set %s, or install pr-review", candidate, killMatrixEnv)
	}
	return candidate, nil
}

// killMatrixPython checks the floor before the engine starts, so a Python too old to run it is a
// diagnosis instead of a traceback.
func killMatrixPython(explicit string) (string, error) {
	interpreter := explicit
	if interpreter == "" {
		interpreter = "python3"
	}
	path, err := exec.LookPath(interpreter)
	if err != nil {
		return "", fmt.Errorf("%s not found: pass --python <exe>", interpreter)
	}
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- a constant version probe against the interpreter the operator named.
	probe := exec.Command(path, "-c", "import sys; print('%d.%d' % sys.version_info[:2]); sys.exit(0 if sys.version_info >= (3, 12) else 1)")
	probe.WaitDelay = waitDelayAfterKill
	out, err := probe.Output()
	version := strings.TrimSpace(string(out))
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return "", fmt.Errorf("kill-matrix.py needs Python %s or newer; %s is %s", killMatrixPythonFloor, path, version)
	case err != nil:
		return "", fmt.Errorf("cannot run %s: %w", path, err)
	}
	return path, nil
}
