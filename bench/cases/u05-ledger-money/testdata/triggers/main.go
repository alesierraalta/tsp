// Command triggers exercises the planted defects of this case against whichever module it is run from:
// `cd fixture && go run ../testdata/triggers/main.go` reads the fixture, and the same command run from
// fix/all or fix/keep-<ID> reads that variant. The output is what KEY.json records as trigger.actual.
package main

import (
	"fmt"

	ledger "bench/u05-ledger-money"
)

func main() {
	cents, err := ledger.ParseCents("1.15")
	fmt.Printf("D1 ParseCents(\"1.15\") -> cents=%d (%s) err=%v\n", cents, ledger.FormatCents(cents), err)

	shares, err := ledger.Split(1000, 3)
	fmt.Printf("D2 Split(1000, 3) -> shares=%v sum=%d err=%v\n", shares, ledger.Sum(shares), err)

	cents, err = ledger.ParseCents("45.00")
	fmt.Printf("ok ParseCents(\"45.00\") -> cents=%d err=%v\n", cents, err)

	if _, err := ledger.ParseCents("1.155"); err != nil {
		fmt.Printf("ok ParseCents(\"1.155\") -> err=%v\n", err)
	} else {
		fmt.Println("ok ParseCents(\"1.155\") -> err=<nil>")
	}
}
