package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/admit"
	"github.com/alesierraalta/tsp/internal/evidence"
)

// sandboxArgsContain reports whether every element of want appears in args as its own argument.
func sandboxArgsContain(t *testing.T, args []string, want ...string) {
	t.Helper()
	for _, w := range want {
		found := false
		for _, a := range args {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("args do not carry %q: %q", w, strings.Join(args, " "))
		}
	}
}

func sandboxArgsAvoid(t *testing.T, args []string, unwanted string) {
	t.Helper()
	for _, a := range args {
		if strings.Contains(a, unwanted) {
			t.Fatalf("args carry %q in %q, which the bounded sandbox must not use", unwanted, a)
		}
	}
}

// The read-only mode asks the runtime for every bound explicitly, and the working tree is still mounted read-only.
func TestSandboxArgsCarryExplicitResourceAndStorageBounds(t *testing.T) {
	dir := t.TempDir()
	args, err := sandboxArgsWithin(dir, "example/go:1", admit.SandboxReadOnly, "printf one", sandboxDefaults, sandboxContainerPaths, "tsp-test")
	if err != nil {
		t.Fatal(err)
	}
	sandboxArgsContain(t, args,
		"--name", "tsp-test",
		"--read-only", "--network", "none",
		"--memory", "4294967296",
		"--memory-swap", "4294967296",
		"--cpus", "2",
		"--pids-limit", "1024",
		"--ulimit", "nofile=4096:4096",
		"--shm-size", "67108864",
		"--tmpfs", "/tmp:rw,exec,size=1073741824,mode=1777",
		"-v", dir+":/w:ro",
		"-w", "/w",
	)
	sandboxArgsAvoid(t, args, ":/w:rw")
	script := args[len(args)-1]
	if !strings.HasPrefix(script, "mkdir -p /tmp/home /tmp/xdg\n") {
		t.Fatalf("script = %q, want the writable user dirs created first", script)
	}
	for _, want := range []string{"tsp-sandbox-bounds-ok", "memory.max", "pids.max", "cpu.max", "printf one"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script does not confirm %q:\n%s", want, script)
		}
	}
}

// A replay needs a writable workspace, so it gets a per-run tmpfs seeded from a read-only copy of the staged tree.
// The host tree is never the writable mount, which is what kept a replayed row unbounded.
func TestSandboxArgsForAReplaySeedABoundedTmpfsFromAReadOnlySource(t *testing.T) {
	dir := t.TempDir()
	args, err := sandboxArgsWithin(dir, "example/go:1", admit.SandboxWritable, "printf one", sandboxDefaults, sandboxContainerPaths, "tsp-test")
	if err != nil {
		t.Fatal(err)
	}
	sandboxArgsContain(t, args,
		"--tmpfs", "/tmp:rw,exec,size=1073741824,mode=1777",
		"--tmpfs", "/w:rw,exec,size=2147483648,mode=1777",
		"-v", dir+":/src:ro",
		"-w", "/w",
	)
	sandboxArgsAvoid(t, args, ":/w:rw")
	script := args[len(args)-1]
	if !strings.Contains(script, "cp -a /src/. /w/") {
		t.Fatalf("script does not seed the bounded workspace from the read-only source:\n%s", script)
	}
	if strings.Index(script, "tsp-sandbox-bounds-ok") > strings.Index(script, "cp -a /src/. /w/") {
		t.Fatalf("script copies before it has confirmed the bounds:\n%s", script)
	}
}

type sandboxPreflightFixture struct {
	paths      sandboxPaths
	binDir     string
	mountsFile string
}

func writeSandboxFakeDF(t *testing.T, binDir string, sizes map[string]int) {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\nmp=\"${2:-}\"\ncase \"$mp\" in\n")
	for _, mp := range []string{"/tmp", "/w", "/dev/shm"} {
		size, ok := sizes[mp]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "%s) size=%d ;;\n", mp, size)
	}
	b.WriteString("*) size=0 ;;\nesac\n")
	b.WriteString("echo \"tmpfs $size 0 $size 0% $mp\"\n")
	path := filepath.Join(binDir, "df")
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatal(err)
	}
}

func sandboxMounts(sizeTmp, sizeWork, sizeShm int, rootOpts, srcOpts string) string {
	return strings.Join([]string{
		fmt.Sprintf("overlay / overlay %s,relatime 0 0", rootOpts),
		fmt.Sprintf("tmpfs /tmp tmpfs rw,size=%dk 0 0", sizeTmp),
		fmt.Sprintf("tmpfs /w tmpfs rw,size=%dk 0 0", sizeWork),
		fmt.Sprintf("/dev/sda1 /src ext4 %s,relatime 0 0", srcOpts),
		fmt.Sprintf("tmpfs /dev/shm tmpfs rw,size=%dk 0 0", sizeShm),
	}, "\n") + "\n"
}

func newSandboxPreflightFixture(t *testing.T) *sandboxPreflightFixture {
	t.Helper()
	root := t.TempDir()
	cgroup := filepath.Join(root, "cgroup")
	if err := os.MkdirAll(cgroup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroup, "memory.max"), []byte("4294967296\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroup, "memory.swap.max"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroup, "pids.max"), []byte("1024\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroup, "cpu.max"), []byte("200000 100000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounts := filepath.Join(root, "mounts")
	if err := os.WriteFile(mounts, []byte(sandboxMounts(1048576, 2097152, 65536, "ro", "ro")), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSandboxFakeDF(t, bin, map[string]int{"/tmp": 1048576, "/w": 2097152, "/dev/shm": 65536})
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &sandboxPreflightFixture{
		paths: sandboxPaths{
			Cgroup:     cgroup,
			ProcMounts: mounts,
			Tmp:        "/tmp",
			Work:       "/w",
			Shm:        "/dev/shm",
			Source:     "/src",
		},
		binDir:     bin,
		mountsFile: mounts,
	}
}

// runSandboxPreflight executes only the preflight half plus a sentinel, so a passing preflight proves it would
// have let the row's command run and a failing one proves the command never starts.
func runSandboxPreflight(t *testing.T, script string) (string, int) {
	t.Helper()
	// ulimit is a shell builtin; lowering the soft limit always succeeds and leaves the check inside the bound.
	cmd := exec.Command("sh", "-c", "ulimit -n 1024\n"+script+"\nprintf 'ROW RAN\\n'\n")
	out, err := cmd.CombinedOutput()
	code := 0
	var exit *exec.ExitError
	if err != nil {
		if !errors.As(err, &exit) {
			t.Fatalf("preflight did not run: %v\n%s", err, out)
		}
		code = exit.ExitCode()
	}
	return string(out), code
}

func TestSandboxPreflightConfirmsTheBoundsTheContainerActuallySees(t *testing.T) {
	f := newSandboxPreflightFixture(t)
	script := sandboxPreflightScript(sandboxDefaults, f.paths, false)
	out, code := runSandboxPreflight(t, script)
	if code != 0 {
		t.Fatalf("preflight = %d, want the row to run\n%s", code, out)
	}
	if !strings.Contains(out, sandboxConfirmationPrefix) {
		t.Fatalf("preflight did not confirm the bounds:\n%s", out)
	}
	for _, want := range []string{"memory=4294967296", "pids=1024", "cpu=200000/100000", "tmp=1048576", "shm=65536", "nofile=1024"} {
		if !strings.Contains(out, want) {
			t.Fatalf("confirmation does not carry %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "ROW RAN") {
		t.Fatalf("a confirmed sandbox must let the row run:\n%s", out)
	}
}

// A control the runtime silently ignored is not a bound: the wrapper has to see the effective value from inside
// the container and refuse before the row's command starts.
func TestSandboxPreflightRefusesBoundsThatAreNotEstablished(t *testing.T) {
	cases := []struct {
		name     string
		writable bool
		break_   func(t *testing.T, f *sandboxPreflightFixture)
		detail   string
	}{
		{
			name: "memory is unlimited",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(filepath.Join(f.paths.Cgroup, "memory.max"), []byte("max\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "memory limit",
		},
		{
			name: "memory is above the bound",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(filepath.Join(f.paths.Cgroup, "memory.max"), []byte("8589934592\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "above the",
		},
		{
			name: "swap is unlimited",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(filepath.Join(f.paths.Cgroup, "memory.swap.max"), []byte("max\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "swap limit",
		},
		{
			name: "pids are above the bound",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(filepath.Join(f.paths.Cgroup, "pids.max"), []byte("4096\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "pids limit",
		},
		{
			name: "cpu is unlimited",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(filepath.Join(f.paths.Cgroup, "cpu.max"), []byte("max 100000\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "cpu quota",
		},
		{
			name: "tmpfs is larger than the bound",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				writeSandboxFakeDF(t, f.binDir, map[string]int{"/tmp": 2097152, "/w": 2097152, "/dev/shm": 65536})
			},
			detail: "above the",
		},
		{
			name: "tmpfs size row is wrapped",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				df := "#!/bin/sh\nprintf 'Filesystem 1024-blocks Used Available Capacity Mounted on\\nvery-long-filesystem-name\\n 9999999 1 1 1%% %s\\n' \"$2\"\n"
				if err := os.WriteFile(filepath.Join(f.binDir, "df"), []byte(df), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			detail: "cannot be read",
		},
		{
			name: "root is writable",
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(f.mountsFile, []byte(sandboxMounts(1048576, 2097152, 65536, "rw", "ro")), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "root is writable",
		},
		{
			name:     "replay source is writable",
			writable: true,
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(f.mountsFile, []byte(sandboxMounts(1048576, 2097152, 65536, "ro", "rw")), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "staged tree",
		},
		{
			name:     "replay workspace is read-only",
			writable: true,
			break_: func(t *testing.T, f *sandboxPreflightFixture) {
				if err := os.WriteFile(f.mountsFile, []byte(strings.Replace(sandboxMounts(1048576, 2097152, 65536, "ro", "ro"), "tmpfs /w tmpfs rw,", "tmpfs /w tmpfs ro,", 1)), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			detail: "workspace",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSandboxPreflightFixture(t)
			c.break_(t, f)
			script := sandboxPreflightScript(sandboxDefaults, f.paths, c.writable)
			out, code := runSandboxPreflight(t, script)
			if code != sandboxBoundsExitCode {
				t.Fatalf("preflight = %d, want %d when a control is not established\n%s", code, sandboxBoundsExitCode, out)
			}
			if !strings.Contains(out, sandboxFailurePrefix) || !strings.Contains(out, c.detail) {
				t.Fatalf("refusal does not name %q:\n%s", c.detail, out)
			}
			if strings.Contains(out, "ROW RAN") {
				t.Fatalf("the row ran without its bounds:\n%s", out)
			}
		})
	}
}

func TestSandboxWrapperScriptsAreValidPOSIXShell(t *testing.T) {
	for _, writable := range []bool{false, true} {
		script := sandboxScript("printf one", sandboxDefaults, sandboxContainerPaths, writable)
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("writable=%v script is not valid sh: %v\n%s", writable, err, out)
		}
		if !strings.HasSuffix(script, "printf one") {
			t.Fatalf("writable=%v script does not end with the row's command:\n%s", writable, script)
		}
	}
}

func TestStripSandboxConfirmationLeavesTheRowsOwnOutput(t *testing.T) {
	line := sandboxConfirmationPrefix + "memory=1\n"
	rest, ok := stripSandboxConfirmation(line + "ok\n")
	if !ok || rest != "ok\n" {
		t.Fatalf("strip = %q, %v; want the confirmation removed", rest, ok)
	}
	if _, ok := stripSandboxConfirmation("ok\n"); ok {
		t.Fatal("output without a confirmation must not be read as confirmed")
	}
}

// installFakeDocker puts a docker on PATH that logs its arguments and runs body. No container exists.
func installFakeDocker(t *testing.T, body string) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "docker.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\n" + body
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestSandboxRunnerRefusesUnconfirmedBounds(t *testing.T) {
	installFakeDocker(t, "case \"$1\" in\nrun) echo \"tsp-sandbox-bounds: the memory limit is 'max', want 4294967296\" >&2; exit 126 ;;\nrm) exit 0 ;;\nesac\n")
	_, err := sandboxRunner("example/go:1", admit.SandboxReadOnly)(context.Background(), t.TempDir(), "printf one")
	var refusal evidence.Refusal
	if !errors.As(err, &refusal) || refusal.Reason != evidence.ReasonSandboxBoundsUnverified {
		t.Fatalf("run = %v, want a %s refusal", err, evidence.ReasonSandboxBoundsUnverified)
	}
	if !strings.Contains(refusal.Detail, "memory limit") {
		t.Fatalf("detail = %q, want the control the sandbox could not establish", refusal.Detail)
	}
}

// A container that exits zero without the preflight's confirmation never proved its bounds: an image whose
// entrypoint ignores the script would otherwise read as a sandboxed pass.
func TestSandboxRunnerRefusesARunThatNeverConfirmedItsBounds(t *testing.T) {
	installFakeDocker(t, "case \"$1\" in\nrun) echo ok ;;\nrm) exit 0 ;;\nesac\n")
	_, err := sandboxRunner("example/go:1", admit.SandboxReadOnly)(context.Background(), t.TempDir(), "printf one")
	var refusal evidence.Refusal
	if !errors.As(err, &refusal) || refusal.Reason != evidence.ReasonSandboxBoundsUnverified {
		t.Fatalf("run = %v, want a %s refusal", err, evidence.ReasonSandboxBoundsUnverified)
	}
}

func TestSandboxRunnerRefusesOutputAboveTheBound(t *testing.T) {
	logPath := installFakeDocker(t, "case \"$1\" in\nrun) i=0; while [ $i -lt 20 ]; do printf '0123456789'; i=$((i+1)); done ;;\nrm) exit 0 ;;\nesac\n")
	bounds := sandboxDefaults
	bounds.OutputBytes = 64
	_, err := runSandboxWithin(context.Background(), t.TempDir(), "example/go:1", admit.SandboxReadOnly, "printf one", bounds, sandboxContainerPaths, "docker")
	var refusal evidence.Refusal
	if !errors.As(err, &refusal) || refusal.Reason != evidence.ReasonSandboxOutputLimit {
		t.Fatalf("run = %v, want a %s refusal", err, evidence.ReasonSandboxOutputLimit)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(logged), "rm -f") {
		t.Fatalf("the run left its container behind:\n%s", logged)
	}
}

// A docker CLI killed by the deadline does not stop the container it started. The runner has to name the
// container and remove it, or a timed-out row keeps consuming the machine after the row is over.
func TestSandboxRunnerStopsContainerAfterDeadline(t *testing.T) {
	logPath := installFakeDocker(t, "case \"$1\" in\nrun) sleep 5 ;;\nrm) exit 0 ;;\nesac\n")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := runSandboxWithin(ctx, t.TempDir(), "example/go:1", admit.SandboxReadOnly, "printf one", sandboxDefaults, sandboxContainerPaths, "docker")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run = %v, want the deadline to stay recognisable", err)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	name := ""
	for _, line := range lines {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "--name" && i+1 < len(fields) {
				name = fields[i+1]
			}
		}
	}
	if name == "" {
		t.Fatalf("the docker run was not named:\n%s", logged)
	}
	if !strings.Contains(string(logged), "rm -f "+name) {
		t.Fatalf("the container %q was not removed after the deadline:\n%s", name, logged)
	}
}

func TestSandboxContainerNameIsUniqueAndDockerSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		name := sandboxContainerName()
		if seen[name] {
			t.Fatalf("container name %q repeated", name)
		}
		seen[name] = true
		if strings.ContainsAny(name, " /:@") || name == "" {
			t.Fatalf("container name %q is not safe for docker", name)
		}
	}
}

// The wrapper reports the values it observed, so the pin compatibility check can assert the line is stripped
// before the row's output is digested.
func TestSandboxConfirmationCarriesTheObservedNumbers(t *testing.T) {
	f := newSandboxPreflightFixture(t)
	script := sandboxPreflightScript(sandboxDefaults, f.paths, true)
	out, code := runSandboxPreflight(t, script)
	if code != 0 {
		t.Fatalf("preflight = %d\n%s", code, out)
	}
	for _, want := range []string{
		"memory=4294967296",
		"pids=1024",
		"cpu=200000/100000",
		"tmp=1048576",
		"work=2097152",
		"shm=65536",
		"nofile=1024",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, out)
		}
	}
}

var _ = strconv.Itoa
