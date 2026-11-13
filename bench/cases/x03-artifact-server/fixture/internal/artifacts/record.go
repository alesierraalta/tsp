// Package artifacts is the artifact server's record set: one record per artifact a job produced, one
// per job, and the events a project's subscriptions are told about. The bodies live in the tree under
// the root; a record names the digest.
package artifacts

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Errors a record reports.
var (
	// ErrInvalid is returned for a record that fails its own validation.
	ErrInvalid = errors.New("artifacts: invalid record")
	// ErrNotFound is returned for a record the project does not carry.
	ErrNotFound = errors.New("artifacts: no such record")
	// ErrDuplicate is returned when a record would replace one with the same identifier.
	ErrDuplicate = errors.New("artifacts: the record already exists")
)

// Status is where an artifact is in its life.
type Status string

// The states an artifact can be in.
const (
	// Staged is an artifact a job uploaded and has not packed.
	Staged Status = "staged"
	// Packed is an artifact whose archive was built.
	Packed Status = "packed"
	// Published is an artifact a project released.
	Published Status = "published"
	// Expired is an artifact the retention sweep removed.
	Expired Status = "expired"
)

// Valid reports whether the status is one this service knows.
func (s Status) Valid() bool {
	switch s {
	case Staged, Packed, Published, Expired:
		return true
	}
	return false
}

// nameRe is the shape an artifact name has: a file name of letters, digits and punctuation, with no
// path separator and no traversal. A name may carry punctuation such as a semicolon or a space, because
// a file a pipeline produced is allowed to be called anything a filesystem accepts.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._+,;:=>@()!#%&-]{0,127}$`)

// ValidName reports whether a name is one an artifact may carry.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Record is one artifact.
type Record struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Job       string    `json:"job"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Digest    string    `json:"digest"`
	Bytes     int64     `json:"bytes"`
	Status    Status    `json:"status"`
	Archive   string    `json:"archive,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	Published time.Time `json:"published,omitempty"`
}

// Validate checks the record. The identifier, the project and the job are values the service minted or
// read from a stored job; the name is the one value a sender writes, and it is checked as a name.
func (r Record) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: no id", ErrInvalid)
	}
	if strings.TrimSpace(r.Project) == "" {
		return fmt.Errorf("%w: %q names no project", ErrInvalid, r.ID)
	}
	if strings.TrimSpace(r.Job) == "" {
		return fmt.Errorf("%w: %q names no job", ErrInvalid, r.ID)
	}
	if !ValidName(r.Name) {
		return fmt.Errorf("%w: %q carries the name %q", ErrInvalid, r.ID, r.Name)
	}
	if strings.Contains(r.Name, "..") {
		return fmt.Errorf("%w: %q carries a traversal in %q", ErrInvalid, r.ID, r.Name)
	}
	if len(r.Digest) != 64 {
		return fmt.Errorf("%w: %q carries digest %q", ErrInvalid, r.ID, r.Digest)
	}
	if r.Bytes < 0 {
		return fmt.Errorf("%w: %q is %d bytes", ErrInvalid, r.ID, r.Bytes)
	}
	if !r.Status.Valid() {
		return fmt.Errorf("%w: %q is %q", ErrInvalid, r.ID, r.Status)
	}
	if r.Created.IsZero() || r.Created.Location() != time.UTC {
		return fmt.Errorf("%w: %q needs a UTC instant", ErrInvalid, r.ID)
	}
	return nil
}

// InProject reports whether the record belongs to a project.
func (r Record) InProject(project string) bool { return r.Project == project }

// Staged reports whether the artifact is waiting for its pack.
func (r Record) IsStaged() bool { return r.Status == Staged }

// JobStatus is where a job is in its life.
type JobStatus string

// The states a job can be in.
const (
	// JobQueued is a job nobody has started.
	JobQueued JobStatus = "queued"
	// JobRunning is a job a runner is working on.
	JobRunning JobStatus = "running"
	// JobSucceeded is a job whose pack finished.
	JobSucceeded JobStatus = "succeeded"
	// JobFailed is a job that stopped with an error.
	JobFailed JobStatus = "failed"
)

// Valid reports whether the job status is one this service knows.
func (s JobStatus) Valid() bool {
	switch s {
	case JobQueued, JobRunning, JobSucceeded, JobFailed:
		return true
	}
	return false
}

// Job is one run of the pipeline: a project's upload, the pack of what it staged and the events its
// subscribers hear about.
type Job struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Kind      string    `json:"kind"`
	Status    JobStatus `json:"status"`
	Artifacts []string  `json:"artifacts"`
	Attempts  int       `json:"attempts"`
	Error     string    `json:"error,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	Finished  time.Time `json:"finished,omitempty"`
}

// Validate checks the job.
func (j Job) Validate() error {
	if strings.TrimSpace(j.ID) == "" {
		return fmt.Errorf("%w: no id", ErrInvalid)
	}
	if strings.TrimSpace(j.Project) == "" {
		return fmt.Errorf("%w: %q names no project", ErrInvalid, j.ID)
	}
	if strings.TrimSpace(j.Kind) == "" {
		return fmt.Errorf("%w: %q names no kind", ErrInvalid, j.ID)
	}
	if !j.Status.Valid() {
		return fmt.Errorf("%w: job %q is %q", ErrInvalid, j.ID, j.Status)
	}
	if j.Attempts < 0 {
		return fmt.Errorf("%w: job %q ran %d times", ErrInvalid, j.ID, j.Attempts)
	}
	if j.Created.IsZero() || j.Created.Location() != time.UTC {
		return fmt.Errorf("%w: job %q needs a UTC instant", ErrInvalid, j.ID)
	}
	if j.Status == JobSucceeded && j.Error != "" {
		return fmt.Errorf("%w: job %q succeeded carrying %q", ErrInvalid, j.ID, j.Error)
	}
	return nil
}

// Done reports whether a job reached a terminal state.
func (j Job) Done() bool { return j.Status == JobSucceeded || j.Status == JobFailed }

// String renders the job for a log line and the console page.
func (j Job) String() string {
	return fmt.Sprintf("%s %s %s (%d artifacts)", j.ID, j.Project, j.Status, len(j.Artifacts))
}
