package gate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/plan"
)

// planWithOneFinding records one defect of another file: the edited test below has no row, which is
// the fact the nudge exists to catch.
const planWithOneFinding = `# Test plan

## Findings

| ID | Finding | Evidence | Pinning test | Status |
|---|---|---|---|---|
| F1 | ` + "`src/app.go:10`" + ` drops the guard | ledger row 1 | ` + "`src/app_test.go :: TestGuard`" + ` | pinned |
`

// EditedFiles reads this session's own tool calls: a test the session wrote is evidence it authored
// the change, which a worktree entry never is.
func TestEditedFilesReadsThisSessionsToolCalls(t *testing.T) {
	transcript := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"reading first"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/README.md"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/src/other_test.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/repo/src/app.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/src/other_test.go"}}]}}`,
	}, "\n") + "\n"

	got := EditedFiles(strings.NewReader(transcript))
	want := []string{"/repo/src/other_test.go", "/repo/src/app.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edited files = %#v, want %#v: only this session's edits, once each, in order", got, want)
	}
}

// A bound session that ran the discipline, edited a test, and recorded no finding for it is asked
// once to record it.
func TestDecideBlocksOnceWhenAnEditedTestHasNoFindingsRow(t *testing.T) {
	repo := &fakeRepo{
		root:   "/repo",
		plan:   planWithOneFinding,
		status: porcelain(),
		transcript: stamped(auditStart,
			`{"name":"Skill","input":{"skill":"tsp"}}`,
			`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/src/other_test.go"}}]}}`),
	}
	dir := t.TempDir()
	deps := repo.deps(auditNow)
	deps.BindingDir = dir
	storedBinding(t, dir, "sess-1", "/repo", plan.DefaultPath, "run-t1")

	res := Decide(Input{SessionID: "sess-1", TranscriptPath: "t"}, deps)
	if !res.Block {
		t.Fatalf("block = false (%#v), want the edited test's missing row to be asked for", res)
	}
	if !strings.Contains(res.BlockReason, "src/other_test.go") {
		t.Fatalf("block reason = %q, want the unrecorded test named", res.BlockReason)
	}
	if res.Entry == nil || !res.Entry.Nudged {
		t.Fatalf("entry = %#v, want the nudge logged", res.Entry)
	}

	second := Decide(Input{SessionID: "sess-1", TranscriptPath: "t", StopHookActive: true}, deps)
	if second.Block {
		t.Fatalf("second settle blocked again: %#v, want one nudge per session", second)
	}
	if second.Entry == nil || !second.Entry.NudgeAccepted {
		t.Fatalf("entry = %#v, want the accepted settle logged", second.Entry)
	}
}

// A row that names the edited test is the recording the nudge asks for, and a source-only change is
// not the nudge's business.
func TestDecideDoesNotBlockARecordedTestOrASourceOnlyChange(t *testing.T) {
	cases := []struct {
		name   string
		edited string
	}{
		{name: "the plan records the edited test", edited: "/repo/src/app_test.go"},
		{name: "the session edited no test", edited: "/repo/src/app.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{
				root:   "/repo",
				plan:   planWithOneFinding,
				status: porcelain(),
				transcript: stamped(auditStart,
					`{"name":"Skill","input":{"skill":"tsp"}}`,
					`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"`+tc.edited+`"}}]}}`),
			}
			dir := t.TempDir()
			deps := repo.deps(auditNow)
			deps.BindingDir = dir
			storedBinding(t, dir, "sess-1", "/repo", plan.DefaultPath, "run-t1")

			res := Decide(Input{SessionID: "sess-1", TranscriptPath: "t"}, deps)
			if res.Block {
				t.Fatalf("block = true (%q), want silence", res.BlockReason)
			}
			if res.Entry != nil && res.Entry.Nudged {
				t.Fatalf("entry = %#v, want no nudge logged", res.Entry)
			}
		})
	}
}

// Without a binding the gate has no plan to ask about, and a session that never ran the discipline
// has nothing to record: neither is nudged.
func TestDecideDoesNotBlockAnUnboundSessionOrOneWithoutTheDiscipline(t *testing.T) {
	cases := []struct {
		name       string
		bound      bool
		transcript string
	}{
		{
			name:  "no binding",
			bound: false,
			transcript: stamped(auditStart,
				`{"name":"Skill","input":{"skill":"tsp"}}`,
				`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/src/other_test.go"}}]}}`),
		},
		{
			name:  "the discipline never ran",
			bound: true,
			transcript: stamped(auditStart,
				`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/src/other_test.go"}}]}}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{root: "/repo", plan: planWithOneFinding, status: porcelain(), transcript: tc.transcript}
			dir := t.TempDir()
			deps := repo.deps(auditNow)
			deps.BindingDir = dir
			if tc.bound {
				storedBinding(t, dir, "sess-1", "/repo", plan.DefaultPath, "run-t1")
			}
			res := Decide(Input{SessionID: "sess-1", TranscriptPath: "t"}, deps)
			if res.Block {
				t.Fatalf("block = true (%q), want silence", res.BlockReason)
			}
		})
	}
}

// The hook refuses the stop with the decision Claude Code reads, and the exit code stays 0 so a host
// that cannot read the JSON still ends the turn instead of breaking it.
func TestRunBlocksTheStopOnceAndAcceptsTheNextSettle(t *testing.T) {
	t.Setenv(BindingEnv, "")
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	rev := exec.Command("git", "rev-parse", "--show-toplevel")
	rev.Dir = repo
	out, err := rev.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	root := strings.TrimSpace(string(out))
	rel := plan.DefaultPath
	if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, rel), []byte(planWithOneFinding), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	edited := filepath.Join(root, "src", "other_test.go")
	transcript := filepath.Join(base, "transcript.jsonl")
	lines := `{"name":"Skill","input":{"skill":"tsp"}}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"` + edited + `"}}]}}` + "\n"
	if err := os.WriteFile(transcript, []byte(lines), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	logPath := filepath.Join(base, "telemetry", "testing-gate.jsonl")
	storedBinding(t, BindingsDir(logPath), "sess-run", root, rel, "run-t1")

	payload := `{"session_id":"sess-run","transcript_path":"` + transcript + `","cwd":"` + repo + `"}`
	var stdout bytes.Buffer
	if code := Run(strings.NewReader(payload), &stdout, logPath, auditNow); code != 0 {
		t.Fatalf("Run exit = %d, want 0: the block rides the JSON, never the exit code", code)
	}
	if !strings.Contains(stdout.String(), `"decision":"block"`) || !strings.Contains(stdout.String(), "src/other_test.go") {
		t.Fatalf("stdout = %q, want a blocking decision naming the unrecorded test", stdout.String())
	}
	row, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(row), `"nudged":true`) {
		t.Fatalf("log row = %s, want the nudge recorded", row)
	}

	var second bytes.Buffer
	payload = `{"session_id":"sess-run","transcript_path":"` + transcript + `","cwd":"` + repo + `","stop_hook_active":true}`
	if code := Run(strings.NewReader(payload), &second, logPath, auditNow); code != 0 {
		t.Fatalf("second Run exit = %d, want 0", code)
	}
	if second.Len() != 0 {
		t.Fatalf("second settle stdout = %q, want no decision: the nudge is asked once", second.String())
	}
	row, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(row), `"nudge_accepted":true`) {
		t.Fatalf("log rows = %s, want the accepted settle recorded", row)
	}
}
