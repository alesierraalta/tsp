package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// surfaceRepo is a committed repository whose base holds no source: each test writes the change it wants
// judged, so the diff is exactly the fixture.
func surfaceRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	verifyGit(t, dir, "init", "-q")
	verifyGit(t, dir, "config", "user.email", "test@example.com")
	verifyGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verifyGit(t, dir, "add", ".")
	verifyGit(t, dir, "commit", "-qm", "base")
	return dir
}

// captureStreams runs the command in-process and returns what a user would have seen on both streams.
func captureStreams(t *testing.T, run func() int) (int, string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	code := run()
	os.Stdout, os.Stderr = oldOut, oldErr
	if err := outW.Close(); err != nil {
		t.Fatal(err)
	}
	if err := errW.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	errOut, err := io.ReadAll(errR)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(out), string(errOut)
}

const surfaceUnguarded = `package app

import (
	"os"
	"path/filepath"
)

func Load(name string) (*os.File, error) {
	p := filepath.Join("data", name)
	return os.Open(p)
}
`

const surfaceGuarded = `package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func Load(name string) (*os.File, error) {
	p := filepath.Join("data", name)
	clean := filepath.Clean(p)
	if !strings.HasPrefix(clean, "data/") {
		return nil, errors.New("outside the data directory")
	}
	return os.Open(clean)
}
`

// The command's whole contract in one test: an unguarded row exits 1, prints the counts, the heuristic the
// report is honest about, the function group and the empty verdict the model has to fill.
func TestProbeSurfaceReportsAnUnguardedRowOnARealChange(t *testing.T) {
	dir := surfaceRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(surfaceUnguarded), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir, "--base", "HEAD"})
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for an unguarded input reaching the filesystem: %s", code, stderr)
	}
	for _, want := range []string{
		"1 row(s) in 1 function(s), 1 unguarded",
		"heuristic: data flow is intra-function; a guard is a check that mentions the input, not proof it is correct",
		"app.go:8 func Load(...)",
		"name string (public) -> file @ app.go:10  guard: none  verdict: ___",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing on a measured change", stderr)
	}
}

// A guarded inventory stays a report, not a failure: exit 0 and the guard text, because the next skill
// step has to attack the promise instead of trusting it.
func TestProbeSurfaceExitsZeroWhenTheOnlyRowIsGuarded(t *testing.T) {
	dir := surfaceRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(surfaceGuarded), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir, "--base", "HEAD"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 when every row is guarded:\n%s", code, out)
	}
	if !strings.Contains(out, "guard: !strings.HasPrefix(clean, \"data/\")") || !strings.Contains(out, "0 unguarded") {
		t.Fatalf("output = %q, want the prefix check recorded as the guard", out)
	}
}

// Unguarded rows come first: the reader fixes what is broken before reading what is promised.
func TestProbeSurfacePrintsUnguardedRowsFirst(t *testing.T) {
	dir := surfaceRepo(t)
	body := surfaceGuarded + `
func Peek(name string) (*os.File, error) {
	return os.Open(name)
}
`
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir, "--base", "HEAD"})
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	unguarded := strings.Index(out, "guard: none")
	guarded := strings.Index(out, "guard: !strings.HasPrefix")
	if unguarded < 0 || guarded < 0 || unguarded > guarded {
		t.Fatalf("unguarded row at %d, guarded at %d: unguarded must come first\n%s", unguarded, guarded, out)
	}
}

// A change the probe has no extractor for exits 2 and says so: silence there would read as clean.
func TestProbeSurfaceRefusesAChangeItCannotMeasure(t *testing.T) {
	dir := surfaceRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "tool.py"), []byte("def load(name):\n    return open(name)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir})
	})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 when nothing in the change is measured: %s", code, stderr)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
	if !strings.Contains(stderr, "nothing in this change was measured") || !strings.Contains(stderr, "python") {
		t.Errorf("stderr = %q, want the unmeasured language named", stderr)
	}
}

// --paths inventories files and directories instead of the change, and --json carries the same rows.
func TestProbeSurfacePathsModeAndJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "load.go"), []byte(surfaceUnguarded), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir, "--paths", "load.go", "--json"})
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1: %s", code, stderr)
	}
	var report struct {
		Measured  []string `json:"measured"`
		Heuristic string   `json:"heuristic"`
		Rows      []struct {
			File         string `json:"file"`
			Function     string `json:"function"`
			FunctionLine int    `json:"function_line"`
			Input        string `json:"input"`
			Origin       string `json:"origin"`
			Sink         string `json:"sink"`
			SinkCall     string `json:"sink_call"`
			SinkLine     int    `json:"sink_line"`
			Guard        string `json:"guard"`
			Guarded      bool   `json:"guarded"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json: %v: %q", err, out)
	}
	if len(report.Rows) != 1 || report.Rows[0].Sink != "file" || report.Rows[0].SinkCall != "os.Open" || report.Rows[0].Guarded {
		t.Fatalf("rows = %+v, want the unguarded os.Open row", report.Rows)
	}
	if report.Rows[0].Input != "name string" || report.Rows[0].Origin != "public" || report.Rows[0].Function != "Load" || report.Heuristic == "" {
		t.Fatalf("row = %+v heuristic %q", report.Rows[0], report.Heuristic)
	}
	clean := t.TempDir()
	if err := os.WriteFile(filepath.Join(clean, "quiet.go"), []byte("package quiet\n\nfunc Quiet() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", clean, "--paths", "quiet.go"})
	})
	if code != 0 || !strings.Contains(out, "0 row(s)") {
		t.Fatalf("clean tree: exit %d, output %q; want 0 and the zero counts", code, out)
	}
}

// A path the user named that does not exist is a usage answer, not a clean report.
func TestProbeSurfaceRefusesAMissingPath(t *testing.T) {
	dir := t.TempDir()
	code, out, stderr := captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir, "--paths", "missing.go"})
	})
	if code != 2 || out != "" || stderr == "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want 2 with the reason on stderr", code, out, stderr)
	}
}

// The probe reads; it never writes what it read.
func TestProbeSurfaceDoesNotTouchTheTree(t *testing.T) {
	dir := surfaceRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(surfaceUnguarded), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	captureStreams(t, func() int {
		return runProbeSurface([]string{"--cwd", dir, "--base", "HEAD"})
	})
	after, err := os.Stat(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatal("the probe rewrote the file it read")
	}
}
