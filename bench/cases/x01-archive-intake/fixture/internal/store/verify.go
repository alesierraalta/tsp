package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"example.com/archive/internal/checksum"
)

// VerifyReport is what one integrity pass found. A deployment runs it from the operator console after a
// disk problem; the report names what is wrong rather than failing at the first broken object.
type VerifyReport struct {
	Records  int      `json:"records"`
	Blobs    int      `json:"blobs"`
	Missing  []string `json:"missing,omitempty"`
	Corrupt  []string `json:"corrupt,omitempty"`
	Orphans  []string `json:"orphans,omitempty"`
	Bytes    int64    `json:"bytes"`
	Problems int      `json:"problems"`
}

// OK reports whether the pass found nothing wrong.
func (r VerifyReport) OK() bool { return r.Problems == 0 }

// Verify walks the metadata and the blobs and compares them: every record must have its blob, every blob
// must hold the bytes its name says, and every blob must be carried by some record. The record set is read
// under the lock; the files are read outside it, so a long pass does not stop commits.
func (s *Store) Verify() (VerifyReport, error) {
	records := s.All()
	report := VerifyReport{Records: len(records)}
	carried := map[string]bool{}
	for _, record := range records {
		report.Bytes += record.Bytes
		if record.Digest == "" {
			report.Corrupt = append(report.Corrupt, record.ID)
			continue
		}
		carried[record.Digest] = true
		blob, err := s.blobPath(record.Digest)
		if err != nil {
			report.Missing = append(report.Missing, record.ID+":"+checksum.Short(record.Digest))
			continue
		}
		body, err := os.ReadFile(blob)
		if err != nil {
			report.Missing = append(report.Missing, record.ID+":"+checksum.Short(record.Digest))
			continue
		}
		if checksum.Sum(body) != record.Digest {
			report.Corrupt = append(report.Corrupt, record.ID)
		}
	}
	blobDir, err := s.path("blobs")
	if err != nil {
		return report, err
	}
	err = filepath.Walk(blobDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		report.Blobs++
		name := filepath.Base(path)
		if !carried[name] {
			key, keyErr := s.KeyOfBlob(path)
			if keyErr != nil {
				return keyErr
			}
			report.Orphans = append(report.Orphans, key)
		}
		return nil
	})
	if err != nil {
		return report, fmt.Errorf("store: verify blobs: %w", err)
	}
	report.Problems = len(report.Missing) + len(report.Corrupt) + len(report.Orphans)
	sort.Strings(report.Missing)
	sort.Strings(report.Corrupt)
	sort.Strings(report.Orphans)
	return report, nil
}

// KeyOfBlob renders a blob path as the key the report prints: the part under the objects directory.
func (s *Store) KeyOfBlob(path string) (string, error) {
	base, err := s.path("blobs")
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", fmt.Errorf("store: key of %s: %w", path, err)
	}
	return filepath.ToSlash(rel), nil
}

// Describe renders the report as the lines the console prints: one line per problem kind, then the total.
func (r VerifyReport) Describe() []string {
	lines := []string{
		fmt.Sprintf("%d records, %d blobs, %d bytes", r.Records, r.Blobs, r.Bytes),
	}
	for _, problem := range []struct {
		label string
		names []string
	}{
		{"missing", r.Missing},
		{"corrupt", r.Corrupt},
		{"orphaned", r.Orphans},
	} {
		if len(problem.names) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d %s: %s", len(problem.names), problem.label, strings.Join(problem.names, ", ")))
	}
	if r.OK() {
		lines = append(lines, "no problems")
	}
	return lines
}
