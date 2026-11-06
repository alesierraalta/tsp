package bench

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Every reviewer process Gentle AI starts must run on the bench's model, and answer in JSON so its cost and the
// model that answered can be read; the arguments Gentle AI chose otherwise pass through untouched.
func TestShimArgsForceTheBenchModelAndJSONOutput(t *testing.T) {
	in := []string{"--print", "--output-format", "text", "--tools", "", "--model", "opus", "--effort", "medium"}
	got, requested := shimArgs(in, "haiku")
	want := []string{"--print", "--output-format", "json", "--tools", "", "--model", "haiku", "--effort", "medium"}
	if !reflect.DeepEqual(got, want) || requested != "opus" {
		t.Fatalf("shimArgs = %q, requested %q; want %q, opus", got, requested, want)
	}
	got, requested = shimArgs([]string{"--print", "--model=sonnet", "--output-format=text"}, "haiku")
	if !reflect.DeepEqual(got, []string{"--print", "--model=haiku", "--output-format=json"}) || requested != "sonnet" {
		t.Fatalf("shimArgs with = forms = %q, %q", got, requested)
	}
	got, requested = shimArgs([]string{"--print"}, "haiku")
	if !reflect.DeepEqual(got, []string{"--print", "--model", "haiku"}) || requested != "" {
		t.Fatalf("shimArgs without a model = %q, %q; want the bench model added", got, requested)
	}
}

func TestParseClaudeJSONReadsTheAnswerCostAndModel(t *testing.T) {
	out := `{"type":"result","result":"{\"lens\":\"review-risk\"}","total_cost_usd":0.0123,"modelUsage":{"claude-haiku-5-5":{"inputTokens":10}}}`
	result, cost, models, err := parseClaudeJSON([]byte(out))
	if err != nil || result != `{"lens":"review-risk"}` || cost != 0.0123 || !reflect.DeepEqual(models, []string{"claude-haiku-5-5"}) {
		t.Fatalf("parseClaudeJSON = %q, %v, %v, %v", result, cost, models, err)
	}
	if _, _, _, err := parseClaudeJSON([]byte("plain text")); err == nil {
		t.Fatal("a non-JSON answer must be reported, not read as an empty result")
	}
}

// The driver follows only what Gentle AI offers: START gets the consent and lenses the bench decides, a collect of
// reviewer results becomes one capture per slot, and anything after the reviewer results ends the arm's run.
func TestNextReviewStepFollowsTheOfferedTransition(t *testing.T) {
	start := `{"result":{"next_transition":{"kind":"execute","reason_code":"fresh_target_ready","execute":{"operation":"review.start","arguments":[{"token":"--cwd=/w"},{"token":"--target=sha256:t"},{"token":"--consent=relay"}]}}}}`
	step, err := nextReviewStep([]byte(start))
	if err != nil || step.Kind != "start" {
		t.Fatalf("start step = %+v, %v", step, err)
	}
	joined := strings.Join(step.Args, " ")
	if strings.Contains(joined, "--consent=relay") || !strings.Contains(joined, "--consent=granted") || !strings.Contains(joined, "--lenses="+gentleReviewLenses) || !strings.HasPrefix(joined, "review start --cwd=/w --target=sha256:t") {
		t.Fatalf("start args = %q", joined)
	}
	collect := `{"next_transition":{"kind":"collect","reason_code":"reviewer_results_required","collect":{"inputs":[{"capture_operation":"review.capture-result","arguments":[{"token":"--lineage=l"},{"token":"--lens=review-risk"}]},{"capture_operation":"review.capture-result","arguments":[{"token":"--lineage=l"},{"token":"--lens=review-reliability"}]}]}}}`
	step, err = nextReviewStep([]byte(collect))
	if err != nil || step.Kind != "capture" || len(step.Captures) != 2 {
		t.Fatalf("collect step = %+v, %v", step, err)
	}
	if got := strings.Join(step.Captures[1], " "); got != "review capture-result --lineage=l --lens=review-reliability --agent=claude-code" {
		t.Fatalf("capture args = %q", got)
	}
	plan := `{"next_transition":{"kind":"collect","reason_code":"correction_plan_required","collect":{"inputs":[{"capture_operation":"review.capture-correction-plan","arguments":[]}]}}}`
	if step, _ := nextReviewStep([]byte(plan)); step.Kind != "done" {
		t.Fatalf("a correction plan must end the arm's run, got %+v", step)
	}
	if step, _ := nextReviewStep([]byte(`{"next_transition":{"kind":"stop","reason_code":"managed_assets_outdated"}}`)); step.Kind != "stop" || step.Reason != "managed_assets_outdated" {
		t.Fatalf("stop step = %+v", step)
	}
	if _, err := nextReviewStep([]byte("not json")); err == nil {
		t.Fatal("an unreadable status must be an error")
	}
}

func asChangeCase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for path, body := range map[string]string{
		"fixture/src/avg.js":  "export const avg = (xs) => xs.slice(1).reduce((a, b) => a + b, 0) / xs.length;\n",
		"fixture/README.md":   "avg\n",
		"fixture/src/old.js":  "export const legacy = 1;\n",
		"fix/all/src/avg.js":  "export const avg = (xs) => xs.reduce((a, b) => a + b, 0) / xs.length;\n",
		"fix/all/src/note.js": "export const note = 1;\n",
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The workspace ends on the fixture exactly as Scaffold leaves it, and its history holds the fixed code as the base,
// so a review of HEAD~1..HEAD sees the defects as the change and a file only the fix had as removed.
func TestScaffoldAsChangeCommitsTheFixAsBaseAndTheFixtureAsCandidate(t *testing.T) {
	caseDir := asChangeCase(t)
	ws := filepath.Join(t.TempDir(), "ws")
	suite, err := ScaffoldAsChange(caseDir, ws, Key{Suite: "true"}, time.Minute)
	if err != nil || !suite.Green {
		t.Fatalf("ScaffoldAsChange = %+v, %v", suite, err)
	}
	head, _ := os.ReadFile(filepath.Join(ws, "src/avg.js"))
	if !strings.Contains(string(head), "slice(1)") {
		t.Fatalf("the workspace must hold the fixture with its defect, got %q", head)
	}
	if _, err := os.Stat(filepath.Join(ws, "src/note.js")); err == nil {
		t.Fatal("a file only fix/all has must not be in the candidate")
	}
	diff, err := gitRun(ws, "diff", "--name-status", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "M\tsrc/avg.js") || !strings.Contains(diff, "D\tsrc/note.js") || strings.Contains(diff, "README") || strings.Contains(diff, "old.js") {
		t.Fatalf("base..candidate diff = %q; want the defect edit and the removed fix-only file, nothing else", diff)
	}
	if count, _ := gitRun(ws, "rev-list", "--count", "HEAD"); strings.TrimSpace(count) != "2" {
		t.Fatalf("history has %s commits, want base and candidate", count)
	}
}

func TestScaffoldAsChangeRefusesACaseWithoutAChange(t *testing.T) {
	caseDir := asChangeCase(t)
	if err := os.RemoveAll(filepath.Join(caseDir, "fix")); err != nil {
		t.Fatal(err)
	}
	if _, err := ScaffoldAsChange(caseDir, filepath.Join(t.TempDir(), "ws"), Key{Suite: "true"}, time.Minute); err == nil || !strings.Contains(err.Error(), "fix") {
		t.Fatalf("a case without fix/all must be refused, got %v", err)
	}
	same := asChangeCase(t)
	fixed, _ := os.ReadFile(filepath.Join(same, "fixture/src/avg.js"))
	os.WriteFile(filepath.Join(same, "fix/all/src/avg.js"), fixed, 0o644)
	os.Remove(filepath.Join(same, "fix/all/src/note.js"))
	if _, err := ScaffoldAsChange(same, filepath.Join(t.TempDir(), "ws"), Key{Suite: "true"}, time.Minute); err == nil || !strings.Contains(err.Error(), "no change") {
		t.Fatalf("a fix that changes nothing must be refused, got %v", err)
	}
}

func TestGentleReviewIsAKnownRunnerOnHaiku(t *testing.T) {
	if !KnownRunner(RunnerGentleReview) || DefaultModel(RunnerGentleReview) != "haiku" {
		t.Fatalf("gentle-review known=%v default=%q", KnownRunner(RunnerGentleReview), DefaultModel(RunnerGentleReview))
	}
}

// A clean control has no fix to diff against, so it is presented over an empty base: the whole fixture is the
// change, and every finding on it is a false positive. EmptyBase presents any case that way, so the defect is
// hidden in the whole code rather than shown as the diff.
func TestScaffoldAsChangeUsesAnEmptyBaseForAControlAndOnRequest(t *testing.T) {
	control := asChangeCase(t)
	if err := os.RemoveAll(filepath.Join(control, "fix")); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		caseDir string
		key     Key
	}{
		"control":    {control, Key{Suite: "true", Control: ControlClean}},
		"empty base": {asChangeCase(t), Key{Suite: "true"}},
	} {
		ws := filepath.Join(t.TempDir(), "ws")
		suite, err := ScaffoldAsChangeFrom(tc.caseDir, ws, tc.key, time.Minute, name == "empty base")
		if err != nil || !suite.Green {
			t.Fatalf("%s: ScaffoldAsChangeFrom = %+v, %v", name, suite, err)
		}
		base, err := gitRun(ws, "ls-tree", "-r", "--name-only", "HEAD~1")
		if err != nil || strings.TrimSpace(base) != "" {
			t.Fatalf("%s: base tree = %q, %v; want empty", name, base, err)
		}
		diff, _ := gitRun(ws, "diff", "--name-status", "HEAD~1", "HEAD")
		if !strings.Contains(diff, "A\tsrc/avg.js") || !strings.Contains(diff, "A\tREADME.md") {
			t.Fatalf("%s: diff = %q; want the whole fixture added", name, diff)
		}
	}
}
