package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionResult records new catches and the remaining defects after an independent review.
type SessionResult struct {
	Session     int      `json:"session"`
	Caught      []string `json:"caught"`
	Remaining   []string `json:"remaining"`
	BrokenTests int      `json:"broken_tests"`
	Turns       int      `json:"turns"`
	CostUSD     float64  `json:"cost_usd"`
	Seconds     float64  `json:"seconds"`
	StopReason  string   `json:"stop_reason,omitempty"`
}

func runSessions(caseDir, ws string, key Key, runOpts, opts Options, res Result) Result {
	started := time.Now()
	caught := map[string]bool{}
	attempts := 0
	var ar AgentResult
	var catch CatchResult
	runOpts.Followup = ""
	for session := 1; session <= opts.Sessions; session++ {
		beforeTurns, beforeCost := res.Turns, res.CostUSD
		tick := time.Now()
		ar, attempts = runAgent(ws, key, runOpts, opts, &res, attempts)
		r := SessionResult{Session: session, Caught: []string{}, Remaining: []string{}, Turns: res.Turns - beforeTurns, CostUSD: res.CostUSD - beforeCost, Seconds: time.Since(tick).Seconds()}
		if !res.Failed {
			catch = Discriminate(caseDir, ws, key, opts.SuiteTimeout)
			r.BrokenTests = catch.BrokenTests
			for _, d := range key.Defects {
				if catch.Caught[d.ID] && !caught[d.ID] {
					caught[d.ID] = true
					r.Caught = append(r.Caught, d.ID)
				}
			}
		}
		for _, d := range key.Defects {
			if !caught[d.ID] {
				r.Remaining = append(r.Remaining, d.ID)
			}
		}
		if res.Failed {
			r.StopReason = "the session failed: " + res.FailReason
		}
		res.Sessions = append(res.Sessions, r)
		if res.Failed {
			break
		}
		// Nothing caught means no user fix; leave even the agent's source edits untouched.
		if len(r.Caught) > 0 {
			if err := applySessionFix(caseDir, ws, r.Remaining); err != nil {
				res.Sessions[len(res.Sessions)-1].StopReason = err.Error()
				res.Notes = append(res.Notes, err.Error())
				break
			}
		}
		if len(r.Remaining) == 0 {
			break
		}
	}
	res.Seconds = time.Since(started).Seconds()
	if res.Failed {
		res.Notes = append(res.Notes, res.FailReason)
		return finish(res, opts, true)
	}
	res.FinalMessage = clipFinalMessage(ar.Result)
	res = merge(res, ScoreWorkspace(ws, key))
	catch.Caught = caught
	res.Catch = catch
	res.Caught = len(caught)
	res.Misses = Diagnose(res, key)
	return finish(res, opts, res.Recall < 1 || res.Caught < res.Total)
}

// Fixed variants are overlays on the fixture. Restore the agent's tests after copying both.
func applySessionFix(caseDir, ws string, remaining []string) error {
	variant := "all"
	switch len(remaining) {
	case 0:
	case 1:
		variant = "keep-" + remaining[0]
	default:
		return fmt.Errorf("sessions stopped: no reference fix for remaining defects %v", remaining)
	}
	overlay := filepath.Join(caseDir, FixDir, variant)
	if st, err := os.Stat(overlay); err != nil || !st.IsDir() {
		return fmt.Errorf("sessions stopped: no fix/%s directory for remaining defects %v", variant, remaining)
	}
	fixture := filepath.Join(caseDir, FixtureDir)
	tests, err := agentTestFiles(fixture, ws)
	if err != nil {
		return err
	}
	saved := map[string][]byte{}
	for _, test := range tests {
		data, err := os.ReadFile(filepath.Join(ws, test))
		if err != nil {
			return err
		}
		saved[test] = data
	}
	if err := copyTree(fixture, ws); err != nil {
		return err
	}
	if err := copyTree(overlay, ws); err != nil {
		return err
	}
	manifest, err := os.ReadFile(filepath.Join(caseDir, sourceDeletionsFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		var deleted []string
		if err := json.Unmarshal(manifest, &deleted); err != nil {
			return err
		}
		root, err := os.OpenRoot(ws)
		if err != nil {
			return err
		}
		defer root.Close()
		for _, path := range deleted {
			if _, keep := saved[path]; keep {
				continue
			}
			if err := root.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	for test, data := range saved {
		if err := os.WriteFile(filepath.Join(ws, test), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func addSessions(a *Aggregate, sessions []SessionResult) {
	for i, s := range sessions {
		if i == len(a.CaughtBySession) {
			a.CaughtBySession = append(a.CaughtBySession, 0)
			a.RemainingBySession = append(a.RemainingBySession, 0)
			a.BrokenBySession = append(a.BrokenBySession, 0)
			a.CompletedBySession = append(a.CompletedBySession, 0)
			a.TurnsBySession = append(a.TurnsBySession, 0)
			a.CostBySession = append(a.CostBySession, 0)
			a.SecondsBySession = append(a.SecondsBySession, 0)
		}
		a.TurnsBySession[i] += s.Turns
		a.CostBySession[i] += s.CostUSD
		a.SecondsBySession[i] += s.Seconds
		a.CaughtBySession[i] += len(s.Caught)
		a.RemainingBySession[i] += len(s.Remaining)
		a.BrokenBySession[i] += s.BrokenTests
		if len(s.Remaining) == 0 {
			a.CompletedBySession[i]++
		}
	}
}

func sessionsTotals(a Aggregate) string {
	if len(a.CaughtBySession) == 0 {
		return ""
	}
	return fmt.Sprintf(" · new catches per session (defect-runs): %s · remaining: %s · tests red on the fixed code: %s · runs reaching zero remaining by session: %s", joinInts(a.CaughtBySession), joinInts(a.RemainingBySession), joinInts(a.BrokenBySession), joinInts(a.CompletedBySession))
}
