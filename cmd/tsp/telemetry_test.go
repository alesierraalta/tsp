package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// telemetryRun runs the binary with TSP_HOME pointed at home and returns stdout, stderr and the exit code apart,
// because the contract is that telemetry never touches stdout.
func telemetryRun(t *testing.T, bin, home, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), append([]string{"TSP_HOME=" + home, "TPP_HOME=", "RDD_PLUS_HOME=", "TSP_RUN_ID="}, env...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0
	case errors.As(err, &ee):
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	t.Fatalf("run %v: %v", args, err)
	return "", "", -1
}

func enableTelemetry(t *testing.T, bin, home string) {
	t.Helper()
	if out, errOut, code := telemetryRun(t, bin, home, "", nil, "feature", "enable", "telemetry"); code != 0 {
		t.Fatalf("enable telemetry = %d\n%s%s", code, out, errOut)
	}
}

func ledgerPath(home string) string { return filepath.Join(home, "telemetry", "events.jsonl") }

// readEvents decodes every ledger line, failing the test on a line that is not one JSON object.
func readEvents(t *testing.T, home string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(ledgerPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("ledger line is not one JSON object: %v\n%q", err, line)
		}
		events = append(events, event)
	}
	return events
}

func eventsOfKind(events []map[string]any, kind string) []map[string]any {
	var out []map[string]any
	for _, event := range events {
		if event["kind"] == kind {
			out = append(out, event)
		}
	}
	return out
}

func TestTelemetryOffWritesNothing(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	if _, _, code := telemetryRun(t, bin, home, "", nil, "version"); code != 0 {
		t.Fatalf("version exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, "telemetry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("telemetry off created %s (err %v)", filepath.Join(home, "telemetry"), err)
	}
	valid := `{"source":"pi","host":{"turns":3}}`
	if out, errOut, code := telemetryRun(t, bin, home, valid, nil, "telemetry", "record"); code != 0 || out != "" {
		t.Fatalf("record with telemetry off = %d %q %q, want 0 and silence", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "telemetry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record with telemetry off created the telemetry directory (err %v)", err)
	}
}

func TestTelemetryCommandEvent(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	plain, _, code := telemetryRun(t, bin, home, "", nil, "version")
	if code != 0 {
		t.Fatalf("version exit = %d", code)
	}
	enableTelemetry(t, bin, home)

	out, errOut, code := telemetryRun(t, bin, home, "", []string{"TSP_RUN_ID=run-42"}, "version")
	if code != 0 || out != plain || errOut != "" {
		t.Fatalf("version with telemetry on = %d %q %q, want %q unchanged and no stderr", code, out, errOut, plain)
	}
	_, _, planCode := telemetryRun(t, bin, home, "", nil, "plan", "check", "--path=secret-dir/plan.md", "--run", "secret-run")

	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 2 {
		t.Fatalf("got %d command events, want 2 (version, plan check): %v", len(commands), commands)
	}
	version := commands[0]
	for key, want := range map[string]any{"schema": 1.0, "source": "tsp", "run_id": "run-42"} {
		if version[key] != want {
			t.Errorf("version event %s = %v, want %v", key, version[key], want)
		}
	}
	for _, key := range []string{"ts", "version", "commit", "skill", "os", "arch", "cpus", "machine"} {
		if _, ok := version[key]; !ok {
			t.Errorf("version event lacks %s: %v", key, version)
		}
	}
	command := version["command"].(map[string]any)
	if command["name"] != "version" || command["exit"] != 0.0 {
		t.Errorf("version command = %v", command)
	}
	if _, ok := command["duration_ms"]; !ok {
		t.Errorf("version command lacks duration_ms: %v", command)
	}

	plan := commands[1]
	if _, ok := plan["run_id"]; ok {
		t.Errorf("an unset TSP_RUN_ID must leave run_id absent, got %v", plan["run_id"])
	}
	pc := plan["command"].(map[string]any)
	if pc["name"] != "plan" || pc["sub"] != "check" || pc["exit"] != float64(planCode) {
		t.Errorf("plan command = %v, want plan/check exit %d", pc, planCode)
	}
	flags, _ := json.Marshal(pc["flags"])
	if string(flags) != `["path","run"]` {
		t.Errorf("plan flags = %s, want only the names [path run]", flags)
	}
	raw, _ := os.ReadFile(ledgerPath(home))
	for _, secret := range []string{"secret-dir", "secret-run", home} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("ledger carries an argument value %q:\n%s", secret, raw)
		}
	}
}

func TestTelemetryFailureLeavesTheCommandAlone(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	plain, _, _ := telemetryRun(t, bin, home, "", nil, "version")
	enableTelemetry(t, bin, home)
	if err := os.WriteFile(filepath.Join(home, "telemetry"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := telemetryRun(t, bin, home, "", nil, "version")
	if code != 0 || out != plain {
		t.Fatalf("version with a broken ledger = %d %q, want 0 %q", code, out, plain)
	}
	if lines := strings.Split(strings.TrimSpace(errOut), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "tsp telemetry:") {
		t.Fatalf("stderr = %q, want one line starting with \"tsp telemetry:\"", errOut)
	}
}

func TestTelemetryRecordHostEvent(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)

	valid := `{"source":"pi","run_id":"run-7","session":"sess-abc","host":{"model":"anthropic/claude-haiku-5-5","turns":3,"tool_calls":5,"tool_errors":0,"wall_ms":1200,"tokens":{"input":10,"output":20,"cache_read":300,"cache_write":40},"per_turn":[{"input":10,"cache_read":300}],"tools":{"read":3,"bash":2},"offer":"accepted"}}`
	out, errOut, code := telemetryRun(t, bin, home, valid, nil, "telemetry", "record")
	if code != 0 || out != "" || errOut != "" {
		t.Fatalf("valid record = %d %q %q", code, out, errOut)
	}
	hosts := eventsOfKind(readEvents(t, home), "host_run")
	if len(hosts) != 1 {
		t.Fatalf("got %d host_run events, want 1", len(hosts))
	}
	event := hosts[0]
	if event["source"] != "pi" || event["run_id"] != "run-7" {
		t.Errorf("host envelope = %v", event)
	}
	if session, _ := event["session"].(string); !strings.HasPrefix(session, "session-") || strings.Contains(session, "sess-abc") {
		t.Errorf("session = %q, want a salted pseudonym", session)
	}
	host := event["host"].(map[string]any)
	if host["turns"] != 3.0 || host["tool_errors"] != 0.0 || host["offer"] != "accepted" {
		t.Errorf("host = %v", host)
	}
	if _, ok := host["compactions"]; ok {
		t.Errorf("an unmeasured count must stay absent, got compactions=%v", host["compactions"])
	}
	turn := host["per_turn"].([]any)[0].(map[string]any)
	if _, ok := turn["output"]; ok || turn["cache_read"] != 300.0 {
		t.Errorf("per_turn[0] = %v, want cache_read 300 and no output", turn)
	}

	for name, body := range map[string]string{
		"unknown key":       `{"source":"pi","host":{"turns":1,"prompt":"fix the bug"}}`,
		"free text model":   `{"source":"pi","host":{"model":"tell me a story"}}`,
		"negative count":    `{"source":"pi","host":{"turns":-1}}`,
		"unknown source":    `{"source":"cursor","host":{"turns":1}}`,
		"unknown offer":     `{"source":"pi","host":{"offer":"maybe"}}`,
		"tool name text":    `{"source":"pi","host":{"tools":{"rm -rf /":1}}}`,
		"two objects":       `{"source":"pi","host":{}} {"source":"pi","host":{}}`,
		"not json":          `turns=3`,
		"run id with space": `{"source":"pi","run_id":"a b","host":{}}`,
	} {
		out, errOut, code := telemetryRun(t, bin, home, body, nil, "telemetry", "record")
		if code != 2 || out != "" || !strings.HasPrefix(errOut, "tsp telemetry record:") {
			t.Errorf("%s: record = %d %q %q, want exit 2, no stdout, a tsp telemetry record: refusal", name, code, out, errOut)
		}
	}
	if got := len(eventsOfKind(readEvents(t, home), "host_run")); got != 1 {
		t.Errorf("refused records wrote host events: %d host_run events, want 1", got)
	}
	raw, _ := os.ReadFile(ledgerPath(home))
	if strings.Contains(string(raw), "fix the bug") || strings.Contains(string(raw), "story") {
		t.Errorf("refused content reached the ledger:\n%s", raw)
	}
}

func TestTelemetryConcurrentWritersKeepWholeLines(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(bin, "version")
			cmd.Env = append(os.Environ(), "TSP_HOME="+home, "TPP_HOME=", "RDD_PLUS_HOME=")
			_ = cmd.Run()
		}()
	}
	wg.Wait()
	if got := len(eventsOfKind(readEvents(t, home), "command")); got != n {
		t.Fatalf("got %d command events from %d concurrent runs", got, n)
	}
}

// telemetryRunConfig runs with only XDG_CONFIG_HOME set, so the state root is whatever the default
// resolution answers and the legacy-migration path is the one under test.
func telemetryRunConfig(t *testing.T, bin, configDir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	env := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "TSP_HOME=") || strings.HasPrefix(entry, "TPP_HOME=") ||
			strings.HasPrefix(entry, "RDD_PLUS_HOME=") || strings.HasPrefix(entry, "XDG_CONFIG_HOME=") ||
			strings.HasPrefix(entry, "TSP_RUN_ID=") {
			continue
		}
		env = append(env, entry)
	}
	cmd.Env = append(env, "XDG_CONFIG_HOME="+configDir, "TSP_HOME=", "TPP_HOME=", "RDD_PLUS_HOME=", "TSP_RUN_ID=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0
	case errors.As(err, &ee):
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	t.Fatalf("run %v: %v", args, err)
	return "", "", -1
}

// D1: an argument that is not a dispatched command is never stored as the command name.
func TestTelemetryUnknownCommandName(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	secret := "/etc/secret-private-plan"
	if _, _, code := telemetryRun(t, bin, home, "", nil, secret); code != 2 {
		t.Fatalf("unknown command exit = %d, want 2", code)
	}
	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 1 {
		t.Fatalf("got %d command events, want 1", len(commands))
	}
	command := commands[0]["command"].(map[string]any)
	if command["name"] != "unknown" {
		t.Errorf("command name = %v, want unknown", command["name"])
	}
	raw, _ := os.ReadFile(ledgerPath(home))
	if strings.Contains(string(raw), "secret-private") {
		t.Errorf("ledger carries the unknown argument:\n%s", raw)
	}
}

// M1: a bare invocation is still one command event, under a stable unknown name.
func TestTelemetryBareInvocationRecordsUnknownName(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	_, _, code := telemetryRun(t, bin, home, "", nil)
	if code != 2 {
		t.Fatalf("bare tsp exit = %d, want 2", code)
	}
	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 1 {
		t.Fatalf("got %d command events, want 1", len(commands))
	}
	command := commands[0]["command"].(map[string]any)
	if command["name"] != "unknown" || command["exit"] != float64(code) {
		t.Errorf("bare command = %v, want name unknown and exit %d", command, code)
	}
}

// M2: stats and export are not part of this slice, so they are neither accepted nor named as subs.
func TestTelemetryUnimplementedSubcommandsAreNotRecorded(t *testing.T) {
	bin := buildCLI(t)
	for _, sub := range []string{"stats", "export"} {
		sub := sub
		t.Run(sub, func(t *testing.T) {
			home := t.TempDir()
			enableTelemetry(t, bin, home)
			_, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", sub)
			if code != 2 || errOut == "" {
				t.Fatalf("telemetry %s = %d %q, want exit 2 and usage on stderr", sub, code, errOut)
			}
			commands := eventsOfKind(readEvents(t, home), "command")
			if len(commands) != 1 {
				t.Fatalf("got %d command events, want 1", len(commands))
			}
			command := commands[0]["command"].(map[string]any)
			if command["name"] != "telemetry" {
				t.Errorf("command name = %v, want telemetry", command["name"])
			}
			if got, ok := command["sub"]; ok {
				t.Errorf("command sub = %v, want absent for unimplemented %s", got, sub)
			}
		})
	}
}

// M3: help must describe the write-failure behavior the CLI actually keeps: exit 0 and one warning.
func TestTelemetryHelpMatchesWriteFailureBehavior(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	if err := os.WriteFile(filepath.Join(home, "telemetry"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "--help")
	if code != 0 || out != "" {
		t.Fatalf("help with a broken ledger = %d %q, want exit 0 and no stdout", code, out)
	}
	if !strings.Contains(errOut, "Usage of telemetry:") {
		t.Fatalf("help is missing from stderr: %q", errOut)
	}
	if strings.Contains(errOut, "1: the ledger could not be written") {
		t.Errorf("help still promises exit 1 on a write failure: %q", errOut)
	}
	if !strings.Contains(errOut, "Exit 0: accepted") || !strings.Contains(errOut, "one stderr warning") {
		t.Errorf("help does not state exit 0 plus one warning: %q", errOut)
	}
	var warnings []string
	for _, line := range strings.Split(strings.TrimRight(errOut, "\n"), "\n") {
		if strings.HasPrefix(line, "tsp telemetry:") {
			warnings = append(warnings, line)
		}
	}
	if len(warnings) != 1 {
		t.Errorf("got %d telemetry warnings, want 1: %q", len(warnings), errOut)
	}
}

// D8: a usage prose word is not a nested subcommand.
func TestTelemetrySubcommandIsNotAUsageWord(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	if _, _, code := telemetryRun(t, bin, home, "", nil, "telemetry", "reads"); code != 2 {
		t.Fatalf("telemetry reads exit = %d, want 2", code)
	}
	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 1 {
		t.Fatalf("got %d command events, want 1", len(commands))
	}
	command := commands[0]["command"].(map[string]any)
	if command["name"] != "telemetry" {
		t.Errorf("command name = %v, want telemetry", command["name"])
	}
	if sub, ok := command["sub"]; ok {
		t.Errorf("command sub = %v, want absent for an undocumented word", sub)
	}
}

// D10: with telemetry never enabled, reading the gate must not migrate a legacy root or create one.
func TestTelemetryOffNeverMigratesTheStateRoot(t *testing.T) {
	bin := buildCLI(t)
	configDir := t.TempDir()
	legacy := filepath.Join(configDir, "tpp")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schemaVersion":1}`
	if err := os.WriteFile(filepath.Join(legacy, "state.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := telemetryRunConfig(t, bin, configDir, "version")
	if code != 0 || out == "" || errOut != "" {
		t.Fatalf("version = %d %q %q, want 0, a version and no stderr", code, out, errOut)
	}
	info, err := os.Lstat(legacy)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("legacy tpp root changed: %v %v", info, err)
	}
	if got, err := os.ReadFile(filepath.Join(legacy, "state.json")); err != nil || string(got) != body {
		t.Fatalf("legacy state = %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(configDir, "tsp")); !os.IsNotExist(err) {
		t.Fatalf("telemetry off created a tsp root: %v", err)
	}
}

// D11: a corrupt state file reads as telemetry off, silently, and never changes the run.
func TestTelemetryCorruptStateIsSilentlyOff(t *testing.T) {
	bin := buildCLI(t)
	configDir := t.TempDir()
	root := filepath.Join(configDir, "tsp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := telemetryRunConfig(t, bin, configDir, "version")
	if code != 0 || out == "" || errOut != "" {
		t.Fatalf("version with corrupt state = %d %q %q, want 0, a version and no stderr", code, out, errOut)
	}
}

// D12: TSP_RUN_ID is stored only when it is an identifier; anything else stays absent.
func TestTelemetryRunIDMustBeAnIdentifier(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	secret := "bad id/with private text"
	telemetryRun(t, bin, home, "", []string{"TSP_RUN_ID=" + secret}, "version")
	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 1 {
		t.Fatalf("got %d command events, want 1", len(commands))
	}
	if runID, ok := commands[0]["run_id"]; ok {
		t.Errorf("run_id = %v, want absent for a value outside the pattern", runID)
	}
	raw, _ := os.ReadFile(ledgerPath(home))
	if strings.Contains(string(raw), "private text") {
		t.Errorf("ledger carries the invalid run id:\n%s", raw)
	}
}

// D14: every command the main dispatch accepts records its own name, not unknown.
func TestTelemetryDispatchedCommandIsAlwaysNamed(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	telemetryRunIn(t, t.TempDir(), bin, home, "", nil, "pin", "--path", "plan.md")
	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 1 {
		t.Fatalf("got %d command events, want 1", len(commands))
	}
	command := commands[0]["command"].(map[string]any)
	if command["name"] != "pin" {
		t.Errorf("command name = %v, want pin", command["name"])
	}
}

// D15: the flags each subcommand's own help text documents are recorded, while an undocumented
// dash-prefixed value stays out.
func TestTelemetryFlagsFromEveryHelpText(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	dir := t.TempDir()
	telemetryRunIn(t, dir, bin, home, "", nil, "check", "--cwd", dir, "--paths", "a.go")
	telemetryRunIn(t, dir, bin, home, "", nil, "probe", "quality", "--cwd", dir, "--max-complexity", "5")
	telemetryRunIn(t, dir, bin, home, "", nil, "findings", "record", "--cwd", dir, "--class", "security", "--since", "HEAD~1")
	telemetryRunIn(t, dir, bin, home, "", nil, "feedback", "--file", "-private")

	commands := eventsOfKind(readEvents(t, home), "command")
	if len(commands) != 4 {
		t.Fatalf("got %d command events, want 4", len(commands))
	}
	want := [][]string{
		{"cwd", "paths"},
		{"cwd", "max-complexity"},
		{"cwd", "class", "since"},
		{"file"},
	}
	for i, expected := range want {
		command := commands[i]["command"].(map[string]any)
		raw, _ := json.Marshal(command["flags"])
		var got []string
		_ = json.Unmarshal(raw, &got)
		if strings.Join(got, ",") != strings.Join(expected, ",") {
			t.Errorf("command %d flags = %v, want %v", i, got, expected)
		}
	}
}
