package check

import (
	"strings"
	"testing"
)

// .no-testing-gate and a declared gate always are two answers to the same question, and they
// disagree. Settling that by silence is the worst outcome available: the repository that asked to
// be held says nothing, and the run it asked for never happens. A policy that contradicts itself
// is a policy the check cannot read, so it fails closed like an unreadable declaration.
func TestOptOutAndARequiredGateInTheSameRepositoryFailClosed(t *testing.T) {
	f := &fakeRepo{
		root:     "/repo",
		status:   " M internal/x.go\x00",
		existing: []string{"internal/x.go"},
		config:   `{"gate":"always"}`,
		optOut:   true,
	}

	res := scopedRun(f, "", "internal/x.go")
	if res.Exit != 2 {
		t.Fatalf("a repository that both opts out and requires the run has no single answer: exit=%d text=%q", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, optOutName) {
		t.Fatalf("the error must name the file that contradicts the declaration: %q", res.Text)
	}
}

// The same shape without a declared gate is a real opt-out and stays quiet: only the contradiction
// is an error.
func TestOptOutAloneStillAnswersNothingToDo(t *testing.T) {
	f := &fakeRepo{root: "/repo", status: " M internal/x.go\x00", existing: []string{"internal/x.go"}, optOut: true}

	res := scopedRun(f, "", "internal/x.go")
	if res.Exit != 0 {
		t.Fatalf("an opt-out nobody contradicted is an answer, not an error: exit=%d text=%q", res.Exit, res.Text)
	}
}
