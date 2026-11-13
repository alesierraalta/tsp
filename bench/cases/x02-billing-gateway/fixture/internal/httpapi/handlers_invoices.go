package httpapi

import (
	"fmt"
	"net/http"

	"example.com/billing/internal/audit"
	"example.com/billing/internal/auth"
	"example.com/billing/internal/clock"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/ids"
	"example.com/billing/internal/money"
	"example.com/billing/internal/pagination"
	"example.com/billing/internal/store"
)

// listInvoices answers `GET /api/invoices` with the tenant's invoices, newest first.
func (a *API) listInvoices(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	page, err := pagination.Parse(r.URL.Query(), a.lim.PageSize)
	if err != nil {
		httpx.FailError(w, httpx.BadRequest(err.Error()))
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !validInvoiceStatus(status) {
		httpx.FailError(w, httpx.BadRequest(fmt.Sprintf("status %q is not one of draft, open, paid, void", status)))
		return
	}
	invoices := a.store.Invoices(tenant)
	views := make([]invoiceView, 0, len(invoices))
	for _, invoice := range invoices {
		if status != "" && string(invoice.Status) != status {
			continue
		}
		views = append(views, newInvoiceView(invoice))
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"tenant": tenant, "invoices": views, "count": len(views), "limit": page.Limit,
	})
}

// getInvoice answers `GET /api/invoices/{id}`.
func (a *API) getInvoice(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	invoice, err := a.store.InvoiceInTenant(tenant, httpx.Param(r, "id"))
	if err != nil {
		httpx.FailError(w, httpx.NotFoundError("no such invoice"))
		return
	}
	httpx.Write(w, http.StatusOK, newInvoiceView(invoice))
}

// paymentRequest is the body of a payment.
type paymentRequest struct {
	Amount money.Amount `json:"amount"`
	Note   string       `json:"note,omitempty"`
}

// recordPayment answers `POST /api/invoices/{id}/payments`. The invoice is paid when the payments cover
// its total, and a payment above it is refused: money the invoice does not ask for is a credit to
// issue, not a correction to hide.
func (a *API) recordPayment(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	target := httpx.Param(r, "id")
	invoice, err := a.store.InvoiceInTenant(tenant, target)
	if err != nil {
		a.record(principal, auth.ActionWriteInvoices, target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.NotFoundError("no such invoice"))
		return
	}
	var request paymentRequest
	if err := httpx.Decode(r, a.lim.BodyBytes, &request); err != nil {
		a.record(principal, auth.ActionWriteInvoices, target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, err)
		return
	}
	if request.Amount.Minor <= 0 {
		a.record(principal, auth.ActionWriteInvoices, target, audit.OutcomeRefused, "the payment is not positive")
		httpx.FailError(w, httpx.BadRequest("the payment is not a positive amount"))
		return
	}
	payment := store.Payment{
		ID: ids.New(ids.Payment), Tenant: tenant, Invoice: invoice.ID,
		Amount: request.Amount, At: a.nowUTC(),
	}
	if err := a.store.AddPayment(payment); err != nil {
		a.record(principal, auth.ActionWriteInvoices, target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	updated, err := a.store.Invoice(invoice.ID)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	a.record(principal, auth.ActionWriteInvoices, target, audit.OutcomeAccepted,
		"payment "+request.Amount.String())
	httpx.Write(w, http.StatusOK, map[string]any{
		"payment": storePaymentView(payment),
		"invoice": newInvoiceView(updated),
	})
}

// paymentView is one payment as a response carries it.
type paymentView struct {
	ID      string     `json:"id"`
	Invoice string     `json:"invoice"`
	Amount  amountView `json:"amount"`
	At      string     `json:"at"`
}

// storePaymentView renders a payment.
func storePaymentView(payment store.Payment) paymentView {
	return paymentView{
		ID: payment.ID, Invoice: payment.Invoice,
		Amount: newAmountView(payment.Amount), At: clock.Format(payment.At),
	}
}

// validInvoiceStatus reports whether a status is one the gateway knows.
func validInvoiceStatus(status string) bool {
	switch store.InvoiceStatus(status) {
	case store.InvoiceDraft, store.InvoiceOpen, store.InvoicePaid, store.InvoiceVoid:
		return true
	}
	return false
}
