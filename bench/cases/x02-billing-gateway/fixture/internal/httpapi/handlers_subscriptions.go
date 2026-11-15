package httpapi

import (
	"net/http"
	"time"

	"example.com/billing/internal/audit"
	"example.com/billing/internal/auth"
	"example.com/billing/internal/clock"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/ids"
	"example.com/billing/internal/invoicing"
	"example.com/billing/internal/money"
	"example.com/billing/internal/proration"
	"example.com/billing/internal/store"
)

// seatsRequest is the body of a seat change.
type seatsRequest struct {
	Seats int `json:"seats"`
}

// seatsResponse is what a seat change answers with.
type seatsResponse struct {
	Subscription subscriptionView `json:"subscription"`
	Change       string           `json:"change"`
	Charge       *amountView      `json:"charge,omitempty"`
	Credit       *amountView      `json:"credit,omitempty"`
	Invoice      string           `json:"invoice,omitempty"`
	Cycle        string           `json:"cycle"`
	Factor       string           `json:"factor"`
}

// changeSeats answers `POST /api/subscriptions/{id}/seats`. The change is priced for the rest of the
// cycle by internal/proration and the part is billed on an invoice of its own, so the cycle's own
// invoice is not rewritten after it was issued.
func (a *API) changeSeats(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	target := httpx.Param(r, "id")
	sub, err := a.store.SubscriptionInTenant(tenant, target)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.NotFoundError("no such subscription"))
		return
	}
	if sub.Status == store.StatusCanceled {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, "the subscription is canceled")
		httpx.FailError(w, httpx.Unprocessable("the subscription is canceled"))
		return
	}
	var request seatsRequest
	if err := httpx.Decode(r, a.lim.BodyBytes, &request); err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, err)
		return
	}
	if request.Seats == sub.Seats {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, "the seat count does not change")
		httpx.FailError(w, httpx.BadRequest("the seat count does not change"))
		return
	}
	if request.Seats < 0 || request.Seats > 10000 {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, "the seat count is outside 0..10000")
		httpx.FailError(w, httpx.BadRequest("the seat count is outside 0..10000"))
		return
	}
	plan, err := a.store.Plan(sub.Plan)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable("the subscription names a plan this tenant does not carry"))
		return
	}
	now := a.nowUTC()
	cycle, err := sub.Cycle(now)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable("the subscription has no cycle at this instant"))
		return
	}
	change := proration.SeatChange{
		Pricing: proration.Pricing{SeatPrice: plan.SeatPrice, IncludedSeats: plan.IncludedSeats},
		From:    sub.Seats,
		To:      request.Seats,
		Period:  cycle,
		At:      now,
	}
	response := seatsResponse{Cycle: cycle.String(), Factor: factorOf(cycle, now)}
	switch {
	case request.Seats > sub.Seats:
		charge, err := change.Charge()
		if err != nil {
			a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
			httpx.FailError(w, httpx.Unprocessable(err.Error()))
			return
		}
		description, quantity := change.Line(charge)
		invoice, err := a.chargeInvoice(sub, plan, cycle, now, description, quantity, charge)
		if err != nil {
			a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
			httpx.FailError(w, err)
			return
		}
		response.Change = "added"
		view := newAmountView(charge)
		response.Charge = &view
		response.Invoice = invoice.ID
	default:
		note, err := proration.SeatCreditNote(change)
		if err != nil {
			a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
			httpx.FailError(w, httpx.Unprocessable(err.Error()))
			return
		}
		if note.Amount.Minor > 0 {
			credit := store.Credit{
				ID: ids.New(ids.Credit), Tenant: tenant, Account: sub.Account,
				Amount: note.Amount, Reason: note.Reason, At: now,
			}
			if err := a.store.AddCredit(credit); err != nil {
				a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
				httpx.FailError(w, err)
				return
			}
			view := newAmountView(note.Amount)
			response.Credit = &view
		}
		response.Change = "removed"
	}
	sub.Seats = request.Seats
	sub.UpdatedAt = now
	if err := a.store.UpdateSubscription(sub); err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	response.Subscription = a.subscriptionView(sub)
	a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeAccepted,
		"seats "+itoa(sub.Seats))
	httpx.Write(w, http.StatusOK, response)
}

// chargeInvoice stores the invoice a mid-cycle seat charge is billed on.
func (a *API) chargeInvoice(sub store.Subscription, plan store.Plan, cycle clock.Period, now time.Time,
	description string, quantity int64, amount money.Amount) (store.Invoice, error) {
	line, err := invoicing.ProrationLine(description, quantity, plan.SeatPrice, amount, plan.SeatPrice.Currency)
	if err != nil {
		return store.Invoice{}, httpx.Unprocessable(err.Error())
	}
	paid, err := money.New(0, plan.SeatPrice.Currency)
	if err != nil {
		return store.Invoice{}, err
	}
	invoice := store.Invoice{
		ID: ids.New(ids.Invoice), Tenant: sub.Tenant, Account: sub.Account,
		Subscription: sub.ID, Period: cycle, Issued: now, Due: now.AddDate(0, 0, 14),
		Status: store.InvoiceOpen, Currency: plan.SeatPrice.Currency,
		Lines: []store.Line{line}, Total: amount, Paid: paid,
	}
	if err := a.store.PutInvoice(invoice); err != nil {
		return store.Invoice{}, httpx.Unprocessable(err.Error())
	}
	return invoice, nil
}

// cancelRequest is the body of a cancellation.
type cancelRequest struct {
	Reason string `json:"reason"`
}

// cancelResponse is what a cancellation answers with.
type cancelResponse struct {
	Subscription subscriptionView `json:"subscription"`
	Credit       *creditView      `json:"credit,omitempty"`
	Cycle        string           `json:"cycle"`
	Factor       string           `json:"factor"`
}

// cancelSubscription answers `POST /api/subscriptions/{id}/cancel`. The days the customer will not use
// are credited through the shared proration, so the credit and the charge that raised the cycle agree.
func (a *API) cancelSubscription(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	target := httpx.Param(r, "id")
	sub, err := a.store.SubscriptionInTenant(tenant, target)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.NotFoundError("no such subscription"))
		return
	}
	if sub.Status == store.StatusCanceled {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, "the subscription is already canceled")
		httpx.FailError(w, httpx.Conflict("the subscription is already canceled"))
		return
	}
	var request cancelRequest
	if r.ContentLength > 0 {
		if err := httpx.Decode(r, a.lim.BodyBytes, &request); err != nil {
			a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeRefused, err.Error())
			httpx.FailError(w, err)
			return
		}
	}
	now := a.nowUTC()
	cycle, err := sub.Cycle(now)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable("the subscription has no cycle at this instant"))
		return
	}
	plan, err := a.store.Plan(sub.Plan)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable("the subscription names a plan this tenant does not carry"))
		return
	}
	subtotal := plan.BasePrice
	if seats, err := plan.SeatsPrice(sub.Seats); err == nil {
		if total, err := subtotal.Add(seats); err == nil {
			subtotal = total
		}
	}
	response := cancelResponse{Cycle: cycle.String(), Factor: factorOf(cycle, now)}
	notes, err := proration.Notes(subtotal, cycle, now)
	if err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	for _, note := range notes {
		credit := store.Credit{
			ID: ids.New(ids.Credit), Tenant: tenant, Account: sub.Account,
			Amount: note.Amount, Reason: note.Reason, At: now,
		}
		if err := a.store.AddCredit(credit); err != nil {
			a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
			httpx.FailError(w, err)
			return
		}
		view := newCreditView(credit)
		response.Credit = &view
	}
	sub.Status = store.StatusCanceled
	sub.CanceledAt = now
	sub.UpdatedAt = now
	if err := a.store.UpdateSubscription(sub); err != nil {
		a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	response.Subscription = a.subscriptionView(sub)
	detail := "canceled"
	if request.Reason != "" {
		detail = "canceled: " + request.Reason
	}
	a.record(principal, auth.ActionWriteBilling, target, audit.OutcomeAccepted, detail)
	httpx.Write(w, http.StatusOK, response)
}
