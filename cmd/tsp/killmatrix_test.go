package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKillMatrixExitIsTheContract(t *testing.T) {
	cases := []struct {
		name   string
		report killMatrixReport
		want   int
	}{
		{"every hunk killed", killMatrixReport{Complete: true}, 0},
		{"an uncovered hunk", killMatrixReport{Complete: true, Uncovered: []string{"internal/x.go:12"}}, 1},
		{"a budget that stopped the matrix", killMatrixReport{Complete: true, NotRun: []string{"internal/x.go:12"}}, 3},
		{"a run that never completed", killMatrixReport{Complete: false}, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := killMatrixExit(c.report); got != c.want {
				t.Fatalf("killMatrixExit = %d, want %d", got, c.want)
			}
		})
	}
}

// fakePython answers the version probe and writes the matrix the engine would have written, so the
// mapping from its JSON to the exit code is proven without running mutants.
func fakePython(t *testing.T, version, matrix string) string {
	t.Helper()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-c\" ]; then echo " + version + "; exit " + map[bool]string{true: "0", false: "1"}[version == "3.12"] + "; fi\n" +
		"prev=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--json\" ]; then printf '%s' '" + matrix + "' > \"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"exit 0\n"
	path := filepath.Join(t.TempDir(), "fake-python")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python: %v", err)
	}
	return path
}

func captureStderr(t *testing.T, run func() int) (int, string) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("temp stderr: %v", err)
	}
	saved := os.Stderr
	os.Stderr = file
	code := run()
	os.Stderr = saved
	if err := file.Close(); err != nil {
		t.Fatalf("close stderr: %v", err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return code, string(data)
}

func TestKillmatrixMapsTheEngineMatrixToItsExitCode(t *testing.T) {
	python := fakePython(t, "3.12", `{"uncovered":["internal/x.go:12"],"complete":true}`)
	instrument := filepath.Join(t.TempDir(), "kill-matrix.py")
	if err := os.WriteFile(instrument, []byte(""), 0o644); err != nil {
		t.Fatalf("write instrument: %v", err)
	}
	matrixOut := filepath.Join(t.TempDir(), "matrix.json")

	code, stderr := captureStderr(t, func() int {
		return runKillmatrix([]string{"--base", "HEAD~1", "--src", "internal/x.go", "--test-cmd", "go test ./...", "--instrument", instrument, "--python", python, "--json", matrixOut})
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (an uncovered hunk); stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "uncovered=1") {
		t.Fatalf("stderr = %q, want the summary naming the uncovered hunk", stderr)
	}
	written, err := os.ReadFile(matrixOut)
	if err != nil || !strings.Contains(string(written), "internal/x.go:12") {
		t.Fatalf("matrix not copied to --json: %v %q", err, written)
	}
}

func TestKillmatrixRefusesBeforeItStarts(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no base", []string{"--src", "a.go", "--test-cmd", "go test ./..."}, "--base is required"},
		{"no test command", []string{"--base", "HEAD~1", "--src", "a.go"}, "--test-cmd is required"},
		{"no source", []string{"--base", "HEAD~1", "--test-cmd", "go test ./..."}, "--src is required"},
		{"no engine", []string{"--base", "HEAD~1", "--src", "a.go", "--test-cmd", "go test ./...", "--instrument", "/nonexistent/kill-matrix.py"}, "install pr-review"},
		{"python too old", []string{"--base", "HEAD~1", "--src", "a.go", "--test-cmd", "go test ./...", "--instrument", "/dev/null", "--python", "PLACEHOLDER"}, "Python 3.12 or newer"},
	}
	old := fakePython(t, "3.11", "{}")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := c.args
			for i, arg := range args {
				if arg == "PLACEHOLDER" {
					args[i] = old
				}
			}
			code, stderr := captureStderr(t, func() int { return runKillmatrix(args) })
			if code != 2 {
				t.Fatalf("exit = %d, want 2 (cannot run); stderr: %s", code, stderr)
			}
			if !strings.Contains(stderr, c.want) {
				t.Fatalf("stderr = %q, want %q", stderr, c.want)
			}
		})
	}
}

// An empty --src is the mistake --base and --test-cmd already refuse: a required flag whose value
// names nothing. Today the required check counts it, so the matrix runs over no file and the exit
// code says the change is covered — an unset shell variable reads as a pass.
func TestKillmatrixRefusesASourceThatNamesNothing(t *testing.T) {
	python := fakePython(t, "3.12", `{"complete":true,"uncovered":[]}`)
	instrument := filepath.Join(t.TempDir(), "kill-matrix.py")
	if err := os.WriteFile(instrument, []byte(""), 0o644); err != nil {
		t.Fatalf("write instrument: %v", err)
	}
	code, stderr := captureStderr(t, func() int {
		return runKillmatrix([]string{"--base", "HEAD~1", "--src", "", "--test-cmd", "go test ./...", "--instrument", instrument, "--python", python})
	})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (cannot run: --src names nothing); stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--src") {
		t.Fatalf("stderr = %q, want the refusal to name --src", stderr)
	}
}

// A budget of -5 minutes is not a duration. The engine treats a negative budget as already spent and
// reports the matrix incomplete; this command drops it instead, so a run the caller meant to cap runs
// unbudgeted and reports a complete matrix. Either refusal or the engine's own "not run" is correct;
// reporting a finished matrix is not.
func TestKillmatrixNeverTurnsABadBudgetIntoNoBudget(t *testing.T) {
	// The engine treats a budget it cannot spend as already spent (kill-matrix.py:
	// `if a.budget and elapsed > budget*60`), so this stub answers that way: a budget that
	// reaches it unspendable leaves hunks not run, while a dropped budget looks complete.
	python := filepath.Join(t.TempDir(), "python")
	script := `#!/bin/sh
if [ "$1" = "-c" ]; then printf '3.12\n'; exit 0; fi
budget=""
while [ $# -gt 0 ]; do
	case "$1" in
	--budget) budget="$2"; shift 2 ;;
	--json) out="$2"; shift 2 ;;
	*) shift ;;
	esac
done
if [ -n "$budget" ] && [ "${budget#-}" != "$budget" ]; then
	printf '{"complete":false,"not_run":["hunk"],"uncovered":[]}' > "$out"
else
	printf '{"complete":true,"uncovered":[]}' > "$out"
fi
exit 0
`
	if err := os.WriteFile(python, []byte(script), 0o755); err != nil {
		t.Fatalf("write python stub: %v", err)
	}
	instrument := filepath.Join(t.TempDir(), "kill-matrix.py")
	if err := os.WriteFile(instrument, []byte(""), 0o644); err != nil {
		t.Fatalf("write instrument: %v", err)
	}
	code, stderr := captureStderr(t, func() int {
		return runKillmatrix([]string{"--base", "HEAD~1", "--src", "internal/x.go", "--test-cmd", "go test ./...", "--budget", "-5", "--instrument", instrument, "--python", python})
	})
	if code != 3 {
		t.Fatalf("exit = %d: a negative budget was silently dropped and the matrix reported complete; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "complete=false") {
		t.Fatalf("stderr = %q, want the engine's incomplete matrix reported", stderr)
	}
}

// The help is the only place an operator learns what the instrument can read: without it a Go change
// fails with "the engine did not finish" and no hint that the engine appends pytest's own junit flags.
func TestKillmatrixHelpNamesTheInstrumentLanguage(t *testing.T) {
	code, stderr := captureStderr(t, func() int { return runKillmatrix([]string{"--help"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for --help; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "pytest") {
		t.Fatalf("help = %q, want it to name pytest as the language the instrument reads", stderr)
	}
}
