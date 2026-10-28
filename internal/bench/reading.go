package bench

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

const planlessNote = "note: - means plan-only metrics are unavailable when no plan was recorded; caught remains meaningful."

// RenderHistory masks plan-only cells for plan-less readings without changing stored rows.
func RenderHistory(text string) string {
	lines := strings.Split(text, "\n")
	var headers []string
	masked := false
	for i, line := range lines {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		if strings.Contains(line, "| cases |") {
			headers = cells
			continue
		}
		if len(cells) != len(headers) {
			continue
		}
		values := map[string]int{}
		for j, h := range headers {
			if name := strings.TrimSpace(h); (name == "no plan" || name == "no_plan") && strings.TrimSpace(cells[j]) == "-" {
				masked = true // stored already masked by AppendHistory
			}
			n, _ := strconv.Atoi(strings.TrimSpace(cells[j]))
			values[strings.TrimSpace(h)] = n
		}
		noPlan := values["no plan"] + values["no_plan"]
		runs := values["runs"]
		if runs < 1 {
			runs = 1
		}
		if noPlan == 0 || noPlan+values["failed"]+values["invalid"] != values["cases"]*runs {
			continue
		}
		for j, h := range headers {
			switch strings.TrimSpace(h) {
			case "reported", "found", "recall", "false positives", "false_positives", "no plan", "no_plan", "light", "micro", "unique found", "unique confirmed", "unique_found", "unique_confirmed", "precision", "pending", "out of scope":
				cells[j] = " - "
			}
		}
		lines[i] = strings.Join(cells, "|")
		masked = true
	}
	result := strings.Join(lines, "\n")
	if masked && !strings.Contains(result, planlessNote) {
		result = strings.TrimRight(result, "\n") + "\n" + planlessNote + "\n"
	}
	return result
}

func hasPlans(a Aggregate) bool {
	for _, r := range a.Cases {
		if r.PlanFound || r.Found > 0 || r.FindingRows > 0 {
			return true
		}
	}
	return len(a.Cases) == 0 // legacy aggregates without case observations are unknown
}

// CaughtDifference uses Wilson bounds combined by Newcombe's unpaired method.
// Defect-runs are treated as independent; repeated/correlated cases warrant caution.
func (c Comparison) CaughtDifference() string {
	a, b := c.Before, c.After
	if a.Defects == 0 || b.Defects == 0 {
		return "caught difference: unavailable (no defect-run denominator)."
	}
	p, q := float64(a.Caught)/float64(a.Defects), float64(b.Caught)/float64(b.Defects)
	al, au := wilson(a.Caught, a.Defects)
	bl, bu := wilson(b.Caught, b.Defects)
	d := q - p
	low := d - math.Hypot(q-bl, au-p)
	high := d + math.Hypot(bu-q, p-al)
	verdict := "outside noise"
	if low <= 0 && high >= 0 {
		verdict = "within noise"
	}
	return fmt.Sprintf("caught difference %d/%d vs %d/%d is %s (95%% Wilson/Newcombe interval: %+.3f to %+.3f).", a.Caught, a.Defects, b.Caught, b.Defects, verdict, low, high)
}

// unclassifiedFamily buckets the case-runs whose key predates families. They are counted, never
// dropped: a reading that silently lost them would report a better recall than the corpus supports.
const unclassifiedFamily = "unclassified"

// familyTotal is one failure kind's caught over defects across a reading's valid case-runs.
type familyTotal struct {
	Name    string
	Caught  int
	Defects int
}

// familyTotals splits a reading by the failure kind its cases declare, in the order Families lists them
// and with unclassified last. Controls have no keyed defect and invalid or failed runs are already out
// of recall's denominator, so all three stay out of the split.
func familyTotals(a Aggregate) []familyTotal {
	counts := map[string]*familyTotal{}
	for _, r := range a.Cases {
		if r.Control || r.Invalid || r.Failed {
			continue
		}
		name := r.Family
		if name == "" {
			name = unclassifiedFamily
		}
		c := counts[name]
		if c == nil {
			c = &familyTotal{Name: name}
			counts[name] = c
		}
		c.Caught += r.Caught
		c.Defects += r.Total
	}
	totals := make([]familyTotal, 0, len(counts))
	for _, name := range Families {
		if c := counts[name]; c != nil {
			totals = append(totals, *c)
			delete(counts, name)
		}
	}
	// A name outside Families (an older or hand-edited key) is still published rather than lost.
	leftover := make([]string, 0, len(counts))
	for name := range counts {
		if name != unclassifiedFamily {
			leftover = append(leftover, name)
		}
	}
	slices.Sort(leftover)
	for _, name := range leftover {
		totals = append(totals, *counts[name])
	}
	if c := counts[unclassifiedFamily]; c != nil {
		totals = append(totals, *c)
	}
	return totals
}

// familySection publishes which failure kinds a reading caught and which it missed, so the next run can
// be aimed at the weakest family instead of the whole corpus.
func familySection(a Aggregate) string {
	totals := familyTotals(a)
	if len(totals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(totals))
	for _, t := range totals {
		parts = append(parts, fmt.Sprintf("%s %d/%d", t.Name, t.Caught, t.Defects))
	}
	return "\ncaught by family: " + strings.Join(parts, " · ") + "\n"
}

// familiesTotals names the failure kinds a reading was aimed at, so a summary of a family-scoped run is never
// mistaken for a reading over the whole corpus. Empty when the run selected every case.
func familiesTotals(a Aggregate) string {
	if len(a.Families) == 0 {
		return ""
	}
	return " · families: " + strings.Join(a.Families, ", ")
}

func planlessSummary(a Aggregate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bench %s\n\nModel: %s · case-runs: %d · %s%s%s", a.TS, a.Model, len(a.Cases), planlessTotals(a), roundsTotals(a)+sessionsTotals(a), familiesTotals(a))
	if a.Corpus != "" {
		fmt.Fprintf(&b, " · corpus: %s", a.Corpus)
	}
	if a.DryRun {
		b.WriteString(" · dry-run")
	}
	if a.CostCeilingHit {
		b.WriteString(" · cost ceiling hit")
	}
	fmt.Fprintf(&b, "\n%s\n\n", planlessNote)
	b.WriteString("| case | reported | pinned | caught | light | micro | false positives | cost USD | turns | minutes | note | precision (pending) |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range a.Cases {
		if r.Control {
			cell := controlCell(r.ControlInventions, r.PlanFound)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | - | - | - | %.3f | %d | %.1f | %s | - |\n", r.Case, cell, cell, cell, r.CostUSD, r.Turns, r.Seconds/60, strings.TrimSpace(noPlanTag(r)))
			continue
		}
		note := strings.TrimSpace(invalidTag(r) + noPlanTag(r))
		if len(r.Notes) > 0 {
			note = strings.TrimSpace(note + " " + strings.Join(r.Notes, "; "))
		}
		if r.Catch.Checked && len(r.Catch.Notes) > 0 {
			note = strings.TrimSpace(note + " " + strings.Join(r.Catch.Notes, "; "))
		}
		fmt.Fprintf(&b, "| %s | - | - | %s | - | - | - | %.3f | %d | %.1f | %s | - |\n", r.Case, cell(r, r.Caught), r.CostUSD, r.Turns, r.Seconds/60, note)
	}
	b.WriteString(familySection(a))
	b.WriteString(missSection(a))
	return b.String()
}

// controlRuns and controlInventions name the two numbers a control arm publishes.
func controlRuns(a Aggregate) int {
	runs, _ := controlTotals(a)
	return runs
}

func controlInventions(a Aggregate) int {
	_, inventions := controlTotals(a)
	return inventions
}

// controlTotals derives the control counts from the cases the summary is about to print, so the
// header can never claim a number the rows below it contradict.
func controlTotals(a Aggregate) (runs, inventions int) {
	for _, r := range a.Cases {
		if !r.Control {
			continue
		}
		runs++
		inventions += r.ControlInventions
	}
	return runs, inventions
}

func planlessTotals(a Aggregate) string {
	return fmt.Sprintf("caught (defect-runs): %d/%d · caught (unique defects): %d/%d · failed: %d runs · invalid: %d runs · controls: %d runs · inventions on controls: %d finding rows · cost (USD): $%.3f", a.Caught, a.Defects, a.UniqueCaught, a.UniqueDefects, a.Failed, a.Invalid, controlRuns(a), controlInventions(a), a.CostUSD)
}

func wilson(k, n int) (float64, float64) {
	p := float64(k) / float64(n)
	z := 1.959963984540054
	v := z * z / float64(n)
	center := (p + v/2) / (1 + v)
	half := z * math.Sqrt(p*(1-p)/float64(n)+v/(4*float64(n))) / (1 + v)
	return center - half, center + half
}
