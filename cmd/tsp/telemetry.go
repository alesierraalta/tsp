package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alesierraalta/tsp/internal/feature"
	"github.com/alesierraalta/tsp/internal/telemetry"
)

// telemetryHelp is the telemetry command's own usage: its subcommands and the exit codes they mean.
const telemetryHelp = `Usage of telemetry:
  tsp telemetry record
      Reads ONE host event as JSON on stdin and keeps it in the local ledger as kind host_run.
      The shape is closed: only source, run_id, session and host (model, turns, tool_calls,
      tool_errors, compactions, subagents, wall_ms, first_tsp_ms, tokens, cost_usd, per_turn,
      tools, offer) are accepted. An unknown key, free text, a negative count, a tool count out of
      bounds or trailing data is refused with exit 2 and NOTHING is written.
      Validation runs even when telemetry is off; with it off a valid event is accepted and the
      ledger is not created.
  tsp telemetry stats [--json]
      Reads the local ledger and groups what it measured by tsp version and skill identity:
      events, distinct runs, command counts and failure rate, p50/p95 duration per subcommand,
      check decision distribution, blocked rate, host token totals, cache hit, turns and tool
      calls per host run, and tokens per pinned red test. Malformed lines are counted and reported.
      A missing or empty ledger says how to enable recording and exits 0. Reading works with the
      feature off. --json prints the same numbers as JSON.
  tsp telemetry export [--format jsonl|csv]
      Writes the ledger back: jsonl is every valid line verbatim, csv is one column per flattened
      dotted field with a sorted header (arrays and dynamic maps JSON-encoded in the cell).
      Malformed lines are skipped and their count is reported on stderr. Default format: jsonl.
Exit 0: accepted or read; 1: the ledger could not be read; 2: usage error or refused host event.
A record write failure adds one stderr warning without changing the exit code.
`

// telemetryDocumentation is every help text the CLI prints: the main usage plus the commands with
// their own help. Command events derive their documented flag names from this one bundle, so a flag
// that is documented only in a subcommand's help is still a known name.
var telemetryDocumentation = strings.Join([]string{usage, probeHelp, qualityHelp, findingsHelp, telemetryHelp}, "\n\n")

// runTelemetry is the `tsp telemetry` command. An unknown subcommand is a usage error, not a
// refusal of data, so it exits 2 with the command's own usage.
func runTelemetry(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, telemetryHelp)
		return 2
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Fprint(os.Stderr, telemetryHelp)
		return 0
	case "record":
		return runTelemetryRecord(args[1:])
	case "stats":
		return runTelemetryStats(args[1:])
	case "export":
		return runTelemetryExport(args[1:])
	default:
		fmt.Fprint(os.Stderr, telemetryHelp)
		return 2
	}
}

// runTelemetryStats prints the ledger's aggregates, as text or as JSON. It never requires the
// feature to be on: reading what is already recorded must keep working after an opt-out.
func runTelemetryStats(args []string) int {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprint(os.Stderr, telemetryHelp)
			return 0
		case "--json":
			asJSON = true
		default:
			fmt.Fprintf(os.Stderr, "telemetry stats: unexpected argument %q\n", arg)
			return 2
		}
	}
	report, err := telemetry.Stats()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tsp telemetry stats:", err)
		return 1
	}
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "tsp telemetry stats:", err)
			return 1
		}
		return 0
	}
	writeStatsText(os.Stdout, report)
	return 0
}

// writeStatsText renders the report for a human. Every number is shown with the definition that
// produced it, because a rate whose denominator is unknown is not a measurement.
func writeStatsText(w io.Writer, report telemetry.Report) {
	fmt.Fprintf(w, "telemetry ledger: %s\n", report.Path)
	fmt.Fprintf(w, "lines: %d valid, %d malformed\n", report.ValidLines, report.MalformedLines)
	if report.ValidLines == 0 && report.MalformedLines == 0 {
		fmt.Fprintln(w, "no events yet; enable recording with: tsp feature enable telemetry")
		return
	}
	for _, group := range report.Groups {
		fmt.Fprintf(w, "\nversion %s  skill %s\n", group.Version, group.Skill)
		fmt.Fprintf(w, "  events %d  runs %d\n", group.Events, group.Runs)
		fmt.Fprintln(w, "  commands:")
		for _, command := range group.Commands {
			fmt.Fprintf(w, "    %s  count %d  failures %d  failure rate %.1f%%  p50 %dms  p95 %dms\n",
				command.Name, command.Count, command.Failures, command.FailureRate*100, command.P50MS, command.P95MS)
		}
		if len(group.Commands) == 0 {
			fmt.Fprintln(w, "    none")
		}
		fmt.Fprintf(w, "  check decisions: %s\n", decisionText(group.CheckDecisions))
		fmt.Fprintf(w, "  blocked: %d/%d = %s (check exit 2 or reason error)\n",
			group.Blocked.Blocked, group.Blocked.Checks, percentText(group.Blocked.Rate))
		hosts := group.Hosts
		fmt.Fprintf(w, "  host: runs %d  input %d  output %d  cache_read %d  cache_write %d  cache hit %s\n",
			hosts.Runs, hosts.Input, hosts.Output, hosts.CacheRead, hosts.CacheWrite, percentText(hosts.CacheHit))
		fmt.Fprintf(w, "  host: turns %d total %s/run  tool_calls %d total %s/run\n",
			hosts.Turns, numberText(hosts.TurnsPerRun), hosts.ToolCalls, numberText(hosts.ToolCallsPerRun))
		fmt.Fprintf(w, "  host offers: %s\n", offerText(hosts.Offers))
		if group.TokensPerPinnedRedTest == nil {
			fmt.Fprintln(w, "  tokens per pinned red test: n/a")
		} else {
			fmt.Fprintf(w, "  tokens per pinned red test: %.2f\n", *group.TokensPerPinnedRedTest)
		}
	}
}

func offerText(offers []telemetry.OfferCount) string {
	if len(offers) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(offers))
	for _, offer := range offers {
		parts = append(parts, fmt.Sprintf("%s %d", offer.Offer, offer.Count))
	}
	return strings.Join(parts, ", ")
}

func decisionText(decisions []telemetry.DecisionCount) string {
	if len(decisions) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		parts = append(parts, fmt.Sprintf("%s %d", decision.Decision, decision.Count))
	}
	return strings.Join(parts, ", ")
}

func percentText(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", *value*100)
}

func numberText(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", *value)
}

// runTelemetryExport writes the ledger back, verbatim or flattened, and reports malformed lines on
// stderr so a script's stdout stays the data.
func runTelemetryExport(args []string) int {
	format := "jsonl"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			fmt.Fprint(os.Stderr, telemetryHelp)
			return 0
		case arg == "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "telemetry export: --format needs jsonl or csv")
				return 2
			}
			i++
			format = args[i]
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		default:
			fmt.Fprintf(os.Stderr, "telemetry export: unexpected argument %q\n", arg)
			return 2
		}
	}
	path, err := telemetry.PathReadOnly()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tsp telemetry export:", err)
		return 1
	}
	var result telemetry.ExportResult
	switch format {
	case "jsonl":
		result, err = telemetry.ExportJSONL(os.Stdout, path)
	case "csv":
		result, err = telemetry.ExportCSV(os.Stdout, path)
	default:
		fmt.Fprintf(os.Stderr, "telemetry export: unknown --format %q (want jsonl or csv)\n", format)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tsp telemetry export:", err)
		return 1
	}
	if result.Malformed > 0 {
		fmt.Fprintf(os.Stderr, "tsp telemetry export: skipped %d malformed line(s)\n", result.Malformed)
	}
	return 0
}

// runTelemetryRecord reads one host event, validates it against the closed allow-list, and notes it
// for the run's single flush. Nothing is written here: the ledger write is main's, so a refused
// event writes nothing and an opted-out install creates nothing.
func runTelemetryRecord(args []string) int {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprint(os.Stderr, telemetryHelp)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "telemetry record: unexpected argument %q\n", arg)
			return 2
		}
	}
	record, err := telemetry.ParseRecord(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tsp telemetry record:", err)
		return 2
	}
	enabled, err := feature.EnabledReadOnly(feature.TelemetryID)
	if err != nil {
		// An unreadable state reads as telemetry off, silently: the host's own call must not fail
		// because a local toggle file cannot be trusted.
		return 0
	}
	if !enabled {
		// A valid host event is accepted while telemetry is off, and accepted means silence: the
		// adapter that sent it must not see its own call as a failure it has to report.
		return 0
	}
	telemetry.Note(record.HostEvent())
	return 0
}
