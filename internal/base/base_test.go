package base

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// cacheDir points os.UserCacheDir at a per-test directory so records never
// touch the developer's real cache.
func cacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	return filepath.Join(dir, "rdd-plus", "bases")
}

func TestSaveThenLoadReturnsTheRecord(t *testing.T) {
	cacheDir(t)
	root := t.TempDir()
	sha := strings.Repeat("a", 40)
	if err := Save(root, Record{Ref: "origin/dev", SHA: sha, At: "2026-10-11T00:00:00Z"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := Load(root)
	if !ok {
		t.Fatal("Load found no record after Save")
	}
	if got.Ref != "origin/dev" || got.SHA != sha {
		t.Fatalf("Load = %+v, want ref origin/dev sha %s", got, sha)
	}
}

func TestLoadWithoutARecordReportsNone(t *testing.T) {
	cacheDir(t)
	if _, ok := Load(t.TempDir()); ok {
		t.Fatal("Load reported a record for a repo that never saved one")
	}
}

func TestLoadToleratesAMalformedRecord(t *testing.T) {
	dir := cacheDir(t)
	root := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, ok := recordPathFor(root)
	if !ok {
		t.Fatal("no record path")
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(root); ok {
		t.Fatal("Load reported a record from malformed bytes; drift detection must fail open")
	}
}

// fakeGit answers the two questions Capture asks git: the upstream tracking
// ref's name and that ref's commit.
func fakeGit(t *testing.T, root, ref, sha string) func(string, ...string) (string, error) {
	t.Helper()
	return func(dir string, args ...string) (string, error) {
		if dir != root {
			t.Fatalf("git ran in %q, want the repo root %q", dir, root)
		}
		switch {
		case slices.Equal(args[:min(len(args), 3)], []string{"rev-parse", "--abbrev-ref", "--symbolic-full-name"}):
			return ref + "\n", nil
		case len(args) == 2 && args[0] == "rev-parse" && args[1] == ref:
			return sha + "\n", nil
		}
		t.Fatalf("unexpected git call %v", args)
		return "", nil
	}
}

func TestCaptureAnswersTheUpstreamRefAndItsCommit(t *testing.T) {
	const root = "/repo"
	sha := strings.Repeat("b", 40)
	rec, ok := Capture(fakeGit(t, root, "origin/dev", sha), root)
	if !ok {
		t.Fatal("Capture reported no record though git answered both questions")
	}
	if rec.Ref != "origin/dev" || rec.SHA != sha {
		t.Fatalf("Capture = %+v, want ref origin/dev sha %s", rec, sha)
	}
	if _, err := time.Parse(time.RFC3339, rec.At); err != nil {
		t.Fatalf("At %q is not RFC3339: %v", rec.At, err)
	}
}

func TestCaptureHasNoRecordWithoutAnUpstream(t *testing.T) {
	git := func(string, ...string) (string, error) {
		return "", errors.New("fatal: no upstream configured")
	}
	if _, ok := Capture(git, "/repo"); ok {
		t.Fatal("Capture reported a record without an upstream; sessions on a detached HEAD must stay silent")
	}
}

func TestCaptureHasNoRecordWhenTheUpstreamCommitIsUnresolvable(t *testing.T) {
	git := func(dir string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "rev-parse" && len(args) == 2 {
			return "", errors.New("fatal: ambiguous argument")
		}
		return "origin/dev\n", nil
	}
	if _, ok := Capture(git, "/repo"); ok {
		t.Fatal("Capture reported a record whose commit git could not resolve")
	}
}

func TestResolveReturnsTheCurrentCommitOfTheRef(t *testing.T) {
	sha := strings.Repeat("c", 40)
	git := func(dir string, args ...string) (string, error) {
		if len(args) == 2 && args[0] == "rev-parse" && args[1] == "origin/dev" {
			return sha + "\n", nil
		}
		t.Fatalf("unexpected git call %v", args)
		return "", nil
	}
	got, ok := Resolve(git, "/repo", "origin/dev")
	if !ok || got != sha {
		t.Fatalf("Resolve = (%q, %v), want (%q, true)", got, ok, sha)
	}
}

func TestResolveHasNoAnswerWhenTheRefIsGone(t *testing.T) {
	git := func(string, ...string) (string, error) {
		return "", errors.New("fatal: ambiguous argument 'origin/dev'")
	}
	if _, ok := Resolve(git, "/repo", "origin/dev"); ok {
		t.Fatal("Resolve answered for an unresolvable ref; a drift warning must name both commits or stay silent")
	}
}
