package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"example.com/ledgerclose/internal/identity"
)

// minEventBytes is the smallest encoded event a well-formed batch can carry. It is the second bound on
// the allocation a decode makes: a body of n bytes cannot hold more than n/minEventBytes events.
const minEventBytes = 48

// maxHeaderBytes bounds the first line of a streamed batch.
const maxHeaderBytes = 256

// readBounded reads at most limit bytes and refuses a source that carries more. The reader is asked
// for one byte over the limit, so "exactly at the limit" is accepted and "over it" is refused rather
// than truncated into a batch the sender did not send.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: no bound", ErrInvalid)
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("usage: read: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, limit)
	}
	return data, nil
}

// decodeEvent decodes one event object. Unknown fields are refused and so is a second value after the
// object, so a payload that carries two events in one place is refused instead of silently losing one.
func decodeEvent(raw []byte) (Event, error) {
	var event Event
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Event{}, fmt.Errorf("%w: an event carries more than one value", ErrMalformed)
	}
	return event, nil
}

// DecodeObject reads the object form, {"count":N,"events":[...]}. The count is checked against the
// configured cap and against the number of events the body actually carries before any slice is
// sized from it, and the slice's capacity is further bounded by what the body could hold.
func DecodeObject(r io.Reader, limits Limits) (Batch, error) {
	data, err := readBounded(r, limits.MaxBody)
	if err != nil {
		return Batch{}, err
	}
	var body struct {
		Count  int               `json:"count"`
		Events []json.RawMessage `json:"events"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return Batch{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Batch{}, fmt.Errorf("%w: the body carries more than one value", ErrMalformed)
	}
	if body.Count < 0 || body.Count > limits.MaxBatch {
		return Batch{}, fmt.Errorf("%w: count %d is not in 0..%d", ErrTooLarge, body.Count, limits.MaxBatch)
	}
	if body.Count != len(body.Events) {
		return Batch{}, fmt.Errorf("%w: count %d, %d events", ErrMalformed, body.Count, len(body.Events))
	}
	hint := body.Count
	if room := len(data) / minEventBytes; hint > room {
		hint = room
	}
	events := make([]Event, 0, hint)
	for _, raw := range body.Events {
		event, err := decodeEvent(raw)
		if err != nil {
			return Batch{}, err
		}
		events = append(events, event)
	}
	return Batch{Events: events}, nil
}

// DecodeStream reads the streamed form: one header line, {"count":N}, then one event per line. The
// first line is bounded on its own, the rest of the body is bounded as a whole, and the count is
// checked against the cap and against the lines that are actually there before it is used.
func DecodeStream(r io.Reader, limits Limits) (Batch, error) {
	data, err := readBounded(r, limits.MaxBody)
	if err != nil {
		return Batch{}, err
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) == 0 || len(bytes.TrimSpace(lines[0])) == 0 {
		return Batch{}, fmt.Errorf("%w: no header line", ErrMalformed)
	}
	if len(lines[0]) > maxHeaderBytes {
		return Batch{}, fmt.Errorf("%w: header of %d bytes", ErrMalformed, len(lines[0]))
	}
	var header struct {
		Count int `json:"count"`
	}
	decoder := json.NewDecoder(bytes.NewReader(lines[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&header); err != nil {
		return Batch{}, fmt.Errorf("%w: header: %v", ErrMalformed, err)
	}
	if header.Count < 0 || header.Count > limits.MaxBatch {
		return Batch{}, fmt.Errorf("%w: count %d is not in 0..%d", ErrTooLarge, header.Count, limits.MaxBatch)
	}
	payload := make([][]byte, 0, header.Count)
	for _, line := range lines[1:] {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		payload = append(payload, line)
	}
	if header.Count != len(payload) {
		return Batch{}, fmt.Errorf("%w: count %d, %d lines", ErrMalformed, header.Count, len(payload))
	}
	hint := header.Count
	if room := len(data) / minEventBytes; hint > room {
		hint = room
	}
	events := make([]Event, 0, hint)
	for _, line := range payload {
		event, err := decodeEvent(line)
		if err != nil {
			return Batch{}, err
		}
		events = append(events, event)
	}
	return Batch{Events: events}, nil
}

// CheckAccounts proves every event names the account it says and the caller's tenant.
func (b Batch) CheckAccounts(directory *identity.Directory, tenant string) error {
	for _, event := range b.Events {
		account, err := directory.ByID(event.Account)
		if err != nil {
			return fmt.Errorf("%w: id %q: %v", ErrInvalid, event.ID, err)
		}
		if err := CheckAccount(event, account, tenant); err != nil {
			return err
		}
	}
	return nil
}

// Validate applies the batch rules: a batch that carries no event, more events than the configured
// cap or two events with the same id is refused as a whole, before anything is stored. A partial
// ingest would leave a sender a batch to reconcile by hand.
func (b Batch) Validate(limits Limits, now time.Time) error {
	if len(b.Events) == 0 {
		return fmt.Errorf("%w: no events", ErrInvalid)
	}
	if len(b.Events) > limits.MaxBatch {
		return fmt.Errorf("%w: %d events, cap %d", ErrTooLarge, len(b.Events), limits.MaxBatch)
	}
	seen := make(map[string]bool, len(b.Events))
	for _, event := range b.Events {
		if seen[event.ID] {
			return fmt.Errorf("%w: id %q", ErrDuplicate, event.ID)
		}
		seen[event.ID] = true
		if err := event.Validate(limits, now); err != nil {
			return err
		}
	}
	return nil
}

// Metrics reports the distinct metrics the batch carries, sorted.
func (b Batch) Metrics() []string {
	seen := make(map[string]bool, len(Metrics))
	out := make([]string, 0, len(Metrics))
	for _, event := range b.Events {
		if seen[event.Metric] {
			continue
		}
		seen[event.Metric] = true
		out = append(out, event.Metric)
	}
	sort.Strings(out)
	return out
}
