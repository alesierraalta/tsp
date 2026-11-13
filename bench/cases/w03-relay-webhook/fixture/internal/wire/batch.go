package wire

import (
	"fmt"
	"sort"
)

// Batch is a decoded set of records plus the totals the pipeline reconciles against.
type Batch struct {
	Records []Record
	Totals  map[string]int64
}

// Split divides records into batches of at most size records. A record is never dropped: the last
// batch keeps whatever is left, which is what makes the totals add up.
func Split(records []Record, size int) ([][]Record, error) {
	if size <= 0 {
		return nil, fmt.Errorf("wire: batch size %d", size)
	}
	var out [][]Record
	for start := 0; start < len(records); start += size {
		end := start + size
		if end > len(records) {
			end = len(records)
		}
		out = append(out, records[start:end])
	}
	return out, nil
}

// Sum totals the amounts of a record set by kind, in cents.
func Sum(records []Record) map[string]int64 {
	totals := map[string]int64{}
	for _, record := range records {
		totals[record.Kind] += record.Amount
	}
	return totals
}

// Build validates a record set and returns it as a batch with its totals.
func Build(records []Record, limits Limits) (Batch, error) {
	if limits.MaxBatch > 0 && len(records) > limits.MaxBatch {
		return Batch{}, fmt.Errorf("%w: %d records above %d", ErrTooLarge, len(records), limits.MaxBatch)
	}
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return Batch{}, err
		}
		if seen[record.ID] {
			return Batch{}, fmt.Errorf("%w: record %q appears twice", ErrInvalid, record.ID)
		}
		seen[record.ID] = true
	}
	return Batch{Records: records, Totals: Sum(records)}, nil
}

// Kinds lists the kinds of a batch, sorted.
func (b Batch) Kinds() []string {
	out := make([]string, 0, len(b.Totals))
	for kind := range b.Totals {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// Len reports how many records the batch carries.
func (b Batch) Len() int { return len(b.Records) }
