package reports

import (
	"errors"

	"example.com/billing/internal/query"
)

// Errors a report reports.
var (
	// ErrNoReport is returned for a report the registry does not carry.
	ErrNoReport = errors.New("reports: no such report")
	// ErrNoTenant is returned for a report run without a tenant. It is the refusal behind
	// every empty answer: a report that ran with no tenant would read every tenant.
	ErrNoTenant = errors.New("reports: the report needs a tenant")
	// ErrLimit is returned for a row bound a deployment does not serve.
	ErrLimit = errors.New("reports: the row bound is not positive")
)

// StatusValues are the usage event statuses the usage report accepts, in the order the catalog lists
// them. A status outside this list is refused rather than run as text.
func StatusValues() []string {
	return []string{"recorded", "billed", "archived"}
}

// InvoiceStatusValues are the invoice statuses the revenue report accepts.
func InvoiceStatusValues() []string {
	return []string{"draft", "open", "paid", "void"}
}

// OpenInvoiceStatusValues are the statuses the aging report accepts: the ones that can still be owed.
func OpenInvoiceStatusValues() []string {
	return []string{"open", "draft", "void"}
}

// Usage is the usage report: one row per metered event, narrowed by status, metric or account.
func Usage() Definition {
	return Definition{
		Name:  "usage",
		Title: "Metered usage",
		Table: "usage_events",
		Columns: []string{
			"id", "account", "subscription", "metric", "quantity", "status", "day",
		},
		Filters: filters(
			FilterSpec{Param: "status", Column: "status", Op: "=", Accepted: StatusValues()},
			FilterSpec{Param: "metric", Column: "metric", Op: "="},
			FilterSpec{Param: "account", Column: "account", Op: "="},
			FilterSpec{Param: "subscription", Column: "subscription", Op: "="},
		),
		Order: "id",
		Limit: 500,
	}
}

// Revenue is the revenue report: the issued invoices of a tenant, one row per invoice.
func Revenue() Definition {
	return Definition{
		Name:  "revenue",
		Title: "Issued revenue",
		Table: "invoices",
		Columns: []string{
			"id", "account", "subscription", "status", "currency", "total", "paid",
			"period_start", "period_end", "issued_day",
		},
		Filters: filters(
			FilterSpec{Param: "status", Column: "status", Op: "=", Accepted: InvoiceStatusValues()},
			FilterSpec{Param: "account", Column: "account", Op: "="},
			FilterSpec{Param: "subscription", Column: "subscription", Op: "="},
		),
		Order: "issued_day",
		Desc:  true,
		Limit: 500,
	}
}

// Aging is the aging report: the invoices a tenant still owes, oldest first.
func Aging() Definition {
	return Definition{
		Name:  "aging",
		Title: "Outstanding invoices",
		Table: "invoices",
		Columns: []string{
			"id", "account", "status", "currency", "total", "paid", "due_day",
		},
		Filters: filters(
			FilterSpec{Param: "status", Column: "status", Op: "=", Accepted: OpenInvoiceStatusValues()},
			FilterSpec{Param: "account", Column: "account", Op: "="},
		),
		Order: "due_day",
		Limit: 500,
	}
}

// Subscriptions is the subscription report: one row per subscription with the price of its seats.
func Subscriptions() Definition {
	return Definition{
		Name:  "subscriptions",
		Title: "Subscriptions",
		Table: "subscriptions",
		Columns: []string{
			"id", "account", "plan_code", "seats", "status", "seat_price", "base_price", "anchor_day",
		},
		Filters: filters(
			FilterSpec{Param: "status", Column: "status", Op: "=", Accepted: []string{"active", "past_due", "canceled"}},
			FilterSpec{Param: "plan", Column: "plan_code", Op: "="},
			FilterSpec{Param: "account", Column: "account", Op: "="},
		),
		Order: "id",
		Limit: 500,
	}
}

// Default returns the reports a deployment serves, in the order the catalog lists them.
func Default() []Definition {
	return []Definition{Usage(), Revenue(), Aging(), Subscriptions()}
}

// NewDefaultRegistry returns the registry of the default reports.
func NewDefaultRegistry() (*Registry, error) {
	return NewRegistry(Default()...)
}

// FilterSpec is a filter as a definition writes it: the parameter, the column, the operator and the
// values it accepts. Definition.definition turns it into the query.Filter the statement is built with.
type FilterSpec struct {
	Param    string
	Column   string
	Op       string
	Accepted []string
}

// filters turns the specs a definition writes into the query package's filters. It is the one place
// a declared filter becomes the filter the statement is built with, so a report cannot declare one
// shape and run another.
func filters(specs ...FilterSpec) []query.Filter {
	out := make([]query.Filter, 0, len(specs))
	for _, spec := range specs {
		out = append(out, query.Filter{
			Param: spec.Param, Column: spec.Column, Op: spec.Op, Values: spec.Accepted,
		})
	}
	return out
}
