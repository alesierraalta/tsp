package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const tmpProbe = "/tmp/tsp-private-tmp-probe"

// A suite that creates a fixed /tmp path exclusively passes every time it runs, because each run gets its own
// /tmp: two suites that bind the same socket path under concurrency no longer collide, and nothing a suite
// leaves in /tmp reaches the next one or the operator.
func TestSuitesRunWithTheirOwnTmp(t *testing.T) {
	if !privateTmpAvailable() {
		t.Skip("this kernel does not allow an unprivileged mount namespace")
	}
	os.Remove(tmpProbe)
	t.Cleanup(func() { os.Remove(tmpProbe) })
	dir, err := os.MkdirTemp(stageRoot(), "privtmp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	suite := "sh -c 'set -C; : > " + tmpProbe + "'"
	for i := 0; i < 2; i++ {
		res, err := RunSuite(dir, suite, time.Minute)
		if err != nil || !res.Green {
			t.Fatalf("RunSuite #%d: green=%v err=%v output=%q", i+1, res.Green, err, res.Output)
		}
		if code, out := runCapture(dir, []string{"sh", "-c", "set -C; : > " + tmpProbe}, time.Minute); code != 0 {
			t.Fatalf("runCapture #%d: exit %d: %q", i+1, code, out)
		}
	}
	if _, err := os.Stat(tmpProbe); !os.IsNotExist(err) {
		t.Fatalf("a suite's /tmp file leaked to the host: %v", err)
	}
}

// The private /tmp must not hand the suite any privilege: a suite that checks a permission error still gets
// one, exactly as it does on the host.
func TestASuiteWithItsOwnTmpKeepsTheOperatorsPermissions(t *testing.T) {
	if !privateTmpAvailable() {
		t.Skip("this kernel does not allow an unprivileged mount namespace")
	}
	dir, err := os.MkdirTemp(stageRoot(), "privperm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "locked"), 0o755); os.RemoveAll(dir) })
	suite := "sh -c 'mkdir locked && chmod 000 locked && ! touch locked/f 2>/dev/null'"
	res, err := RunSuite(dir, suite, time.Minute)
	if err != nil || !res.Green {
		t.Fatalf("a write into a mode-000 directory must be denied: green=%v err=%v output=%q", res.Green, err, res.Output)
	}
}

// A suite whose own directory sits under /tmp keeps the host's /tmp: a fresh /tmp would hide the directory
// it runs in, so it would not even find its own files.
func TestASuiteUnderTmpStillSeesItsOwnDirectory(t *testing.T) {
	dir := t.TempDir()
	if !strings.HasPrefix(dir, "/tmp/") {
		t.Skip("the test temp dir is not under /tmp here")
	}
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := RunSuite(dir, "sh -c 'test -f "+filepath.Join(dir, "marker")+"'", time.Minute)
	if err != nil || !res.Green {
		t.Fatalf("green=%v err=%v output=%q", res.Green, err, res.Output)
	}
}

// An operator whose TMPDIR sits under /tmp still gets a working temp dir inside the private /tmp: the fresh
// tmpfs hides that directory, and go cannot even create its work dir there ("stat /tmp/x: no such file").
func TestASuiteWithItsOwnTmpGetsAUsableTempDirWhenTMPDIRIsUnderTmp(t *testing.T) {
	if !privateTmpAvailable() {
		t.Skip("this kernel does not allow an unprivileged mount namespace")
	}
	hostTmp, err := os.MkdirTemp("/tmp", "tsp-tmpdir-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(hostTmp) })
	t.Setenv("TMPDIR", hostTmp)
	dir, err := os.MkdirTemp(stageRoot(), "privtmpdir-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	res, err := RunSuite(dir, `sh -c 'd=$(mktemp -d) && test -d "$d"'`, time.Minute)
	if err != nil || !res.Green {
		t.Fatalf("mktemp -d with TMPDIR=%s: green=%v err=%v output=%q", hostTmp, res.Green, err, res.Output)
	}
}

// A TMPDIR outside /tmp stays visible in the private /tmp, so the suite keeps using the operator's choice.
func TestASuiteWithItsOwnTmpKeepsATMPDIROutsideTmp(t *testing.T) {
	if !privateTmpAvailable() {
		t.Skip("this kernel does not allow an unprivileged mount namespace")
	}
	dir, err := os.MkdirTemp(stageRoot(), "privkeep-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMPDIR", dir)
	res, err := RunSuite(dir, `sh -c 'test "$TMPDIR" = "`+dir+`"'`, time.Minute)
	if err != nil || !res.Green {
		t.Fatalf("TMPDIR outside /tmp must reach the suite unchanged: green=%v err=%v output=%q", res.Green, err, res.Output)
	}
}

// The catch check stages fixture + fix + agent tests outside /tmp, so that check is the one that gets a
// private /tmp: it is where a collision would silently flip a catch.
func TestStageBuildsOutsideTmp(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := stage(fixture, "", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if underTmp(dir) {
		t.Fatalf("stage dir %s is under /tmp", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("fixture not copied: %v", err)
	}
}
