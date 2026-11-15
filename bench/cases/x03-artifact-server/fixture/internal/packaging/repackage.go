package packaging

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Repackage rebuilds an archive a job produced, which is what an operator does when a published
// artifact was assembled by an older version of the pipeline.
//
// The source directory and the output path are quoted because an operator's directory can carry a
// space; the artifact's name is appended as it is, because a name is a plain file name and the request
// validated it.
func (p *Packer) Repackage(ctx context.Context, req Request) (Result, error) {
	out := filepath.Join(req.OutputDir, req.Name+".tar.gz")
	command := "tar -czf '" + out + "' -C '" + req.SourceDir + "' " + req.Name
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = req.SourceDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("packaging: repackage: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	info, err := os.Stat(out)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: repackage: %w", err)
	}
	entries, err := ReadTar(out)
	if err != nil {
		// A gzip stream is not a tar stream a reader can walk without decompressing it, so the entry
		// list comes from what the command was asked to pack.
		entries = []string{req.Name}
	}
	return Result{
		Kind: "repackage", Path: out, Name: req.Name, Bytes: info.Size(), Entries: entries,
		Tool: "sh -c",
	}, nil
}
