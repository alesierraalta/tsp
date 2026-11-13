package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/events"
	"example.com/shiptrack/internal/httpx"
	"example.com/shiptrack/internal/workspace"
)

// The ingest window. A scan older than maxEventAge is refused because the period it belongs to may
// already be closed, and a scan further than maxEventFuture ahead is refused because the carrier's clock
// is wrong. Both are durations, so the comparison is on instants and not on a formatted date.
const (
	maxEventAge    = 90 * 24 * time.Hour
	maxEventFuture = 5 * time.Minute
)

// ingestLimits is the bound set the wire validation reads for this deployment.
func (a *API) ingestLimits() events.Limits {
	return events.Limits{
		MaxGrams:   a.deps.Config.Caps.MaxGrams,
		MaxNote:    a.deps.Config.Caps.MaxNote,
		MaxAge:     maxEventAge,
		MaxFuture:  maxEventFuture,
		MaxPerBody: a.deps.Config.Caps.MaxBatch,
	}
}

// ingestEvents accepts a batch of carrier scans. The tenant and the partner come from the token; the body
// names the shipment, the carrier and the lane, and each is validated against the directory before it
// reaches the store. A batch that fails validation stores nothing: a partially accepted batch would leave
// the sender unable to tell which scans landed.
func (a *API) ingestEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	body, err := httpx.ReadBody(r, a.deps.Config.Caps.MaxBody)
	if err != nil {
		httpx.JSONError(w, http.StatusRequestEntityTooLarge, "too large", "the request body is over the cap")
		return
	}
	wires, err := events.DecodeBatch(body, a.deps.Config.Caps.MaxBody, a.deps.Config.Caps.MaxBatch)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	now := a.now()
	accepted := make([]events.Event, 0, len(wires))
	for i, wire := range wires {
		event, err := wire.Validate(now, p.partner, a.deps.Directory, a.ingestLimits())
		if err != nil {
			httpx.JSONError(w, http.StatusBadRequest, "invalid", fmt.Sprintf("event %d: %v", i, err))
			return
		}
		if err := a.deps.Store.AddEvent(event); err != nil {
			httpx.Error(w, err)
			return
		}
		accepted = append(accepted, event)
	}
	// The payload archive is written after the store holds the scan: a file with no record would be a
	// scan a partner cannot query, and the record is what a statement bills against.
	for _, event := range accepted {
		raw, err := json.Marshal(wireOf(event))
		if err != nil {
			httpx.Error(w, err)
			return
		}
		name, err := workspace.EventPayloadName(event.Tenant, clock.FormatDay(now, a.deps.Config.ZoneOrDefault()), event.ID)
		if err != nil {
			httpx.Error(w, err)
			return
		}
		if _, err := a.deps.Workspace.Write(name, raw, 0o644); err != nil {
			httpx.Error(w, err)
			return
		}
		event.PayloadName = name
	}
	if err := a.auditWrite(p, "events.ingest", fmt.Sprintf("%d scans %s", len(accepted), p.partner.ID)); err != nil {
		httpx.Error(w, err)
		return
	}
	ids := make([]string, 0, len(accepted))
	shipments := map[string]bool{}
	for _, event := range accepted {
		ids = append(ids, event.ID)
		shipments[event.Shipment] = true
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"accepted":  len(accepted),
		"events":    ids,
		"shipments": len(shipments),
	})
}

// wireOf renders an accepted event back into the wire form the archive keeps. The archive holds what the
// carrier sent, not the service's record of it, so a re-read can be compared with the sender's copy.
func wireOf(event events.Event) events.Wire {
	origin, destination, _ := strings.Cut(event.Lane, ">")
	return events.Wire{
		Shipment:    event.Shipment,
		Carrier:     event.Carrier,
		Origin:      origin,
		Destination: destination,
		Service:     event.Service,
		Kind:        string(event.Kind),
		At:          clock.FormatInstant(event.Instant),
		Facility:    event.Facility,
		Grams:       event.Grams,
		Note:        event.Note,
	}
}
