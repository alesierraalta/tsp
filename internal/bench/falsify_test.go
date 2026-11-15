package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const falsifyAgentTests = `package rc

import "testing"

func TestOrderIsSorted(t *testing.T) {
	if Order() != "abc" {
		t.Fatal(Order())
	}
}

func TestWrongReason(t *testing.T) {
	t.Fatal("assertion the candidate fix does not touch")
}

func TestOrderHasThreeLetters(t *testing.T) {
	if len(Order()) != 3 {
		t.Fatal(Order())
	}
}

func TestOrderIsCab(t *testing.T) {
	if Order() != "cab" {
		t.Fatal(Order())
	}
}
`

func falsifyFix(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "order.go", "package rc\n\nfunc Order() string { return \"abc\" }\n\nfunc Len() int { return len(Order()) }\n")
}

func headFile(t *testing.T, dir, rel string) []byte {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "show", "HEAD:"+rel).Output()
	if err != nil {
		t.Fatalf("git show HEAD:%s: %v", rel, err)
	}
	return out
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func verdictsByName(rep FalsifyReport) map[string]string {
	got := map[string]string{}
	for _, v := range rep.Verdicts {
		got[v.Test] = v.Verdict
	}
	return got
}

// The green half is what the prose gate never checked: a test red on HEAD's code can still be red with the
// candidate fix (the wrong reason), and a test green with the fix may already pass on HEAD (it pins nothing).
// One run names all four answers per test.
func TestFalsifyNamesEachTestsVerdict(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	rep, err := Falsify(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("Falsify: %v", err)
	}
	want := map[string]string{
		"example.com/rc.TestOrderIsSorted":        FalsifyWitness,
		"example.com/rc.TestWrongReason":          FalsifyWrongReason,
		"example.com/rc.TestOrderHasThreeLetters": FalsifyPinsNothing,
		"example.com/rc.TestOrderIsCab":           FalsifyInverted,
	}
	if got := verdictsByName(rep); !reflect.DeepEqual(got, want) {
		t.Fatalf("verdicts = %v, want %v", got, want)
	}
}

// The command restores the source it read as the candidate fix, so the next run starts from HEAD's code. The
// patch it saves first is the only copy of the fix.
func TestFalsifyRestoresTheSourceAndSavesTheCandidateFix(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	agentTests := readFile(t, filepath.Join(dir, "agent_test.go"))
	rep, err := Falsify(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("Falsify: %v", err)
	}
	if got, want := readFile(t, filepath.Join(dir, "order.go")), headFile(t, dir, "order.go"); string(got) != string(want) {
		t.Fatalf("source after the run = %q, want HEAD's %q", got, want)
	}
	if got := readFile(t, filepath.Join(dir, "agent_test.go")); string(got) != string(agentTests) {
		t.Fatalf("the run changed the test file it never restores:\n%s", got)
	}
	if _, err := os.Stat(rep.Patch); err != nil {
		t.Fatalf("patch %q not saved: %v", rep.Patch, err)
	}
	if !strings.HasSuffix(rep.Patch, ".patch") || !strings.Contains(filepath.ToSlash(rep.Patch), "/.git/tsp/falsify/") {
		t.Fatalf("patch %q is not under <git-dir>/tsp/falsify", rep.Patch)
	}
	patch := string(readFile(t, rep.Patch))
	if !strings.Contains(patch, "order.go") || !strings.Contains(patch, `+func Order() string { return "abc" }`) {
		t.Fatalf("patch does not hold the candidate fix:\n%s", patch)
	}
	if want := []string{"order.go"}; !reflect.DeepEqual(rep.Restored, want) {
		t.Fatalf("restored = %v, want %v", rep.Restored, want)
	}
	if status := gitStatus(t, dir); strings.Contains(status, "order.go") {
		t.Fatalf("the source still differs from HEAD:\n%s", status)
	}
}

// A new source file is part of the candidate fix even when git has never tracked it: the patch must carry it
// and the restore must remove it, or the next run would read it as code HEAD already had.
func TestFalsifySavesAndRemovesANewUntrackedSourceFile(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	writeFile(t, dir, "extra.go", "package rc\n\n// Extra is new in the candidate fix.\nfunc Extra() int { return 1 }\n")
	rep, err := Falsify(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("Falsify: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "extra.go")); !os.IsNotExist(err) {
		t.Fatalf("the new source file was not removed: %v", err)
	}
	patch := string(readFile(t, rep.Patch))
	if !strings.Contains(patch, "extra.go") || !strings.Contains(patch, "Extra is new in the candidate fix") {
		t.Fatalf("patch does not hold the new source file:\n%s", patch)
	}
	if want := []string{"extra.go", "order.go"}; !reflect.DeepEqual(rep.Restored, want) {
		t.Fatalf("restored = %v, want %v", rep.Restored, want)
	}
}

// Without a source diff there is no candidate fix to judge, and the command refuses before it touches
// anything: telling the agent to write the fix first is the answer, not an empty report.
func TestFalsifyRefusesWithoutASourceDiff(t *testing.T) {
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	_, err := Falsify(dir, "", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "write the candidate fix in the source first") {
		t.Fatalf("error = %v, want the candidate-fix refusal", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "tsp")); !os.IsNotExist(err) {
		t.Fatalf("a refused run wrote under the git dir: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "agent_test.go")); string(got) != falsifyAgentTests {
		t.Fatalf("a refused run changed the test file:\n%s", got)
	}
}

// Saving the patch comes before anything else: when it fails nothing may be restored or deleted, because the
// only copy of the candidate fix is the working tree.
func TestFalsifyLeavesTheTreeWhenThePatchCannotBeSaved(t *testing.T) {
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	writeFile(t, dir, "extra.go", "package rc\n\nfunc Extra() int { return 1 }\n")
	before := map[string]string{}
	for _, rel := range []string{"order.go", "agent_test.go", "extra.go"} {
		before[rel] = string(readFile(t, filepath.Join(dir, rel)))
	}
	// A file where the patch directory must go makes the save fail.
	writeFile(t, dir, ".git/tsp", "not a directory\n")
	if _, err := Falsify(dir, "", time.Minute); err == nil {
		t.Fatal("Falsify saved the patch into a path that cannot hold it")
	}
	for rel, want := range before {
		if got := string(readFile(t, filepath.Join(dir, rel))); got != want {
			t.Fatalf("%s changed after a failed save:\n%s", rel, got)
		}
	}
}

// A linked worktree keeps its git dir outside the checkout: the patch has to land in that git dir, not in a
// .git the worktree does not have.
func TestFalsifySavesUnderTheLinkedWorktreesGitDir(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if out, err := exec.Command("git", "-C", dir, "worktree", "add", "-q", "-b", "linked", linked).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	writeFile(t, linked, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, linked)
	rep, err := Falsify(linked, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("Falsify in a linked worktree: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(rep.Patch), "/.git/worktrees/") {
		t.Fatalf("patch %q is not under the linked worktree's git dir", rep.Patch)
	}
	if got, want := readFile(t, filepath.Join(linked, "order.go")), headFile(t, linked, "order.go"); string(got) != string(want) {
		t.Fatalf("source after the run = %q, want HEAD's %q", got, want)
	}
}

func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return string(out)
}
