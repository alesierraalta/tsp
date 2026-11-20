package main

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cliStatsFixture is the ledger the stats/export CLI tests read: two versions, two runs, domain
// events the reader must decode generically, and one malformed line.
var cliStatsFixture = []string{
	`{"ts":"2026-01-01T00:00:00Z","schema":1,"source":"tsp","kind":"command","version":"1.0.0","skill":"skill-a 1","command":{"name":"version","exit":0,"duration_ms":100}}`,
	`{"ts":"2026-01-01T00:00:01Z","schema":1,"source":"tsp","kind":"command","run_id":"r1","version":"1.0.0","skill":"skill-a 1","command":{"name":"version","exit":1,"duration_ms":300}}`,
	`{"ts":"2026-01-01T00:00:02Z","schema":1,"source":"tsp","kind":"command","run_id":"r1","version":"1.0.0","skill":"skill-a 1","command":{"name":"check","exit":2,"duration_ms":200}}`,
	`{"ts":"2026-01-01T00:00:03Z","schema":1,"source":"tsp","kind":"check","run_id":"r1","version":"1.0.0","skill":"skill-a 1","check":{"decision":"always","exit":2,"reason":"witness","red":2}}`,
	`{"ts":"2026-01-01T00:00:04Z","schema":1,"source":"tsp","kind":"check","run_id":"r1","version":"1.0.0","skill":"skill-a 1","check":{"decision":"never","exit":0,"reason":"error","red":1}}`,
	`{"ts":"2026-01-01T00:00:05Z","schema":1,"source":"pi","kind":"host_run","run_id":"r1","version":"1.0.0","skill":"skill-a 1","host":{"model":"m","turns":4,"tool_calls":8,"offer":"accepted","tokens":{"input":100,"output":50,"cache_read":300,"cache_write":50}}}`,
	`{"ts":"2026-01-01T00:00:06Z","schema":1,"source":"pi","kind":"host_run","run_id":"r2","version":"1.0.0","skill":"skill-a 1","host":{"turns":2,"tool_calls":3,"offer":"declined","tokens":{"input":40,"output":10,"cache_read":60,"cache_write":0}}}`,
	`{"ts":"2026-01-01T00:00:07Z","schema":1,"source":"tsp","kind":"finding","run_id":"r2","version":"1.0.0","skill":"skill-a 1","finding":{"action":"record","class":"defect","count":1}}`,
	`{"ts":"2026-01-01T00:00:08Z","schema":1,"source":"tsp","kind":"plan","run_id":"r2","version":"1.0.0","skill":"skill-a 1","plan":{"action":"init","findings":1,"evidence":2}}`,
	`{"ts":"2026-01-01T00:00:09Z","schema":1,"source":"tsp","kind":"command","version":"2.0.0","skill":"skill-b 2","command":{"name":"plan","sub":"check","exit":0,"duration_ms":400}}`,
	`{not json`,
}

func writeCLIStatsLedger(t *testing.T, home string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "telemetry"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath(home), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryStatsReportsTheLedger(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	writeCLIStatsLedger(t, home, cliStatsFixture)

	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "stats")
	if code != 0 || errOut != "" {
		t.Fatalf("stats = %d, stderr %q", code, errOut)
	}
	for _, want := range []string{
		"lines: 10 valid, 1 malformed",
		"version 1.0.0  skill skill-a 1",
		"events 9  runs 2",
		"failure rate 100.0%",
		"p50 200ms",
		"p95 300ms",
		"check decisions: always 1, never 1",
		"blocked: 2/2 = 100.0%",
		"cache hit 65.5%",
		"host offers: accepted 1, declined 1",
		"tokens per pinned red test: 166.67",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stats output lacks %q:\n%s", want, out)
		}
	}
}

func TestTelemetryStatsJSON(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	writeCLIStatsLedger(t, home, cliStatsFixture)

	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "stats", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("stats --json = %d, stderr %q", code, errOut)
	}
	var report struct {
		ValidLines     int `json:"valid_lines"`
		MalformedLines int `json:"malformed_lines"`
		Groups         []struct {
			Version  string `json:"version"`
			Skill    string `json:"skill"`
			Events   int    `json:"events"`
			Runs     int    `json:"runs"`
			Commands []struct {
				Name        string  `json:"name"`
				Count       int     `json:"count"`
				Failures    int     `json:"failures"`
				FailureRate float64 `json:"failure_rate"`
				P50MS       int64   `json:"p50_duration_ms"`
				P95MS       int64   `json:"p95_duration_ms"`
			} `json:"commands"`
			Blocked struct {
				Checks  int      `json:"checks"`
				Blocked int      `json:"blocked"`
				Rate    *float64 `json:"rate"`
			} `json:"blocked"`
			Hosts struct {
				Runs        int      `json:"runs"`
				Input       int64    `json:"input"`
				Output      int64    `json:"output"`
				CacheRead   int64    `json:"cache_read"`
				CacheWrite  int64    `json:"cache_write"`
				CacheHit    *float64 `json:"cache_hit"`
				Turns       int64    `json:"turns"`
				ToolCalls   int64    `json:"tool_calls"`
				TurnsPerRun *float64 `json:"turns_per_run"`
				Offers      []struct {
					Offer string `json:"offer"`
					Count int    `json:"count"`
				} `json:"offers"`
			} `json:"hosts"`
			TokensPerPinnedRedTest *float64 `json:"tokens_per_pinned_red_test"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stats --json is not JSON: %v\n%s", err, out)
	}
	if report.ValidLines != 10 || report.MalformedLines != 1 || len(report.Groups) != 2 {
		t.Fatalf("report = %+v", report)
	}
	a := report.Groups[0]
	if a.Version != "1.0.0" || a.Events != 9 || a.Runs != 2 {
		t.Fatalf("group A = %+v", a)
	}
	var version *struct {
		Name        string  `json:"name"`
		Count       int     `json:"count"`
		Failures    int     `json:"failures"`
		FailureRate float64 `json:"failure_rate"`
		P50MS       int64   `json:"p50_duration_ms"`
		P95MS       int64   `json:"p95_duration_ms"`
	}
	for i := range a.Commands {
		if a.Commands[i].Name == "version" {
			version = &a.Commands[i]
		}
	}
	if version == nil || version.Count != 2 || version.Failures != 1 || version.FailureRate != 0.5 || version.P50MS != 100 || version.P95MS != 300 {
		t.Fatalf("version command = %+v", version)
	}
	if a.Blocked.Checks != 2 || a.Blocked.Blocked != 2 || a.Blocked.Rate == nil || *a.Blocked.Rate != 1 {
		t.Errorf("blocked = %+v", a.Blocked)
	}
	if a.Hosts.Runs != 2 || a.Hosts.Input != 140 || a.Hosts.Output != 60 || a.Hosts.CacheRead != 360 || a.Hosts.CacheWrite != 50 {
		t.Errorf("hosts = %+v", a.Hosts)
	}
	if a.Hosts.CacheHit == nil || math.Abs(*a.Hosts.CacheHit-360.0/550.0) > 1e-9 {
		t.Errorf("cache hit = %v, want 360/550", a.Hosts.CacheHit)
	}
	if a.Hosts.Turns != 6 || a.Hosts.ToolCalls != 11 || a.Hosts.TurnsPerRun == nil || *a.Hosts.TurnsPerRun != 3 {
		t.Errorf("turns/tool calls = %+v", a.Hosts)
	}
	if len(a.Hosts.Offers) != 2 || a.Hosts.Offers[0].Offer != "accepted" || a.Hosts.Offers[1].Offer != "declined" {
		t.Errorf("offers = %+v", a.Hosts.Offers)
	}
	if a.TokensPerPinnedRedTest == nil || math.Abs(*a.TokensPerPinnedRedTest-500.0/3.0) > 1e-9 {
		t.Errorf("tokens per pinned red test = %v, want 500/3", a.TokensPerPinnedRedTest)
	}
}

func TestTelemetryStatsEmptyLedgerSaysHowToEnable(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "stats")
	if code != 0 || errOut != "" {
		t.Fatalf("empty stats = %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "tsp feature enable telemetry") {
		t.Fatalf("empty stats does not say how to enable:\n%s", out)
	}
	out, _, code = telemetryRun(t, bin, home, "", nil, "telemetry", "stats", "--json")
	if code != 0 || !strings.Contains(out, `"valid_lines": 0`) {
		t.Fatalf("empty stats --json = %d %q", code, out)
	}
}

func TestTelemetryExportJSONL(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	writeCLIStatsLedger(t, home, cliStatsFixture)
	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "export", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("export jsonl = %d %q", code, errOut)
	}
	want := strings.Join(cliStatsFixture[:10], "\n") + "\n"
	if out != want {
		t.Fatalf("export jsonl:\n%s\nwant:\n%s", out, want)
	}
	if !strings.Contains(errOut, "1 malformed") {
		t.Errorf("export jsonl stderr = %q, want the malformed count", errOut)
	}
}

func TestTelemetryExportCSV(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	writeCLIStatsLedger(t, home, cliStatsFixture)
	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "export", "--format", "csv")
	if code != 0 {
		t.Fatalf("export csv = %d %q", code, errOut)
	}
	rows, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("export csv is not CSV: %v\n%s", err, out)
	}
	if len(rows) != 11 {
		t.Fatalf("csv rows = %d, want header + 10 valid lines", len(rows))
	}
	header := strings.Join(rows[0], ",")
	for _, column := range []string{"command.name", "host.tokens.input", "kind", "check.decision", "plan.action"} {
		if !strings.Contains(header, column) {
			t.Errorf("csv header lacks %q: %s", column, header)
		}
	}
}

func TestTelemetryExportRefusesUnknownFormat(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	out, errOut, code := telemetryRun(t, bin, home, "", nil, "telemetry", "export", "--format", "yaml")
	if code != 2 || out != "" {
		t.Fatalf("unknown format = %d %q, want 2 and no stdout", code, out)
	}
	if !strings.Contains(errOut, "format") {
		t.Fatalf("unknown format stderr = %q", errOut)
	}
}
