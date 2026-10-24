// Package scan turns the findings a real scanner produced into TSP findings, and never invents one.
//
// The scanners stay external: this package owns the scope (the files the change touched), the
// normalized shape (tool, rule, file, line, level, message), the severity gate, and the refusal to
// report an empty result when nothing was actually scanned. Structured results arrive as SARIF, the
// format semgrep, gitleaks, trivy and CodeQL all emit, so a scanner needs no adapter to be useful.
package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/gitdiff"
)

// ErrUnusable marks a scan that measured nothing: no engine installed, or an unreadable report. A
// clean scan and an empty one are different answers and only one of them means the code is clean.
var ErrUnusable = errors.New("scan unusable")

// Severity levels SARIF uses, in the order this package gates on.
var severityRank = map[string]int{"note": 1, "warning": 2, "error": 3}

// Finding is one scanner result, normalized.
type Finding struct {
	Tool    string `json:"tool"`
	Rule    string `json:"rule"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Report is what one scan measured.
type Report struct {
	Engine   string        `json:"engine"`
	Scanned  []string      `json:"scanned,omitempty"`
	Findings []Finding     `json:"findings,omitempty"`
	Elapsed  time.Duration `json:"elapsed"`
}

// Blocking reports whether a finding reaches the named level, so a repository can gate on errors while
// still reading the notes. A level this package does not know blocks nothing: guessing that "critical"
// means "error" is how a gate quietly changes meaning, so the caller validates the word instead.
func (r Report) Blocking(level string) bool {
	threshold, known := severityRank[strings.ToLower(strings.TrimSpace(level))]
	if !known {
		return false
	}
	for _, finding := range r.Findings {
		if severityRank[finding.Level] >= threshold {
			return true
		}
	}
	return false
}

// Deps are the process boundaries, injected so the decision is testable without a repository.
type Deps struct {
	Git      func(dir string, args ...string) (string, error)
	LookPath func(name string) (string, error)
	Run      func(ctx context.Context, dir, name string, args ...string) (string, error)
}

// Options are one scan.
type Options struct {
	Cwd    string
	Base   string        // commit-ish the change is measured from; empty reads the working tree
	Sarif  string        // read this report instead of running an engine
	Level  string        // gate at this level, default error
	Budget time.Duration // wall clock for the engine
	Deps   Deps
}

// Run scans the change: the report the caller named, or the engine installed for this stack.
func Run(ctx context.Context, opts Options) (Report, error) {
	if strings.TrimSpace(opts.Sarif) != "" {
		changed, err := changedAt(opts.Cwd, opts.Base, opts.Deps)
		if err != nil {
			return Report{}, err
		}
		return ReadSarif(opts.Sarif, changed)
	}

	d := opts.Deps
	if d.Git == nil {
		d.Git = gitdiff.Default
	}
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	if d.Run == nil {
		d.Run = runAt
	}

	root, err := gitdiff.Root(opts.Cwd, d.Git)
	if err != nil {
		return Report{}, fmt.Errorf("%w: not a git repository: nothing to scan", ErrUnusable)
	}
	changed, err := changedAt(root, opts.Base, d)
	if err != nil {
		return Report{}, err
	}
	if len(changed) == 0 {
		return Report{Engine: "gitleaks", Scanned: changed}, nil
	}

	engine, err := d.LookPath("gitleaks")
	if err != nil {
		return Report{}, fmt.Errorf("%w: no scanner is installed, so nothing was scanned: install gitleaks (https://github.com/gitleaks/gitleaks) or pass --sarif with a report another tool wrote", ErrUnusable)
	}

	work, err := os.MkdirTemp("", "tsp-scan-")
	if err != nil {
		return Report{}, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	defer os.RemoveAll(work)
	report := filepath.Join(work, "gitleaks.sarif")

	started := time.Now()
	if _, err := d.Run(ctx, root, engine, "detect", "--no-git", "--redact", "--report-format", "sarif", "--report-path", report); err != nil {
		// gitleaks exits 1 when it finds a leak; that is a result, not a failure.
		if _, statErr := os.Stat(report); statErr != nil {
			return Report{}, fmt.Errorf("%w: gitleaks could not scan: %v", ErrUnusable, err)
		}
	}
	out, err := ReadSarif(report, changed)
	if err != nil {
		return Report{}, err
	}
	out.Engine = "gitleaks"
	out.Elapsed = time.Since(started)
	sort.Slice(out.Findings, func(i, j int) bool {
		if out.Findings[i].File != out.Findings[j].File {
			return out.Findings[i].File < out.Findings[j].File
		}
		return out.Findings[i].Line < out.Findings[j].Line
	})
	return out, nil
}

// ReadSarif normalizes a report: every result becomes a finding, and only the files in scope survive.
// A nil scope means the caller did not narrow the report, so everything it says counts; an empty
// non-nil scope means the change touched nothing, so nothing survives. The two are different answers
// and conflating them would report an untouched file as part of the change.
func ReadSarif(path string, scope []string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	var doc sarif
	if err := json.Unmarshal(data, &doc); err != nil {
		return Report{}, fmt.Errorf("%w: the report is not SARIF: %v", ErrUnusable, err)
	}
	scoped := scope != nil
	report := Report{Scanned: scope}
	inScope := map[string]bool{}
	for _, p := range scope {
		inScope[filepath.ToSlash(strings.TrimSpace(p))] = true
	}
	for _, run := range doc.Runs {
		tool := run.Tool.Driver.Name
		if report.Engine == "" {
			report.Engine = tool
		}
		for _, result := range run.Results {
			file, line := result.location()
			file = normalizePath(file)
			if scoped && !inScope[file] {
				continue
			}
			level := strings.ToLower(strings.TrimSpace(result.Level))
			if level == "" {
				level = "warning"
			}
			report.Findings = append(report.Findings, Finding{
				Tool:    tool,
				Rule:    result.RuleID,
				File:    file,
				Line:    line,
				Level:   level,
				Message: strings.TrimSpace(result.Message.Text),
			})
		}
	}
	return report, nil
}

// sarif is the part of the format this package understands.
type sarif struct {
	Runs []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool struct {
		Driver struct {
			Name string `json:"name"`
		} `json:"driver"`
	} `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// location is the first physical location of a result: the file and line a person can open.
func (r sarifResult) location() (string, int) {
	if len(r.Locations) == 0 {
		return "", 0
	}
	physical := r.Locations[0].PhysicalLocation
	return physical.ArtifactLocation.URI, physical.Region.StartLine
}

// normalizePath turns a report's URI into a repository-relative path.
func normalizePath(uri string) string {
	uri = strings.TrimSpace(uri)
	uri = strings.TrimPrefix(uri, "file://")
	uri = filepath.ToSlash(uri)
	if abs, err := filepath.Abs(uri); err == nil {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, abs); err == nil && !strings.HasPrefix(rel, "..") {
				uri = filepath.ToSlash(rel)
			}
		}
	}
	return strings.TrimPrefix(uri, "./")
}

// changedAt is the candidate's paths, read the one way every command reads them.
func changedAt(dir, base string, d Deps) ([]string, error) {
	paths, err := gitdiff.Changed(dir, base, d.Git)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	return paths, nil
}

func runAt(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
