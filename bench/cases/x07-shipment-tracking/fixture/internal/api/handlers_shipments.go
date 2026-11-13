package api

import (
	"fmt"
	"net/http"
	"strings"

	"example.com/shiptrack/internal/checksum"
	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/httpx"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/idgen"
	"example.com/shiptrack/internal/labels"
	"example.com/shiptrack/internal/workspace"
)

// shipmentRequest is the body a partner posts to create a shipment. The partner and the tenant are not
// fields: they come from the token, and a body that carried them would be a body that could name another
// partner.
type shipmentRequest struct {
	Shipment string `json:"shipment"`
	Carrier  string `json:"carrier"`
	Lane     string `json:"lane"`
	Service  string `json:"service"`
	Grams    int64  `json:"grams"`
}

// createShipment accepts a shipment and assigns it to a carrier that serves its lane. The carrier must
// serve the lane and the service in the directory, and the rate book must carry the service: a shipment
// the service cannot price is refused here rather than at close, when the partner is waiting for a
// statement.
func (a *API) createShipment(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	body, err := httpx.ReadBody(r, a.deps.Config.Caps.MaxBody)
	if err != nil {
		httpx.BadRequest(w, "the request body is over the cap")
		return
	}
	var request shipmentRequest
	if err := httpx.DecodeJSON(body, &request); err != nil {
		httpx.BadRequest(w, "the body is not a shipment")
		return
	}
	if request.Shipment == "" {
		minted, err := idgen.New("shp")
		if err != nil {
			httpx.Error(w, err)
			return
		}
		request.Shipment = minted
	}
	if !idgen.Valid(request.Shipment) {
		httpx.BadRequest(w, "the shipment id is not one this service mints")
		return
	}
	origin, destination, ok := strings.Cut(request.Lane, ">")
	if !ok || strings.Count(request.Lane, ">") != 1 || !identity.Canonical(origin) || !identity.Canonical(destination) {
		httpx.BadRequest(w, "the lane is not <origin>><destination>")
		return
	}
	if !identity.Canonical(request.Service) {
		httpx.BadRequest(w, "the service is not a name")
		return
	}
	if request.Grams <= 0 || request.Grams > a.deps.Config.Caps.MaxGrams {
		httpx.BadRequest(w, "the weight is outside the deployment's cap")
		return
	}
	carrier, err := a.deps.Directory.ResolveCarrier(request.Carrier, origin, destination, request.Service)
	if err != nil {
		httpx.BadRequest(w, "the carrier does not serve this lane and service")
		return
	}
	quote, err := a.deps.Book.Rate(request.Service, request.Grams)
	if err != nil {
		httpx.BadRequest(w, "the service has no rate")
		return
	}
	now := a.now()
	shipment, err := a.deps.Store.AddShipment(storeShipment(request, p, carrier.ID, now))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if err := a.auditWrite(p, "shipment.assign", fmt.Sprintf("%s %s %s", shipment.ID, carrier.ID, request.Lane)); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"shipment": shipment,
		"quote": map[string]any{
			"service":   quote.Service,
			"kilograms": quote.Kilograms,
			"base":      quote.Base.String(),
			"fuel":      quote.Fuel.String(),
			"total":     quote.Total.String(),
		},
	})
}

// listShipments lists the partner's shipments, newest first. A page above the deployment's size is
// refused rather than clamped by the parameter reader, and the list itself is capped at the page size.
func (a *API) listShipments(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	limit, err := httpx.IntParam(r, "limit", a.deps.Config.Caps.PageSize, 1, a.deps.Config.Caps.PageSize)
	if err != nil {
		httpx.BadRequest(w, "the limit is not accepted")
		return
	}
	all := a.deps.Store.Shipments(p.partner.Tenant, p.partner.ID)
	if len(all) > limit {
		all = all[:limit]
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"shipments": all,
		"count":     len(all),
		"limit":     limit,
	})
}

// showShipment reads one shipment and its timeline. A shipment of another partner is answered as missing,
// not as forbidden: a partner that guesses an id learns nothing about whether it exists.
func (a *API) showShipment(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	id := httpx.Path(r, "id")
	shipment, err := a.deps.Store.Shipment(id)
	if err != nil || !ownedBy(shipment.Tenant, shipment.Partner, p) {
		httpx.JSONError(w, http.StatusNotFound, "not found", "no such shipment")
		return
	}
	timeline := a.deps.Store.Events(shipment.ID)
	orders := make([]map[string]any, 0, len(timeline))
	for _, event := range timeline {
		orders = append(orders, map[string]any{
			"id":       event.ID,
			"kind":     string(event.Kind),
			"at":       clock.FormatInstant(event.Instant),
			"facility": event.Facility,
			"note":     event.Note,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"shipment": shipment,
		"timeline": orders,
		"events":   len(orders),
	})
}

// renderLabel renders a shipment's label with the deployment's renderer. The input file is written through
// the workspace under a name built from validated parts; the renderer receives two absolute paths and no
// shell; and the artifact is read back through the workspace, which refuses a symbolic link and a file
// above the bound, before its digest is reported.
func (a *API) renderLabel(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	if a.deps.Renderer == nil {
		httpx.JSONError(w, http.StatusNotFound, "not found", "this deployment renders no labels")
		return
	}
	id := httpx.Path(r, "id")
	shipment, err := a.deps.Store.Shipment(id)
	if err != nil || !ownedBy(shipment.Tenant, shipment.Partner, p) {
		httpx.JSONError(w, http.StatusNotFound, "not found", "no such shipment")
		return
	}
	at := shipment.Created
	if due, ok := a.deps.Store.Due(shipment.ID); ok {
		at = due
	}
	spec, err := labels.NewSpec(p.partner, shipment.ID, shipment.Carrier, shipment.Lane, shipment.Service, shipment.Grams, at, "")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	payload, err := spec.Marshal()
	if err != nil {
		httpx.Error(w, err)
		return
	}
	inputName, err := workspace.LabelInputName(p.partner.ID, shipment.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if _, err := a.deps.Workspace.Write(inputName, payload, 0o600); err != nil {
		httpx.Error(w, err)
		return
	}
	outputName, err := workspace.LabelName(p.partner.ID, shipment.ID, "pdf")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	inputPath, err := a.deps.Workspace.Resolve(inputName)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	outputPath, err := a.deps.Workspace.Resolve(outputName)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if err := a.deps.Renderer.Render(r.Context(), inputPath, outputPath); err != nil {
		a.auditRefusal(p, r, "label.render", err)
		httpx.Error(w, err)
		return
	}
	// The artifact is opened through the workspace by its name, not by the path the renderer was given: the
	// read proves the file is inside the tree, is a regular file and is inside the bound.
	artifact, err := a.deps.Workspace.Read(outputName, maxLabelBytes)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if err := a.auditWrite(p, "label.render", fmt.Sprintf("%s %d bytes", shipment.ID, len(artifact))); err != nil {
		httpx.Error(w, err)
		return
	}
	w.Header().Set("X-Label-Digest", checksum.Of(artifact))
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"shipment": shipment.ID,
		"label":    outputName,
		"bytes":    len(artifact),
		"digest":   checksum.Of(artifact),
	})
}

// ownedBy reports whether a record belongs to the principal's partner. The comparison is exact: a
// shipment of "acme" is not one of "Acme", and the tenant is compared as well.
func ownedBy(tenant, partner string, p principal) bool {
	return tenant == p.partner.Tenant && partner == p.partner.ID
}
