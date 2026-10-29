package findings_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

const fixedValue = "package sample\nfunc Value() int { return 1 }\n"
const passingTest = "package sample\nimport \"testing\"\nfunc TestStillOne(t *testing.T) { if Value() != 1 { t.Fatal(\"no\") } }\n"
const fixInstruction = "settle each item: when the contract promises what its test asserts, fix the source; when it does not, record `tsp findings verdict F<n> rejected --reason \"<why>\"` and remove that test. Never edit a witness test to make it pass, and never record wontfix, a check waiver or .no-testing-gate: those decisions are the user's. Then run `tsp findings next`."
const promiseLine = "F1 open value_test.go example.invalid/sample.TestPromise — Keeps the promise."

// next answers what the TSP loop owes now and fails the test on any other exit code.
func next(t *testing.T, d string, code int) findings.Answer {
	t.Helper()
	commitPending(t, d)
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
	if loop := storedLoop(t); a.Tree == "" || a.Tree != loop.FixTree || command(t, d, "git", "cat-file", "-t", a.Tree) != "commit" {
		t.Fatalf("the answer names the commit it judged: %q, loop %+v", a.Tree, loop)
	}
}

// Rounds after the first are judged against the fix commit: a test that pins what the fix broke passes on the base
// commit's code, so judged there it would pin nothing and the regression would go unrecorded.
func TestARoundAfterAFixIsJudgedAgainstTheFixedCode(t *testing.T) {
	d := repo(t)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 2 * x }\n")
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 3 * x }\n")
	fix := commitAll(t, d, "fix the promise and break the double")
	a := next(t, d, 1)
	if a.Next != "tsp" || a.Scope == nil || a.Scope.Base != fix || a.Tree != fix {
		t.Fatalf("round two must attack the fix commit: %+v", a)
	}
	put(t, d, "double_test.go", "package sample\nimport \"testing\"\n// Doubles its argument.\nfunc TestDouble(t *testing.T) { if Double(2) != 4 { t.Fatal(\"not doubled\") } }\n")
	commitAll(t, d, "pin the regression")
	out, code := recordSuite(t, d, "")
	if code != 0 || out != "F2 recorded double_test.go example.invalid/sample.TestDouble\n" {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	loop := storedLoop(t)
	if len(loop.Rounds) != 2 || loop.Rounds[1].N != 2 || loop.Rounds[1].Base != fix || !reflect.DeepEqual(loop.Rounds[1].New, []string{"F2"}) || loop.Pending != "" {
		t.Fatalf("round 2: %+v", loop)
	}
	b := next(t, d, 1)
	if b.Next != "fix" || b.Round != 2 || len(b.Items) != 1 || b.Items[0].ID != "F2" {
		t.Fatalf("answer: %+v", b)
	}
}

// A record after the fix moves the round's end: the next attack starts from the commit just judged.
func TestADryRoundOverTheFixedCodeConverges(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	commitAll(t, d, "fix the promise")
	next(t, d, 1)
	put(t, d, "more_test.go", passingTest)
	commitAll(t, d, "passing coverage")
	if out, code := recordSuite(t, d, ""); code != 1 || out != "no defect witnesses recorded\n" {
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
	commitAll(t, d, "round one")
	if out, code := recordSuite(t, d, ""); code != 1 || out != "no defect witnesses recorded\n" {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	a := next(t, d, 0)
	reason := "converged after 1 round: 0 fixed, 0 rejected, 0 wontfix"
	if a.Next != "done" || a.Round != 1 || a.Reason != reason || a.Review != "" || a.Text() != "done: "+reason+"\n"+notesText(a) {
		t.Fatalf("answer: %+v\n%q", a, a.Text())
	}
}

// Rejecting an invented witness ends with removing its test: while the test is still in the suite the loop owes
// that removal, and once it is gone the rejection settles the finding.
func TestARejectedTestStaysOwedUntilItIsRemoved(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	if out, code := findings.SetVerdict(d, "F1", "rejected", "not promised"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	a := next(t, d, 1)
	want := findings.Item{ID: "F1", Status: "rejected", File: "value_test.go", Test: "example.invalid/sample.TestPromise", Detail: "rejected by the user (not promised): remove this test"}
	if a.Next != "fix" || !reflect.DeepEqual(a.Items, []findings.Item{want}) {
		t.Fatalf("the rejected test is still in the suite: %+v", a)
	}
	if got := storedFindings(t)[0]; got.File != "value_test.go" || got.Verdict == nil {
		t.Fatalf("the rejection keeps its file and reason: %+v", got)
	}
	command(t, d, "git", "rm", "-q", "value_test.go")
	if a := next(t, d, 0); a.Next != "done" {
		t.Fatalf("removing the rejected test settles the finding: %+v", a)
	}
}
