// Package importer runs the import pipeline: it classifies the entries of a stream and applies
// each one through the staging rule of its kind.
package importer

import (
	"fmt"

	"example.com/relay/internal/archive"
	"example.com/relay/internal/store"
	"example.com/relay/internal/wire"
)

// The directories a stream may carry. An entry under any other directory is refused, so a stream
// cannot put a file somewhere the pipeline has no rule for.
const (
	KindAttachments = "attachments"
	KindThumbnails  = "thumbnails"
	KindRecords     = "records"
)

// Kinds lists the directories a stream may carry.
var Kinds = []string{KindAttachments, KindThumbnails, KindRecords}

// Report is what one import did.
type Report struct {
	Entries int      `json:"entries"`
	Written int      `json:"written"`
	Records int      `json:"records"`
	Paths   []string `json:"paths"`
	IDs     []string `json:"ids"`
}

// Importer applies a stream to the staging directory and the record store.
type Importer struct {
	staging string
	store   *store.Store
	limits  wire.Limits
}

// New returns an importer over a staging directory and a record store.
func New(staging string, st *store.Store) *Importer {
	return &Importer{staging: staging, store: st, limits: wire.DefaultLimits()}
}

// SetLimits replaces the decode limits.
func (im *Importer) SetLimits(limits wire.Limits) {
	if limits != (wire.Limits{}) {
		im.limits = limits
	}
}

// Run applies every entry of a stream. The first entry that cannot be applied stops the run and is
// reported with its name, because a partially applied stream that reported success would leave the
// staging directory in a state nobody described.
func (im *Importer) Run(entries []archive.Entry) (Report, error) {
	report := Report{Entries: len(entries)}
	for _, entry := range entries {
		switch kind := archive.Kind(entry.Name); kind {
		case KindAttachments:
			path, err := StageAttachment(im.staging, entry)
			if err != nil {
				return report, fmt.Errorf("importer: %q: %w", entry.Name, err)
			}
			report.Paths = append(report.Paths, path)
			report.Written++
		case KindThumbnails:
			path, err := StageThumbnail(im.staging, entry)
			if err != nil {
				return report, fmt.Errorf("importer: %q: %w", entry.Name, err)
			}
			report.Paths = append(report.Paths, path)
			report.Written++
		case KindRecords:
			record, err := wire.Decode(entry.Body, im.limits)
			if err != nil {
				return report, fmt.Errorf("importer: %q: %w", entry.Name, err)
			}
			if err := im.store.Upsert(record); err != nil {
				return report, fmt.Errorf("importer: %q: %w", entry.Name, err)
			}
			report.IDs = append(report.IDs, record.ID)
			report.Records++
		default:
			return report, fmt.Errorf("importer: %q is under %q, which is not an import directory", entry.Name, kind)
		}
	}
	return report, nil
}

// Stage is the flat staging write the pipeline uses for a file with no rule of its own.
func Stage(dir string, entry archive.Entry) (string, error) {
	return archive.Write(dir, entry)
}

// Store is the record store the pipeline writes to.
func (im *Importer) Store() *store.Store { return im.store }
