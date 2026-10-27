package check

import (
	"errors"
	"strings"
	"testing"
)

// scopedRun is the offer's own call: the check answers for the paths this turn edited, in a tree that
// may already carry the commit that work became.
func scopedRun(f *fakeRepo, since string, paths ...string) Result {
	d := f.deps()
	d.PlanPath = ""
	d.Paths = paths
	d.Since = since
	return Run(f.root, d)
}

// A turn that commits before it ends leaves a clean tree. Reading the tree alone answered "no
// production source is changed" for the very source that commit changed, so the host that asks
// whether TSP is owed heard nothing after every committed work unit and never offered the run.
func TestSinceSeesTheCommittedWorkUnit(t *testing.T) {
	f := &fakeRepo{
		root:     "/repo",
		status:   "",
		diff:     "M\x00internal/x.go\x00M\x00internal/x_test.go\x00",
		existing: []string{"internal/x.go"},
	}
	res := scopedRun(f, "BASE", "internal/x.go")
	if res.Exit != 0 {
		t.Fatalf("a committed work unit that brought a test owes nothing: exit %d, %q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "a test was added or changed with it: internal/x_test.go") {
		t.Fatalf("the test the commit brought must be named: %q", res.Text)
	}
}

// The same commit without a test owes one: the range widens what changed, it does not absolve it.
func TestSinceStillOwesWhenTheCommitBroughtNoTest(t *testing.T) {
	f := &fakeRepo{
		root:     "/repo",
		status:   "",
		diff:     "M\x00internal/x.go\x00",
		existing: []string{"internal/x.go"},
	}
	res := scopedRun(f, "BASE", "internal/x.go")
	if res.Exit != 1 {
		t.Fatalf("a committed work unit with no test owes one: exit %d, %q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, noWitnessDebt) {
		t.Fatalf("the debt must be named: %q", res.Text)
	}
}

// A work unit that deleted a test did not bring one: the range carries statuses, and counting the
// path alone would report the opposite change as the evidence the verdict is looking for.
func TestSinceDoesNotCountADeletedTestAsTheTestItBrought(t *testing.T) {
	f := &fakeRepo{
		root:     "/repo",
		status:   "",
		diff:     "M\x00internal/x.go\x00D\x00internal/x_test.go\x00",
		existing: []string{"internal/x.go", "internal/x_test.go"},
	}
	res := scopedRun(f, "BASE", "internal/x.go")
	if res.Exit != 1 {
		t.Fatalf("a unit that removed its test owes one: exit %d, %q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, noWitnessDebt) {
		t.Fatalf("the debt must be named: %q", res.Text)
	}
}

// Without --since a clean tree keeps its old answer exactly: this is what every standalone caller,
// hook and CI run still gets, and no Gentle-only reading may leak into it.
func TestWithoutSinceACleanTreeStillAnswersNothingChanged(t *testing.T) {
	f := &fakeRepo{root: "/repo", status: "", existing: []string{"internal/x.go"}}
	res := scopedRun(f, "", "internal/x.go")
	if res.Exit != 0 {
		t.Fatalf("a clean tree owes nothing without --since: exit %d, %q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "no production source is changed: nothing to check") {
		t.Fatalf("the tree-only answer must stay byte for byte: %q", res.Text)
	}
}

// A --since that names nothing git can resolve is the caller's mistake, not a clean tree: answering
// "nothing to check" would hide the debt the caller asked about, so it is refused and named.
func TestSinceThatDoesNotResolveIsRefused(t *testing.T) {
	f := &fakeRepo{root: "/repo", status: "", sinceErr: errors.New("exit status 128")}
	res := scopedRun(f, "nope", "internal/x.go")
	if res.Exit != 2 {
		t.Fatalf("an unresolvable --since is refused: exit %d, %q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "nope") || !strings.Contains(res.Text, "not a commit") {
		t.Fatalf("the refusal must name the ref: %q", res.Text)
	}
}

// A range git cannot read is a failure to answer, never a verdict: the check says so instead of
// reporting a settled change it never read.
func TestSinceThatCannotBeReadFailsClosed(t *testing.T) {
	f := &fakeRepo{root: "/repo", status: "", diffErr: errors.New("boom")}
	res := scopedRun(f, "BASE", "internal/x.go")
	if res.Exit != 2 {
		t.Fatalf("an unreadable range fails closed: exit %d, %q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "could not be read") {
		t.Fatalf("the failure must say what could not be read: %q", res.Text)
	}
}
