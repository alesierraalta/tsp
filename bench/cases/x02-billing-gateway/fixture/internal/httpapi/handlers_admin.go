package httpapi

import (
	"net/http"
	"strconv"

	"example.com/billing/internal/audit"
	"example.com/billing/internal/auth"
	"example.com/billing/internal/clock"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/invoicing"
	"example.com/billing/internal/reconcile"
	"example.com/billing/internal/store"
)

// itoa renders a count, for a detail line of the trail.
func itoa(value int) string { return strconv.Itoa(value) }

// itoa64 renders a quantity, for a detail line of the trail.
func itoa64(value int64) string { return strconv.FormatInt(value, 10) }

// console answers `GET /api/admin/console` with what an operator looks at: the configuration, the
// record counts and the trail's own summary. The signing secret is never part of the page.
func (a *API) console(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if _, err := tenantOf(principal); err != nil {
		httpx.FailError(w, err)
		return
	}
	stats := make([]map[string]any, 0, 8)
	for _, stat := range a.store.Stats() {
		stats = append(stats, map[string]any{"name": stat.Name, "count": stat.Count})
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"config":   a.cfg.String(),
		"operator": principal.Subject,
		"records":  stats,
		"trail":    a.trail.Summary(),
		"dropped":  a.trail.Dropped(),
		"routes":   len(a.Patterns()),
		"tax":      a.tax.Regions(),
		"at":       clock.Format(a.nowUTC()),
	})
}

// auditTrail answers `GET /api/admin/audit` with the trail of the caller's tenant. The trail is readable
// only with the audit scope, which the route's action requires.
func (a *API) auditTrail(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	records := a.trail.OfTenant(tenant)
	views := make([]map[string]any, 0, len(records))
	for _, record := range records {
		views = append(views, map[string]any{
			"id": record.ID, "at": clock.Format(record.At), "subject": record.Subject,
			"action": record.Action, "target": record.Target,
			"outcome": record.Outcome, "detail": record.Detail,
		})
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"tenant": tenant, "records": views, "count": len(views),
		"summary": a.trail.Summary(), "dropped": a.trail.Dropped(),
	})
}

// sweepResponse is what a sweep answers with.
type sweepResponse struct {
	PastDue    int      `json:"past_due"`
	Invoices   []string `json:"invoices"`
	Reconciled bool     `json:"reconciled"`
	Findings   int      `json:"findings"`
	At         string   `json:"at"`
}

// sweep answers `POST /api/admin/sweep`. It moves the tenant's invoices past their grace period to past
// due, reconciles the tenant's books and records both in the trail.
func (a *API) sweep(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	now := a.nowUTC()
	invoices := a.store.Invoices(tenant)
	overdue := invoicing.PastDue(invoices, now, int64(a.cfg.RetentionGraceDays()))
	response := sweepResponse{Invoices: make([]string, 0, len(overdue)), At: clock.Format(now)}
	for _, invoice := range overdue {
		if invoice.Subscription != "" {
			if sub, err := a.store.Subscription(invoice.Subscription); err == nil && sub.Status == store.StatusActive {
				sub.Status = store.StatusPastDue
				sub.UpdatedAt = now
				if err := a.store.UpdateSubscription(sub); err != nil {
					a.record(principal, auth.ActionOperate, invoice.ID, audit.OutcomeFailed, err.Error())
					httpx.FailError(w, err)
					return
				}
			}
		}
		invoice.Status = store.InvoiceVoid
		if err := a.store.UpdateInvoice(invoice); err != nil {
			a.record(principal, auth.ActionOperate, invoice.ID, audit.OutcomeFailed, err.Error())
			httpx.FailError(w, err)
			return
		}
		response.Invoices = append(response.Invoices, invoice.ID)
		response.PastDue++
	}
	payments := make([]store.Payment, 0, 16)
	for _, invoice := range invoices {
		payments = append(payments, a.store.PaymentsOf(tenant, invoice.ID)...)
	}
	report, err := reconcile.Ledger(tenant, invoices, payments)
	if err != nil {
		a.record(principal, auth.ActionOperate, "", audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	response.Reconciled = report.Clean()
	response.Findings = len(report.Findings)
	a.record(principal, auth.ActionOperate, "", audit.OutcomeAccepted,
		"past due "+itoa(response.PastDue)+", findings "+itoa(response.Findings))
	httpx.Write(w, http.StatusOK, response)
}
