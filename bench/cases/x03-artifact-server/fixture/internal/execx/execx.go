// Package execx runs the external tools the packaging stage needs. A tool is named from a fixed
// allowlist and every argument is checked before it is handed to the process, which is started without
// a shell: an artifact name is a filename, never a fragment of a command line.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Errors Run reports.
var (
	// ErrTool is returned for a tool that is not on the allowlist.
	ErrTool = errors.New("execx: the tool is not allowed")
	// ErrArg is returned for an argument that is not one a process may carry.
	ErrArg = errors.New("execx: the argument is not allowed")
	// ErrTimeout is returned when a tool runs past its deadline.
	ErrTimeout = errors.New("execx: the tool ran past its deadline")
)

// Result is what a tool run produced.
type Result struct {
	Tool   string
	Args   []string
	Stdout string
	Stderr string
}

// String renders the run for a log line.
func (r Result) String() string {
	return r.Tool + " " + strings.Join(r.Args, " ")
}

// Runner runs the allowlisted tools. It is a value so a test can replace the runner and assert the
// argv a stage produced without starting a process.
type Runner interface {
	Run(ctx context.Context, tool string, args ...string) (Result, error)
}

// Exec is the runner a deployment uses.
type Exec struct {
	// Timeout bounds one run.
	Timeout time.Duration
	// MaxOutput bounds the bytes of stdout and stderr a run keeps.
	MaxOutput int
	// Env is the environment a tool runs with. An empty value runs with the process environment.
	Env []string
	// LookPath resolves a tool, so a missing binary is a refusal before anything starts.
	LookPath func(string) (string, error)
}

// New returns the deployment's runner.
func New() *Exec {
	return &Exec{Timeout: 2 * time.Minute, MaxOutput: 1 << 20, LookPath: exec.LookPath}
}

// Run starts one tool. The tool is checked against the allowlist and each argument is checked before
// the process exists, and no argument is ever interpreted by a shell.
func (e *Exec) Run(ctx context.Context, tool string, args ...string) (Result, error) {
	result := Result{Tool: tool, Args: append([]string(nil), args...)}
	if err := CheckTool(tool); err != nil {
		return result, err
	}
	for _, arg := range args {
		if err := CheckArg(arg); err != nil {
			return result, fmt.Errorf("%w: %v", ErrArg, err)
		}
	}
	if e.LookPath != nil {
		if _, err := e.LookPath(tool); err != nil {
			return result, fmt.Errorf("execx: %q is not installed: %w", tool, err)
		}
	}
	if e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	if len(e.Env) > 0 {
		cmd.Env = e.Env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buffer: &stdout, limit: e.MaxOutput}
	cmd.Stderr = &limitedWriter{buffer: &stderr, limit: e.MaxOutput}
	err := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return result, fmt.Errorf("%w: %s", ErrTimeout, result)
		}
		return result, fmt.Errorf("execx: %s: %w: %s", result, err, strings.TrimSpace(result.Stderr))
	}
	return result, nil
}

// limitedWriter keeps the first limit bytes written to it and drops the rest, so a tool that prints a
// gigabyte cannot exhaust the process's memory.
type limitedWriter struct {
	buffer *bytes.Buffer
	limit  int
}

// Write appends what fits and reports the whole write as written, so the tool never blocks on a full
// pipe.
func (w *limitedWriter) Write(p []byte) (int, error) {
	room := w.limit - w.buffer.Len()
	if room <= 0 {
		return len(p), nil
	}
	if len(p) > room {
		w.buffer.Write(p[:room])
		return len(p), nil
	}
	w.buffer.Write(p)
	return len(p), nil
}
