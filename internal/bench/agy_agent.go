package bench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// agyAgent runs Antigravity headlessly in the workspace. It reports no price and has no
// turn-cap flag: MaxTurns and --max-cost-usd do not apply; the wall-clock timeout still does, and it ends the
// commands agy started too: they outlive agy otherwise, and one holding the output pipe keeps Run from returning.
func agyAgent(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	cmd := exec.CommandContext(ctx, "agy", agyArgs(key, opts)...)
	boundProcessTree(cmd)
	cmd.Dir = ws
	cmd.Env = agyEnv(os.Environ(), opts.ConfigDir, opts.BinDir)
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	started := time.Now()
	err := cmd.Run()
	ar := ParseAgyStream(bytes.NewReader(out.Bytes()))
	if ar.DurationMS == 0 {
		ar.DurationMS = time.Since(started).Milliseconds()
	}
	ar.Raw = tail(out.String(), 64*1024) + "\n--- stderr ---\n" + tail(errb.String(), 4*1024)
	ar.Transcript = out.Bytes()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		ar.TimedOut = true
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ar.ExitCode = ee.ExitCode()
		}
		if !ar.TimedOut {
			return ar, fmt.Errorf("agy: %v: %s", err, tail(errb.String(), 500))
		}
	}
	return ar, nil
}

func agyArgs(key Key, opts Options) []string {
	args := []string{"-p", agentPrompt(key, opts), "--output-format", "stream-json", "--dangerously-skip-permissions"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

// ParseAgyStream counts completed tool actions, not num_turns (which counts user turns).
// The result status is authoritative: Antigravity can exit zero on authentication failure.
func ParseAgyStream(r io.Reader) AgentResult {
	var out AgentResult
	done := false
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var ev struct {
			Event          string `json:"event"`
			ConversationID string `json:"conversation_id"`
			Step           struct {
				State string `json:"state"`
				Type  string `json:"step_type"`
			} `json:"step_update"`
			Result struct {
				ConversationID string  `json:"conversation_id"`
				Status         string  `json:"status"`
				Response       string  `json:"response"`
				Error          string  `json:"error"`
				Duration       float64 `json:"duration_seconds"`
			} `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if out.SessionID == "" {
			out.SessionID = ev.ConversationID
		}
		switch ev.Event {
		case "step_update":
			if ev.Step.Type == "tool" && ev.Step.State == "DONE" {
				out.Turns++
			}
		case "result":
			done = true
			if out.SessionID == "" {
				out.SessionID = ev.Result.ConversationID
			}
			out.Result = strings.TrimSpace(ev.Result.Response)
			out.DurationMS = int64(ev.Result.Duration * 1000)
			out.IsError = ev.Result.Status != "SUCCESS"
			out.ErrorText = ""
			if out.IsError {
				out.ErrorText = strings.TrimSpace(ev.Result.Error)
				if out.ErrorText == "" {
					out.ErrorText = "agy result did not succeed"
				}
			}
		}
	}
	if !done {
		out.IsError = true
		out.ErrorText = "agy stream ended without a result event"
	}
	if err := sc.Err(); err != nil {
		out.IsError = true
		out.ErrorText = "agy stream: " + err.Error()
	}
	return out
}
