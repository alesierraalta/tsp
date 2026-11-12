// Command triggers exercises the planted defect of this case against whichever module it is run from:
// `cd fixture && go run ../testdata/triggers/main.go` reads the fixture, and the same command run from
// fix/all or fix/keep-<ID> reads that variant. The output is what KEY.json records as trigger.actual.
package main

import (
	"fmt"
	"os"

	assets "bench/u01-asset-resolver"
)

func line(label string, got string, err error) {
	if err != nil {
		fmt.Printf("%s -> err=%v\n", label, err)
		return
	}
	fmt.Printf("%s -> path=%q err=<nil>\n", label, got)
}

func main() {
	r, err := assets.New("/srv/assets", map[string]string{
		"app":   "bundles/app.min.js",
		"admin": "../../etc/passwd",
	})
	if err != nil {
		fmt.Println("New:", err)
		os.Exit(1)
	}

	got, err := r.Resolve("../assets-v2/key.pem")
	line(`D1 Resolve("../assets-v2/key.pem")`, got, err)

	got, err = r.Bundle("admin")
	line(`D2 Bundle("admin") with manifest admin=../../etc/passwd`, got, err)

	got, err = r.Resolve("img/logo.png")
	line(`ok Resolve("img/logo.png")`, got, err)
}
