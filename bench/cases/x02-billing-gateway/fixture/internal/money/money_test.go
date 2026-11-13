package money

import (
	"errors"
	"testing"
)

func TestNewNormalisesTheCurrency(t *testing.T) {
	amount, err := New(1234, "eur")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if amount.Currency != "EUR" || amount.Minor != 1234 {
		t.Fatalf("amount = %+v", amount)
	}
	if _, err := New(1, "EURO"); !errors.Is(err, ErrCurrency) {
		t.Fatalf("a four-letter currency was accepted: %v", err)
	}
	if _, err := New(1, ""); !errors.Is(err, ErrCurrency) {
		t.Fatalf("an empty currency was accepted: %v", err)
	}
}

func TestAddKeepsTheCurrencyAndRefusesAMismatch(t *testing.T) {
	eur, _ := New(1000, "EUR")
	usd, _ := New(500, "USD")
	if _, err := eur.Add(usd); !errors.Is(err, ErrMismatch) {
		t.Fatalf("adding two currencies: %v", err)
	}
	sum, err := eur.Add(eur)
	if err != nil || sum.Minor != 2000 || sum.Currency != "EUR" {
		t.Fatalf("sum = %+v err=%v", sum, err)
	}
	if _, err := eur.Add(Amount{Minor: maxInt64, Currency: "EUR"}); !errors.Is(err, ErrOverflow) {
		t.Fatalf("an overflowing sum was accepted: %v", err)
	}
}

func TestNegAndAbs(t *testing.T) {
	amount, _ := New(-1500, "EUR")
	absolute, err := amount.Abs()
	if err != nil || absolute.Minor != 1500 {
		t.Fatalf("abs = %+v err=%v", absolute, err)
	}
	negated, err := absolute.Neg()
	if err != nil || negated.Minor != -1500 {
		t.Fatalf("neg = %+v err=%v", negated, err)
	}
	if _, err := negated.NonNegative(); !errors.Is(err, ErrNegative) {
		t.Fatalf("a negative amount passed NonNegative: %v", err)
	}
}

func TestSplitAddsUpToTheTotal(t *testing.T) {
	shares, err := Split(1000, 3)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	var sum int64
	for _, share := range shares {
		sum += share
	}
	if sum != 1000 {
		t.Fatalf("shares %v add up to %d", shares, sum)
	}
	if shares[0] != 334 || shares[1] != 333 || shares[2] != 333 {
		t.Fatalf("shares = %v, want the remainder on the first share", shares)
	}
	if _, err := Split(1000, 0); err == nil {
		t.Fatal("splitting over no parts was accepted")
	}
}

func TestSplitEvenlyCarriesTheCurrency(t *testing.T) {
	total, _ := New(101, "EUR")
	shares, err := SplitEvenly(total, 4)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	back, err := SumShares(shares)
	if err != nil {
		t.Fatalf("sum: %v", err)
	}
	if back.Minor != total.Minor || back.Currency != "EUR" {
		t.Fatalf("shares add up to %+v, want %+v", back, total)
	}
}

func TestProrateRoundsHalfUpAndStaysInIntegers(t *testing.T) {
	cases := []struct {
		amount, numer, denom, want int64
	}{
		{1001, 15, 30, 501},
		{1000, 7, 30, 233},
		{5000, 7, 30, 1167},
		{1000, 30, 30, 1000},
		{1000, 0, 30, 0},
		{-1001, 15, 30, -501},
	}
	for _, c := range cases {
		got, err := Prorate(c.amount, c.numer, c.denom)
		if err != nil {
			t.Fatalf("prorate(%d, %d, %d): %v", c.amount, c.numer, c.denom, err)
		}
		if got != c.want {
			t.Errorf("prorate(%d, %d, %d) = %d, want %d", c.amount, c.numer, c.denom, got, c.want)
		}
	}
	if _, err := Prorate(1000, 31, 30); err == nil {
		t.Fatal("a factor above the whole cycle was accepted")
	}
	if _, err := Prorate(1000, 1, 0); err == nil {
		t.Fatal("a zero denominator was accepted")
	}
}

func TestRoundToNearest(t *testing.T) {
	cases := []struct{ total, step, want int64 }{
		{1234, 5, 1235},
		{1232, 5, 1230},
		{1230, 5, 1230},
	}
	for _, c := range cases {
		got, err := RoundToNearest(c.total, c.step)
		if err != nil || got != c.want {
			t.Errorf("round(%d, %d) = %d err=%v, want %d", c.total, c.step, got, err, c.want)
		}
	}
}

func TestAllocateGivesTheRemainderToTheEarlyWeights(t *testing.T) {
	shares, err := Allocate(100, []int64{1, 1, 1})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if shares[0] != 34 || shares[1] != 33 || shares[2] != 33 {
		t.Fatalf("shares = %v", shares)
	}
	zero, err := Allocate(100, []int64{0, 2, 2})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if zero[0] != 0 || zero[1]+zero[2] != 100 {
		t.Fatalf("shares = %v", zero)
	}
}

func TestParseAndFormat(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{"12.34 EUR", 1234},
		{"12,34 EUR", 1234},
		{"0.05 eur", 5},
		{"-1.00 EUR", -100},
		{"1500 JPY", 1500},
	}
	for _, c := range cases {
		amount, err := Parse(c.text)
		if err != nil {
			t.Errorf("parse(%q): %v", c.text, err)
			continue
		}
		if amount.Minor != c.want {
			t.Errorf("parse(%q) = %d, want %d", c.text, amount.Minor, c.want)
		}
	}
	if _, err := Parse("12.345 EUR"); err == nil {
		t.Fatal("an amount with too many decimals was accepted")
	}
	if _, err := Parse("EUR"); err == nil {
		t.Fatal("an amount without a currency was accepted")
	}
	if got := formatMinor(123456, "EUR"); got != "1,234.56 EUR" {
		t.Fatalf("format = %q", got)
	}
	if got := formatMinor(-123456, "EUR"); got != "-1,234.56 EUR" {
		t.Fatalf("format = %q", got)
	}
}

func TestMarshalRefusesABareNumber(t *testing.T) {
	amount, _ := New(1234, "EUR")
	encoded, err := amount.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Amount
	if err := back.UnmarshalJSON(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Minor != amount.Minor || back.Currency != amount.Currency {
		t.Fatalf("round trip = %+v, want %+v", back, amount)
	}
	if err := back.UnmarshalJSON([]byte("1234")); err == nil {
		t.Fatal("a bare number was accepted as an amount")
	}
}
