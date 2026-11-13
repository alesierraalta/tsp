package proration

import (
	"testing"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
)

// april is the cycle the tests price against: 01 Apr 2024 to 01 May 2024, thirty UTC days.
func april(t *testing.T) clock.Period {
	t.Helper()
	period, err := clock.New(
		time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	return period
}

// eur returns an amount in EUR.
func eur(t *testing.T, minor int64) money.Amount {
	t.Helper()
	amount, err := money.New(minor, "EUR")
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	return amount
}

func TestRemainingCountsWholeDays(t *testing.T) {
	period := april(t)
	if got := period.Days(); got != 30 {
		t.Fatalf("april covers %d days", got)
	}
	factor, err := Remaining(period, time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if factor.Numer != 7 || factor.Denom != 30 {
		t.Fatalf("factor = %+v, want 7/30", factor)
	}
	whole, err := Remaining(period, time.Date(2024, 4, 1, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if !whole.Whole() {
		t.Fatalf("the first day of the cycle is not the whole cycle: %+v", whole)
	}
	if _, err := Remaining(period, time.Date(2024, 5, 2, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("a change outside the cycle was accepted")
	}
}

func TestMidCycleIsTheSharedSplitOfTheCycle(t *testing.T) {
	period := april(t)
	at := time.Date(2024, 4, 24, 10, 0, 0, 0, time.UTC) // 7 of 30 days left
	got, err := MidCycle(eur(t, 1001), period, at)
	if err != nil {
		t.Fatalf("mid cycle: %v", err)
	}
	if got.Minor != 234 {
		t.Fatalf("mid cycle = %d minor, want 234", got.Minor)
	}
	elapsed, err := ElapsedPart(eur(t, 1001), period, at)
	if err != nil {
		t.Fatalf("elapsed: %v", err)
	}
	if elapsed.Minor != 767 {
		t.Fatalf("elapsed = %d minor, want 767", elapsed.Minor)
	}
	// Where neither part falls on a half minor unit the two parts add up exactly.
	if got.Minor+elapsed.Minor != 1001 {
		t.Fatalf("%d + %d does not add up to 1001", got.Minor, elapsed.Minor)
	}
}

func TestFactorOfKeepsTheCurrency(t *testing.T) {
	period := april(t)
	factor, err := Remaining(period, time.Date(2024, 4, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	part, err := factor.Of(eur(t, 5000))
	if err != nil {
		t.Fatalf("of: %v", err)
	}
	if part.Minor != 1167 || part.Currency != "EUR" {
		t.Fatalf("part = %+v, want 1167 EUR", part)
	}
	if _, err := factor.Of(money.Amount{Minor: 5000, Currency: "EUR"}); err != nil {
		t.Fatalf("of: %v", err)
	}
}

func TestSeatCreditIsTheSharedSplitOfTheRemovedSeats(t *testing.T) {
	period := april(t)
	change := SeatChange{
		Pricing: Pricing{SeatPrice: eur(t, 1001)},
		From:    3,
		To:      2,
		Period:  period,
		At:      time.Date(2024, 4, 16, 0, 0, 0, 0, time.UTC),
	}
	credit, err := change.Credit()
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if credit.Minor != 501 {
		t.Fatalf("credit = %d minor, want 501", credit.Minor)
	}
	note, err := SeatCreditNote(change)
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	if note.Amount.Minor != credit.Minor || note.Reason == "" {
		t.Fatalf("note = %+v", note)
	}
}

func TestSeatChargeIsWholeAtTheStartOfACycle(t *testing.T) {
	period := april(t)
	change := SeatChange{
		Pricing: Pricing{SeatPrice: eur(t, 1000), IncludedSeats: 2},
		From:    5,
		To:      8,
		Period:  period,
		At:      time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC),
	}
	charge, err := change.Charge()
	if err != nil {
		t.Fatalf("charge: %v", err)
	}
	// Three billable seats added (five carried above the two included, eight carried above them),
	// the whole cycle to serve: the charge is the whole price of the added seats.
	if charge.Minor != 3000 {
		t.Fatalf("charge = %d minor, want 3000", charge.Minor)
	}
	description, quantity := change.Line(charge)
	if quantity != 3 || description == "" {
		t.Fatalf("line = %q q=%d", description, quantity)
	}
}

func TestSeatChargeOfNoBillableSeatsIsZero(t *testing.T) {
	period := april(t)
	change := SeatChange{
		Pricing: Pricing{SeatPrice: eur(t, 1000), IncludedSeats: 10},
		From:    3,
		To:      5,
		Period:  period,
		At:      time.Date(2024, 4, 20, 0, 0, 0, 0, time.UTC),
	}
	charge, err := change.Charge()
	if err != nil {
		t.Fatalf("charge: %v", err)
	}
	if charge.Minor != 0 {
		t.Fatalf("charge = %d minor, want nothing above the included seats", charge.Minor)
	}
}

func TestSeatChangeValidation(t *testing.T) {
	period := april(t)
	base := SeatChange{
		Pricing: Pricing{SeatPrice: eur(t, 1000)},
		From:    4,
		To:      4,
		Period:  period,
		At:      time.Date(2024, 4, 10, 0, 0, 0, 0, time.UTC),
	}
	if err := base.Validate(); err == nil {
		t.Fatal("a change that changes nothing was accepted")
	}
	base.To = 6
	if err := base.Validate(); err != nil {
		t.Fatalf("a valid change was refused: %v", err)
	}
	base.From = -1
	if err := base.Validate(); err == nil {
		t.Fatal("a negative seat count was accepted")
	}
}

func TestAddOnIsPricedOverTheDaysItCovers(t *testing.T) {
	period := april(t)
	addOn := AddOn{
		Name:     "audit export",
		Price:    eur(t, 3000),
		StartsAt: time.Date(2024, 4, 16, 0, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	charge, err := addOn.Charge(period)
	if err != nil {
		t.Fatalf("charge: %v", err)
	}
	if charge.Minor != 1500 {
		t.Fatalf("charge = %d minor, want half of the cycle", charge.Minor)
	}
}

func TestTrialAndRetainerUseTheSharedSplit(t *testing.T) {
	period := april(t)
	trial := Trial{
		Period:      period,
		Price:       eur(t, 4900),
		EndsAt:      time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		ConvertedAt: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
	}
	charge, err := trial.Charge()
	if err != nil {
		t.Fatalf("trial: %v", err)
	}
	if charge.Minor != 4900 {
		t.Fatalf("trial charge = %d minor, want the whole price", charge.Minor)
	}
	retainer := Retainer{Monthly: eur(t, 2000), Period: period,
		HeldAt: time.Date(2024, 4, 24, 0, 0, 0, 0, time.UTC)}
	held, err := retainer.Charge()
	if err != nil {
		t.Fatalf("retainer: %v", err)
	}
	if held.Minor != 467 {
		t.Fatalf("retainer = %d minor, want 467", held.Minor)
	}
}

func TestNotesCreditTheDaysNotServed(t *testing.T) {
	period := april(t)
	notes, err := Notes(eur(t, 3000), period, time.Date(2024, 4, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("notes: %v", err)
	}
	if len(notes) != 1 || notes[0].Amount.Minor != 1500 {
		t.Fatalf("notes = %+v", notes)
	}
	if _, err := Notes(eur(t, 3000), period, time.Date(2024, 4, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("notes: %v", err)
	}
}
