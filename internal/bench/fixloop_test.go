package bench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// fixedOrderSource is the fix the fake agent applies: Order is corrected, Value stays defective, so the
	// agent's fixed code differs from the case's fully fixed code in a way a test can distinguish.
	fixedOrderSource = "package model\n\nfunc Order() string { return \"abc\" }\n\nfunc Value() int { return 0 }\n"
	// fixWroteTest is what the fix step leaves behind: a test file it rewrote, which the loop must undo.
	fixWroteTest = "package model\n\nimport \"testing\"\n\nfunc TestFixWroteThis(t *testing.T) {}\n"
	// orderStillCab is red on the agent's fixed code and on the fully fixed code: a test that proves nothing.
	orderStillCab = "package model\n\nimport \"testing\"\n\nfunc TestOrderStaysReversed(t *testing.T) {\n\tif Order() != \"cba\" {\n\t\tt.Fatal(Order())\n\t}\n}\n"
	// valueIsOne is red on the agent's fixed code and green on the fully fixed code: it separates the two.
	valueIsOne = "package model\n\nimport \"testing\"\n\nfunc TestValueIsOne(t *testing.T) {\n\tif Value() != 1 {\n\t\tt.Fatal(Value())\n\t}\n}\n"
)

// fixLoopAgent is one agent process per step, switching on call order and on the prompt it was given: round 1
// leaves a test, the fix step rewrites the source and a test file, and the two arms each write their own test.
type fixLoopAgent struct {
	mu      sync.Mutex
	prompts []string
	base    string // the bench prompt the arms must reproduce
	round1  string // the test file round 1 leaves
}

func (a *fixLoopAgent) run(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	prompt := agentPrompt(key, opts)
	a.mu.Lock()
	a.prompts = append(a.prompts, prompt)
	call := len(a.prompts)
	a.mu.Unlock()
	res := AgentResult{CostUSD: 1, Turns: 3}
	switch call {
	case 1:
		return res, writeTestFile(filepath.Join(ws, "model", "agent_test.go"), a.round1)
	case 2:
		if err := writeTestFile(filepath.Join(ws, "model", "model.go"), fixedOrderSource); err != nil {
			return res, err
		}
		return res, writeTestFile(filepath.Join(ws, "model", "agent_test.go"), fixWroteTest)
	}
	if prompt == a.base {
		return res, writeTestFile(filepath.Join(ws, "model", "order_test.go"), orderStillCab)
	}
	return res, writeTestFile(filepath.Join(ws, "model", "value_test.go"), valueIsOne)
}

func writeTestFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

// tspBinary builds the CLI once per test, so `findings record` and `findings check` are exercised for real
// rather than through a stub that could accept a prompt the binary would refuse.
func tspBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the tsp binary")
	}
	for _, bin := range []string{"go", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	bin := filepath.Join(t.TempDir(), "tsp")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/tsp")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tsp: %v\n%s", err, out)
	}
	return bin
}

func runFixLoopReading(t *testing.T, round1 string, keep bool) (Aggregate, int, *fixLoopAgent, string, string, Key) {
	t.Helper()
	dir := sessionCase(t)
	key, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	agent := &fixLoopAgent{base: agentPrompt(key, Options{}), round1: round1}
	out, benchDir := filepath.Join(t.TempDir(), "results"), t.TempDir()
	agg, code := Run(Options{CasesGlob: dir, Runs: 1, Model: "m", FixLoop: true, TspPath: tspBinary(t),
		Timeout: 2 * time.Minute, SuiteTimeout: 2 * time.Minute, Out: out, BenchDir: benchDir, Agent: agent.run, Keep: keep})
	return agg, code, agent, out, benchDir, key
}

// The loop the user runs in practice: round 1 pins a defect, the agent fixes it, and the same fixed workspace is
// measured twice, once plain and once with what the registry says. Round 1 stays the run's own result.
func TestFixLoopMeasuresBothRoundTwoArms(t *testing.T) {
	agg, code, agent, out, benchDir, key := runFixLoopReading(t, witnessTest, true)
	if code != 0 || len(agg.Cases) != 1 {
		t.Fatalf("code %d cases %+v", code, agg.Cases)
	}
	res := agg.Cases[0]
	loop := res.FixLoop
	if loop == nil {
		t.Fatal("the run recorded no fix loop")
	}
	base := agentPrompt(key, Options{})
	if len(agent.prompts) != 4 {
		t.Fatalf("agent calls = %d, want round 1, the fix and two arms: %q", len(agent.prompts), agent.prompts)
	}
	witness := "example.com/sub/model.TestOrderIsSorted"
	if len(loop.Witnesses) != 1 || loop.Witnesses[0] != witness {
		t.Fatalf("witnesses = %v, want %s", loop.Witnesses, witness)
	}
	if fixPrompt := agent.prompts[1]; fixPrompt == base || !strings.Contains(fixPrompt, witness) {
		t.Errorf("fix prompt = %q, want the round-1 witness and not the bench prompt", fixPrompt)
	}
	if agent.prompts[2] != base {
		t.Errorf("plain arm prompt = %q, want the bench prompt byte for byte (%q)", agent.prompts[2], base)
	}
	if report := agent.prompts[3]; !strings.Contains(report, witness) || !strings.Contains(report, "fixed") {
		t.Errorf("registry arm prompt = %q, want the findings check report naming %s as fixed", report, witness)
	}
	if !loop.Valid || loop.Reason != "" {
		t.Fatalf("loop valid = %v reason = %q", loop.Valid, loop.Reason)
	}
	if !loop.FixTouchedTests {
		t.Error("the fix rewrote a test file but fix_touched_tests is false")
	}
	if loop.Check == "" || loop.Record == "" {
		t.Errorf("record = %q check = %q, want both", loop.Record, loop.Check)
	}
	plain, registry := armOf(t, loop, FixArmPlain), armOf(t, loop, FixArmRegistry)
	if want := []string{"example.com/sub/model.TestOrderStaysReversed"}; !reflect.DeepEqual(plain.NewFalse, want) || len(plain.NewReal) != 0 {
		t.Errorf("plain arm new_false = %v new_real = %v, want %v and none", plain.NewFalse, plain.NewReal, want)
	}
	if want := []string{"example.com/sub/model.TestValueIsOne"}; !reflect.DeepEqual(registry.NewReal, want) || len(registry.NewFalse) != 0 {
		t.Errorf("registry arm new_real = %v new_false = %v, want %v and none", registry.NewReal, registry.NewFalse, want)
	}
	if len(plain.StillRed) != 0 || len(registry.StillRed) != 0 {
		t.Errorf("still red: plain %v registry %v, want none: the fix turned the witness green", plain.StillRed, registry.StillRed)
	}
	// The run's own numbers stay round 1's, so a fix-loop reading is still comparable on what round 1 found.
	if res.Caught != 1 || res.CostUSD != 1 || res.Turns != 3 {
		t.Errorf("main result caught = %d cost = %v turns = %d, want round 1's numbers only", res.Caught, res.CostUSD, res.Turns)
	}
	if loop.Fix.CostUSD != 1 || loop.Fix.Turns != 3 || plain.CostUSD != 1 || registry.CostUSD != 1 || plain.Turns != 3 || registry.Turns != 3 {
		t.Errorf("step accounts: fix %+v plain %+v registry %+v", loop.Fix, plain, registry)
	}
	// Both arms were forked from the fixed workspace, whose HEAD holds the fix and whose tests stayed round 1's.
	runDir := filepath.Join(out, res.Case, "1")
	for _, arm := range []string{FixArmPlain, FixArmRegistry} {
		ws := filepath.Join(runDir, "arm-"+arm)
		got, err := os.ReadFile(filepath.Join(ws, "model", "agent_test.go"))
		if err != nil || string(got) != witnessTest {
			t.Errorf("%s arm test file = %q err %v, want round 1's bytes restored", arm, got, err)
		}
		tree, _ := exec.Command("git", "-C", ws, "ls-tree", "-r", "--name-only", "HEAD").CombinedOutput()
		if strings.Contains(string(tree), "agent_test.go") || !strings.Contains(string(tree), "model/model.go") {
			t.Errorf("%s arm HEAD tree = %s, want the fix's source and round 1's tests uncommitted", arm, tree)
		}
		src, _ := exec.Command("git", "-C", ws, "show", "HEAD:model/model.go").CombinedOutput()
		if !strings.Contains(string(src), `return "abc"`) {
			t.Errorf("%s arm HEAD source = %s, want the agent's fix committed", arm, src)
		}
	}
	if agg.FixLoop == nil || agg.FixLoop.ValidRuns != 1 || agg.FixLoop.SkippedRuns != 0 || agg.FixLoop.FailedRuns != 0 {
		t.Fatalf("aggregate fix loop = %+v", agg.FixLoop)
	}
	if plainTotals, registryTotals := armOf2(t, agg.FixLoop, FixArmPlain), armOf2(t, agg.FixLoop, FixArmRegistry); plainTotals.NewFalse != 1 || registryTotals.NewReal != 1 {
		t.Errorf("aggregate arms = %+v", agg.FixLoop.Arms)
	}
	if s := Summary(agg); !strings.Contains(s, "fix loop") || !strings.Contains(s, "new real") || !strings.Contains(s, "still red") {
		t.Errorf("summary has no fix-loop line:\n%s", s)
	}
	if raw, err := os.ReadFile(filepath.Join(benchDir, "history.jsonl")); err != nil || !strings.Contains(string(raw), `"fix_loop":true`) {
		t.Errorf("history row = %s err %v, want fix_loop recorded", raw, err)
	}
	if !agg.Provenance.FixLoop {
		t.Error("the reading's provenance does not say it measured a fix loop")
	}
}

// A run whose round 1 pinned nothing has nothing to fix: the loop is skipped, said so, and costs nothing more.
func TestFixLoopSkipsARunThatPinnedNothing(t *testing.T) {
	agg, code, agent, _, _, _ := runFixLoopReading(t, blindTest, false)
	if code != 0 || len(agg.Cases) != 1 {
		t.Fatalf("code %d cases %d", code, len(agg.Cases))
	}
	if len(agent.prompts) != 1 {
		t.Fatalf("agent calls = %d, want only round 1: the loop had nothing to fix", len(agent.prompts))
	}
	loop := agg.Cases[0].FixLoop
	if loop == nil || !loop.Skipped || loop.Valid || loop.Reason == "" || len(loop.Arms) != 0 {
		t.Fatalf("loop = %+v, want a skipped loop with a reason", loop)
	}
	if agg.FixLoop == nil || agg.FixLoop.SkippedRuns != 1 || agg.FixLoop.ValidRuns != 0 {
		t.Fatalf("aggregate fix loop = %+v", agg.FixLoop)
	}
	if s := Summary(agg); !strings.Contains(s, "fix loop") || !strings.Contains(s, "skipped") {
		t.Errorf("summary does not report the skipped loop:\n%s", s)
	}
}

// A reading without the flag is the reading it always was: no fix-loop work, no fix-loop key, no new summary text.
func TestRunWithoutFixLoopKeepsItsShape(t *testing.T) {
	dir := sessionCase(t)
	key, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	agent := &fixLoopAgent{base: agentPrompt(key, Options{}), round1: witnessTest}
	out := filepath.Join(t.TempDir(), "results")
	agg, code := Run(Options{CasesGlob: dir, Runs: 1, Timeout: 2 * time.Minute, SuiteTimeout: 2 * time.Minute,
		Out: out, BenchDir: t.TempDir(), Agent: agent.run})
	if code != 0 || len(agg.Cases) != 1 {
		t.Fatalf("code %d cases %d", code, len(agg.Cases))
	}
	if len(agent.prompts) != 1 {
		t.Fatalf("agent calls = %d, want one pass", len(agent.prompts))
	}
	if agg.Cases[0].FixLoop != nil || agg.FixLoop != nil || agg.Provenance.FixLoop {
		t.Fatalf("a run without --fix-loop recorded a loop: %+v %+v", agg.Cases[0].FixLoop, agg.FixLoop)
	}
	raw, err := os.ReadFile(filepath.Join(out, agg.Cases[0].Case, "1", "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fix_loop") {
		t.Errorf("result.json carries a fix_loop key without the flag:\n%s", raw)
	}
	if s := Summary(agg); strings.Contains(s, "fix loop") {
		t.Errorf("summary gained fix-loop text without the flag:\n%s", s)
	}
}

// RedCheck names a test with the runner's own key and testsOn names it with the same parser, so the round-2
// classification can look one up in the other. This pins the formats equal instead of assuming it.
func TestFixLoopTestNamesMatchBetweenRedCheckAndTestsOn(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	for _, bin := range []string{"go", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	dir := sessionCase(t)
	key, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(t.TempDir(), "ws")
	if _, err := Scaffold(dir, ws, key, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(ws, "model", "agent_test.go"), witnessTest); err != nil {
		t.Fatal(err)
	}
	rep, err := RedCheck(ws, "", time.Minute)
	if err != nil || len(rep.Red) != 1 {
		t.Fatalf("red check: %v red = %v", err, rep.Red)
	}
	onAll, _, _, err := testsOn(filepath.Join(dir, FixtureDir), filepath.Join(dir, FixDir, "all"), ws, rep.TestFiles, key.Suite, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range rep.Red {
		passed, ok := onAll[name]
		if !ok || !passed {
			t.Fatalf("RedCheck named %q and testsOn reported %v; the two disagree on a test's name", name, onAll)
		}
	}
}

// Options.Task replaces the whole prompt: the fix step must never carry the bench prompt or the case request, and
// every prompt that does not set it stays exactly what it was.
func TestTaskReplacesTheWholePrompt(t *testing.T) {
	key := Key{Defects: []Defect{{ID: "D1"}}}
	plain := agentPrompt(key, Options{})
	if got := agentPrompt(key, Options{Task: "fix the witness"}); got != "fix the witness" {
		t.Fatalf("prompt with a task = %q, want only the task", got)
	}
	if got := agentPrompt(key, Options{Prompt: "custom", Task: "fix the witness"}); got != "fix the witness" {
		t.Fatalf("prompt with a task and a custom prompt = %q, want only the task", got)
	}
	if again := agentPrompt(key, Options{}); again != plain {
		t.Fatalf("the bench prompt changed: %q -> %q", plain, again)
	}
}

func armOf(t *testing.T, loop *FixLoop, arm string) FixArm {
	t.Helper()
	for _, got := range loop.Arms {
		if got.Arm == arm {
			return got
		}
	}
	t.Fatalf("no %s arm in %+v", arm, loop.Arms)
	return FixArm{}
}

func armOf2(t *testing.T, totals *FixLoopTotals, arm string) FixArmTotals {
	t.Helper()
	if totals == nil {
		t.Fatal("no fix-loop totals")
	}
	for _, got := range totals.Arms {
		if got.Arm == arm {
			return got
		}
	}
	t.Fatalf("no %s arm in %+v", arm, totals.Arms)
	return FixArmTotals{}
}
