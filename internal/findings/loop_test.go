package findings_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/findings"
	"os/exec"
	"strings"
)

const fixedValue = "package sample\nfunc Value() int { return 1 }\n"
const passingTest = "package sample\nimport \"testing\"\nfunc TestStillOne(t *testing.T) { if Value() != 1 { t.Fatal(\"no\") } }\n"
const fixInstruction = "settle each item: when the contract promises what its test asserts, fix the source; when it does not, record `tsp findings verdict F<n> rejected --reason \"<why>\"` and remove that test. Never edit a witness test to make it pass, and never record wontfix, a check waiver or .no-testing-gate: those decisions are the user's. Then run `tsp findings next`."
const promiseLine = "F1 open value_test.go example.invalid/sample.TestPromise — Keeps the promise."

// next answers what the TSP loop owes now and fails the test on any other exit code.
func next(t *testing.T, d string, code int) findings.Answer {
	t.Helper()
	a, got := findings.Next(d, wait)
	if got != code {
		t.Fatalf("next exit=%d want %d: %+v\n%s", got, code, a, a.Text())
	}
	return a
}

// withNotes is a message followed by the answer's notes, as next writes it.
func withNotes(s string, a findings.Answer) string {
	for _, n := range a.Notes {
		s += "\n" + n
	}
	return s
}

// notesText is the answer's notes as the text output ends with them.
func notesText(a findings.Answer) string {
	s := ""
	for _, n := range a.Notes {
		s += n + "\n"
	}
	return s
}

func registryFile(t *testing.T) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "topic.json"))
	if len(files) != 1 {
		t.Fatalf("registry: %v", files)
	}
	return files[0]
}

func storedLoop(t *testing.T) *findings.Loop {
	t.Helper()
	data, err := os.ReadFile(registryFile(t))
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Loop *findings.Loop `json:"loop"`
	}
	if err := json.Unmarshal(data, &stored); err != nil || stored.Loop == nil {
		t.Fatalf("stored loop: %s %v", data, err)
	}
	return stored.Loop
}

// openRoundOne records round 1 with one open witness and asks for its fix.
func openRoundOne(t *testing.T) string {
	t.Helper()
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	next(t, d, 1)
	return d
}

func TestNextWithoutARoundIsAnError(t *testing.T) {
	d := repo(t)
	a := next(t, d, 2)
	want := "no TSP round is open on topic: run the TSP flow, then `tsp findings record`"
	if a.Next != "error" || a.Reason != want || a.Message != want || a.Items == nil || len(a.Items) != 0 || a.Scope != nil || a.Text() != "error: "+want+"\n" {
		t.Fatalf("answer: %+v\n%q", a, a.Text())
	}
	if files, _ := filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "*.json")); len(files) != 0 {
		t.Fatalf("an answer with nothing to judge wrote a registry: %v", files)
	}
}

// A registry written before the loop existed keeps working: check reviews it as before, and next says no round
// is open without touching it.
func TestARegistryFromBeforeTheLoopStillReviews(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	path := registryFile(t)
	var raw map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	delete(raw, "loop")
	old, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	a := next(t, d, 2)
	if a.Next != "error" {
		t.Fatalf("answer: %+v", a)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(after, old) {
		t.Fatalf("next changed a registry it could not judge:\n%s", after)
	}
	review(t, d, 1, "open", 0, 0, 1, 0, 0, 1)
}

func TestNextAsksForAFixWhileAWitnessIsOpen(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	a := next(t, d, 1)
	item := findings.Item{ID: "F1", Status: "open", File: "value_test.go", Test: "example.invalid/sample.TestPromise", Detail: "Keeps the promise."}
	review := "review 1 on topic: 0 fixed, 0 open, 1 new, 0 regressed"
	message := promiseLine + "\n" + fixInstruction
	if a.Next != "fix" || a.Round != 1 || a.MaxRounds != 5 || a.Reason != "round 1 of 5 left 1 item unsettled" || a.Review != review ||
		!reflect.DeepEqual(a.Items, []findings.Item{item}) || a.Scope != nil || a.Message != message {
		t.Fatalf("answer: %+v", a)
	}
	if want := "fix: round 1 of 5 left 1 item unsettled\n" + review + "\n" + message + "\n"; a.Text() != want {
		t.Fatalf("text:\n%s\nwant:\n%s", a.Text(), want)
	}
	if !reflect.DeepEqual(a.Rounds, []findings.RoundCount{{N: 1, New: []string{"F1"}}}) {
		t.Fatalf("rounds: %+v", a.Rounds)
	}
	if loop := storedLoop(t); a.Tree == "" || a.Tree != loop.FixTree || snapshotCommand(t, d, "cat-file", "-t", a.Tree) != "tree" {
		t.Fatalf("the answer names the tree it judged: %q, loop %+v", a.Tree, loop)
	}
}

// Once everything is settled, source the last round never saw owes another round, aimed at that change. The
// snapshot naming it is a tree of the fixed working tree; taking it moves no ref, no commit and no index entry.
func TestNextAsksForTheNextRoundAfterTheFixChangedSource(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	status := command(t, d, "git", "status", "--porcelain", "--untracked-files=all")
	refs := command(t, d, "git", "for-each-ref")
	head := command(t, d, "git", "rev-parse", "HEAD")
	a := next(t, d, 1)
	loop := storedLoop(t)
	since := loop.Rounds[0].End
	if a.Next != "tsp" || a.Round != 2 || a.Reason != "round 2 of 5 — the last fix changed 1 source file: value.go" ||
		a.Review != "review 2 on topic: 1 fixed, 0 open, 0 new, 0 regressed" || len(a.Items) != 0 || a.Scope == nil ||
		a.Scope.Since != since || a.Scope.Base != loop.Pending || !reflect.DeepEqual(a.Scope.Paths, []string{"value.go"}) {
		t.Fatalf("answer: %+v scope %+v loop %+v", a, a.Scope, loop)
	}
	env, e := bench.SnapshotEnv(d)
	if e != nil {
		t.Fatal(e)
	}
	for i, v := range env {
		env[i] = "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
	}
	message := fmt.Sprintf("round 2 of 5 of the TSP loop: the last fix changed value.go. Attack that change and everything it affects (`env %s git --literal-pathspecs diff %s %s -- 'value.go'`); do not re-attack code it did not touch. Then run `tsp findings record` and `tsp findings next`.", strings.Join(env, " "), since, loop.Pending)
	if a.Message != message || a.Text() != "tsp: "+a.Reason+"\n"+a.Review+"\n"+message+"\n" {
		t.Fatalf("message:\n%s\nwant:\n%s\ntext:\n%s", a.Message, message, a.Text())
	}
	if kind := snapshotCommand(t, d, "cat-file", "-t", loop.Pending); kind != "tree" {
		t.Fatalf("pending snapshot is a %s", kind)
	}
	if body := snapshotCommand(t, d, "show", loop.Pending+":value.go"); body+"\n" != fixedValue {
		t.Fatalf("snapshot holds %q", body)
	}
	if command(t, d, "git", "status", "--porcelain", "--untracked-files=all") != status || command(t, d, "git", "for-each-ref") != refs || command(t, d, "git", "rev-parse", "HEAD") != head {
		t.Fatal("taking the snapshot changed the repository")
	}
	again := next(t, d, 1)
	if again.Next != "tsp" || again.Round != 2 || again.Scope == nil || again.Scope.Base != a.Scope.Base {
		t.Fatalf("a re-query moved the round: %+v", again)
	}
}

// The reason rounds after the first are judged against the snapshot: a test that pins what the fix broke passes on
// HEAD's code, so judged there it would pin nothing and the regression would go unrecorded.
func TestARoundAfterAFixIsJudgedAgainstTheFixedCode(t *testing.T) {
	d := repo(t)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 2 * x }\n")
	command(t, d, "git", "add", ".")
	command(t, d, "git", "commit", "-qm", "double")
	put(t, d, "value_test.go", redTest)
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 3 * x }\n")
	a := next(t, d, 1)
	put(t, d, "double_test.go", "package sample\nimport \"testing\"\n// Doubles its argument.\nfunc TestDouble(t *testing.T) { if Double(2) != 4 { t.Fatal(\"not doubled\") } }\n")
	// Asking again before the round is recorded must not swallow the round's own test into the base.
	if again := next(t, d, 1); again.Next != "fix" || len(again.Items) != 1 || again.Items[0].Test != "example.invalid/sample.TestDouble" || storedLoop(t).Pending != a.Scope.Base {
		t.Fatalf("a re-query hid the unrecorded failure or moved the pending base: %+v", again)
	}
	out, code := findings.Record(d, "", wait)
	if code != 0 || out != "F2 recorded double_test.go example.invalid/sample.TestDouble\n" {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	loop := storedLoop(t)
	if len(loop.Rounds) != 2 || loop.Rounds[1].N != 2 || loop.Rounds[1].Base != a.Scope.Base || !reflect.DeepEqual(loop.Rounds[1].New, []string{"F2"}) || loop.Pending != "" {
		t.Fatalf("round 2: %+v", loop)
	}
	b := next(t, d, 1)
	if b.Next != "fix" || b.Round != 2 || len(b.Items) != 1 || b.Items[0].ID != "F2" {
		t.Fatalf("answer: %+v", b)
	}
}

// Recording again after the fix refreshes the round but never moves where it ended, or the fix would hide from next.
func TestARecordAfterTheFixKeepsWhereTheRoundEnded(t *testing.T) {
	d := openRoundOne(t)
	end := storedLoop(t).Rounds[0].End
	put(t, d, "value.go", fixedValue)
	record(t, d)
	if loop := storedLoop(t); len(loop.Rounds) != 1 || loop.Rounds[0].End != end || !reflect.DeepEqual(loop.Rounds[0].New, []string{"F1"}) {
		t.Fatalf("refresh moved the round: %+v", loop)
	}
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("the fix was hidden: %+v", a)
	}
}

func TestADryRoundOverTheFixedCodeConverges(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	put(t, d, "more_test.go", passingTest)
	if out, code := findings.Record(d, "", wait); code != 1 || out != "no defect witnesses recorded\n" {
		t.Fatalf("dry round exit=%d: %q", code, out)
	}
	a := next(t, d, 0)
	reason := "converged after 2 rounds: 1 fixed, 0 rejected, 0 wontfix"
	if a.Next != "done" || a.Round != 2 || a.Reason != reason || a.Message != withNotes(reason, a) || len(a.Items) != 0 || a.Scope != nil ||
		!reflect.DeepEqual(a.Rounds, []findings.RoundCount{{N: 1, New: []string{"F1"}}, {N: 2, New: []string{}}}) {
		t.Fatalf("answer: %+v", a)
	}
	if want := "done: " + reason + "\n" + a.Review + "\n" + notesText(a); a.Text() != want {
		t.Fatalf("text %q want %q", a.Text(), want)
	}
	if !storedLoop(t).Closed {
		t.Fatal("a converged loop stays open")
	}
}

func TestARoundThatFindsNothingConvergesAtOnce(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", "package sample\nimport \"testing\"\nfunc TestZero(t *testing.T) { if Value() != 0 { t.Fatal(\"no\") } }\n")
	if out, code := findings.Record(d, "", wait); code != 1 || out != "no defect witnesses recorded\n" {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	a := next(t, d, 0)
	reason := "converged after 1 round: 0 fixed, 0 rejected, 0 wontfix"
	if a.Next != "done" || a.Round != 1 || a.Reason != reason || a.Review != "" || a.Text() != "done: "+reason+"\n"+notesText(a) {
		t.Fatalf("answer: %+v\n%q", a, a.Text())
	}
}

// Rejecting an invented witness changes no source, so the round that found it already attacked this code. Its test
// still has to leave the suite, or the loop would converge with a red test in it.
func TestARejectedFindingSettlesWithoutAnotherRound(t *testing.T) {
	d := openRoundOne(t)
	if out, code := findings.SetVerdict(d, "F1", "rejected", "the contract promises 0"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	still := findings.Item{ID: "F1", Status: "rejected", File: "value_test.go", Test: "example.invalid/sample.TestPromise", Detail: "rejected, but its test is still in the suite: remove it"}
	if a := next(t, d, 1); a.Next != "fix" || !reflect.DeepEqual(a.Items, []findings.Item{still}) {
		t.Fatalf("a rejected test left in the suite: %+v", a)
	}
	if err := os.Remove(filepath.Join(d, "value_test.go")); err != nil {
		t.Fatal(err)
	}
	a := next(t, d, 0)
	reason := "converged after 1 round: 0 fixed, 1 rejected, 0 wontfix"
	message := reason + "\nF1 rejected value_test.go example.invalid/sample.TestPromise — the contract promises 0"
	if a.Next != "done" || a.Reason != reason || a.Message != withNotes(message, a) || a.Text() != "done: "+reason+"\n"+a.Review+"\n"+message[len(reason)+1:]+"\n"+notesText(a) {
		t.Fatalf("answer: %+v\n%q", a, a.Text())
	}
}

// Five rounds is the cap: a fix after the fifth round has no round left to attack it, so the user decides.
func TestTheFifthRoundIsTheLast(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", "package sample\nimport \"testing\"\nfunc TestAlways(t *testing.T) {}\n")
	if out, code := findings.Record(d, "", wait); code != 1 {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	path := registryFile(t)
	var raw map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	loop := raw["loop"].(map[string]any)
	first := loop["rounds"].([]any)[0].(map[string]any)
	rounds := []any{first}
	for n := 2; n <= 5; n++ {
		copied := map[string]any{}
		for k, v := range first {
			copied[k] = v
		}
		copied["n"] = n
		rounds = append(rounds, copied)
	}
	loop["rounds"] = rounds
	seeded, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(path, seeded, 0600); err != nil {
		t.Fatal(err)
	}
	put(t, d, "value.go", fixedValue)
	a := next(t, d, 3)
	reason := "5 rounds reached: value.go changed after round 5 and no round has attacked that change"
	if a.Next != "stop" || a.Round != 5 || a.Reason != reason || a.Message != reason+" — the loop needs the user's decision" || a.Scope != nil || a.Text() != "stop: "+a.Message+"\n" {
		t.Fatalf("answer: %+v\n%q", a, a.Text())
	}
	if !storedLoop(t).Closed {
		t.Fatal("a stopped loop stays open")
	}
}

func snapshotCommand(t *testing.T, d string, args ...string) string {
	t.Helper()
	env, e := bench.SnapshotEnv(d)
	if e != nil {
		t.Fatal(e)
	}
	c := exec.Command("git", append([]string{"-C", d}, args...)...)
	c.Env = append(os.Environ(), env...)
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("private git %v: %v: %s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
