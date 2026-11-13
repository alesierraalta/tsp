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
	dir := writeTree(t, map[string]string{"tool.rb": "def f(name)\n  open(name)\nend\n"})
	report, err := probe(t, dir, "tool.rb")
	if !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable", err)
	}
	if joined := strings.Join(report.Skipped, "; "); !strings.Contains(joined, "ruby") {
		t.Errorf("skipped = %q, want ruby named", joined)
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

// An allocation sized by the length of a collection the caller handed in is proportional: the memory it takes
// is bounded by bytes that are already in memory, so nothing amplifies. A row here is a probe a model spends
// on a promise nobody can break.
func TestAnAllocationSizedByTheLengthOfAnInputCollectionIsNotARow(t *testing.T) {
	body := `package fixture

func Copy(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func Keys(m map[string]int) []string {
	keys := make([]string, len(m))
	i := 0
	for key := range m {
		keys[i] = key
		i++
	}
	return keys
}
`
	dir := writeTree(t, map[string]string{"copy.go": body})
	report, err := probe(t, dir, "copy.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 || report.Blocking() {
		t.Fatalf("rows = %+v, want none: len() of a collection already in memory is not amplification", report.Rows)
	}
}

// The other side of that line: a scalar size, or arithmetic on one, is the caller's number and multiplying it
// is how a small request becomes a large allocation.
func TestAnAllocationSizedByAScalarIsStillARow(t *testing.T) {
	body := `package fixture

func Grow(n int) []byte {
	return make([]byte, n*4)
}

func Pad(n int) []byte {
	return make([]byte, n+1)
}
`
	dir := writeTree(t, map[string]string{"grow.go": body})
	report, err := probe(t, dir, "grow.go")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range report.Rows {
		if row.Sink == SinkAllocation {
			count++
			if row.Guarded {
				t.Errorf("row = %+v, want the scalar size unguarded", row)
			}
		}
	}
	if count != 2 {
		t.Fatalf("allocation rows = %d (%+v), want one per scalar-sized make", count, report.Rows)
	}
}

// A loop that walks one element per item of a collection the caller already passed is proportional; the caller
// cannot make it run longer than the data it handed over.
func TestALoopBoundedByTheLengthOfAnInputCollectionIsNotARow(t *testing.T) {
	body := `package fixture

import "strings"

func Join(lines []string) string {
	var b strings.Builder
	for i := 0; i < len(lines); i++ {
		b.WriteString(lines[i])
	}
	return b.String()
}
`
	dir := writeTree(t, map[string]string{"join.go": body})
	report, err := probe(t, dir, "join.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("rows = %+v, want none: a len()-bounded loop is not an input-bounded loop", report.Rows)
	}
}

// The check that reads the status of one operation is not a check on the value another operation received.
// Reporting it as a guard hides a promise nobody kept, which is the worst of both answers.
func TestAnErrorFromAnUnrelatedCallIsNotAGuard(t *testing.T) {
	body := `package fixture

import "os"

type Options struct {
	Name string
}

func Load(o Options) (*os.File, error) {
	_, err := os.Stat(o.Name)
	if err != nil {
		return nil, err
	}
	return os.Open(o.Name)
}
`
	dir := writeTree(t, map[string]string{"load.go": body})
	report, err := probe(t, dir, "load.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) == 0 {
		t.Fatal("no rows, want both the Stat and the Open reported")
	}
	for _, row := range report.Rows {
		if row.Guarded {
			t.Errorf("row = %+v, want unguarded: err comes from the Stat, not from a check on Name", row)
		}
	}
}

// The same status check, when the call that produced it parses the input, is the guard: the error is the
// verdict on the caller's string, not on something else that happened to receive it.
func TestAnErrorFromAParseOfTheInputIsAGuard(t *testing.T) {
	body := `package fixture

import (
	"net/url"
	"os"
)

func Fetch(name string) error {
	u, err := url.Parse(name)
	if err != nil {
		return err
	}
	f, err := os.Open(u.Path)
	if err != nil {
		return err
	}
	return f.Close()
}
`
	dir := writeTree(t, map[string]string{"fetch.go": body})
	report, err := probe(t, dir, "fetch.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if !row.Guarded || !strings.Contains(row.Guard, "err != nil") {
		t.Fatalf("row = %+v, want the parse's error check recorded as the guard", row)
	}
}

// A context and an error are runtime handles, not data: the caller's own deadline and verdict cannot be made
// hostile, so a row that names one asks a model to attack nothing.
func TestAContextOrAnErrorParameterIsNotAnInput(t *testing.T) {
	body := `package fixture

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

func Run(ctx context.Context, command string) error {
	return exec.CommandContext(ctx, "sh", "-c", command).Run()
}

func Log(w io.Writer, err error) {
	fmt.Fprintf(w, "failed: %v", err)
}
`
	dir := writeTree(t, map[string]string{"run.go": body})
	report, err := probe(t, dir, "run.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("rows = %+v, want only the command reaching exec", report.Rows)
	}
	row := report.Rows[0]
	if row.Sink != SinkExec || row.Input != "command string" {
		t.Fatalf("row = %+v, want the command as the only input", row)
	}
}

// A report written into a buffer the function declared is a value, not a stream the caller chose: whatever the
// buffer reaches is reported where it lands. The same call into a writer the caller passed stays a row.
func TestAWriteIntoALocalBufferIsNotATemplateSink(t *testing.T) {
	body := `package fixture

import (
	"fmt"
	"io"
	"strings"
)

func Script(name string) string {
	var s strings.Builder
	fmt.Fprintf(&s, "echo %s", name)
	return s.String()
}

func Render(w io.Writer, name string) {
	fmt.Fprintf(w, "hello %s", name)
}
`
	dir := writeTree(t, map[string]string{"write.go": body})
	report, err := probe(t, dir, "write.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("rows = %+v, want only the write into the caller's writer", report.Rows)
	}
	if row := report.Rows[0]; row.Sink != SinkTemplate || row.Function != "Render" {
		t.Fatalf("row = %+v, want the template row from Render", row)
	}
}

// A scan that stops when the length the function computed runs out cannot outlast the value: `end := len(v)` is
// the same bound as len(v) itself, written once.
func TestALoopBoundedByAComputedLengthIsNotARow(t *testing.T) {
	body := `package fixture

import "strings"

func Trim(value string) string {
	end := len(value)
	for end > 0 && strings.ContainsRune(".,!?", rune(value[end-1])) {
		end--
	}
	return value[:end]
}
`
	dir := writeTree(t, map[string]string{"trim.go": body})
	report, err := probe(t, dir, "trim.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("rows = %+v, want none: the scan is bounded by len(value)", report.Rows)
	}
}

// A scan clipped by the collection it walks is proportional even when a scalar opens it, while the same loop
// without the clip runs as long as the caller says and stays a row.
func TestAScanClippedByACollectionIsNotARowButAScalarBoundIs(t *testing.T) {
	body := `package fixture

func Clipped(lines []string, start, end int) int {
	n := 0
	for i := start; i < end && i < len(lines); i++ {
		n++
	}
	return n
}

func Scalar(lines []string, start, end int) int {
	n := 0
	for i := start; i < end; i++ {
		n++
	}
	return n
}
`
	dir := writeTree(t, map[string]string{"scan.go": body})
	report, err := probe(t, dir, "scan.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) == 0 {
		t.Fatal("no rows, want the scalar-bounded loop reported")
	}
	for _, row := range report.Rows {
		if row.Function != "Scalar" {
			t.Errorf("row = %+v, want only the loop the caller's number bounds", row)
		}
	}
}

// A name this function uses for a call's status keeps that meaning wherever it is assigned from a call, so
// `if err := collect(path); err != nil` is not a guard for the path either.
func TestAStatusNameAssignedFromASingleCallIsStillNotAGuard(t *testing.T) {
	body := `package fixture

import "os"

type Options struct {
	Name string
}

func Load(o Options) (*os.File, error) {
	_, err := os.Stat(o.Name)
	if err != nil {
		return nil, err
	}
	if err := touch(o.Name); err != nil {
		return nil, err
	}
	return os.Open(o.Name)
}

func touch(name string) error { return nil }
`
	dir := writeTree(t, map[string]string{"load.go": body})
	report, err := probe(t, dir, "load.go")
	if err != nil {
		t.Fatal(err)
	}
	open := lineOf(t, body, "os.Open(o.Name)")
	for _, row := range report.Rows {
		if row.Sink == SinkFile && row.SinkLine == open {
			if row.Guarded {
				t.Fatalf("row = %+v, want the Open unguarded: err holds a call's status, not the input", row)
			}
			return
		}
	}
	t.Fatalf("no file row on the Open line: %+v", report.Rows)
}

// A name a call assigned and the function then compares to nil holds that call's status: it has no value of
// its own to guard with, so the check is about the call, not about the input it was handed.
func TestANilCheckedCallResultIsAStatusNotAGuard(t *testing.T) {
	body := `package fixture

import (
	"fmt"
	"io"
)

func Render(w io.Writer, name string) int {
	err := inspect(name)
	if err != nil {
		return 0
	}
	fmt.Fprintf(w, "hello %s", name)
	return 1
}

func inspect(name string) error { return nil }
`
	dir := writeTree(t, map[string]string{"render.go": body})
	report, err := probe(t, dir, "render.go")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkTemplate)
	if row.Guarded {
		t.Fatalf("row = %+v, want unguarded: err is the call's status, not the name", row)
	}
}

// The JavaScript extractor reads the file with a lexer instead of a parser, so the row says so: a heuristic
// row is still a promise, but a reader knows how much the tool claims.
func TestAnUnguardedJavaScriptParameterReachingTheFilesystemIsARow(t *testing.T) {
	body := `import fs from "node:fs";
import path from "node:path";

export function load(name) {
  return fs.readFileSync(path.join("data", name));
}
`
	dir := writeTree(t, map[string]string{"load.js": body})
	report, err := probe(t, dir, "load.js")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if row.Input != "name" || row.Origin != OriginPublic {
		t.Errorf("input = %q (%s), want the exported parameter", row.Input, row.Origin)
	}
	if row.Guarded || row.Guard != "" {
		t.Errorf("guard = %q guarded=%v, want none", row.Guard, row.Guarded)
	}
	if row.Function != "load" || row.SinkLine != lineOf(t, body, "fs.readFileSync") || row.SinkCall != "fs.readFileSync" {
		t.Errorf("row = %+v, want function load on the readFileSync line", row)
	}
	if row.Fidelity != FidelityHeuristic || report.Fidelity["javascript"] != FidelityHeuristic {
		t.Errorf("fidelity = %q/%q, want heuristic in the row and in the report", row.Fidelity, report.Fidelity["javascript"])
	}
	if !report.Blocking() {
		t.Error("an unguarded input reaching the filesystem must block")
	}
}

// The same flow with the check a reviewer would accept: the row stays, guarded, because the next skill step
// attacks the promise instead of trusting it.
func TestAGuardedJavaScriptSinkIsRecorded(t *testing.T) {
	body := `import fs from "node:fs";
import path from "node:path";

export function load(name) {
  const full = path.join("data", name);
  if (!full.startsWith("data/")) {
    throw new Error("outside the data directory");
  }
  return fs.readFileSync(full);
}
`
	dir := writeTree(t, map[string]string{"load.js": body})
	report, err := probe(t, dir, "load.js")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if !row.Guarded || !strings.Contains(row.Guard, "startsWith") {
		t.Fatalf("guard = %q guarded=%v, want the startsWith check recorded", row.Guard, row.Guarded)
	}
	if report.Blocking() {
		t.Errorf("a guarded inventory must not block: %+v", report.Rows)
	}
}

// A sink the source only talks about is not a sink: strings, template text, regex literals and comments are
// not data flow, which is the whole reason the lexer exists.
func TestASinkInsideJavaScriptTextIsNotARow(t *testing.T) {
	body := `import fs from "node:fs";

// fs.readFileSync(name) would be a sink if this comment were code.
/* child_process.execSync(cmd) is the same story. */
export function quiet(name) {
  const note = "fs.readFileSync(name)";
  const pattern = /fs\.readFileSync\(name\)/;
  const html = ` + "`" + `<div>${"static"}</div>` + "`" + `;
  return note + pattern.source + html;
}
`
	dir := writeTree(t, map[string]string{"quiet.js": body})
	report, err := probe(t, dir, "quiet.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 || report.Blocking() {
		t.Fatalf("rows = %+v, want none: text carries no flow", report.Rows)
	}
}

// Prototype pollution: a computed key written into an object, where the key came from the caller.
func TestAComputedJavaScriptKeyFromAnInputIsARow(t *testing.T) {
	body := `export function apply(target, source) {
  for (const key in source) {
    target[key] = source[key];
  }
  return target;
}
`
	dir := writeTree(t, map[string]string{"merge.js": body})
	report, err := probe(t, dir, "merge.js")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkDecode)
	if row.Function != "apply" || row.Guarded {
		t.Fatalf("row = %+v, want the unguarded merge in apply", row)
	}
	if row.SinkLine != lineOf(t, body, "target[key] = source[key]") {
		t.Errorf("sink line = %d, want the computed assignment", row.SinkLine)
	}
}

// A request value handed to a shell is the other defect, and the origin column has to say http.
func TestAJavaScriptRequestValueReachingExecIsARow(t *testing.T) {
	body := `import { execSync } from "node:child_process";

export function run(req) {
  const command = req.query.cmd;
  return execSync(command);
}
`
	dir := writeTree(t, map[string]string{"run.js": body})
	report, err := probe(t, dir, "run.js")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkExec)
	if row.Origin != OriginHTTP || row.Guard != "" {
		t.Errorf("row = %+v, want an unguarded http value reaching exec", row)
	}
	if row.SinkLine != lineOf(t, body, "execSync(command)") {
		t.Errorf("sink line = %d, want the execSync line", row.SinkLine)
	}
}

// TypeScript is the same file with annotations: the type is not a name, and a typed parameter is still an
// input.
func TestATypeScriptParameterIsMeasured(t *testing.T) {
	report, err := probe(t, ".", filepath.Join("testdata", "ts", "load.ts"))
	if err != nil {
		t.Fatal(err)
	}
	wants(t, report, SinkFile, SinkAllocation)
	if report.Fidelity["typescript"] != FidelityHeuristic {
		t.Errorf("fidelity = %q, want heuristic", report.Fidelity["typescript"])
	}
	for _, row := range report.Rows {
		if strings.Contains(row.Input, "string") && !strings.HasPrefix(row.Input, "name") && !strings.HasPrefix(row.Input, "count") {
			t.Errorf("row input = %q, want the parameter name with its type", row.Input)
		}
	}
}

// A class method is callable from outside when the class is exported, and the parameters of a handle — a
// request — are not inputs themselves: what the request is asked for is.
func TestAJavaScriptMethodRecordsItsParameters(t *testing.T) {
	body := `import fs from "node:fs";

export class AssetStore {
  read(name) {
    return fs.readFileSync(name);
  }

  _cache(name) {
    return fs.readFileSync(name);
  }
}
`
	dir := writeTree(t, map[string]string{"store.js": body})
	report, err := probe(t, dir, "store.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("rows = %+v, want one per method", report.Rows)
	}
	for _, row := range report.Rows {
		want := OriginPublic
		if row.Function == "_cache" {
			want = OriginInternal
		}
		if row.Origin != want {
			t.Errorf("row = %+v, want origin %s", row, want)
		}
	}
}

// A JavaScript test file is not shipped code: measuring it would report the fixture, not the promise.
func TestAJavaScriptTestFileIsNotMeasured(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"load.js":      "export function load(name) { return name; }\n",
		"load.test.js": "export function load(name) { return name; }\n",
	})
	report, err := probe(t, dir, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if strings.HasSuffix(row.File, ".test.js") {
			t.Fatalf("row = %+v, want the test file skipped", row)
		}
	}
	if joined := strings.Join(report.Skipped, "; "); !strings.Contains(joined, "load.test.js") {
		t.Errorf("skipped = %q, want the test file named", joined)
	}
}

// Python is read by indentation, not by an interpreter: an unguarded parameter opened as a path is the same
// promise, and the row says it was read heuristically.
func TestAnUnguardedPythonParameterReachingTheFilesystemIsARow(t *testing.T) {
	body := `import os


def load(name):
    return open(os.path.join("data", name)).read()
`
	dir := writeTree(t, map[string]string{"load.py": body})
	report, err := probe(t, dir, "load.py")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if row.Input != "name" || row.Origin != OriginPublic || row.Guarded {
		t.Errorf("row = %+v, want the public unguarded parameter", row)
	}
	if row.Function != "load" || row.SinkLine != lineOf(t, body, "open(os.path.join") {
		t.Errorf("row = %+v, want function load on the open line", row)
	}
	if row.Fidelity != FidelityHeuristic || report.Fidelity["python"] != FidelityHeuristic {
		t.Errorf("fidelity = %q/%q, want heuristic", row.Fidelity, report.Fidelity["python"])
	}
	if !report.Blocking() {
		t.Error("an unguarded input reaching the filesystem must block")
	}
}

// The check a Python reviewer would accept, in the shape Python writes it.
func TestAGuardedPythonSinkIsRecorded(t *testing.T) {
	body := `import os


def load(name):
    path = os.path.join("data", name)
    if not path.startswith("data/"):
        raise ValueError("outside the data directory")
    with open(path) as handle:
        return handle.read()
`
	dir := writeTree(t, map[string]string{"load.py": body})
	report, err := probe(t, dir, "load.py")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkFile)
	if !row.Guarded || !strings.Contains(row.Guard, "startswith") {
		t.Fatalf("guard = %q guarded=%v, want the startswith check recorded", row.Guard, row.Guarded)
	}
	if report.Blocking() {
		t.Errorf("a guarded inventory must not block: %+v", report.Rows)
	}
}

// A sink inside a docstring, a comment or a plain string is not a call.
func TestASinkInsidePythonTextIsNotARow(t *testing.T) {
	body := `import os

# open(name) would be a sink if this comment were code.
def quiet(name):
    """open(name) and os.system(name) are only a promise in a docstring."""
    note = "open(name)"
    pattern = r"os\.system\(name\)"
    return note + pattern
`
	dir := writeTree(t, map[string]string{"quiet.py": body})
	report, err := probe(t, dir, "quiet.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 || report.Blocking() {
		t.Fatalf("rows = %+v, want none: text carries no flow", report.Rows)
	}
}

// An f-string handed to the database is the injection a Python review is looking for.
func TestAPythonFStringReachingTheDatabaseIsARow(t *testing.T) {
	body := `def find(cursor, name):
    cursor.execute(f"SELECT * FROM users WHERE name = '{name}'")
`
	dir := writeTree(t, map[string]string{"find.py": body})
	report, err := probe(t, dir, "find.py")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkSQL)
	if row.Guarded || row.SinkLine != lineOf(t, body, "cursor.execute") {
		t.Errorf("row = %+v, want the unguarded f-string on the execute line", row)
	}
	if row.Input != "name" {
		t.Errorf("input = %q, want the parameter, not the cursor handle", row.Input)
	}
}

// shell=True turns a parameter into a program.
func TestAPythonShellCommandFromAnInputIsARow(t *testing.T) {
	body := `import subprocess


def run(command):
    return subprocess.run(command, shell=True, capture_output=True)
`
	dir := writeTree(t, map[string]string{"run.py": body})
	report, err := probe(t, dir, "run.py")
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, report, SinkExec)
	if row.Guarded || row.SinkCall != "subprocess.run" {
		t.Errorf("row = %+v, want the unguarded subprocess.run", row)
	}
}

// A method's first parameter is the object, not an input: the caller never sends it.
func TestAPythonMethodSkipsSelf(t *testing.T) {
	body := `class Store:
    def read(self, name):
        with open(name) as handle:
            return handle.read()

    def _secret(self, name):
        return open(name).read()
`
	dir := writeTree(t, map[string]string{"store.py": body})
	report, err := probe(t, dir, "store.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("rows = %+v, want one per method", report.Rows)
	}
	for _, row := range report.Rows {
		if row.Input != "name" {
			t.Errorf("input = %q, want the parameter after self", row.Input)
		}
		if row.Function == "_secret" && row.Origin != OriginInternal {
			t.Errorf("row = %+v, want the underscore method internal", row)
		}
		if row.Function == "read" && row.Origin != OriginPublic {
			t.Errorf("row = %+v, want the plain method public", row)
		}
	}
}

// A Python test file is not shipped code either.
func TestAPythonTestFileIsNotMeasured(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"load.py":      "def load(name):\n    return open(name)\n",
		"test_load.py": "def load(name):\n    return open(name)\n",
		"load_test.py": "def load(name):\n    return open(name)\n",
	})
	report, err := probe(t, dir, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 1 || report.Rows[0].File != "load.py" {
		t.Fatalf("rows = %+v, want only the shipped file measured", report.Rows)
	}
}

// An allocation or a loop the caller's collection already bounds is proportional, in Python too: a row there
// is a probe nobody can break.
func TestAPythonLoopOverACollectionIsNotARow(t *testing.T) {
	body := `def render(rows):
    width = len(rows)
    out = []
    for row in rows:
        out.append(str(row))
    return "".join(out), width
`
	dir := writeTree(t, map[string]string{"render.py": body})
	report, err := probe(t, dir, "render.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("rows = %+v, want none: the loop walks the caller's own collection", report.Rows)
	}
}

// fixture reads one file from testdata, so the bytes a test asserts on are the bytes a reviewer can run the
// probe against themselves.
func fixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A spread rebuilds an object from a row of the caller's array: it copies own properties and cannot reach a
// prototype, so it is not the prototype-pollution sink and a row there is a probe nobody can break.
func TestASpreadRebuiltFromAnInputIsNotARow(t *testing.T) {
	dir := writeTree(t, map[string]string{"rows.js": `export function normalize(rows) {
  return rows.map((row) => ({ ...row, amount: Number(row.amount) }));
}
`})
	report, err := probe(t, dir, "rows.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Sink == SinkDecode {
			t.Fatalf("row = %+v, want no decode row: a spread cannot pollute a prototype", row)
		}
	}
}

// A JavaScript loop or allocation bounded by the length of a collection the caller handed over is
// proportional: the caller cannot make it run longer than the data it passed.
func TestAJavaScriptLoopBoundedByACollectionIsNotARow(t *testing.T) {
	report, err := probe(t, ".", filepath.Join("testdata", "js", "copy.js"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("rows = %+v, want none: rows.length bounds both", report.Rows)
	}
}

// A name holding a call's verdict is not the input: `if (!exists)` says the call failed, not that the value was
// examined, so it does not guard the read that follows.
func TestAJavaScriptVerdictNameIsNotAGuard(t *testing.T) {
	body := fixture(t, filepath.Join("js", "verdict.js"))
	report, err := probe(t, ".", filepath.Join("testdata", "js", "verdict.js"))
	if err != nil {
		t.Fatal(err)
	}
	read := lineOf(t, body, "fs.readFileSync(name)")
	for _, row := range report.Rows {
		if row.Sink == SinkFile && row.SinkLine == read {
			if row.Guarded {
				t.Fatalf("row = %+v, want the read unguarded: exists is a verdict, not the value", row)
			}
			return
		}
	}
	t.Fatalf("no file row on the read line: %+v", report.Rows)
}

// A subprocess handed an argv list runs one program with the arguments it was given: the value cannot become a
// shell command, so a row there is a probe nobody can break.
func TestAPythonArgvListIsNotACommandInjectionRow(t *testing.T) {
	body := `import subprocess


def probe(name):
    return subprocess.run(["ls", "-l", name], capture_output=True)
`
	dir := writeTree(t, map[string]string{"probe.py": body})
	report, err := probe(t, dir, "probe.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Sink == SinkExec {
			t.Fatalf("row = %+v, want no exec row: an argv list does not reach a shell", row)
		}
	}
}

// rowsOf answers every row one function produced, so a test can talk about a function instead of a sink kind.
func rowsOf(report Report, function string) []Row {
	var found []Row
	for _, row := range report.Rows {
		if row.Function == function {
			found = append(found, row)
		}
	}
	return found
}

// S9: the sink of a returned path is the caller's, so an exported function that joins an input into a path and
// returns it is a row even when nothing in the function opens it. The guard is the one written here, and it
// shows its own body one call deep, because a reader has to see what the check does not inspect.
func TestAnExportedFunctionReturningAJoinedPathIsARow(t *testing.T) {
	body := `package fixture

import (
	"errors"
	"path/filepath"
	"strings"
)

type Resolver struct{ root string }

func (r *Resolver) inside(p string) bool {
	return strings.HasPrefix(p, r.root)
}

func (r *Resolver) Resolve(requestPath string) (string, error) {
	joined := filepath.Join(r.root, requestPath)
	if !r.inside(joined) {
		return "", errors.New("outside the root")
	}
	return joined, nil
}

func (r *Resolver) Bundle(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty")
	}
	return filepath.Join(r.root, name), nil
}

func Echo(name string) string { return name }

func internalName(name string) string { return filepath.Join("data", name) }
`
	dir := writeTree(t, map[string]string{"resolve.go": body})
	report, err := probe(t, dir, "resolve.go")
	if err != nil {
		t.Fatal(err)
	}

	resolved := rowsOf(report, "Resolve")
	if len(resolved) != 1 || resolved[0].Sink != SinkReturnedPath {
		t.Fatalf("Resolve rows = %+v, want one %q row", resolved, SinkReturnedPath)
	}
	row := resolved[0]
	if row.SinkLine != lineOf(t, body, "return joined, nil") || row.Input != "requestPath string" || row.Origin != OriginPublic {
		t.Errorf("row = %+v, want the public parameter at the return line", row)
	}
	if !row.Guarded || !strings.Contains(row.Guard, "!r.inside(joined) -> ") {
		t.Errorf("guard = %q, want the guard followed by the body it calls", row.Guard)
	}
	if row.GuardBody != "strings.HasPrefix(p, r.root)" {
		t.Errorf("guard body = %q, want the body of inside", row.GuardBody)
	}

	bundled := rowsOf(report, "Bundle")
	if len(bundled) != 1 || bundled[0].Sink != SinkReturnedPath {
		t.Fatalf("Bundle rows = %+v, want one %q row", bundled, SinkReturnedPath)
	}
	if bundled[0].Guarded {
		t.Errorf("row = %+v, want it unguarded: an empty check is presence, not containment", bundled[0])
	}
	if bundled[0].SinkLine != lineOf(t, body, `return filepath.Join(r.root, name), nil`) {
		t.Errorf("row = %+v, want it at Bundle's return", bundled[0])
	}

	// A value handed back unchanged is not built here, and an unexported function's caller is inside the
	// package, so neither is a returned row.
	for _, function := range []string{"Echo", "internalName"} {
		if rows := rowsOf(report, function); len(rows) != 0 {
			t.Errorf("%s rows = %+v, want none", function, rows)
		}
	}
}

// A guard written as a call into another file of the package is still one call deep: the body lives in the
// package, and the report reads it there.
func TestAGoGuardBodyResolvesAcrossThePackage(t *testing.T) {
	files := map[string]string{
		"resolve.go": `package fixture

import "path/filepath"

func Read(name string) (string, error) {
	p := filepath.Join("data", name)
	if !inside(p) {
		return "", nil
	}
	return p, nil
}
`,
		"guard.go": `package fixture

import "strings"

func inside(p string) bool {
	return strings.HasPrefix(p, "data/")
}
`,
	}
	dir := writeTree(t, files)
	report, err := probe(t, dir, "resolve.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "Read")
	if len(rows) != 1 || rows[0].Sink != SinkReturnedPath {
		t.Fatalf("rows = %+v, want one returned path", rows)
	}
	if rows[0].GuardBody != `strings.HasPrefix(p, "data/")` {
		t.Errorf("guard body = %q, want the body from the other file", rows[0].GuardBody)
	}
}

// S9 in a lexer language: the returned path is the promise, and the guard's body is the sentence a reader has
// to attack.
func TestAJavaScriptReturnedPathAndItsGuardBody(t *testing.T) {
	body := `import path from 'node:path';

const ROOT = '/srv';

function inside(p) {
  return p.startsWith(ROOT + '/');
}

export function resolvePath(userPath) {
  const joined = path.join(ROOT, userPath);
  if (!inside(joined)) throw new Error('outside the root');
  return joined;
}

export function echo(value) {
  return value;
}
`
	dir := writeTree(t, map[string]string{"paths.js": body})
	report, err := probe(t, dir, "paths.js")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "resolvePath")
	if len(rows) != 1 || rows[0].Sink != SinkReturnedPath {
		t.Fatalf("resolvePath rows = %+v, want one %q row", rows, SinkReturnedPath)
	}
	if rows[0].SinkLine != lineOf(t, body, "return joined;") || rows[0].Input != "userPath" {
		t.Errorf("row = %+v, want the parameter at the return line", rows[0])
	}
	if !rows[0].Guarded || rows[0].GuardBody == "" || !strings.Contains(rows[0].GuardBody, "startsWith") {
		t.Errorf("row = %+v, want the guard's body resolved", rows[0])
	}
	if rows := rowsOf(report, "echo"); len(rows) != 0 {
		t.Errorf("echo rows = %+v, want none: the value is handed back unchanged", rows)
	}
}

// Markup built from an input and returned to the caller is the template-injection promise, and the escaper it
// calls is the guard whose body a reader has to attack.
func TestAJavaScriptReturnedMarkupIsARow(t *testing.T) {
	body := "function escapeAttr(value) {\n" +
		"  return String(value).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;' })[c]);\n" +
		"}\n\n" +
		"export function attr(name, value) {\n" +
		"  return `${name}=\"${escapeAttr(value)}\"`;\n" +
		"}\n"
	dir := writeTree(t, map[string]string{"render.js": body})
	report, err := probe(t, dir, "render.js")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "attr")
	if len(rows) == 0 {
		t.Fatalf("attr rows = %+v, want a returned markup row", report.Rows)
	}
	for _, row := range rows {
		if row.Sink != SinkReturnedMarkup {
			continue
		}
		if row.SinkLine != lineOf(t, body, "return `${name}") {
			t.Errorf("row = %+v, want it at the return", row)
		}
		return
	}
	t.Fatalf("attr rows = %+v, want one %q row", rows, SinkReturnedMarkup)
}

// The same promise in Python: os.path.join hands the caller a path it did not check itself.
func TestAPythonReturnedPathAndItsGuardBody(t *testing.T) {
	body := `import os

BASE = "/srv"


def inside(p):
    return p.startswith(BASE + "/")


def resolve(user_path):
    joined = os.path.join(BASE, user_path)
    if not inside(joined):
        raise ValueError("outside the root")
    return joined


def echo(value):
    return value
`
	dir := writeTree(t, map[string]string{"paths.py": body})
	report, err := probe(t, dir, "paths.py")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "resolve")
	if len(rows) != 1 || rows[0].Sink != SinkReturnedPath {
		t.Fatalf("resolve rows = %+v, want one %q row", rows, SinkReturnedPath)
	}
	if rows[0].SinkLine != lineOf(t, body, "    return joined") || rows[0].Input != "user_path" {
		t.Errorf("row = %+v, want the parameter at the return line", rows[0])
	}
	if !rows[0].Guarded || !strings.Contains(rows[0].GuardBody, "startswith") {
		t.Errorf("row = %+v, want the guard's body resolved", rows[0])
	}
	if rows := rowsOf(report, "echo"); len(rows) != 0 {
		t.Errorf("echo rows = %+v, want none: the value is handed back unchanged", rows)
	}
}

// S12: a value built in a helper of the same package and handed to a sink there is the caller's promise. The
// flow is followed one hop — far enough to see the sink, not far enough to invent one.
func TestAnInputPassedToAPackageHelperThatReachesASinkIsARow(t *testing.T) {
	files := map[string]string{
		"read.go": `package fixture

import "os"

func openBuilt(p string) (*os.File, error) {
	return os.Open("data/" + p)
}

func Load(name string) (*os.File, error) {
	return openBuilt(name)
}

func openPath(p string) (*os.File, error) {
	return os.Open(p)
}

func Read(name string) (*os.File, error) {
	return openPath(name)
}

func passthrough(p string) (*os.File, error) {
	return openPath(p)
}

func ReadTwo(name string) (*os.File, error) {
	return passthrough(name)
}
`,
	}
	dir := writeTree(t, files)
	report, err := probe(t, dir, "read.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "Load")
	if len(rows) != 1 || rows[0].Sink != SinkFile {
		t.Fatalf("Load rows = %+v, want one %q row", rows, SinkFile)
	}
	if rows[0].SinkLine != lineOf(t, files["read.go"], "return openBuilt(name)") {
		t.Errorf("sink line = %d, want the call the input was handed to", rows[0].SinkLine)
	}
	if !strings.Contains(rows[0].SinkCall, "openBuilt") || rows[0].Input != "name string" {
		t.Errorf("row = %+v, want the parameter named at the helper call", rows[0])
	}
	// A helper that hands the parameter straight to the file call adds nothing the caller did not write, and
	// the hop stops at one: forwarding the value to another helper is not followed.
	for _, function := range []string{"Read", "ReadTwo"} {
		if rows := rowsOf(report, function); len(rows) != 0 {
			t.Errorf("%s rows = %+v, want none: a pass-through helper adds nothing", function, rows)
		}
	}
}

// A guard written inside the helper is the check standing in front of the sink, so the caller's row carries
// it instead of claiming the input is unguarded.
func TestAPackageHelperCarriesItsOwnGuardIntoTheCallersRow(t *testing.T) {
	body := `package fixture

import (
		"os"
		"strings"
)

func openInside(p string) (*os.File, error) {
		full := "data/" + p
		if !strings.HasPrefix(full, "data/") {
			return nil, os.ErrNotExist
		}
		return os.Open(full)
}

func Load(name string) (*os.File, error) {
		return openInside(name)
}
`
	dir := writeTree(t, map[string]string{"load.go": body})
	report, err := probe(t, dir, "load.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "Load")
	if len(rows) != 1 || rows[0].Sink != SinkFile {
		t.Fatalf("Load rows = %+v, want one %q row", rows, SinkFile)
	}
	if !rows[0].Guarded || !strings.Contains(rows[0].Guard, "HasPrefix") {
		t.Errorf("row = %+v, want the helper's own check as the guard", rows[0])
	}
}

// S12: a statement executor is a method whose name says so, not only one of eight spelled-out names. The
// statement text is built by concatenation, so nothing between the filter and the query is a bound argument.
func TestAConcatenatedStatementReachingAQueryTextCallIsARow(t *testing.T) {
	body := `package fixture

type DB struct{}

func (db *DB) QueryText(statement string) (int, error) { return 0, nil }

func Export(db *DB, tenant, status string) (int, error) {
	where := "tenant = '" + tenant + "'"
	if status != "" {
		where += " AND status = '" + status + "'"
	}
	statement := "SELECT id FROM events WHERE " + where
	return db.QueryText(statement)
}
`
	dir := writeTree(t, map[string]string{"export.go": body})
	report, err := probe(t, dir, "export.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "Export")
	var sql []Row
	for _, row := range rows {
		if row.Sink == SinkSQL {
			sql = append(sql, row)
		}
	}
	if len(sql) == 0 {
		t.Fatalf("Export rows = %+v, want a %q row for the concatenated statement", rows, SinkSQL)
	}
	if sql[0].SinkLine != lineOf(t, body, "return db.QueryText(statement)") {
		t.Errorf("sink line = %d, want the QueryText call", sql[0].SinkLine)
	}
	named := false
	for _, row := range sql {
		if row.Input == "tenant string" || row.Input == "status string" {
			named = true
		}
	}
	if !named {
		t.Errorf("rows = %+v, want the parameter the statement was built from", sql)
	}
}

// S12: a case-folding comparison is the access decision, so it cannot also be the guard that makes it safe.
// The row is unguarded even though the comparison is what the row is about.
func TestACaseFoldedAccessCheckIsAnUnguardedCaseSink(t *testing.T) {
	body := `package fixture

import "strings"

type Token struct{ Subject string }
type Principal struct{ Subject string }

var accounts = []Principal{{Subject: "kelvin"}}

func Resolve(tok Token) (Principal, error) {
	for _, account := range accounts {
		if strings.EqualFold(account.Subject, tok.Subject) {
			return account, nil
		}
	}
	return Principal{}, nil
}
`
	dir := writeTree(t, map[string]string{"resolve.go": body})
	report, err := probe(t, dir, "resolve.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "Resolve")
	var cases []Row
	for _, row := range rows {
		if row.Sink == SinkCase {
			cases = append(cases, row)
		}
	}
	if len(cases) != 1 {
		t.Fatalf("Resolve rows = %+v, want one %q row", rows, SinkCase)
	}
	if cases[0].Guarded || cases[0].Guard != "" {
		t.Errorf("row = %+v, want the fold comparison reported as the sink it is, not as its own guard", cases[0])
	}
	if cases[0].SinkLine != lineOf(t, body, "strings.EqualFold") {
		t.Errorf("sink line = %d, want the EqualFold line", cases[0].SinkLine)
	}
	if !report.Blocking() {
		t.Error("an unguarded case-folding access check must block")
	}
}

// S12: not every Date near an input is a time zone promise. A timestamp that is built and formatted is a
// rendering; the row belongs to the time that decides something.
func TestAJavaScriptTimezoneRowNeedsTheTimeToDecideSomething(t *testing.T) {
	body := `export function stamp(value) {
  return new Date(value).toISOString();
}

export function expired(value, limit) {
  return new Date(value) <= new Date(limit);
}
`
	dir := writeTree(t, map[string]string{"dates.js": body})
	report, err := probe(t, dir, "dates.js")
	if err != nil {
		t.Fatal(err)
	}
	if rows := rowsOf(report, "stamp"); len(rows) != 0 {
		t.Errorf("stamp rows = %+v, want none: formatting a timestamp decides nothing", rows)
	}
	rows := rowsOf(report, "expired")
	if len(rows) == 0 {
		t.Fatalf("expired rows = %+v, want a %q row", report.Rows, SinkTimezone)
	}
	for _, row := range rows {
		if row.Sink != SinkTimezone {
			t.Errorf("row = %+v, want the comparison reported as %q", row, SinkTimezone)
		}
	}
}

// S12: a computed key written into a map the function just built is a rename of where the value sits. The
// prototype that matters is one the caller owns.
func TestAJavaScriptComputedKeyNeedsATargetTheCallerOwns(t *testing.T) {
	body := `export function collect(rows) {
  const out = {};
  for (const [key, value] of Object.entries(rows)) out[key] = value;
  return out;
}

export function apply(target, patch) {
  for (const key of Object.keys(patch)) target[key] = patch[key];
  return target;
}
`
	dir := writeTree(t, map[string]string{"keys.js": body})
	report, err := probe(t, dir, "keys.js")
	if err != nil {
		t.Fatal(err)
	}
	if rows := rowsOf(report, "collect"); len(rows) != 0 {
		t.Errorf("collect rows = %+v, want none: the object is a map this function built", rows)
	}
	rows := rowsOf(report, "apply")
	if len(rows) != 1 || rows[0].Sink != SinkDecode {
		t.Fatalf("apply rows = %+v, want one %q row", rows, SinkDecode)
	}
}

// S12: one input reaching one kind of sink is one promise, however many statements the lexer walks. A report
// that repeats the same paragraph per statement is the noise a small model pays turns for.
func TestAJavaScriptInputReachingOneKindIsOneRow(t *testing.T) {
	body := `export function span(value) {
  const first = new Date(value).getTime();
  const second = new Date(value).getTime() + 1;
  if (first > second) throw new Error('out of order');
  return second - first;
}
`
	dir := writeTree(t, map[string]string{"span.js": body})
	report, err := probe(t, dir, "span.js")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "span")
	if len(rows) != 1 {
		t.Fatalf("span rows = %d (%+v), want one row for one input reaching one kind", len(rows), rows)
	}
	if rows[0].SinkLine != lineOf(t, body, "const first = new Date(value)") {
		t.Errorf("sink line = %d, want the first statement the input reaches the kind at", rows[0].SinkLine)
	}
}

// S12: a method this file writes for itself is not the statement executor the catalog is about. `prepare`
// names a database call on a receiver that is one, and a helper on the object this file defines.
func TestAJavaScriptOwnHelperNamedLikeAStatementCallIsNotASQLRow(t *testing.T) {
	body := `export class Relay {
  prepare(delivery) {
    return { host: delivery.url };
  }

  send(delivery, db) {
    const plan = this.prepare(delivery);
    return db.query('SELECT ' + plan.host);
  }
}
`
	dir := writeTree(t, map[string]string{"relay.js": body})
	report, err := probe(t, dir, "relay.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Sink != SinkSQL {
			continue
		}
		if strings.Contains(row.SinkCall, "this.prepare") {
			t.Errorf("row = %+v, want no statement call for the file's own helper", row)
		}
		if !strings.Contains(row.SinkCall, "query") {
			t.Errorf("row = %+v, want the statement the database was handed", row)
		}
	}
}

// A library call is not a helper of this package even when a function here shares its name: `os.Remove` is the
// standard library's, and the file's imports are what tells them apart.
func TestALibraryCallThatSharesAPackageFunctionNameIsNotAHop(t *testing.T) {
	body := `package fixture

import (
	"errors"
	"os"
	"strings"
)

var errOutside = errors.New("outside /tmp")

func Remove(name string) error {
	if !strings.HasPrefix(name, "/tmp/") {
		return errOutside
	}
	return os.Remove(name)
}

func Cleanup(name string) error {
	return os.Remove(name)
}
`
	dir := writeTree(t, map[string]string{"remove.go": body})
	report, err := probe(t, dir, "remove.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "Cleanup")
	if len(rows) != 1 || rows[0].Sink != SinkFile {
		t.Fatalf("Cleanup rows = %+v, want one %q row: os.Remove is the library's call", rows, SinkFile)
	}
	if rows[0].SinkCall != "os.Remove" || rows[0].Guarded {
		t.Errorf("row = %+v, want the library call itself with no guard borrowed from the file's own Remove", rows[0])
	}
}
