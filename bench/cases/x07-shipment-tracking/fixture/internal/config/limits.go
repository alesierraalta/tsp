package config

import (
	"fmt"
)

// Caps are the deployment's request and record bounds. Every field has a hard ceiling as well as a
// floor: a deployment that raised a page to a million rows or a body to a gigabyte would be a
// deployment one request can turn into an out-of-memory event, so the ceiling is the service's, not the
// operator's.
type Caps struct {
	PageSize          int
	MaxBody           int64
	MaxBatch          int
	MaxGrams          int64
	MaxNote           int
	MaxLines          int
	MaxResponse       int64
	MaxEvents         int
	MaxShipments      int
	CloseWindowMonths int
	MaxExportCells    int
	TrailCap          int
}

// Ceilings are the hard limits a configured cap must sit inside, in the order Caps declares them.
type Ceiling struct {
	Key  string
	Min  int64
	Max  int64
	What string
}

// Ceilings lists the bounds ParseCaps and Caps.Validate apply. They are exported so the operations
// document and the tests read the same numbers the service enforces.
var Ceilings = []Ceiling{
	{KeyPageSize, 1, 500, "page size"},
	{KeyMaxBody, 1024, 4 << 20, "request body bytes"},
	{KeyMaxBatch, 1, 5000, "events per batch"},
	{KeyMaxGrams, 1, 10000000, "shipment grams"},
	{KeyMaxNote, 0, 4096, "note bytes"},
	{KeyMaxLines, 1, 20000, "statement lines"},
	{KeyMaxResp, 1024, 1 << 20, "delivery response bytes"},
	{KeyMaxEvents, 1, 10000000, "events held"},
	{KeyMaxShips, 1, 1000000, "shipments held"},
	{KeyCloseWin, 1, 24, "close window months"},
	{KeyMaxCells, 1, 50000, "export cells"},
	{KeyTrailCap, 1, 1000000, "audit entries"},
}

// Defaults are the caps a deployment gets when a key is unset.
func defaults() Caps {
	return Caps{
		PageSize:          100,
		MaxBody:           1 << 20,
		MaxBatch:          500,
		MaxGrams:          1000000,
		MaxNote:           500,
		MaxLines:          5000,
		MaxResponse:       256 << 10,
		MaxEvents:         200000,
		MaxShipments:      100000,
		CloseWindowMonths: 6,
		MaxExportCells:    20000,
		TrailCap:          100000,
	}
}

// ParseCaps reads the caps from an environment. A value outside its ceiling is refused with the range,
// not clamped: an operator who asked for a page of 10,000 rows hears that the service allows 500.
func ParseCaps(env Env) (Caps, error) {
	caps := defaults()
	var err error
	if caps.PageSize, err = env.Int(KeyPageSize, caps.PageSize); err != nil {
		return Caps{}, err
	}
	if caps.MaxBody, err = env.Int64(KeyMaxBody, caps.MaxBody); err != nil {
		return Caps{}, err
	}
	if caps.MaxBatch, err = env.Int(KeyMaxBatch, caps.MaxBatch); err != nil {
		return Caps{}, err
	}
	if caps.MaxGrams, err = env.Int64(KeyMaxGrams, caps.MaxGrams); err != nil {
		return Caps{}, err
	}
	if caps.MaxNote, err = env.Int(KeyMaxNote, caps.MaxNote); err != nil {
		return Caps{}, err
	}
	if caps.MaxLines, err = env.Int(KeyMaxLines, caps.MaxLines); err != nil {
		return Caps{}, err
	}
	if caps.MaxResponse, err = env.Int64(KeyMaxResp, caps.MaxResponse); err != nil {
		return Caps{}, err
	}
	if caps.MaxEvents, err = env.Int(KeyMaxEvents, caps.MaxEvents); err != nil {
		return Caps{}, err
	}
	if caps.MaxShipments, err = env.Int(KeyMaxShips, caps.MaxShipments); err != nil {
		return Caps{}, err
	}
	if caps.CloseWindowMonths, err = env.Int(KeyCloseWin, caps.CloseWindowMonths); err != nil {
		return Caps{}, err
	}
	if caps.MaxExportCells, err = env.Int(KeyMaxCells, caps.MaxExportCells); err != nil {
		return Caps{}, err
	}
	if caps.TrailCap, err = env.Int(KeyTrailCap, caps.TrailCap); err != nil {
		return Caps{}, err
	}
	if err := caps.Validate(); err != nil {
		return Caps{}, err
	}
	return caps, nil
}

// Validate checks every cap against its ceiling.
func (c Caps) Validate() error {
	values := []int64{
		int64(c.PageSize), c.MaxBody, int64(c.MaxBatch), c.MaxGrams, int64(c.MaxNote), int64(c.MaxLines),
		c.MaxResponse, int64(c.MaxEvents), int64(c.MaxShipments), int64(c.CloseWindowMonths),
		int64(c.MaxExportCells), int64(c.TrailCap),
	}
	for i, ceiling := range Ceilings {
		if values[i] < ceiling.Min || values[i] > ceiling.Max {
			return fmt.Errorf("config: %s %d is outside %d..%d (%s)",
				ceiling.Key, values[i], ceiling.Min, ceiling.Max, ceiling.What)
		}
	}
	return nil
}
