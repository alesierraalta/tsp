package main

import (
	"fmt"
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
Exit 0: accepted, also when the ledger cannot be written — that failure adds one stderr warning
      ("tsp telemetry: <what failed>") and never changes the exit code; 2: usage error or a refused
      event that wrote nothing.
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
	default:
		fmt.Fprint(os.Stderr, telemetryHelp)
		return 2
	}
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
