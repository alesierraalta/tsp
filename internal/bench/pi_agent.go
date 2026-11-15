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

// piAgent spawns the real Pi CLI with the run's prompt and reads its JSON event stream, mirroring
// claudeAgent. Pi has no turn cap flag, so --timeout is the only wall-clock limit this runner
// enforces; --max-turns applies to the claude runner alone.
func piAgent(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	cmd := exec.CommandContext(ctx, "pi", piArgs(key, opts)...)
	cmd.Dir = ws
	cmd.Env = piEnv(os.Environ(), opts.ConfigDir, opts.BinDir)
	if opts.ConfigMode == ConfigProduct {
		// The extension acts on the gate it reads from its children: without the observation a print-mode
		// session has nobody to answer the offer, and the arm would measure the skill alone again.
		cmd.Env = piProductEnv(os.Environ(), opts.ConfigDir, opts.BinDir)
	}
	boundProcessTree(cmd)
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	started := time.Now()
	err := runBounded(cmd)
	ar := ParsePiStream(bytes.NewReader(out.Bytes()))
	// Pi's stream carries no duration; the run's own clock is the only measure of it.
	ar.DurationMS = time.Since(started).Milliseconds()
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
			// An authentication or model failure is reported on stderr, not as an event: Pi prints
			// the reason and exits non-zero. The run must carry that text, not a bare exit status.
			return ar, fmt.Errorf("pi: %v: %s", err, tail(errb.String(), 500))
		}
	}
	return ar, nil
}

// piArgs builds the Pi invocation. A run with its own config reads no AGENTS.md or CLAUDE.md from the
// workspace's ancestors and loads no discovered or built-in extension, and a bare run (ConfigNone) disables every
// discovered and configured skill. The product arm names the one extension it does load — the tsp adapter, at the
// path `tsp sync --hosts pi` installs it — so discovery stays off and nothing of the operator's can ride along.
func piArgs(key Key, opts Options) []string {
	args := []string{"-p", agentPrompt(key, opts), "--mode", "json", "--no-session"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ConfigDir != "" {
		args = append(args, "--no-context-files", "--no-extensions")
		switch opts.ConfigMode {
		case ConfigNone:
			args = append(args, "--no-skills")
		case ConfigProduct:
			args = append(args, "-e", PiExtensionFile(opts.ConfigDir))
		}
	}
	return args
}

// piContentPart is one part of an assistant message; the text parts are the answer, thinking and
// tool calls are not.
type piContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// piEvent is the subset of Pi's JSON event union (docs/json.md) that says what a bench run needs:
// the session header carrying the session id, the assistant messages carrying content, usage, and
// the stop reason, and the turn ends that count turns. The field names are Pi's wire names.
type piEvent struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Message struct {
		Role         string          `json:"role"`
		Content      []piContentPart `json:"content"`
		Usage        piUsage         `json:"usage"`
		StopReason   string          `json:"stopReason"`
		ErrorMessage string          `json:"errorMessage"`
	} `json:"message"`
}

// piUsage is the per-message usage Pi reports; the cost is what a paid model spends on that
// message and the counters the tokens it read and wrote, so the run's cost and tokens are their sums.
type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
	Cost       struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

// ParsePiStream reads Pi's JSON event stream into the same AgentResult the Claude parser fills.
// Each line is an event; a line that is not JSON is ignored, so a partial stream still yields what
// it has. Cost is summed over the assistant messages, each of which reports its own usage, and
// turns are the completed turns. Pi reports a failed model call as an assistant message whose stop
// reason is "error" (its print mode prints that message's reason to stderr), so an errored message
// is what an error event looks like here; the last assistant message decides the outcome.
func ParsePiStream(r io.Reader) AgentResult {
	var out AgentResult
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev piEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "session":
			if out.SessionID == "" {
				out.SessionID = ev.ID
			}
		case "turn_end":
			out.Turns++
		case "message_end":
			if ev.Message.Role != "assistant" {
				continue
			}
			u := ev.Message.Usage
			out.CostUSD += u.Cost.Total
			out.Tokens.Add(Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Reasoning: u.Reasoning})
			if ev.Message.StopReason == "error" || ev.Message.StopReason == "aborted" {
				out.IsError = true
				out.ErrorText = strings.TrimSpace(ev.Message.ErrorMessage)
				if out.ErrorText == "" {
					out.ErrorText = "request " + ev.Message.StopReason
				}
				continue
			}
			out.IsError, out.ErrorText = false, ""
			if text := piText(ev.Message.Content); text != "" {
				out.Result = text
			}
		}
	}
	return out
}

// piText joins the text parts of an assistant message, ignoring thinking and tool calls.
func piText(content []piContentPart) string {
	var b strings.Builder
	for _, part := range content {
		if part.Type == "text" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
