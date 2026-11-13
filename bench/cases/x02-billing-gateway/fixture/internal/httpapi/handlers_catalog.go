package httpapi

import (
	"net/http"

	"example.com/billing/internal/auth"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/pagination"
	"example.com/billing/internal/store"
	"example.com/billing/internal/textutil"
)

// planView is one plan as a response carries it.
type planView struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	BasePrice        string `json:"base_price"`
	SeatPrice        string `json:"seat_price"`
	UnitPrice        string `json:"unit_price"`
	IncludedSeats    int    `json:"included_seats"`
	IncludedQuantity int64  `json:"included_quantity"`
	Metric           string `json:"metric"`
	Active           bool   `json:"active"`
}

// accountView is one account as a response carries it.
type accountView struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

// subscriptionView is one subscription as a response carries it.
type subscriptionView struct {
	ID       string `json:"id"`
	Account  string `json:"account"`
	Plan     string `json:"plan"`
	PlanCode string `json:"plan_code"`
	Seats    int    `json:"seats"`
	Status   string `json:"status"`
	Anchor   string `json:"anchor"`
	// Cycle is the billing cycle the subscription is in at the instant of the request.
	Cycle string `json:"cycle"`
	// SeatPrice is what one seat costs for a whole cycle.
	SeatPrice string `json:"seat_price"`
}

// listPlans answers `GET /api/plans` with the tenant's plans.
func (a *API) listPlans(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
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
	plans := a.store.Plans(tenant)
	views := make([]planView, 0, len(plans))
	for _, plan := range plans {
		views = append(views, planView{
			ID: plan.ID, Code: plan.Code, Name: plan.Name,
			BasePrice: plan.BasePrice.String(), SeatPrice: plan.SeatPrice.String(),
			UnitPrice: plan.UnitPrice.String(), IncludedSeats: plan.IncludedSeats,
			IncludedQuantity: plan.IncludedQuantity, Metric: plan.Metric, Active: plan.Active,
		})
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"tenant": tenant, "plans": views, "count": len(views), "limit": page.Limit,
	})
}

// listAccounts answers `GET /api/accounts` with the tenant's accounts.
func (a *API) listAccounts(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
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
	accounts := a.store.Accounts(tenant)
	views := make([]accountView, 0, len(accounts))
	for _, account := range accounts {
		views = append(views, accountView{
			ID: account.ID, Email: account.Email,
			Name: textutil.Title(account.Name), Created: account.Created.Format("2006-01-02"),
		})
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"tenant": tenant, "accounts": views, "count": len(views), "limit": page.Limit,
	})
}

// listSubscriptions answers `GET /api/subscriptions` with the tenant's subscriptions.
func (a *API) listSubscriptions(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
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
	subs := a.store.Subscriptions(tenant)
	views := make([]subscriptionView, 0, len(subs))
	for _, sub := range subs {
		views = append(views, a.subscriptionView(sub))
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"tenant": tenant, "subscriptions": views, "count": len(views), "limit": page.Limit,
	})
}

// getSubscription answers `GET /api/subscriptions/{id}`.
func (a *API) getSubscription(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	sub, err := a.store.SubscriptionInTenant(tenant, httpx.Param(r, "id"))
	if err != nil {
		httpx.FailError(w, httpx.NotFoundError("no such subscription"))
		return
	}
	httpx.Write(w, http.StatusOK, a.subscriptionView(sub))
}

// subscriptionView renders one subscription for a response, with the cycle it is in.
func (a *API) subscriptionView(sub store.Subscription) subscriptionView {
	plan, err := a.store.Plan(sub.Plan)
	if err != nil {
		plan = store.Plan{}
	}
	cycle := ""
	if period, err := sub.Cycle(a.nowUTC()); err == nil {
		cycle = period.String()
	}
	return subscriptionView{
		ID: sub.ID, Account: sub.Account, Plan: sub.Plan, PlanCode: plan.Code,
		Seats: sub.Seats, Status: string(sub.Status), Anchor: sub.Anchor.Format("2006-01-02"),
		Cycle: cycle, SeatPrice: plan.SeatPrice.String(),
	}
}
