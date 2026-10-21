package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Arms of a fix-loop run's second pass. `plain` is the bench prompt round 1 got; `registry` is that prompt plus
// what `tsp findings check` reports about the findings round 1 recorded.
const (
	FixArmPlain    = "plain"
	FixArmRegistry = "registry"
)

// FindingsDir is the private state root a fix loop gives `tsp findings`, beside the run it belongs to. Every
// run gets its own, so a reading never writes the operator's registry and two runs never read each other's.
const FindingsDir = ".findings-home"

// FixLoop is the round-2 measurement a --fix-loop run makes: round 1's witnesses, the findings record, the
// agent's fix, and the two round-2 arms forked from the same fixed workspace. The run's own fields stay round
// 1's — scored, caught, costed and timed exactly as a one-pass run — so a fix-loop reading is still comparable
// on what round 1 found; the loop's work is counted here instead.
type FixLoop struct {
	// Valid says the loop measured what it is for: round 1 pinned a witness, the registry recorded it, the fix was
	// committed, and both arms ran and were classified. A loop that did not is not a loop reading: its reason says
	// why, and the run's round-1 numbers stay valid evidence on their own.
	Valid           bool     `json:"valid"`
	Reason          string   `json:"reason,omitempty"`
	Skipped         bool     `json:"skipped,omitempty"` // round 1 pinned nothing, so there was nothing to fix
	Witnesses       []string `json:"witnesses,omitempty"`
	Record          string   `json:"record,omitempty"`            // what `tsp findings record` said
	Check           string   `json:"check,omitempty"`             // what `tsp findings check` said: the registry arm's report
	FixTouchedTests bool     `json:"fix_touched_tests,omitempty"` // the fix rewrote test files, which were restored
	FixPaths        []string `json:"fix_paths,omitempty"`         // the non-test paths the fix commit holds
	FixCommit       string   `json:"fix_commit,omitempty"`
	Fix             FixStep  `json:"fix"`
	Arms            []FixArm `json:"arms,omitempty"`
}

// FixStep is the agent process that fixed round 1's witnesses: its work, kept apart from the run's own cost and
// turns so the round-1 numbers stay what a one-pass run would have recorded.
type FixStep struct {
	Turns   int     `json:"turns"`
	CostUSD float64 `json:"cost_usd"`
	Seconds float64 `json:"seconds"`
	Reason  string  `json:"reason,omitempty"`
}

// FixArm is one round-2 pass over the fixed workspace. Every red test is classified against the case's fully
// fixed code: NewReal separates the agent's fix from correct code, NewFalse proves nothing, and StillRed is a
// round-1 witness the fix did not turn green.
type FixArm struct {
	Arm      string   `json:"arm"`
	NewReal  []string `json:"new_real,omitempty"`
	NewFalse []string `json:"new_false,omitempty"`
	StillRed []string `json:"still_red,omitempty"`
	Turns    int      `json:"turns"`
	CostUSD  float64  `json:"cost_usd"`
	Seconds  float64  `json:"seconds"`
	Reason   string   `json:"reason,omitempty"`
	Notes    []string `json:"notes,omitempty"`
}

// FixLoopTotals is the reading's view of the loop: how many runs were valid or skipped, what the fix step cost,
// and per-arm totals. A reading without the flag carries none, so its summary stays the one it was.
type FixLoopTotals struct {
	ValidRuns   int            `json:"valid_runs"`
	SkippedRuns int            `json:"skipped_runs"`
	FailedRuns  int            `json:"failed_runs"`
	FixCostUSD  float64        `json:"fix_cost_usd"`
	FixSeconds  float64        `json:"fix_seconds"`
	Arms        []FixArmTotals `json:"arms,omitempty"`
}

// FixArmTotals sums one arm over the runs whose loop completed.
type FixArmTotals struct {
	Arm      string  `json:"arm"`
	Runs     int     `json:"runs"`
	NewReal  int     `json:"new_real"`
	NewFalse int     `json:"new_false"`
	StillRed int     `json:"still_red"`
	CostUSD  float64 `json:"cost_usd"`
	Seconds  float64 `json:"seconds"`
}

// fixPrompt is the whole instruction the fix step gets: round 1's witnesses and nothing about the bench, the
// skill or the case, so the fix is the agent's own work on what its own tests pinned.
func fixPrompt(witnesses []string) string {
	var b strings.Builder
	b.WriteString("Fix the defects these failing tests pin. They fail on the code this repository's HEAD holds, and each one names behaviour the code does not have yet.\n\n")
	for _, name := range witnesses {
		fmt.Fprintf(&b, "- %s\n", name)
	}
	b.WriteString("\nChange the source so each test passes. Do not weaken, delete or rewrite the tests: they are the record of what you were asked to fix, and this step is judged on whether they turn green.\n")
	return b.String()
}

// registryArmPrompt is the plain arm's prompt plus what the registry knows: the report is the whole point of the
// arm, so it is appended rather than summarised away.
func registryArmPrompt(base, report string) string {
	return base + "\n\nThe source has changed since these findings were recorded, and `tsp findings check` reports:\n\n" +
		strings.TrimRight(report, "\n") + "\n"
}

// runFixLoop takes over once round 1 is scored and returns the loop's record. It never changes the run's own
// numbers: a failed step is the loop's reason, not a failed run, because round 1 already happened and is valid
// evidence by itself.
func runFixLoop(caseDir, ws string, key Key, runOpts, opts Options) *FixLoop {
	loop := &FixLoop{}
	l, e := os.OpenRoot(filepath.Dir(ws))
	if e != nil {
		loop.Skipped, loop.Reason = true, "the run's directory could not be opened: "+e.Error()
		return loop
	}
	defer l.Close()
	// D4: a round that pinned nothing has nothing to fix, so the loop does not run and says so.
	rep, err := RedCheck(ws, "", opts.SuiteTimeout)
	if err != nil {
		loop.Skipped, loop.Reason = true, "round 1's witnesses could not be read: "+err.Error()
		return loop
	}
	if len(rep.Red) == 0 {
		loop.Skipped, loop.Reason = true, "round 1 left no test failing on the unmodified code: nothing to fix"
		return loop
	}
	loop.Witnesses = append([]string(nil), rep.Red...)
	// D2: the registry is written before the fix, so it holds the witnesses of the code round 1 measured.
	home := filepath.Join(filepath.Dir(ws), FindingsDir)
	record, code, err := runTsp(ws, findingsEnv(home), opts, "findings", "record", "--cwd", ws, "--suite", key.Suite,
		"--timeout", opts.SuiteTimeout.String())
	if err != nil || code != 0 {
		loop.Reason = "tsp findings record: " + tspFailure(record, code, err)
	} else {
		loop.Record = record
	}
	// D3: round 1's tests are the record the fix is judged on, so they are saved before the fix can touch them.
	round1, err := testFileBytes(ws)
	if err != nil {
		loop.Reason = joinReason(loop.Reason, "round 1's test files could not be read: "+err.Error())
		return loop
	}
	fixDir := filepath.Join(filepath.Dir(ws), "fix")
	if err := os.MkdirAll(fixDir, 0o755); err != nil {
		loop.Reason = joinReason(loop.Reason, "the fix step's log directory could not be created: "+err.Error())
		return loop
	}
	fixOpts := runOpts
	fixOpts.Followup, fixOpts.Task = "", fixPrompt(loop.Witnesses)
	started := time.Now()
	var fixWork Result
	runAgentIn(fixDir, ws, key, fixOpts, opts, &fixWork, 0)
	loop.Fix = FixStep{Turns: fixWork.Turns, CostUSD: fixWork.CostUSD, Seconds: time.Since(started).Seconds()}
	if fixWork.Failed {
		// The fixed workspace this loop is built on does not exist, so there is nothing to fork: the arms would
		// measure a workspace the fix never touched.
		loop.Fix.Reason = fixWork.FailReason
		loop.Reason = joinReason(loop.Reason, "the fix step failed: "+fixWork.FailReason)
		return loop
	}
	touched, err := restoreTestFiles(ws, round1)
	loop.FixTouchedTests = len(touched) > 0
	if err != nil {
		loop.Reason = joinReason(loop.Reason, "the fix's test files could not be restored: "+err.Error())
		return loop
	}
	paths, commit, err := commitFix(ws)
	loop.FixPaths, loop.FixCommit = paths, commit
	if err != nil {
		loop.Reason = joinReason(loop.Reason, err.Error())
		return loop
	}
	if len(paths) == 0 {
		// Both arms would measure round 1's unfixed code, so neither is paid for.
		loop.Fix.Reason = joinReason(loop.Fix.Reason, "the fix changed no non-test path, so HEAD still holds round 1's code")
		loop.Reason = joinReason(loop.Reason, loop.Fix.Reason)
		return loop
	}
	// D5: the registry is keyed by the repository's location, so the check runs in the fixed workspace itself,
	// before the fork, and its report is what the registry arm is told.
	check, code, err := runTsp(ws, findingsEnv(home), opts, "findings", "check", "--cwd", ws,
		"--timeout", opts.SuiteTimeout.String())
	if err != nil || code >= 2 {
		loop.Reason = joinReason(loop.Reason, "tsp findings check: "+tspFailure(check, code, err))
	} else {
		loop.Check = check
	}
	loop.Arms = runFixArms(caseDir, ws, key, runOpts, opts, loop)
	for _, arm := range loop.Arms {
		if arm.Reason != "" {
			loop.Reason = joinReason(loop.Reason, arm.Arm+" arm: "+arm.Reason)
		}
	}
	loop.Valid = loop.Reason == "" && len(loop.Arms) == 2
	return loop
}

// runFixArms forks the fixed workspace once per arm and runs one round in each. The two are paired: same fixed
// code, same prompt, same model, and the only difference is what the registry report says.
func runFixArms(caseDir, ws string, key Key, runOpts, opts Options, loop *FixLoop) []FixArm {
	base := agentPrompt(key, runOpts)
	arms := make([]FixArm, 0, 2)
	for _, arm := range []struct{ name, report string }{
		{FixArmPlain, ""},
		{FixArmRegistry, loop.Check},
	} {
		got := FixArm{Arm: arm.name}
		armWS := filepath.Join(filepath.Dir(ws), "arm-"+arm.name)
		if err := copyTree(ws, armWS); err != nil {
			got.Reason = "the fixed workspace could not be forked: " + err.Error()
			arms = append(arms, got)
			continue
		}
		logDir := filepath.Join(filepath.Dir(ws), "log-"+arm.name)
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			got.Reason = "the arm's log directory could not be created: " + err.Error()
			arms = append(arms, got)
			continue
		}
		armOpts := runOpts
		armOpts.Followup, armOpts.Task = "", ""
		if arm.name == FixArmRegistry {
			if arm.report == "" {
				// The registry arm without a report is not a registry arm: it runs the plain prompt and the
				// result says the loop is not valid, instead of comparing two plain passes as if they differed.
				got.Notes = append(got.Notes, "no findings report was available: the arm ran the plain prompt")
				got.Reason = "no findings report was available"
			} else {
				armOpts.Task = registryArmPrompt(base, arm.report)
			}
		}
		started := time.Now()
		var armWork Result
		runAgentIn(logDir, armWS, key, armOpts, opts, &armWork, 0)
		got.Turns, got.CostUSD, got.Seconds = armWork.Turns, armWork.CostUSD, time.Since(started).Seconds()
		if armWork.Failed {
			got.Reason = joinReason(got.Reason, "the arm failed: "+armWork.FailReason)
		} else {
			newReal, newFalse, stillRed, notes, err := classifyArm(caseDir, armWS, key, opts, loop.Witnesses)
			got.NewReal, got.NewFalse, got.StillRed = newReal, newFalse, stillRed
			got.Notes = append(got.Notes, notes...)
			if err != nil {
				got.Reason = joinReason(got.Reason, err.Error())
			}
		}
		arms = append(arms, got)
		if !opts.Keep && got.Reason == "" {
			_ = removeTree(armWS)
			_ = removeTree(logDir)
		}
	}
	return arms
}

// classifyArm applies D6 to one arm: over the tests red on the arm's HEAD, one that is green on the case's fully
// fixed code separates the agent's fixed code from correct code, and one that is red there too proves nothing.
// A round-1 witness still red is a regression the fix did not turn green.
func classifyArm(caseDir, ws string, key Key, opts Options, witnesses []string) (newReal, newFalse, stillRed, notes []string, err error) {
	rep, err := RedCheck(ws, "", opts.SuiteTimeout)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("the arm's red tests could not be read: %w", err)
	}
	if len(rep.Red) == 0 {
		return nil, nil, nil, []string{"no test was red on the arm's HEAD"}, nil
	}
	fixture := filepath.Join(caseDir, FixtureDir)
	tests, err := agentTestFiles(fixture, ws)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	onFixed, _, out, err := testsOn(fixture, filepath.Join(caseDir, FixDir, "all"), ws, tests, key.Suite, opts.SuiteTimeout)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("the arm's tests could not be run against the fully fixed code: %v: %s", err, tail(out, 400))
	}
	pinned := map[string]bool{}
	for _, name := range witnesses {
		pinned[name] = true
	}
	for _, name := range rep.Red {
		passed, ran := onFixed[name]
		switch {
		case !ran:
			// RedCheck and testsOn read the runner's per-test names through the same parser, so this cannot
			// happen without one of them having changed shape: it is said out loud, never guessed around.
			notes = append(notes, name+" was not reported when the arm's tests ran against the fully fixed code")
		case passed && pinned[name]:
			stillRed = append(stillRed, name)
		case passed:
			newReal = append(newReal, name)
		case pinned[name]:
			notes = append(notes, name+" is a round-1 witness that is red on the fully fixed code too; it is not counted")
		default:
			newFalse = append(newFalse, name)
		}
	}
	sort.Strings(newReal)
	sort.Strings(newFalse)
	sort.Strings(stillRed)
	return newReal, newFalse, stillRed, notes, nil
}

// testFileBytes reads every test file the workspace holds, so the fix's edits to them can be undone.
func testFileBytes(ws string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(ws, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(ws, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && ignoredPathRe.MatchString(rel+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTestFile(rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = data
		return nil
	})
	return out, err
}

// restoreTestFiles puts every test file back to its round-1 bytes and removes a test file the fix created: round
// 2 measures what the fix did to the source, so a test the fix wrote may not decide it. A test round 1 left and
// the fix deleted is round 1's too. It returns the paths it had to touch.
func restoreTestFiles(ws string, round1 map[string][]byte) ([]string, error) {
	now, err := testFileBytes(ws)
	if err != nil {
		return nil, err
	}
	var touched []string
	for path, data := range now {
		orig, existed := round1[path]
		if existed && bytes.Equal(orig, data) {
			continue
		}
		if !existed {
			if err := os.Remove(filepath.Join(ws, filepath.FromSlash(path))); err != nil {
				return touched, err
			}
			touched = append(touched, path)
			continue
		}
		if err := os.WriteFile(filepath.Join(ws, filepath.FromSlash(path)), orig, 0o644); err != nil {
			return touched, err
		}
		touched = append(touched, path)
	}
	for path, data := range round1 {
		if _, ok := now[path]; ok {
			continue
		}
		target := filepath.Join(ws, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return touched, err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return touched, err
		}
		touched = append(touched, path)
	}
	sort.Strings(touched)
	return touched, nil
}

// commitFix commits the fix's non-test changes and leaves round 1's tests uncommitted, so HEAD holds the agent's
// fixed code while RedCheck and the registry read exactly the tests round 1 left. It returns the committed paths
// and the commit HEAD holds afterwards.
func commitFix(ws string) ([]string, string, error) {
	changed, err := gitChangedPaths(ws)
	if err != nil {
		return nil, "", err
	}
	var paths []string
	for _, path := range changed {
		if !isTestFile(path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	head, err := gitRun(ws, "rev-parse", "HEAD")
	if err != nil {
		return paths, "", fmt.Errorf("the fix's commit could not be read back: %v: %s", err, strings.TrimSpace(head))
	}
	if len(paths) == 0 {
		return nil, strings.TrimSpace(head), nil
	}
	if out, err := gitRun(ws, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return paths, "", fmt.Errorf("the fix could not be staged: %v: %s", err, strings.TrimSpace(out))
	}
	if out, err := gitRun(ws, "commit", "-qm", "agent fix"); err != nil {
		return paths, "", fmt.Errorf("the fix could not be committed: %v: %s", err, strings.TrimSpace(out))
	}
	head, err = gitRun(ws, "rev-parse", "HEAD")
	return paths, strings.TrimSpace(head), err
}

// gitChangedPaths lists the paths the workspace changed or added since HEAD, one per entry, a rename reported as
// its destination path.
func gitChangedPaths(dir string) ([]string, error) {
	out, err := gitRun(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("the workspace's changes could not be listed: %v: %s", err, strings.TrimSpace(out))
	}
	var paths []string
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		status, path := entry[:2], entry[3:]
		paths = append(paths, filepath.ToSlash(path))
		if status[0] == 'R' || status[0] == 'C' {
			// The next field is the path it came from; committing only the destination would leave the origin's
			// deletion unstaged, so HEAD would still hold a file the fix moved away.
			if i+1 < len(fields) && fields[i+1] != "" {
				paths = append(paths, filepath.ToSlash(fields[i+1]))
			}
			i++
		}
	}
	return paths, nil
}

// findingsEnv is the environment `tsp findings` runs with: a private state root, and none of the operator's.
func findingsEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "TSP_HOME="), strings.HasPrefix(kv, "TPP_HOME="), strings.HasPrefix(kv, "RDD_PLUS_HOME="):
			continue
		}
		env = append(env, kv)
	}
	return append(env, "TSP_HOME="+home)
}

// runTsp runs the bench's own binary as tsp in dir and returns its output and exit code. A binary that could not
// start is an error; an exit code is an answer the caller decides on, because an open finding exits 1 and is
// still a report.
func runTsp(dir string, env []string, opts Options, args ...string) (string, int, error) {
	if opts.TspPath == "" {
		return "", -1, errors.New("no tsp binary to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*opts.SuiteTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, opts.TspPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	boundProcessTree(cmd)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err == nil {
		return buf.String(), 0, nil
	}
	var ee *exec.ExitError
	// A process killed by its deadline or a signal never answered: its partial output is not a report.
	if errors.As(err, &ee) && ctx.Err() == nil && ee.ExitCode() >= 0 {
		return buf.String(), ee.ExitCode(), nil
	}
	if ctx.Err() != nil {
		return buf.String(), -1, fmt.Errorf("no answer within %s", 4*opts.SuiteTimeout)
	}
	return buf.String(), -1, err
}

// tspFailure describes a step that did not answer, keeping what it said so the reason is checkable.
func tspFailure(out string, code int, err error) string {
	reason := fmt.Sprintf("exit %d", code)
	if err != nil {
		reason = err.Error()
	}
	if text := strings.TrimSpace(out); text != "" {
		reason += ": " + tail(text, 400)
	}
	return reason
}

// joinReason appends one reason to another, in the order the steps ran.
func joinReason(reason, add string) string {
	if reason == "" {
		return add
	}
	return reason + "; " + add
}

// addFixLoop adds one run's loop to the reading's totals.
func addFixLoop(a *Aggregate, loop *FixLoop) {
	if loop == nil {
		return
	}
	if a.FixLoop == nil {
		a.FixLoop = &FixLoopTotals{Arms: []FixArmTotals{{Arm: FixArmPlain}, {Arm: FixArmRegistry}}}
	}
	t := a.FixLoop
	t.FixCostUSD += loop.Fix.CostUSD
	t.FixSeconds += loop.Fix.Seconds
	switch {
	case loop.Skipped:
		t.SkippedRuns++
	case loop.Valid:
		t.ValidRuns++
	default:
		t.FailedRuns++
	}
	for _, arm := range loop.Arms {
		var slot *FixArmTotals
		for i := range t.Arms {
			if t.Arms[i].Arm == arm.Arm {
				slot = &t.Arms[i]
			}
		}
		if slot == nil {
			t.Arms = append(t.Arms, FixArmTotals{Arm: arm.Arm})
			slot = &t.Arms[len(t.Arms)-1]
		}
		if arm.Reason != "" {
			continue
		}
		slot.Runs++
		slot.NewReal += len(arm.NewReal)
		slot.NewFalse += len(arm.NewFalse)
		slot.StillRed += len(arm.StillRed)
		slot.CostUSD += arm.CostUSD
		slot.Seconds += arm.Seconds
	}
}

// fixLoopTotals is the summary's fix-loop line: what the loop measured, per arm. Empty for a reading that did not
// run one, so every other summary stays byte for byte what it was.
func fixLoopTotals(a Aggregate) string {
	if a.FixLoop == nil {
		return ""
	}
	t := a.FixLoop
	arms := make([]string, 0, len(t.Arms))
	for _, arm := range t.Arms {
		arms = append(arms, fmt.Sprintf("%s: %d new real, %d new false, %d still red (cost USD $%.3f, %.0fs, %d runs)",
			arm.Arm, arm.NewReal, arm.NewFalse, arm.StillRed, arm.CostUSD, arm.Seconds, arm.Runs))
	}
	return fmt.Sprintf(" · fix loop: %d valid runs, %d skipped, %d failed · fix step (cost USD $%.3f, %.0fs) · %s",
		t.ValidRuns, t.SkippedRuns, t.FailedRuns, t.FixCostUSD, t.FixSeconds, strings.Join(arms, " · "))
}

// withFixLoop puts the fix-loop line on a summary's header line, where every aggregate total lives. The plan-less
// renderer is shared with the plan reader, so the line is added here rather than in both.
func withFixLoop(text string, agg Aggregate) string {
	line := fixLoopTotals(agg)
	if line == "" {
		return text
	}
	head, rest, found := strings.Cut(text, "\n")
	if !found {
		return text + line
	}
	return head + line + "\n" + rest
}
