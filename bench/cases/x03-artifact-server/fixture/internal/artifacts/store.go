package artifacts

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"example.com/artifacts/internal/clock"
)

// Store is the in-memory record set, safe for concurrent readers and writers. A store is opened over a
// clock and the deployment's caps: a store with no cap would accept every upload a project sent.
type Store struct {
	mu        sync.RWMutex
	now       clock.Clock
	records   map[string]Record
	jobs      map[string]Job
	maxEvents int
	maxBytes  int64
	events    int
	bytes     int64
}

// Open returns an empty store.
func Open(now clock.Clock, maxBytes int64) (*Store, error) {
	if now == nil {
		return nil, fmt.Errorf("artifacts: a store needs a clock")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("artifacts: the store needs a positive byte cap")
	}
	return &Store{
		now:       now,
		records:   make(map[string]Record),
		jobs:      make(map[string]Job),
		maxBytes:  maxBytes,
		maxEvents: 10000,
	}, nil
}

// SetClock replaces the clock the store stamps records with.
func (s *Store) SetClock(now clock.Clock) {
	if now == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Now returns the store's current instant.
func (s *Store) Now() time.Time {
	s.mu.RLock()
	now := s.now
	s.mu.RUnlock()
	return now().UTC()
}

// MaxBytes returns the byte cap a project's artifacts are held to.
func (s *Store) MaxBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxBytes
}

// Put stores one artifact record.
func (s *Store) Put(record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[record.ID]; exists {
		return fmt.Errorf("%w: artifact %q", ErrDuplicate, record.ID)
	}
	if s.bytes+record.Bytes > s.maxBytes {
		return fmt.Errorf("%w: the deployment stores %d bytes", ErrInvalid, s.maxBytes)
	}
	s.records[record.ID] = record
	s.bytes += record.Bytes
	return nil
}

// Update replaces an artifact record the store carries.
func (s *Store) Update(record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.records[record.ID]
	if !ok {
		return fmt.Errorf("%w: artifact %q", ErrNotFound, record.ID)
	}
	s.bytes += record.Bytes - existing.Bytes
	s.records[record.ID] = record
	return nil
}

// Get returns one artifact record.
func (s *Store) Get(id string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	if !ok {
		return Record{}, fmt.Errorf("%w: artifact %q", ErrNotFound, id)
	}
	return record, nil
}

// InProject returns an artifact only when it belongs to the project given, which is what a request's
// handler resolves a path parameter through.
func (s *Store) InProject(project, id string) (Record, error) {
	record, err := s.Get(id)
	if err != nil {
		return Record{}, err
	}
	if record.Project != project {
		return Record{}, fmt.Errorf("%w: artifact %q in project %q", ErrNotFound, id, project)
	}
	return record, nil
}

// List returns the artifacts of a project in id order.
func (s *Store) List(project string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		if record.Project == project {
			out = append(out, record)
		}
	}
	sortRecords(out)
	return out
}

// All returns every artifact in id order, for the operator console.
func (s *Store) All() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	sortRecords(out)
	return out
}

// OfJob returns the artifacts of one job, in id order.
func (s *Store) OfJob(job string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		if record.Job == job {
			out = append(out, record)
		}
	}
	sortRecords(out)
	return out
}

// Delete removes one artifact record and returns it, so the caller can drop the blob it named.
func (s *Store) Delete(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return Record{}, fmt.Errorf("%w: artifact %q", ErrNotFound, id)
	}
	delete(s.records, id)
	s.bytes -= record.Bytes
	return record, nil
}

// PutJob stores one job.
func (s *Store) PutJob(job Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("%w: job %q", ErrDuplicate, job.ID)
	}
	if len(s.jobs) >= s.maxEvents {
		return fmt.Errorf("%w: the store carries %d jobs", ErrInvalid, s.maxEvents)
	}
	s.jobs[job.ID] = job
	return nil
}

// UpdateJob replaces a job the store carries.
func (s *Store) UpdateJob(job Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("%w: job %q", ErrNotFound, job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

// Job returns one job.
func (s *Store) Job(id string) (Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: job %q", ErrNotFound, id)
	}
	return job, nil
}

// Jobs returns the jobs of a project in id order.
func (s *Store) Jobs(project string) []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		if job.Project == project {
			out = append(out, job)
		}
	}
	sortJobs(out)
	return out
}

// Stats returns the number of records the store carries.
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int{"artifacts": len(s.records), "jobs": len(s.jobs)}
}

// Bytes returns the bytes the store's artifacts account for.
func (s *Store) Bytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytes
}

// sortRecords orders records by id.
func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
}

// sortJobs orders jobs by id.
func sortJobs(jobs []Job) {
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
}
