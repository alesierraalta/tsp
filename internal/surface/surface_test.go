package surface

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree lays a fixture down and returns its directory: the probe reads real files, so the test
// provides real files instead of mocking a reader.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func probe(t *testing.T, dir string, paths ...string) (Report, error) {
	t.Helper()
	return Run(context.Background(), Options{Cwd: dir, Paths: paths})
}

// lineOf answers the 1-based line of a needle, so a fixture edit does not need a hand-counted number.
func lineOf(t *testing.T, body, needle string) int {
	t.Helper()
	index := strings.Index(body, needle)
	if index < 0 {
		t.Fatalf("%q is not in the fixture", needle)
	}
	return strings.Count(body[:index], "\n") + 1
}

func rowFor(t *testing.T, report Report, sink string) Row {
	t.Helper()
	for _, row := range report.Rows {
		if row.Sink == sink {
			return row
		}
	}
	t.Fatalf("no %q row in %+v", sink, report.Rows)
	return Row{}
}

func wants(t *testing.T, report Report, sinks ...string) {
	t.Helper()
	have := map[string]bool{}
	for _, row := range report.Rows {
		have[row.Sink] = true
	}
	for _, sink := range sinks {
		if !have[sink] {
			t.Errorf("no %q row: %+v", sink, report.Rows)
		}
	}
}

// The defect the probe exists for: an exported parameter joined into a path and opened, with nothing
// between the caller's string and the filesystem.
func TestAnUnguardedParameterReachingTheFilesystemIsARow(t *testing.T) {
	body := `package fixture

import (
	"os"
	"path/filepath"
)

func Load(name string) (*os.File, error) {
	p := filepath.Join("data", name)
	return os.Open(p)
}
`
	dir := writeTree(t, map[string]string{"load.go": body})
	report, err := probe(t, dir, "load.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if row.Input != "name string" || row.Origin != OriginPublic {
		t.Errorf("input = %q (%s), want the exported parameter named with its type", row.Input, row.Origin)
	}
	if row.Guard != "" || row.Guarded {
		t.Errorf("guard = %q guarded=%v, want none", row.Guard, row.Guarded)
	}
	if row.Function != "Load" || row.File != "load.go" || row.SinkLine != lineOf(t, body, "os.Open(p)") {
		t.Errorf("row located at %s:%d func %s, want load.go:%d func Load", row.File, row.SinkLine, row.Function, lineOf(t, body, "os.Open(p)"))
	}
	if !report.Blocking() || report.Unguarded() != 1 {
		t.Errorf("blocking=%v unguarded=%d, want one unguarded row", report.Blocking(), report.Unguarded())
	}
}

// The same flow with the check a reviewer would accept: the row stays in the report, guarded, because a
// promise that the input was checked is what the next skill step has to attack.
func TestAGuardBetweenTheInputAndTheSinkIsRecorded(t *testing.T) {
	body := `package fixture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func Read(name string) ([]byte, error) {
	p := filepath.Join("data", name)
	clean := filepath.Clean(p)
	if !strings.HasPrefix(clean, "data/") {
		return nil, errors.New("outside the data directory")
	}
	return os.ReadFile(clean)
}
`
	dir := writeTree(t, map[string]string{"read.go": body})
	report, err := probe(t, dir, "read.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if !row.Guarded || !strings.Contains(row.Guard, "HasPrefix") {
		t.Fatalf("guard = %q guarded=%v, want the prefix check recorded", row.Guard, row.Guarded)
	}
	if report.Blocking() {
		t.Errorf("a guarded inventory must not fail the probe: %+v", report.Rows)
	}
}

// A request value handed to a shell is the other defect: the origin column has to say http.
func TestARequestValueReachingExecIsARow(t *testing.T) {
	body := `package fixture

import (
	"net/http"
	"os/exec"
)

func handle(w http.ResponseWriter, r *http.Request) {
	cmd := r.URL.Query().Get("cmd")
	_, _ = exec.Command("sh", "-c", cmd).Output()
}
`
	dir := writeTree(t, map[string]string{"handle.go": body})
	report, err := probe(t, dir, "handle.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkExec)
	if row.Origin != OriginHTTP || !strings.Contains(row.Input, "Query") {
		t.Errorf("input = %q (%s), want the request value with origin http", row.Input, row.Origin)
	}
	if row.Guarded || row.SinkLine != lineOf(t, body, "exec.Command") {
		t.Errorf("row = %+v, want an unguarded row on the exec line", row)
	}
}

// An allocation sized by an input is a memory-exhaustion sink nobody calls dangerous.
func TestAnInputSizedAllocationIsARow(t *testing.T) {
	body := `package fixture

func Allocate(n int) []byte {
	return make([]byte, n)
}
`
	dir := writeTree(t, map[string]string{"alloc.go": body})
	report, err := probe(t, dir, "alloc.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkAllocation)
	if row.Input != "n int" || row.Origin != OriginPublic || row.Guarded {
		t.Errorf("row = %+v, want the input-sized make reported unguarded", row)
	}
}

// A loop bounded by an input runs as long as the caller decides.
func TestAnInputBoundedLoopIsARowAndACapGuardsIt(t *testing.T) {
	body := `package fixture

func Count(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		total++
	}
	return total
}

func Bounded(n int) int {
	if n > 1000 {
		return 0
	}
	total := 0
	for i := 0; i < n; i++ {
		total++
	}
	return n
}
`
	dir := writeTree(t, map[string]string{"loop.go": body})
	report, err := probe(t, dir, "loop.go")
	if err != nil {
		t.Fatal(err)
	}
	wants(t, report, SinkLoop)
	var count, bounded []Row
	for _, row := range report.Rows {
		switch row.Function {
		case "Count":
			count = append(count, row)
		case "Bounded":
			bounded = append(bounded, row)
		}
	}
	if len(count) == 0 || count[0].Guarded {
		t.Fatalf("Count rows = %+v, want an unguarded loop bound", count)
	}
	if len(bounded) == 0 || !bounded[0].Guarded || !strings.Contains(bounded[0].Guard, "n > 1000") {
		t.Fatalf("Bounded rows = %+v, want the cap recorded as the guard", bounded)
	}
}

// Money-like arithmetic fed by an input drifts without anyone noticing.
func TestAnInputAddedToAMoneyLikeTotalIsARow(t *testing.T) {
	body := `package fixture

func Charge(amount float64) float64 {
	var total float64
	total += amount
	return total
}
`
	dir := writeTree(t, map[string]string{"money.go": body})
	report, err := probe(t, dir, "money.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkAccumulation)
	if row.Input != "amount float64" || row.Guarded {
		t.Errorf("row = %+v, want the accumulating total reported", row)
	}
}

// A sink the source only talks about is not a sink: strings and comments are not data flow.
func TestASinkInsideAStringOrCommentIsNotARow(t *testing.T) {
	body := `package fixture

// os.Open(name) would be a sink if this comment were code.
func Quiet(name string) string {
	_ = "os.Open(name)"
	_ = "exec.Command(name)"
	// exec.Command(name)
	return name
}
`
	dir := writeTree(t, map[string]string{"quiet.go": body})
	report, err := probe(t, dir, "quiet.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 || report.Blocking() {
		t.Fatalf("rows = %+v, want none: a literal or a comment carries no flow", report.Rows)
	}
}

// An unexported function's parameter is internal: the caller is in the same package, which changes how
// much a guard matters.
func TestAnUnexportedFunctionsParameterSaysInternal(t *testing.T) {
	body := `package fixture

import "os"

func loadName(name string) (*os.File, error) {
	return os.Open(name)
}
`
	dir := writeTree(t, map[string]string{"internal.go": body})
	report, err := probe(t, dir, "internal.go")
	if err != nil {
		t.Fatal(err)
	}
	if row := rowFor(t, report, SinkFile); row.Origin != OriginInternal {
		t.Errorf("origin = %q, want internal", row.Origin)
	}
}

// A language without an extractor is named, not passed over as clean.
func TestALanguageWithoutAnExtractorIsNotMeasured(t *testing.T) {
	dir := writeTree(t, map[string]string{"tool.py": "def f(name):\n    return open(name)\n"})
	report, err := probe(t, dir, "tool.py")
	if !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable", err)
	}
	if joined := strings.Join(report.Skipped, "; "); !strings.Contains(joined, "python") {
		t.Errorf("skipped = %q, want python named", joined)
	}
}

// A directory in --paths mode means every source file under it, at any depth.
func TestAPathDirectoryInventoriesEverySourceFileUnderIt(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/one.go":    "package a\n\nimport \"os\"\n\nfunc One(p string) { os.Open(p) }\n",
		"a/b/two.go":  "package b\n\nimport \"os\"\n\nfunc Two(p string) { os.Open(p) }\n",
		"a/readme.md": "not source",
	})
	report, err := probe(t, dir, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("rows = %+v, want one per file", report.Rows)
	}
}

// Context arguments are accepted and unused: the shape mirrors the other probes so the CLI can grow a
// cancellation point without changing its callers.
func TestRunAcceptsAContext(t *testing.T) {
	dir := writeTree(t, map[string]string{"one.go": "package one\n\nfunc One() {}\n"})
	report, err := Run(context.Background(), Options{Cwd: dir, Paths: []string{"one.go"}})
	if err != nil || len(report.Rows) != 0 {
		t.Fatalf("err = %v rows = %+v, want a clean empty report", err, report.Rows)
	}
	if report.Heuristic == "" {
		t.Error("every report carries the heuristic sentence")
	}
}
