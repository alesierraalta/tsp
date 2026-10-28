package bench

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alesierraalta/tsp/internal/state"
)

// SnapshotEnv reads TSP's private snapshot storage with the repository's own objects as fallback, so a round based
// on a commit can still read that commit. It never makes a snapshot depend on the repository: Snapshot writes every
// byte it names with SnapshotWriteEnv first.
func SnapshotEnv(top string) ([]string, error) {
	_, objects, e := snapshotStore(top)
	if e != nil {
		return nil, e
	}
	original, e := snapshotGit(top, nil, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if e != nil {
		return nil, e
	}
	return []string{"GIT_OBJECT_DIRECTORY=" + objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES=" + strconv.Quote(original)}, nil
}

// SnapshotWriteEnv names TSP's private objects alone: writing with it copies bytes in even when the repository
// already holds an identical object, so no snapshot can be pruned by the repository's own garbage collection.
func SnapshotWriteEnv(top string) ([]string, error) {
	_, objects, e := snapshotStore(top)
	if e != nil {
		return nil, e
	}
	return []string{"GIT_OBJECT_DIRECTORY=" + objects}, nil
}

// snapshotStore is the private object directory of the repository at top, created and verified as TSP's own.
func snapshotStore(top string) (string, string, error) {
	common, e := snapshotGit(top, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return "", "", e
	}
	root, e := state.Root()
	if e != nil {
		return "", "", e
	}
	sum := sha256.Sum256([]byte(filepath.Clean(common)))
	objects := filepath.Join(root, "snapshots", fmt.Sprintf("%x", sum[:]), "objects")
	if e = os.MkdirAll(objects, 0700); e != nil {
		return "", "", e
	}
	private := filepath.Dir(objects)
	for _, p := range []string{private, objects} {
		info, e := os.Lstat(p)
		if e != nil {
			return "", "", e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("snapshot store is not a private directory: %s", p)
		}
		if e = os.Chmod(p, 0700); e != nil {
			return "", "", e
		}
	}
	return private, objects, nil
}

func snapshotGit(top string, env []string, args ...string) (string, error) {
	b, e := snapshotRun(top, env, nil, args...)
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(b), nil
}

func snapshotRun(top string, env []string, stdin io.Reader, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", top}, args...)...)
	c.Env = append(os.Environ(), env...)
	c.Stdin = stdin
	var stderr strings.Builder
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return string(b), nil
}

// Snapshot captures every non-ignored working input as raw bytes in private storage and answers the tree that names
// them. Bytes are read directly and written by TSP, so no index flag, clean filter, text normalization or
// repository object decides what a snapshot holds. An input that cannot be captured fails closed.
func Snapshot(top string) (string, error) {
	inputs, e := workingInputs(top)
	if e != nil {
		return "", e
	}
	write, e := SnapshotWriteEnv(top)
	if e != nil {
		return "", e
	}
	index, e := os.CreateTemp("", "tsp-snapshot-index-")
	if e != nil {
		return "", e
	}
	name := index.Name()
	index.Close()
	// Git starts its own index; an empty file is not one.
	if e = os.Remove(name); e != nil {
		return "", e
	}
	defer os.Remove(name)
	env := append(append([]string{}, write...), "GIT_INDEX_FILE="+name)
	for _, input := range inputs {
		oid, mode, skipped, e := writeInput(top, write, input)
		if e != nil {
			return "", e
		}
		if skipped {
			continue
		}
		if _, e = snapshotGit(top, env, "update-index", "--add", "--replace", "--cacheinfo", mode+","+oid+","+input); e != nil {
			return "", e
		}
	}
	return snapshotGit(top, env, "write-tree")
}

// workingInputs names the files a test can read: what Git versions or would version, with ignored paths left out.
// A submodule is refused, because its parent records a commit and never the state of its working tree.
func workingInputs(top string) ([]string, error) {
	out, e := snapshotRun(top, nil, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if e != nil {
		return nil, e
	}
	var inputs []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			inputs = append(inputs, path)
		}
	}
	staged, e := snapshotRun(top, nil, nil, "ls-files", "-s", "-z")
	if e != nil {
		return nil, e
	}
	for _, entry := range strings.Split(staged, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		if !ok || !strings.HasPrefix(meta, "160000 ") {
			continue
		}
		return nil, fmt.Errorf("the working tree holds a submodule at %s: the TSP loop cannot capture what a submodule's working tree changes, so commit it there or keep it out of the loop", path)
	}
	return withoutState(inputs, top)
}

// withoutState drops TSP's own state directory when it sits inside the worktree: it is the harness's bookkeeping, not
// an input a test reads, and capturing it would make every snapshot differ from the last.
func withoutState(inputs []string, top string) ([]string, error) {
	root, e := state.Root()
	if e != nil {
		return nil, e
	}
	rel, e := filepath.Rel(filepath.Clean(top), filepath.Clean(root))
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return inputs, nil
	}
	skip := filepath.ToSlash(rel)
	kept := make([]string, 0, len(inputs))
	for _, path := range inputs {
		if path == skip || strings.HasPrefix(path, skip+"/") {
			continue
		}
		kept = append(kept, path)
	}
	return kept, nil
}

// writeInput copies one input's own bytes into private storage and names its mode. A file that is gone from the
// worktree is not an input any more; a file that cannot be read fails the snapshot.
func writeInput(top string, env []string, path string) (string, string, bool, error) {
	full := filepath.Join(top, filepath.FromSlash(path))
	info, e := os.Lstat(full)
	if errors.Is(e, os.ErrNotExist) {
		return "", "", true, nil
	}
	if e != nil {
		return "", "", false, fmt.Errorf("cannot capture %s: %w", path, e)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, e := os.Readlink(full)
		if e != nil {
			return "", "", false, fmt.Errorf("cannot capture %s: %w", path, e)
		}
		oid, e := hashObject(top, env, []byte(target))
		return oid, "120000", false, e
	case info.IsDir():
		return "", "", false, fmt.Errorf("cannot capture %s: it is a directory, not a file tests read", path)
	case !info.Mode().IsRegular():
		return "", "", false, fmt.Errorf("cannot capture %s: it is not a regular file", path)
	}
	data, e := os.ReadFile(full)
	if e != nil {
		return "", "", false, fmt.Errorf("cannot capture %s: %w", path, e)
	}
	oid, e := hashObject(top, env, data)
	if e != nil {
		return "", "", false, e
	}
	if info.Mode()&0111 != 0 {
		return oid, "100755", false, nil
	}
	return oid, "100644", false, nil
}

func hashObject(top string, env []string, data []byte) (string, error) {
	out, e := snapshotRun(top, env, bytes.NewReader(data), "hash-object", "-w", "--stdin", "--no-filters")
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(out), nil
}

// SnapshotAlive reports whether a snapshot still holds every object it names, reading private storage alone: the
// repository's own store is not consulted, so a snapshot that only borrowed a repository blob reads as lost.
func SnapshotAlive(top, tree string) error {
	write, e := SnapshotWriteEnv(top)
	if e != nil {
		return e
	}
	if _, e = snapshotGit(top, write, "cat-file", "-e", tree+"^{tree}"); e != nil {
		return e
	}
	out, e := snapshotRun(top, write, nil, "ls-tree", "-r", "-z", "--full-tree", tree)
	if e != nil {
		return e
	}
	for _, entry := range strings.Split(out, "\x00") {
		meta, _, ok := strings.Cut(entry, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 3 {
			continue
		}
		if _, e = snapshotGit(top, write, "cat-file", "-e", fields[2]); e != nil {
			return fmt.Errorf("snapshot object %s is gone: %w", fields[2], e)
		}
	}
	return nil
}
