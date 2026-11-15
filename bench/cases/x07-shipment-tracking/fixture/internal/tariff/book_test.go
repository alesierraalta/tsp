package tariff_test

import (
	"testing"

	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/tariff"
)

func book(t *testing.T) *tariff.Book {
	t.Helper()
	b, err := tariff.NewBook(1900)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	for _, service := range tariff.DefaultServices() {
		if _, err := b.AddService(service); err != nil {
			t.Fatalf("service %q: %v", service.Name, err)
		}
	}
	return b
}

func TestRateBillsWholeKilograms(t *testing.T) {
	b := book(t)
	quote, err := b.Rate("ground", 1500)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	// 1500 grams is two billed kilograms: the part of a kilogram is billed as a whole one.
	if quote.Kilograms != 2 {
		t.Fatalf("kilograms = %d", quote.Kilograms)
	}
	if quote.Base != 698 {
		t.Fatalf("base = %s, want 6.98", quote.Base)
	}
	// The fuel surcharge is 8.5% of the base, rounded half to even: 59.33 minor units become 59.
	if quote.Fuel != 59 {
		t.Fatalf("fuel = %s, want 0.59", quote.Fuel)
	}
	if got := quote.Base + quote.Fuel; got != quote.Total {
		t.Fatalf("total %s != %s", quote.Total, got)
	}
}

func TestRateAppliesTheMinimumCharge(t *testing.T) {
	b := book(t)
	quote, err := b.Rate("freight", 50000)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	if quote.Base < 4999 {
		t.Fatalf("base %s is below the minimum charge", quote.Base)
	}
}

func TestRateRefusesAnUnpricedWeight(t *testing.T) {
	b := book(t)
	if _, err := b.Rate("ground", 2000000); err == nil {
		t.Fatal("a weight above the highest band was priced")
	}
	if _, err := b.Rate("hovercraft", 1000); err == nil {
		t.Fatal("an unknown service was priced")
	}
	if _, err := b.Rate("ground", 0); err == nil {
		t.Fatal("a zero weight was priced")
	}
}

func TestBookRefusesBadRates(t *testing.T) {
	b, err := tariff.NewBook(10001)
	if err == nil || b != nil {
		t.Fatal("a tax rate above 100% was accepted")
	}
	strict, err := tariff.NewBook(0)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := strict.AddService(tariff.Service{Name: "bad"}); err == nil {
		t.Fatal("a service with no bands was accepted")
	}
	if _, err := strict.AddService(tariff.Service{Name: "bad", Bands: []tariff.Band{
		{MaxGrams: 1000, PerKg: money.Amount(100)},
		{MaxGrams: 1000, PerKg: money.Amount(200)},
	}}); err == nil {
		t.Fatal("a repeated band ceiling was accepted")
	}
}

func TestSummariseAddsQuotes(t *testing.T) {
	b := book(t)
	first, err := b.Rate("ground", 1500)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	second, err := b.Rate("express", 4200)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	summary, err := tariff.Summarise([]tariff.Quote{first, second})
	if err != nil {
		t.Fatalf("summarise: %v", err)
	}
	if summary.Lines != 2 || summary.Net != first.Total+second.Total {
		t.Fatalf("summary = %+v", summary)
	}
	tax, err := b.Tax(summary.Net)
	if err != nil {
		t.Fatalf("tax: %v", err)
	}
	if tax <= 0 || tax > summary.Net {
		t.Fatalf("tax = %s on a net of %s", tax, summary.Net)
	}
}
