package money_test

import (
	"errors"
	"testing"

	"example.com/shiptrack/internal/money"
)

func TestParseAndRender(t *testing.T) {
	cases := []struct {
		raw  string
		want money.Amount
		text string
	}{
		{"1.15", 115, "1.15"},
		{"45", 4500, "45.00"},
		{"-0.05", -5, "-0.05"},
		{"0", 0, "0.00"},
	}
	for _, c := range cases {
		got, err := money.Parse(c.raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.raw, err)
		}
		if got != c.want {
			t.Fatalf("Parse(%q) = %d, want %d", c.raw, got, c.want)
		}
		if got.String() != c.text {
			t.Fatalf("%d.String() = %q, want %q", got, got.String(), c.text)
		}
	}
	for _, raw := range []string{"1.155", " 1.00", "1,000.00", "1e3", "", "-"} {
		if _, err := money.Parse(raw); err == nil {
			t.Fatalf("Parse(%q) accepted a value it must refuse", raw)
		}
	}
}

func TestApplyBasisPointsRoundsHalfToEven(t *testing.T) {
	cases := []struct {
		amount money.Amount
		bp     int32
		want   money.Amount
	}{
		{250, 100, 2},     // 2.5 minor units round to the even 2
		{350, 100, 4},     // 3.5 -> 4
		{1000, 1250, 125}, // 12.5% of 10.00 is 1.25
		{-250, 100, -2},
		{-350, 100, -4},
	}
	// The first case is checked by hand below: 250 * 100 / 10000 = 2.5, and half to even is 2.
	for i, c := range cases {
		got, err := money.ApplyBasisPoints(c.amount, c.bp)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got != c.want {
			t.Fatalf("case %d: %s at %d bp = %s, want %s", i, c.amount, c.bp, got, c.want)
		}
	}
}

func TestSplitAddsBackToTheTotal(t *testing.T) {
	shares, err := money.Split(1000, []int64{1, 1, 1})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	var sum money.Amount
	for _, share := range shares {
		sum += share
	}
	if sum != 1000 {
		t.Fatalf("shares %v sum to %s", shares, sum)
	}
	if len(shares) != 3 {
		t.Fatalf("shares = %v", shares)
	}
}

func TestSplitCappedRespectsCeilings(t *testing.T) {
	// Ten minor units across three lines whose ceilings cannot take an even split.
	shares, err := money.SplitCapped(10, []int64{1, 1, 1}, []money.Amount{2, 8, 8})
	if err != nil {
		t.Fatalf("capped split: %v", err)
	}
	var sum money.Amount
	for i, share := range shares {
		if share.Negative() {
			t.Fatalf("share %d is negative", i)
		}
		sum += share
	}
	if sum != 10 {
		t.Fatalf("shares %v sum to %s", shares, sum)
	}
	if shares[0] > 2 {
		t.Fatalf("share 0 = %s is above its ceiling", shares[0])
	}
	// A total the ceilings cannot hold is refused, not truncated.
	if _, err := money.SplitCapped(100, []int64{1, 1}, []money.Amount{10, 10}); !errors.Is(err, money.ErrAllocate) {
		t.Fatalf("an impossible allocation returned %v", err)
	}
}

func TestCheckedArithmetic(t *testing.T) {
	if _, err := money.Add(money.Amount(1<<62), money.Amount(1<<62)); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("overflow was not refused: %v", err)
	}
	if _, err := money.MulInt(money.Amount(1<<40), 1<<40); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("product overflow was not refused: %v", err)
	}
	got, err := money.MulInt(money.Amount(349), 3)
	if err != nil || got != 1047 {
		t.Fatalf("349 * 3 = %s err %v", got, err)
	}
}
