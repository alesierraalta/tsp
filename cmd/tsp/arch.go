package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/alesierraalta/tsp/internal/arch"
)

// runProbeArch reports the import edges a change added that the repository declared it never wants. It
// is a probe, not a gate: exit 1 means the probe found something, exit 2 means it could not measure.
func runProbeArch(args []string) int {
	stderr := os.Stderr
	parsed, code := parseArchFlags(args, stderr)
	if code != 0 {
		return code
	}
	report, err := arch.Run(nil, arch.Options{Cwd: parsed.dir, Base: parsed.base, RulesPath: parsed.rules})
	if err != nil {
		fmt.Fprintln(stderr, "probe arch:", err)
		return 2
	}
	if code := printArchReport(report, parsed.asJSON, stderr); code != 0 {
		return code
	}
	if len(report.Findings) > 0 {
		return 1
	}
	return 0
}

// archFlags are the probe's inputs after parsing.
type archFlags struct {
	dir    string
	base   string
	rules  string
	asJSON bool
}

// parseArchFlags reads the probe's flags and names every refusal it can make up front.
func parseArchFlags(args []string, stderr io.Writer) (archFlags, int) {
	var parsed archFlags
	flags := flag.NewFlagSet("probe arch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&parsed.dir, "cwd", ".", "repository to probe")
	flags.StringVar(&parsed.base, "base", "", "commit or ref to diff against (default: the working tree)")
	flags.StringVar(&parsed.rules, "rules", "", "architecture declaration (default: <repo>/.tsp-arch.json)")
	flags.BoolVar(&parsed.asJSON, "json", false, "print the report as JSON")
	if err := flags.Parse(args); err != nil {
		return archFlags{}, 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(stderr, "probe arch:", "unexpected argument:", flags.Arg(0))
		return archFlags{}, 2
	}
	return parsed, 0
}

// printArchReport writes the report in the shape the caller asked for, naming what was not measured.
func printArchReport(report arch.Report, asJSON bool, out io.Writer) int {
	if asJSON {
		body, err := json.Marshal(report)
		if err != nil {
			fmt.Fprintln(out, "probe arch:", "cannot encode the report:", err)
			return 2
		}
		fmt.Fprintln(out, string(body))
		return 0
	}
	for _, finding := range report.Findings {
		fmt.Fprintf(out, "%s:%d %s %s\n", finding.File, finding.Line, finding.Rule, finding.Message)
	}
	for _, note := range report.NotChecked {
		fmt.Fprintln(out, "note:", note)
	}
	return 0
}
