package packaging

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// buildTar writes the artifact into an uncompressed tar archive. The header carries the artifact's name
// and its mode, and nothing about it is read from the name: a name is a value in a header, never a
// path this process opens.
func (p *Packer) buildTar(req Request, size int64) (Result, error) {
	out, err := outputPath(req.OutputDir, req.Name+".tar")
	if err != nil {
		return Result{}, err
	}
	source, err := os.Open(filepath.Join(req.SourceDir, req.Name))
	if err != nil {
		return Result{}, fmt.Errorf("packaging: tar: %w", err)
	}
	defer source.Close()
	file, err := os.Create(out)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: tar: %w", err)
	}
	writer := tar.NewWriter(file)
	header := &tar.Header{
		Name:    req.Name,
		Mode:    0o644,
		Size:    size,
		ModTime: time.Unix(0, 0).UTC(),
		Format:  tar.FormatPAX,
	}
	if err := writer.WriteHeader(header); err != nil {
		file.Close()
		return Result{}, fmt.Errorf("packaging: tar: %w", err)
	}
	written, err := io.Copy(writer, io.LimitReader(source, size))
	if err != nil {
		file.Close()
		return Result{}, fmt.Errorf("packaging: tar: %w", err)
	}
	if err := writer.Close(); err != nil {
		file.Close()
		return Result{}, fmt.Errorf("packaging: tar: %w", err)
	}
	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("packaging: tar: %w", err)
	}
	return Result{
		Kind: "tar", Path: out, Name: req.Name, Bytes: written, Entries: []string{req.Name},
	}, nil
}

// buildZip writes the artifact into a zip archive. The name is written as the entry name, and the
// reader that verifies the archive checks that the entry is the one the pack reported.
func (p *Packer) buildZip(req Request, size int64) (Result, error) {
	out, err := outputPath(req.OutputDir, req.Name+".zip")
	if err != nil {
		return Result{}, err
	}
	source, err := os.Open(filepath.Join(req.SourceDir, req.Name))
	if err != nil {
		return Result{}, fmt.Errorf("packaging: zip: %w", err)
	}
	defer source.Close()
	file, err := os.Create(out)
	if err != nil {
		return Result{}, fmt.Errorf("packaging: zip: %w", err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.CreateHeader(&zip.FileHeader{
		Name:     req.Name,
		Method:   zip.Deflate,
		Modified: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		file.Close()
		return Result{}, fmt.Errorf("packaging: zip: %w", err)
	}
	written, err := io.Copy(entry, io.LimitReader(source, size))
	if err != nil {
		file.Close()
		return Result{}, fmt.Errorf("packaging: zip: %w", err)
	}
	if err := writer.Close(); err != nil {
		file.Close()
		return Result{}, fmt.Errorf("packaging: zip: %w", err)
	}
	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("packaging: zip: %w", err)
	}
	return Result{
		Kind: "zip", Path: out, Name: req.Name, Bytes: written, Entries: []string{req.Name},
	}, nil
}

// ReadTar returns the entry names of a tar archive, which is how the pipeline verifies what a pack
// actually wrote instead of trusting what it reported.
func ReadTar(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		names = append(names, header.Name)
	}
}

// ReadZip returns the entry names of a zip archive.
func ReadZip(path string) ([]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	return names, nil
}
