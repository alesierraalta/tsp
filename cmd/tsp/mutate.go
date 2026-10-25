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
	coefficient := fs.Int("timeout-coefficient", 0, "multiply the engine's per-mutant test budget")
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
		Cwd:                *cwd,
		Base:               *base,
		MaxMutants:         *maxMutants,
		Workers:            *workers,
		Timeout:            *timeout,
		TimeoutCoefficient: *coefficient,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate:", err)
		return 2
	}

	if *asJSON {
		if code := printMutateJSON(report); code != 0 {
			return code
		}
	} else {
		printMutateReport(report, *failOn)
	}
	if report.Blocking(*failOn) {
		return 1
	}
	return 0
}

// printMutateJSON writes the machine report.
func printMutateJSON(report mutate.Report) int {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate:", err)
		return 2
	}
	fmt.Println(string(encoded))
	return 0
}

// printMutateReport states what was counted, what was judged, and what nobody judged, because a
// mutant with no status is not a kill.
func printMutateReport(report mutate.Report, failOn int) {
	if report.MutationCount == 0 {
		fmt.Println("tsp mutate: no production Go source changed, so no mutant was measured")
		return
	}
	fmt.Printf("tsp mutate: %d mutant(s) from %d package(s), %d killed, %d survived, %d not covered, %d not viable, %d timed out, %s\n",
		report.MutationCount, len(report.Targets), report.Killed, report.Survived, report.NotCovered, report.NotViable, report.TimedOut, report.Elapsed.Round(time.Millisecond))
	if report.TimedOut > 0 && report.Killed+report.Survived > 0 {
		fmt.Printf("tsp mutate: %d mutant(s) timed out and were never judged: raise --timeout-coefficient before trusting this report\n", report.TimedOut)
	}
	for _, survivor := range report.Survivors {
		fmt.Printf("  %s:%d %s — no test kills this mutation\n", survivor.File, survivor.Line, survivor.Operator)
	}
	if report.Survived > 0 && failOn >= 0 {
		fmt.Printf("tsp mutate: %d survivor(s) above the threshold of %d: the tests do not pin this change\n", report.Survived, failOn)
	}
}
