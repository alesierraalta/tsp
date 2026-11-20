// Package telemetry is the local, content-free ledger of what tsp did: one append-only JSONL file,
// one event per line, written only when the `telemetry` feature is on. It knows counts, enums,
// durations, versions and salted pseudonyms; it never knows code, prompts, finding text, flag
// values, positional arguments or paths. A fact nobody measured is an absent field, never a zero.
//
// The package imports state, feature, sanitize and filelock only, so any package may call Note
// without risking an import cycle.
package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alesierraalta/tsp/internal/feature"
	"github.com/alesierraalta/tsp/internal/filelock"
	"github.com/alesierraalta/tsp/internal/sanitize"
	"github.com/alesierraalta/tsp/internal/state"
)

// Schema is the version of the line format every event carries; a reader refuses what it does not know.
const Schema = 1

// LedgerFile is the one file, inside the telemetry directory, that events are appended to.
const LedgerFile = "events.jsonl"

// DirName is the telemetry directory under the state root.
const DirName = "telemetry"

// RunIDEnv is the variable a host sets to make every tsp it spawns one run.
const RunIDEnv = "TSP_RUN_ID"

// The sources a host event may name.
const (
	SourceTSP      = "tsp"
	SourcePi       = "pi"
	SourceClaude   = "claude"
	SourceOpencode = "opencode"
)

// The event kinds this package writes; domain events (#489) add theirs beside these.
const (
	KindCommand = "command"
	KindHostRun = "host_run"
)

// The offers a host run may name.
const (
	OfferAccepted = "accepted"
	OfferDeclined = "declined"
	OfferAborted  = "aborted"
	OfferNone     = "none"
)

// Base is what the process knows and this package cannot: the build and the skill identity. main
// passes it so the package never imports buildinfo or feedback.
type Base struct {
	Version string // buildinfo.Version
	Commit  string // buildinfo.Revision()
	Skill   string // feedback.EmbeddedSkillIdentity()
}

// Event is one ledger line: the envelope every kind shares plus at most one kind's body.
type Event struct {
	TS      string `json:"ts"`
	Schema  int    `json:"schema"`
	Source  string `json:"source"`
	Kind    string `json:"kind"`
	RunID   string `json:"run_id,omitempty"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Skill   string `json:"skill"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	CPUs    int    `json:"cpus"`
	Repo    string `json:"repo,omitempty"`
	Machine string `json:"machine,omitempty"`
	// Session is the raw host session id a caller notes; Flush replaces it with a salted pseudonym
	// before anything is written, so the ledger never carries the host's own identity value.
	Session string `json:"session,omitempty"`

	Command *Command `json:"command,omitempty"`
	Host    *Host    `json:"host,omitempty"`
}

// Command is one subcommand's event: its name, the documented sub it ran, the flag NAMES it was
// given, its exit code and its wall time.
type Command struct {
	Name       string   `json:"name"`
	Sub        string   `json:"sub,omitempty"`
	Flags      []string `json:"flags,omitempty"`
	Exit       int      `json:"exit"`
	DurationMS int64    `json:"duration_ms"`
}

// Host is one host run's usage, as the host adapter reports it. Every count is a pointer so a
// field the host never measured stays absent instead of reading as zero.
type Host struct {
	Model       string           `json:"model,omitempty"`
	Turns       *int64           `json:"turns,omitempty"`
	ToolCalls   *int64           `json:"tool_calls,omitempty"`
	ToolErrors  *int64           `json:"tool_errors,omitempty"`
	Compactions *int64           `json:"compactions,omitempty"`
	Subagents   *int64           `json:"subagents,omitempty"`
	WallMS      *int64           `json:"wall_ms,omitempty"`
	FirstTSPMS  *int64           `json:"first_tsp_ms,omitempty"`
	Tokens      *Tokens          `json:"tokens,omitempty"`
	CostUSD     *float64         `json:"cost_usd,omitempty"`
	PerTurn     []Tokens         `json:"per_turn,omitempty"`
	Tools       map[string]int64 `json:"tools,omitempty"`
	Offer       string           `json:"offer,omitempty"`
}

// Tokens is one turn's or one run's token counts; absent stays absent.
type Tokens struct {
	Input      *int64 `json:"input,omitempty"`
	Output     *int64 `json:"output,omitempty"`
	CacheRead  *int64 `json:"cache_read,omitempty"`
	CacheWrite *int64 `json:"cache_write,omitempty"`
}

var (
	bufferMu sync.Mutex
	buffer   []Event
)

// Note keeps one event in memory until the process's single Flush. It never writes, never fails and
// never changes what the command does; a caller may call it with telemetry off.
func Note(event Event) {
	bufferMu.Lock()
	defer bufferMu.Unlock()
	buffer = append(buffer, event)
}

// takeBuffer returns the buffered events and empties the buffer, so one process flushes once.
func takeBuffer() []Event {
	bufferMu.Lock()
	defer bufferMu.Unlock()
	events := buffer
	buffer = nil
	return events
}

// Dir answers the telemetry directory under the state root; it creates nothing.
func Dir() (string, error) {
	root, err := state.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, DirName), nil
}

// Path answers the ledger path under the state root; it creates nothing.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, LedgerFile), nil
}

// DirReadOnly answers the telemetry directory under the state root without migrating a legacy root,
// creating a directory or writing an alias. Reading the ledger must not move a machine that never
// opted in.
func DirReadOnly() (string, error) {
	root, err := state.RootReadOnly()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, DirName), nil
}

// PathReadOnly answers the ledger path read-only; it creates nothing and migrates nothing.
func PathReadOnly() (string, error) {
	dir, err := DirReadOnly()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, LedgerFile), nil
}

// Run is one process's telemetry: the gate as it stood before the command ran, plus the events the
// command noted. Reading the gate up front is what makes `tsp feature enable telemetry` the first
// recorded run and not one recorded after the fact, and what keeps `disable` from erasing its own
// command event.
type Run struct {
	recorded bool
	err      error
}

// Begin reads the telemetry gate once, before the subcommand runs, without migrating or creating
// anything: an opted-out machine must stay exactly as it was. An unreadable state is not a reason to
// change a command, so it reads as off and silently loses only the events it could not gate.
func Begin() Run {
	enabled, err := feature.EnabledReadOnly(feature.TelemetryID)
	if err != nil {
		return Run{recorded: false}
	}
	return Run{recorded: enabled}
}

// Flush is Begin's answer plus the one write the run makes: it stamps the buffered events and the
// command event with the envelope, then appends every line in one locked write. When the telemetry
// feature was off at Begin, or the run noted nothing and has no subcommand to describe, it touches
// nothing: an opted-out install carries no directory, no salt and no file.
func (r Run) Flush(base Base, command Command) error {
	if r.err != nil {
		takeBuffer()
		return r.err
	}
	events := takeBuffer()
	if len(events) == 0 && command.Name == "" {
		return nil
	}
	if !r.recorded {
		return nil
	}
	if command.Name != "" {
		events = append(events, Event{Source: SourceTSP, Kind: KindCommand, Command: &command})
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	key, err := sanitize.LoadKeyIn(dir)
	if err != nil {
		return err
	}
	repo := pseudonym(key, "repo", gitCommonDir())
	machine := pseudonym(key, "machine", hostname())
	runID := strings.TrimSpace(os.Getenv(RunIDEnv))
	if !identityRe.MatchString(runID) {
		// The host's environment is not the ledger's schema: a value outside the identifier pattern
		// stays absent instead of being copied raw.
		runID = ""
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range events {
		stamp(&events[i], base, key, runID, repo, machine, now)
	}
	payload, err := marshalLines(events)
	if err != nil {
		return err
	}
	return appendLocked(filepath.Join(dir, LedgerFile), payload)
}

// stamp fills the envelope a caller does not fill itself. A value already set wins, so a domain
// event may carry its own run id or source; "not measured" stays absent.
func stamp(event *Event, base Base, key *sanitize.Key, runID, repo, machine, now string) {
	if event.TS == "" {
		event.TS = now
	}
	if event.Schema == 0 {
		event.Schema = Schema
	}
	if event.Source == "" {
		event.Source = SourceTSP
	}
	if event.Version == "" {
		event.Version = base.Version
	}
	if event.Commit == "" {
		event.Commit = base.Commit
	}
	if event.Skill == "" {
		event.Skill = base.Skill
	}
	if event.OS == "" {
		event.OS = runtime.GOOS
	}
	if event.Arch == "" {
		event.Arch = runtime.GOARCH
	}
	if event.CPUs == 0 {
		event.CPUs = runtime.NumCPU()
	}
	if event.RunID == "" {
		event.RunID = runID
	}
	if event.Repo == "" {
		event.Repo = repo
	}
	if event.Machine == "" {
		event.Machine = machine
	}
	if event.Session != "" {
		event.Session = key.ID("session", event.Session)
	}
}

// marshalLines renders one JSON object per line, newline terminated, as one write's payload.
func marshalLines(events []Event) ([]byte, error) {
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	for i := range events {
		if err := encoder.Encode(&events[i]); err != nil {
			return nil, fmt.Errorf("encode telemetry event: %w", err)
		}
	}
	return payload.Bytes(), nil
}

// appendLocked appends payload to the ledger in one write under an exclusive advisory lock, so
// concurrent tsp processes never interleave their lines.
func appendLocked(path string, payload []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open telemetry ledger: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return fmt.Errorf("protect telemetry ledger: %w", err)
	}
	switch err := filelock.Lock(file); {
	case err == nil:
		defer filelock.Unlock(file)
	case lockUnavailable(err):
		// This platform has no cross-process lock; the single O_APPEND write is the best it has, and
		// losing the event is worse than appending it unserialized.
	default:
		return fmt.Errorf("lock telemetry ledger: %w", err)
	}
	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("write telemetry ledger: %w", err)
	}
	return nil
}

// pseudonym seals one identity value, and answers "" when the value is unknown so the caller's
// field stays absent instead of naming a value nobody read.
func pseudonym(key *sanitize.Key, prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return key.ID(prefix, value)
}

// gitCommonDir answers the canonical common git directory of the working tree, which names one
// repository across its worktrees; an empty answer leaves the repo field absent.
func gitCommonDir() string {
	cmd := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hostname answers this machine's name, empty when the host will not say.
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}
