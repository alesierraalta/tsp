//go:build linux || darwin

package bench

import (
	"os"
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

// A suite that finishes in time is unaffected: its exit code and output come back as before.
func TestRunSuiteStillReportsAFinishedSuite(t *testing.T) {
	res, err := RunSuite(t.TempDir(), `sh -c "echo ok; exit 3"`, 5*time.Second)
	if err != nil || res.ExitCode != 3 || !strings.Contains(res.Output, "ok") {
		t.Fatalf("RunSuite = %+v, %v; want exit 3 with its output", res, err)
	}
}
