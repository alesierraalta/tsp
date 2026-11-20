package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/feature"
	"github.com/alesierraalta/tsp/internal/filelock"
)

// hashedID is the shape the cursor stores: a sha256 hex prefix of the last message id, never the id
// itself. The test computes it independently so the contract is the format, not the helper.
func hashedID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:16])
}

// claudeTranscript is a real-shaped Claude Code transcript: one user prompt, one assistant message
// Claude wrote as three separate JSONL lines (each repeating the same message.id and the same
// usage), a tool result marked as an error, a second assistant message, and one line that is not
// JSON at all — the last line a transcript can always end with.
func claudeTranscript() string {
	return strings.Join([]string{
		`{"type":"user","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"run tsp"}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[{"type":"text","text":"working"}]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[{"type":"tool_use","id":"toolu_2","name":"Read","input":{"file_path":"secret"}}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"boom"}]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:04Z","message":{"id":"msg_2","model":"claude-sonnet-4","usage":{"input_tokens":3,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[{"type":"text","text":"done"}]}}`,
		`not json at all`,
	}, "\n") + "\n"
}

// claudeNextTurn appends the turn a later Stop must count, and nothing already counted.
func claudeNextTurn() string {
	return `{"type":"assistant","timestamp":"2026-01-01T00:00:10Z","message":{"id":"msg_3","model":"claude-sonnet-4","usage":{"input_tokens":7,"output_tokens":8,"cache_read_input_tokens":200,"cache_creation_input_tokens":9},"content":[{"type":"tool_use","id":"toolu_3","name":"Skill","input":{"skill":"tsp"}}]}}` + "\n" +
		`{"type":"user","timestamp":"2026-01-01T00:00:11Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_3","content":"ok"}]}}` + "\n"
}

// One message.id is one turn: Claude repeats the same usage on every content-block line, so the
// tokens are counted once, while the tool calls those lines carry are counted with it.
func TestParseTranscriptDedupesAssistantLinesSharingOneMessageID(t *testing.T) {
	transcript, err := ParseTranscript(strings.NewReader(claudeTranscript()))
	if err != nil {
		t.Fatalf("parse transcript: %v", err)
	}
	if len(transcript.Turns) != 2 {
		t.Fatalf("turns = %d, want 2 (msg_1 and msg_2)", len(transcript.Turns))
	}
	first := transcript.Turns[0]
	if first.ID != "msg_1" || first.Model != "claude-sonnet-4" {
		t.Fatalf("first turn = %#v", first)
	}
	for name, got := range map[string]*int64{
		"input": first.Tokens.Input, "output": first.Tokens.Output,
		"cache_read": first.Tokens.CacheRead, "cache_write": first.Tokens.CacheWrite,
	} {
		if got == nil {
			t.Fatalf("first turn %s is absent, want the repeated usage counted once", name)
		}
	}
	if *first.Tokens.Input != 10 || *first.Tokens.Output != 2 || *first.Tokens.CacheRead != 100 || *first.Tokens.CacheWrite != 5 {
		t.Fatalf("first turn tokens = %+v, want input 10 output 2 cache_read 100 cache_write 5", first.Tokens)
	}
	if first.Tools["Bash"] != 1 || first.Tools["Read"] != 1 {
		t.Fatalf("first turn tools = %v, want one Bash and one Read", first.Tools)
	}
	if first.ToolErrors != 1 {
		t.Fatalf("first turn tool errors = %d, want the is_error tool_result counted", first.ToolErrors)
	}
	// A measured zero is a value, not an absent field.
	if second := transcript.Turns[1]; second.Tokens.CacheRead == nil || *second.Tokens.CacheRead != 0 || second.Tokens.CacheWrite == nil {
		t.Fatalf("second turn tokens = %+v, want measured zeros for the cache counts", second.Tokens)
	}

	event := transcript.HostEvent("sess-claude-raw")
	if event.Source != SourceClaude || event.Kind != KindHostRun || event.Session != "sess-claude-raw" {
		t.Fatalf("event envelope = %+v, want source claude and raw session kept for Flush to seal", event)
	}
	host := event.Host
	if host == nil {
		t.Fatal("host body is nil")
	}
	if host.Model != "claude-sonnet-4" {
		t.Fatalf("model = %q", host.Model)
	}
	if host.Turns == nil || *host.Turns != 2 {
		t.Fatalf("turns = %v, want 2", host.Turns)
	}
	if host.ToolCalls == nil || *host.ToolCalls != 2 {
		t.Fatalf("tool calls = %v, want 2", host.ToolCalls)
	}
	if host.ToolErrors == nil || *host.ToolErrors != 1 {
		t.Fatalf("tool errors = %v, want 1", host.ToolErrors)
	}
	if host.WallMS == nil || *host.WallMS != 4000 {
		t.Fatalf("wall_ms = %v, want 4000 from the transcript timestamps", host.WallMS)
	}
	if host.Tokens == nil || host.Tokens.Input == nil || *host.Tokens.Input != 13 || *host.Tokens.Output != 6 {
		t.Fatalf("run tokens = %+v, want input 13 output 6", host.Tokens)
	}
	if len(host.PerTurn) != 2 {
		t.Fatalf("per_turn = %v, want the two deduped turns", host.PerTurn)
	}
}

// Two Stops over one growing transcript: the first counts its turns, the second only the turns the
// cursor has not seen, and a third Stop with nothing new writes nothing.
func TestCaptureClaudeCountsOnlyNewTurns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}

	counted, err := CaptureClaude("sess-claude-raw", strings.NewReader(claudeTranscript()))
	if err != nil || !counted {
		t.Fatalf("first capture = %v, %v; want true, nil", counted, err)
	}
	events := takeBuffer()
	if len(events) != 1 || events[0].Host == nil {
		t.Fatalf("first capture events = %+v, want one host_run", events)
	}
	if *events[0].Host.Turns != 2 {
		t.Fatalf("first capture turns = %d, want 2", *events[0].Host.Turns)
	}

	dir := filepath.Join(home, DirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read telemetry dir: %v", err)
	}
	cursorName := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "cursor-") {
			cursorName = entry.Name()
		}
	}
	if cursorName == "" {
		t.Fatalf("no cursor file in %s: %v", dir, entries)
	}
	if strings.Contains(cursorName, "sess-claude-raw") {
		t.Fatalf("cursor file %q carries the raw session id", cursorName)
	}
	cursorBody, err := os.ReadFile(filepath.Join(dir, cursorName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cursorBody), "sess-claude-raw") {
		t.Fatalf("cursor body carries the raw session id: %s", cursorBody)
	}

	second := claudeTranscript() + claudeNextTurn()
	counted, err = CaptureClaude("sess-claude-raw", strings.NewReader(second))
	if err != nil || !counted {
		t.Fatalf("second capture = %v, %v; want true, nil", counted, err)
	}
	events = takeBuffer()
	if len(events) != 1 || events[0].Host == nil {
		t.Fatalf("second capture events = %+v, want one host_run", events)
	}
	host := events[0].Host
	if host.Turns == nil || *host.Turns != 1 {
		t.Fatalf("second capture turns = %v, want only the new turn", host.Turns)
	}
	if host.Tokens == nil || host.Tokens.Input == nil || *host.Tokens.Input != 7 || *host.Tokens.Output != 8 {
		t.Fatalf("second capture tokens = %+v, want only msg_3's input 7 output 8", host.Tokens)
	}
	if host.ToolCalls == nil || *host.ToolCalls != 1 || host.Tools["Skill"] != 1 {
		t.Fatalf("second capture tool calls = %v tools = %v, want one Skill", host.ToolCalls, host.Tools)
	}
	if host.WallMS == nil || *host.WallMS != 1000 {
		t.Fatalf("second capture wall_ms = %v, want 1000 from the new turns' timestamps", host.WallMS)
	}

	counted, err = CaptureClaude("sess-claude-raw", strings.NewReader(second))
	if err != nil || counted {
		t.Fatalf("third capture = %v, %v; want false, nil: nothing new was written", counted, err)
	}
	if events := takeBuffer(); len(events) != 0 {
		t.Fatalf("third capture noted %+v, want nothing", events)
	}
}

// Telemetry off is the absence of everything: no event, no telemetry directory and no cursor.
func TestCaptureClaudeWritesNothingWhenTelemetryIsOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	counted, err := CaptureClaude("sess-claude-raw", strings.NewReader(claudeTranscript()))
	if err != nil || counted {
		t.Fatalf("capture with telemetry off = %v, %v; want false, nil", counted, err)
	}
	if events := takeBuffer(); len(events) != 0 {
		t.Fatalf("telemetry off noted %+v", events)
	}
	if _, err := os.Stat(filepath.Join(home, DirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("telemetry off created %s (err %v)", filepath.Join(home, DirName), err)
	}
}

// A transcript with no assistant usage yet is not a run: nothing is noted and no cursor is written.
func TestCaptureClaudeWithoutAssistantTurnsWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}
	counted, err := CaptureClaude("sess-claude-raw", strings.NewReader(`{"type":"user","message":{"role":"user","content":"go"}}`))
	if err != nil || counted {
		t.Fatalf("capture without turns = %v, %v; want false, nil", counted, err)
	}
	if events := takeBuffer(); len(events) != 0 {
		t.Fatalf("capture without turns noted %+v", events)
	}
	if _, err := os.Stat(filepath.Join(home, DirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capture without turns created %s (err %v)", filepath.Join(home, DirName), err)
	}
}

// A model the record allow-list would refuse is not a model: the transcript may carry prose here, so
// the field stays absent instead of persisting what the pattern refuses.
func TestParseTranscriptOmitsModelOutsideTheRecordPattern(t *testing.T) {
	transcript, err := ParseTranscript(strings.NewReader(strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","message":{"id":"msg_1","model":"claude sonnet with spaces","usage":{"input_tokens":5,"output_tokens":1},"content":[]}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"msg_2","model":"claude-haiku-4","usage":{"input_tokens":1,"output_tokens":1},"content":[]}}`,
	}, "\n") + "\n"))
	if err != nil {
		t.Fatalf("parse transcript: %v", err)
	}
	if len(transcript.Turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(transcript.Turns))
	}
	if transcript.Turns[0].Model != "" {
		t.Fatalf("turn model = %q, want absent: it does not match the record pattern", transcript.Turns[0].Model)
	}
	if transcript.Turns[1].Model != "claude-haiku-4" {
		t.Fatalf("turn model = %q, want the valid model kept", transcript.Turns[1].Model)
	}
	// The first valid model the transcript carries is what the run reports; the refused one is not.
	if transcript.HostEvent("sess-claude-raw").Host.Model != "claude-haiku-4" {
		t.Fatalf("host model = %q, want only the model the record pattern accepts", transcript.HostEvent("sess-claude-raw").Host.Model)
	}
}

// A negative or fractional counter was never measured: it stays absent and is never summed, while
// the turn and its valid counters survive.
func TestParseTranscriptOmitsNegativeAndFractionalUsage(t *testing.T) {
	transcript, err := ParseTranscript(strings.NewReader(
		`{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","message":{"id":"msg_1","model":"claude-sonnet-4","usage":{"input_tokens":-5,"output_tokens":3,"cache_read_input_tokens":1.5,"cache_creation_input_tokens":4},"content":[]}}` + "\n"))
	if err != nil {
		t.Fatalf("parse transcript: %v", err)
	}
	if len(transcript.Turns) != 1 {
		t.Fatalf("turns = %d, want the turn kept with only its valid counters", len(transcript.Turns))
	}
	tokens := transcript.Turns[0].Tokens
	if tokens.Input != nil || tokens.CacheRead != nil {
		t.Fatalf("tokens = %+v, want the negative and fractional counters absent", tokens)
	}
	if tokens.Output == nil || *tokens.Output != 3 || tokens.CacheWrite == nil || *tokens.CacheWrite != 4 {
		t.Fatalf("tokens = %+v, want the valid counters kept", tokens)
	}
	host := transcript.HostEvent("sess-claude-raw").Host
	if host.Tokens == nil || host.Tokens.Input != nil || host.Tokens.CacheRead != nil {
		t.Fatalf("run tokens = %+v, want only the measured counters summed", host.Tokens)
	}
	if host.Tokens.Output == nil || *host.Tokens.Output != 3 {
		t.Fatalf("run tokens = %+v, want output 3", host.Tokens)
	}
	if len(host.PerTurn) != 1 || host.PerTurn[0].Input != nil {
		t.Fatalf("per_turn = %+v, want the refused counter absent there too", host.PerTurn)
	}
}

// The cursor body is a high-water mark, not a copy of the transcript: it carries only a hash of the
// last message id, and the next Stop aligns by hashing the transcript's ids the same way.
func TestCaptureClaudeStoresOnlyAHashedCursorBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}
	if counted, err := CaptureClaude("sess-claude-raw", strings.NewReader(claudeTranscript())); err != nil || !counted {
		t.Fatalf("capture = %v, %v; want true, nil", counted, err)
	}
	takeBuffer()
	dir := filepath.Join(home, DirName)
	body, err := os.ReadFile(cursorPath(dir, "sess-claude-raw"))
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if strings.Contains(string(body), "msg_2") {
		t.Fatalf("cursor body carries the raw last message id: %s", body)
	}
	if !strings.Contains(string(body), hashedID("msg_2")) {
		t.Fatalf("cursor body = %s, want the hash of the last message id %q", body, hashedID("msg_2"))
	}
}

// A cursor holding a hash of a message the transcript still carries aligns there, whatever its turn
// count says: the hash is the high-water mark, not a positional guess.
func TestCaptureClaudeAlignsOnTheHashedCursor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, DirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := cursorPath(dir, "sess-claude-raw")
	if err := os.WriteFile(path, []byte(`{"turns":0,"last_hash":"`+hashedID("msg_1")+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	counted, err := CaptureClaude("sess-claude-raw", strings.NewReader(claudeTranscript()))
	if err != nil || !counted {
		t.Fatalf("capture = %v, %v; want true, nil", counted, err)
	}
	events := takeBuffer()
	if len(events) != 1 || events[0].Host == nil {
		t.Fatalf("capture events = %+v, want one host_run", events)
	}
	host := events[0].Host
	if host.Turns == nil || *host.Turns != 1 || host.Tokens == nil || host.Tokens.Input == nil || *host.Tokens.Input != 3 {
		t.Fatalf("capture host = %+v, want only msg_2 counted once", host)
	}
}

// The cursor's read-modify-write is serialized: while another Stop holds the cursor lock, this one
// waits, so a racing Stop cannot read the same high-water mark and count the same turns again.
func TestCaptureClaudeWaitsForTheCursorLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, DirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	holder, err := os.OpenFile(cursorLockPath(cursorPath(dir, "sess-claude-raw")), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := filelock.Lock(holder); err != nil {
		holder.Close()
		t.Skipf("no cross-process file lock on this platform: %v", err)
	}
	defer func() { filelock.Unlock(holder); holder.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = CaptureClaude("sess-claude-raw", strings.NewReader(claudeTranscript()))
	}()
	select {
	case <-done:
		t.Fatal("capture finished while another Stop held the cursor lock: the cursor is not serialized")
	case <-time.After(300 * time.Millisecond):
	}
	if err := filelock.Unlock(holder); err != nil {
		t.Fatalf("release cursor lock: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("capture never finished after the cursor lock was released")
	}
	takeBuffer()
}

// Without the raw session id there is no key for the cursor, so the run cannot be counted without
// risking a double count on the next Stop.
func TestCaptureClaudeWithoutSessionWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSP_HOME", home)
	takeBuffer()
	if _, err := feature.Set(feature.TelemetryID, true); err != nil {
		t.Fatal(err)
	}
	counted, err := CaptureClaude("", strings.NewReader(claudeTranscript()))
	if err != nil || counted {
		t.Fatalf("capture without session = %v, %v; want false, nil", counted, err)
	}
	if events := takeBuffer(); len(events) != 0 {
		t.Fatalf("capture without session noted %+v", events)
	}
}
