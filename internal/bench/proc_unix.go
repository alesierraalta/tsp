//go:build linux || darwin

package bench

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// runMarkPrefix starts the variable that marks every process a bounded run starts. Each run gets a variable of
// its own (TSP_RUN_<mark>=1), not a shared name with its mark as the value: a run started inside another run
// inherits the outer mark, and with a shared name the inner one would replace it, so the outer run's kill would
// miss everything the inner run started.
const runMarkPrefix = "TSP_RUN_"

// runMarkEntry is the environment entry that carries a run's mark.
func runMarkEntry(mark string) string { return runMarkPrefix + mark + "=1" }

// boundProcessTree makes a context timeout end the whole tree a suite started, not only its first process.
// node --test runs each test file in its own worker: killing the parent alone leaves a worker that loops on a
// planted defect running for hours, and it holds the output pipe open so Wait never returns. The suite gets
// its own process group, the cancel kills that group, and Wait stops waiting on the pipe shortly after.
//
// A group is not enough for an agent: Pi's bash tool spawns every command detached, in a group of its own,
// and only kills it itself while Pi is alive. Killing Pi's group left those commands running for hours, so the
// cancel also kills every descendant it finds below the process, whatever group or session it moved to.
//
// A background job whose parent already returned (`node server.js &` inside a tool command) is reparented
// and no longer a descendant at all. The run is therefore marked through its environment, which every process
// it starts inherits, and the cancel also kills every process carrying that mark. Call it after setting
// cmd.Env: replacing the environment afterwards drops the mark.
func boundProcessTree(cmd *exec.Cmd) {
	mark := newRunMark()
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, runMarkEntry(mark))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		below := append(descendants(cmd.Process.Pid), markedProcesses(mark)...)
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		for _, pid := range below {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
}

// runBounded runs a command bounded by boundProcessTree and, however it ends, kills every process still carrying
// the run's mark: a run that finished in time can leave a job behind (a test server it never stopped), and
// across a reading of a hundred runs those pile up just as a timed-out run's did.
func runBounded(cmd *exec.Cmd) error {
	err := cmd.Run()
	if mark := runMarkOf(cmd); mark != "" {
		for _, pid := range markedProcesses(mark) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	return commandOutcome(cmd, err)
}

// runMarkOf reads the mark boundProcessTree gave the command: the last one, since a run started inside another
// run inherits the outer mark first and appends its own.
func runMarkOf(cmd *exec.Cmd) string {
	for i := len(cmd.Env) - 1; i >= 0; i-- {
		if name, ok := strings.CutSuffix(cmd.Env[i], "=1"); ok && strings.HasPrefix(name, runMarkPrefix) {
			return strings.TrimPrefix(name, runMarkPrefix)
		}
	}
	return ""
}

// newRunMark returns a value no other run shares, so concurrent runs never kill each other's processes.
func newRunMark() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// markedProcesses lists every process whose environment carries the run's mark, this process excluded. A
// process may not be read (another user's, or gone): it was not started by this run and is skipped.
func markedProcesses(mark string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	want := []byte(runMarkEntry(mark))
	self := os.Getpid()
	var marked []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err != nil {
			continue
		}
		for _, entry := range bytes.Split(env, []byte{0}) {
			if bytes.Equal(entry, want) {
				marked = append(marked, pid)
				break
			}
		}
	}
	return marked
}

// descendants lists every process below root by following parent pids in /proc. It is taken before the kill,
// while the tree is still linked: once the root dies its orphans are reparented and the link is gone. Where
// /proc does not exist (darwin) it finds nothing and the group kill stands alone, as before.
func descendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// The command name may hold spaces and parentheses; the parent pid is the second field after the
		// last closing parenthesis.
		stat := string(raw)
		end := strings.LastIndexByte(stat, ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(stat[end+1:])
		if len(fields) < 2 {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	var below []int
	for queue := []int{root}; len(queue) > 0; queue = queue[1:] {
		for _, child := range children[queue[0]] {
			below = append(below, child)
			queue = append(queue, child)
		}
	}
	return below
}
