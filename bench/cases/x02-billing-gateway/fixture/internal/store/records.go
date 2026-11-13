// Package store is the billing service's records: tenants, plans, accounts, subscriptions, usage
// events, invoices, credits and payments. It holds them in memory and maintains a projection of them
// that the reports read through the statement engine, so a report never walks the records directly.
package store

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/config"
	"example.com/billing/internal/ids"
	"example.com/billing/internal/money"
	"example.com/billing/internal/query"
	"example.com/billing/internal/sqlrun"
)

// Errors the store reports.
var (
	// ErrNotFound is returned for a record the store does not carry.
	ErrNotFound = errors.New("store: no such record")
	// ErrDuplicate is returned when a record would replace another with the same id.
	ErrDuplicate = errors.New("store: the record already exists")
	// ErrInvalid is returned for a record that fails its own validation.
	ErrInvalid = errors.New("store: invalid record")
	// ErrCap is returned when a write would exceed a deployment cap.
	ErrCap = errors.New("store: the deployment cap is reached")
)

// SubscriptionStatus is where a subscription is in its life.
type SubscriptionStatus string

// The states a subscription can be in.
const (
	// StatusActive is a subscription that bills.
	StatusActive SubscriptionStatus = "active"
	// StatusPastDue is a subscription whose last invoice was not paid on time.
	StatusPastDue SubscriptionStatus = "past_due"
	// StatusCanceled is a subscription that no longer bills.
	StatusCanceled SubscriptionStatus = "canceled"
)

// Valid reports whether the status is one this service knows.
func (s SubscriptionStatus) Valid() bool {
	switch s {
	case StatusActive, StatusPastDue, StatusCanceled:
		return true
	}
	return false
}

// InvoiceStatus is where an invoice is in its life.
type InvoiceStatus string

// The states an invoice can be in.
const (
	// InvoiceDraft is an invoice that has not been issued; its lines may still change.
	InvoiceDraft InvoiceStatus = "draft"
	// InvoiceOpen is an issued invoice the customer owes.
	InvoiceOpen InvoiceStatus = "open"
	// InvoicePaid is an invoice covered by payments.
	InvoicePaid InvoiceStatus = "paid"
	// InvoiceVoid is an invoice withdrawn before payment; it is never collected.
	InvoiceVoid InvoiceStatus = "void"
)

// Valid reports whether the status is one this service knows.
func (s InvoiceStatus) Valid() bool {
	switch s {
	case InvoiceDraft, InvoiceOpen, InvoicePaid, InvoiceVoid:
		return true
	}
	return false
}

// EventStatus is where a usage event is in its life.
type EventStatus string

// The states a usage event can be in.
const (
	// EventRecorded is an event that has not been billed.
	EventRecorded EventStatus = "recorded"
	// EventBilled is an event an invoice carries.
	EventBilled EventStatus = "billed"
	// EventArchived is an event kept for the retention window but out of the current period.
	EventArchived EventStatus = "archived"
)

// Valid reports whether the status is one this service knows.
func (s EventStatus) Valid() bool {
	switch s {
	case EventRecorded, EventBilled, EventArchived:
		return true
	}
	return false
}

// Tenant is one customer organization. A tenant is the boundary of every listing: a token names one,
// and no route reads a tenant from a request body.
type Tenant struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

// tenantIDRe is the shape a tenant identifier has: the name the operator writes in the deployment's
// tenant list. A tenant is named by the operator, not minted, because it is the name a token and a
// record carry and an operator has to be able to write it.
var tenantIDRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ValidTenantID reports whether value is a tenant identifier.
func ValidTenantID(value string) bool { return tenantIDRe.MatchString(value) }

// Validate checks the tenant.
func (t Tenant) Validate() error {
	if !ValidTenantID(t.ID) {
		return fmt.Errorf("%w: tenant id %q", ErrInvalid, t.ID)
	}
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: tenant %q has no name", ErrInvalid, t.ID)
	}
	return nil
}

// Plan is one price list a tenant sells. The prices are per cycle: BasePrice for the plan itself,
// SeatPrice per seat, UnitPrice per metered unit above the plan's included quantity.
type Plan struct {
	ID            string       `json:"id"`
	Tenant        string       `json:"tenant"`
	Code          string       `json:"code"`
	Name          string       `json:"name"`
	BasePrice     money.Amount `json:"base_price"`
	SeatPrice     money.Amount `json:"seat_price"`
	UnitPrice     money.Amount `json:"unit_price"`
	IncludedSeats int          `json:"included_seats"`
	// IncludedQuantity is how many metered units the base price covers; -1 marks a plan that is not
	// metered at all.
	IncludedQuantity int64  `json:"included_quantity"`
	Metric           string `json:"metric"`
	Active           bool   `json:"active"`
}

// Validate checks the plan.
func (p Plan) Validate() error {
	if !ids.ValidKind(p.ID, ids.Plan) {
		return fmt.Errorf("%w: plan id %q", ErrInvalid, p.ID)
	}
	if !ValidTenantID(p.Tenant) {
		return fmt.Errorf("%w: plan %q names tenant %q", ErrInvalid, p.ID, p.Tenant)
	}
	if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: plan %q needs a code and a name", ErrInvalid, p.ID)
	}
	for _, amount := range []money.Amount{p.BasePrice, p.SeatPrice, p.UnitPrice} {
		if err := amount.Validate(); err != nil {
			return fmt.Errorf("%w: plan %q: %v", ErrInvalid, p.ID, err)
		}
		if _, err := amount.NonNegative(); err != nil {
			return fmt.Errorf("%w: plan %q: %v", ErrInvalid, p.ID, err)
		}
	}
	if p.IncludedSeats < 0 {
		return fmt.Errorf("%w: plan %q includes %d seats", ErrInvalid, p.ID, p.IncludedSeats)
	}
	if p.IncludedQuantity < -1 {
		return fmt.Errorf("%w: plan %q includes %d units", ErrInvalid, p.ID, p.IncludedQuantity)
	}
	if p.Metric != "" && !query.ValidName(p.Metric) {
		return fmt.Errorf("%w: plan %q carries metric %q", ErrInvalid, p.ID, p.Metric)
	}
	return nil
}

// SeatsPrice returns the price of the seats a subscription carries above the plan's included count.
func (p Plan) SeatsPrice(seats int) (money.Amount, error) {
	billable := seats - p.IncludedSeats
	if billable <= 0 {
		return money.Zero(p.SeatPrice.Currency), nil
	}
	return p.SeatPrice.Mul(int64(billable))
}

// Account is one paying customer inside a tenant.
type Account struct {
	ID      string    `json:"id"`
	Tenant  string    `json:"tenant"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

// Validate checks the account.
func (a Account) Validate() error {
	if !ids.ValidKind(a.ID, ids.Account) {
		return fmt.Errorf("%w: account id %q", ErrInvalid, a.ID)
	}
	if !ValidTenantID(a.Tenant) {
		return fmt.Errorf("%w: account %q names tenant %q", ErrInvalid, a.ID, a.Tenant)
	}
	if !strings.Contains(a.Email, "@") || strings.HasPrefix(a.Email, "@") || strings.HasSuffix(a.Email, "@") {
		return fmt.Errorf("%w: account %q carries email %q", ErrInvalid, a.ID, a.Email)
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("%w: account %q has no name", ErrInvalid, a.ID)
	}
	return nil
}

// Subscription is one account's use of one plan.
type Subscription struct {
	ID         string             `json:"id"`
	Tenant     string             `json:"tenant"`
	Account    string             `json:"account"`
	Plan       string             `json:"plan"`
	Seats      int                `json:"seats"`
	Anchor     time.Time          `json:"anchor"`
	Status     SubscriptionStatus `json:"status"`
	CanceledAt time.Time          `json:"canceled_at,omitempty"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

// Validate checks the subscription.
func (s Subscription) Validate() error {
	if !ids.ValidKind(s.ID, ids.Subscription) {
		return fmt.Errorf("%w: subscription id %q", ErrInvalid, s.ID)
	}
	if !ValidTenantID(s.Tenant) {
		return fmt.Errorf("%w: subscription %q names tenant %q", ErrInvalid, s.ID, s.Tenant)
	}
	if !ids.ValidKind(s.Account, ids.Account) {
		return fmt.Errorf("%w: subscription %q names account %q", ErrInvalid, s.ID, s.Account)
	}
	if !ids.ValidKind(s.Plan, ids.Plan) {
		return fmt.Errorf("%w: subscription %q names plan %q", ErrInvalid, s.ID, s.Plan)
	}
	if s.Seats < 0 || s.Seats > 10000 {
		return fmt.Errorf("%w: subscription %q carries %d seats", ErrInvalid, s.ID, s.Seats)
	}
	if !s.Status.Valid() {
		return fmt.Errorf("%w: subscription %q is %q", ErrInvalid, s.ID, s.Status)
	}
	if s.Anchor.IsZero() || s.Anchor.Location() != time.UTC {
		return fmt.Errorf("%w: subscription %q needs a UTC anchor", ErrInvalid, s.ID)
	}
	if s.Status == StatusCanceled && s.CanceledAt.IsZero() {
		return fmt.Errorf("%w: subscription %q is canceled without an instant", ErrInvalid, s.ID)
	}
	return nil
}

// Cycle returns the billing cycle of a subscription that contains at.
func (s Subscription) Cycle(at time.Time) (clock.Period, error) {
	period, err := clock.MonthlyCycle(s.Anchor, at)
	if err != nil {
		return clock.Period{}, err
	}
	return period, nil
}

// Line is one line of an invoice: a description, a quantity and what it came to.
type Line struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Description string       `json:"description"`
	Quantity    int64        `json:"quantity"`
	UnitPrice   money.Amount `json:"unit_price"`
	Amount      money.Amount `json:"amount"`
}

// Validate checks the line.
func (l Line) Validate(currency string) error {
	if !ids.ValidKind(l.ID, ids.Line) {
		return fmt.Errorf("%w: line id %q", ErrInvalid, l.ID)
	}
	if strings.TrimSpace(l.Kind) == "" || strings.TrimSpace(l.Description) == "" {
		return fmt.Errorf("%w: line %q needs a kind and a description", ErrInvalid, l.ID)
	}
	if l.Quantity < 0 {
		return fmt.Errorf("%w: line %q carries quantity %d", ErrInvalid, l.ID, l.Quantity)
	}
	if l.Amount.Currency != currency || l.UnitPrice.Currency != currency {
		return fmt.Errorf("%w: line %q is not in %s", ErrInvalid, l.ID, currency)
	}
	return nil
}

// Invoice is one bill for one subscription and one cycle.
type Invoice struct {
	ID           string        `json:"id"`
	Tenant       string        `json:"tenant"`
	Account      string        `json:"account"`
	Subscription string        `json:"subscription"`
	Period       clock.Period  `json:"period"`
	Issued       time.Time     `json:"issued"`
	Due          time.Time     `json:"due"`
	Status       InvoiceStatus `json:"status"`
	Currency     string        `json:"currency"`
	Lines        []Line        `json:"lines"`
	Total        money.Amount  `json:"total"`
	Paid         money.Amount  `json:"paid"`
}

// Validate checks the invoice and every line it carries.
func (i Invoice) Validate() error {
	if !ids.ValidKind(i.ID, ids.Invoice) {
		return fmt.Errorf("%w: invoice id %q", ErrInvalid, i.ID)
	}
	if !ValidTenantID(i.Tenant) || !ids.ValidKind(i.Account, ids.Account) {
		return fmt.Errorf("%w: invoice %q names tenant %q and account %q", ErrInvalid, i.ID, i.Tenant, i.Account)
	}
	if !i.Status.Valid() {
		return fmt.Errorf("%w: invoice %q is %q", ErrInvalid, i.ID, i.Status)
	}
	if err := i.Total.Validate(); err != nil {
		return fmt.Errorf("%w: invoice %q: %v", ErrInvalid, i.ID, err)
	}
	if i.Total.Currency != i.Currency {
		return fmt.Errorf("%w: invoice %q totals in %s and is billed in %s", ErrInvalid, i.ID, i.Total.Currency, i.Currency)
	}
	if !i.Due.After(i.Issued) {
		return fmt.Errorf("%w: invoice %q is due %s and issued %s", ErrInvalid, i.ID, clock.Format(i.Due), clock.Format(i.Issued))
	}
	if len(i.Lines) > 0 {
		var sum money.Amount
		for n, line := range i.Lines {
			if err := line.Validate(i.Currency); err != nil {
				return err
			}
			if n == 0 {
				sum = money.Zero(i.Currency)
			}
			next, err := sum.Add(line.Amount)
			if err != nil {
				return fmt.Errorf("%w: invoice %q: %v", ErrInvalid, i.ID, err)
			}
			sum = next
		}
		if cmp, err := sum.Cmp(i.Total); err != nil || cmp != 0 {
			return fmt.Errorf("%w: invoice %q totals %s but its lines add up to %s", ErrInvalid, i.ID, i.Total, sum)
		}
	}
	if cmp, err := i.Paid.Cmp(i.Total); err == nil && cmp > 0 {
		return fmt.Errorf("%w: invoice %q is paid %s of %s", ErrInvalid, i.ID, i.Paid, i.Total)
	}
	return nil
}

// UsageEvent is one metered quantity a customer reported.
type UsageEvent struct {
	ID           string      `json:"id"`
	Tenant       string      `json:"tenant"`
	Account      string      `json:"account"`
	Subscription string      `json:"subscription"`
	Metric       string      `json:"metric"`
	Quantity     int64       `json:"quantity"`
	At           time.Time   `json:"at"`
	Status       EventStatus `json:"status"`
}

// Validate checks the usage event.
func (e UsageEvent) Validate() error {
	if !ids.ValidKind(e.ID, ids.UsageEvent) {
		return fmt.Errorf("%w: event id %q", ErrInvalid, e.ID)
	}
	if !ValidTenantID(e.Tenant) || !ids.ValidKind(e.Account, ids.Account) {
		return fmt.Errorf("%w: event %q names tenant %q and account %q", ErrInvalid, e.ID, e.Tenant, e.Account)
	}
	if !query.ValidName(e.Metric) {
		return fmt.Errorf("%w: event %q carries metric %q", ErrInvalid, e.ID, e.Metric)
	}
	if e.Quantity < 0 {
		return fmt.Errorf("%w: event %q carries quantity %d", ErrInvalid, e.ID, e.Quantity)
	}
	if !e.Status.Valid() {
		return fmt.Errorf("%w: event %q is %q", ErrInvalid, e.ID, e.Status)
	}
	if e.At.IsZero() || e.At.Location() != time.UTC {
		return fmt.Errorf("%w: event %q needs a UTC instant", ErrInvalid, e.ID)
	}
	return nil
}

// Credit is money an account is owed, applied to the next invoice.
type Credit struct {
	ID      string       `json:"id"`
	Tenant  string       `json:"tenant"`
	Account string       `json:"account"`
	Amount  money.Amount `json:"amount"`
	Reason  string       `json:"reason"`
	At      time.Time    `json:"at"`
}

// Validate checks the credit.
func (c Credit) Validate() error {
	if !ids.ValidKind(c.ID, ids.Credit) {
		return fmt.Errorf("%w: credit id %q", ErrInvalid, c.ID)
	}
	if !ValidTenantID(c.Tenant) || !ids.ValidKind(c.Account, ids.Account) {
		return fmt.Errorf("%w: credit %q names tenant %q and account %q", ErrInvalid, c.ID, c.Tenant, c.Account)
	}
	if _, err := c.Amount.NonNegative(); err != nil {
		return fmt.Errorf("%w: credit %q: %v", ErrInvalid, c.ID, err)
	}
	if strings.TrimSpace(c.Reason) == "" {
		return fmt.Errorf("%w: credit %q has no reason", ErrInvalid, c.ID)
	}
	return nil
}

// Payment is money received against an invoice.
type Payment struct {
	ID      string       `json:"id"`
	Tenant  string       `json:"tenant"`
	Invoice string       `json:"invoice"`
	Amount  money.Amount `json:"amount"`
	At      time.Time    `json:"at"`
}

// Validate checks the payment.
func (p Payment) Validate() error {
	if !ids.ValidKind(p.ID, ids.Payment) {
		return fmt.Errorf("%w: payment id %q", ErrInvalid, p.ID)
	}
	if !ValidTenantID(p.Tenant) || !ids.ValidKind(p.Invoice, ids.Invoice) {
		return fmt.Errorf("%w: payment %q names tenant %q and invoice %q", ErrInvalid, p.ID, p.Tenant, p.Invoice)
	}
	if _, err := p.Amount.NonNegative(); err != nil {
		return fmt.Errorf("%w: payment %q: %v", ErrInvalid, p.ID, err)
	}
	return nil
}

// sortedByID returns the records of a map in id order, which is the order a listing pages through.
func sortedByID[T any](records map[string]T, id func(T) string) []T {
	out := make([]T, 0, len(records))
	for _, record := range records {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return id(out[i]) < id(out[j]) })
	return out
}

// limitsOf returns the caps a store enforces.
func limitsOf(limits config.Limits) config.Limits { return limits }

// sqlrunValue converts a text column.
func textValue(value string) sqlrun.Value { return sqlrun.TextValue(value) }

// numberValue converts a number column.
func numberValue(value int64) sqlrun.Value { return sqlrun.NumberValue(value) }
