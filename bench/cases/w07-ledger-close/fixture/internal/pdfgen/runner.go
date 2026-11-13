// Package pdfgen runs the deployment's PDF renderer. It is the only place the service starts a
// process, and the process is always the binary the configuration named, with an argument vector
// this package builds.
package pdfgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Errors the runner reports.
var (
	ErrNotConfigured = errors.New("pdfgen: no renderer configured")
	ErrBinary        = errors.New("pdfgen: unusable renderer")
	ErrArguments     = errors.New("pdfgen: unusable path")
	ErrFailed        = errors.New("pdfgen: renderer failed")
	ErrOutput        = errors.New("pdfgen: renderer output is not a PDF")
)

// The bounds one conversion has.
const (
	MaxStderrBytes = 8 << 10
	MaxInputBytes  = 4 << 20
	MaxOutputBytes = 32 << 20
)

// Runner renders one HTML file into one PDF file.
type Runner struct {
	binary string
	env    []string
}

// New returns a runner over an absolute binary path. The binary is checked once, here: an absolute
// path, a regular file, executable. A path that fails a check is refused at construction, so the
// close either has a renderer it can run or knows it has none.
func New(binary string) (*Runner, error) {
	if strings.TrimSpace(binary) == "" {
		return nil, ErrNotConfigured
	}
	if !filepath.IsAbs(binary) {
		return nil, fmt.Errorf("%w: %q is not absolute", ErrBinary, binary)
	}
	info, err := os.Stat(binary)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBinary, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("%w: %q is not an executable file", ErrBinary, binary)
	}
	// The renderer runs with a fixed environment: whatever the service holds is not a renderer's
	// business, and a secret in the environment would otherwise be inherited by a subprocess.
	return &Runner{binary: binary, env: []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}}, nil
}

// Args is the argument vector for one conversion. The paths are absolute and come from the
// workspace, which has already proved they are inside the root. Each is passed after its flag
// rather than as a bare argument, and a path whose last segment begins with "-" is refused anyway:
// a renderer that parses its values for flags would otherwise read a file name as an option.
func Args(source, target string) ([]string, error) {
	for _, path := range []string{source, target} {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("%w: %q is not absolute", ErrArguments, path)
		}
		if strings.HasPrefix(filepath.Base(path), "-") {
			return nil, fmt.Errorf("%w: %q starts with a dash", ErrArguments, path)
		}
		if strings.ContainsRune(path, 0) {
			return nil, fmt.Errorf("%w: %q carries a NUL byte", ErrArguments, path)
		}
	}
	return []string{"--input", source, "--output", target}, nil
}

// Render converts source into target. The command is started without a shell, with the fixed
// environment, and its standard error is kept up to MaxStderrBytes and no further. The input is
// checked for size and regularity first; the output is checked for size, regularity and the PDF
// signature afterwards.
func (r *Runner) Render(ctx context.Context, source, target string) error {
	args, err := Args(source, target)
	if err != nil {
		return err
	}
	input, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArguments, err)
	}
	if !input.Mode().IsRegular() || input.Size() > MaxInputBytes {
		return fmt.Errorf("%w: %q is not a statement of at most %d bytes", ErrArguments, source, MaxInputBytes)
	}
	if output, err := os.Lstat(target); err == nil && output.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %q is a symbolic link", ErrArguments, target)
	}
	stderr := &cappedBuffer{limit: MaxStderrBytes}
	command := exec.CommandContext(ctx, r.binary, args...)
	command.Env = append([]string(nil), r.env...)
	command.Stderr = stderr
	command.Stdout = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%w: %v: %s", ErrFailed, err, stderr.String())
	}
	rendered, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFailed, err)
	}
	if !rendered.Mode().IsRegular() || rendered.Size() == 0 || rendered.Size() > MaxOutputBytes {
		return fmt.Errorf("%w: %d bytes at %q", ErrOutput, rendered.Size(), target)
	}
	header := make([]byte, 5)
	file, err := os.Open(target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFailed, err)
	}
	defer file.Close()
	if _, err := file.Read(header); err != nil {
		return fmt.Errorf("%w: %v", ErrFailed, err)
	}
	if !bytes.Equal(header, []byte("%PDF-")) {
		return fmt.Errorf("%w: %q does not start with a PDF signature", ErrOutput, target)
	}
	return nil
}

// cappedBuffer keeps the first limit bytes written to it and discards the rest, counting what it
// dropped. It is a bytes.Buffer with a bound, so a child process cannot grow this process's memory.
type cappedBuffer struct {
	limit   int
	kept    bytes.Buffer
	dropped int64
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	room := b.limit - b.kept.Len()
	if room > len(p) {
		room = len(p)
	}
	if room > 0 {
		b.kept.Write(p[:room])
	}
	if dropped := len(p) - room; dropped > 0 {
		b.dropped += int64(dropped)
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	text := strings.TrimSpace(b.kept.String())
	if b.dropped > 0 {
		text += fmt.Sprintf(" (+%d bytes dropped)", b.dropped)
	}
	return text
}
