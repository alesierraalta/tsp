// Package verify runs the blind verification of a candidate change: the writer produces code, tests
// and a report; this package freezes that change outside the live tree, hands a second process an
// input package with no path to the writer's conversation, and accepts only a structured verdict.
// The verifier is deliberately a separate session: the point is that the agent that wrote the tests
// is not the agent that decides whether those tests mean anything.
package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/hookcmd"
)

// ErrUnusable marks every way the verifier can fail to produce a verdict: no session command, a
// candidate that could not be frozen, a session that exited non-zero, a missing or malformed
// verdict. Callers map it to exit 2, never to success, because the absence of a verdict is not
// approval.
var ErrUnusable = errors.New("verifier unusable")

// The three verdicts the session may return. Anything else is a malformed verdict.
const (
	// VerdictPinned means the changed tests fail on the unmodified code: they pin the behaviour
	// the change introduced.
	VerdictPinned = "pinned"
	// VerdictUnpinned means the tests pass on the unmodified code, so they prove nothing. It
	// requires at least one finding, because a rejection without evidence is an opinion.
	VerdictUnpinned = "unpinned"
	// VerdictInsufficient means the session could not judge. It is honest and not a pass.
	VerdictInsufficient = "insufficient"
)

// Names inside the input package and the artifact published in the repository.
const (
	DiffFile     = "00-candidate.diff"
	PlanFile     = "01-plan.json"
	CheckFile    = "02-check.txt"
	RedcheckFile = "03-redcheck.txt"
	TaskFile     = "04-task.md"
	VerdictFile  = "verdict.json"

	// ArtifactName is where the verdict is published, next to the other repository-level markers
	// (.tsp-waiver.json, .no-testing-gate) so a later command can read what the verifier decided.
	ArtifactName = ".tsp-verify.json"
)

// Finding is one problem the verifier can point at. Evidence is required: a finding without it is a
// hypothesis, and the artifact is read by people deciding whether to ship.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Claim    string `json:"claim"`
	Evidence string `json:"evidence"`
}

// Verdict is the only shape a verifier session may return.
type Verdict struct {
	Verdict  string    `json:"verdict"`
	Findings []Finding `json:"findings,omitempty"`
	Notes    string    `json:"notes,omitempty"`
}

// Options configures one verification run.
type Options struct {
	// Cwd is any directory inside the repository under verification.
	Cwd string
	// Base is the commit the change is applied on; defaults to HEAD.
	Base string
	// SessionCmd is the command that runs the verifier session. It receives the package directory
	// as $1 and in TSP_VERIFY_PACKAGE, the frozen tree in TSP_VERIFY_FROZEN, and must write
	// VerdictFile into the package. Empty means the run is unusable.
	SessionCmd string
	// Task is the task text the writer says it was asked to do. It is copied into the package.
	Task string
	// IncludeUntracked lists untracked files to copy into the frozen tree, because a diff cannot
	// carry them.
	IncludeUntracked []string
	// PlanPath optionally names a plan JSON to hand to the verifier.
	PlanPath string
	// CheckText and RedCheckText are the outputs of check and redcheck for this candidate. Empty
	// means they were not run, and the package says so instead of pretending they passed.
	CheckText    string
	RedCheckText string
	// Timeout bounds the session; zero means thirty minutes.
	Timeout time.Duration
	// Keep leaves the frozen tree and package on disk and returns their paths, for a human or a
	// later command that needs to look at exactly what the verifier saw.
	Keep bool
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Result is what a completed verification produced.
type Result struct {
	Verdict Verdict
	// PackageDir and FrozenDir are set always; they are removed unless Options.Keep was set.
	PackageDir string
	FrozenDir  string
	// ArtifactPath is where the verdict was published inside the repository.
	ArtifactPath string
	Base         string
	At           time.Time
	// Kept reports that the frozen tree and package were left on disk for inspection.
	Kept bool
}

// Blocking reports whether the result stops a change from being called verified.
func (r Result) Blocking() bool {
	return r.Verdict.Verdict != VerdictPinned || len(r.Verdict.Findings) > 0
}

// Run freezes the candidate, builds the package, runs the session and validates the verdict. Any
// failure returns ErrUnusable, wrapped with what specifically could not be established.
func Run(ctx context.Context, opts Options) (Result, error) {
	if strings.TrimSpace(opts.SessionCmd) == "" {
		return Result{}, fmt.Errorf("%w: no session command, so nothing can judge the candidate", ErrUnusable)
	}
	base := strings.TrimSpace(opts.Base)
	if base == "" {
		base = "HEAD"
	}
	// applyCandidate reads the base from the same options the caller passed, so the default is
	// written back instead of kept in a local the rest of the run cannot see.
	opts.Base = base
	repo, err := repoRoot(opts.Cwd)
	if err != nil {
		return Result{}, err
	}
	work, err := os.MkdirTemp("", "tsp-verify-")
	if err != nil {
		return Result{}, fmt.Errorf("%w: make work directory: %v", ErrUnusable, err)
	}
	pkg := filepath.Join(work, "package")
	frozen := filepath.Join(work, "frozen")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		os.RemoveAll(work)
		return Result{}, fmt.Errorf("%w: make package directory: %v", ErrUnusable, err)
	}

	keep := opts.Keep
	cleanup := func() {
		if keep {
			return
		}
		makeWritable(frozen)
		_, _ = git(repo, "worktree", "remove", "--force", frozen)
		os.RemoveAll(work)
	}

	if _, err := git(repo, "worktree", "add", "--detach", frozen, base); err != nil {
		cleanup()
		return Result{}, fmt.Errorf("%w: freeze %s: %v", ErrUnusable, base, err)
	}
	if err := applyCandidate(repo, frozen, pkg, opts); err != nil {
		cleanup()
		return Result{}, err
	}
	makeReadOnly(frozen)

	session := sessionCommand(ctx, opts.SessionCmd, pkg, frozen)
	var out strings.Builder
	session.Stdout = &out
	session.Stderr = &out
	if err := session.Run(); err != nil {
		cleanup()
		return Result{}, fmt.Errorf("%w: session command failed: %v: %s", ErrUnusable, err, tail(out.String(), 400))
	}

	verdict, err := readVerdict(filepath.Join(pkg, VerdictFile))
	if err != nil {
		cleanup()
		return Result{}, err
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	result := Result{Verdict: verdict, PackageDir: pkg, FrozenDir: frozen, Base: base, At: now().UTC(), Kept: keep}
	result.ArtifactPath = filepath.Join(repo, ArtifactName)
	if err := publish(result); err != nil {
		cleanup()
		return Result{}, err
	}
	cleanup()
	return result, nil
}

// sessionCommand parses the operator's command and runs it with the package directory appended as
// its last argument. It never hands the string to a shell: ShellWords splits it once, so a value
// from the environment or a file cannot become a second command.
func sessionCommand(ctx context.Context, command, pkg, frozen string) *exec.Cmd {
	words, err := hookcmd.ShellWords(command)
	if err != nil || len(words) == 0 {
		words = []string{command}
	}
	cmd := exec.CommandContext(ctx, words[0], append(words[1:], pkg)...)
	cmd.Env = append(os.Environ(),
		"TSP_VERIFY_PACKAGE="+pkg,
		"TSP_VERIFY_FROZEN="+frozen,
		"TSP_VERIFY_VERDICT="+filepath.Join(pkg, VerdictFile),
	)
	return cmd
}

// applyCandidate writes the diff and the documents the verifier reads, then applies the diff to the
// frozen tree. The package never receives a transcript: no option accepts one.
func applyCandidate(repo, frozen, pkg string, opts Options) error {
	diff, err := git(repo, "diff", "--binary", opts.Base)
	if err != nil {
		return fmt.Errorf("%w: read candidate diff: %v", ErrUnusable, err)
	}
	if err := os.WriteFile(filepath.Join(pkg, DiffFile), []byte(diff), 0o644); err != nil {
		return fmt.Errorf("%w: write candidate diff: %v", ErrUnusable, err)
	}
	if strings.TrimSpace(diff) != "" {
		cmd := exec.Command("git", "-C", frozen, "apply", "--binary", "-")
		cmd.Stdin = strings.NewReader(diff)
		if err2 := cmd.Run(); err2 != nil {
			return fmt.Errorf("%w: apply candidate to frozen tree: %v", ErrUnusable, err2)
		}
	}
	for _, path := range opts.IncludeUntracked {
		if err := copyInto(repo, frozen, path); err != nil {
			return fmt.Errorf("%w: include untracked %s: %v", ErrUnusable, path, err)
		}
	}
	if opts.PlanPath != "" {
		data, err := os.ReadFile(opts.PlanPath)
		if err != nil {
			return fmt.Errorf("%w: read plan: %v", ErrUnusable, err)
		}
		if err := os.WriteFile(filepath.Join(pkg, PlanFile), data, 0o644); err != nil {
			return fmt.Errorf("%w: write plan: %v", ErrUnusable, err)
		}
	}
	if err := os.WriteFile(filepath.Join(pkg, CheckFile), []byte(orNotRun(opts.CheckText, "check")), 0o644); err != nil {
		return fmt.Errorf("%w: write check output: %v", ErrUnusable, err)
	}
	if err := os.WriteFile(filepath.Join(pkg, RedcheckFile), []byte(orNotRun(opts.RedCheckText, "redcheck")), 0o644); err != nil {
		return fmt.Errorf("%w: write redcheck output: %v", ErrUnusable, err)
	}
	task := opts.Task
	if strings.TrimSpace(task) == "" {
		task = "no task text was supplied"
	}
	if err := os.WriteFile(filepath.Join(pkg, TaskFile), []byte(task+"\n"), 0o644); err != nil {
		return fmt.Errorf("%w: write task: %v", ErrUnusable, err)
	}
	return nil
}

func orNotRun(text, name string) string {
	if strings.TrimSpace(text) == "" {
		return name + " was not run for this candidate\n"
	}
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

// readVerdict parses and validates the session's answer. A verdict that contradicts itself is
// malformed: "unpinned" without a finding is a rejection with no evidence, and unknown fields mean
// the session answered a different contract.
func readVerdict(path string) (Verdict, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Verdict{}, fmt.Errorf("%w: the session wrote no %s: %v", ErrUnusable, VerdictFile, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var v Verdict
	if err := dec.Decode(&v); err != nil {
		return Verdict{}, fmt.Errorf("%w: malformed %s: %v", ErrUnusable, VerdictFile, err)
	}
	switch v.Verdict {
	case VerdictPinned, VerdictUnpinned, VerdictInsufficient:
	default:
		return Verdict{}, fmt.Errorf("%w: verdict %q is not one of %s, %s, %s", ErrUnusable, v.Verdict, VerdictPinned, VerdictUnpinned, VerdictInsufficient)
	}
	for i, f := range v.Findings {
		if strings.TrimSpace(f.File) == "" || f.Line <= 0 || strings.TrimSpace(f.Claim) == "" || strings.TrimSpace(f.Evidence) == "" {
			return Verdict{}, fmt.Errorf("%w: finding %d needs a file, a positive line, a claim and evidence", ErrUnusable, i+1)
		}
	}
	if v.Verdict == VerdictUnpinned && len(v.Findings) == 0 {
		return Verdict{}, fmt.Errorf("%w: verdict %q needs at least one finding: a rejection without evidence is an opinion", ErrUnusable, VerdictUnpinned)
	}
	return v, nil
}

// Publish validates the verdict a host left in an existing package and publishes the artifact, so a
// change reviewed by a harness that freezes and judges its own candidate (RDD) needs no second model:
// the host writes the same verdict document, and TSP reads it with the same rules it applies to a
// session it ran itself. An unusable document is refused, never published.
func Publish(packageDir, base string, now time.Time) (Result, error) {
	if strings.TrimSpace(base) == "" {
		return Result{}, fmt.Errorf("%w: --publish needs the base the verdict is about", ErrUnusable)
	}
	abs, err := filepath.Abs(packageDir)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	verdict, err := readVerdict(filepath.Join(abs, VerdictFile))
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Verdict:      verdict,
		PackageDir:   abs,
		ArtifactPath: filepath.Join(abs, ArtifactName),
		Base:         base,
		At:           now,
		Kept:         true,
	}
	if err := publish(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

// publish writes the artifact a later command reads. It carries the verdict and where it came from,
// never the session's raw output, so the artifact cannot be mistaken for an approval transcript.
func publish(result Result) error {
	payload := struct {
		Verdict  string    `json:"verdict"`
		Findings []Finding `json:"findings,omitempty"`
		Notes    string    `json:"notes,omitempty"`
		Base     string    `json:"base"`
		At       string    `json:"at"`
		Blind    bool      `json:"blind"`
	}{
		Verdict:  result.Verdict.Verdict,
		Findings: result.Verdict.Findings,
		Notes:    result.Verdict.Notes,
		Base:     result.Base,
		At:       result.At.Format(time.RFC3339),
		Blind:    true,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode artifact: %v", ErrUnusable, err)
	}
	if err := os.WriteFile(result.ArtifactPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrUnusable, ArtifactName, err)
	}
	return nil
}

// repoRoot resolves the top of the worktree holding dir.
func repoRoot(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%w: %s is not inside a git worktree", ErrUnusable, dir)
	}
	return strings.TrimSpace(out), nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return string(out), fmt.Errorf("%v: %s", err, msg)
		}
	}
	return string(out), err
}

// copyInto copies one repository-relative path into the frozen tree, creating parents.
func copyInto(repo, frozen, rel string) error {
	cleaned := filepath.Clean(rel)
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return fmt.Errorf("%s is not repository-relative", rel)
	}
	src := filepath.Join(repo, cleaned)
	dst := filepath.Join(frozen, cleaned)
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// makeReadOnly removes the write bits so the session cannot fix what it was asked to judge. Windows
// ignores the mode bits, so the call is a no-op there rather than a lie.
func makeReadOnly(root string) {
	walkMode(root, func(path string, d os.DirEntry, info os.FileInfo) {
		if info.Mode()&os.ModeSymlink != 0 {
			return
		}
		if d.IsDir() {
			_ = os.Chmod(path, 0o555)
			return
		}
		_ = os.Chmod(path, 0o444)
	})
}

// makeWritable restores the write bits so the frozen tree can be removed.
func makeWritable(root string) {
	walkMode(root, func(path string, d os.DirEntry, info os.FileInfo) {
		if info.Mode()&os.ModeSymlink != 0 {
			return
		}
		if d.IsDir() {
			_ = os.Chmod(path, 0o755)
			return
		}
		_ = os.Chmod(path, 0o644)
	})
}

func walkMode(root string, fn func(path string, d os.DirEntry, info os.FileInfo)) {
	if runtime.GOOS == "windows" {
		return
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fn(path, d, info)
		return nil
	})
}

func tail(text string, n int) string {
	text = strings.TrimSpace(text)
	if len(text) <= n {
		return text
	}
	return "..." + text[len(text)-n:]
}
