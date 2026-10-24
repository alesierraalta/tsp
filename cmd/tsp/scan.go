package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/scan"
)

// runScan brings a real scanner's findings into the change: it runs the engine installed for this
// machine, or reads a SARIF report another tool already wrote, and keeps only the files the change
// touched. The exit code is the contract: 0 nothing at the gated level, 1 findings, 2 nothing was
// scanned — a clean scan and an empty one are different answers.
func runScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "repository to scan (default: current directory)")
	base := fs.String("base", "", "commit the change is measured from (default: the working tree)")
	sarifPath := fs.String("sarif", "", "read this SARIF report instead of running an engine")
	level := fs.String("level", "warning", "gate at this level: note, warning or error")
	timeout := fs.Duration("timeout", 10*time.Minute, "time limit for the engine")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	switch strings.ToLower(strings.TrimSpace(*level)) {
	case "note", "warning", "error":
	default:
		fmt.Fprintf(os.Stderr, "scan: unknown level %q: name note, warning or error\n", *level)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	report, err := scan.Run(ctx, scan.Options{
		Cwd:    *cwd,
		Base:   *base,
		Sarif:  *sarifPath,
		Level:  *level,
		Budget: *timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		return 2
	}

	if *asJSON {
		encoded, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			fmt.Fprintln(os.Stderr, "scan:", marshalErr)
			return 2
		}
		fmt.Println(string(encoded))
	} else if len(report.Findings) == 0 && len(report.Scanned) == 0 {
		fmt.Println("tsp scan: the change touched nothing, so there was nothing to scan")
	} else if len(report.Findings) == 0 {
		fmt.Printf("tsp scan: %s scanned the change and reported nothing\n", report.Engine)
	} else {
		fmt.Printf("tsp scan: %d finding(s) from %s in %s\n", len(report.Findings), report.Engine, report.Elapsed.Round(time.Millisecond))
		for _, finding := range report.Findings {
			fmt.Printf("  %s:%d %s %s — %s\n", finding.File, finding.Line, finding.Level, finding.Rule, finding.Message)
		}
	}
	if report.Blocking(*level) {
		return 1
	}
	return 0
}
