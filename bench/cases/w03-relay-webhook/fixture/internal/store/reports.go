package store

import (
	"encoding/csv"
	"fmt"
	"sort"
	"strings"

	"example.com/relay/internal/wire"
)

// Row is one report row: the record's fields plus the kind total it belongs to.
type Row struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Account string `json:"account"`
	Amount  int64  `json:"amount"`
	At      string `json:"at"`
}

// Report renders the records of one kind, in insertion order.
func (s *Store) Report(kind string, limit int) []Row {
	records := s.OfKind(kind)
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	rows := make([]Row, 0, len(records))
	for _, record := range records {
		rows = append(rows, Row{
			ID:      record.ID,
			Kind:    record.Kind,
			Account: record.Account,
			Amount:  record.Amount,
			At:      record.At,
		})
	}
	return rows
}

// Reconciliation is the per-kind comparison of what a stream declared with what the store holds.
type Reconciliation struct {
	Declared map[string]int64 `json:"declared"`
	Stored   map[string]int64 `json:"stored"`
}

// Reconcile compares declared totals with the store's. The difference is reported per kind, so a
// missing record is visible as the kind it belongs to.
func (s *Store) Reconcile(declared map[string]int64) Reconciliation {
	stored := s.TotalsByKind()
	out := Reconciliation{Declared: map[string]int64{}, Stored: map[string]int64{}}
	seen := map[string]bool{}
	for kind, amount := range declared {
		out.Declared[kind] = amount
		out.Stored[kind] = stored[kind]
		seen[kind] = true
	}
	for kind, amount := range stored {
		if seen[kind] {
			continue
		}
		out.Stored[kind] = amount
	}
	return out
}

// Difference is the declared minus the stored total of one kind.
func (r Reconciliation) Difference(kind string) int64 {
	return r.Declared[kind] - r.Stored[kind]
}

// Matched reports whether every kind reconciles exactly.
func (r Reconciliation) Matched() bool {
	for kind, declared := range r.Declared {
		if r.Stored[kind] != declared {
			return false
		}
	}
	for kind, stored := range r.Stored {
		if r.Declared[kind] != stored {
			return false
		}
	}
	return true
}

// Kinds lists the kinds of a reconciliation, sorted.
func (r Reconciliation) Kinds() []string {
	seen := map[string]bool{}
	for kind := range r.Declared {
		seen[kind] = true
	}
	for kind := range r.Stored {
		seen[kind] = true
	}
	out := make([]string, 0, len(seen))
	for kind := range seen {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// CSV renders records as CSV, quoting through the encoder so an account name with a comma stays one
// cell.
func CSV(rows []Row) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write([]string{"id", "kind", "account", "amount", "at"}); err != nil {
		return "", err
	}
	for _, row := range rows {
		if err := w.Write([]string{
			row.ID, row.Kind, row.Account, fmt.Sprintf("%d", row.Amount), row.At,
		}); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ValidateBatch refuses a batch the store cannot apply: every record must validate and the batch
// must not carry the same id twice.
func ValidateBatch(records []wire.Record) error {
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
		if seen[record.ID] {
			return fmt.Errorf("store: record %q appears twice in one batch", record.ID)
		}
		seen[record.ID] = true
	}
	return nil
}
