// Package gitdiff answers one question the whole harness asks the same way: which paths did this change
// touch? A commit range when the caller names a base, the working tree otherwise. Every command that
// scopes its work to a change uses this, so "the change" means the same thing everywhere.
package gitdiff

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
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

// Range is one run of lines the change occupies on the new side of a file.
type Range struct{ First, Last int }

// WholeFile is the range that means "every line": a new or untracked file.
func WholeFile(ranges []Range) bool {
	return len(ranges) == 1 && ranges[0].First <= 1 && ranges[0].Last >= 1<<30
}

// ChangedRanges reads the lines a change touches, per file. A base is compared with the working tree, so
// a probe run before committing sees the change it is about to review; an untracked file is entirely new.
func ChangedRanges(root, base string, git Runner) (map[string][]Range, error) {
	if git == nil {
		git = Default
	}
	args := []string{"diff", "-U0", "--no-renames"}
	if base != "" {
		args = append(args, base)
	}
	out, err := git(root, args...)
	if err != nil {
		return nil, fmt.Errorf("git diff failed while reading the changed lines")
	}
	ranges := parseHunks(out)
	untracked, err := git(root, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("git status failed while reading the changed lines")
	}
	for _, entry := range strings.Split(untracked, "\x00") {
		if !strings.HasPrefix(entry, "?? ") {
			continue
		}
		ranges[strings.TrimPrefix(entry, "?? ")] = []Range{{First: 1, Last: 1 << 30}}
	}
	return ranges, nil
}

// parseHunks reads the new-side ranges out of a unified diff: the file changes with "+++ b/<path>" and the
// range is the second number of the "@@" header.
func parseHunks(diff string) map[string][]Range {
	ranges := map[string][]Range{}
	current := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			current = path
			if path == "/dev/null" {
				current = ""
			}
		case strings.HasPrefix(line, "@@ ") && current != "":
			if span, ok := hunkRange(line); ok {
				ranges[current] = append(ranges[current], span)
			}
		}
	}
	return ranges
}

// hunkRange reads one "@@ -a,b +c,d @@" header as the range of lines the change occupies on the new side.
func hunkRange(header string) (Range, bool) {
	for _, field := range strings.Fields(header) {
		if !strings.HasPrefix(field, "+") {
			continue
		}
		first, count, ok := plusSpec(field)
		if !ok || count == 0 {
			return Range{}, false
		}
		return Range{First: first, Last: first + count - 1}, true
	}
	return Range{}, false
}

// plusSpec parses the "+c,d" half of a hunk header; a missing count means one line.
func plusSpec(field string) (first, count int, ok bool) {
	parts := strings.SplitN(strings.TrimPrefix(field, "+"), ",", 2)
	first, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	count = 1
	if len(parts) == 2 {
		if parsed, err := strconv.Atoi(parts[1]); err == nil {
			count = parsed
		}
	}
	return first, count, true
}
