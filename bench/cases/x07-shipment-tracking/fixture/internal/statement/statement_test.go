package statement_test

import (
	"testing"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/statement"
)

func partner() identity.Partner {
	return identity.Partner{Tenant: "acme", ID: "shop", Name: "Shop"}
}

func items() []statement.Item {
	return []statement.Item{
		{Shipment: "shp_00000000000000000000000000000001", Carrier: "fastco", Service: "ground", Lane: "mad>bcn", Grams: 1500, Base: 698, Fuel: 59, At: time.Date(2024, 5, 3, 9, 0, 0, 0, time.UTC)},
		{Shipment: "shp_00000000000000000000000000000002", Carrier: "fastco", Service: "express", Lane: "mad>bcn", Grams: 4200, Base: 2687, Fuel: 296, At: time.Date(2024, 5, 17, 14, 0, 0, 0, time.UTC)},
		{Shipment: "shp_00000000000000000000000000000003", Carrier: "fastco", Service: "ground", Lane: "mad>bcn", Grams: 800, Base: 399, Fuel: 34, At: time.Date(2024, 5, 29, 7, 30, 0, 0, time.UTC)},
	}
}

func TestBuildTotalsAddUp(t *testing.T) {
	built, err := statement.Build(partner(), clock.Period{Year: 2024, Month: 5}, items(), 1900,
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(built.Lines) != 3 {
		t.Fatalf("lines = %d", len(built.Lines))
	}
	if built.Total != built.Net+built.Tax {
		t.Fatalf("total %s != net %s + tax %s", built.Total, built.Net, built.Tax)
	}
	var lineTotal, fuelTotal, taxTotal money.Amount
	for _, line := range built.Lines {
		lineTotal += line.Total
		fuelTotal += line.Fuel
		taxTotal += line.Tax
	}
	if lineTotal != built.Total {
		t.Fatalf("lines sum to %s, statement says %s", lineTotal, built.Total)
	}
	if fuelTotal != built.Fuel || taxTotal != built.Tax {
		t.Fatalf("allocated fuel %s tax %s against %s and %s", fuelTotal, taxTotal, built.Fuel, built.Tax)
	}
	// No line is charged more fuel or tax than its freight.
	for _, line := range built.Lines {
		if line.Fuel > line.Base || line.Tax > line.Base {
			t.Fatalf("line %s carries more surcharge than freight", line.Shipment)
		}
	}
}

func TestBuildRefusesAnEmptyStatement(t *testing.T) {
	if _, err := statement.Build(partner(), clock.Period{Year: 2024, Month: 5}, nil, 1900, time.Now()); err == nil {
		t.Fatal("an empty statement was built")
	}
}

func TestCloseableNeedsAFinishedPeriodInsideTheWindow(t *testing.T) {
	now := time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC)
	if err := statement.CheckCloseable(clock.Period{Year: 2024, Month: 5}, now, 6); err != nil {
		t.Fatalf("May is closeable in June: %v", err)
	}
	if err := statement.CheckCloseable(clock.Period{Year: 2024, Month: 6}, now, 6); err == nil {
		t.Fatal("a running period was closeable")
	}
	if err := statement.CheckCloseable(clock.Period{Year: 2023, Month: 11}, now, 6); err == nil {
		t.Fatal("a period outside the window was closeable")
	}
}

func TestLineCountAndSummary(t *testing.T) {
	built, err := statement.Build(partner(), clock.Period{Year: 2024, Month: 5}, items(), 1900,
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if built.LineCount() != 3 {
		t.Fatalf("LineCount = %d", built.LineCount())
	}
	if built.Summary() == "" {
		t.Fatal("the statement has no summary")
	}
}
