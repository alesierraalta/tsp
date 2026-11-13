// Package packaging turns a staged directory into the archive a job publishes. Three kinds are built
// here: a tar and a zip in this process, and a compressed bundle through the packaging tool. Every kind
// is handed the artifact name as one argument of a process or one header of an archive, so a name is
// always a name and never a fragment of a command line.
package packaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/artifacts/internal/execx"
)

// Errors a pack reports.
var (
	// ErrKind is returned for a kind this deployment does not build.
	ErrKind = errors.New("packaging: unknown kind")
	// ErrRequest is returned for a request that cannot describe a pack.
	ErrRequest = errors.New("packaging: invalid request")
	// ErrName is returned for an artifact name that is not one a pack may carry.
	ErrName = errors.New("packaging: invalid artifact name")
)

// Kinds are the archives a pack builds.
var Kinds = []string{"tar", "zip", "bundle", "repackage"}

// Request is one pack.
type Request struct {
	// Kind is one of Kinds.
	Kind string
	// SourceDir is the directory the artifact sits in.
	SourceDir string
	// OutputDir is the directory the archive is written to. It must already exist.
	OutputDir string
	// Name is the artifact's name inside its source directory.
	Name string
}

// Validate checks the request before anything is read or written.
func (r Request) Validate() error {
	if !knownKind(r.Kind) {
		return fmt.Errorf("%w: %q", ErrKind, r.Kind)
	}
	if !filepath.IsAbs(r.SourceDir) || !filepath.IsAbs(r.OutputDir) {
		return fmt.Errorf("%w: %q and %q are not absolute directories", ErrRequest, r.SourceDir, r.OutputDir)
	}
	if filepath.Clean(r.SourceDir) == "/" || filepath.Clean(r.OutputDir) == "/" {
		return fmt.Errorf("%w: neither directory may be the filesystem root", ErrRequest)
	}
	if err := execx.CheckName(r.Name); err != nil {
		return fmt.Errorf("%w: %v", ErrName, err)
	}
	if strings.Contains(r.Name, string(filepath.Separator)) {
		return fmt.Errorf("%w: %q carries a directory separator", ErrName, r.Name)
	}
	return nil
}

// knownKind reports whether a kind is one this package builds.
func knownKind(kind string) bool {
	for _, candidate := range Kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// Result is what a pack produced.
type Result struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Bytes   int64    `json:"bytes"`
	Entries []string `json:"entries"`
	Tool    string   `json:"tool,omitempty"`
}

// Validate checks the result a pack reported.
func (r Result) Validate() error {
	if !knownKind(r.Kind) {
		return fmt.Errorf("%w: %q", ErrKind, r.Kind)
	}
	if r.Path == "" {
		return fmt.Errorf("%w: the pack reports no path", ErrRequest)
	}
	if r.Bytes < 0 {
		return fmt.Errorf("%w: the pack reports %d bytes", ErrRequest, r.Bytes)
	}
	return nil
}

// Packer builds the archives. The runner is a value so a test can assert the argv a bundle produced
// without starting a process.
type Packer struct {
	runner execx.Runner
	// MaxBytes bounds the artifact a pack reads, so a job cannot publish a file the deployment cannot
	// carry.
	MaxBytes int64
}

// NewPacker returns a packer over a runner.
func NewPacker(runner execx.Runner) *Packer {
	if runner == nil {
		runner = execx.New()
	}
	return &Packer{runner: runner, MaxBytes: 512 << 20}
}

// Pack builds the archive the request names.
func (p *Packer) Pack(ctx context.Context, req Request) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(filepath.Join(req.SourceDir, req.Name))
	if err != nil {
		return Result{}, fmt.Errorf("packaging: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("%w: %q is not a regular file", ErrRequest, req.Name)
	}
	if p.MaxBytes > 0 && info.Size() > p.MaxBytes {
		return Result{}, fmt.Errorf("%w: %q is %d bytes", ErrRequest, req.Name, info.Size())
	}
	switch req.Kind {
	case "tar":
		return p.buildTar(req, info.Size())
	case "zip":
		return p.buildZip(req, info.Size())
	case "bundle":
		return p.buildBundle(ctx, req)
	case "repackage":
		return p.Repackage(ctx, req)
	}
	return Result{}, fmt.Errorf("%w: %q", ErrKind, req.Kind)
}

// buildBundle compresses the artifact with the packaging tool. The tool receives the name as one
// argument and runs without a shell, which is what keeps a name a name.
func (p *Packer) buildBundle(ctx context.Context, req Request) (Result, error) {
	out := filepath.Join(req.OutputDir, req.Name+".tar.gz")
	run, err := p.runner.Run(ctx, "tar", "-czf", out, "-C", req.SourceDir, "--", req.Name)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: bundle: %w", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: bundle: %w", err)
	}
	return Result{
		Kind: "bundle", Path: out, Name: req.Name, Bytes: info.Size(),
		Entries: []string{req.Name}, Tool: strings.TrimSpace(run.Tool),
	}, nil
}

// outputPath returns the archive path of a pack, and refuses a directory that does not exist.
func outputPath(dir, name string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("packaging: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", ErrRequest, dir)
	}
	return filepath.Join(dir, name), nil
}
