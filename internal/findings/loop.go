package findings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/arch"
	"github.com/alesierraalta/tsp/internal/check"
	"github.com/alesierraalta/tsp/internal/gate"
	"github.com/alesierraalta/tsp/internal/quality"
)

// MaxRounds is how many TSP rounds one loop runs before it hands the decision to the user.
const MaxRounds = 5

// The loop's answers. A host reads them from `tsp findings next --json`.
const (
	NextFix   = "fix"
	NextTSP   = "tsp"
	NextDone  = "done"
	NextStop  = "stop"
	NextError = "error"
)

const userDecides = " — the loop needs the user's decision"

var errMoved = errors.New("the TSP loop moved while it was judged: run next again")

// Loop is the TSP loop on one branch: the rounds TSP ran, and what the last answer owed. It lives in the branch's
// registry, so whether a finding was already fixed is read from there and never from a conversation's memory.
type Loop struct {
	Rounds []Round `json:"rounds"`
	// Next is the last answer next gave.
	Next string `json:"next,omitempty"`
	// Pending is the tree the next round's tests are judged against, set when next answers tsp: a round after a
	// fix attacks the code that fix left, which no commit holds.
	Pending string `json:"pending,omitempty"`
	// FixTree is the working tree when the last fix answer was given, Unsettled what that answer listed, and
	// NoProgress how many fixes in a row since then left exactly the same items unsettled.
	FixTree    string   `json:"fix_tree,omitempty"`
	Unsettled  []string `json:"unsettled,omitempty"`
	NoProgress int      `json:"no_progress,omitempty"`
	// StartedFailing counts, per test that is not a finding, the answers at which it newly appeared among the
	// failing tests: twice means a fix made it pass and a later one broke it again.
	StartedFailing map[string]int `json:"started_failing,omitempty"`
	// Closed is set by done and stop; the next record opens a new loop.
	Closed bool `json:"closed,omitempty"`
}

// Round is one TSP pass of the loop.
type Round struct {
	N int `json:"n"`
	// Base is the code the round's tests were judged against: HEAD's commit for round 1, the pending tree later.
	Base string `json:"base"`
	// End is the working tree when the round was first recorded. A later record never moves it, because the code
	// the round attacked is what next compares the current source with.
	End   string    `json:"end"`
	New   []string  `json:"new"`
	Suite string    `json:"suite,omitempty"`
	At    time.Time `json:"recorded_at"`
}

// Answer is what the loop owes now, as next prints it.
type Answer struct {
	Next      string       `json:"next"`
	Round     int          `json:"round"`
	MaxRounds int          `json:"max_rounds"`
	Reason    string       `json:"reason"`
	Review    string       `json:"review"`
	Items     []Item       `json:"items"`
	Scope     *Scope       `json:"scope"`
	Rounds    []RoundCount `json:"rounds"`
	// Message is the instruction for the agent on fix and tsp, and the summary for the user otherwise.
	Message string `json:"message"`
	// Notes say what the answer did not hold against: the gates that could not measure.
	Notes []string `json:"notes"`
	// Tree is the working tree this answer judged, as a git tree id: the same tree and the same fix answer twice
	// means nothing changed in between.
	Tree string `json:"tree"`
}

// Item is one thing a fix has to settle.
type Item struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	File   string `json:"file"`
	Test   string `json:"test"`
	Detail string `json:"detail"`
}

// Scope is the change the next round attacks: the source that differs between the last round's end and base.
type Scope struct {
	Since string   `json:"since"`
	Base  string   `json:"base"`
	Paths []string `json:"paths"`
}

// RoundCount names the findings each round recorded first.
type RoundCount struct {
	N   int      `json:"n"`
	New []string `json:"new"`
}

func open(l *Loop) bool { return l != nil && !l.Closed && len(l.Rounds) > 0 }

// roundBase is the code a record judges its tests against: the pending tree once next has answered tsp, HEAD
// otherwise, so round 1 reads exactly as a record outside any loop always has.
func roundBase(l *Loop) string {
	if open(l) && l.Pending != "" {
		return l.Pending
	}
	return "HEAD"
}

// advance books the round a record belongs to: a new loop when none is open, the round a tsp answer opened, or
// the round already open, whose end stays where its first record put it so a later fix is never hidden from next.
func advance(l *Loop, commit, end, suite string, created []string) *Loop {
	now := time.Now().UTC()
	if !open(l) {
		return &Loop{Rounds: []Round{{N: 1, Base: commit, End: end, New: created, Suite: suite, At: now}}}
	}
	if l.Pending != "" {
		l.Rounds = append(l.Rounds, Round{N: len(l.Rounds) + 1, Base: l.Pending, End: end, New: created, Suite: suite, At: now})
		l.Pending = ""
		return l
	}
	last := &l.Rounds[len(l.Rounds)-1]
	last.New = append(last.New, created...)
	if last.Suite == "" {
		last.Suite = suite
	}
	return l
}

// gitIn runs git in dir with extra environment and returns its standard output alone.
func gitIn(dir string, env []string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), env...)
	var stderr strings.Builder
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(b)), nil
}

// snapshot writes the working tree as a git tree object, untracked files included and ignored ones left out. It
// goes through a copy of the index, so it moves no ref, makes no commit and leaves the real index as it was; the
// objects it writes are unreferenced, and git's own garbage collection removes them.
func snapshot(top string) (string, error) {
	index, e := gitIn(top, nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp("", "tsp-snapshot-")
	if e != nil {
		return "", e
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)
	data, e := os.ReadFile(index)
	switch {
	case e == nil:
		e = os.WriteFile(name, data, 0600)
	case errors.Is(e, os.ErrNotExist):
		// No index yet: git starts one of its own, but not from an empty file.
		e = os.Remove(name)
	}
	if e != nil {
		return "", e
	}
	env := []string{"GIT_INDEX_FILE=" + name}
	if _, e = gitIn(top, env, "add", "-A"); e != nil {
		return "", e
	}
	return gitIn(top, env, "write-tree")
}

// changedSource names the production source that differs between two trees, as `tsp check` counts it.
func changedSource(top, from, to string) ([]string, error) {
	if from == to {
		return nil, nil
	}
	out, e := gitIn(top, nil, "diff", "--name-only", "-z", "--no-renames", from, to, "--")
	if e != nil {
		return nil, e
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" && gate.IsProductionSource(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// sameLoop compares two loops as the registry stores them.
func sameLoop(a, b *Loop) bool {
	x, ex := json.Marshal(a)
	y, ey := json.Marshal(b)
	return ex == nil && ey == nil && bytes.Equal(x, y)
}

// Next answers what the TSP loop on this branch owes now: fix what is unsettled, run a round over the source the
// last fix changed, stop and hand the decision to the user, or nothing more because the loop converged.
func Next(dir string, timeout time.Duration) (Answer, int) {
	r, path, top, e := load(dir)
	if e != nil {
		return failed(e)
	}
	if !open(r.Loop) {
		return failed(fmt.Errorf("no TSP round is open on %s: run the TSP flow, then `tsp findings record`", r.Branch))
	}
	snap, e := snapshot(top)
	if e != nil {
		return failed(e)
	}
	changed, e := changedSource(top, r.Loop.Rounds[len(r.Loop.Rounds)-1].End, snap)
	if e != nil {
		return failed(e)
	}
	// Asking again after a tsp answer keeps its base while the source still matches it: the round's own tests may
	// already sit in the working tree, and a base that swallowed them would leave the round nothing to judge.
	keep := false
	if r.Loop.Pending != "" {
		moved, e := changedSource(top, r.Loop.Pending, snap)
		if e != nil {
			return failed(e)
		}
		keep = len(moved) == 0
	}
	var a Answer
	var settled []byte
	suites := make([]string, 0, len(r.Loop.Rounds))
	for _, round := range r.Loop.Rounds {
		suites = append(suites, round.Suite)
	}
	j, e := reviewAll(dir, timeout, suites, func(latest *registry, runs map[string]suiteRun) error {
		if !sameLoop(latest.Loop, r.Loop) {
			return errMoved
		}
		a = decide(latest, settle(latest, runs, top), nil, snap, changed, keep, nil)
		settled, _ = json.Marshal(latest.Loop)
		return nil
	})
	if e != nil {
		return failed(e)
	}
	if a.Next == "" {
		// Nothing is unsettled and the last round attacked this very code: the gates over the loop's whole change
		// decide whether it converged. They run outside the registry's lock, because they run work of their own.
		g := runGates(top, r.Loop.Rounds[0].Base, snap)
		e = saveAmended(dir, path, func(latest *registry) error {
			if now, _ := json.Marshal(latest.Loop); !bytes.Equal(now, settled) {
				return errMoved
			}
			a = decide(latest, g.items, g.notes, snap, changed, keep, &g)
			return nil
		})
		if e != nil {
			return failed(e)
		}
	}
	a.Review = j.summary
	return a, exitOf(a.Next)
}

// failingTests names the tests a run saw fail, outside the given witnesses.
func failingTests(run suiteRun, witnesses map[string]bool) []string {
	failing := []string{}
	for name, passed := range run.tests {
		if !passed && !witnesses[name] {
			failing = append(failing, name)
		}
	}
	sort.Strings(failing)
	return failing
}

// ranClean reports a run that exited cleanly with no failing test, a suite with no test at all included: nothing
// in it is broken, even when it gave no per-test report.
func ranClean(run suiteRun) bool { return run.code == 0 && len(failingTests(run, nil)) == 0 }

// settle lists what a fix still owes: the findings not yet fixed or closed by a verdict, every suite that did not run
// test by test, and every test in the suites the loop ran that fails and is not a finding. A failure older than the
// loop counts too, because the loop cannot tell it from one the change caused. While a round is pending its tests
// are witnesses nobody has recorded yet, so failures are read again once the round is recorded.
func settle(r *registry, runs map[string]suiteRun, top string) []Item {
	items := unsettled(r.Findings, top)
	witnesses := map[string]bool{}
	for _, f := range r.Findings {
		witnesses[f.Test] = true
	}
	suites := make([]string, 0, len(runs))
	for suite := range runs {
		suites = append(suites, suite)
	}
	sort.Strings(suites)
	for _, suite := range suites {
		run := runs[suite]
		if run.failed && !ranClean(run) {
			items = append(items, Item{Status: "suite", Test: suite, Detail: fmt.Sprintf("did not run test by test (exit %d)", run.code)})
			continue
		}
		if r.Loop.Pending != "" {
			continue
		}
		for _, name := range failingTests(run, witnesses) {
			items = append(items, Item{Status: "failing", Test: name, Detail: "fails now and is not one of the loop's findings"})
		}
	}
	return items
}

// unsettled lists the findings a fix still owes: any status but fixed or a verdict, and a rejected finding whose test
// is still in the suite, since rejecting an invented test ends with removing it.
func unsettled(fs []Finding, top string) []Item {
	items := []Item{}
	for _, f := range fs {
		switch f.Status {
		case "fixed", "wontfix":
			continue
		case "rejected":
			if file, _, _ := locate(top, f.Test, []string{f.File}); file != "" {
				items = append(items, Item{ID: f.ID, Status: f.Status, File: f.File, Test: f.Test, Detail: "rejected, but its test is still in the suite: remove it"})
			}
			continue
		case "":
			f.Status = "open"
		}
		items = append(items, Item{ID: f.ID, Status: f.Status, File: f.File, Test: f.Test, Detail: f.Promise})
	}
	return items
}

// gateReport is what the gates over the loop's whole change found, and what they could not measure.
type gateReport struct {
	items []Item
	notes []string
}

// runGates holds the loop's whole change, from round 1's base to the current tree, to the repository's own
// standards: the test the check owes changed source, the complexity and duplication the quality probe measures,
// and the import edges the architecture declaration refuses. A gate that cannot measure is named, never passed.
func runGates(top, base, snap string) gateReport {
	var g gateReport
	notMeasured := func(gate string, e error) {
		g.notes = append(g.notes, fmt.Sprintf("not measured: %s: %v", gate, e))
	}
	paths, e := changedSource(top, base, snap)
	switch {
	case e != nil:
		notMeasured("tsp check", e)
	case len(paths) > 0:
		res := check.Run(top, check.Deps{Paths: paths, Since: base})
		switch res.Exit {
		case 0:
		case 1:
			g.items = append(g.items, Item{Status: "check", Detail: check.DebtLine(res.Text)})
		default:
			notMeasured("tsp check", errors.New(strings.TrimSpace(res.Reason+" "+check.DebtLine(res.Text))))
		}
	}
	if q, e := quality.Run(context.Background(), quality.Options{Cwd: top, Base: base}); e != nil {
		notMeasured("probe quality", e)
	} else {
		for _, f := range q.Findings {
			g.items = append(g.items, Item{Status: "quality", File: f.File, Detail: fmt.Sprintf("line %d: %s", f.Line, f.Message)})
		}
	}
	if r, e := arch.Run(context.Background(), arch.Options{Cwd: top, Base: base}); e != nil {
		notMeasured("probe arch", e)
	} else {
		for _, f := range r.Findings {
			g.items = append(g.items, Item{Status: "arch", File: f.File, Detail: fmt.Sprintf("line %d: %s", f.Line, f.Message)})
		}
	}
	return g
}

// decide picks the answer and books it in the registry's loop. With nothing unsettled and nothing changed since the
// last round it answers nothing until the gates have run (gates is nil): only they can call the loop converged.
func decide(r *registry, items []Item, notes []string, snap string, changed []string, keep bool, gates *gateReport) Answer {
	l := r.Loop
	a := Answer{Round: len(l.Rounds), MaxRounds: MaxRounds, Items: items, Rounds: counts(l), Notes: append([]string{}, notes...), Tree: snap}
	switch {
	case len(items) > 0:
		a = settleOrStop(r, a, snap)
	case len(changed) > 0 && a.Round >= MaxRounds:
		a.Next, l.Closed = NextStop, true
		a.Reason = fmt.Sprintf("%d rounds reached: %s changed after round %d and no round has attacked that change", MaxRounds, strings.Join(changed, ", "), a.Round)
		a.Message = a.Reason + userDecides + noteLines(a.Notes)
	case len(changed) > 0:
		if !keep {
			l.Pending = snap
		}
		l.FixTree, l.Unsettled, l.NoProgress = "", nil, 0
		since := l.Rounds[len(l.Rounds)-1].End
		a.Next, a.Round = NextTSP, a.Round+1
		a.Scope = &Scope{Since: since, Base: l.Pending, Paths: changed}
		a.Reason = fmt.Sprintf("round %d of %d — the last fix changed %s: %s", a.Round, MaxRounds, plural(len(changed), "source file"), strings.Join(changed, ", "))
		a.Message = fmt.Sprintf("round %d of %d of the TSP loop: the last fix changed %s. Attack that change and everything it affects (`git diff %s %s -- %s`); do not re-attack code it did not touch. Then run `tsp findings record` and `tsp findings next`.",
			a.Round, MaxRounds, strings.Join(changed, ", "), since, l.Pending, strings.Join(changed, " ")) + noteLines(a.Notes)
	case gates == nil:
		return Answer{}
	default:
		a.Next, l.Closed = NextDone, true
		fixed, rejected, wontfix, closedLines := tally(r.Findings, l)
		a.Reason = fmt.Sprintf("converged after %s: %d fixed, %d rejected, %d wontfix", plural(a.Round, "round"), fixed, rejected, wontfix)
		a.Message = a.Reason + closedLines + noteLines(a.Notes)
	}
	l.Next = a.Next
	return a
}

// settleOrStop answers fix, unless the fixes have stopped converging: a finding or a test they undid twice, or
// two fixes in a row that left exactly the same items unsettled. Asking again over the same tree is not a fix.
func settleOrStop(r *registry, a Answer, snap string) Answer {
	l := r.Loop
	keys := make([]string, 0, len(a.Items))
	for _, it := range a.Items {
		key := itemKey(it)
		keys = append(keys, key)
		if it.Status == "failing" && !slices.Contains(l.Unsettled, key) {
			if l.StartedFailing == nil {
				l.StartedFailing = map[string]int{}
			}
			l.StartedFailing[key]++
		}
	}
	switch {
	case l.Next == NextFix && slices.Equal(keys, l.Unsettled) && snap != l.FixTree:
		l.NoProgress++
	case !slices.Equal(keys, l.Unsettled):
		l.NoProgress = 0
	}
	l.Unsettled, l.FixTree = keys, snap
	undone := ""
	for _, it := range a.Items {
		if f := findingByID(r.Findings, it.ID); f != nil && f.Status == "regressed" && f.Regressions >= 2 {
			undone = it.ID + " regressed twice"
		} else if it.Status == "failing" && l.StartedFailing[itemKey(it)] >= 2 {
			undone = it.Test + " failed again after a fix had made it pass"
		}
		if undone != "" {
			break
		}
	}
	switch {
	case undone != "":
		a.Next, l.Closed = NextStop, true
		a.Reason = undone + ": the fixes keep undoing each other"
	case l.NoProgress >= 2:
		a.Next, l.Closed = NextStop, true
		a.Reason = fmt.Sprintf("no progress: %s stayed unsettled after two fixes", plural(len(a.Items), "item"))
	default:
		a.Next = NextFix
		a.Reason = fmt.Sprintf("round %d of %d left %s unsettled", a.Round, MaxRounds, plural(len(a.Items), "item"))
		a.Message = itemLines(a.Items) + fixInstruction(a.Items) + noteLines(a.Notes)
		return a
	}
	a.Message = a.Reason + userDecides + "\n" + strings.TrimSuffix(itemLines(a.Items), "\n") + noteLines(a.Notes)
	return a
}

// fixInstruction says how to settle the items listed, naming only the kinds of item the list holds.
func fixInstruction(items []Item) string {
	var failing, suite, gated bool
	for _, it := range items {
		switch it.Status {
		case "failing":
			failing = true
		case "suite":
			suite = true
		case "check", "quality", "arch":
			gated = true
		}
	}
	var b strings.Builder
	b.WriteString("settle each item: when the contract promises what its test asserts, fix the source; when it does not, record `tsp findings verdict F<n> rejected --reason \"<why>\"` and remove that test.")
	if suite {
		b.WriteString(" A suite that did not run test by test is fixed first: nothing it holds can be judged until it does.")
	}
	if failing {
		b.WriteString(" A failing test is not a finding, and the loop does not converge while it fails: fix the source that makes it fail. Never edit or remove that test; when it cannot pass otherwise, stop and say so, because that decision is the user's.")
	}
	if gated {
		b.WriteString(" A check, quality or arch item is fixed in the code or the tests it names.")
	}
	b.WriteString(" Never edit a witness test to make it pass, and never record wontfix, a check waiver or .no-testing-gate: those decisions are the user's. Then run `tsp findings next`.")
	return b.String()
}

// itemKey names an item across answers: a finding by its id and status, anything else by what it is about.
func itemKey(it Item) string {
	if it.ID != "" {
		return it.ID + ":" + it.Status
	}
	return it.Status + ":" + it.File + ":" + it.Test
}

func findingByID(fs []Finding, id string) *Finding {
	for i := range fs {
		if id != "" && fs[i].ID == id {
			return &fs[i]
		}
	}
	return nil
}

func itemLines(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		var fields []string
		for _, f := range []string{it.ID, it.Status, it.File, it.Test} {
			if f != "" {
				fields = append(fields, f)
			}
		}
		b.WriteString(strings.Join(fields, " "))
		if it.Detail != "" {
			fmt.Fprintf(&b, " — %s", it.Detail)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func noteLines(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return "\n" + strings.Join(notes, "\n")
}

func counts(l *Loop) []RoundCount {
	out := make([]RoundCount, 0, len(l.Rounds))
	for _, round := range l.Rounds {
		out = append(out, RoundCount{N: round.N, New: append([]string{}, round.New...)})
	}
	return out
}

// tally counts how this loop's findings ended and lists the ones a verdict closed, with the reason.
func tally(fs []Finding, l *Loop) (fixed, rejected, wontfix int, closedLines string) {
	mine := map[string]bool{}
	for _, round := range l.Rounds {
		for _, id := range round.New {
			mine[id] = true
		}
	}
	var b strings.Builder
	for _, f := range fs {
		if !mine[f.ID] {
			continue
		}
		switch f.Status {
		case "fixed":
			fixed++
		case "rejected", "wontfix":
			if f.Status == "rejected" {
				rejected++
			} else {
				wontfix++
			}
			fmt.Fprintf(&b, "\n%s %s %s %s", f.ID, f.Status, f.File, f.Test)
			if f.Verdict != nil && f.Verdict.Reason != "" {
				fmt.Fprintf(&b, " — %s", f.Verdict.Reason)
			}
		}
	}
	return fixed, rejected, wontfix, b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func exitOf(next string) int {
	switch next {
	case NextDone:
		return 0
	case NextFix, NextTSP:
		return 1
	case NextStop:
		return 3
	}
	return 2
}

func failed(e error) (Answer, int) {
	return Answer{Next: NextError, MaxRounds: MaxRounds, Reason: e.Error(), Items: []Item{}, Rounds: []RoundCount{}, Message: e.Error(), Notes: []string{}}, 2
}

// Text is the answer for a reader: the verdict line, the review, then the items and what to do with them.
func (a Answer) Text() string {
	head, body := a.Reason, a.Message
	if a.Next != NextFix && a.Next != NextTSP {
		head, body, _ = strings.Cut(a.Message, "\n")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", a.Next, head)
	if a.Review != "" {
		b.WriteString(a.Review + "\n")
	}
	if body != "" {
		b.WriteString(strings.TrimSuffix(body, "\n") + "\n")
	}
	return b.String()
}
