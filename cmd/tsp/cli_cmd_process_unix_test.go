//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func testShellQuote(path string) string { return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'" }

// The deadline must end the whole group, not just the shell that started the run: a child that
// ignores TERM and a grandchild that inherited the pipes would otherwise outlive the test.
func TestCLICmdDeadlineReapsTheWholeGroup(t *testing.T) {
	dir := t.TempDir()
	parentPID := filepath.Join(dir, "parent.pid")
	childPID := filepath.Join(dir, "child.pid")
	childScript := filepath.Join(dir, "child.sh")
	parentScript := filepath.Join(dir, "parent.sh")
	writeScript := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeScript(childScript, "#!/bin/sh\nprintf '%s\\n' $$ > "+testShellQuote(childPID)+"\nexec sleep 30\n")
	writeScript(parentScript, "#!/bin/sh\ntrap '' TERM INT\nprintf '%s\\n' $$ > "+testShellQuote(parentPID)+"\n"+
		testShellQuote(childScript)+" &\nwhile [ ! -s "+testShellQuote(childPID)+" ]; do :; done\nsleep 1\n")

	readPIDs := func() []int {
		var pids []int
		for _, path := range []string{parentPID, childPID} {
			body, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if pid, err := strconv.Atoi(strings.TrimSpace(string(body))); err == nil && pid > 0 {
				pids = append(pids, pid)
			}
		}
		return pids
	}
	t.Cleanup(func() {
		for _, pid := range readPIDs() {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	cmd := cliCmdDeadline(t, 400*time.Millisecond, parentScript)
	_ = cmd.Run()

	pids := readPIDs()
	if len(pids) != 2 {
		t.Fatalf("the runaway never recorded both pids: %v", pids)
	}
	for deadline := time.Now().Add(2 * time.Second); ; {
		survivors := 0
		for _, pid := range pids {
			if syscall.Kill(pid, 0) == nil {
				survivors++
			}
		}
		if survivors == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %v survived the deadline", survivors, pids)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
