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
	"github.com/alesierraalta/tsp/internal/bench"
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
	// StartedFailing counts, per test that is not a finding, its failures that came after it was seen passing (the
	// first failure counts one), and Recovered marks the tests seen passing since: two means a fix made the test
	// pass and a later one broke it again.
	StartedFailing map[string]int  `json:"started_failing,omitempty"`
	Recovered      map[string]bool `json:"recovered,omitempty"`
	// Closed is set by done and stop. Final is the answer that closed the loop, repeated while the tree is the one
	// it judged; the next record over another tree opens a new loop.
	Closed bool    `json:"closed,omitempty"`
	Final  *Answer `json:"final,omitempty"`
}

// Round is one TSP pass of the loop.
type Round struct {
	N int `json:"n"`
	// Base is the code the round's tests were judged against: HEAD's commit for round 1, the pending tree later.
	Base string `json:"base"`
	// End is the working tree when the round was first recorded. A later record never moves it, because the code
	// the round attacked is what next compares the current source with.
	End string   `json:"end"`
	New []string `json:"new"`
	// Suites are every suite a record of this round ran: next reruns all of them.
	Suites []string  `json:"suites,omitempty"`
	At     time.Time `json:"recorded_at"`
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
	// Lines are the review's line for every finding, so a host can show the whole review beside the verdict.
	Lines []string `json:"lines"`
}

// Item is one thing a fix has to settle.
type Item struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Suite  string `json:"suite,omitempty"`
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

// roundBase retains the attacked base even when a duplicate record follows a terminal answer. A round without one is
// pinned to the commit its record judges, so work committed between records never silently becomes its base.
func roundBase(l *Loop, pin string) string {
	switch {
	case l == nil || len(l.Rounds) == 0:
		return pin
	case l.Pending != "":
		return l.Pending
	}
	return l.Rounds[len(l.Rounds)-1].Base
}

// alive is the loop a record or a next can still follow. Every snapshot it holds outside any ref must still answer
// from private storage, and a closed loop is no exception: without them a later record cannot judge the round it
// would continue, so it has to start a new loop instead of failing against a missing base.
func alive(top string, l *Loop) (*Loop, bool) {
	if l == nil {
		return l, true
	}
	gone := func() (*Loop, bool) {
		closed := *l
		closed.Closed = true
		return &closed, false
	}
	held := []string{l.Pending}
	for _, round := range l.Rounds {
		held = append(held, round.Base, round.End)
	}
	for _, object := range held {
		switch {
		case object == "", object == "HEAD":
			continue
		case lapsed(top, object):
			return gone()
		}
	}
	return l, true
}

// lapsed reports whether a recorded revision is no longer readable. A snapshot must answer from private storage;
// a round based on a commit only needs the repository to still hold that commit.
func lapsed(top, object string) bool {
	if kind, e := gitIn(top, nil, "cat-file", "-t", object); e == nil && kind == "commit" {
		return false
	}
	return bench.SnapshotAlive(top, object) != nil
}

// withSuite adds the suite a record ran to the ones its round already holds.
func withSuite(suites []string, suite string) []string {
	if suite == "" || slices.Contains(suites, suite) {
		return suites
	}
	return append(suites, suite)
}

// advance books the round a record belongs to: a new loop when none is open, the round a tsp answer opened, or
// the round already open, whose end stays where its first record put it so a later fix is never hidden from next.
func advance(l *Loop, commit, end, suite string, created []string) *Loop {
	now := time.Now().UTC()
	if !open(l) {
		return &Loop{Rounds: []Round{{N: 1, Base: commit, End: end, New: created, Suites: withSuite(nil, suite), At: now}}}
	}
	if l.Pending != "" {
		l.Rounds = append(l.Rounds, Round{N: len(l.Rounds) + 1, Base: l.Pending, End: l.Pending, New: created, Suites: withSuite(nil, suite), At: now})
		l.Pending = ""
		return l
	}
	last := &l.Rounds[len(l.Rounds)-1]
	last.New = append(last.New, created...)
	last.Suites = withSuite(last.Suites, suite)
	return l
}

// gitIn runs git in dir with extra environment and returns its standard output alone.
func gitIn(dir string, env []string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	private, e := bench.SnapshotEnv(dir)
	if e != nil {
		return "", e
	}
	c.Env = append(os.Environ(), append(private, env...)...)
	var stderr strings.Builder
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	if strings.ContainsRune(string(b), 0) {
		return string(b), nil
	}
	return strings.TrimSpace(string(b)), nil
}

func snapshot(top string) (string, error) { return bench.Snapshot(top) }

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
	if _, ok := alive(top, r.Loop); !ok {
		return failed(errors.New("the TSP loop's private snapshots are unavailable (removed state, or another clone at the same path): run the TSP flow, then `tsp findings record` to start a new loop"))
	}
	noRound := fmt.Errorf("no TSP round is open on %s: run the TSP flow, then `tsp findings record`", r.Branch)
	if r.Loop == nil || len(r.Loop.Rounds) == 0 {
		return failed(noRound)
	}
	snap, e := snapshot(top)
	if e != nil {
		return failed(e)
	}
	env, e := bench.SnapshotEnv(top)
	if e != nil {
		return failed(e)
	}
	diffGit := "env " + quoted(env) + " git --literal-pathspecs diff"
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
	var notes []string
	var settled []byte
	suites := make([]string, 0, len(r.Loop.Rounds))
	for _, round := range r.Loop.Rounds {
		suites = append(suites, round.Suites...)
	}
	j, e := reviewAll(dir, timeout, suites, func(latest *registry, runs map[string]suiteRun, j judged) error {
		if !sameLoop(latest.Loop, r.Loop) {
			return errMoved
		}
		if e := unchanged(top, snap); e != nil {
			return e
		}
		var items []Item
		latest.Loop.Closed = false
		items, notes = settle(latest, runs, top)
		a = decide(latest, items, notes, snap, changed, keep, nil, diffGit)
		conclude(latest.Loop, &a, j)
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
			if e := unchanged(top, snap); e != nil {
				return e
			}
			a = decide(latest, g.items, append(notes, g.notes...), snap, changed, keep, &g, diffGit)
			conclude(latest.Loop, &a, j)
			return nil
		})
		if e != nil {
			return failed(e)
		}
	}
	return a, exitOf(a.Next)
}

var errChanged = errors.New("the working tree changed while next judged it: run next again")

// unchanged refuses to answer for a tree that moved while next judged it: a suite or a person edited what the
// answer would be about, so the answer would describe code that no longer exists.
func unchanged(top, snap string) error {
	now, e := snapshot(top)
	if e != nil {
		return e
	}
	if now != snap {
		return errChanged
	}
	return nil
}

// conclude stamps the review an answer was judged with, and keeps a terminal answer so asking again over the same
// tree repeats it.
func conclude(l *Loop, a *Answer, j judged) {
	a.Review, a.Lines = j.summary, reviewLines(j.lines)
	if a.Next == NextDone || a.Next == NextStop {
		final := *a
		l.Final = &final
	}
}

// reviewLines splits a review's per-finding text into its lines.
func reviewLines(s string) []string {
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// endedOn reports a closed loop whose end judged this very tree: a record over it attacks nothing new, so the loop
// stays closed and its end stands.
func endedOn(l *Loop, tree string) bool {
	return l != nil && l.Closed && l.Final != nil && l.Final.Tree == tree
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
// test by test, and every test in the suites the loop ran that fails and is not a finding of that suite. A failure
// older than the loop counts too, because the loop cannot tell it from one the change caused. While a round is
// pending its tests are witnesses nobody has recorded yet, so failures are read again once the round is recorded.
// The notes name the witnesses the user kept failing with wontfix, so the end never reads as every test passing.
func settle(r *registry, runs map[string]suiteRun, top string) ([]Item, []string) {
	items := unsettled(r.Findings, top)
	l := r.Loop
	witnesses := map[string]map[string]bool{}
	var notes []string
	for _, f := range r.Findings {
		key := suiteKey(f.Suite)
		if witnesses[key] == nil {
			witnesses[key] = map[string]bool{}
		}
		if f.Status != "rejected" {
			witnesses[key][f.Test] = true
		} else if file, _, _ := relocate(top, f); file != "" {
			// Its explicit removal item already holds the loop; do not also call it an unrelated failure.
			witnesses[key][f.Test] = true
		}
		if passed, ran := runs[suiteKey(f.Suite)].tests[f.Test]; f.Status == "wontfix" && ran && !passed && f.Verdict != nil {
			notes = append(notes, fmt.Sprintf("kept failing by the user's wontfix: %s %s %s — %s", f.ID, f.File, f.Test, f.Verdict.Reason))
		}
	}
	suites := make([]string, 0, len(runs))
	for suite := range runs {
		suites = append(suites, suite)
	}
	sort.Strings(suites)
	for _, suite := range suites {
		run := runs[suite]
		if run.failed && !ranClean(run) {
			items = append(items, Item{Status: "suite", Test: run.suite, Detail: fmt.Sprintf("did not run test by test (exit %d)", run.code)})
			continue
		}
		for name, passed := range run.tests {
			if key := itemKey(Item{Status: "failing", Suite: suite, Test: name}); passed && l.StartedFailing[key] > 0 {
				if l.Recovered == nil {
					l.Recovered = map[string]bool{}
				}
				l.Recovered[key] = true
			}
		}
		for _, name := range failingTests(run, witnesses[suiteKey(run.suite)]) {
			items = append(items, Item{Status: "failing", Suite: run.suite, Test: name, Detail: "fails now and is not one of the loop's findings"})
		}
	}
	return items, notes
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
			if file, _, _ := relocate(top, f); file != "" {
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
func decide(r *registry, items []Item, notes []string, snap string, changed []string, keep bool, gates *gateReport, diffGit string) Answer {
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
		a.Message = fmt.Sprintf("round %d of %d of the TSP loop: the last fix changed %s. Attack that change and everything it affects (`%s %s %s -- %s`); do not re-attack code it did not touch. Then run `tsp findings record` and `tsp findings next`.",
			a.Round, MaxRounds, strings.Join(changed, ", "), diffGit, since, l.Pending, quoted(changed)) + noteLines(a.Notes)
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
		if it.Status != "failing" {
			continue
		}
		if l.StartedFailing == nil {
			l.StartedFailing = map[string]int{}
		}
		// A test counts as failing again only after it was seen passing: an answer in between that found its suite
		// not building proves nothing about it.
		switch {
		case l.StartedFailing[key] == 0:
			l.StartedFailing[key] = 1
		case l.Recovered[key]:
			l.StartedFailing[key]++
			delete(l.Recovered, key)
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

// quoted joins paths for a POSIX shell command, each one quoted, so a name with spaces or shell syntax is one word
// and runs nothing.
func quoted(paths []string) string {
	words := make([]string, len(paths))
	for i, p := range paths {
		words[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return strings.Join(words, " ")
}

// itemKey names an item across answers: a finding by its id and status, anything else by what it is about.
func itemKey(it Item) string {
	if it.ID != "" {
		return it.ID + ":" + it.Status
	}
	return it.Status + ":" + suiteKey(it.Suite) + ":" + it.File + ":" + it.Test
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
	return Answer{Next: NextError, MaxRounds: MaxRounds, Reason: e.Error(), Items: []Item{}, Rounds: []RoundCount{}, Message: e.Error(), Notes: []string{}, Lines: []string{}}, 2
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
