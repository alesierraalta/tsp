package bench

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func diagnoseKey() Key {
	return Key{ID: "case-a", Defects: []Defect{
		{ID: "D1", File: "src/a.js", Line: 10, Keywords: []string{"rounding"}},
		{ID: "D2", File: "src/a.js", Line: 20, Keywords: []string{"thousands separator"}},
		{ID: "D3", File: "src/b.js", Line: 5, Keywords: []string{"timezone"}},
	}}
}

// Every defect a run did not catch gets one reason, so a reading says where the run lost it: it saw the
// defect and left no test, it wrote tests that missed what it saw, its tests were blind to a defect it
// never saw, or it never looked at all.
func TestDiagnoseNamesWhyEachDefectWasMissed(t *testing.T) {
	key := diagnoseKey()
	cases := []struct {
		name string
		res  Result
		want []Miss
	}{
		{
			name: "no test: a defect in the plan and one named only in the final message were seen, the third was not",
			res: Result{
				Defects:      []DefectResult{{ID: "D1", Found: true}, {ID: "D2"}, {ID: "D3"}},
				FinalMessage: "The parser stops at the comma, so a thousands separator drops the amount (src/a.js:21).",
				Catch:        CatchResult{Checked: true, Caught: map[string]bool{}},
			},
			want: []Miss{
				{Defect: "D1", Class: MissSeenNoTest, SeenIn: "plan"},
				{Defect: "D2", Class: MissSeenNoTest, SeenIn: "final message"},
				{Defect: "D3", Class: MissNotLooked},
			},
		},
		{
			name: "tests written: a caught defect is no miss, a seen one the tests missed, an unseen one they were blind to",
			res: Result{
				Defects: []DefectResult{{ID: "D1"}, {ID: "D2", Found: true}, {ID: "D3"}},
				Catch: CatchResult{Checked: true, TestFiles: []string{"tests/a.test.js"},
					Caught: map[string]bool{"D1": true, "D2": false, "D3": false}},
			},
			want: []Miss{
				{Defect: "D2", Class: MissSeenTestsMissed, SeenIn: "plan"},
				{Defect: "D3", Class: MissTestsBlind},
			},
		},
		{
			// A row can match two defects mechanically; adjudication decides which one it claims. Once every row
			// is decided, only a confirmed defect was seen in the plan: the other match was the scorer's guess.
			name: "an adjudicated plan credits only the defect its rows were decided for",
			res: Result{
				Defects:              []DefectResult{{ID: "D1", Found: true, Confirmed: true}, {ID: "D2", Found: true}, {ID: "D3"}},
				FindingRows:          1,
				AdjudicationComplete: true,
				Catch: CatchResult{Checked: true, TestFiles: []string{"tests/a.test.js"},
					Caught: map[string]bool{"D1": true, "D2": false, "D3": false}},
			},
			want: []Miss{
				{Defect: "D2", Class: MissTestsBlind},
				{Defect: "D3", Class: MissTestsBlind},
			},
		},
		{
			name: "a plan still pending adjudication keeps the mechanical credit",
			res: Result{
				Defects:             []DefectResult{{ID: "D1", Found: true}, {ID: "D2", Found: true}, {ID: "D3"}},
				FindingRows:         1,
				PendingAdjudication: 1,
				Catch: CatchResult{Checked: true, TestFiles: []string{"tests/a.test.js"},
					Caught: map[string]bool{"D1": true, "D2": false, "D3": false}},
			},
			want: []Miss{
				{Defect: "D2", Class: MissSeenTestsMissed, SeenIn: "plan"},
				{Defect: "D3", Class: MissTestsBlind},
			},
		},
		{
			// A test green while the defect stands and red once it is fixed is not blind: it distinguishes the
			// defect in the wrong direction, defending it. Folding it into tests_blind hides an oracle mistake.
			name: "a test that pins the defective behaviour is inverted, not blind",
			res: Result{
				Defects: []DefectResult{{ID: "D1"}, {ID: "D2", Found: true}, {ID: "D3"}},
				Catch: CatchResult{Checked: true, TestFiles: []string{"tests/a.test.js"},
					Caught:     map[string]bool{"D1": false, "D2": false, "D3": true},
					InvertedBy: map[string][]string{"D1": {"pins it"}, "D2": {"pins it too"}}},
			},
			want: []Miss{
				{Defect: "D1", Class: MissTestsInverted},
				{Defect: "D2", Class: MissTestsInverted, SeenIn: "plan"},
			},
		},
		{
			name: "a defect the catch check never observed is not checkable",
			res: Result{
				Defects: []DefectResult{{ID: "D1"}, {ID: "D2"}, {ID: "D3"}},
				Catch: CatchResult{Checked: true, TestFiles: []string{"tests/a.test.js"},
					Caught: map[string]bool{"D1": true, "D2": true}},
			},
			want: []Miss{{Defect: "D3", Class: MissNotCheckable}},
		},
		{
			name: "a failed run has no verdict to explain",
			res:  Result{Failed: true, Defects: []DefectResult{{ID: "D1"}}},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Diagnose(tc.res, key); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Diagnose = %+v\nwant      %+v", got, tc.want)
			}
		})
	}
}

// The summary turns the per-run reasons into a section a reader can act on: how many defect-runs were
// lost to each reason, with the runs that show it.
func TestSummaryExplainsWhyDefectsWereMissed(t *testing.T) {
	key := diagnoseKey()
	runs := []Result{
		{Case: "case-a", Run: 1, Total: 3, Defects: []DefectResult{{ID: "D1", Found: true}, {ID: "D2"}, {ID: "D3"}},
			Catch: CatchResult{Checked: true, Caught: map[string]bool{}}},
		{Case: "case-a", Run: 2, Total: 3, Defects: []DefectResult{{ID: "D1"}, {ID: "D2", Found: true}, {ID: "D3"}},
			Catch: CatchResult{Checked: true, TestFiles: []string{"tests/a.test.js"}, BrokenTests: 1,
				Caught: map[string]bool{"D1": true, "D2": false, "D3": false}}},
	}
	for i := range runs {
		runs[i].Misses = Diagnose(runs[i], key)
	}
	agg := Aggregate{Cases: runs}
	finalizeAggregate(&agg)
	if want := map[string]int{MissSeenNoTest: 1, MissNotLooked: 2, MissSeenTestsMissed: 1, MissTestsBlind: 1}; !reflect.DeepEqual(agg.Misses, want) {
		t.Fatalf("aggregate misses = %v, want %v", agg.Misses, want)
	}
	got := Summary(agg)
	for _, want := range []string{
		"## Why defects were missed",
		"| seen, no test written | 1 | case-a/1 D1 |",
		"| seen, tests missed it | 1 | case-a/2 D2 |",
		"| tests blind to it | 1 | case-a/2 D3 |",
		"| never looked | 2 | case-a/1 D2, case-a/1 D3 |",
		"runs with tests red on the reference fix: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
}

// A result written before the final message was recorded still has its agent log: rescoring reads the
// last answer back from it whichever runner wrote it.
func TestFinalMessageFromLogReadsTheLastAnswer(t *testing.T) {
	dir := t.TempDir()
	log := "attempt 1 exit_code=0 is_error=false timed_out=false err=<nil>\n" +
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}` + "\n" +
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"the parser drops the comma"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "agent.log"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := finalMessageFromLog(dir); got != "the parser drops the comma" {
		t.Fatalf("final message = %q", got)
	}
	// The log keeps only the tail of the stream, so a Pi run's last event usually starts mid-line and no
	// parser can read it: the answer is still the first text of the last assistant message in that tail.
	truncated := t.TempDir()
	tail := "attempt 1 exit_code=0 is_error=false timed_out=false err=<nil>\n" +
		`ed\n"}]},{"role":"toolResult","content":[{"type":"text","text":"feedback is disabled"}]},` +
		`{"role":"assistant","content":[{"type":"thinking","thinking":"done"},{"type":"text","text":"the comma drops \"1,234\""}],"stopReason":"stop"}],"willRetry":false}` + "\n" +
		`{"type":"agent_settled","aborted":false}` + "\n"
	if err := os.WriteFile(filepath.Join(truncated, "agent.log"), []byte(tail), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := finalMessageFromLog(truncated); got != `the comma drops "1,234"` {
		t.Fatalf("final message from a truncated tail = %q", got)
	}
	if got := finalMessageFromLog(t.TempDir()); got != "" {
		t.Fatalf("a run without a log has no final message, got %q", got)
	}
}
