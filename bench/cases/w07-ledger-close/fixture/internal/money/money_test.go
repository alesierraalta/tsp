package money

import (
	"errors"
	"math"
	"testing"
)

func TestParseReadsDecimalAmounts(t *testing.T) {
	cases := []struct {
		raw  string
		want Amount
	}{
		{"1.15", 115},
		{"45", 4500},
		{"-0.05", -5},
		{"+2.5", 250},
		{"0.00", 0},
	}
	for _, c := range cases {
		got, err := Parse(c.raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.raw, err)
		}
		if got != c.want {
			t.Fatalf("Parse(%q) = %s, want %s", c.raw, got, c.want)
		}
	}
}

func TestParseRefusesTheFormsItDoesNotWrite(t *testing.T) {
	for _, raw := range []string{"", " 1.00", "1.00 ", "1.234", "1,000.00", "1e3", ".50", "1.", "twelve", "92233720368547758.08"} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("Parse(%q) was accepted", raw)
		}
	}
}

func TestStringRoundTripsEveryAmount(t *testing.T) {
	for _, amount := range []Amount{0, 1, -1, 115, -4500, Amount(math.MaxInt64), Amount(math.MinInt64)} {
		rendered := amount.String()
		parsed, err := Parse(rendered)
		if err != nil {
			t.Fatalf("Parse(%q) of %s: %v", rendered, amount, err)
		}
		if parsed != amount {
			t.Fatalf("%s rendered as %q and parsed back as %s", amount, rendered, parsed)
		}
	}
}

func TestAddRefusesToLeaveTheRange(t *testing.T) {
	if _, err := Add(Amount(math.MaxInt64), 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Add above the range: %v", err)
	}
	if _, err := Add(Amount(math.MinInt64), -1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Add below the range: %v", err)
	}
	if sum, err := Add(115, 15); err != nil || sum != 130 {
		t.Fatalf("Add(115, 15) = %s, %v", sum, err)
	}
}

func TestMulRefusesAnOverflow(t *testing.T) {
	if _, err := Mul(Amount(math.MaxInt64), 2); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Mul above the range: %v", err)
	}
	if _, err := Mul(Amount(math.MinInt64), -1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Mul of the most negative amount by -1: %v", err)
	}
	if product, err := Mul(100, 3); err != nil || product != 300 {
		t.Fatalf("Mul(100, 3) = %s, %v", product, err)
	}
}

func TestRoundRatioRoundsHalfToEven(t *testing.T) {
	cases := []struct {
		amount, numerator, denominator int64
		want                           Amount
	}{
		{5, 1, 2, 2},             // 2.5 becomes 2
		{7, 1, 2, 4},             // 3.5 becomes 4
		{-5, 1, 2, -2},           // -2.5 becomes -2
		{-7, 1, 2, -4},           // -3.5 becomes -4
		{10, 1, 3, 3},            // 3.33
		{1000, 2500, 10000, 250}, // a quarter
	}
	for _, c := range cases {
		got, err := RoundRatio(Amount(c.amount), c.numerator, c.denominator)
		if err != nil {
			t.Fatalf("RoundRatio(%d, %d, %d): %v", c.amount, c.numerator, c.denominator, err)
		}
		if int64(got) != int64(c.want) {
			t.Fatalf("RoundRatio(%d, %d, %d) = %s, want %s", c.amount, c.numerator, c.denominator, got, c.want)
		}
	}
}

func TestRoundRatioRefusesADenominatorItCannotUse(t *testing.T) {
	for _, denominator := range []int64{0, -1} {
		if _, err := RoundRatio(100, 1, denominator); !errors.Is(err, ErrRounding) {
			t.Fatalf("RoundRatio with denominator %d: %v", denominator, err)
		}
	}
}
