package api

import (
	"time"

	"example.com/shiptrack/internal/store"
)

// storeShipment turns a validated request into the record the store keeps. The partner, the tenant and
// the carrier are values the handler already resolved, so the conversion cannot change them; the status
// is "in-transit" because no scan has arrived yet.
func storeShipment(request shipmentRequest, p principal, carrier string, now time.Time) store.Shipment {
	return store.Shipment{
		ID:      request.Shipment,
		Tenant:  p.partner.Tenant,
		Partner: p.partner.ID,
		Carrier: carrier,
		Lane:    request.Lane,
		Service: request.Service,
		Grams:   request.Grams,
		Created: now.UTC(),
		Status:  "in-transit",
	}
}
