package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

// The bounds a host event may not cross. They are wide enough for a real session and narrow enough
// that a host cannot turn the ledger into a copy of its own transcript.
const (
	maxTools     = 200
	maxPerTurn   = 5000
	maxToolName  = 64
	maxIdentity  = 128
	maxModelName = 128
)

var (
	identityRe  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	modelRe     = regexp.MustCompile(`^[A-Za-z0-9._:/@-]{1,128}$`)
	toolNameRe  = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)
	knownSource = map[string]bool{SourcePi: true, SourceClaude: true, SourceOpencode: true}
	knownOffer  = map[string]bool{OfferAccepted: true, OfferDeclined: true, OfferAborted: true, OfferNone: true}
)

// Record is the input `tsp telemetry record` accepts on stdin: a host's report of one run. It is a
// closed shape — DisallowUnknownFields refuses any key it does not know, so free text a host might
// add is a refusal rather than a line in the ledger.
type Record struct {
	Source  string `json:"source"`
	RunID   string `json:"run_id,omitempty"`
	Session string `json:"session,omitempty"`
	Host    *Host  `json:"host,omitempty"`
}

// ParseRecord decodes and validates one host event. Every refusal names the field, never its value,
// because the value is what must not be persisted or echoed.
func ParseRecord(r io.Reader) (Record, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("read host event: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return Record{}, errors.New("refused trailing data after the host event")
	}
	if err := record.validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

// validate applies the closed allow-list: enums, identifiers, non-negative counts and the bounds.
func (r Record) validate() error {
	if !knownSource[r.Source] {
		return fmt.Errorf("source must be %s, %s or %s", SourcePi, SourceClaude, SourceOpencode)
	}
	if err := validateIdentity("run_id", r.RunID); err != nil {
		return err
	}
	if err := validateIdentity("session", r.Session); err != nil {
		return err
	}
	if r.Host == nil {
		return nil
	}
	return r.Host.validate()
}

// validateIdentity accepts an absent identifier and refuses one that is not an identifier shape.
func validateIdentity(field, value string) error {
	if value == "" {
		return nil
	}
	if !identityRe.MatchString(value) {
		return fmt.Errorf("%s must match %s", field, identityRe)
	}
	return nil
}

// validate applies the closed allow-list to the host body.
func (h Host) validate() error {
	if h.Model != "" && !modelRe.MatchString(h.Model) {
		return fmt.Errorf("model must match %s", modelRe)
	}
	if h.Offer != "" && !knownOffer[h.Offer] {
		return fmt.Errorf("offer must be %s, %s, %s or %s", OfferAccepted, OfferDeclined, OfferAborted, OfferNone)
	}
	for _, count := range []struct {
		name  string
		value *int64
	}{
		{"turns", h.Turns},
		{"tool_calls", h.ToolCalls},
		{"tool_errors", h.ToolErrors},
		{"compactions", h.Compactions},
		{"subagents", h.Subagents},
		{"wall_ms", h.WallMS},
		{"first_tsp_ms", h.FirstTSPMS},
	} {
		if err := nonNegative(count.name, count.value); err != nil {
			return err
		}
	}
	if h.CostUSD != nil && *h.CostUSD < 0 {
		return errors.New("cost_usd must not be negative")
	}
	if h.Tokens != nil {
		if err := h.Tokens.validate("tokens"); err != nil {
			return err
		}
	}
	if len(h.PerTurn) > maxPerTurn {
		return fmt.Errorf("per_turn holds at most %d turns", maxPerTurn)
	}
	for i := range h.PerTurn {
		if err := h.PerTurn[i].validate("per_turn"); err != nil {
			return err
		}
	}
	if len(h.Tools) > maxTools {
		return fmt.Errorf("tools holds at most %d names", maxTools)
	}
	for name, count := range h.Tools {
		if !toolNameRe.MatchString(name) {
			return fmt.Errorf("a tool name must match %s", toolNameRe)
		}
		if count < 0 {
			return errors.New("a tool count must not be negative")
		}
	}
	return nil
}

// validate applies the same non-negative rule to one turn's counts.
func (t Tokens) validate(field string) error {
	for _, count := range []struct {
		name  string
		value *int64
	}{
		{field + ".input", t.Input},
		{field + ".output", t.Output},
		{field + ".cache_read", t.CacheRead},
		{field + ".cache_write", t.CacheWrite},
	} {
		if err := nonNegative(count.name, count.value); err != nil {
			return err
		}
	}
	return nil
}

// nonNegative refuses a count below zero: an absent count is how "not measured" is spelled.
func nonNegative(field string, value *int64) error {
	if value != nil && *value < 0 {
		return fmt.Errorf("%s must not be negative", field)
	}
	return nil
}

// HostEvent turns a validated record into the event main flushes as kind host_run. The session stays
// raw here and becomes a salted pseudonym in Flush, which is the only place the salt exists.
func (r Record) HostEvent() Event {
	return Event{
		Source:  r.Source,
		Kind:    KindHostRun,
		RunID:   r.RunID,
		Session: r.Session,
		Host:    r.Host,
	}
}
