package importer

import (
	"fmt"
	"os"
	"path/filepath"

	"example.com/relay/internal/archive"
)

// StageAttachment writes one attachment under the staging directory and returns the path it wrote.
//
// The name of an entry arrives from the sender of the stream, so the path is decided by the
// archive rule rather than by a join of the sender's text.
func StageAttachment(dir string, entry archive.Entry) (string, error) {
	path, err := archive.Write(dir, entry)
	if err != nil {
		return "", err
	}
	return path, nil
}

// StageManifest writes one manifest under the staging directory and returns the path it wrote.
func StageManifest(dir string, entry archive.Entry) (string, error) {
	path, err := archive.Write(dir, entry)
	if err != nil {
		return "", err
	}
	return path, nil
}

// StageThumbnail writes one thumbnail under the staging directory and returns the path it wrote.
//
// Thumbnails arrive keyed by the entry name the sender wrote, so the name goes through the same
// archive rule as every other entry: a name that walks out of the staging directory is refused.
func StageThumbnail(dir string, entry archive.Entry) (string, error) {
	path, err := archive.Write(dir, entry)
	if err != nil {
		return "", err
	}
	return path, nil
}

// Staged lists the files a staging directory carries, as paths relative to it.
func Staged(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		key, err := archive.Key(dir, path)
		if err != nil {
			return err
		}
		out = append(out, key)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("importer: list %q: %w", dir, err)
	}
	return out, nil
}
