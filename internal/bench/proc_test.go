//go:build linux || darwin

package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// grandchildAlive starts a suite whose child spawns a sleeping grandchild and then outlives the timeout, as
// node --test does with one process per test file, and reports whether the grandchild survived the kill. The
// run must also return close to its timeout: a grandchild holding the output pipe must not hold the caller.
func grandchildAlive(t *testing.T, run func(dir, suite string)) bool {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")
	start := time.Now()
	run(dir, `sh -c "sleep 30 & echo $! > `+pidFile+`; wait"`)
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("the timed-out suite returned after %s, held by its grandchild", took.Round(time.Second))
	}
	return survived(t, pidFile)
}

// survived reads the pid a timed-out run wrote and reports whether that process outlived the kill, killing it
// if so, so a failing test does not leave it behind either.
func survived(t *testing.T, pidFile string) bool {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the suite never started its grandchild: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("grandchild pid %q: %v", raw, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return true
}

// A suite killed by its timeout takes its whole process tree with it: a surviving node --test worker holds the
// output pipe open and keeps running for hours after the catch check moved on.
func TestRunSuiteTimeoutKillsTheWholeProcessTree(t *testing.T) {
	if grandchildAlive(t, func(dir, suite string) { _, _ = RunSuite(dir, suite, 300*time.Millisecond) }) {
		t.Fatal("RunSuite left a grandchild of the timed-out suite running")
	}
}

func TestPerTestTimeoutKillsTheWholeProcessTree(t *testing.T) {
	if grandchildAlive(t, func(dir, suite string) {
		_, _ = runCapture(dir, []string{"sh", "-c", strings.TrimSuffix(strings.TrimPrefix(suite, `sh -c "`), `"`)}, 300*time.Millisecond)
	}) {
		t.Fatal("runCapture left a grandchild of the timed-out suite running")
	}
}

// A descendant in a session of its own escapes a kill aimed at the process group: Pi's bash tool spawns every
// command detached and only kills it itself while Pi is alive, so killing the agent's group left a
// `node --test | head` running for hours after the run's timeout. The timeout must end it too.
func TestTimeoutKillsADescendantInASessionOfItsOwn(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid is not installed")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "detached.pid")
	_, _ = runCapture(dir, []string{"sh", "-c", "setsid sleep 30 & echo $! > " + pidFile + "; wait"}, 300*time.Millisecond)
	if survived(t, pidFile) {
		t.Fatal("the timeout left a descendant that runs in a session of its own")
	}
}

// A background job an agent's command leaves behind (`node server.js &` inside a Pi tool command that then
// returns) loses its parent long before the timeout: it is reparented and no longer a descendant of the run,
// yet it was started by the run and must not outlive it.
func TestTimeoutKillsAnOrphanTheRunLeftBehind(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid is not installed")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "orphan.pid")
	// The detached shell starts the job and exits at once, so the job is an orphan before the timeout fires.
	script := "setsid sh -c 'sleep 30 & echo $! > " + pidFile + "'; sleep 5"
	_, _ = runCapture(dir, []string{"sh", "-c", script}, 500*time.Millisecond)
	if survived(t, pidFile) {
		t.Fatal("the timeout left an orphan the run had started")
	}
}

// The kill is aimed: a process this run did not start (a concurrent run's, the operator's) survives it, even
// when it runs the same command in the same directory.
func TestTimeoutSparesAProcessTheRunDidNotStart(t *testing.T) {
	dir := t.TempDir()
	outsider := exec.Command("sleep", "30")
	outsider.Dir = dir
	outsider.Env = append(os.Environ(), runMarkEnv+"=another-run")
	if err := outsider.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outsider.Process.Kill(); _ = outsider.Wait() }()
	_, _ = runCapture(dir, []string{"sh", "-c", "sleep 30"}, 300*time.Millisecond)
	if err := syscall.Kill(outsider.Process.Pid, 0); err != nil {
		t.Fatalf("the timeout killed a process another run had started: %v", err)
	}
}

// The run is marked through its environment, which every process it starts inherits; a caller that assigns
// cmd.Env after boundProcessTree would drop the mark without a word, and the orphan kill would find nothing.
// Every caller in the package therefore sets its environment first.
func TestNoCallerReplacesTheEnvironmentAfterBoundingTheTree(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		bound := false
		for i, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "boundProcessTree(cmd)":
				bound = true
			case bound && strings.HasPrefix(line, "cmd.Env ="):
				t.Errorf("%s:%d replaces cmd.Env after boundProcessTree, dropping the run mark", file, i+1)
			case strings.HasPrefix(line, "func "):
				bound = false
			}
		}
	}
}

// A suite that finishes in time is unaffected: its exit code and output come back as before.
func TestRunSuiteStillReportsAFinishedSuite(t *testing.T) {
	res, err := RunSuite(t.TempDir(), `sh -c "echo ok; exit 3"`, 5*time.Second)
	if err != nil || res.ExitCode != 3 || !strings.Contains(res.Output, "ok") {
		t.Fatalf("RunSuite = %+v, %v; want exit 3 with its output", res, err)
	}
}
