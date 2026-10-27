package findings

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/gate"
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

const fixInstruction = "settle each item: when the contract promises what its test asserts, fix the source; when it does not, record `tsp findings verdict F<n> rejected --reason \"<why>\"` and remove that test. Never edit a witness test to make it pass and never record wontfix: that decision is the user's. Then run `tsp findings next`."

const userDecides = " — the loop needs the user's decision"

// Loop is the TSP loop on one branch: the rounds TSP ran, and what the last answer owed. It lives in the branch's
// registry, so whether a finding was already fixed is read from there and never from a conversation's memory.
type Loop struct {
	Rounds []Round `json:"rounds"`
	// Next is the last answer next gave.
	Next string `json:"next,omitempty"`
	// Pending is the tree the next round's tests are judged against, set when next answers tsp: a round after a
	// fix attacks the code that fix left, which no commit holds.
	Pending string `json:"pending,omitempty"`
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
// goes through a copy of the index, so it moves no ref, makes no commit and leaves the real index as it was.
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

// Next answers what the TSP loop on this branch owes now: fix what is unsettled, run a round over the source the
// last fix changed, stop and hand the decision to the user, or nothing more because the loop converged.
func Next(dir string, timeout time.Duration) (Answer, int) {
	r, _, top, e := load(dir)
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
	j, e := reviewAll(dir, timeout, func(latest *registry) error {
		if !reflect.DeepEqual(latest.Loop, r.Loop) {
			return errors.New("the TSP loop moved while it was judged: run next again")
		}
		a = decide(latest, snap, changed, keep)
		return nil
	})
	if e != nil {
		return failed(e)
	}
	a.Review = j.summary
	return a, exitOf(a.Next)
}

// decide picks the answer from a reviewed registry and books it in the registry's loop.
func decide(r *registry, snap string, changed []string, keep bool) Answer {
	l := r.Loop
	a := Answer{Round: len(l.Rounds), MaxRounds: MaxRounds, Items: unsettled(r.Findings), Rounds: counts(l)}
	switch {
	case len(a.Items) > 0:
		a.Next = NextFix
		a.Reason = fmt.Sprintf("round %d of %d left %s unsettled", a.Round, MaxRounds, plural(len(a.Items), "item"))
		a.Message = itemLines(a.Items) + fixInstruction
	case len(changed) > 0 && a.Round >= MaxRounds:
		a.Next, l.Closed = NextStop, true
		a.Reason = fmt.Sprintf("%d rounds reached: %s changed after round %d and no round has attacked that change", MaxRounds, strings.Join(changed, ", "), a.Round)
		a.Message = a.Reason + userDecides
	case len(changed) > 0:
		if !keep {
			l.Pending = snap
		}
		since := l.Rounds[len(l.Rounds)-1].End
		a.Next, a.Round = NextTSP, a.Round+1
		a.Scope = &Scope{Since: since, Base: l.Pending, Paths: changed}
		a.Reason = fmt.Sprintf("round %d of %d — the last fix changed %s: %s", a.Round, MaxRounds, plural(len(changed), "source file"), strings.Join(changed, ", "))
		a.Message = fmt.Sprintf("round %d of %d of the TSP loop: the last fix changed %s. Attack that change and everything it affects (`git diff %s %s -- %s`); do not re-attack code it did not touch. Then run `tsp findings record` and `tsp findings next`.",
			a.Round, MaxRounds, strings.Join(changed, ", "), since, l.Pending, strings.Join(changed, " "))
	default:
		a.Next, l.Closed = NextDone, true
		fixed, rejected, wontfix, closedLines := tally(r.Findings, l)
		a.Reason = fmt.Sprintf("converged after %s: %d fixed, %d rejected, %d wontfix", plural(a.Round, "round"), fixed, rejected, wontfix)
		a.Message = a.Reason + closedLines
	}
	l.Next = a.Next
	return a
}

// unsettled lists the findings a fix still owes: any status but fixed or a verdict.
func unsettled(fs []Finding) []Item {
	items := []Item{}
	for _, f := range fs {
		switch f.Status {
		case "fixed", "rejected", "wontfix":
			continue
		case "":
			f.Status = "open"
		}
		items = append(items, Item{ID: f.ID, Status: f.Status, File: f.File, Test: f.Test, Detail: f.Promise})
	}
	return items
}

func itemLines(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "%s %s %s %s", it.ID, it.Status, it.File, it.Test)
		if it.Detail != "" {
			fmt.Fprintf(&b, " — %s", it.Detail)
		}
		b.WriteString("\n")
	}
	return b.String()
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
	return Answer{Next: NextError, MaxRounds: MaxRounds, Reason: e.Error(), Items: []Item{}, Rounds: []RoundCount{}, Message: e.Error()}, 2
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
