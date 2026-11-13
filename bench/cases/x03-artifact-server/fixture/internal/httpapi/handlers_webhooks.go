package httpapi

import (
	"net/http"

	"example.com/artifacts/internal/audit"
	"example.com/artifacts/internal/auth"
	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/httpx"
	"example.com/artifacts/internal/ids"
	"example.com/artifacts/internal/notify"
)

// subscriptionView is one subscription as a response carries it.
type subscriptionView struct {
	ID       string   `json:"id"`
	Project  string   `json:"project"`
	Endpoint string   `json:"endpoint"`
	Kinds    []string `json:"kinds"`
	Active   bool     `json:"active"`
}

// newSubscriptionView renders a subscription. The secret never travels: it is the subscription's
// signing key, and a response that carried it would hand a caller the ability to forge a delivery.
func newSubscriptionView(sub notify.Subscription) subscriptionView {
	return subscriptionView{
		ID: sub.ID, Project: sub.Project, Endpoint: sub.Endpoint,
		Kinds: append([]string(nil), sub.Kinds...), Active: sub.Active,
	}
}

// listSubscriptions answers `GET /api/subscriptions`.
func (a *API) listSubscriptions(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	subs := a.subs(project)
	views := make([]subscriptionView, 0, len(subs))
	for _, sub := range subs {
		views = append(views, newSubscriptionView(sub))
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"project": project, "subscriptions": views, "count": len(views),
	})
}

// subscriptionRequest is the body of a subscription.
type subscriptionRequest struct {
	Endpoint string   `json:"endpoint"`
	Secret   string   `json:"secret"`
	Kinds    []string `json:"kinds"`
}

// createSubscription answers `POST /api/subscriptions`. The endpoint is stored as the sender wrote it;
// whether it may be called is decided when a delivery runs, so a subscription can be recorded before
// the host is added to the deployment's allowlist.
func (a *API) createSubscription(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	var request subscriptionRequest
	if err := httpx.Decode(r, a.cfg.MaxBody, &request); err != nil {
		a.record(principal, "webhooks.write", "", audit.OutcomeRefused, err.Error())
		httpx.FailError(w, err)
		return
	}
	sub := notify.Subscription{
		ID: ids.New(ids.Subscription), Project: project, Endpoint: request.Endpoint,
		Secret: request.Secret, Kinds: request.Kinds, Active: true,
	}
	if err := sub.Validate(); err != nil {
		a.record(principal, "webhooks.write", sub.ID, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.BadRequest(err.Error()))
		return
	}
	if a.addSub == nil {
		httpx.FailError(w, httpx.Unprocessable("this deployment does not carry subscriptions"))
		return
	}
	if err := a.addSub(sub); err != nil {
		a.record(principal, "webhooks.write", sub.ID, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	a.record(principal, "webhooks.write", sub.ID, audit.OutcomeAccepted, "subscribed "+sub.KindList())
	httpx.Write(w, http.StatusCreated, newSubscriptionView(sub))
}

// eventRequest is the body of a broadcast or a re-delivery.
type eventRequest struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Detail  string `json:"detail,omitempty"`
}

// broadcast answers `POST /api/events/broadcast`: one event goes to every subscription of the project
// that asks for it.
func (a *API) broadcast(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	var request eventRequest
	if err := httpx.Decode(r, a.cfg.MaxBody, &request); err != nil {
		a.record(principal, "webhooks.write", "", audit.OutcomeRefused, err.Error())
		httpx.FailError(w, err)
		return
	}
	event := notify.Event{
		ID: ids.New(ids.Event), Project: project, Kind: request.Kind,
		Subject: request.Subject, Detail: request.Detail, At: clock.Format(a.nowUTC()),
	}
	if err := event.Validate(); err != nil {
		a.record(principal, "webhooks.write", "", audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.BadRequest(err.Error()))
		return
	}
	fanout, err := a.relay.Broadcast(r.Context(), a.subs(project), event)
	if err != nil && fanout.Event == "" {
		a.record(principal, "webhooks.write", event.ID, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	outcome := audit.OutcomeAccepted
	detail := "delivered to " + itoa(fanout.Delivered)
	if len(fanout.Refused) > 0 {
		outcome = audit.OutcomeFailed
		detail = "refused " + itoa(len(fanout.Refused)) + " of " + itoa(len(fanout.Deliveries))
	}
	a.record(principal, "webhooks.write", event.ID, outcome, detail)
	httpx.Write(w, http.StatusOK, map[string]any{
		"event": event.ID, "delivered": fanout.Delivered, "skipped": fanout.Skipped,
		"deliveries": fanout.Deliveries, "refused": fanout.Refused,
	})
}

// redeliver answers `POST /api/subscriptions/{id}/deliver`, which sends one event to one subscription
// through the guarded delivery path.
func (a *API) redeliver(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	target := httpx.Param(r, "id")
	var selected notify.Subscription
	found := false
	for _, sub := range a.subs(project) {
		if sub.ID == target {
			selected, found = sub, true
			break
		}
	}
	if !found {
		a.record(principal, "webhooks.write", target, audit.OutcomeRefused, "no such subscription")
		httpx.FailError(w, httpx.NotFoundError("no such subscription"))
		return
	}
	var request eventRequest
	if err := httpx.Decode(r, a.cfg.MaxBody, &request); err != nil {
		httpx.FailError(w, err)
		return
	}
	event := notify.Event{
		ID: ids.New(ids.Event), Project: project, Kind: request.Kind,
		Subject: request.Subject, Detail: request.Detail, At: clock.Format(a.nowUTC()),
	}
	delivery, err := a.relay.Deliver(r.Context(), selected, event)
	if err != nil {
		a.record(principal, "webhooks.write", target, audit.OutcomeRefused, err.Error())
		httpx.Write(w, http.StatusOK, map[string]any{
			"delivery": delivery, "error": err.Error(), "delivered": false,
		})
		return
	}
	a.record(principal, "webhooks.write", target, audit.OutcomeAccepted, "delivered")
	httpx.Write(w, http.StatusOK, map[string]any{"delivery": delivery, "delivered": true})
}

// itoa renders a count.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
