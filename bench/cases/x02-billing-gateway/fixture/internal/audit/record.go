// Package audit is the billing service's trail: one record per mutating request and per refusal, kept
// in a bounded ring in memory and readable only with the audit scope. A record carries the operator's
// reason, which the client never sees.
package audit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/ids"
)

// Errors a record reports.
var (
	// ErrRecord is returned for a record that is not complete.
	ErrRecord = errors.New("audit: incomplete record")
)

// Outcomes a record can carry.
const (
	// OutcomeAccepted is a request the service performed.
	OutcomeAccepted = "accepted"
	// OutcomeRefused is a request the service refused.
	OutcomeRefused = "refused"
	// OutcomeFailed is a request the service attempted and could not finish.
	OutcomeFailed = "failed"
)

// Record is one entry of the trail.
type Record struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Tenant  string    `json:"tenant"`
	Subject string    `json:"subject"`
	// Action is what was asked for: "subscription.seats", "invoice.payment", "report.export".
	Action string `json:"action"`
	// Target is the record the action was aimed at, when there is one.
	Target  string `json:"target,omitempty"`
	Outcome string `json:"outcome"`
	// Detail is the reason an operator needs. It is written to the trail and never to the client.
	Detail string `json:"detail,omitempty"`
}

// Validate checks the record.
func (r Record) Validate() error {
	if !ids.ValidKind(r.ID, ids.Audit) {
		return fmt.Errorf("%w: id %q", ErrRecord, r.ID)
	}
	if strings.TrimSpace(r.Subject) == "" {
		return fmt.Errorf("%w: no subject", ErrRecord)
	}
	if strings.TrimSpace(r.Action) == "" {
		return fmt.Errorf("%w: no action", ErrRecord)
	}
	switch r.Outcome {
	case OutcomeAccepted, OutcomeRefused, OutcomeFailed:
	default:
		return fmt.Errorf("%w: outcome %q", ErrRecord, r.Outcome)
	}
	if r.At.IsZero() || r.At.Location() != time.UTC {
		return fmt.Errorf("%w: instant %s", ErrRecord, clock.Format(r.At))
	}
	return nil
}

// String renders the record the way a console line reads.
func (r Record) String() string {
	detail := r.Detail
	if detail == "" {
		detail = "-"
	}
	return fmt.Sprintf("%s %s %s %s %s %s", clock.Format(r.At), r.Tenant, r.Subject, r.Action, r.Outcome, detail)
}
