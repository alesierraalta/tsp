package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunnerReview is a reviewer that reads the change and reports what it finds, with no test to write: the role RDD
// plays. It exists so a measurement never depends on one vendor — RDD's lenses run inside Claude Code, which is
// the only reason the RDD arm could not run on another model (#410).
const RunnerReview = "review"

// ReviewPrompt is what the reviewer is asked. The answer shape is the one the plan's findings use, so the same
// matcher scores a reviewer and a test-writing run.
const ReviewPrompt = `Review the change in this repository (git diff against the base commit) for defects it introduces. Report only defects you can point at: a file and line, and one sentence on what goes wrong and for which input. Answer with one JSON object and nothing else: {"findings":[{"location":"path:line","claim":"one sentence","severity":"high|medium|low","causality":"introduced|pre-existing"}]}. If the change introduces no defect, answer {"findings":[]}. Do not modify any file.`

// ScoreReviewText scores a reviewer's answer: the findings JSON it returned is read by the same matcher the plan's
// Findings rows go through, so a reviewer and a test-writing run are measured the same way.
func ScoreReviewText(text string, key Key) (Result, error) {
	answer, err := findingsJSON(text)
	if err != nil {
		return Result{}, err
	}
	line, err := json.Marshal(reviewerCall{Model: "review", Result: answer})
	if err != nil {
		return Result{}, err
	}
	return ScoreReview(append(line, '\n'), key), nil
}

// findingsJSON returns the last {"findings":...} object in the text, which is how a model's prose and its answer
// can arrive together and still be read.
func findingsJSON(text string) (string, error) {
	for i := strings.LastIndex(text, `{"findings"`); i >= 0; i = strings.LastIndex(text[:i], `{"findings"`) {
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err == nil {
			return string(raw), nil
		}
	}
	return "", errors.New("the reviewer answered with no {\"findings\":...} object")
}

// reviewAgent runs the reviewer through the configured runtime (Pi by default, like the other bare arms) in the
// workspace, and keeps its answer as the run's result.
func reviewAgent(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	model := opts.Model
	if model == "" {
		model = DefaultModel(RunnerReview)
	}
	args := []string{"-p", ReviewPrompt, "--mode", "json", "--no-session"}
	if opts.ConfigDir != "" {
		args = append(args, "--no-context-files", "--no-extensions")
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, "pi", args...)
	cmd.Dir = ws
	// The environment goes in before the tree is bounded: bounding adds the run's mark to it, and replacing it
	// afterwards would drop the mark and leave the run's descendants unkillable.
	cmd.Env = piEnv(os.Environ(), opts.ConfigDir, opts.BinDir)
	boundProcessTree(cmd)
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := runBounded(cmd)
	ar := parsePiAnswer(out.Bytes())
	ar.Raw = tail(out.String(), 32*1024) + "\n--- stderr ---\n" + tail(errb.String(), 4*1024)
	ar.Transcript = out.Bytes()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		ar.TimedOut = true
	}
	if err != nil && !ar.TimedOut {
		return ar, fmt.Errorf("pi: %v: %s", err, tail(errb.String(), 400))
	}
	return ar, nil
}

// parsePiAnswer reads Pi's json output: the last assistant text, and the cost it reported when it reports one.
func parsePiAnswer(out []byte) AgentResult {
	ar := AgentResult{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Role string `json:"role"`
			// Pi's json mode wraps the assistant's parts in a message object; a tool result also carries text, and
			// reading that would score the tool's output instead of what the reviewer concluded.
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Result     string  `json:"result"`
			TotalCost  float64 `json:"total_cost_usd"`
			NumTurns   int     `json:"num_turns"`
			CostUSD    float64 `json:"cost_usd"`
			DurationMS int64   `json:"duration_ms"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Result != "" {
			ar.Result = ev.Result
		}
		// Only an assistant message is the reviewer's answer: a tool result also carries text, and reading it
		// would score the tool's output instead of what the reviewer concluded.
		if ev.Message.Role == "assistant" || ev.Role == "assistant" {
			for _, c := range ev.Message.Content {
				if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
					ar.Result = c.Text
				}
			}
			for _, c := range ev.Content {
				if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
					ar.Result = c.Text
				}
			}
		}
		if ev.TotalCost != 0 {
			ar.CostUSD = ev.TotalCost
		}
		if ev.CostUSD != 0 {
			ar.CostUSD = ev.CostUSD
		}
		if ev.NumTurns != 0 {
			ar.Turns = ev.NumTurns
		}
		if ev.DurationMS != 0 {
			ar.DurationMS = ev.DurationMS
		}
	}
	return ar
}
