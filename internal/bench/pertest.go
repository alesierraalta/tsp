package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/hookcmd"
)

// perTestCommand selects a runner's per-test reporter, preserving the user's arguments.
// Other runners keep their command and are read as one whole-suite result.
// The suite is one field stored as one string, so it is split by the shared rule the hook command uses; a
// suite the splitter cannot read is reported instead of rewritten, for the same reason the whole-suite path
// refuses it: the words read so far are the head of a command nobody wrote.
func perTestCommand(suite string) (argv []string, parser func(string) map[string]bool, err error) {
	fields, err := hookcmd.ShellWords(suite)
	if err != nil {
		return nil, nil, err
	}
	if len(fields) == 0 {
		return nil, nil, nil
	}
	runner := fields
	if fields[0] == "npx" && len(fields) > 1 {
		runner = fields[1:]
	}
	if runner[0] == "jest" {
		return append(fields, "--json"), parseJSJSON, nil
	}
	if runner[0] == "vitest" && len(runner) > 1 && runner[1] == "run" {
		return append(fields, "--reporter=json"), parseJSJSON, nil
	}
	if fields[0] == "cargo" && len(fields) > 1 && fields[1] == "test" {
		return fields, parseCargo, nil
	}
	switch {
	case fields[0] == "node" && len(fields) > 1 && fields[1] == "--test":
		return append(fields[:2:2], append([]string{"--test-reporter=tap"}, fields[2:]...)...), parseTAP, nil
	case fields[0] == "go" && len(fields) > 1 && fields[1] == "test":
		return append(fields[:2:2], append([]string{"-json"}, fields[2:]...)...), parseGoJSON, nil
	case fields[0] == "pytest":
		return append(fields[:1:1], append(pytestFlags(), fields[1:]...)...), parsePytest, nil
	case strings.HasPrefix(fields[0], "python") && len(fields) > 2 && fields[1] == "-m" && fields[2] == "pytest":
		return append(fields[:3:3], append(pytestFlags(), fields[3:]...)...), parsePytest, nil
	}
	return fields, nil, nil
}

// runCapture runs argv in dir and returns the exit code with the complete combined output.
func runCapture(dir string, argv []string, timeout time.Duration) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	argv = suiteArgv(dir, argv)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	boundProcessTree(cmd)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if ee := (&exec.ExitError{}); errors.As(err, &ee) {
			code = ee.ExitCode()
		}
	}
	return code, buf.String()
}

// goValueFlags are the go test and build flags whose value may be the next word, so that word is
// not a package ("-coverpkg ./..." would otherwise read as a pattern covering every directory).
var goValueFlags = map[string]bool{
	"-bench": true, "-benchtime": true, "-blockprofile": true, "-blockprofilerate": true, "-count": true,
	"-covermode": true, "-coverpkg": true, "-coverprofile": true, "-cpu": true, "-cpuprofile": true,
	"-fuzz": true, "-fuzzminimizetime": true, "-fuzztime": true, "-list": true, "-memprofile": true,
	"-memprofilerate": true, "-mutexprofile": true, "-mutexprofilefraction": true, "-outputdir": true,
	"-parallel": true, "-run": true, "-shuffle": true, "-skip": true, "-timeout": true, "-trace": true,
	"-vet": true, "-C": true, "-asmflags": true, "-compiler": true, "-exec": true, "-gccgoflags": true,
	"-gcflags": true, "-installsuffix": true, "-ldflags": true, "-mod": true, "-modfile": true, "-o": true,
	"-overlay": true, "-p": true, "-pgo": true, "-pkgdir": true, "-tags": true, "-toolexec": true,
	"-buildmode": true,
}

// goFlagName reduces a go test flag word to its canonical spelling: one dash, without the "test."
// prefix the test binary's flags also accept, so "--test.run" looks up as "-run".
func goFlagName(word string) string {
	return "-" + strings.TrimPrefix(strings.TrimLeft(word, "-"), "test.")
}

// withTestPackages adds to a go test argv the directory of every Go test file its package patterns do
// not already cover, so a test placed outside a scoped suite runs instead of being scored without ever
// executing. The packages go right after the suite's own: go test reads a package named after a flag
// as an argument for the test binary and silently skips it.
func withTestPackages(argv, testFiles []string) []string {
	var patterns []string
	at := 2
	for i := 2; i < len(argv); i++ {
		if argv[i] == "-args" || argv[i] == "--args" {
			break // everything after -args belongs to the test binary
		}
		if strings.HasPrefix(argv[i], "-") {
			if goValueFlags[goFlagName(argv[i])] {
				i++
			}
			continue
		}
		patterns = append(patterns, argv[i])
		at = i + 1
	}
	implicit := len(patterns) == 0
	if implicit {
		patterns = []string{"."}
	}
	var extra []string
	for _, f := range testFiles {
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}
		dir := path.Dir(filepath.ToSlash(f))
		if goPatternsCover(append(patterns, extra...), dir) {
			continue
		}
		if dir == "." {
			extra = append(extra, ".")
		} else {
			extra = append(extra, "./"+dir)
		}
	}
	if len(extra) == 0 {
		return argv
	}
	if implicit {
		extra = append([]string{"."}, extra...)
	}
	out := append(append([]string{}, argv[:at]...), extra...)
	return append(out, argv[at:]...)
}

// goPatternsCover reports whether relative package patterns (., ./x, ./x/, ./x/..., ./...) include dir.
func goPatternsCover(patterns []string, dir string) bool {
	for _, p := range patterns {
		p = strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/")
		if p == "" {
			p = "."
		}
		switch base, recursive := strings.CutSuffix(p, "/..."); {
		case p == "...":
			return true
		case recursive && (dir == base || strings.HasPrefix(dir, base+"/")):
			return true
		case !recursive && p == dir:
			return true
		}
	}
	return false
}

// withPythonTestFiles adds agent files not already selected by a pytest path.
func withPythonTestFiles(dir string, argv, testFiles []string, start int) []string {
	var paths []string
	for i := start; i < len(argv); i++ {
		switch argv[i] {
		case "-p", "-k", "-m", "-c", "-o", "--override-ini", "--rootdir", "--confcutdir", "--ignore", "--ignore-glob", "--deselect", "--maxfail", "--basetemp", "--junitxml", "--junit-xml", "--import-mode", "--tb", "--capture", "-r", "--verbosity", "--log-level", "--color", "--durations":
			i++
			continue
		}
		if strings.HasPrefix(argv[i], "-") {
			continue
		}
		// A plugin flag's value (`-W error`, `--timeout 30`) is a word the list above cannot know; only a word
		// that names something in the tree, or a node id in a file of it, is a path the suite selects.
		p := path.Clean(filepath.ToSlash(argv[i]))
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(strings.SplitN(p, "::", 2)[0]))); err != nil {
			continue
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		// A suite that names no path collects what its configuration (testpaths) says; any explicit path
		// would replace that collection, so the suite runs exactly as written.
		return argv
	}
	out := append([]string{}, argv...)
	for _, f := range testFiles {
		f = path.Clean(filepath.ToSlash(f))
		if !strings.HasSuffix(f, ".py") || !isTestFile(f) {
			continue
		}
		covered := false
		for _, p := range paths {
			if p == f {
				covered = true
				break
			}
			st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p)))
			if err == nil && st.IsDir() && (p == "." || strings.HasPrefix(f, p+"/")) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, f)
			paths = append(paths, f)
		}
	}
	return out
}

// SupportsPerTest reports whether the shared runner can read individual outcomes.
func SupportsPerTest(suite string) bool {
	_, parser, err := perTestCommand(suite)
	return err == nil && parser != nil
}

// PerTest runs the same per-test runner used by RedCheck.
func PerTest(dir, suite string, timeout time.Duration) (map[string]bool, int, string, error) {
	return perTest(dir, suite, timeout)
}

// perTest runs the suite and returns each test's outcome by name. A runner without a per-test
// format yields the single entry "suite". A suite the splitter cannot read is refused before it runs.
// Go and pytest suites also run agent tests outside their scoped paths.
func perTest(dir, suite string, timeout time.Duration, testFiles ...string) (map[string]bool, int, string, error) {
	argv, parser, err := perTestCommand(suite)
	if err != nil {
		return nil, -1, "", err
	}
	if argv == nil {
		return nil, -1, "", nil
	}
	if argv[0] == "go" && len(argv) > 1 && argv[1] == "test" {
		argv = withTestPackages(argv, testFiles)
	} else if argv[0] == "pytest" {
		argv = withPythonTestFiles(dir, argv, testFiles, 1)
	} else if strings.HasPrefix(argv[0], "python") && len(argv) > 2 && argv[1] == "-m" && argv[2] == "pytest" {
		argv = withPythonTestFiles(dir, argv, testFiles, 3)
	}
	code, out := runCapture(dir, argv, timeout)
	if parser == nil {
		return map[string]bool{"suite": code == 0}, code, out, nil
	}
	tests := parser(out)
	if len(tests) == 0 {
		// A build failure or an empty run: the whole suite stands as one result.
		tests = map[string]bool{"suite": code == 0}
	}
	return tests, code, out, nil
}

// Jest and Vitest share assertionResults. Combined output can include diagnostics.
func parseJSJSON(out string) map[string]bool {
	res := map[string]bool{}
	for i := 0; i < len(out); i++ {
		if out[i] != '{' {
			continue
		}
		var report struct {
			TestResults []struct {
				AssertionResults []struct{ FullName, Status string }
			}
		}
		if json.NewDecoder(strings.NewReader(out[i:])).Decode(&report) != nil || report.TestResults == nil {
			continue
		}
		for _, file := range report.TestResults {
			for _, test := range file.AssertionResults {
				if test.FullName != "" && (test.Status == "passed" || test.Status == "failed") {
					res[test.FullName] = test.Status == "passed"
				}
			}
		}
		return res
	}
	return res
}

var cargoLine = regexp.MustCompile(`^test (\S+) \.\.\. (ok|FAILED|ignored)$`)

func parseCargo(out string) map[string]bool {
	res := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if m := cargoLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil && m[2] != "ignored" {
			res[m[1]] = m[2] == "ok"
		}
	}
	return res
}

var tapLine = regexp.MustCompile(`^( *)(ok|not ok) \d+ - (.*?)(?: # .*)?$`)

// parseTAP reads node's TAP output; nested subtests are named parent/child by indentation.
func parseTAP(out string) map[string]bool {
	res := map[string]bool{}
	type frame struct {
		indent int
		name   string
	}
	var pending []frame // subtests seen deeper than the line being read, until their parent line closes them
	for _, line := range strings.Split(out, "\n") {
		m := tapLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, ok, name := len(m[1]), m[2] == "ok", strings.TrimSpace(m[3])
		// Children are printed before their parent: name them once the parent line arrives.
		var children []frame
		for len(pending) > 0 && pending[len(pending)-1].indent > indent {
			children = append(children, pending[len(pending)-1])
			pending = pending[:len(pending)-1]
		}
		for _, c := range children {
			passed := res[c.name]
			delete(res, c.name)
			res[name+"/"+c.name] = passed
		}
		res[name] = ok
		pending = append(pending, frame{indent, name})
	}
	return res
}

// pytestFlags ask pytest for one summary line per test (-rA) and keep its cache out of the tree it runs in.
func pytestFlags() []string { return []string{"-rA", "-p", "no:cacheprovider"} }

var pytestLine = regexp.MustCompile(`^(PASSED|FAILED) (\S+::\S+)`)

// parsePytest reads the -rA short summary; a test is named path::test. ERROR lines are left out: a setup or
// collection error is not a test outcome, and a failed build must never read as a red witness.
func parsePytest(out string) map[string]bool {
	res := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if m := pytestLine.FindStringSubmatch(line); m != nil {
			res[m[2]] = m[1] == "PASSED"
		}
	}
	return res
}

// parseGoJSON reads go test -json events; a test is named package.Test.
func parseGoJSON(out string) map[string]bool {
	res := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		var ev struct {
			Action, Package, Test string
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass":
			res[ev.Package+"."+ev.Test] = true
		case "fail":
			res[ev.Package+"."+ev.Test] = false
		}
	}
	return res
}
