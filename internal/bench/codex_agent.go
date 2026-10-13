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
	"path/filepath"
	"strings"
	"time"
)

// codexAgent spawns the real Codex CLI with the run's prompt and reads its JSON event stream. Codex
// reports token usage but no price, so a codex run records no cost; --max-cost-usd never stops it.
func codexAgent(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	cmd := exec.CommandContext(ctx, "codex", codexArgs(key, opts, ws)...)
	cmd.Dir = ws
	cmd.Env = codexEnv(os.Environ(), opts.ConfigDir, opts.BinDir)
	// Codex reads extra prompt input from stdin when it is open; an empty reader ends that read.
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	started := time.Now()
	err := cmd.Run()
	ar := ParseCodexStream(bytes.NewReader(out.Bytes()))
	ar.DurationMS = time.Since(started).Milliseconds()
	ar.Raw = tail(out.String(), 64*1024) + "\n--- stderr ---\n" + tail(errb.String(), 4*1024)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		ar.TimedOut = true
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ar.ExitCode = ee.ExitCode()
		}
		if !ar.TimedOut {
			return ar, fmt.Errorf("codex: %v: %s", err, tail(errb.String(), 500))
		}
	}
	return ar, nil
}

// codexArgs runs codex non-interactively in the workspace and nowhere else: no user config, no rules
// files, no saved session, and no approval prompt a batch run could never answer. The bench already
// runs every agent in a throwaway workspace, so the sandbox is the workspace itself.
func codexArgs(key Key, opts Options, ws string) []string {
	// Codex runs inside the workspace, so a relative -C would resolve a second time from there.
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	args := []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config",
		"--ignore-rules", "--dangerously-bypass-approvals-and-sandbox", "-C", ws}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	return append(args, agentPrompt(key))
}

// codexEvent is the subset of Codex's exec JSON events a bench run needs: the thread id, the items the
// agent completed, and the failures a turn or the stream reports.
type codexEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Error    struct {
		Message string `json:"message"`
	} `json:"error"`
	Item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
}

// ParseCodexStream reads Codex's exec event stream into the same AgentResult the other parsers fill.
// A codex exec run is one turn of the model that performs many actions, so Turns counts the actions the
// agent completed (commands, file changes, tool calls), the unit closest to the other runners' turns;
// reasoning, plans, and messages are not actions. The last agent message is the answer, and a completed
// turn after a reported error means the run recovered.
func ParseCodexStream(r io.Reader) AgentResult {
	var out AgentResult
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "thread.started":
			if out.SessionID == "" {
				out.SessionID = ev.ThreadID
			}
		case "item.completed":
			switch ev.Item.Type {
			case "agent_message":
				if text := strings.TrimSpace(ev.Item.Text); text != "" {
					out.Result = text
				}
			case "reasoning", "todo_list", "error":
			default:
				out.Turns++
			}
		case "turn.completed":
			out.IsError, out.ErrorText = false, ""
		case "turn.failed":
			out.IsError = true
			out.ErrorText = strings.TrimSpace(ev.Error.Message)
			if out.ErrorText == "" {
				out.ErrorText = "turn failed"
			}
		case "error":
			out.IsError = true
			out.ErrorText = strings.TrimSpace(ev.Message)
		}
	}
	return out
}
