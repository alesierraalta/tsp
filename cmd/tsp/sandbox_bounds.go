package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alesierraalta/tsp/internal/admit"
	"github.com/alesierraalta/tsp/internal/evidence"
)

// sandboxBounds are the resource and storage limits every sandboxed observation runs under. They are asked of
// the container runtime explicitly and then confirmed from inside the container, because a runtime that
// ignores a flag (no memory controller, a rootless setup without cgroup delegation) still starts the
// container and would otherwise let an unbounded run read as a sandboxed one.
//
// The tmpfs mounts are memory: /tmp and a replay's /w count against Memory, so their caps are well below it.
type sandboxBounds struct {
	Memory      int64 // bytes; swap is set to the same value, so the container has no swap on top of it
	CPUs        int
	Pids        int
	NoFile      int
	Shm         int64 // bytes
	Tmp         int64 // bytes, the writable /tmp (GOCACHE, HOME and XDG_CACHE_HOME live there)
	Work        int64 // bytes, a replay's writable copy of the staged tree
	OutputBytes int   // the most combined stdout and stderr one observation may produce
}

var sandboxDefaults = sandboxBounds{
	Memory:      4 << 30,
	CPUs:        2,
	Pids:        1024,
	NoFile:      4096,
	Shm:         64 << 20,
	Tmp:         1 << 30,
	Work:        2 << 30,
	OutputBytes: 8 << 20,
}

// sandboxPaths are where the preflight looks inside the container. They are parameters only so the preflight
// can be exercised on this machine against fixture files; the runner always uses sandboxContainerPaths.
type sandboxPaths struct {
	Cgroup     string
	ProcMounts string
	Tmp        string
	Work       string
	Shm        string
	Source     string
}

var sandboxContainerPaths = sandboxPaths{
	Cgroup:     "/sys/fs/cgroup",
	ProcMounts: "/proc/mounts",
	Tmp:        "/tmp",
	Work:       "/w",
	Shm:        "/dev/shm",
	Source:     "/src",
}

const (
	// sandboxConfirmationPrefix opens the one line the preflight prints when every bound holds. The runner
	// strips it before the output is digested, so a pin taken before the bounds existed still matches.
	sandboxConfirmationPrefix = "tsp-sandbox-bounds-ok "
	// sandboxFailurePrefix opens the line the preflight prints, on stderr, naming the bound it could not
	// confirm. The row's command never starts after it.
	sandboxFailurePrefix = "tsp-sandbox-bounds: "
	// sandboxBoundsExitCode is the preflight's exit status. It is read together with the failure line: a
	// docker that cannot invoke the command also exits 126, but never prints that line.
	sandboxBoundsExitCode = 126
)

// sandboxPreflightScript is POSIX sh that reads the limits the container actually received and exits before
// the row's command when one is missing or looser than asked. It reads cgroup v2 files: a cgroup v1 host has
// none of them and is refused, which is the point, since its bounds are not confirmed.
func sandboxPreflightScript(b sandboxBounds, p sandboxPaths, writable bool) string {
	cg := func(name string) string { return shellQuote(filepath.Join(p.Cgroup, name)) }
	var s strings.Builder
	fmt.Fprintf(&s, `tsp_fail() { printf '%%s%%s\n' %s "$1" >&2; exit %d; }
tsp_num() { case "$1" in ''|*[!0-9]*) return 1 ;; esac; }
tsp_kib() { df -Pk "$1" 2>/dev/null | tail -n 1 | awk '{print $2}'; }
tsp_mount() { awk -v mp="$1" -v f="$2" '$2 == mp { v = $f } END { print v }' %s; }
tsp_mem=$(cat %s 2>/dev/null) || tsp_fail "the memory limit cannot be read from the container's cgroup"
tsp_num "$tsp_mem" || tsp_fail "the memory limit is '$tsp_mem', want %d"
[ "$tsp_mem" -le %d ] || tsp_fail "the memory limit is $tsp_mem, above the %d bound"
tsp_pids=$(cat %s 2>/dev/null) || tsp_fail "the pids limit cannot be read from the container's cgroup"
tsp_num "$tsp_pids" || tsp_fail "the pids limit is '$tsp_pids', want %d"
[ "$tsp_pids" -le %d ] || tsp_fail "the pids limit is $tsp_pids, above the %d bound"
read -r tsp_quota tsp_period < %s 2>/dev/null || tsp_fail "the cpu quota cannot be read from the container's cgroup"
tsp_num "$tsp_quota" && tsp_num "$tsp_period" && [ "$tsp_period" -gt 0 ] || tsp_fail "the cpu quota is '$tsp_quota $tsp_period', want at most %d CPUs"
[ "$tsp_quota" -le $((%d * tsp_period)) ] || tsp_fail "the cpu quota is $tsp_quota/$tsp_period, above the %d CPU bound"
tsp_nofile=$(ulimit -n)
tsp_num "$tsp_nofile" || tsp_fail "the open-file limit is '$tsp_nofile', want %d"
[ "$tsp_nofile" -le %d ] || tsp_fail "the open-file limit is $tsp_nofile, above the %d bound"
`, shellQuote(sandboxFailurePrefix), sandboxBoundsExitCode, shellQuote(p.ProcMounts),
		cg("memory.max"), b.Memory, b.Memory, b.Memory,
		cg("pids.max"), b.Pids, b.Pids, b.Pids,
		cg("cpu.max"), b.CPUs, b.CPUs, b.CPUs,
		b.NoFile, b.NoFile, b.NoFile)
	size := func(v, mp, what string, bound int64) {
		fmt.Fprintf(&s, `%s=$(tsp_kib %s)
tsp_num "$%s" && [ "$%s" -gt 0 ] || tsp_fail "the size of %s at %s cannot be read"
[ "$%s" -le %d ] || tsp_fail "%s at %s is $%s KiB, above the %d KiB bound"
`, v, shellQuote(mp), v, v, what, mp, v, bound>>10, what, mp, v, bound>>10)
	}
	size("tsp_tmp", p.Tmp, "the /tmp tmpfs", b.Tmp)
	size("tsp_shm", p.Shm, "the shared-memory mount", b.Shm)
	fmt.Fprintf(&s, `case ",$(tsp_mount / 4)," in *,ro,*) ;; *) tsp_fail "the container root is writable, want it read-only" ;; esac
`)
	confirm := "printf '" + sandboxConfirmationPrefix + "memory=%s pids=%s cpu=%s/%s tmp=%s shm=%s nofile=%s"
	confirmArgs := ` "$tsp_mem" "$tsp_pids" "$tsp_quota" "$tsp_period" "$tsp_tmp" "$tsp_shm" "$tsp_nofile"`
	if writable {
		fmt.Fprintf(&s, `case ",$(tsp_mount %s 4)," in *,ro,*) ;; *) tsp_fail "the staged tree at %s is not mounted read-only" ;; esac
[ "$(tsp_mount %s 3)" = tmpfs ] || tsp_fail "the replay workspace at %s is not a tmpfs"
case ",$(tsp_mount %s 4)," in *,rw,*) ;; *) tsp_fail "the replay workspace at %s is not writable" ;; esac
`, shellQuote(p.Source), p.Source, shellQuote(p.Work), p.Work, shellQuote(p.Work), p.Work)
		size("tsp_work", p.Work, "the replay workspace", b.Work)
		confirm += " work=%s"
		confirmArgs += ` "$tsp_work"`
	}
	fmt.Fprintf(&s, "%s\\n'%s\n", confirm, confirmArgs)
	return s.String()
}

// sandboxScript is everything the container runs: the writable user directories first (the root is
// read-only, so nothing under /tmp exists until something makes it), the preflight, a replay's copy of the
// staged tree into its bounded workspace, and last the row's own command, whose exit status is the script's.
func sandboxScript(command string, b sandboxBounds, p sandboxPaths, writable bool) string {
	var s strings.Builder
	s.WriteString("mkdir -p /tmp/home /tmp/xdg\n")
	s.WriteString(sandboxPreflightScript(b, p, writable))
	if writable {
		fmt.Fprintf(&s, "cp -a %s/. %s/ || tsp_fail \"the staged tree did not fit in the replay workspace at %s\"\n",
			p.Source, p.Work, p.Work)
	}
	s.WriteString(command)
	return s.String()
}

// sandboxArgsWithin is docker's argument vector for one bounded observation. The read-only mode mounts the
// tree read-only at /w. A replay needs to write, and a writable bind mount is unbounded disk on this machine,
// so it mounts the staged copy read-only at /src and works on a bounded tmpfs copy at /w instead: each run
// starts from the staged tree, and nothing one run writes reaches the next.
func sandboxArgsWithin(dir, image, mount, command string, b sandboxBounds, p sandboxPaths, name string) ([]string, error) {
	if !filepath.IsAbs(dir) {
		return nil, evidence.Refusal{
			Reason: evidence.ReasonMisconfigured,
			Detail: fmt.Sprintf("the sandbox mounts the working directory by absolute path, and %q is not one", dir),
		}
	}
	writable := mount == admit.SandboxWritable
	args := []string{
		"run", "--rm", "--quiet",
		"--name", name,
		"--network", "none",
		"--read-only",
		"--memory", fmt.Sprint(b.Memory),
		"--memory-swap", fmt.Sprint(b.Memory),
		"--cpus", fmt.Sprint(b.CPUs),
		"--pids-limit", fmt.Sprint(b.Pids),
		"--ulimit", fmt.Sprintf("nofile=%d:%d", b.NoFile, b.NoFile),
		"--shm-size", fmt.Sprint(b.Shm),
		"--tmpfs", fmt.Sprintf("%s:rw,exec,size=%d,mode=1777", p.Tmp, b.Tmp),
	}
	if writable {
		args = append(args,
			"--tmpfs", fmt.Sprintf("%s:rw,exec,size=%d,mode=1777", p.Work, b.Work),
			"-v", dir+":"+p.Source+":ro")
	} else {
		args = append(args, "-v", dir+":"+p.Work+":"+mount)
	}
	return append(args,
		"-w", p.Work,
		"-e", "GOCACHE=/tmp/gocache",
		"-e", "HOME=/tmp/home",
		"-e", "XDG_CACHE_HOME=/tmp/xdg",
		image, "sh", "-c", sandboxScript(command, b, p, writable),
	), nil
}

// sandboxContainerName is unique per observation so a run the deadline cut short can be removed by name: a
// docker CLI that is killed does not stop the container it started.
func sandboxContainerName() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("tsp-sandbox-%d", time.Now().UnixNano())
	}
	return "tsp-sandbox-" + hex.EncodeToString(b[:])
}

// stripSandboxConfirmation removes the preflight's confirmation line from the front of the output, and
// reports whether it was there.
func stripSandboxConfirmation(output string) (string, bool) {
	if !strings.HasPrefix(output, sandboxConfirmationPrefix) {
		return output, false
	}
	if i := strings.IndexByte(output, '\n'); i >= 0 {
		return output[i+1:], true
	}
	return "", true
}

// sandboxBoundsFailure returns the detail of the preflight's failure line, when the output carries one.
func sandboxBoundsFailure(output string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(output))
	for sc.Scan() {
		if detail, ok := strings.CutPrefix(sc.Text(), sandboxFailurePrefix); ok {
			return detail, true
		}
	}
	return "", false
}

// cappedBuffer keeps at most limit bytes and calls over once when more arrive.
type cappedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	limit    int
	exceeded bool
	over     func()
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exceeded {
		return len(p), nil
	}
	if room := c.limit - c.buf.Len(); len(p) > room {
		c.buf.Write(p[:max(room, 0)])
		c.exceeded = true
		c.over()
		return len(p), nil
	}
	return c.buf.Write(p)
}

// runSandboxWithin runs one bounded observation with the docker binary named by docker.
func runSandboxWithin(ctx context.Context, dir, image, mount, command string, b sandboxBounds, p sandboxPaths, docker string) (string, error) {
	name := sandboxContainerName()
	args, err := sandboxArgsWithin(dir, image, mount, command, b, p, name)
	if err != nil {
		return "", err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := &cappedBuffer{limit: b.OutputBytes, over: cancel}
	cmd := exec.CommandContext(runCtx, docker, args...)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	out.mu.Lock()
	output, exceeded := out.buf.String(), out.exceeded
	out.mu.Unlock()
	if exceeded || ctx.Err() != nil {
		removeSandboxContainer(docker, name)
	}
	output, _ = stripSandboxConfirmation(output)
	if exceeded {
		return output, evidence.Refusal{
			Reason: evidence.ReasonSandboxOutputLimit,
			Detail: fmt.Sprintf("the command printed more than the sandbox's %d-byte output bound, so it was stopped and its container removed; narrow the command so its output fits", b.OutputBytes),
		}
	}
	if err == nil {
		return output, nil
	}
	// The caller's deadline must stay recognisable to the admission, so it is returned as it is rather
	// than wrapped in a refusal about the sandbox.
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	var exit *exec.ExitError
	if detail, ok := sandboxBoundsFailure(output); ok && errors.As(err, &exit) && exit.ExitCode() == sandboxBoundsExitCode {
		return output, evidence.Refusal{
			Reason: evidence.ReasonSandboxBoundsUnverified,
			Detail: "the sandbox could not confirm its bounds, so the command never ran: " + detail + "; the container runtime must apply cgroup v2 memory, cpu and pids limits for this mode",
		}
	}
	return output, sandboxRefusal(image, output, err)
}

// removeSandboxContainer stops a container whose docker CLI was killed. It runs on its own short deadline,
// because the caller's has already passed.
func removeSandboxContainer(docker, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, docker, "rm", "-f", name).Run()
}
