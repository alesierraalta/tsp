package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"example.com/ledgerclose/internal/auth"
	"example.com/ledgerclose/internal/calendar"
	"example.com/ledgerclose/internal/export"
	"example.com/ledgerclose/internal/httpx"
	"example.com/ledgerclose/internal/invoice"
	"example.com/ledgerclose/internal/journal"
	"example.com/ledgerclose/internal/query"
	"example.com/ledgerclose/internal/render"
	"example.com/ledgerclose/internal/usage"
	"example.com/ledgerclose/internal/workspace"
)

// record appends one audit entry. A handler that cannot audit a write does not complete it: the
// trail is the record the service is asked for, and a write without one is a gap.
func (a *API) record(principal auth.Principal, action, ref string) error {
	_, err := a.trail.Append(journal.Entry{
		At: a.now(), Actor: principal.Subject, Tenant: principal.Tenant, Action: action, Ref: ref,
	})
	return err
}

// tenantAccounts is the scope of a read: the account ids the caller's tenant owns. It is passed to the store
// with the selection, so a query can only answer with rows of the tenant the token asserts.
func (a *API) tenantAccounts(principal auth.Principal) []string {
	accounts := a.directory.Tenant(principal.Tenant)
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "period": calendar.Of(a.now()).String(), "routes": a.Routes(),
		"accounts": a.directory.Len(), "rates": a.book.Len(),
	})
}

// handleIngest accepts a batch of metered events.
//
// The body is bounded before it is decoded, whichever form it arrives in: an object body goes through
// usage.DecodeObject and an application/x-ndjson body through usage.DecodeStream, and both decode one
// event at a time through the same rules. Nothing is stored until the batch, its events and its accounts
// check out: a half-applied batch is work the sender cannot reconcile.
func (a *API) handleIngest(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := a.principal(w, r, auth.ActionUsageWrite)
	if !ok {
		return
	}
	var batch usage.Batch
	var err error
	if r.Body == nil {
		httpx.Fail(w, http.StatusBadRequest, "the request carries no body")
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-ndjson") {
		batch, err = usage.DecodeStream(io.LimitReader(r.Body, a.limits.MaxBody+1), a.limits)
	} else {
		batch, err = usage.DecodeObject(r.Body, a.limits)
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	if err := batch.Validate(a.limits, a.now()); err != nil {
		a.fail(w, err)
		return
	}
	if err := batch.CheckAccounts(a.directory, principal.Tenant); err != nil {
		a.fail(w, err)
		return
	}
	if err := a.store.PutEvents(batch.Events); err != nil {
		a.fail(w, err)
		return
	}
	ref := "batch-" + batch.Events[0].ID
	if err := a.record(principal, "usage.ingest", ref); err != nil {
		a.fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"accepted": batch.Len(), "metrics": batch.Metrics(), "ref": ref,
	})
}

// handleUsageCSV exports the tenant's events. Each declared parameter is looked up in the definition and an
// undeclared one is refused, so a filter narrows the caller's rows and never widens them; the rows are
// filtered by tenant before the page is cut. The selection the export ran is published in a header.
func (a *API) handleUsageCSV(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := a.principal(w, r, auth.ActionInvoiceExport)
	if !ok {
		return
	}
	builder, events, err := a.selection(principal, r)
	if err != nil {
		a.fail(w, err)
		return
	}
	body, err := export.Usage(events)
	if err != nil {
		a.fail(w, err)
		return
	}
	if err := a.record(principal, auth.ActionInvoiceExport, "export-usage"); err != nil {
		a.fail(w, err)
		return
	}
	statement, args := builder.Statement()
	w.Header().Set("X-Ledger-Selection", fmt.Sprintf("%s (%d arguments)", statement, len(args)))
	httpx.WriteCSV(w, http.StatusOK, "usage.csv", body)
}

// selection builds the selection a request asks for, reads it to the store's bound, keeps the caller's
// tenant's rows and only then cuts the page: cutting first would let another tenant's rows spend it.
func (a *API) selection(principal auth.Principal, r *http.Request) (*query.Builder, []usage.Event, error) {
	builder, err := query.New(a.usageDefinition())
	if err != nil {
		return nil, nil, err
	}
	declared := map[string]bool{"limit": true, "sort": true, "desc": true}
	for _, param := range builder.Params() {
		declared[param.Name] = true
	}
	params := r.URL.Query()
	for name := range params {
		if !declared[name] {
			return nil, nil, fmt.Errorf("%w: %q is not a parameter of usage_events", query.ErrUnknown, name)
		}
	}
	for _, param := range builder.Params() {
		if value := params.Get(param.Name); value != "" || params.Has(param.Name) {
			if err := builder.Filter(param.Name, value); err != nil {
				return nil, nil, err
			}
		}
	}
	if name := params.Get("sort"); name != "" {
		if err := builder.Sort(name, params.Get("desc") == "true"); err != nil {
			return nil, nil, err
		}
	}
	limit, err := httpx.QueryInt(r, "limit", 0)
	if err != nil {
		return nil, nil, err
	}
	if limit > 0 {
		if err := builder.Limit(limit); err != nil {
			return nil, nil, err
		}
	}
	criteria := builder.Criteria()
	criteria.Accounts = a.tenantAccounts(principal)
	events, err := a.store.SelectEvents(criteria)
	if err != nil {
		return nil, nil, err
	}
	return builder, events, nil
}

// handleClose runs the close for one period. The period is parsed and checked against the window before
// anything is read; the accounts are the caller's tenant's, and each statement is written through the
// workspace, which resolves the name, and rendered from the file just written.
func (a *API) handleClose(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := a.principal(w, r, auth.ActionCloseRun)
	if !ok {
		return
	}
	var body struct {
		Period string `json:"period"`
	}
	if err := httpx.DecodeJSON(w, r, a.limits.MaxBody, &body); err != nil {
		a.fail(w, err)
		return
	}
	period, err := calendar.Parse(body.Period)
	if err != nil {
		a.fail(w, err)
		return
	}
	if err := a.cfg.WindowAllows(period, a.now()); err != nil {
		a.fail(w, err)
		return
	}
	issued := make([]map[string]any, 0, 8)
	for _, account := range a.directory.Tenant(principal.Tenant) {
		events := a.store.EventsByAccount(account.ID, period, a.limits.MaxLines+1)
		if len(events) == 0 {
			continue
		}
		statement, err := invoice.Assemble(account, period, events, a.book, a.limits, a.now())
		if err != nil {
			a.fail(w, err)
			return
		}
		if err := a.writeStatement(r.Context(), statement); err != nil {
			a.fail(w, err)
			return
		}
		if err := a.store.PutInvoice(statement); err != nil {
			a.fail(w, err)
			return
		}
		if err := a.record(principal, "close.statement", statement.ID); err != nil {
			a.fail(w, err)
			return
		}
		issued = append(issued, map[string]any{
			"id": statement.ID, "account": statement.Account, "lines": statement.LineCount(),
			"total": statement.Total.String(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"period": period.String(), "statements": issued, "count": len(issued),
	})
}

// writeStatement writes the HTML statement and, when a renderer is configured, renders the PDF from it.
// Both names come from workspace.StatementName, which validates every part and builds the name.
func (a *API) writeStatement(ctx context.Context, statement invoice.Invoice) error {
	html, err := workspace.StatementName(statement.Tenant, statement.Period, statement.Account, ".html")
	if err != nil {
		return err
	}
	if _, err := a.workspace.Write(html, []byte(render.Statement(statement)), 0o644); err != nil {
		return err
	}
	if a.renderer == nil {
		return nil
	}
	source, err := a.workspace.StatementPath(statement.Tenant, statement.Period, statement.Account, ".html")
	if err != nil {
		return err
	}
	target, err := a.workspace.StatementPath(statement.Tenant, statement.Period, statement.Account, ".pdf")
	if err != nil {
		return err
	}
	renderCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return a.renderer.Render(renderCtx, source, target)
}

// statementOf returns the statement a request names, and refuses one of another tenant.
func (a *API) statementOf(w http.ResponseWriter, r *http.Request, principal auth.Principal) (invoice.Invoice, bool) {
	statement, err := a.store.Invoice(r.PathValue("id"))
	if err != nil {
		a.fail(w, err)
		return invoice.Invoice{}, false
	}
	if statement.Tenant != principal.Tenant {
		// A statement of another tenant is not there for this caller: the answer is the one a missing id
		// gets, so the surface does not confirm that an id exists elsewhere.
		httpx.Fail(w, http.StatusNotFound, "no such statement")
		return invoice.Invoice{}, false
	}
	return statement, true
}

func (a *API) handleStatement(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := a.principal(w, r, auth.ActionInvoiceRead)
	if !ok {
		return
	}
	statement, ok := a.statementOf(w, r, principal)
	if !ok {
		return
	}
	httpx.WriteHTML(w, http.StatusOK, render.Statement(statement))
}

// handleStatementFile serves the rendered PDF of a statement. The name is built from the stored
// statement's own fields, and the workspace refuses a name that is not inside the root.
func (a *API) handleStatementFile(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := a.principal(w, r, auth.ActionInvoiceRead)
	if !ok {
		return
	}
	statement, ok := a.statementOf(w, r, principal)
	if !ok {
		return
	}
	name, err := workspace.StatementName(statement.Tenant, statement.Period, statement.Account, ".pdf")
	if err != nil {
		a.fail(w, err)
		return
	}
	body, err := a.workspace.Read(name)
	if err != nil {
		a.fail(w, err)
		return
	}
	httpx.WriteBytes(w, "application/pdf", body)
}

// handleDeliver posts a statement to a partner endpoint. The endpoint is a request value and it reaches the
// outbound client, which checks it on the parsed URL against the partner allowlist; this handler does not
// decide by itself. The payload is the statement the caller's tenant owns.
func (a *API) handleDeliver(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := a.principal(w, r, auth.ActionDelivery)
	if !ok {
		return
	}
	var body struct {
		Invoice  string `json:"invoice"`
		Endpoint string `json:"endpoint"`
	}
	if err := httpx.DecodeJSON(w, r, a.limits.MaxBody, &body); err != nil {
		a.fail(w, err)
		return
	}
	if a.notifier == nil {
		httpx.Fail(w, http.StatusBadRequest, "this deployment has no partner configured")
		return
	}
	statement, err := a.store.Invoice(body.Invoice)
	if err != nil {
		a.fail(w, err)
		return
	}
	if statement.Tenant != principal.Tenant {
		httpx.Fail(w, http.StatusNotFound, "no such statement")
		return
	}
	payload, err := json.Marshal(map[string]any{
		"id": statement.ID, "account": statement.Account, "period": statement.Period.String(),
		"total": statement.Total.String(), "currency": statement.Currency, "lines": statement.LineCount(),
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	result, err := a.notifier.Deliver(r.Context(), body.Endpoint, payload)
	if err != nil {
		_ = a.record(principal, "delivery.refused", statement.ID)
		a.fail(w, err)
		return
	}
	if err := a.record(principal, "delivery.sent", statement.ID); err != nil {
		a.fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": result.Status, "delivered": result.Delivered, "host": result.Host,
	})
}
