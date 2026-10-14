package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The reasons a run can lose a defect it was asked to catch, from the closest miss to the furthest.
const (
	MissSeenNoTest      = "seen_no_test"      // the run named the defect but left no test at all
	MissSeenTestsMissed = "seen_tests_missed" // the run named the defect and wrote tests, none of which distinguish it
	MissTestsBlind      = "tests_blind"       // the run wrote tests but never named the defect, and none distinguish it
	MissNotLooked       = "not_looked"        // the run neither named the defect nor wrote a test
	MissNotCheckable    = "not_checkable"     // the catch check never observed the defect's variant
)

// missOrder is the order the summary lists the reasons in, with the label a reader sees.
var missOrder = []struct{ class, label string }{
	{MissSeenNoTest, "seen, no test written"},
	{MissSeenTestsMissed, "seen, tests missed it"},
	{MissTestsBlind, "tests blind to it"},
	{MissNotLooked, "never looked"},
	{MissNotCheckable, "not checkable"},
}

// maxMissExamples bounds the runs each reason names in the summary.
const maxMissExamples = 5

// maxFinalMessage bounds the final answer a result keeps; a mention of a defect sits in its prose, not in a dump.
const maxFinalMessage = 8000

// Miss says why one run did not catch one defect. SeenIn names where the run showed it had found the
// defect: its plan's findings or its final message.
type Miss struct {
	Defect string `json:"defect"`
	Class  string `json:"class"`
	SeenIn string `json:"seen_in,omitempty"`
}

// Diagnose gives every defect a valid run did not catch one reason. It is mechanical: "seen" means the
// plan credited a finding to the defect, or the final message cites its location or names one of its
// keywords. Once every finding row is adjudicated the plan credits only confirmed defects: a row can match
// two defects by line or keyword, and the decision names the one it claims. A failed or invalid run, and a
// clean control, has no verdict to explain.
func Diagnose(r Result, key Key) []Miss {
	if r.Failed || r.Invalid || r.Control {
		return nil
	}
	decided := r.FindingRows > 0 && r.PendingAdjudication == 0 && r.AdjudicationComplete
	found := map[string]bool{}
	for _, d := range r.Defects {
		found[d.ID] = d.Found && (!decided || d.Confirmed)
	}
	wroteTests := len(r.Catch.TestFiles) > 0
	var misses []Miss
	for _, d := range key.Defects {
		caught, observed := r.Catch.Caught[d.ID]
		if caught {
			continue
		}
		m := Miss{Defect: d.ID}
		switch {
		case found[d.ID]:
			m.SeenIn = "plan"
		case mentions(r.FinalMessage, d):
			m.SeenIn = "final message"
		}
		switch {
		case !r.Catch.Checked || (wroteTests && !observed):
			m.Class = MissNotCheckable
		case m.SeenIn != "" && !wroteTests:
			m.Class = MissSeenNoTest
		case m.SeenIn != "":
			m.Class = MissSeenTestsMissed
		case wroteTests:
			m.Class = MissTestsBlind
		default:
			m.Class = MissNotLooked
		}
		misses = append(misses, m)
	}
	return misses
}

// mentions reports whether free text points at the defect: a citation of its file near its line, or one
// of its keywords. Agents describe a defect in prose more often than they cite it.
func mentions(text string, d Defect) bool {
	if text == "" {
		return false
	}
	if matchDefect(text, "", d).kind != "" {
		return true
	}
	lower := strings.ToLower(text)
	for _, kw := range d.Keywords {
		if kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// finalMessageFromLog reads a run's last answer back from its agent log, for results written before the
// final message was recorded. The log holds the raw stream of whichever runner ran, so each parser is
// tried; the others find no event of theirs and answer nothing.
func finalMessageFromLog(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "agent.log"))
	if err != nil {
		return ""
	}
	for _, parse := range []func(string) string{
		func(s string) string { return ParsePiStream(strings.NewReader(s)).Result },
		func(s string) string { return ParseCodexStream(strings.NewReader(s)).Result },
		func(s string) string { return ParseStream(strings.NewReader(s)).Result },
	} {
		if text := parse(string(data)); text != "" {
			return clipFinalMessage(text)
		}
	}
	return clipFinalMessage(lastAssistantText(string(data)))
}

// assistantTextRe is the first text block of an assistant message, as a JSON string literal.
var assistantTextRe = regexp.MustCompile(`"type":"text","text":("(?:[^"\\]|\\.)*")`)

// lastAssistantText reads the answer out of a log whose last event no parser can read: the log keeps only
// the tail of the stream, and Pi's closing event carries every message on one long line, so the tail
// usually starts in the middle of it. The answer is the first text block of the last assistant message.
func lastAssistantText(log string) string {
	i := strings.LastIndex(log, `"role":"assistant"`)
	if i < 0 {
		return ""
	}
	m := assistantTextRe.FindStringSubmatch(log[i:])
	if m == nil {
		return ""
	}
	var text string
	if json.Unmarshal([]byte(m[1]), &text) != nil {
		return ""
	}
	return text
}

func clipFinalMessage(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > maxFinalMessage {
		return text[:maxFinalMessage]
	}
	return text
}

// countMisses tallies the reasons across the valid runs.
func countMisses(cases []Result) map[string]int {
	var counts map[string]int
	for _, r := range cases {
		for _, m := range r.Misses {
			if counts == nil {
				counts = map[string]int{}
			}
			counts[m.Class]++
		}
	}
	return counts
}

// missSection renders why defects were missed: the count per reason with the runs that show it, then the
// test-quality signals that explain a "tests missed it". It is empty when there is nothing to explain.
func missSection(agg Aggregate) string {
	examples := map[string][]string{}
	redOnFix, inverted := 0, 0
	for _, r := range agg.Cases {
		for _, m := range r.Misses {
			if len(examples[m.Class]) < maxMissExamples {
				examples[m.Class] = append(examples[m.Class], fmt.Sprintf("%s/%d %s", r.Case, r.Run, m.Defect))
			}
		}
		if r.Catch.BrokenTests-r.Catch.InvertedTests > 0 {
			redOnFix++
		}
		if r.Catch.InvertedTests > 0 {
			inverted++
		}
	}
	if len(agg.Misses) == 0 && redOnFix == 0 && inverted == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Why defects were missed\n\n| reason | defect-runs | examples |\n|---|---|---|\n")
	for _, o := range missOrder {
		if n := agg.Misses[o.class]; n > 0 {
			fmt.Fprintf(&b, "| %s | %d | %s |\n", o.label, n, strings.Join(examples[o.class], ", "))
		}
	}
	fmt.Fprintf(&b, "\nruns with tests red on the reference fix: %d · runs with tests that pin the defective behaviour: %d\n", redOnFix, inverted)
	return b.String()
}
