package check

import (
	"strings"
	"testing"
)

// gateRun is a change that owes a test: one production source file changed, no test with it.
func gateRun(f *fakeRepo) Result {
	f.status = " M internal/x.go\x00"
	d := f.deps()
	d.PlanPath = ""
	d.Paths = []string{"internal/x.go"}
	return Run(f.root, d)
}

// A Gentle session runs the harness, so the debt it finds gates the run without asking: the question
// this replaces was answered by the repository and the runtime, never by the model.
func TestGentleSessionRequiresTheRun(t *testing.T) {
	f := &fakeRepo{root: "/repo", existing: []string{"internal/x.go"}, gentle: true}
	res := gateRun(f)
	if res.Exit != 1 || res.Gate != DecisionRequired {
		t.Fatalf("a Gentle session requires the run: exit %d gate %q, %q", res.Exit, res.Gate, res.Text)
	}
	if !strings.Contains(res.Reason, "verified Gentle signal") {
		t.Fatalf("the reason must name the signal that decided: %q", res.Reason)
	}
}

// Without that signal the same debt is the zone a human decides in: a repository used without Gentle
// keeps the behavior it already had, and nothing about the harness reaches it.
func TestStandaloneSessionAsks(t *testing.T) {
	f := &fakeRepo{root: "/repo", existing: []string{"internal/x.go"}, gentle: false}
	res := gateRun(f)
	if res.Exit != 1 || res.Gate != DecisionAsk {
		t.Fatalf("a standalone session asks: exit %d gate %q, %q", res.Exit, res.Gate, res.Text)
	}
}

// Declared levels are the repository's own answer and beat the runtime in both directions.
func TestDeclaredGateBeatsTheRuntimeSignal(t *testing.T) {
	tests := []struct {
		body   string
		gentle bool
		exit   int
		gate   string
	}{
		{body: `{"gate":"never"}`, gentle: true, exit: 0, gate: DecisionNone},
		{body: `{"gate":"never"}`, gentle: false, exit: 0, gate: DecisionNone},
		{body: `{"gate":"always"}`, gentle: false, exit: 1, gate: DecisionRequired},
		{body: `{"gate":"always"}`, gentle: true, exit: 1, gate: DecisionRequired},
		{body: `{"gate":"ask"}`, gentle: true, exit: 1, gate: DecisionAsk},
		{body: `{"gate":"ask"}`, gentle: false, exit: 1, gate: DecisionAsk},
	}
	for _, tc := range tests {
		t.Run(tc.body, func(t *testing.T) {
			f := &fakeRepo{root: "/repo", config: tc.body, existing: []string{"internal/x.go"}, gentle: tc.gentle}
			res := gateRun(f)
			if res.Exit != tc.exit || res.Gate != tc.gate {
				t.Fatalf("exit %d gate %q, want exit %d gate %q: %q", res.Exit, res.Gate, tc.exit, tc.gate, res.Text)
			}
		})
	}
}

// Never is not silence: the debt is still named, so the run's evidence is recorded while the
// repository decides on its own that it does not block on it.
func TestGateNeverStillNamesTheDebt(t *testing.T) {
	f := &fakeRepo{root: "/repo", config: `{"gate":"never"}`, existing: []string{"internal/x.go"}, gentle: true}
	res := gateRun(f)
	if !strings.Contains(res.Text, noWitnessDebt) {
		t.Fatalf("the debt must survive the decision: %q", res.Text)
	}
	if !strings.Contains(res.Text, "gate: none") {
		t.Fatalf("the answer must say why it does not gate: %q", res.Text)
	}
}

// A change that owes nothing carries no gate at all, whatever the repository declares.
func TestNoDebtCarriesNoGate(t *testing.T) {
	f := &fakeRepo{root: "/repo", config: `{"gate":"always"}`, status: ""}
	d := f.deps()
	d.PlanPath = ""
	res := Run(f.root, d)
	if res.Exit != 0 || res.Gate != DecisionNone {
		t.Fatalf("a settled tree carries no gate: exit %d gate %q", res.Exit, res.Gate)
	}
}

// An unreadable declaration is an error, never a debt: turning it into one would let a typo read as
// "this change owes a test" and, under never, hide the error entirely.
func TestUnreadableDeclarationIsNotADebt(t *testing.T) {
	f := &fakeRepo{root: "/repo", config: `{"gate":"sometimes"}`, existing: []string{"internal/x.go"}, gentle: true}
	res := gateRun(f)
	if res.Gate != "" {
		t.Fatalf("an unreadable declaration carries no gate: %q", res.Gate)
	}
	if !strings.Contains(res.Text, "could not be read") {
		t.Fatalf("the declaration error must be reported: %q", res.Text)
	}
	if res.Exit != 2 {
		t.Fatalf("a declaration that cannot be read fails closed, never as a debt: exit %d", res.Exit)
	}
}
