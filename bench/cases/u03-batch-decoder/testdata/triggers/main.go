// Command triggers exercises the planted defect of this case against whichever module it is run from:
// `cd fixture && go run ../testdata/triggers/main.go` reads the fixture, and the same command run from
// fix/all reads the corrected module. The output is what KEY.json records as trigger.actual.
package main

import (
	"fmt"
	"os"

	batchdec "bench/u03-batch-decoder"
)

func main() {
	block := "v1 cols=2 rows=1\nsocks|3 x2000000\n"
	rows, err := batchdec.Decode(block)
	if err != nil {
		fmt.Printf("D1 Decode(header rows=1, one row with x2000000) -> rows=0 err=%v\n", err)
	} else {
		fmt.Printf("D1 Decode(header rows=1, one row with x2000000) -> rows=%d err=<nil>\n", len(rows))
	}

	small, err := batchdec.Decode("v1 cols=2 rows=2\nshirt|7\nsocks|3 x3\n")
	fmt.Printf("ok Decode(rows=2, x3) -> rows=%d err=%v\n", len(small), err)

	if _, err := batchdec.ParseHeader("v1 cols=2 rows=100001"); err != nil {
		fmt.Printf("ok ParseHeader(rows=100001) -> err=%v\n", err)
	} else {
		fmt.Println("ok ParseHeader(rows=100001) -> err=<nil>")
		os.Exit(2)
	}
}
