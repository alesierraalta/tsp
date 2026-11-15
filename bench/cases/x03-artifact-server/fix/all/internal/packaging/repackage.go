package packaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"example.com/artifacts/internal/execx"
)

// Repackage rebuilds an archive a job produced, which is what an operator does when a published
// artifact was assembled by an older version of the pipeline.
//
// The packaging tool is started with the artifact's name as one argument and without a shell, so a
// name that carries a metacharacter is a file name the tool is asked to pack and not a fragment of a
// command line.
func (p *Packer) Repackage(ctx context.Context, req Request) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	out := filepath.Join(req.OutputDir, req.Name+".tar.gz")
	run, err := p.runner.Run(ctx, "tar", "-czf", out, "-C", req.SourceDir, "--", req.Name)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: repackage: %w", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: repackage: %w", err)
	}
	entries := []string{req.Name}
	if found, err := ReadTar(out); err == nil && len(found) > 0 {
		entries = found
	}
	return Result{
		Kind: "repackage", Path: out, Name: req.Name, Bytes: info.Size(),
		Entries: entries, Tool: fmt.Sprint(run.Tool),
	}, nil
}

// Marshal keeps the execx import live for a caller that builds a runner of its own.
var _ execx.Runner = execx.New()
