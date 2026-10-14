package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Rescore applies the current scoring and catch rules to a finished run: each case is re-read
// from the plan kept beside its result.json and, when the workspace was kept, its tests are
// re-checked. Cost, turns, and failure state come from the original run. The result is written
// under <results>/rescored so the original aggregate is never rewritten.
// lookup maps a case name to its case directory (fixture/, fix/, KEY.json).
func Rescore(results string, lookup func(caseName string) (string, error), suiteTimeout time.Duration) (Aggregate, error) {
	orig, err := readAggregate(results)
	if err != nil {
		return orig, err
	}
	skillVersion := skillVersionOf(results)
	provenance := provenanceForRescore(orig, skillVersion)
	agg := Aggregate{TS: time.Now().UTC().Format(time.RFC3339), Out: filepath.Join(results, "rescored"), Model: provenance.Model,
		RescoredFrom: results, RunTS: orig.TS, SkillVersion: provenance.SkillVersion, Corpus: provenance.Corpus, Runs: provenance.Runs,
		Provenance: provenance}
	for _, old := range orig.Cases {
		res := old
		agg.CostUSD += old.CostUSD
		if old.Invalid || old.Failed {
			agg.Cases = append(agg.Cases, res)
			continue
		}
		caseDir, err := lookup(old.Case)
		if err != nil {
			return agg, fmt.Errorf("%s: %w", old.Case, err)
		}
		key, err := LoadKey(caseDir)
		if err != nil {
			return agg, fmt.Errorf("%s: %w", old.Case, err)
		}
		dir := filepath.Join(results, old.Case, fmt.Sprint(max(old.Run, 1)))
		adj, err := runAdjudication(dir, old)
		if err != nil {
			return agg, fmt.Errorf("%s: %w", old.Case, err)
		}
		ws := filepath.Join(dir, "ws")
		if st, err := os.Stat(ws); err == nil && st.IsDir() {
			scored, err := ScoreWorkspaceWithAdjudication(ws, key, adj)
			if err != nil {
				return agg, fmt.Errorf("%s: %w", old.Case, err)
			}
			res = merge(old, scored)
			res.Catch = Discriminate(caseDir, ws, key, suiteTimeout)
		} else {
			scored, err := ScorePlanFileWithAdjudication(filepath.Join(dir, "test-plan.md"), key, adj)
			if err != nil {
				return agg, fmt.Errorf("%s: %w", old.Case, err)
			}
			res = merge(old, scored)
			res.Catch = old.Catch // no workspace to re-check: the run's own verdict stands
		}
		res.Caught = res.Catch.Count()
		if res.FinalMessage == "" {
			res.FinalMessage = finalMessageFromLog(dir)
		}
		res.Misses = Diagnose(res, key)
		agg.Cases = append(agg.Cases, res)
	}
	finalizeAggregate(&agg)
	if err := os.MkdirAll(agg.Out, 0o755); err != nil {
		return agg, err
	}
	if err := writeJSON(filepath.Join(agg.Out, "aggregate.json"), agg); err != nil {
		return agg, err
	}
	if err := os.WriteFile(filepath.Join(agg.Out, "summary.md"), []byte(Summary(agg)), 0o644); err != nil {
		return agg, err
	}
	return agg, nil
}

// runAdjudication reads the record kept in a run's directory and checks it decides this run.
// Applying one run's decisions to another run of the same case is the misattribution the record
// exists to prevent, so the run number is checked rather than trusted. A malformed record is a
// refusal: a rescore that ignored it would silently re-open the decisions it holds.
func runAdjudication(dir string, old Result) (*Adjudication, error) {
	path := filepath.Join(dir, AdjudicationFile)
	adj, err := LoadAdjudication(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%s: %w", AdjudicationFile, err)
	}
	if run := max(old.Run, 1); adj.Run != run {
		return nil, fmt.Errorf("%s decides run %d of case %q, not run %d of case %q", AdjudicationFile, adj.Run, adj.Case, run, old.Case)
	}
	return &adj, nil
}

// skillVersionOf reads the skill version the history recorded for a results directory, so a
// rescore row stays attributed to the version that produced the run.
func skillVersionOf(results string) string {
	for _, dir := range []string{filepath.Dir(filepath.Dir(results)), "bench"} {
		data, err := os.ReadFile(filepath.Join(dir, "history.jsonl"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var e HistoryEntry
			if json.Unmarshal([]byte(line), &e) == nil && e.Out == results && e.Kind != KindRescore {
				return e.SkillVersion
			}
		}
	}
	return ""
}
