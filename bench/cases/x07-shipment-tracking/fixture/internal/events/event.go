// Package events is the carrier scan an ingest accepts and the timeline a shipment's scans make. The
// wire form is what a carrier posts; the service validates every field of it, mints the event's own
// identifier, and stores the result. Nothing in the wire form is trusted to name a tenant or a
// partner: those come from the token, which is the only place a request's identity is decided.
package events

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/idgen"
)

// Errors validation and ingestion report.
var (
	ErrInvalid   = errors.New("events: invalid event")
	ErrDuplicate = errors.New("events: duplicate event")
	ErrOrder     = errors.New("events: out of order")
)

// Kind is what a scan says happened. The set is closed: a kind the service does not know is refused,
// because a timeline that carries an unread status is a status the console cannot show.
type Kind string

// The kinds a carrier may report.
const (
	KindPickedUp       Kind = "picked-up"
	KindDeparted       Kind = "departed"
	KindArrived        Kind = "arrived"
	KindOutForDelivery Kind = "out-for-delivery"
	KindDelivered      Kind = "delivered"
	KindException      Kind = "exception"
	KindReturned       Kind = "returned"
)

// kinds is the closed set, in the order the service lists it.
var kinds = []Kind{
	KindPickedUp,
	KindDeparted,
	KindArrived,
	KindOutForDelivery,
	KindDelivered,
	KindException,
	KindReturned,
}

// Kinds lists the accepted kinds.
func Kinds() []Kind { return append([]Kind(nil), kinds...) }

// ParseKind reads a kind exactly. "Delivered" is not "delivered": the comparison is a string
// comparison, so a sender cannot smuggle a new status in through a capital letter.
func ParseKind(raw string) (Kind, error) {
	for _, kind := range kinds {
		if string(kind) == raw {
			return kind, nil
		}
	}
	return "", fmt.Errorf("%w: kind %q", ErrInvalid, raw)
}

// Final reports whether a kind ends a shipment's timeline.
func (k Kind) Final() bool { return k == KindDelivered || k == KindReturned }

// Event is one accepted scan.
type Event struct {
	ID          string
	Tenant      string
	Partner     string
	Shipment    string
	Carrier     string
	Lane        string
	Service     string
	Kind        Kind
	Instant     time.Time
	Facility    string
	Grams       int64
	Note        string
	Recorded    time.Time
	PayloadName string
}

// Lane renders the lane an event belongs to. It is built from the two validated facility codes, so a
// lane is a display value and never a path.
func Lane(origin, destination string) string { return origin + ">" + destination }

// Wire is the form a carrier posts. Every field is a string or an integer, so the decoder can refuse
// an unknown field and a value of the wrong type before validation reads it.
type Wire struct {
	Shipment    string `json:"shipment"`
	Carrier     string `json:"carrier"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Service     string `json:"service"`
	Kind        string `json:"kind"`
	At          string `json:"at"`
	Facility    string `json:"facility"`
	Grams       int64  `json:"grams"`
	Note        string `json:"note"`
}

// Limits are the ingest bounds a deployment accepts, carried as values so validation does not read
// configuration on its own.
type Limits struct {
	MaxGrams   int64
	MaxNote    int
	MaxAge     time.Duration
	MaxFuture  time.Duration
	MaxPerBody int
}

// Validate turns a wire event into an event the service can store. The shipment, carrier, facility,
// lane and service must be known and exact; the instant must carry an offset and sit inside the
// ingest window; the weight must be inside the deployment's cap. The tenant and the partner come from
// the caller's token, never from the body.
func (w Wire) Validate(now time.Time, partner identity.Partner, directory *identity.Directory, limits Limits) (Event, error) {
	if !idgen.Valid(w.Shipment) {
		return Event{}, fmt.Errorf("%w: shipment %q", ErrInvalid, w.Shipment)
	}
	if !identity.Canonical(w.Carrier) {
		return Event{}, fmt.Errorf("%w: carrier %q", ErrInvalid, w.Carrier)
	}
	if !identity.Canonical(w.Origin) || !identity.Canonical(w.Destination) {
		return Event{}, fmt.Errorf("%w: lane %q->%q", ErrInvalid, w.Origin, w.Destination)
	}
	if !identity.Canonical(w.Service) {
		return Event{}, fmt.Errorf("%w: service %q", ErrInvalid, w.Service)
	}
	carrier, err := directory.ResolveCarrier(w.Carrier, w.Origin, w.Destination, w.Service)
	if err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	kind, err := ParseKind(w.Kind)
	if err != nil {
		return Event{}, err
	}
	instant, err := clock.ParseInstant(w.At)
	if err != nil {
		return Event{}, fmt.Errorf("%w: at %q", ErrInvalid, w.At)
	}
	if !clock.Within(instant, now.UTC(), limits.MaxAge, limits.MaxFuture) {
		return Event{}, fmt.Errorf("%w: at %q is outside the ingest window", ErrInvalid, w.At)
	}
	if w.Grams <= 0 || w.Grams > limits.MaxGrams {
		return Event{}, fmt.Errorf("%w: grams %d", ErrInvalid, w.Grams)
	}
	if !identity.Canonical(w.Facility) {
		return Event{}, fmt.Errorf("%w: facility %q", ErrInvalid, w.Facility)
	}
	if len(w.Note) > limits.MaxNote {
		return Event{}, fmt.Errorf("%w: note is %d bytes", ErrInvalid, len(w.Note))
	}
	if err := checkText(w.Note); err != nil {
		return Event{}, err
	}
	id, err := idgen.New("evt")
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:       id,
		Tenant:   partner.Tenant,
		Partner:  partner.ID,
		Shipment: w.Shipment,
		Carrier:  carrier.ID,
		Lane:     Lane(w.Origin, w.Destination),
		Service:  w.Service,
		Kind:     kind,
		Instant:  instant,
		Facility: w.Facility,
		Grams:    w.Grams,
		Note:     w.Note,
		Recorded: now.UTC(),
	}, nil
}

// checkText refuses a control character in a value that will be rendered. The value is escaped where
// it is interpolated as well; a refused control character is how a page avoids carrying a byte the
// terminal would act on.
func checkText(value string) error {
	for _, r := range value {
		if r < 0x20 && r != '\t' || r == 0x7f {
			return fmt.Errorf("%w: text carries a control character", ErrInvalid)
		}
	}
	return nil
}

// Timeline sorts events by instant, then by id so two scans of the same instant have a stable order.
func Timeline(list []Event) []Event {
	sorted := append([]Event(nil), list...)
	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].Instant.Equal(sorted[j].Instant) {
			return sorted[i].Instant.Before(sorted[j].Instant)
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

// Latest is the last event of a shipment's timeline, or the zero event.
func Latest(list []Event) (Event, bool) {
	if len(list) == 0 {
		return Event{}, false
	}
	return Timeline(list)[len(list)-1], true
}

// Status is the status a timeline ends on: the last scan's kind, or "in-transit" for a shipment with
// no scan yet.
func Status(list []Event) string {
	last, ok := Latest(list)
	if !ok {
		return "in-transit"
	}
	return string(last.Kind)
}

// Label is a short human name for a kind, used in a console and in a statement description.
func (k Kind) Label() string {
	return strings.ReplaceAll(string(k), "-", " ")
}
