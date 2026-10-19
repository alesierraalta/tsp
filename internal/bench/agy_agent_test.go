package bench

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeAgySource builds a stand-in operator ~/.gemini/antigravity-cli holding the login plus the operator's own
// instructions and skills beside it, which an isolated run must never carry.
func writeAgySource(t *testing.T) string {
	t.Helper()
	gemini := filepath.Join(t.TempDir(), ".gemini")
	src := filepath.Join(gemini, "antigravity-cli")
	for name, body := range map[string]string{
		filepath.Join(src, AgyAuthFile):                                   `{"access_token":"operator-token"}`,
		filepath.Join(gemini, "GEMINI.md"):                                "operator instructions",
		filepath.Join(gemini, "config", "skills", "op-skill", "SKILL.md"): "---\nname: op-skill\n---\n",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func TestAgyIsAKnownRunnerWithItsOwnAgent(t *testing.T) {
	if !KnownRunner(RunnerAgy) {
		t.Fatal("agy must be a runner the bench can spawn")
	}
	if got := reflect.ValueOf(runnerAgent(RunnerAgy)).Pointer(); got != reflect.ValueOf(agyAgent).Pointer() {
		t.Fatal("RunnerAgy must select agyAgent")
	}
	if DefaultModel(RunnerAgy) != "gemini-3.8-flash-low" {
		t.Fatalf("agy default model = %q", DefaultModel(RunnerAgy))
	}
}

// agy has no config-dir variable and reads everything from HOME, so both arms live in the config's own home: the
// login copied (never linked, F25) and, for the tsp arm, the skill under test and nothing else.
func TestAgyConfigsHoldOnlyTheLoginAndTheSkillUnderTest(t *testing.T) {
	src := writeAgySource(t)
	token := filepath.Join(IsolatedHomeDir, ".gemini", "antigravity-cli", AgyAuthFile)
	assertLogin := func(cfg string) {
		t.Helper()
		st, err := os.Lstat(filepath.Join(cfg, token))
		if err != nil {
			t.Fatalf("the agy config must carry the login at %s: %v", token, err)
		}
		if st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm() != 0o600 {
			t.Fatalf("the login must be an owner-only copy, got mode %v", st.Mode())
		}
		if data, _ := os.ReadFile(filepath.Join(cfg, token)); string(data) != `{"access_token":"operator-token"}` {
			t.Fatalf("the login copy differs from the operator's: %q", data)
		}
		if _, err := os.Stat(filepath.Join(cfg, IsolatedHomeDir, ".gemini", "GEMINI.md")); err == nil {
			t.Fatal("the operator's GEMINI.md must not reach the run")
		}
	}

	bare := filepath.Join(t.TempDir(), "agy-none")
	if err := WriteNoneConfig(bare, RunnerAgy, src); err != nil {
		t.Fatal(err)
	}
	if got := dirNames(t, bare); got != IsolatedHomeDir {
		t.Fatalf("agy bare config holds %q, want only %q", got, IsolatedHomeDir)
	}
	assertLogin(bare)
	if _, err := os.Stat(filepath.Join(bare, IsolatedHomeDir, ".gemini", "config", "skills")); err == nil {
		t.Fatal("a bare agy config must carry no skills")
	}

	tsp := filepath.Join(t.TempDir(), "agy-bench")
	if err := WriteBenchAgyConfig(tsp, src); err != nil {
		t.Fatal(err)
	}
	assertLogin(tsp)
	if got := dirNames(t, filepath.Join(tsp, IsolatedHomeDir, ".gemini", "config", "skills")); got != BenchSkill {
		t.Fatalf("the agy bench config carries skills %q, want only %q", got, BenchSkill)
	}
	if _, err := os.Stat(AgySkillFile(tsp)); err != nil {
		t.Fatalf("AgySkillFile must name the skill the run loads: %v", err)
	}
}

// A batch run cannot answer a permission prompt, so it runs headless with every permission granted.
func TestAgyArgsRunHeadless(t *testing.T) {
	key := Key{ID: "c1"}
	args := agyArgs(key, Options{Model: "gemini-3.8-flash-high"})
	joined := strings.Join(args, " ")
	for _, want := range []string{"-p " + agentPrompt(key, Options{}), "--output-format stream-json",
		"--dangerously-skip-permissions", "--model gemini-3.8-flash-high"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("agy args %q lack %q", joined, want)
		}
	}
}

// agy reads no config variable, so its environment only moves HOME to the config's own and adds nothing else.
func TestAgyEnvIsolatesHomeOnly(t *testing.T) {
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, IsolatedHomeDir), 0o700); err != nil {
		t.Fatal(err)
	}
	base := []string{"HOME=/operator", "PATH=/usr/bin", "XDG_CONFIG_HOME=/operator/.config"}
	env := agyEnv(base, cfg, "/tsp/bin")
	if got := envValues(env, "HOME"); !reflect.DeepEqual(got, []string{filepath.Join(cfg, IsolatedHomeDir)}) {
		t.Fatalf("HOME = %v, want the config's own home", got)
	}
	if got := envValues(env, "PATH"); !reflect.DeepEqual(got, []string{"/tsp/bin" + string(os.PathListSeparator) + "/usr/bin"}) {
		t.Fatalf("PATH = %v, want the binary under test first", got)
	}
	for _, e := range env {
		if strings.HasPrefix(e, "=") || strings.HasPrefix(e, "XDG_CONFIG_HOME=") {
			t.Fatalf("agy env carries %q", e)
		}
	}
}

// agy runs the agent's commands as its own children. A timeout must end that whole tree: a test the agent started
// that loops on a planted defect would otherwise keep running for hours after the run was abandoned.
func TestAgyTimeoutEndsTheProcessesAgyStarted(t *testing.T) {
	bin := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := "#!/bin/sh\nsleep 300 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(filepath.Join(bin, "agy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	ar, _ := agyAgent(ctx, t.TempDir(), Key{ID: "c1"}, Options{})
	if !ar.TimedOut || time.Since(started) > 10*time.Second {
		t.Fatalf("the run must stop at its timeout: timedOut=%v after %s", ar.TimedOut, time.Since(started))
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d that agy started is still running after the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestParseAgyStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"event":"init","conversation_id":"conv-1","init":{"model":"m"}}`,
		`{"event":"step_update","step_update":{"step_index":0,"state":"DONE","step_type":"user_input"}}`,
		`{"event":"step_update","step_update":{"step_index":2,"state":"DONE","step_type":"agent_response"}}`,
		`{"event":"step_update","step_update":{"step_index":3,"state":"ACTIVE","step_type":"tool","tool_name":"write_to_file"}}`,
		`{"event":"step_update","step_update":{"step_index":3,"state":"DONE","step_type":"tool","tool_name":"write_to_file"}}`,
		`not json`,
		`{"event":"step_update","step_update":{"step_index":4,"state":"ACTIVE","step_type":"tool","tool_name":"run_command"}}`,
		`{"event":"step_update","step_update":{"step_index":4,"state":"DONE","step_type":"tool","tool_name":"run_command"}}`,
		`{"event":"result","result":{"conversation_id":"conv-1","status":"SUCCESS","response":"final answer\n","duration_seconds":9.5,"num_turns":1}}`,
	}, "\n")
	got := ParseAgyStream(strings.NewReader(stream))
	if got.SessionID != "conv-1" || got.Result != "final answer" || got.Turns != 2 || got.IsError || got.DurationMS != 9500 {
		t.Fatalf("parsed = %+v, want session conv-1, the response, two finished tool steps, 9500ms, no error", got)
	}

	// agy exits 0 when the login fails; the result status is the only place the failure shows.
	failed := ParseAgyStream(strings.NewReader(`{"event":"result","result":{"conversation_id":"","status":"ERROR","response":"","error":"authentication failed or timed out"}}`))
	if !failed.IsError || failed.ErrorText != "authentication failed or timed out" {
		t.Fatalf("an ERROR status must be an error with its message: %+v", failed)
	}
	for name, stream := range map[string]string{
		"empty":            "",
		"init only":        `{"event":"init","conversation_id":"c"}`,
		"steps, no result": `{"event":"step_update","step_update":{"state":"DONE","step_type":"tool"}}`,
	} {
		got := ParseAgyStream(strings.NewReader(stream))
		if !got.IsError || got.ErrorText == "" {
			t.Fatalf("%s: a stream without a result event must be an error, got %+v", name, got)
		}
	}
}
