package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/alesierraalta/tsp/internal/quality"
)

// runProbeQuality reports hygiene findings on the lines a change touches. It is a probe, not a gate:
// exit 1 means the probe found something, exit 2 means it could not measure.
func runProbeQuality(args []string) int {
	stderr := os.Stderr
	parsed, code := parseQualityFlags(args, stderr)
	if code != 0 {
		return code
	}
	report, err := quality.Run(nil, quality.Options{Cwd: parsed.dir, Base: parsed.base, MaxComplexity: parsed.maxComplexity, DupWindow: parsed.dupWindow})
	if err != nil {
		fmt.Fprintln(stderr, "probe quality:", err)
		return 2
	}
	if code := printQualityReport(report, parsed.asJSON, stderr); code != 0 {
		return code
	}
	if len(report.Findings) > 0 {
		return 1
	}
	return 0
}

// qualityFlags are the probe's inputs after parsing.
type qualityFlags struct {
	dir           string
	base          string
	maxComplexity int
	dupWindow     int
	asJSON        bool
}

// parseQualityFlags reads the probe's arguments; a non-zero code is the exit the caller returns.
const qualityHelp = "Usage of probe quality:\n  tsp probe quality [dir] [--cwd <dir>] [--base <ref>] [--max-complexity N] [--dup-window N] [--json]\nReports duplication and complexity inside the lines a change touches, and names the languages it cannot\nmeasure. --base compares that ref with the working tree.\nExit 1: findings; 0: clean (no text output); 2: nothing measurable or usage error. Never writes.\n"

func parseQualityFlags(args []string, stderr io.Writer) (qualityFlags, int) {
	fs := flag.NewFlagSet("probe quality", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, qualityHelp) }
	base := fs.String("base", "", "commit or ref the change is compared with; empty means the working tree")
	maxComplexity := fs.Int("max-complexity", 15, "report functions whose score is above this")
	dupWindow := fs.Int("dup-window", 6, "consecutive lines that count as one duplicated block")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	cwd := fs.String("cwd", ".", "directory the change lives in")
	dir := "."
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		dir = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return qualityFlags{}, 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "probe quality: unexpected argument "+fs.Arg(0))
		return qualityFlags{}, 2
	}
	if *cwd != "." {
		dir = *cwd
	}
	return qualityFlags{dir: dir, base: *base, maxComplexity: *maxComplexity, dupWindow: *dupWindow, asJSON: *asJSON}, 0
}

// printQualityReport writes the report; a non-zero code means the output could not be written.
func printQualityReport(report quality.Report, asJSON bool, stderr io.Writer) int {
	if asJSON {
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, "probe quality:", err)
			return 2
		}
		return 0
	}
	for _, f := range report.Findings {
		fmt.Printf("%s:%d %s — %s\n", f.File, f.Line, f.Rule, f.Message)
	}
	return 0
}
