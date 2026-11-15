package query

import (
	"errors"
	"strings"
	"testing"
)

func definition() Definition {
	return Definition{
		Table: "usage_events",
		Columns: []string{
			"usage_id", "usage_account", "usage_metric", "usage_quantity",
		},
		Params: []Param{
			{Name: "account", Column: "usage_account", Kind: KindText},
			{Name: "metric", Column: "usage_metric", Kind: KindEnum, Enum: []string{"api_call", "seat"}},
		},
		Sorts:        map[string]string{"occurred": "usage_id", "account": "usage_account"},
		MaxLimit:     20,
		DefaultLimit: 5,
	}
}

func TestBuilderRefusesWhatTheDefinitionDoesNotDeclare(t *testing.T) {
	builder, err := New(definition())
	if err != nil {
		t.Fatalf("builder: %v", err)
	}
	if err := builder.Filter("status", "open"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an undeclared filter was accepted: %v", err)
	}
	if err := builder.Filter("metric", "storage_gb"); !errors.Is(err, ErrValue) {
		t.Fatalf("a value outside the declared set was accepted: %v", err)
	}
	if err := builder.Sort("usage_account", false); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an undeclared order was accepted: %v", err)
	}
	if err := builder.Limit(21); !errors.Is(err, ErrLimit) {
		t.Fatalf("a page above the cap was accepted: %v", err)
	}
	if err := builder.Limit(0); !errors.Is(err, ErrLimit) {
		t.Fatalf("a page of zero rows was accepted: %v", err)
	}
}

func TestStatementCarriesPlaceholdersAndNotValues(t *testing.T) {
	builder, err := New(definition())
	if err != nil {
		t.Fatalf("builder: %v", err)
	}
	if err := builder.Filter("account", "team-api"); err != nil {
		t.Fatalf("filter: %v", err)
	}
	if err := builder.Filter("metric", "api_call"); err != nil {
		t.Fatalf("filter: %v", err)
	}
	if err := builder.Sort("account", true); err != nil {
		t.Fatalf("sort: %v", err)
	}
	if err := builder.Limit(10); err != nil {
		t.Fatalf("limit: %v", err)
	}
	statement, args := builder.Statement()
	if strings.Contains(statement, "team-api") || strings.Contains(statement, "api_call") {
		t.Fatalf("a value reached the statement: %s", statement)
	}
	if strings.Count(statement, "?") != 3 {
		t.Fatalf("the statement carries %d placeholders: %s", strings.Count(statement, "?"), statement)
	}
	if len(args) != 3 {
		t.Fatalf("the statement carries %d arguments", len(args))
	}
	if !strings.Contains(statement, "usage_account = ?") {
		t.Fatalf("the declared column did not reach the statement: %s", statement)
	}
	if !strings.Contains(statement, "ORDER BY usage_account DESC") {
		t.Fatalf("the declared order did not reach the statement: %s", statement)
	}
	criteria := builder.Criteria()
	if len(criteria.Clauses) != 2 || criteria.Limit != 10 {
		t.Fatalf("criteria = %+v", criteria)
	}
}

func TestDefinitionIsValidated(t *testing.T) {
	cases := map[string]func(*Definition){
		"table with a space":    func(d *Definition) { d.Table = "usage events" },
		"no columns":            func(d *Definition) { d.Columns = nil },
		"column twice":          func(d *Definition) { d.Columns = append(d.Columns, "usage_id") },
		"filter on a stranger":  func(d *Definition) { d.Params[0].Column = "usage_secret" },
		"enum without values":   func(d *Definition) { d.Params[1].Enum = nil },
		"sort on a stranger":    func(d *Definition) { d.Sorts["account"] = "usage_secret" },
		"default above the max": func(d *Definition) { d.DefaultLimit = 21 },
	}
	for name, mutate := range cases {
		broken := definition()
		mutate(&broken)
		if err := broken.Validate(); err == nil {
			t.Fatalf("%s was accepted: %+v", name, broken)
		}
	}
}
