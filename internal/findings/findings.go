// Package findings tracks defect witnesses outside the repository, per branch.
package findings

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/filelock"
	"github.com/alesierraalta/tsp/internal/hookcmd"
	"github.com/alesierraalta/tsp/internal/state"
)

type Finding struct {
	ID          string    `json:"id"`
	Test        string    `json:"test"`
	File        string    `json:"file"`
	Suite       string    `json:"suite"`
	Fingerprint string    `json:"fingerprint"`
	Promise     string    `json:"promise"`
	Commit      string    `json:"recorded_commit"`
	Time        time.Time `json:"recorded_at"`
	Status      string    `json:"status,omitempty"`
	Reviewed    bool      `json:"reviewed"`
	WasFixed    bool      `json:"was_fixed"`
	Verdict     *Verdict  `json:"verdict,omitempty"`
}

type Verdict struct {
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
	Fingerprint string `json:"fingerprint"`
	Commit      string `json:"commit"`
}

type registry struct {
	Version    int       `json:"version"`
	Repository string    `json:"repository"`
	Branch     string    `json:"branch"`
	Review     int       `json:"review"`
	NextID     int       `json:"next_id"`
	Findings   []Finding `json:"findings"`
}

func git(dir string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	b, e := c.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func load(dir string) (*registry, string, string, error) {
	top, e := git(dir, "rev-parse", "--show-toplevel")
	if e != nil {
		return nil, "", "", e
	}
	branch, e := git(top, "symbolic-ref", "--short", "HEAD")
	if e != nil {
		return nil, "", "", fmt.Errorf("detached HEAD: findings require a branch")
	}
	common, e := git(top, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return nil, "", "", e
	}
	root, e := state.Root()
	if e != nil {
		return nil, "", "", e
	}
	sum := sha256.Sum256([]byte(filepath.Clean(common)))
	name := url.PathEscape(branch)
	if len(name) > 200 {
		hash := sha256.Sum256([]byte(branch))
		name = name[:175] + "-" + fmt.Sprintf("%x", hash[:12])
	}
	path := filepath.Join(root, "findings", fmt.Sprintf("%x", sum[:8]), name+".json")
	r := &registry{Version: 1, Repository: top, Branch: branch, NextID: 1}
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return r, path, top, nil
	}
	if e != nil {
		return nil, "", "", e
	}
	r = nil
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, "", "", fmt.Errorf("unreadable registry: %w", e)
	}
	if r == nil || r.Version != 1 || r.Branch != branch || r.NextID < 1 {
		return nil, "", "", fmt.Errorf("unreadable registry: invalid version, branch or next id")
	}
	seen := map[string]bool{}
	for _, f := range r.Findings {
		if f.ID == "" || f.Test == "" || seen[f.ID] {
			return nil, "", "", fmt.Errorf("unreadable registry: invalid finding")
		}
		seen[f.ID] = true
	}
	return r, path, top, nil
}
func save(r *registry, path string) error {
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	return state.WriteAtomic(path, append(b, '\n'))
}

func lockRegistry(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := filelock.Lock(file); err != nil {
		file.Close()
		return nil, err
	}
	return func() { filelock.Unlock(file); file.Close() }, nil
}

var unsupportedRunner = errors.New("supported runners: go test, pytest, node --test, jest, vitest run, cargo test")

// ignored reports whether git refuses to version a path. A witness there proves nothing a later review could
// find: bench copies and other generated workspaces disappear, and the run then reports a debt it can never
// settle.
func ignored(dir, rel string) bool {
	if rel == "" {
		return false
	}
	_, e := git(dir, "check-ignore", "-q", "--", rel)
	return e == nil
}

func failure(e error) (string, int) { return "tsp findings: " + e.Error() + "\n", 2 }

// Record records or refreshes precisely the witnesses RedCheck found.
func Record(dir, suite string, timeout time.Duration) (string, int) {
	r, path, top, e := load(dir)
	if e != nil {
		return failure(e)
	}
	if suite != "" && !bench.SupportsPerTest(suite) {
		return failure(unsupportedRunner)
	}
	rep, e := bench.RedCheck(top, suite, timeout)
	if e != nil {
		return failure(e)
	}
	if rep.Suite != "" && !bench.SupportsPerTest(rep.Suite) {
		return failure(unsupportedRunner)
	}
	if len(rep.Red) == 0 {
		return "no defect witnesses recorded\n", 1
	}
	commit, e := git(top, "rev-parse", "HEAD")
	if e != nil {
		return failure(e)
	}
	unlock, e := lockRegistry(path)
	if e != nil {
		return failure(e)
	}
	defer unlock()
	r, _, _, e = load(dir)
	if e != nil {
		return failure(e)
	}
	var out, skipped strings.Builder
	for _, name := range rep.Red {
		file, fingerprint, promise := locate(top, name, rep.TestFiles)
		if ignored(top, file) {
			fmt.Fprintf(&skipped, "skipped %s %s: witness under a git-ignored path\n", file, name)
			continue
		}
		index := -1
		for i := range r.Findings {
			if r.Findings[i].Test == name {
				index = i
				break
			}
		}
		if index < 0 {
			r.Findings = append(r.Findings, Finding{ID: fmt.Sprintf("F%d", r.NextID), Test: name})
			r.NextID++
			index = len(r.Findings) - 1
		}
		f := &r.Findings[index]
		f.File, f.Fingerprint, f.Promise, f.Suite, f.Commit, f.Time = file, fingerprint, promise, rep.Suite, commit, time.Now().UTC()
		fmt.Fprintf(&out, "%s recorded %s %s\n", f.ID, f.File, f.Test)
	}
	// A run with no red witness at all returned above, before locking, exactly as before this change. Reaching
	// here with nothing recorded means every witness it found lives where git cannot version it.
	if out.Len() == 0 {
		return skipped.String() + "nothing recorded: every witness lives under a path git ignores\n", 1
	}
	if e = save(r, path); e != nil {
		return failure(e)
	}
	return skipped.String() + out.String(), 0
}

// SetVerdict records a human decision against the current test fingerprint.
func SetVerdict(dir, id, decision, reason string) (string, int) {
	r, path, top, err := load(dir)
	if err != nil {
		return failure(err)
	}
	if decision != "rejected" && decision != "wontfix" && decision != "clear" {
		return failure(fmt.Errorf("verdict must be rejected, wontfix or clear"))
	}
	if decision != "clear" && strings.TrimSpace(reason) == "" {
		return failure(fmt.Errorf("%s requires a non-empty --reason", decision))
	}
	unlock, err := lockRegistry(path)
	if err != nil {
		return failure(err)
	}
	defer unlock()
	r, _, _, err = load(dir)
	if err != nil {
		return failure(err)
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.ID != id {
			continue
		}
		out := id + " verdict cleared\n"
		f.Verdict = nil
		if decision != "clear" {
			_, fingerprint, _ := locate(top, f.Test, []string{f.File})
			commit, err := git(top, "rev-parse", "HEAD")
			if err != nil {
				return failure(err)
			}
			f.Verdict = &Verdict{Decision: decision, Reason: reason, Fingerprint: fingerprint, Commit: commit}
			out = fmt.Sprintf("%s %s: %s\n", id, decision, reason)
		}
		if err := save(r, path); err != nil {
			return failure(err)
		}
		return out, 0
	}
	return failure(fmt.Errorf("unknown finding %s", id))
}

// Check runs each recorded suite once and saves the computed review.
func Check(dir string, timeout time.Duration) (string, int) {
	r, path, top, e := load(dir)
	if e != nil {
		return failure(e)
	}
	if len(r.Findings) == 0 {
		return fmt.Sprintf("no findings recorded on %s\n", r.Branch), 0
	}
	original := append([]Finding(nil), r.Findings...)
	type run struct {
		tests  map[string]bool
		failed bool
		code   int
	}
	runs := map[string]run{}
	fixed, open, newCount, regressed, need := 0, 0, 0, 0, 0
	cannotJudge := false
	var dropped []Finding // witnesses whose path git ignores: pruned from the registry, never reported as debt
	var lines strings.Builder
	writeLine := func(f Finding, promise, suffix string) {
		fmt.Fprintf(&lines, "%s %s %s %s", f.ID, f.Status, f.File, f.Test)
		if promise != "" {
			fmt.Fprintf(&lines, " — %s", promise)
		}
		fmt.Fprintln(&lines, suffix)
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		if ignored(top, f.File) {
			dropped = append(dropped, *f)
			continue
		}
		file, fingerprint, _ := locate(top, f.Test, []string{f.File})
		suffix := ""
		if f.Verdict != nil {
			if fingerprint == f.Verdict.Fingerprint {
				f.Status, f.Reviewed = f.Verdict.Decision, true
				writeLine(*f, f.Verdict.Reason, "")
				continue
			}
			f.Verdict = nil
			suffix = " (verdict dropped: test changed)"
		}
		result, ok := runs[f.Suite]
		if !ok {
			tests, code, _, err := bench.PerTest(top, f.Suite, timeout)
			_, whole := tests["suite"]
			// Cargo uses 101 for test failures as well as build errors; only a
			// parsed per-test report makes that exit code judgeable.
			argv, _ := hookcmd.ShellWords(f.Suite)
			cargoFailure := code == 101 && len(argv) > 1 && argv[0] == "cargo" && argv[1] == "test"
			result = run{tests: tests, failed: err != nil || whole || len(tests) == 0 || code < 0 || (code > 1 && !cargoFailure), code: code}
			runs[f.Suite] = result
		}
		previous := f.Status
		passed, ran := result.tests[f.Test]
		if file == "" && ran {
			file, fingerprint, _ = relocate(top, *f)
			if file != "" {
				f.File = file
			}
		}
		switch {
		case !ran && !result.failed:
			f.Status = "missing"
			need++
		case result.failed && result.code != 0:
			f.Status = "unknown"
			need++
		case file == "":
			f.Status = "missing"
			need++
		case result.failed:
			f.Status = "unknown"
			need++
		case !ran:
			f.Status = "unknown"
			need++
		case !passed:
			if f.WasFixed {
				f.Status = "regressed"
				regressed++
			} else {
				f.Status = "open"
				if f.Reviewed {
					open++
				} else {
					newCount++
				}
			}
		case fingerprint == "" || f.Fingerprint == "" || fingerprint != f.Fingerprint:
			f.Status = "test-changed"
			need++
		default:
			f.Status = "fixed"
			f.WasFixed = true
			if previous != "fixed" {
				fixed++
			}
		}
		cannotJudge = cannotJudge || f.Status == "unknown"
		f.Reviewed = true
		writeLine(*f, f.Promise, suffix)
	}
	unlock, e := lockRegistry(path)
	if e != nil {
		return failure(e)
	}
	defer unlock()
	latest, _, _, e := load(dir)
	if e != nil {
		return failure(e)
	}
	for i := range latest.Findings {
		for j, computed := range r.Findings {
			if latest.Findings[i].ID == computed.ID && reflect.DeepEqual(latest.Findings[i], original[j]) {
				latest.Findings[i] = computed
				break
			}
		}
	}
	var removed []Finding
	if len(dropped) > 0 {
		kept := latest.Findings[:0]
		for _, f := range latest.Findings {
			gone := false
			for _, want := range dropped {
				if f.ID == want.ID && reflect.DeepEqual(f, want) {
					gone = true
					break
				}
			}
			if gone {
				removed = append(removed, f)
				continue
			}
			kept = append(kept, f)
		}
		latest.Findings = kept
	}
	r = latest
	// Only what the registry really lost is announced, so the report never claims a drop the registry kept.
	for _, f := range removed {
		fmt.Fprintf(&lines, "%s dropped %s %s: witness under a git-ignored path\n", f.ID, f.File, f.Test)
	}
	r.Review++
	report := fmt.Sprintf("review %d on %s: %d fixed, %d open, %d new, %d regressed", r.Review, r.Branch, fixed, open, newCount, regressed)
	if need > 0 {
		report += fmt.Sprintf(", %d need review", need)
	}
	if e = save(r, path); e != nil {
		return failure(e)
	}
	code := 0
	if open+newCount+regressed+need > 0 {
		code = 1
	}
	if cannotJudge {
		code = 2
	}
	return report + "\n" + lines.String(), code
}
