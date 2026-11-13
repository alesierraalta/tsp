package api

import (
	"fmt"
	"net/http"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/httpx"
	"example.com/shiptrack/internal/idgen"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/notify"
	"example.com/shiptrack/internal/render"
	"example.com/shiptrack/internal/statement"
	"example.com/shiptrack/internal/store"
	"example.com/shiptrack/internal/workspace"
)

// closeRequest is the body of a close: the period only. The partner is the token's.
type closeRequest struct {
	Period string `json:"period"`
}

// closeStatement closes a partner's period into a statement. The period must be over by the clock and
// inside the deployment's window; the shipments are the partner's, created inside the period; each is
// priced by the rate book, and the statement is assembled with the capped allocation of fuel and tax.
func (a *API) closeStatement(w http.ResponseWriter, r *http.Request) {
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
	var request closeRequest
	if err := httpx.DecodeJSON(body, &request); err != nil {
		httpx.BadRequest(w, "the body is not a close")
		return
	}
	period, err := clock.ParsePeriod(request.Period)
	if err != nil {
		httpx.BadRequest(w, "the period is not YYYY-MM")
		return
	}
	now := a.now()
	if err := statement.CheckCloseable(period, now, a.deps.Config.Caps.CloseWindowMonths); err != nil {
		httpx.Error(w, err)
		return
	}
	items := make([]statement.Item, 0, 64)
	for _, shipment := range a.deps.Store.Shipments(p.partner.Tenant, p.partner.ID) {
		if !period.Contains(shipment.Created) {
			continue
		}
		quote, err := a.deps.Book.Rate(shipment.Service, shipment.Grams)
		if err != nil {
			httpx.Error(w, err)
			return
		}
		items = append(items, statement.Item{
			Shipment: shipment.ID,
			Carrier:  shipment.Carrier,
			Service:  shipment.Service,
			Lane:     shipment.Lane,
			Grams:    shipment.Grams,
			Base:     quote.Base,
			Fuel:     quote.Fuel,
			At:       shipment.Created,
		})
	}
	if len(items) == 0 {
		httpx.JSONError(w, http.StatusConflict, "statement refused",
			fmt.Sprintf("partner %q has no shipments in %s", p.partner.ID, period))
		return
	}
	built, err := statement.Build(p.partner, period, items, a.deps.Book.TaxBasisPoints(), now)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	page, err := render.StatementPage(built)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	name, err := workspace.StatementName(p.partner.ID, period, "html")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if _, err := a.deps.Workspace.Write(name, []byte(page), 0o644); err != nil {
		httpx.Error(w, err)
		return
	}
	id, err := idgen.New("stm")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	record, err := a.deps.Store.RecordStatement(store.Statement{
		ID:      id,
		Tenant:  built.Tenant,
		Partner: built.Partner.ID,
		Period:  period,
		Lines:   len(built.Lines),
		Net:     built.Net,
		Tax:     built.Tax,
		Total:   built.Total,
		Written: now,
		Paths:   []string{name},
	})
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if err := a.auditWrite(p, "statement.close", built.Summary()); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"statement": record,
		"path":      name,
		"summary":   built.Summary(),
	})
}

// showStatement answers a closed statement. The page is read back through the workspace by its name: the
// statement a partner fetches is the file the close wrote, opened through the containment rule, not a
// path a request supplied.
func (a *API) showStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	period, err := clock.ParsePeriod(httpx.Path(r, "period"))
	if err != nil {
		httpx.BadRequest(w, "the period is not YYYY-MM")
		return
	}
	record, ok := a.deps.Store.Statement(p.partner.Tenant, p.partner.ID, period)
	if !ok {
		httpx.JSONError(w, http.StatusNotFound, "not found", "no statement for this period")
		return
	}
	name, err := workspace.StatementName(p.partner.ID, period, "html")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	page, err := a.deps.Workspace.Read(name, maxStatementBytes)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if httpx.AcceptsJSON(r) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"period": record.Period,
			"lines":  record.Lines,
			"net":    record.Net.String(),
			"tax":    record.Tax.String(),
			"total":  record.Total.String(),
		})
		return
	}
	httpx.HTML(w, http.StatusOK, string(page))
}

// listStatements lists the partner's closed statements, newest period first.
func (a *API) listStatements(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	all := a.deps.Store.Statements(p.partner.Tenant)
	mine := make([]store.Statement, 0, len(all))
	for _, record := range all {
		if record.Partner == p.partner.ID {
			mine = append(mine, record)
		}
	}
	if httpx.AcceptsJSON(r) {
		httpx.JSON(w, http.StatusOK, map[string]any{"statements": mine, "count": len(mine)})
		return
	}
	built := make([]statement.Statement, 0, len(mine))
	for _, record := range mine {
		built = append(built, statement.Statement{
			Tenant:  record.Tenant,
			Partner: p.partner,
			Period:  record.Period,
			Net:     record.Net,
			Tax:     record.Tax,
			Total:   record.Total,
			Built:   record.Written,
		})
	}
	httpx.HTML(w, http.StatusOK, render.StatementIndex(p.partner.ID, built))
}

// deliverStatement posts a partner's statement notice to the partner's registered endpoint. The endpoint
// is read from the partner record, never from the body; internal/egress decides on the parsed URL whether
// the host is allowed, and the attempt is recorded whether it succeeded or failed.
func (a *API) deliverStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	if a.deps.Notify == nil {
		httpx.JSONError(w, http.StatusConflict, "delivery refused", "this deployment has no partner endpoints")
		return
	}
	if p.partner.Endpoint == "" {
		httpx.JSONError(w, http.StatusConflict, "delivery refused", "the partner has no endpoint")
		return
	}
	body, err := httpx.ReadBody(r, a.deps.Config.Caps.MaxBody)
	if err != nil {
		httpx.BadRequest(w, "the request body is over the cap")
		return
	}
	var request closeRequest
	if err := httpx.DecodeJSON(body, &request); err != nil {
		httpx.BadRequest(w, "the body is not a delivery")
		return
	}
	period, err := clock.ParsePeriod(request.Period)
	if err != nil {
		httpx.BadRequest(w, "the period is not YYYY-MM")
		return
	}
	record, ok := a.deps.Store.Statement(p.partner.Tenant, p.partner.ID, period)
	if !ok {
		httpx.JSONError(w, http.StatusNotFound, "not found", "no statement for this period")
		return
	}
	payload := notify.Payload{
		Event:     "statement.closed",
		Partner:   p.partner.ID,
		Tenant:    p.partner.Tenant,
		Period:    period.String(),
		Lines:     record.Lines,
		Net:       record.Net.String(),
		Tax:       record.Tax.String(),
		Total:     record.Total.String(),
		Currency:  "EUR",
		Statement: fmt.Sprintf("/api/statements/%s", period),
	}
	delivery, err := a.deps.Notify.Deliver(r.Context(), p.partner, p.partner.Endpoint, payload)
	stored, storeErr := a.deps.Store.RecordDelivery(store.Delivery{
		ID:       delivery.ID,
		Partner:  delivery.Partner,
		Endpoint: delivery.Endpoint,
		Status:   delivery.Status,
		Attempts: delivery.Attempts,
		At:       delivery.At,
	})
	_, trailErr := a.deps.Trail.Append("delivery.attempt", p.partner.Describe(), delivery.Describe(), a.now())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if storeErr != nil {
		httpx.Error(w, storeErr)
		return
	}
	if trailErr != nil {
		httpx.Error(w, trailErr)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"delivery": stored,
		"status":   delivery.Status,
		"attempts": delivery.Attempts,
	})
}

// maxStatementBytes bounds the page a statement read returns. A statement is a page a partner reads, and
// a page above this is not one the service wrote: the close bounds its own output by the line cap.
const maxStatementBytes = 4 << 20

// maxLabelBytes bounds the artifact a label read returns. It matches the renderer's own output bound.
const maxLabelBytes = 32 << 20

// amountText renders an amount for a page.
func amountText(amount money.Amount) string { return amount.String() }
