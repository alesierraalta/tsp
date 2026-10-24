package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/alesierraalta/tsp/internal/mutate"
)

// runMutate is the mutation run a change owes: the production source it touched is mutated by the
// engine for its stack, and every mutant the tests fail to kill is a line the change left unpinned.
// The exit code is the contract: 0 no survivor at the threshold, 1 survivors, 2 nothing was measured
// (no engine, or a change above the budget) — an unmeasured run never reads as a pass.
func runMutate(args []string) int {
	fs := flag.NewFlagSet("mutate", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "repository to mutate (default: current directory)")
	base := fs.String("base", "", "commit the change is measured from (default: the working tree)")
	maxMutants := fs.Int("max-mutants", 60, "refuse a change that reaches more mutants than this; 0 means no budget")
	workers := fs.Int("workers", 0, "parallel test runners the engine may use (default: the engine decides)")
	timeout := fs.Duration("timeout", 20*time.Minute, "time limit for the whole run")
	failOn := fs.Int("fail-on-survivors", 0, "exit 1 when more mutants than this survive; a negative value only reports")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	report, err := mutate.Run(ctx, mutate.Options{
		Cwd:        *cwd,
		Base:       *base,
		MaxMutants: *maxMutants,
		Workers:    *workers,
		Timeout:    *timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate:", err)
		return 2
	}

	if *asJSON {
		encoded, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			fmt.Fprintln(os.Stderr, "mutate:", marshalErr)
			return 2
		}
		fmt.Println(string(encoded))
	} else if report.MutationCount == 0 {
		fmt.Println("tsp mutate: no production Go source changed, so no mutant was measured")
	} else {
		fmt.Printf("tsp mutate: %d mutant(s) from %d package(s), %d killed, %d survived, %d not covered, %s\n",
			report.MutationCount, len(report.Targets), report.Killed, report.Survived, report.NotCovered, report.Elapsed.Round(time.Millisecond))
		for _, survivor := range report.Survivors {
			fmt.Printf("  %s:%d %s — no test kills this mutation\n", survivor.File, survivor.Line, survivor.Operator)
		}
		if report.Blocking(*failOn) {
			fmt.Printf("tsp mutate: %d survivor(s) above the threshold of %d: the tests do not pin this change\n", report.Survived, *failOn)
		}
	}
	if report.Blocking(*failOn) {
		return 1
	}
	return 0
}
