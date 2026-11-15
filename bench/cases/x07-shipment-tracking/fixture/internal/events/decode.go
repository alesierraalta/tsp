package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrTooLarge is reported for a body above the bound the caller allowed.
var ErrTooLarge = errors.New("events: body too large")

// minEventBytes is the fewest bytes an encoded event can take. A batch's slice is sized from the
// number of events it claims, capped by the bytes actually present divided by this constant, so a
// claimed count cannot make the service allocate memory the body cannot back.
const minEventBytes = 48

// batchEnvelope is the form a batch posts: a claimed count and the events. The events are read as raw
// messages first, so the count is checked against what is actually there before any event is parsed.
type batchEnvelope struct {
	Count  int               `json:"count"`
	Events []json.RawMessage `json:"events"`
}

// DecodeObject reads one event from a body. The body is bounded before it is parsed, unknown fields
// are refused, and a second JSON value after the object is refused: a body that carries two objects
// is not one event, and reading only the first would silently drop the second.
func DecodeObject(body []byte, limit int64) (Wire, error) {
	if limit <= 0 {
		return Wire{}, fmt.Errorf("%w: bound %d", ErrTooLarge, limit)
	}
	if int64(len(body)) > limit {
		return Wire{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, len(body))
	}
	var wire Wire
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Wire{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := noTrailing(decoder); err != nil {
		return Wire{}, err
	}
	return wire, nil
}

// DecodeBatch reads the batch form. The count a batch claims must be inside the deployment's cap and
// must equal the number of events that are actually there; the slice is sized from the smaller of the
// claimed count and the bytes the body can back. An event whose form is wrong fails the whole batch:
// a partially accepted batch would leave the sender with no way to know which scans landed.
func DecodeBatch(body []byte, limit int64, maxCount int) ([]Wire, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: bound %d", ErrTooLarge, limit)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLarge, len(body))
	}
	if maxCount < 1 {
		return nil, fmt.Errorf("%w: count cap %d", ErrInvalid, maxCount)
	}
	var envelope batchEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := noTrailing(decoder); err != nil {
		return nil, err
	}
	if envelope.Count < 0 || envelope.Count > maxCount {
		return nil, fmt.Errorf("%w: count %d is outside 0..%d", ErrInvalid, envelope.Count, maxCount)
	}
	if envelope.Count != len(envelope.Events) {
		return nil, fmt.Errorf("%w: count %d but %d events", ErrInvalid, envelope.Count, len(envelope.Events))
	}
	// A claimed count is a number a sender writes; the body is what limits the memory. Pre-sizing by the
	// smaller of the two keeps a body of "{"count":500000,"events":[]}" from allocating the claim.
	capacity := envelope.Count
	if bound := len(body) / minEventBytes; bound < capacity {
		capacity = bound
	}
	events := make([]Wire, 0, capacity)
	for i, raw := range envelope.Events {
		var wire Wire
		item := json.NewDecoder(bytes.NewReader(raw))
		item.DisallowUnknownFields()
		if err := item.Decode(&wire); err != nil {
			return nil, fmt.Errorf("%w: event %d: %v", ErrInvalid, i, err)
		}
		if err := noTrailing(item); err != nil {
			return nil, fmt.Errorf("%w: event %d: %v", ErrInvalid, i, err)
		}
		events = append(events, wire)
	}
	return events, nil
}

// DecodeReader bounds a reader and decodes one event from it. The read stops one byte over the bound,
// so a body above it is reported rather than buffered in full.
func DecodeReader(r io.Reader, limit int64) (Wire, error) {
	if limit <= 0 {
		return Wire{}, fmt.Errorf("%w: bound %d", ErrTooLarge, limit)
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return Wire{}, fmt.Errorf("%w: read: %v", ErrInvalid, err)
	}
	if int64(len(body)) > limit {
		return Wire{}, fmt.Errorf("%w: body is over %d bytes", ErrTooLarge, limit)
	}
	return DecodeObject(body, limit)
}

// noTrailing refuses anything after the value that was read.
func noTrailing(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("%w: a second JSON value follows", ErrInvalid)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}
