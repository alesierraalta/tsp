package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// claudeGateTranscript is the shape Claude Code writes: one line per assistant content block, each
// repeating the same message.id and usage, tool calls by name, one failed tool result, and a second
// assistant message. The Skill call is the TSP-run evidence the gate already looks for.
func claudeGateTranscript() string {
	return strings.Join([]string{
		`{"type":"user","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"run tsp"}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[{"type":"text","text":"working"}]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[{"type":"tool_use","id":"toolu_2","name":"Skill","input":{"skill":"tsp"}}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"boom"}]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:04Z","message":{"id":"msg_2","model":"claude-sonnet-4","usage":{"input_tokens":3,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[{"type":"text","text":"done"}]}}`,
	}, "\n") + "\n"
}

// claudeGateNextTurn is the traffic a later Stop must count, and only it.
func claudeGateNextTurn() string {
	return `{"type":"assistant","timestamp":"2026-01-01T00:00:10Z","message":{"id":"msg_3","model":"claude-sonnet-4","usage":{"input_tokens":7,"output_tokens":8,"cache_read_input_tokens":200,"cache_creation_input_tokens":9},"content":[{"type":"text","text":"again"}]}}` + "\n" +
		`{"type":"user","timestamp":"2026-01-01T00:00:11Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_3","content":"ok"}]}}` + "\n"
}

func writeClaudeTranscript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

func gatePayload(session, transcript, cwd string) string {
	return `{"session_id":"` + session + `","transcript_path":"` + transcript + `","cwd":"` + cwd + `"}`
}

// Two Stops over one growing transcript: the first host_run counts its deduped turns, the second
// only the turns the cursor has not seen. The gate's stdout and exit stay exactly as before.
func TestTelemetryGateRecordsClaudeUsageOnlyOnce(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	gateLog := filepath.Join(dir, "gate", "testing-gate.jsonl")
	env := []string{"TESTING_GATE_LOG=" + gateLog}
	payload := gatePayload("sess-claude-1", transcript, dir)

	writeClaudeTranscript(t, transcript, claudeGateTranscript())
	if out, errOut, code := telemetryRun(t, bin, home, payload, env, "gate"); code != 0 || out != "" {
		t.Fatalf("first gate stop = %d %q %q, want 0 and no stdout", code, out, errOut)
	}
	hosts := eventsOfKind(readEvents(t, home), "host_run")
	if len(hosts) != 1 {
		t.Fatalf("got %d host_run events after the first stop, want 1", len(hosts))
	}
	first := hosts[0]
	if first["source"] != "claude" {
		t.Fatalf("source = %v, want claude", first["source"])
	}
	session, _ := first["session"].(string)
	if !strings.HasPrefix(session, "session-") || strings.Contains(session, "sess-claude-1") {
		t.Fatalf("session = %q, want a salted pseudonym", session)
	}
	host := first["host"].(map[string]any)
	if host["turns"] != 2.0 || host["tool_calls"] != 2.0 || host["tool_errors"] != 1.0 {
		t.Fatalf("first host = %v, want 2 deduped turns, 2 tool calls and 1 error", host)
	}
	if host["model"] != "claude-sonnet-4" || host["wall_ms"] != 4000.0 {
		t.Fatalf("first host = %v, want model and 4000 wall ms", host)
	}
	tokens := host["tokens"].(map[string]any)
	if tokens["input"] != 13.0 || tokens["output"] != 6.0 || tokens["cache_read"] != 100.0 || tokens["cache_write"] != 5.0 {
		t.Fatalf("first tokens = %v, want the repeated usage counted once", tokens)
	}

	writeClaudeTranscript(t, transcript, claudeGateTranscript()+claudeGateNextTurn())
	if out, errOut, code := telemetryRun(t, bin, home, payload, env, "gate"); code != 0 || out != "" {
		t.Fatalf("second gate stop = %d %q %q, want 0 and no stdout", code, out, errOut)
	}
	hosts = eventsOfKind(readEvents(t, home), "host_run")
	if len(hosts) != 2 {
		t.Fatalf("got %d host_run events after the second stop, want 2", len(hosts))
	}
	second := hosts[1]["host"].(map[string]any)
	if second["turns"] != 1.0 {
		t.Fatalf("second turns = %v, want only the new turn", second["turns"])
	}
	secondTokens := second["tokens"].(map[string]any)
	if secondTokens["input"] != 7.0 || secondTokens["output"] != 8.0 {
		t.Fatalf("second tokens = %v, want only msg_3", secondTokens)
	}

	entries, err := os.ReadDir(filepath.Join(home, "telemetry"))
	if err != nil {
		t.Fatalf("read telemetry dir: %v", err)
	}
	cursors := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "cursor-") {
			cursors++
			if strings.Contains(entry.Name(), "sess-claude-1") {
				t.Fatalf("cursor file %q carries the raw session id", entry.Name())
			}
		}
	}
	if cursors != 1 {
		t.Fatalf("got %d cursor files, want one per session: %v", cursors, entries)
	}
	raw, err := os.ReadFile(ledgerPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sess-claude-1") {
		t.Fatalf("the ledger carries the raw session id:\n%s", raw)
	}
}

// Two Stops that race over one session and one transcript count its turns once in total: the cursor
// is read, advanced and written under one exclusive lock, so the loser sees the advanced cursor.
func TestTelemetryGateConcurrentStopsCountTurnsOnce(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	writeClaudeTranscript(t, transcript, claudeGateTranscript())
	payload := gatePayload("sess-claude-1", transcript, dir)

	type result struct {
		out, errOut string
		code        int
		err         error
	}
	run := func(logName string) result {
		cmd := exec.Command(bin, "gate")
		cmd.Env = append(os.Environ(), "TSP_HOME="+home, "TPP_HOME=", "RDD_PLUS_HOME=", "TSP_RUN_ID=",
			"TESTING_GATE_LOG="+filepath.Join(dir, logName))
		cmd.Stdin = strings.NewReader(payload)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var ee *exec.ExitError
		code := 0
		if errors.As(err, &ee) {
			code = ee.ExitCode()
			err = nil
		}
		return result{out: stdout.String(), errOut: stderr.String(), code: code, err: err}
	}

	results := make(chan result, 2)
	start := make(chan struct{})
	for _, name := range []string{"gate-a.jsonl", "gate-b.jsonl"} {
		go func(name string) {
			<-start
			results <- run(name)
		}(name)
	}
	close(start)
	for i := 0; i < 2; i++ {
		if got := <-results; got.err != nil || got.code != 0 || got.out != "" {
			t.Fatalf("concurrent gate stop = %d %q %q (%v), want 0 and no stdout", got.code, got.out, got.errOut, got.err)
		}
	}

	total := 0.0
	for _, event := range eventsOfKind(readEvents(t, home), "host_run") {
		host, ok := event["host"].(map[string]any)
		if !ok {
			continue
		}
		if turns, ok := host["turns"].(float64); ok {
			total += turns
		}
	}
	if total != 2.0 {
		t.Fatalf("concurrent Stops counted %v turns in total, want the transcript's 2 counted once", total)
	}
}

// Telemetry off writes nothing at all: no ledger, no salt, and no cursor either.
func TestTelemetryGateOffWritesNothing(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	gateLog := filepath.Join(dir, "gate", "testing-gate.jsonl")
	writeClaudeTranscript(t, transcript, claudeGateTranscript())
	payload := gatePayload("sess-claude-1", transcript, dir)
	if out, errOut, code := telemetryRun(t, bin, home, payload, []string{"TESTING_GATE_LOG=" + gateLog}, "gate"); code != 0 || out != "" {
		t.Fatalf("gate stop with telemetry off = %d %q %q, want 0 and no stdout", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "telemetry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("telemetry off created %s (err %v)", filepath.Join(home, "telemetry"), err)
	}
}

// owedPlan is the minimum plan the gate reports as owing a layer sweep for one bound run.
const owedPlan = "## Layer matrix\n\n| Layer | Skill | Scope | Status | Run |\n|---|---|---|---|---|\n" +
	"| Security | `appsec-adversarial-auditor` | input | pending | run-t1 |\n\n" +
	"## Ranked targets\n\n| Target | Verdict | Status | Run |\n|---|---|---|---|\n" +
	"| 1. auth | probe | pending | run-t1 |\n"

// runAuditedGate runs one real Stop that audits a bound plan and answers the hook's stdout and exit.
func runAuditedGate(t *testing.T, bin, home string) (string, int) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	rel := filepath.Join("docs", "testing", "test-plan.md")
	if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, rel), []byte(owedPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(base, "transcript.jsonl")
	writeClaudeTranscript(t, transcript, claudeGateTranscript())
	logPath := filepath.Join(base, "gate", "testing-gate.jsonl")
	env := []string{"TESTING_GATE_LOG=" + logPath}
	if _, errOut, code := telemetryRun(t, bin, home, "", env, "bind", "--session", "sess-audit", "--path", rel, "--run", "run-t1", "--cwd", repo); code != 0 {
		t.Fatalf("bind = %d: %s", code, errOut)
	}
	payload := gatePayload("sess-audit", transcript, repo)
	out, errOut, code := telemetryRun(t, bin, home, payload, env, "gate")
	if errOut != "" {
		t.Fatalf("gate stderr = %q, want silence", errOut)
	}
	return out, code
}

// The gate's decision, stdout and exit are byte-identical with telemetry on and off; only the
// ledger gains a line.
func TestTelemetryGateOutputIsIdenticalOnAndOff(t *testing.T) {
	bin := buildCLI(t)
	offHome, onHome := t.TempDir(), t.TempDir()
	enableTelemetry(t, bin, onHome)
	offOut, offCode := runAuditedGate(t, bin, offHome)
	onOut, onCode := runAuditedGate(t, bin, onHome)
	if offOut != onOut || offCode != onCode {
		t.Fatalf("gate output changed with telemetry on:\noff %d %q\non  %d %q", offCode, offOut, onCode, onOut)
	}
	if !strings.Contains(onOut, "layer(s) assigned and never invoked") {
		t.Fatalf("audit stdout = %q, want the owed-layer line", onOut)
	}
	if got := len(eventsOfKind(readEvents(t, onHome), "host_run")); got != 1 {
		t.Fatalf("got %d host_run events, want the audited stop recorded once", got)
	}
}
