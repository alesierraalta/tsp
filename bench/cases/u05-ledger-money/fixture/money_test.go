package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseCentsReadsAWholeAmount(t *testing.T) {
	got, err := ParseCents("45.00")
	if err != nil {
		t.Fatalf("ParseCents: %v", err)
	}
	if got != 4500 {
		t.Fatalf("ParseCents(45.00) = %d, want 4500", got)
	}
}

func TestParseCentsReadsAnAmountWithoutFraction(t *testing.T) {
	got, err := ParseCents("7")
	if err != nil {
		t.Fatalf("ParseCents: %v", err)
	}
	if got != 700 {
		t.Fatalf("ParseCents(7) = %d, want 700", got)
	}
}

func TestParseCentsReadsANegativeAmount(t *testing.T) {
	got, err := ParseCents("-12.30")
	if err != nil {
		t.Fatalf("ParseCents: %v", err)
	}
	if got != -1230 {
		t.Fatalf("ParseCents(-12.30) = %d, want -1230", got)
	}
}

func TestParseCentsRefusesAThirdFractionDigit(t *testing.T) {
	if _, err := ParseCents("1.155"); !errors.Is(err, ErrFormat) {
		t.Fatalf("ParseCents(1.155) = %v, want ErrFormat", err)
	}
}

func TestParseCentsRefusesWhatIsNotANumber(t *testing.T) {
	for _, amount := range []string{"", "  ", "EUR 1.00", "1,15"} {
		if _, err := ParseCents(amount); !errors.Is(err, ErrFormat) {
			t.Fatalf("ParseCents(%q) = %v, want ErrFormat", amount, err)
		}
	}
}

func TestFormatCents(t *testing.T) {
	cases := map[int64]string{0: "0.00", 5: "0.05", 4500: "45.00", -1230: "-12.30", -5: "-0.05"}
	for cents, want := range cases {
		if got := FormatCents(cents); got != want {
			t.Fatalf("FormatCents(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestFormatCentsRoundTripsAParsedAmount(t *testing.T) {
	for _, amount := range []string{"45.00", "0.05", "-12.30"} {
		cents, err := ParseCents(amount)
		if err != nil {
			t.Fatalf("ParseCents(%q): %v", amount, err)
		}
		if got := FormatCents(cents); got != amount {
			t.Fatalf("FormatCents(ParseCents(%q)) = %q", amount, got)
		}
	}
}

func TestSplitOfAnEvenTotal(t *testing.T) {
	shares, err := Split(1000, 4)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if !reflect.DeepEqual(shares, []int64{250, 250, 250, 250}) {
		t.Fatalf("Split(1000, 4) = %v", shares)
	}
}

func TestSplitIntoOneShare(t *testing.T) {
	shares, err := Split(1999, 1)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if !reflect.DeepEqual(shares, []int64{1999}) {
		t.Fatalf("Split(1999, 1) = %v", shares)
	}
}

func TestSplitRefusesNoShares(t *testing.T) {
	if _, err := Split(1000, 0); !errors.Is(err, ErrParts) {
		t.Fatalf("Split with no shares = %v, want ErrParts", err)
	}
}

func TestApplyRateRoundsHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		cents, bps, want int64
	}{
		{10000, 2000, 2000},
		{1, 5000, 1},
		{-1, 5000, -1},
		{1, 4999, 0},
		{0, 2000, 0},
	}
	for _, c := range cases {
		if got := ApplyRate(c.cents, c.bps); got != c.want {
			t.Fatalf("ApplyRate(%d, %d) = %d, want %d", c.cents, c.bps, got, c.want)
		}
	}
}

func TestSumAddsAmounts(t *testing.T) {
	if got := Sum([]int64{4500, 1200, -300}); got != 5400 {
		t.Fatalf("Sum = %d, want 5400", got)
	}
	if got := Sum(nil); got != 0 {
		t.Fatalf("Sum(nil) = %d", got)
	}
}

func TestAccountTracksABalance(t *testing.T) {
	var account Account
	if got := account.Post(Entry{Account: "cash", Cents: 4500, Memo: "invoice"}); got != 4500 {
		t.Fatalf("Post = %d", got)
	}
	if got := account.Post(Entry{Account: "cash", Cents: -500, Memo: "refund"}); got != 4000 {
		t.Fatalf("Post = %d", got)
	}
	if account.Balance() != 4000 {
		t.Fatalf("Balance = %d", account.Balance())
	}
	if len(account.Entries()) != 2 {
		t.Fatalf("Entries = %v", account.Entries())
	}
}

func TestEntriesAreDetached(t *testing.T) {
	var account Account
	account.Post(Entry{Account: "cash", Cents: 100, Memo: "invoice"})
	entries := account.Entries()
	entries[0].Cents = 999999
	if account.Entries()[0].Cents != 100 {
		t.Fatal("Entries returned the journal itself")
	}
}

func TestReconcileReportsAMismatch(t *testing.T) {
	var account Account
	account.Post(Entry{Account: "cash", Cents: 4500, Memo: "invoice"})
	if err := account.Reconcile(4500); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if err := account.Reconcile(4000); err == nil {
		t.Fatal("Reconcile accepted a different balance")
	}
}
