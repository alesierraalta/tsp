package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// calibrateRepo is a throwaway worktree as the seeder defines it: a git repository on a calib-* branch holding one
// Go file with probing sites (a comparison, an if condition and two bool returns).
func calibrateRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	src := `package demo

func wider(a, b int) bool {
	if a > b {
		return true
	}
	return false
}
`
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", branch}, {"add", "-A"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "base"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func readDemo(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "demo.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The seeder only runs on a throwaway worktree, and --list is how an operator sees whether the files have any
// probing site at all before a calibration run.
func TestCalibrateListsTheSitesOfAThrowawayWorktree(t *testing.T) {
	bin := buildCLI(t)
	dir := calibrateRepo(t, "calib-test")
	out, code := runCLI(t, bin, "calibrate", "--root", dir, "--files", "demo.go", "--list")
	if code != 0 || !strings.Contains(out, "total candidate sites: ") {
		t.Fatalf("calibrate --list = %d %s", code, out)
	}
	if !regexp.MustCompile(`(?m)^\s+[1-9][0-9]*\s+demo\.go$`).MatchString(out) {
		t.Fatalf("--list does not count the file's sites: %s", out)
	}
	// --list writes nothing.
	if got := readDemo(t, dir); !strings.Contains(got, "if a > b {") {
		t.Fatalf("--list changed the file: %s", got)
	}
}

// A seed applies, the key lands outside the tree with its sha256, and replaying the key reproduces the same
// mutants: that is what lets two skill versions face identical ground.
func TestCalibrateSeedsAndReplaysASealedKey(t *testing.T) {
	bin := buildCLI(t)
	dir := calibrateRepo(t, "calib-test")
	before := readDemo(t, dir)
	key := filepath.Join(t.TempDir(), "key.json")

	out, code := runCLI(t, bin, "calibrate", "--root", dir, "--files", "demo.go", "--count", "1", "--seed", "3", "--key-out", key)
	if code != 0 || !strings.Contains(out, "applied: 1") {
		t.Fatalf("calibrate = %d %s", code, out)
	}
	if !regexp.MustCompile(`key sha256: [0-9a-f]{64}`).MatchString(out) {
		t.Fatalf("the key digest is not printed: %s", out)
	}
	seeded := readDemo(t, dir)
	if seeded == before {
		t.Fatal("no mutant was applied")
	}
	if _, err := os.Stat(key); err != nil {
		t.Fatalf("the key is not outside the tree: %v", err)
	}

	// Replay starts from the pristine tree and must land the same bytes.
	if out, err := exec.Command("git", "-C", dir, "checkout", "--", "demo.go").CombinedOutput(); err != nil {
		t.Fatalf("restore: %v %s", err, out)
	}
	out, code = runCLI(t, bin, "calibrate", "--root", dir, "--replay", key)
	if code != 0 || !strings.Contains(out, "replayed: 1") {
		t.Fatalf("calibrate --replay = %d %s", code, out)
	}
	if got := readDemo(t, dir); got != seeded {
		t.Fatalf("replay produced other bytes:\n--- seeded\n%s\n--- replayed\n%s", seeded, got)
	}
}

// The guard is the script's: calibration never seeds a branch that is not disposable.
func TestCalibrateRefusesABranchThatIsNotThrowaway(t *testing.T) {
	bin := buildCLI(t)
	dir := calibrateRepo(t, "main")
	key := filepath.Join(t.TempDir(), "key.json")
	out, code := runCLI(t, bin, "calibrate", "--root", dir, "--files", "demo.go", "--count", "1", "--key-out", key)
	if code != 1 || !strings.Contains(out, "calib-") {
		t.Fatalf("a main branch must be refused naming the allowed prefixes: %d %s", code, out)
	}
	if _, err := os.Stat(key); err == nil {
		t.Fatal("a refused run wrote the key")
	}
}

// A missing value the operator still holds on the line is a usage error (2); a key path the plan refuses is the
// plan's answer (1).
func TestCalibrateUsageErrors(t *testing.T) {
	bin := buildCLI(t)
	dir := calibrateRepo(t, "calib-test")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no root", []string{"calibrate", "--files", "demo.go", "--list"}, "--root"},
		{"no files", []string{"calibrate", "--root", dir, "--list"}, "--files"},
		{"no key outside list", []string{"calibrate", "--root", dir, "--files", "demo.go", "--count", "1"}, "--key-out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runCLI(t, bin, tc.args...)
			if code != 2 || !strings.Contains(out, tc.want) {
				t.Fatalf("= %d, want 2 naming %q: %s", code, tc.want, out)
			}
		})
	}
	// The key cannot live inside the worktree: that is the seeder refusing, not a missing flag.
	inside := filepath.Join(dir, "key.json")
	out, code := runCLI(t, bin, "calibrate", "--root", dir, "--files", "demo.go", "--count", "1", "--key-out", inside)
	if code != 1 || !strings.Contains(out, "outside") {
		t.Fatalf("a key inside the tree must be refused: %d %s", code, out)
	}
	if _, err := os.Stat(inside); err == nil {
		t.Fatal("the refused key was written")
	}
}
