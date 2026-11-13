// Package wire decodes the record files the import pipeline accepts.
package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Errors a decoder reports.
var (
	ErrMalformed = errors.New("wire: malformed record")
	ErrTooLarge  = errors.New("wire: record above the size limit")
	ErrInvalid   = errors.New("wire: record is not acceptable")
)

// Record is one metered record. Amount is integer cents: the ledger has no fractional amounts.
type Record struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Account string `json:"account"`
	Amount  int64  `json:"amount"`
	At      string `json:"at"`
}

// Limits bound a decode.
type Limits struct {
	MaxRecord int
	MaxBatch  int
}

// DefaultLimits are the bounds a deployment runs with.
func DefaultLimits() Limits {
	return Limits{MaxRecord: 64 << 10, MaxBatch: 500}
}

// Decode reads one record from one file, refusing a body above the limit before it decodes it.
func Decode(body []byte, limits Limits) (Record, error) {
	if limits.MaxRecord <= 0 {
		return Record{}, fmt.Errorf("wire: limits are not set")
	}
	if len(body) > limits.MaxRecord {
		return Record{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, len(body))
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var record Record
	if err := dec.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Record{}, fmt.Errorf("%w: the body carries more than one record", ErrMalformed)
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Validate refuses a record the ledger cannot hold.
func (r Record) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: a record needs an id", ErrInvalid)
	}
	if r.Account == "" {
		return fmt.Errorf("%w: record %q has no account", ErrInvalid, r.ID)
	}
	if r.Kind == "" {
		return fmt.Errorf("%w: record %q has no kind", ErrInvalid, r.ID)
	}
	if r.Amount < 0 {
		return fmt.Errorf("%w: record %q has a negative amount %d", ErrInvalid, r.ID, r.Amount)
	}
	return nil
}

// DecodeBatch reads a JSON array of records, enforcing the batch count before it allocates the
// slice: a file that declares more records than the batch limit is refused, never truncated.
func DecodeBatch(body []byte, limits Limits) ([]Record, error) {
	if limits.MaxBatch <= 0 {
		return nil, fmt.Errorf("wire: limits are not set")
	}
	var envelope struct {
		Records []Record `json:"records"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if len(envelope.Records) > limits.MaxBatch {
		return nil, fmt.Errorf("%w: %d records", ErrTooLarge, len(envelope.Records))
	}
	for _, record := range envelope.Records {
		if err := record.Validate(); err != nil {
			return nil, err
		}
	}
	return envelope.Records, nil
}
