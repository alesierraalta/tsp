package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunnerGentleReview runs Gentle AI's native review (`gentle-ai review`) over the case presented as a change:
// the reviewer reads the diff from the fixed base to the candidate that holds the defects, writes no test, and
// its findings are kept as the run's result. It needs --as-change, so both arms of a comparison read the same
// repository.
const RunnerGentleReview = "gentle-review"

// gentleReviewLenses are the four lenses a native review can use; the bench asks for all of them so the arm is
// the full outside view RDD offers rather than whichever lenses an agent would have picked.
const gentleReviewLenses = "risk,resilience,readability,reliability"

// Environment the claude shim reads. The bench puts a `claude` on PATH that re-executes this binary with them set,
// so every reviewer process Gentle AI starts runs on the bench's model and reports its cost.
const (
	shimEnv      = "TSP_BENCH_CLAUDE_SHIM"
	shimRealEnv  = "TSP_BENCH_CLAUDE_REAL"
	shimModelEnv = "TSP_BENCH_CLAUDE_MODEL"
	shimLogEnv   = "TSP_BENCH_CLAUDE_LOG"
)

// ScaffoldAsChange presents a case as a change: a base commit holding the fixed code (the fixture with fix/all
// over it) and a candidate commit holding the fixture as shipped, defects included. The workspace ends on the
// candidate, so the suite and every score read the same tree Scaffold gives; only the history differs.
func ScaffoldAsChange(caseDir, ws string, key Key, suiteTimeout time.Duration) (SuiteResult, error) {
	return ScaffoldAsChangeFrom(caseDir, ws, key, suiteTimeout, false)
}

// ScaffoldAsChangeFrom is ScaffoldAsChange with the base chosen: emptyBase commits a base with no files, so the
// whole fixture is the change and a defect sits in it rather than in a diff of its own. A clean control has no
// fix to diff against and always gets the empty base, which makes every finding on it a false positive.
func ScaffoldAsChangeFrom(caseDir, ws string, key Key, suiteTimeout time.Duration, emptyBase bool) (SuiteResult, error) {
	src := filepath.Join(caseDir, FixtureDir)
	fix := filepath.Join(caseDir, "fix", "all")
	if st, err := os.Stat(fix); key.IsCleanControl() && (err != nil || !st.IsDir()) {
		emptyBase = true
	}
	needed := []string{src}
	if !emptyBase {
		needed = append(needed, fix)
	}
	for _, dir := range needed {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return SuiteResult{}, fmt.Errorf("case %s has no %s directory, so it cannot be presented as a change", caseDir, dir)
		}
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return SuiteResult{}, err
	}
	steps := [][]string{
		{"init", "-q"},
		{"config", "user.email", "bench@rdd-plus"},
		{"config", "user.name", "rdd-plus bench"},
	}
	if emptyBase {
		steps = append(steps, []string{"commit", "-q", "--allow-empty", "-m", "base: empty"})
	} else {
		if err := copyTree(src, ws); err != nil {
			return SuiteResult{}, fmt.Errorf("copy fixture: %w", err)
		}
		if err := copyTree(fix, ws); err != nil {
			return SuiteResult{}, fmt.Errorf("copy fix/all: %w", err)
		}
		steps = append(steps, []string{"add", "-A"}, []string{"commit", "-qm", "base: fixed"})
	}
	if err := gitSteps(ws, steps); err != nil {
		return SuiteResult{}, err
	}
	if err := clearWorktree(ws); err != nil {
		return SuiteResult{}, err
	}
	if err := copyTree(src, ws); err != nil {
		return SuiteResult{}, fmt.Errorf("copy fixture: %w", err)
	}
	if _, err := os.Stat(filepath.Join(ws, KeyFile)); err == nil {
		return SuiteResult{}, errors.New("answer key leaked into the workspace")
	}
	if err := gitSteps(ws, [][]string{{"add", "-A"}}); err != nil {
		return SuiteResult{}, err
	}
	if _, err := gitRun(ws, "diff", "--cached", "--quiet"); err == nil {
		return SuiteResult{}, fmt.Errorf("case %s: fix/all does not change the fixture, so there is no change to review", caseDir)
	}
	if err := gitSteps(ws, [][]string{{"commit", "-qm", "candidate"}}); err != nil {
		return SuiteResult{}, err
	}
	return RunSuite(ws, key.Suite, suiteTimeout)
}

func gitSteps(ws string, steps [][]string) error {
	for _, args := range steps {
		if out, err := gitRun(ws, args...); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(out))
		}
	}
	return nil
}

// clearWorktree removes everything but .git, so the candidate commit records deletions as well as edits.
func clearWorktree(ws string) error {
	entries, err := os.ReadDir(ws)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(ws, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// reviewerCall is one reviewer process the shim ran: what Gentle AI asked for, what the bench ran instead, and
// what came back.
type reviewerCall struct {
	RequestedModel string   `json:"requested_model"`
	Model          string   `json:"model"`
	ResolvedModels []string `json:"resolved_models,omitempty"`
	CostUSD        float64  `json:"cost_usd"`
	DurationMS     int64    `json:"duration_ms"`
	ExitCode       int      `json:"exit_code"`
	Result         string   `json:"result"`
}

// shimArgs rewrites a reviewer's claude arguments: the model becomes the bench's, and text output becomes JSON so
// the cost and the model that answered can be read. It returns the model Gentle AI asked for.
func shimArgs(args []string, model string) ([]string, string) {
	out := make([]string, 0, len(args)+2)
	requested, sawModel := "", false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--model" && i+1 < len(args):
			requested, sawModel = args[i+1], true
			out = append(out, a, model)
			i++
		case strings.HasPrefix(a, "--model="):
			requested, sawModel = strings.TrimPrefix(a, "--model="), true
			out = append(out, "--model="+model)
		case a == "--output-format" && i+1 < len(args):
			out = append(out, a, "json")
			i++
		case strings.HasPrefix(a, "--output-format="):
			out = append(out, "--output-format=json")
		default:
			out = append(out, a)
		}
	}
	if !sawModel {
		out = append(out, "--model", model)
	}
	return out, requested
}

// parseClaudeJSON reads `claude --output-format json`: the answer text, its cost, and the models that answered.
func parseClaudeJSON(data []byte) (result string, cost float64, models []string, err error) {
	var v struct {
		Result     string                     `json:"result"`
		TotalCost  float64                    `json:"total_cost_usd"`
		ModelUsage map[string]json.RawMessage `json:"modelUsage"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &v); err != nil {
		return "", 0, nil, err
	}
	for m := range v.ModelUsage {
		models = append(models, m)
	}
	return v.Result, v.TotalCost, models, nil
}

// RunClaudeShim is the `claude` a gentle-review run puts on PATH. It reports false when this process is not the
// shim. As the shim it runs the real claude with the bench's model, prints only the answer text the way the
// reviewer adapter expects, appends one reviewerCall line to the log, and exits with claude's status.
func RunClaudeShim() (int, bool) {
	if os.Getenv(shimEnv) != "1" {
		return 0, false
	}
	args, requested := shimArgs(os.Args[1:], os.Getenv(shimModelEnv))
	started := time.Now()
	cmd := exec.Command(os.Getenv(shimRealEnv), args...)
	cmd.Env = withoutShimEnv(os.Environ())
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	out, err := cmd.Output()
	call := reviewerCall{RequestedModel: requested, Model: os.Getenv(shimModelEnv), DurationMS: time.Since(started).Milliseconds()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		call.ExitCode = ee.ExitCode()
	} else if err != nil {
		call.ExitCode = 127
	}
	result, cost, models, perr := parseClaudeJSON(out)
	if perr != nil {
		result = string(out)
	}
	call.Result, call.CostUSD, call.ResolvedModels = result, cost, models
	if line, merr := json.Marshal(call); merr == nil {
		if f, ferr := os.OpenFile(os.Getenv(shimLogEnv), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); ferr == nil {
			fmt.Fprintf(f, "%s\n", line)
			f.Close()
		}
	}
	fmt.Print(result)
	if call.ExitCode == 0 && perr != nil {
		return 1, true
	}
	return call.ExitCode, true
}

func withoutShimEnv(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "TSP_BENCH_CLAUDE_") {
			out = append(out, kv)
		}
	}
	return out
}

// reviewStep is what the driver does with one transition Gentle AI offers.
type reviewStep struct {
	Kind      string     // "start", "execute", "capture", "done", "stop"
	Args      []string   // the command to run for start and execute
	Captures  [][]string // one argument list per reviewer slot for capture
	Reason    string
	Operation string
}

// nextReviewStep turns one `review status --next-transition` answer into the driver's next step. The driver only
// follows what Gentle AI offers: it never builds a lineage, a target or a revision of its own.
func nextReviewStep(status []byte) (reviewStep, error) {
	var doc struct {
		Result         *json.RawMessage `json:"result"`
		NextTransition json.RawMessage  `json:"next_transition"`
	}
	if err := json.Unmarshal(status, &doc); err != nil {
		return reviewStep{}, fmt.Errorf("review status is not JSON: %w", err)
	}
	raw := doc.NextTransition
	if doc.Result != nil {
		var inner struct {
			NextTransition json.RawMessage `json:"next_transition"`
		}
		if err := json.Unmarshal(*doc.Result, &inner); err == nil && inner.NextTransition != nil {
			raw = inner.NextTransition
		}
	}
	var t struct {
		Kind    string `json:"kind"`
		Reason  string `json:"reason_code"`
		Execute struct {
			Operation string `json:"operation"`
			Arguments []struct {
				Token string `json:"token"`
			} `json:"arguments"`
		} `json:"execute"`
		Collect struct {
			Inputs []struct {
				Capture   string `json:"capture_operation"`
				Arguments []struct {
					Token string `json:"token"`
				} `json:"arguments"`
			} `json:"inputs"`
		} `json:"collect"`
	}
	if err := json.Unmarshal(raw, &t); err != nil || t.Kind == "" {
		return reviewStep{}, fmt.Errorf("review status offers no transition: %s", tail(string(status), 300))
	}
	step := reviewStep{Reason: t.Reason, Operation: t.Execute.Operation}
	switch t.Kind {
	case "execute":
		args := []string{"review", strings.TrimPrefix(t.Execute.Operation, "review.")}
		for _, a := range t.Execute.Arguments {
			if t.Execute.Operation == "review.start" && strings.HasPrefix(a.Token, "--consent") {
				continue
			}
			args = append(args, a.Token)
		}
		step.Args = args
		step.Kind = "execute"
		if t.Execute.Operation == "review.start" {
			step.Kind = "start"
			step.Args = append(step.Args, "--consent=granted", "--lenses="+gentleReviewLenses, "--lenses-reason=bench: the whole outside view on every case")
		}
	case "collect":
		for _, in := range t.Collect.Inputs {
			if in.Capture != "review.capture-result" {
				// Anything past the reviewer results (a correction plan, a refuter, a validator) is the review's own
				// follow-up: the findings the arm is scored on are already captured.
				step.Kind = "done"
				return step, nil
			}
			args := []string{"review", "capture-result"}
			for _, a := range in.Arguments {
				args = append(args, a.Token)
			}
			step.Captures = append(step.Captures, append(args, "--agent=claude-code"))
		}
		step.Kind = "capture"
	default:
		step.Kind = "stop"
	}
	return step, nil
}

// gentleReviewAgent drives one native review of the candidate commit against its base, headless: consent is
// granted for the exact frozen candidate, every lens is captured through Claude Code, and the run ends once the
// reviewer results are in (or the review is approved). The answer is the reviewers' JSON, one entry per lens.
func gentleReviewAgent(ctx context.Context, ws string, _ Key, opts Options) (AgentResult, error) {
	started := time.Now()
	base, err := gitRun(ws, "rev-parse", "HEAD~1")
	if err != nil {
		return AgentResult{}, errors.New("gentle-review reviews a change: run it with --as-change so the workspace has a base commit")
	}
	base = strings.TrimSpace(base)
	gentle, err := exec.LookPath("gentle-ai")
	if err != nil {
		return AgentResult{}, fmt.Errorf("gentle-review needs gentle-ai on PATH: %w", err)
	}
	realClaude, err := exec.LookPath("claude")
	if err != nil {
		return AgentResult{}, fmt.Errorf("gentle-review needs claude on PATH: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return AgentResult{}, err
	}
	shimDir := filepath.Join(filepath.Dir(ws), "gentle-review-bin")
	logPath := filepath.Join(filepath.Dir(ws), "reviewers.jsonl")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return AgentResult{}, err
	}
	script := fmt.Sprintf("#!/bin/sh\nexec %s \"$@\"\n", shellQuote(self))
	if err := os.WriteFile(filepath.Join(shimDir, "claude"), []byte(script), 0o755); err != nil {
		return AgentResult{}, err
	}
	model := opts.Model
	if model == "" {
		model = DefaultModel(RunnerGentleReview)
	}
	env := append(withoutShimEnv(os.Environ()),
		"PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		shimEnv+"=1", shimRealEnv+"="+realClaude, shimModelEnv+"="+model, shimLogEnv+"="+logPath)
	runGentle := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, gentle, args...)
		cmd.Dir, cmd.Env = ws, env
		boundProcessTree(cmd)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		if err != nil {
			return out.Bytes(), fmt.Errorf("gentle-ai %s: %v: %s", strings.Join(args[:2], " "), err, tail(errb.String()+out.String(), 400))
		}
		return out.Bytes(), nil
	}
	lineage := ""
	var notes []string
	for i := 0; i < 40; i++ {
		statusArgs := []string{"review", "status", "--cwd=" + ws, "--contract=gentle-ai.review-integration/v2", "--next-transition=true"}
		if lineage != "" {
			statusArgs = append(statusArgs, "--lineage="+lineage)
		} else {
			statusArgs = append(statusArgs, "--base-ref="+base, "--committed-only=true")
		}
		out, err := runGentle(statusArgs...)
		if err != nil && len(out) == 0 {
			return reviewResult(logPath, started, notes), err
		}
		step, err := nextReviewStep(out)
		if err != nil {
			return reviewResult(logPath, started, notes), err
		}
		switch step.Kind {
		case "start":
			startOut, err := runGentle(step.Args...)
			if err != nil {
				return reviewResult(logPath, started, notes), err
			}
			var st struct {
				Lineage string `json:"lineage_id"`
			}
			if json.Unmarshal(startOut, &st) != nil || st.Lineage == "" {
				return reviewResult(logPath, started, notes), errors.New("review start returned no lineage")
			}
			lineage = st.Lineage
		case "execute":
			if _, err := runGentle(step.Args...); err != nil {
				return reviewResult(logPath, started, notes), err
			}
			if step.Operation == "review.acknowledge-approved" {
				notes = append(notes, "approved")
				return reviewResult(logPath, started, notes), nil
			}
		case "capture":
			for _, c := range step.Captures {
				if _, err := runGentle(append(c, "--cwd="+ws)...); err != nil {
					return reviewResult(logPath, started, notes), err
				}
			}
		case "done":
			notes = append(notes, "review asks for "+step.Reason)
			return reviewResult(logPath, started, notes), nil
		default:
			notes = append(notes, "review stopped: "+step.Reason)
			return reviewResult(logPath, started, notes), nil
		}
	}
	return reviewResult(logPath, started, notes), errors.New("gentle-review: the review did not settle in 40 transitions")
}

// reviewResult reads the reviewer calls the shim logged into the run's answer: the reviewers' results as one JSON
// array, their summed cost, and one turn per reviewer process.
func reviewResult(logPath string, started time.Time, notes []string) AgentResult {
	data, _ := os.ReadFile(logPath)
	ar := AgentResult{DurationMS: time.Since(started).Milliseconds(), Transcript: data}
	var results []json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var c reviewerCall
		if json.Unmarshal([]byte(line), &c) != nil {
			continue
		}
		ar.CostUSD += c.CostUSD
		ar.Turns++
		if json.Valid([]byte(c.Result)) {
			results = append(results, json.RawMessage(c.Result))
		} else {
			quoted, _ := json.Marshal(c.Result)
			results = append(results, quoted)
		}
	}
	answer, _ := json.Marshal(map[string]any{"reviewers": results, "notes": notes})
	ar.Result = string(answer)
	ar.Raw = tail(string(data), 64*1024)
	return ar
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
