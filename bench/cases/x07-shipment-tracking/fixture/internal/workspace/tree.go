package workspace

import (
	"fmt"
	"path"
	"strings"

	"example.com/shiptrack/internal/clock"
)

// StatementName is the name a partner's statement is stored under:
//
//	partners/<partner>/statements/<period>/statement.<ext>
//
// Every variable part is validated by its own rule before it is joined, and the period is rendered by
// clock.Period, which cannot produce a slash. The name is built here and nowhere else, so no handler
// assembles one from request text.
func StatementName(partner string, period clock.Period, ext string) (string, error) {
	if !ValidSegment(partner) {
		return "", fmt.Errorf("%w: partner %q", ErrBadName, partner)
	}
	if !period.Valid() {
		return "", fmt.Errorf("%w: period %q", ErrBadName, period)
	}
	if !validExtension(ext) {
		return "", fmt.Errorf("%w: extension %q", ErrBadName, ext)
	}
	return path.Join("partners", partner, "statements", period.String(), "statement."+ext), nil
}

// LabelName is the name a shipment's rendered label is stored under:
//
//	partners/<partner>/labels/<shipment>/label.<ext>
//
// A label belongs to a partner, so a partner that is not a name segment is refused here even though
// the caller has already resolved it: the name rule does not rely on its caller.
func LabelName(partner, shipment, ext string) (string, error) {
	if !ValidSegment(partner) {
		return "", fmt.Errorf("%w: partner %q", ErrBadName, partner)
	}
	if !ValidIdentifier(shipment) {
		return "", fmt.Errorf("%w: shipment %q", ErrBadName, shipment)
	}
	if !validExtension(ext) {
		return "", fmt.Errorf("%w: extension %q", ErrBadName, ext)
	}
	return path.Join("partners", partner, "labels", shipment, "label."+ext), nil
}

// LabelInputName is the name the label renderer's input file is written under. It sits beside the
// output so the renderer is never handed a path from another part of the tree:
//
//	partners/<partner>/labels/<shipment>/label.input.json
func LabelInputName(partner, shipment string) (string, error) {
	if !ValidSegment(partner) {
		return "", fmt.Errorf("%w: partner %q", ErrBadName, partner)
	}
	if !ValidIdentifier(shipment) {
		return "", fmt.Errorf("%w: shipment %q", ErrBadName, shipment)
	}
	return path.Join("partners", partner, "labels", shipment, "label.input.json"), nil
}

// ExportName is the name a report export is stored under:
//
//	exports/<partner>/<report>-<period>.csv
//
// The report name is required to be a slug, which is the form a definition carries, so a report name
// a request supplies is looked up in the catalog before it reaches here.
func ExportName(partner, report string, period clock.Period) (string, error) {
	if !ValidSegment(partner) {
		return "", fmt.Errorf("%w: partner %q", ErrBadName, partner)
	}
	if !ValidSegment(report) {
		return "", fmt.Errorf("%w: report %q", ErrBadName, report)
	}
	if !period.Valid() {
		return "", fmt.Errorf("%w: period %q", ErrBadName, period)
	}
	return path.Join("exports", partner, report+"-"+period.String()+".csv"), nil
}

// EventPayloadName is the name an accepted event payload is archived under:
//
//	events/<tenant>/<day>/<event>.json
//
// The day is a calendar day rendered by the service, and the event id is a minted identifier, so the
// name is bounded even for a deployment that ingests for years.
func EventPayloadName(tenant, day, event string) (string, error) {
	if !ValidSegment(tenant) {
		return "", fmt.Errorf("%w: tenant %q", ErrBadName, tenant)
	}
	if len(day) != 10 || day[4] != '-' || day[7] != '-' {
		return "", fmt.Errorf("%w: day %q", ErrBadName, day)
	}
	for i := 0; i < len(day); i++ {
		if day[i] == '-' {
			continue
		}
		if day[i] < '0' || day[i] > '9' {
			return "", fmt.Errorf("%w: day %q", ErrBadName, day)
		}
	}
	if !ValidIdentifier(event) {
		return "", fmt.Errorf("%w: event %q", ErrBadName, event)
	}
	return path.Join("events", tenant, day, event+".json"), nil
}

// Parse reports the segments of a name the workspace produced. It is used by an operator view that
// prints where an object sits, and it refuses a name that would not resolve.
func Parse(name string) ([]string, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	clean := path.Clean(filepathSlash(name))
	if clean == "." {
		return nil, fmt.Errorf("%w: %q", ErrBadName, name)
	}
	return strings.Split(clean, "/"), nil
}

// filepathSlash normalizes the separators of a name for Parse, which reports slash-separated segments.
func filepathSlash(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}

func validExtension(ext string) bool {
	if len(ext) < 1 || len(ext) > 8 {
		return false
	}
	for i := 0; i < len(ext); i++ {
		c := ext[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
