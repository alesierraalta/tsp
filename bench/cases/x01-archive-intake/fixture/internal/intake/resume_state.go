package intake

import (
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
	"fmt"
)

// ResumeState is what the admin page prints about a batch that is not committed: which entries the
// store carries and which the tree still holds.
type ResumeState struct {
	Batch     string
	Tenant    string
	Entries   []ResumeEntry
	Committed bool
	Used      int64
}

// ResumeEntry is one entry's resume state.
type ResumeEntry struct {
	Name      string
	Kind      string
	Pages     int
	Staged    bool
	Committed bool
	Object    string
}

// State reports the resume state of one batch without running the pipeline.
func (r *Runner) State(batch string) (ResumeState, error) {
	layout, err := r.staging.Layout(batch)
	if err != nil {
		return ResumeState{}, err
	}
	if !layout.Exists() {
		return ResumeState{}, fmt.Errorf("%w: batch %s", staging.ErrMissing, batch)
	}
	m, err := layout.ReadManifest()
	if err != nil {
		return ResumeState{}, err
	}
	committed := r.Committed(batch)
	state := ResumeState{Batch: batch, Tenant: m.Tenant}
	if used, err := layout.Used(); err == nil {
		state.Used = used
	}
	if _, done := layout.CommittedAt(); done {
		state.Committed = true
	}
	for _, e := range m.Entries {
		entry := ResumeEntry{Name: e.Name, Kind: e.KindOf(), Staged: layout.EntryExists(e)}
		if record, ok := committed[e.Name]; ok {
			entry.Committed = true
			entry.Object = record.ID
			entry.Pages = record.Pages
		}
		state.Entries = append(state.Entries, entry)
	}
	return state, nil
}

// RecordsOf is the store records of a batch, for the report page that renders a finished import.
func (r *Runner) RecordsOf(batch string) []store.Record { return r.store.UnderBatch(batch) }
