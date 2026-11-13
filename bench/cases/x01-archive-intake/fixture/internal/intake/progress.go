package intake

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"example.com/archive/internal/clock"
	"example.com/archive/internal/textutil"
)

// Stage names the pipeline step a progress record belongs to.
type Stage string

// The stages, in the order the pipeline runs them.
const (
	StageStage   Stage = "stage"
	StageScan    Stage = "scan"
	StageExtract Stage = "extract"
	StageCommit  Stage = "commit"
	StageReport  Stage = "report"
)

// Progress records what one batch did and how long each stage took. The admin page prints it, and the
// report page writes it into the import summary.
type Progress struct {
	mu     sync.Mutex
	batch  string
	start  time.Time
	events []Event
}

// Event is one finished stage of one entry.
type Event struct {
	Stage   Stage
	Entry   string
	At      time.Time
	Elapsed time.Duration
	Detail  string
	Err     string
}

// NewProgress starts a record for one batch.
func NewProgress(batch string, now time.Time) *Progress {
	return &Progress{batch: batch, start: now.UTC()}
}

// Add appends one event.
func (p *Progress) Add(event Event) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	p.events = append(p.events, event)
}

// Events returns a copy of the recorded events, oldest first.
func (p *Progress) Events() []Event {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Event, len(p.events))
	copy(out, p.events)
	return out
}

// Elapsed is how long the batch has been running.
func (p *Progress) Elapsed(now time.Time) time.Duration {
	if p == nil {
		return 0
	}
	return now.UTC().Sub(p.start)
}

// StageTotals sums the recorded time per stage, in stage order.
func (p *Progress) StageTotals() []StageTotal {
	if p == nil {
		return nil
	}
	totals := map[Stage]StageTotal{}
	for _, event := range p.Events() {
		total := totals[event.Stage]
		total.Stage = event.Stage
		total.Events++
		total.Elapsed += event.Elapsed
		if event.Err != "" {
			total.Failed++
		}
		totals[event.Stage] = total
	}
	out := make([]StageTotal, 0, len(totals))
	for _, total := range totals {
		out = append(out, total)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stage < out[j].Stage })
	return out
}

// StageTotal is one stage's summed time and event counts.
type StageTotal struct {
	Stage   Stage
	Events  int
	Failed  int
	Elapsed time.Duration
}

// Describe renders one event as the line the admin page prints.
func (e Event) Describe() string {
	return textutil.Sentence(
		string(e.Stage),
		e.Entry,
		clock.Stamp(e.At),
		e.Elapsed.Round(time.Millisecond).String(),
		e.Detail,
		e.Err,
	)
}

// Summary is the human line the import report prints above the entry table: how many entries the
// batch carried, how many committed, and how long the run took.
func (r Result) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d entries, %d pages, %d bytes", len(r.Entries), r.Pages, r.Bytes)
	if failed := r.Failed(); failed > 0 {
		fmt.Fprintf(&b, ", %d failed", failed)
	}
	if r.Committed {
		b.WriteString(", committed")
	} else {
		b.WriteString(", not committed")
	}
	if r.Started != "" && r.Finished != "" {
		if start, err := clock.ParseRFC3339(r.Started); err == nil {
			if end, err := clock.ParseRFC3339(r.Finished); err == nil {
				fmt.Fprintf(&b, " in %s", end.Sub(start).Round(time.Millisecond))
			}
		}
	}
	return b.String()
}
