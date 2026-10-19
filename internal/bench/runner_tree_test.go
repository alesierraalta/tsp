package bench

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The per-run timeout is a promise about the whole agent, not its first process: the commands an agent
// starts (a test looping on a planted defect) must end with it, and one that holds the output pipe must not
// keep the run from returning. agy keeps it (TestAgyTimeoutEndsTheProcessesAgyStarted); every runner must.
func TestEveryRunnerTimeoutEndsTheProcessesItStarted(t *testing.T) {
	for _, tc := range []struct {
		binary string
		agent  Agent
	}{{"claude", claudeAgent}, {"codex", codexAgent}, {"pi", piAgent}} {
		t.Run(tc.binary, func(t *testing.T) {
			bin := t.TempDir()
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			script := "#!/bin/sh\nsleep 300 &\necho $! > " + pidFile + "\nwait\n"
			if err := os.WriteFile(filepath.Join(bin, tc.binary), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			child := func() int {
				data, _ := os.ReadFile(pidFile)
				pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
				return pid
			}
			t.Cleanup(func() {
				if pid := child(); pid > 0 {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan struct{})
			go func() {
				_, _ = tc.agent(ctx, t.TempDir(), Key{ID: "c1"}, Options{})
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s run did not return within 5s of a 1s timeout", tc.binary)
			}
			deadline := time.Now().Add(3 * time.Second)
			for pid := child(); pid > 0 && syscall.Kill(pid, 0) == nil; {
				if time.Now().After(deadline) {
					t.Fatalf("process %d that %s started is still running after the timeout", pid, tc.binary)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}
