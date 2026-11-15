package httpapi

import (
	"net/http"
	"time"

	"example.com/billing/internal/audit"
	"example.com/billing/internal/auth"
	"example.com/billing/internal/clock"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/ids"
	"example.com/billing/internal/store"
)

// usageRequest is the body of a usage report.
type usageRequest struct {
	Account string `json:"account"`
	Metric  string `json:"metric"`
	// Quantity is what the reporter measured; it is not scaled, rounded or capped here, because a
	// quantity the gateway changed is a quantity nobody can explain to the customer.
	Quantity int64 `json:"quantity"`
	// At is when the usage happened. It defaults to the request's instant.
	At string `json:"at,omitempty"`
}

// usageResponse is what a usage report answers with.
type usageResponse struct {
	Event        string `json:"event"`
	Subscription string `json:"subscription"`
	Status       string `json:"status"`
	Quantity     int64  `json:"quantity"`
	Metric       string `json:"metric"`
	At           string `json:"at"`
}

// recordUsage answers `POST /api/usage`. The event is recorded against the account's active
// subscription of the caller's tenant, and the metric is the plan's own unless the plan meters none.
func (a *API) recordUsage(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	var request usageRequest
	if err := httpx.Decode(r, a.lim.BodyBytes, &request); err != nil {
		a.record(principal, auth.ActionWriteUsage, "", audit.OutcomeRefused, err.Error())
		httpx.FailError(w, err)
		return
	}
	if request.Account == "" {
		a.record(principal, auth.ActionWriteUsage, "", audit.OutcomeRefused, "the request names no account")
		httpx.FailError(w, httpx.BadRequest("the request names no account"))
		return
	}
	if request.Metric == "" {
		a.record(principal, auth.ActionWriteUsage, request.Account, audit.OutcomeRefused, "the request names no metric")
		httpx.FailError(w, httpx.BadRequest("the request names no metric"))
		return
	}
	if request.Quantity < 0 {
		a.record(principal, auth.ActionWriteUsage, request.Account, audit.OutcomeRefused, "the quantity is negative")
		httpx.FailError(w, httpx.BadRequest("the quantity is not a non-negative number"))
		return
	}
	at := a.nowUTC()
	if request.At != "" {
		parsed, err := clock.Parse(request.At)
		if err != nil {
			a.record(principal, auth.ActionWriteUsage, request.Account, audit.OutcomeRefused, "the instant does not parse")
			httpx.FailError(w, httpx.BadRequest("the instant is not RFC 3339"))
			return
		}
		if parsed.After(at) {
			a.record(principal, auth.ActionWriteUsage, request.Account, audit.OutcomeRefused, "the instant is in the future")
			httpx.FailError(w, httpx.BadRequest("the usage happened in the future"))
			return
		}
		at = parsed
	}
	account, err := a.store.AccountInTenant(tenant, request.Account)
	if err != nil {
		a.record(principal, auth.ActionWriteUsage, request.Account, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.NotFoundError("no such account"))
		return
	}
	sub, err := a.store.ActiveSubscriptionOf(tenant, account.ID)
	if err != nil {
		a.record(principal, auth.ActionWriteUsage, request.Account, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.Unprocessable("the account has no active subscription"))
		return
	}
	plan, err := a.store.Plan(sub.Plan)
	if err != nil {
		a.record(principal, auth.ActionWriteUsage, sub.ID, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable("the subscription names a plan this tenant does not carry"))
		return
	}
	metric := request.Metric
	if plan.Metric != "" && metric != plan.Metric {
		a.record(principal, auth.ActionWriteUsage, sub.ID, audit.OutcomeRefused,
			"the plan meters "+plan.Metric)
		httpx.FailError(w, httpx.BadRequest("the plan meters "+plan.Metric+" and not "+metric))
		return
	}
	event := store.UsageEvent{
		ID: ids.New(ids.UsageEvent), Tenant: tenant, Account: account.ID,
		Subscription: sub.ID, Metric: metric, Quantity: request.Quantity,
		At: at.UTC(), Status: store.EventRecorded,
	}
	if err := a.store.AddEvent(event); err != nil {
		a.record(principal, auth.ActionWriteUsage, sub.ID, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	a.record(principal, auth.ActionWriteUsage, sub.ID, audit.OutcomeAccepted,
		itoa64(request.Quantity)+" "+metric)
	httpx.Write(w, http.StatusCreated, usageResponse{
		Event: event.ID, Subscription: sub.ID, Status: string(event.Status),
		Quantity: event.Quantity, Metric: event.Metric, At: clock.Format(event.At),
	})
}

// nowAt returns a fixed instant for a handler that needs one, which keeps the calls short.
func nowAt(t time.Time) time.Time { return t.UTC() }
