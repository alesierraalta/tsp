// Package base remembers which upstream tracking ref and commit a plan's
// evidence was admitted against, so the stop gate can warn when that base
// moved before the run's report was written.
package base

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Record is the upstream tracking ref and the commit a plan's evidence was
// admitted against.
type Record struct {
	Ref string `json:"ref"` // e.g. origin/dev
	SHA string `json:"sha"` // full commit id the evidence was admitted on
	At  string `json:"at"`  // RFC3339 admission time, informational
}

// gitFunc answers one git invocation in dir; it matches gate.Deps.Git so the
// same helpers serve the hook and the CLI.
type gitFunc func(dir string, args ...string) (string, error)

// recordPathFor keys each repository by a hash of its root so sibling clones
// never read each other's base. The cache root is kept as rdd-plus so binaries
// from before the rename share the same state (same reasoning as plan-lock's
// lockRoot); it is not os.TempDir because TMPDIR differs per process.
func recordPathFor(root string) (string, bool) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(base, "rdd-plus", "bases", hex.EncodeToString(sum[:])[:16]+".json"), true
}

// Save writes root's record atomically.
func Save(root string, rec Record) error {
	path, ok := recordPathFor(root)
	if !ok {
		return os.ErrInvalid
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "base-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Load reads root's record; an unreadable or missing record reads as none so
// drift detection fails open and never blocks a gate.
func Load(root string) (Record, bool) {
	path, ok := recordPathFor(root)
	if !ok {
		return Record{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, false
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Record{}, false
	}
	if rec.Ref == "" || rec.SHA == "" {
		return Record{}, false
	}
	return rec, true
}

// Capture resolves the upstream tracking ref of root into a record. It has no
// record when the session runs on a detached HEAD or when git cannot name the
// upstream's commit; either way there is no base to remember.
func Capture(git gitFunc, root string) (Record, bool) {
	out, err := git(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return Record{}, false
	}
	ref := strings.TrimSpace(out)
	if ref == "" {
		return Record{}, false
	}
	sha, ok := Resolve(git, root, ref)
	if !ok {
		return Record{}, false
	}
	return Record{Ref: ref, SHA: sha, At: time.Now().UTC().Format(time.RFC3339)}, true
}

// Resolve returns the current commit of ref, or no answer when git cannot
// resolve it: a drift warning must name both commits or stay silent.
func Resolve(git gitFunc, root, ref string) (string, bool) {
	out, err := git(root, "rev-parse", ref)
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(out)
	if sha == "" {
		return "", false
	}
	return sha, true
}
