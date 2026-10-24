// Package gitdiff answers one question the whole harness asks the same way: which paths did this change
// touch? A commit range when the caller names a base, the working tree otherwise. Every command that
// scopes its work to a change uses this, so "the change" means the same thing everywhere.
package gitdiff

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner is one git invocation, injected so callers can test their decisions without a repository.
type Runner func(dir string, args ...string) (string, error)

// ErrNotRepository marks a directory git does not know: there is no change to scope.
var ErrNotRepository = errors.New("not a git repository")

// Root resolves the repository root, so every later path is relative to the same place.
func Root(dir string, git Runner) (string, error) {
	if git == nil {
		git = Default
	}
	out, err := git(dir, "rev-parse", "--show-toplevel")
	root := strings.TrimSpace(out)
	if err != nil || root == "" {
		return "", ErrNotRepository
	}
	return root, nil
}

// Changed lists the paths the change touched: `<base>...HEAD` when base names a commit, the working
// tree (including untracked files) otherwise.
func Changed(dir, base string, git Runner) ([]string, error) {
	if git == nil {
		git = Default
	}
	if base != "" {
		out, err := git(dir, "diff", "--name-only", "--no-renames", base+"...HEAD")
		if err != nil {
			return nil, fmt.Errorf("git diff failed for base %s", base)
		}
		return splitLines(out), nil
	}
	out, err := git(dir, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("git status failed in %s", dir)
	}
	names := []string{}
	for _, entry := range strings.Split(out, "\x00") {
		if len(entry) < 4 {
			continue
		}
		names = append(names, entry[3:])
	}
	return names, nil
}

// Default runs git the ordinary way.
func Default(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

func splitLines(out string) []string {
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}
