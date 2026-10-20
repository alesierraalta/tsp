package bench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SourceFile marks a case shipped by reference: instead of carrying fixture/ and fix/, it names the upstream
// repository and commit whose tree is the fixture, and the patch that turns it into fix/all. Real-bug cases
// are whole projects; vendoring them would copy third-party code into this repository.
const SourceFile = "SOURCE.json"

const sourceDeletionsFile = "source-deletions.json"

// Source is the content of SOURCE.json.
type Source struct {
	Repo   string `json:"repo"`   // anything git fetch accepts: a URL or, in tests, a local path
	Commit string `json:"commit"` // the full commit whose tree is the fixture
	Patch  string `json:"patch"`  // the fix, relative to the case directory, applied with -p1
}

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// DefaultCaseCache is where materialized cases and fetched sources live between runs.
func DefaultCaseCache() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "tsp", "bench-cases")
	}
	return filepath.Join(os.TempDir(), "tsp-bench-cases")
}

// MaterializeCase returns the directory a run reads the case from. A vendored case is used in place. A case
// shipped by reference is built once under cache, keyed by its source and patch, and its key is refreshed on
// every call so an edited request or defect needs no new fetch.
func MaterializeCase(caseDir, cache string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(caseDir, SourceFile))
	if os.IsNotExist(err) {
		return caseDir, nil
	}
	if err != nil {
		return "", err
	}
	var src Source
	if err := json.Unmarshal(raw, &src); err != nil {
		return "", fmt.Errorf("%s: %w", SourceFile, err)
	}
	if src.Repo == "" || !fullCommit.MatchString(src.Commit) || src.Patch == "" {
		return "", fmt.Errorf("%s needs repo, a full 40-hex commit and patch", SourceFile)
	}
	patch, err := os.ReadFile(filepath.Join(caseDir, filepath.Clean(src.Patch)))
	if err != nil {
		return "", err
	}
	key, err := os.ReadFile(filepath.Join(caseDir, KeyFile))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(append([]byte("source-layout-v2\x00"), append(raw, 0)...), patch...))
	dir := filepath.Join(cache, filepath.Base(caseDir)+"-"+hex.EncodeToString(sum[:])[:12])
	if _, err := os.Stat(filepath.Join(dir, FixDir, "all")); err != nil {
		if err := buildSourceCase(src, patch, cache, dir); err != nil {
			return "", fmt.Errorf("%s: %w", filepath.Base(caseDir), err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, sourceDeletionsFile)); err != nil {
		return "", fmt.Errorf("source cache deletion manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, KeyFile), key, 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

// buildSourceCase writes fixture/ and fix/all/ into a scratch directory and renames it into place, so an
// interrupted build never leaves a half case the next run would take as complete.
func buildSourceCase(src Source, patch []byte, cache, dir string) error {
	repo, err := fetchCommit(src, cache)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(cache, ".build-")
	if err != nil {
		return err
	}
	defer removeTree(tmp)
	fixture, fixed := filepath.Join(tmp, FixtureDir), filepath.Join(tmp, FixDir, "all")
	for _, d := range []string{fixture, fixed} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
		if err := extractRev(repo, src.Commit, d); err != nil {
			return fmt.Errorf("extract %s: %w", src.Commit, err)
		}
	}
	// The ceiling keeps git apply from adopting a repository the cache happens to sit in: it patches the
	// directory as plain files.
	apply := exec.Command("git", "apply", "-p1", "--whitespace=nowarn", "-")
	apply.Dir = fixed
	apply.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(fixed))
	apply.Stdin = strings.NewReader(string(patch))
	if out, err := apply.CombinedOutput(); err != nil {
		return fmt.Errorf("%s does not apply to %s: %s", src.Patch, src.Commit, strings.TrimSpace(string(out)))
	}
	deleted := []string{}
	if err := filepath.WalkDir(fixture, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(fixture, path)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(filepath.Join(fixed, rel)); os.IsNotExist(err) {
			deleted = append(deleted, rel)
		} else if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(tmp, sourceDeletionsFile), deleted); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

// fetchCommit returns a bare repository under cache that holds src.Commit, fetching only that commit.
func fetchCommit(src Source, cache string) (string, error) {
	name := strings.Trim(regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(src.Repo, "_"), "_")
	repo := filepath.Join(cache, "repos", name+".git")
	if _, err := gitRun(cache, "--git-dir="+repo, "cat-file", "-e", src.Commit+"^{commit}"); err == nil {
		return repo, nil
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return "", err
	}
	if out, err := gitRun(cache, "init", "-q", "--bare", repo); err != nil {
		return "", fmt.Errorf("git init: %s", strings.TrimSpace(out))
	}
	// A whole project over the network outlasts gitRun's minute; the fetch gets its own bound.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "--git-dir="+repo, "fetch", "-q", "--depth=1", src.Repo, src.Commit).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fetch %s %s: %s", src.Repo, src.Commit, strings.TrimSpace(string(out)))
	}
	return repo, nil
}

// logOf is the run's log, or a sink when the caller gave none.
func logOf(opts Options) io.Writer {
	if opts.Log == nil {
		return io.Discard
	}
	return opts.Log
}
