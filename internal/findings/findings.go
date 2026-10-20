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
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/bench"
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
	path := filepath.Join(root, "findings", fmt.Sprintf("%x", sum[:8]), url.PathEscape(branch)+".json")
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

var unsupportedRunner = errors.New("supported runners: go test, pytest, node --test")

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
	var out strings.Builder
	for _, name := range rep.Red {
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
		file, fingerprint, promise := locate(top, name, rep.TestFiles)
		f.File, f.Fingerprint, f.Promise, f.Suite, f.Commit, f.Time = file, fingerprint, promise, rep.Suite, commit, time.Now().UTC()
		fmt.Fprintf(&out, "%s recorded %s %s\n", f.ID, f.File, f.Test)
	}
	if e = save(r, path); e != nil {
		return failure(e)
	}
	return out.String(), 0
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
	type run struct {
		tests  map[string]bool
		failed bool
		code   int
	}
	runs := map[string]run{}
	fixed, open, newCount, regressed, need := 0, 0, 0, 0, 0
	cannotJudge := false
	var lines strings.Builder
	for i := range r.Findings {
		f := &r.Findings[i]
		result, ok := runs[f.Suite]
		if !ok {
			tests, code, _, err := bench.PerTest(top, f.Suite, timeout)
			_, whole := tests["suite"]
			result = run{tests: tests, failed: err != nil || whole || len(tests) == 0 || code < 0 || code > 1, code: code}
			runs[f.Suite] = result
		}
		file, fingerprint, _ := locate(top, f.Test, []string{f.File})
		_, fileErr := os.Stat(filepath.Join(top, f.File))
		fileGone := f.File != "" && errors.Is(fileErr, os.ErrNotExist)
		previous := f.Status
		passed, ran := result.tests[f.Test]
		switch {
		case fileGone:
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
		fmt.Fprintf(&lines, "%s %s %s %s", f.ID, f.Status, f.File, f.Test)
		if f.Promise != "" {
			fmt.Fprintf(&lines, " — %s", f.Promise)
		}
		lines.WriteByte('\n')
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
