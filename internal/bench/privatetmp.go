package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// privateTmpPrefix runs the rest of argv in a mount namespace with a fresh tmpfs on /tmp. Suites bind
// sockets and write files at fixed /tmp paths; with several suites running at once the second one fails
// with "address already in use" and the run is wrongly marked invalid, or a catch check flips without any
// error. --map-current-user keeps the suite's uid and --keep-caps lets the shell mount; setpriv then drops
// every capability before the suite starts, because a suite that kept them could write where the operator
// cannot and would fail its own permission tests (gin's TestSaveUploadedFileWithPermissionFailed did).
// A TMPDIR under /tmp is hidden by the fresh tmpfs, so it is pointed at the private /tmp instead.
var privateTmpPrefix = []string{"unshare", "--map-current-user", "--keep-caps", "-m",
	"sh", "-c", `mount -t tmpfs tmpfs /tmp && case "${TMPDIR:-}" in /tmp/*) export TMPDIR=/tmp;; esac && exec setpriv --inh-caps=-all --ambient-caps=-all --bounding-set=-all -- "$@"`, "sh"}

var (
	privateTmpOnce sync.Once
	privateTmpOK   bool
)

// privateTmpAvailable reports, once per process, whether this kernel lets an unprivileged user mount a
// tmpfs in its own namespace. Where it does not, suites run exactly as before.
func privateTmpAvailable() bool {
	privateTmpOnce.Do(func() {
		probe := append(append([]string{}, privateTmpPrefix...), "true")
		privateTmpOK = exec.Command(probe[0], probe[1:]...).Run() == nil
	})
	return privateTmpOK
}

// underTmp reports whether dir lies under /tmp, where a fresh /tmp would hide the directory itself.
func underTmp(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return true
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return abs == "/tmp" || strings.HasPrefix(abs, "/tmp/")
}

// suiteArgv is the command a suite actually runs in dir: wrapped in a private /tmp when the kernel allows
// it and dir is not itself under /tmp, else unchanged.
func suiteArgv(dir string, argv []string) []string {
	if len(argv) == 0 || underTmp(dir) || !privateTmpAvailable() {
		return argv
	}
	return append(append([]string{}, privateTmpPrefix...), argv...)
}

// stageRoot is where catch checks stage their trees: outside /tmp, so the suite they run can get a private
// one. It falls back to the system temp dir when no user cache directory exists.
func stageRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return os.TempDir()
	}
	root := filepath.Join(base, "tsp", "stage")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return os.TempDir()
	}
	return root
}
