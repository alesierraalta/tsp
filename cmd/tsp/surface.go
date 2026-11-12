package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/alesierraalta/tsp/internal/surface"
)

// runProbeSurface inventories the inputs a change's functions read and the sinks each one reaches. It is a
// probe, not a gate: exit 1 means an input reaches a sink with no guard in front of it, exit 2 means the
// probe could not measure.
func runProbeSurface(args []string) int {
	stderr := os.Stderr
	// Asking for help is not a mistake: a caller learning the flags gets the usage and exit 0.
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stderr, surfaceHelp)
		return 0
	}
	parsed, code := parseSurfaceFlags(args, stderr)
	if code != 0 {
		return code
	}
	report, err := surface.Run(nil, surface.Options{Cwd: parsed.dir, Base: parsed.base, Paths: parsed.paths})
	if err != nil {
		fmt.Fprintln(stderr, "probe surface:", err)
		return 2
	}
	if code := printSurfaceReport(report, parsed.asJSON, os.Stdout, stderr); code != 0 {
		return code
	}
	if report.Blocking() {
		return 1
	}
	return 0
}

// surfaceFlags are the probe's inputs after parsing.
type surfaceFlags struct {
	dir    string
	base   string
	paths  []string
	asJSON bool
}

// surfaceHelp is what the probe answers when it is asked what it does.
const surfaceHelp = "Usage of probe surface:\n  tsp probe surface [dir] [--cwd <dir>] [--base <ref>] [--paths <path> ...] [--json]\nLists, for the functions a change touches (or the paths given), every input and its origin, the sensitive\nsinks it reaches and whether a guard stands between them. --base compares that ref with the working tree.\nExit 1: an input reaches a sink unguarded; 0: every row is guarded or there is none; 2: nothing measurable\nor usage error. Never writes.\n"

// pathsFlag collects a repeatable --paths value.
type pathsFlag []string

// String renders the collected paths for the flag package.
func (p *pathsFlag) String() string { return fmt.Sprint([]string(*p)) }

// Set appends one path.
func (p *pathsFlag) Set(value string) error {
	*p = append(*p, value)
	return nil
}

// parseSurfaceFlags reads the probe's arguments; a non-zero code is the exit the caller returns.
func parseSurfaceFlags(args []string, stderr io.Writer) (surfaceFlags, int) {
	var paths pathsFlag
	var parsed surfaceFlags
	flags := flag.NewFlagSet("probe surface", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, surfaceHelp) }
	flags.StringVar(&parsed.dir, "cwd", ".", "repository the change lives in")
	flags.StringVar(&parsed.base, "base", "", "commit or ref to diff against (default: the working tree)")
	flags.Var(&paths, "paths", "file or directory to inventory instead of the change (repeatable)")
	flags.BoolVar(&parsed.asJSON, "json", false, "print the report as JSON")
	rest := args
	if len(rest) > 0 && rest[0] != "" && rest[0][0] != '-' {
		parsed.dir = rest[0]
		rest = rest[1:]
	}
	if err := flags.Parse(rest); err != nil {
		return surfaceFlags{}, 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(stderr, "probe surface: unexpected argument "+flags.Arg(0))
		return surfaceFlags{}, 2
	}
	parsed.paths = paths
	return parsed, 0
}

// surfaceGroup is one function's rows, printed under one header.
type surfaceGroup struct {
	file     string
	function string
	line     int
	rows     []surface.Row
}

// printSurfaceReport writes the report in the shape the caller asked for.
func printSurfaceReport(report surface.Report, asJSON bool, out, stderr io.Writer) int {
	if asJSON {
		if err := json.NewEncoder(out).Encode(report); err != nil {
			fmt.Fprintln(stderr, "probe surface:", err)
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "surface: %d row(s) in %d function(s), %d unguarded; heuristic: %s\n", len(report.Rows), report.Functions(), report.Unguarded(), report.Heuristic)
	for _, group := range groupSurfaceRows(report.Rows) {
		fmt.Fprintf(out, "%s:%d func %s(...)\n", group.file, group.line, group.function)
		for _, row := range group.rows {
			fmt.Fprintf(out, "  %s (%s) -> %s @ %s:%d  guard: %s  verdict: ___\n", row.Input, row.Origin, row.Sink, row.File, row.SinkLine, guardText(row))
		}
	}
	for _, note := range report.Skipped {
		fmt.Fprintln(out, "not measured:", note)
	}
	return 0
}

// guardText is the guard a reader sees, or the word that says there is none.
func guardText(row surface.Row) string {
	if row.Guard == "" {
		return "none"
	}
	return row.Guard
}

// groupSurfaceRows groups the rows by function, keeping the order the probe sorted them in: a function with
// an unguarded row prints before one whose rows are all guarded.
func groupSurfaceRows(rows []surface.Row) []surfaceGroup {
	index := map[string]*surfaceGroup{}
	var order []*surfaceGroup
	for _, row := range rows {
		key := fmt.Sprintf("%s:%d:%s", row.File, row.FunctionLine, row.Function)
		group, seen := index[key]
		if !seen {
			group = &surfaceGroup{file: row.File, function: row.Function, line: row.FunctionLine}
			index[key] = group
			order = append(order, group)
		}
		group.rows = append(group.rows, row)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if ua, ub := hasUnguarded(a), hasUnguarded(b); ua != ub {
			return ua
		}
		if a.file != b.file {
			return a.file < b.file
		}
		return a.line < b.line
	})
	groups := make([]surfaceGroup, 0, len(order))
	for _, group := range order {
		groups = append(groups, *group)
	}
	return groups
}

// hasUnguarded reports whether a group carries a row nobody guarded.
func hasUnguarded(group *surfaceGroup) bool {
	for _, row := range group.rows {
		if !row.Guarded {
			return true
		}
	}
	return false
}
