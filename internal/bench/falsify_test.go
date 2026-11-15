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

// gitDo runs git in dir and fails the test when it exits non-zero.
func gitDo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A staged version of a source file that differs from both HEAD and the worktree has no home in the patch: it
// is the worktree that `git diff HEAD` holds, and `git restore --staged --worktree` would drop the staged one
// without a copy anywhere. The command refuses before it saves or restores anything, and names the files.
func TestFalsifyRefusesWhenTheIndexAndTheWorktreeDisagree(t *testing.T) {
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	writeFile(t, dir, "order.go", "package rc\n\nfunc Order() string { return \"staged\" }\n")
	gitDo(t, dir, "add", "order.go")
	falsifyFix(t, dir)
	worktree := readFile(t, filepath.Join(dir, "order.go"))
	_, err := Falsify(dir, "", time.Minute)
	if err == nil {
		t.Fatal("Falsify accepted a source file whose staged version differs from the worktree, and lost it")
	}
	for _, want := range []string{"order.go", "stage or unstage consistently, then rerun"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if got := readFile(t, filepath.Join(dir, "order.go")); string(got) != string(worktree) {
		t.Fatalf("a refused run rewrote the source:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "tsp")); !os.IsNotExist(err) {
		t.Fatalf("a refused run saved a patch under the git dir: %v", err)
	}
}

// Staging the fix and leaving the worktree at the same version is one version, not three: the patch holds it
// and the restore puts it back, so the refusal above must not fire.
func TestFalsifyAcceptsAFixThatIsOnlyStaged(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	gitDo(t, dir, "add", "order.go")
	rep, err := Falsify(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("Falsify refused a fix that is only staged: %v", err)
	}
	if patch := string(readFile(t, rep.Patch)); !strings.Contains(patch, `+func Order() string { return "abc" }`) {
		t.Fatalf("the patch does not hold the staged fix:\n%s", patch)
	}
	if got, want := readFile(t, filepath.Join(dir, "order.go")), headFile(t, dir, "order.go"); string(got) != string(want) {
		t.Fatalf("the source after the run = %q, want HEAD's %q", got, want)
	}
	if status := gitStatus(t, dir); strings.Contains(status, "order.go") {
		t.Fatalf("the staged source is not back at HEAD:\n%s", status)
	}
}

// A source file whose name holds a glob character has to be restored as that exact path. Given to git as a
// pathspec, `a*.go` also matches the agent's test file, and the restore silently reverts the tests the command
// promises never to touch while its proof still passes.
func TestFalsifyRestoresTheNamedPathNeverItsGlobMatches(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	// HEAD tracks a source file named after a glob, and the test file the agent will edit: both match `a*.go`.
	writeFile(t, dir, "a*.go", "package rc\n\nfunc Helper() string { return \"one\" }\n")
	writeFile(t, dir, "agent_test.go", "package rc\n\nimport \"testing\"\n\nfunc TestHelper(t *testing.T) {\n\tif Helper() == \"\" {\n\t\tt.Fatal()\n\t}\n}\n")
	gitDo(t, dir, "add", "-A")
	gitDo(t, dir, "commit", "-qm", "a source file with a glob in its name")
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	writeFile(t, dir, "a*.go", "package rc\n\nfunc Helper() string { return \"two\" }\n")
	agentTests := readFile(t, filepath.Join(dir, "agent_test.go"))
	rep, err := Falsify(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("Falsify: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "agent_test.go")); string(got) != string(agentTests) {
		t.Fatalf("the restore expanded the glob and reverted the test file it never restores:\n%s", got)
	}
	if want := []string{"a*.go", "order.go"}; !reflect.DeepEqual(rep.Restored, want) {
		t.Fatalf("restored = %v, want %v", rep.Restored, want)
	}
	if got, want := readFile(t, filepath.Join(dir, "a*.go")), headFile(t, dir, "a*.go"); string(got) != string(want) {
		t.Fatalf("the glob-named source after the run = %q, want HEAD's %q", got, want)
	}
}

// A submodule change in the source set cannot be reproduced from a patch, and restoring it half-succeeds: the
// index goes back to HEAD while the submodule's checkout does not. The command refuses before it touches the
// tree rather than promising a restore it cannot prove.
func TestFalsifyRefusesASubmoduleChange(t *testing.T) {
	dir := redCheckRepo(t)
	sub := filepath.Join(dir, "vendor", "lib")
	writeFile(t, sub, "f.txt", "one\n")
	gitDo(t, sub, "init", "-q")
	gitDo(t, sub, "add", "-A")
	gitDo(t, sub, "commit", "-qm", "one")
	gitDo(t, dir, "add", "vendor/lib")
	gitDo(t, dir, "commit", "-qm", "a submodule at HEAD")
	writeFile(t, sub, "f.txt", "two\n")
	gitDo(t, sub, "add", "-A")
	gitDo(t, sub, "commit", "-qm", "two")
	gitDo(t, dir, "add", "vendor/lib")
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	source := readFile(t, filepath.Join(dir, "order.go"))
	_, err := Falsify(dir, "", time.Minute)
	if err == nil {
		t.Fatal("Falsify claimed to save and restore a submodule change it cannot reproduce")
	}
	for _, want := range []string{"submodule changes are not supported", "vendor/lib"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if got := readFile(t, filepath.Join(dir, "order.go")); string(got) != string(source) {
		t.Fatalf("a refused run rewrote the source:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "tsp")); !os.IsNotExist(err) {
		t.Fatalf("a refused run saved a patch under the git dir: %v", err)
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

// The patch is the only copy of the candidate fix from the moment the restore starts, so it has to be on disk
// and complete before anything is undone: the save syncs the file and its directory, then the restore runs.
func TestFalsifySavesTheCompletePatchBeforeItRestores(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", falsifyAgentTests)
	falsifyFix(t, dir)
	head := string(headFile(t, dir, "order.go"))
	var atRestore string
	falsifyBeforeRestore = func(patch string) {
		atRestore = patch
		data := readFile(t, patch)
		if !strings.Contains(string(data), `+func Order() string { return "abc" }`) {
			t.Errorf("the patch on disk is not the candidate fix:\n%s", data)
		}
		tmp, err := filepath.Glob(filepath.Join(filepath.Dir(patch), "*.tmp"))
		if err != nil || len(tmp) != 0 {
			t.Errorf("leftover temp files beside the patch: %v, %v", tmp, err)
		}
		if got := string(readFile(t, filepath.Join(dir, "order.go"))); got == head {
			t.Errorf("the source was already restored before the patch was saved and complete")
		}
	}
	defer func() { falsifyBeforeRestore = nil }()
	if _, err := Falsify(dir, "", 2*time.Minute); err != nil {
		t.Fatalf("Falsify: %v", err)
	}
	if atRestore == "" {
		t.Fatal("the restore ran without the saved patch being observed first")
	}
}

// The patch name is a nanosecond timestamp, which is not a lock: two runs that land on the same name must not
// overwrite one another, because the first run's fix would be gone with no copy left anywhere.
func TestFalsifyPatchNamesNeverOverwriteAnotherRun(t *testing.T) {
	dir := redCheckRepo(t)
	falsifyNow = func() time.Time { return time.Date(2024, 5, 6, 7, 8, 9, 10, time.UTC) }
	defer func() { falsifyNow = time.Now }()
	first, err := falsifySavePatch(dir, []byte("the first run's candidate fix\n"))
	if err != nil {
		t.Fatalf("save the first patch: %v", err)
	}
	second, err := falsifySavePatch(dir, []byte("the second run's candidate fix\n"))
	if err != nil {
		t.Fatalf("save the second patch: %v", err)
	}
	if first == second {
		t.Fatalf("both runs wrote %q: the second overwrote the first", first)
	}
	if got := string(readFile(t, first)); got != "the first run's candidate fix\n" {
		t.Fatalf("the first patch now reads %q: it was overwritten", got)
	}
	if got := string(readFile(t, second)); got != "the second run's candidate fix\n" {
		t.Fatalf("the second patch reads %q", got)
	}
	patches, err := filepath.Glob(filepath.Join(dir, ".git", "tsp", "falsify", "*.patch"))
	if err != nil || len(patches) != 2 {
		t.Fatalf("patches = %v, %v; want two beside each other", patches, err)
	}
}

// The proof of the restore is the whole working-tree status, not the list of paths the command meant to restore:
// a glob that expanded, or any other file the restore rewrote, fails the proof instead of passing it.
func TestFalsifyProofNamesAChangeOutsideTheSourceSet(t *testing.T) {
	before := []string{" M agent_test.go", " M order.go", "?? extra.go"}
	allowed := setOf([]string{"a*.go", "order.go"})
	if got, want := falsifyOutside(before, []string{" M agent_test.go", " M a*.go", "?? extra.go"}, allowed), []string(nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("a status whose only change is inside the source set named %v, want %v", got, want)
	}
	got := falsifyOutside(before, []string{" M order.go"}, allowed)
	if want := []string{"agent_test.go", "extra.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outside = %v, want %v", got, want)
	}
}

// The files a failed or partial restore leaves behind are read from git status, never from the list of files the
// command meant to restore: a file already back at HEAD must not be reported as still changed.
func TestFalsifyStillChangedReadsTheStatusNotTheSourceList(t *testing.T) {
	after := []string{"?? extra.go"}
	if got, want := falsifySourceLeft(after, []string{"extra.go", "order.go"}), []string{"extra.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("still changed = %v, want %v", got, want)
	}
}
