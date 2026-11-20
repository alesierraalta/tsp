package telemetry

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// statsFixture is a small ledger with two versions/skills, two runs, domain events whose Go types
// may not exist yet, and one malformed line. The reader must decode it generically.
var statsFixture = []string{
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

func writeStatsLedger(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), LedgerFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := ""
	for _, line := range lines {
		data += line + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func closeEnough(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

func TestLoadReportGroupsByVersionAndSkill(t *testing.T) {
	report, err := LoadReport(writeStatsLedger(t, statsFixture))
	if err != nil {
		t.Fatal(err)
	}
	if report.ValidLines != 10 || report.MalformedLines != 1 {
		t.Fatalf("lines = %d valid / %d malformed, want 10/1", report.ValidLines, report.MalformedLines)
	}
	if len(report.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(report.Groups), report.Groups)
	}
	a := report.Groups[0]
	if a.Version != "1.0.0" || a.Skill != "skill-a 1" {
		t.Fatalf("first group = %q/%q", a.Version, a.Skill)
	}
	if a.Events != 9 || a.Runs != 2 {
		t.Errorf("group A events/runs = %d/%d, want 9/2", a.Events, a.Runs)
	}
}

func TestLoadReportCommandsAndDurations(t *testing.T) {
	report, _ := LoadReport(writeStatsLedger(t, statsFixture))
	a := report.Groups[0]
	commands := map[string]CommandStats{}
	for _, command := range a.Commands {
		commands[command.Name] = command
	}
	version, ok := commands["version"]
	if !ok {
		t.Fatalf("no version command stats: %+v", a.Commands)
	}
	if version.Count != 2 || version.Failures != 1 || !closeEnough(version.FailureRate, 0.5) {
		t.Errorf("version stats = %+v, want 2 runs, 1 failure, rate 0.5", version)
	}
	if version.P50MS != 100 || version.P95MS != 300 {
		t.Errorf("version p50/p95 = %d/%d, want 100/300", version.P50MS, version.P95MS)
	}
	check, ok := commands["check"]
	if !ok || check.Count != 1 || check.Failures != 1 || check.P50MS != 200 {
		t.Errorf("check stats = %+v, want one failed 200ms run", check)
	}
	b := report.Groups[1]
	if len(b.Commands) != 1 || b.Commands[0].Name != "plan check" {
		t.Errorf("version 2 commands = %+v, want one plan check", b.Commands)
	}
}

func TestLoadReportChecksBlockedAndHosts(t *testing.T) {
	report, _ := LoadReport(writeStatsLedger(t, statsFixture))
	a := report.Groups[0]
	if len(a.CheckDecisions) != 2 || a.CheckDecisions[0].Decision != "always" || a.CheckDecisions[1].Decision != "never" {
		t.Fatalf("check decisions = %+v, want always and never", a.CheckDecisions)
	}
	if a.Blocked.Checks != 2 || a.Blocked.Blocked != 2 || a.Blocked.Rate == nil || !closeEnough(*a.Blocked.Rate, 1) {
		t.Errorf("blocked = %+v, want 2/2 = 1 (exit 2 and reason error)", a.Blocked)
	}
	h := a.Hosts
	if h.Runs != 2 || h.Input != 140 || h.Output != 60 || h.CacheRead != 360 || h.CacheWrite != 50 {
		t.Errorf("host totals = %+v, want runs 2, 140/60/360/50", h)
	}
	if h.CacheHit == nil || !closeEnough(*h.CacheHit, 360.0/550.0) {
		t.Errorf("cache hit = %v, want 360/550", h.CacheHit)
	}
	if h.Turns != 6 || h.ToolCalls != 11 {
		t.Errorf("turns/tool calls = %d/%d, want 6/11", h.Turns, h.ToolCalls)
	}
	if h.TurnsPerRun == nil || !closeEnough(*h.TurnsPerRun, 3) {
		t.Errorf("turns per run = %v, want 3", h.TurnsPerRun)
	}
	if h.ToolCallsPerRun == nil || !closeEnough(*h.ToolCallsPerRun, 5.5) {
		t.Errorf("tool calls per run = %v, want 5.5", h.ToolCallsPerRun)
	}
	if len(h.Offers) != 2 || h.Offers[0].Offer != "accepted" || h.Offers[0].Count != 1 || h.Offers[1].Offer != "declined" || h.Offers[1].Count != 1 {
		t.Errorf("offers = %+v, want accepted 1, declined 1", h.Offers)
	}
	if a.TokensPerPinnedRedTest == nil || !closeEnough(*a.TokensPerPinnedRedTest, 500.0/3.0) {
		t.Errorf("tokens per pinned red test = %v, want 500/3", a.TokensPerPinnedRedTest)
	}
}

func TestLoadReportCountsHostRunsWithoutARunID(t *testing.T) {
	report, err := LoadReport(writeStatsLedger(t, []string{
		`{"schema":1,"kind":"host_run","version":"1","skill":"s","host":{"offer":"declined"}}`,
		`{"schema":1,"kind":"host_run","run_id":"r1","version":"1","skill":"s","host":{"turns":1}}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	group := report.Groups[0]
	if group.Runs != 1 {
		t.Errorf("runs = %d, want 1 distinct non-empty run id", group.Runs)
	}
	if group.Hosts.Runs != 2 {
		t.Errorf("host runs = %d, want every host_run event counted", group.Hosts.Runs)
	}
	if len(group.Hosts.Offers) != 1 || group.Hosts.Offers[0].Offer != "declined" {
		t.Errorf("offers = %+v, want declined 1", group.Hosts.Offers)
	}
}

func TestLoadReportCacheHitNeedsAllThreeCounters(t *testing.T) {
	path := writeStatsLedger(t, []string{
		`{"schema":1,"kind":"host_run","version":"1","skill":"s","host":{"tokens":{"input":10,"cache_read":90}}}`,
		`{"schema":1,"kind":"host_run","version":"1","skill":"s","host":{"tokens":{"input":10,"cache_read":90,"cache_write":0}}}`,
	})
	report, err := LoadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	h := report.Groups[0].Hosts
	if h.CacheHit == nil || !closeEnough(*h.CacheHit, 0.9) {
		t.Fatalf("cache hit = %v, want 0.9 only from the run with all three counters", h.CacheHit)
	}
}

func TestLoadReportAbsentLedgerIsEmptyNotAnError(t *testing.T) {
	report, err := LoadReport(filepath.Join(t.TempDir(), "missing", LedgerFile))
	if err != nil {
		t.Fatalf("absent ledger = %v, want an empty report", err)
	}
	if report.ValidLines != 0 || report.MalformedLines != 0 || len(report.Groups) != 0 {
		t.Fatalf("absent ledger report = %+v, want empty", report)
	}
}

func TestReportJSONUsesStableFieldNames(t *testing.T) {
	report, _ := LoadReport(writeStatsLedger(t, statsFixture))
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"path", "valid_lines", "malformed_lines", "groups"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("report JSON lacks %q: %s", key, data)
		}
	}
}
