package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/feature"
)

// sampleUsage is the shape of the real usage text: a header line naming the command, indented
// continuation lines, and one or more parenthesized alternative lists.
const sampleUsage = `usage: tsp <command> [flags]

commands:
  plan     write the skeleton and check the contract
           (init | check | gaps | add-finding)
           [--path <path>] [--run <slug>] [--before] [--b] [--a] [--turns] [-h]
  feedback record an honest process report
           (--template | --file <path> | --summary)
  telemetry the local usage ledger
           (record | reads one host event as JSON on stdin; stats | export read it back)
  version  print the version
`

// sampleCommands is the dispatch table's own names, the one source of truth for a command name.
var sampleCommands = []string{"plan", "feedback", "telemetry", "version"}

// A flag name is a name: values, positional arguments and anything after "--" never reach the
// ledger, an unknown shape is dropped, and a repeat is recorded once.
func TestFlagNamesKeepsNamesOnly(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"empty", nil, nil},
		{"names with values", []string{"--path", "secret-dir/plan.md", "--run=secret-run"}, []string{"path", "run"}},
		{"positional only", []string{"check", "some/path.md"}, nil},
		{"stops at double dash", []string{"--before", "--", "--after"}, []string{"before"}},
		{"deduplicates in order", []string{"--b", "--a", "--b"}, []string{"b", "a"}},
		{"negative number is not a flag name", []string{"--turns", "-1"}, []string{"turns"}},
		{"short flag", []string{"-h"}, []string{"h"}},
		{"uppercase refused", []string{"--CamelCase"}, nil},
		{"empty name refused", []string{"--=x", "---"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FlagNames(sampleUsage, "plan", tc.args)
			if len(got) != len(tc.want) {
				t.Fatalf("FlagNames(%v) = %v, want %v", tc.args, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("FlagNames(%v) = %v, want %v", tc.args, got, tc.want)
				}
			}
		})
	}
}

// A sub is recorded only when the usage text itself documents it, so a positional path can never
// land in the field.
func TestSubcommandOnlyRecordsDocumentedWords(t *testing.T) {
	cases := []struct {
		command string
		args    []string
		want    string
	}{
		{"plan", []string{"check", "--path", "p.md"}, "check"},
		{"telemetry", []string{"record"}, "record"},
		{"plan", []string{"docs/testing/test-plan.md"}, ""},
		{"plan", []string{"--json"}, ""},
		{"plan", nil, ""},
	}
	for _, tc := range cases {
		if got := Subcommand(sampleUsage, tc.command, tc.args); got != tc.want {
			t.Errorf("Subcommand(%s, %v) = %q, want %q", tc.command, tc.args, got, tc.want)
		}
	}
	// feedback's alternatives are flags: it has no documented subcommand, so its path stays out.
	if got := Subcommand(sampleUsage, "feedback", []string{"report.md"}); got != "" {
		t.Errorf("Subcommand(feedback, report.md) = %q, want empty", got)
	}
}

// NewCommand carries the wall time and exit code unchanged and derives the rest from the usage text.
func TestNewCommandDescribesTheRun(t *testing.T) {
	command := NewCommand(sampleUsage, sampleCommands, "plan", []string{"check", "--path=x.md"}, 3, 1500*1000*1000)
	if command.Name != "plan" || command.Sub != "check" || command.Exit != 3 || command.DurationMS != 1500 {
		t.Fatalf("NewCommand = %+v", command)
	}
	if len(command.Flags) != 1 || command.Flags[0] != "path" {
		t.Fatalf("flags = %v", command.Flags)
	}
}

// A host event is accepted only inside the closed shape: unknown keys, free text, negative counts,
// an unknown enum value, an out-of-bounds map or trailing data are refused.
func TestParseRecordAcceptsTheClosedShapeAndRefusesTheRest(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"valid", `{"source":"pi","run_id":"run-7","session":"s1","host":{"model":"p/m","turns":3,"tokens":{"input":1},"per_turn":[{"cache_read":2}],"tools":{"read":3},"offer":"accepted"}}`, true},
		{"no host", `{"source":"claude"}`, true},
		{"unknown key", `{"source":"pi","host":{"turns":1,"prompt":"fix the bug"}}`, false},
		{"free text model", `{"source":"pi","host":{"model":"tell me a story"}}`, false},
		{"negative count", `{"source":"pi","host":{"turns":-1}}`, false},
		{"negative tokens", `{"source":"pi","host":{"tokens":{"cache_read":-1}}}`, false},
		{"negative per turn", `{"source":"pi","host":{"per_turn":[{"output":-2}]}}`, false},
		{"negative cost", `{"source":"pi","host":{"cost_usd":-0.5}}`, false},
		{"unknown source", `{"source":"cursor"}`, false},
		{"unknown offer", `{"source":"pi","host":{"offer":"maybe"}}`, false},
		{"tool name text", `{"source":"pi","host":{"tools":{"rm -rf /":1}}}`, false},
		{"negative tool count", `{"source":"pi","host":{"tools":{"read":-1}}}`, false},
		{"run id with space", `{"source":"pi","run_id":"a b"}`, false},
		{"session with slash", `{"source":"pi","session":"a/b"}`, false},
		{"trailing data", `{"source":"pi"} {"source":"pi"}`, false},
		{"not json", `turns=3`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record, err := ParseRecord(strings.NewReader(tc.body))
			if tc.ok && err != nil {
				t.Fatalf("ParseRecord(%s) refused a valid event: %v", tc.body, err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("ParseRecord(%s) accepted a refused event: %+v", tc.body, record)
				}
				// A refusal names the field, never the value a host sent.
				if strings.Contains(err.Error(), "story") || strings.Contains(err.Error(), "rm -rf") {
					t.Fatalf("refusal echoes the value: %v", err)
				}
			}
		})
	}
}

// The bounds exist so a host cannot turn the ledger into a transcript: a tool map and a per-turn
// array over their limits are refused, one exactly at the limit is accepted.
func TestParseRecordBounds(t *testing.T) {
	tools := make([]string, maxTools+1)
	for i := range tools {
		tools[i] = `"t` + strings.Repeat("a", 1) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `":1`
	}
	over := `{"source":"pi","host":{"tools":{` + strings.Join(tools, ",") + `}}}`
	if _, err := ParseRecord(strings.NewReader(over)); err == nil {
		t.Fatal("a tool map over the bound was accepted")
	}
	turns := make([]string, maxPerTurn+1)
	for i := range turns {
		turns[i] = `{"input":1}`
	}
	long := `{"source":"pi","host":{"per_turn":[` + strings.Join(turns, ",") + `]}}`
	if _, err := ParseRecord(strings.NewReader(long)); err == nil {
		t.Fatal("a per-turn array over the bound was accepted")
	}
}

// Flush is where a raw session becomes a pseudonym and where an unmeasured count stays absent.
func TestFlushStampsTheEnvelopeAndSealsTheSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	t.Setenv(RunIDEnv, "")
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}
	Note(Event{
		Source:  SourcePi,
		Kind:    KindHostRun,
		Session: "sess-abc",
		Host:    &Host{Turns: int64p(3)},
	})
	gate := Begin()
	if err := gate.Flush(Base{Version: "1.2.3", Commit: "abc1234", Skill: "test-strategy 0.7.25"},
		Command{Name: "version", Exit: 0, DurationMS: 7}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, DirName, LedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (host_run, command):\n%s", len(lines), data)
	}
	var host Event
	if err := json.Unmarshal([]byte(lines[0]), &host); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(host.Session, "session-") || strings.Contains(host.Session, "sess-abc") {
		t.Fatalf("session = %q, want a salted pseudonym", host.Session)
	}
	if host.Host == nil || host.Host.Compactions != nil {
		t.Fatalf("an unmeasured count must stay absent: %+v", host.Host)
	}
	if host.Version != "1.2.3" || host.Commit != "abc1234" || host.Skill != "test-strategy 0.7.25" || host.Schema != Schema {
		t.Fatalf("envelope = %+v", host)
	}
	if host.Repo == "" || host.Machine == "" {
		t.Fatalf("repo and machine pseudonyms are missing: %+v", host)
	}
	if strings.Contains(string(data), home) {
		t.Fatalf("the ledger carries a path:\n%s", data)
	}
}

// With the feature off a flush returns nil and creates no directory: opting out is the absence of
// the ledger, not an empty one.
func TestFlushOffTouchesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	Note(Event{Source: SourcePi, Kind: KindHostRun, Host: &Host{Turns: int64p(1)}})
	gate := Begin()
	if err := gate.Flush(Base{}, Command{Name: "version"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, DirName)); !os.IsNotExist(err) {
		t.Fatalf("telemetry off created %s (err %v)", filepath.Join(home, DirName), err)
	}
}

func int64p(value int64) *int64 { return &value }

// D1: only a command the usage text dispatches becomes a command name.
func TestNewCommandUnknownName(t *testing.T) {
	command := NewCommand(sampleUsage, sampleCommands, "/etc/secret-plan", nil, 2, 0)
	if command.Name != "unknown" {
		t.Fatalf("NewCommand name = %q, want unknown", command.Name)
	}
}

// D3: a dash-prefixed VALUE is not a flag name unless the command's usage documents that name.
func TestNewCommandFlagsOnlyDocumentedNames(t *testing.T) {
	command := NewCommand(sampleUsage, sampleCommands, "feedback", []string{"--file", "-private", "--summary"}, 0, 0)
	if len(command.Flags) != 2 || command.Flags[0] != "file" || command.Flags[1] != "summary" {
		t.Fatalf("flags = %v, want only the documented [file summary]", command.Flags)
	}
}

// D8: usage prose is not a subcommand; only the explicit allow-list is.
func TestNewCommandSubcommandIsNotAUsageWord(t *testing.T) {
	command := NewCommand(sampleUsage, sampleCommands, "telemetry", []string{"reads"}, 2, 0)
	if command.Sub != "" {
		t.Fatalf("sub = %q, want empty for a usage prose word", command.Sub)
	}
}
