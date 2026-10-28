package bench

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alesierraalta/tsp/internal/state"
)

// SnapshotEnv isolates snapshot objects from the repository. The repository is read-only fallback storage;
// snapshot writes go only beneath TSP's private state directory, never into .git/objects.
func SnapshotEnv(top string) ([]string, error) {
	common, e := snapshotGit(top, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return nil, e
	}
	root, e := state.Root()
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256([]byte(filepath.Clean(common)))
	objects := filepath.Join(root, "snapshots", fmt.Sprintf("%x", sum[:]), "objects")
	if e = os.MkdirAll(objects, 0700); e != nil {
		return nil, e
	}
	private := filepath.Dir(objects)
	for _, p := range []string{private, objects} {
		info, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("snapshot store is not a private directory: %s", p)
		}
		if e = os.Chmod(p, 0700); e != nil {
			return nil, e
		}
	}
	original, e := snapshotGit(top, nil, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if e != nil {
		return nil, e
	}
	return []string{"GIT_OBJECT_DIRECTORY=" + objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES=" + strconv.Quote(original)}, nil
}

func snapshotGit(top string, env []string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", top}, args...)...)
	c.Env = append(os.Environ(), env...)
	var stderr strings.Builder
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(b)), nil
}

// Snapshot captures all non-ignored working inputs in private storage, using a temporary index.
// An unreadable input fails closed instead of silently omitting data a test might consume.
func Snapshot(top string) (string, error) {
	index, e := snapshotGit(top, nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp("", "tsp-snapshot-")
	if e != nil {
		return "", e
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)
	info, e := os.Stat(index)
	switch {
	case e == nil:
		// The copy keeps the index's own mtime: git trusts a file's stat only when the file is older than its
		// index, so a copy stamped now would read an edit made in the second the index was written as no change.
		var data []byte
		if data, e = os.ReadFile(index); e == nil {
			if e = os.WriteFile(name, data, 0600); e == nil {
				e = os.Chtimes(name, info.ModTime(), info.ModTime())
			}
		}
	case errors.Is(e, os.ErrNotExist):
		// No index yet: git starts one of its own, but not from an empty file.
		e = os.Remove(name)
	}
	if e != nil {
		return "", e
	}
	env, e := SnapshotEnv(top)
	if e != nil {
		return "", e
	}
	env = append(env, "GIT_INDEX_FILE="+name)
	if _, e = snapshotGit(top, env, "--literal-pathspecs", "add", "-A", "--", "."); e != nil {
		return "", e
	}
	return snapshotGit(top, env, "write-tree")
}
