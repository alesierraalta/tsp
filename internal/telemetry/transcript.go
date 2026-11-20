package telemetry

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/feature"
	"github.com/alesierraalta/tsp/internal/filelock"
)

// Turn is one distinct assistant message of a Claude Code transcript: its tokens are counted once
// even though Claude writes one JSONL line per content block repeating the same usage, and the tool
// calls and errors that follow it are kept with it so a later Stop can count only new turns.
type Turn struct {
	ID         string
	Model      string
	Tokens     Tokens
	Tools      map[string]int64
	ToolErrors int64
	Start      time.Time
	End        time.Time
}

// Transcript is one host transcript reduced to what a host_run carries. Turns are in transcript
// order, deduplicated by message id; First and Last bound the wall time the transcript itself marks.
type Transcript struct {
	Model string
	Turns []Turn
	First time.Time
	Last  time.Time
}

// transcriptLine is the part of one JSONL entry this package reads. Everything else — prompts,
// text, tool inputs and outputs — is deliberately not decoded, so no content can reach the ledger.
type transcriptLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Usage   *transcriptUse  `json:"usage"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// transcriptUse is one assistant message's usage, as Claude reports it. Each counter decodes
// leniently: a value the package cannot count as a non-negative integer was never measured, so it
// stays absent instead of turning a whole turn into a parse failure or a negative total.
type transcriptUse struct {
	Input      transcriptCount `json:"input_tokens"`
	Output     transcriptCount `json:"output_tokens"`
	CacheRead  transcriptCount `json:"cache_read_input_tokens"`
	CacheWrite transcriptCount `json:"cache_creation_input_tokens"`
}

// transcriptCount is one wire counter reduced to what may be persisted: a non-negative integer, or
// nothing. Decoding never fails, because an uncountable field is an absent measurement, not a
// reason to drop the message that carried it.
type transcriptCount struct {
	value *int64
}

// UnmarshalJSON keeps a non-negative integer and drops everything else: a negative, a fraction, a
// string that is not an integer, null and an absent field all read as not measured.
func (c *transcriptCount) UnmarshalJSON(data []byte) error {
	c.value = nil
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return nil
	}
	text = strings.Trim(text, `"`)
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < 0 {
		return nil
	}
	c.value = &value
	return nil
}

// transcriptBlock is the only part of a content block this package reads: a tool call's name and a
// tool result's error flag. No text, input or output is decoded.
type transcriptBlock struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

// ParseTranscript reads one Claude Code transcript JSONL stream. Lines it cannot read are skipped:
// a transcript can end with a line the host was still writing, and that is not a reason to lose the
// whole measurement. One message.id is one turn, however many content-block lines repeat it.
func ParseTranscript(r io.Reader) (Transcript, error) {
	var transcript Transcript
	reader := bufio.NewReaderSize(r, 64*1024)
	byID := map[string]int{}
	current := -1
	for {
		line, readErr := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			current = transcript.consumeLine(line, byID, current)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return transcript, fmt.Errorf("read transcript: %w", readErr)
		}
	}
	return transcript, nil
}

// consumeLine folds one JSONL line into the transcript and answers the turn a following tool result
// belongs to. It returns the unchanged current turn for a line that carries no assistant message.
func (t *Transcript) consumeLine(line string, byID map[string]int, current int) int {
	var parsed transcriptLine
	if json.Unmarshal([]byte(line), &parsed) != nil {
		return current
	}
	stamp, stamped := parseTranscriptTime(parsed.Timestamp)
	if stamped {
		if t.First.IsZero() || stamp.Before(t.First) {
			t.First = stamp
		}
		if t.Last.IsZero() || stamp.After(t.Last) {
			t.Last = stamp
		}
	}
	blocks := parseTranscriptBlocks(parsed.Message.Content)
	switch parsed.Type {
	case "assistant":
		index, fresh := -1, false
		if parsed.Message.ID != "" {
			if known, ok := byID[parsed.Message.ID]; ok {
				index = known
			}
		}
		if index < 0 {
			t.Turns = append(t.Turns, Turn{ID: parsed.Message.ID, Model: transcriptModel(parsed.Message.Model), Start: stamp, End: stamp})
			index = len(t.Turns) - 1
			if parsed.Message.ID != "" {
				byID[parsed.Message.ID] = index
			}
			fresh = true
			if t.Model == "" && parsed.Message.Model != "" {
				t.Model = transcriptModel(parsed.Message.Model)
			}
		}
		turn := &t.Turns[index]
		if turn.Model == "" && parsed.Message.Model != "" {
			turn.Model = transcriptModel(parsed.Message.Model)
		}
		if fresh && parsed.Message.Usage != nil {
			turn.Tokens = parsed.Message.Usage.tokens()
		}
		if stamped {
			if turn.Start.IsZero() || stamp.Before(turn.Start) {
				turn.Start = stamp
			}
			if turn.End.IsZero() || stamp.After(turn.End) {
				turn.End = stamp
			}
		}
		for _, block := range blocks {
			if block.Type != "tool_use" || !toolNameRe.MatchString(block.Name) {
				continue
			}
			if turn.Tools == nil {
				turn.Tools = map[string]int64{}
			}
			if _, known := turn.Tools[block.Name]; !known && len(turn.Tools) >= maxTools {
				continue
			}
			turn.Tools[block.Name]++
		}
		return index
	case "user":
		if current < 0 {
			return current
		}
		turn := &t.Turns[current]
		for _, block := range blocks {
			if block.Type == "tool_result" && block.IsError {
				turn.ToolErrors++
			}
		}
		if stamped {
			if turn.End.IsZero() || stamp.After(turn.End) {
				turn.End = stamp
			}
		}
		return current
	default:
		return current
	}
}

// tokens converts the wire usage into this package's count shape, keeping every measured zero and
// dropping every counter the wire could not state as a non-negative integer.
func (u *transcriptUse) tokens() Tokens {
	return Tokens{Input: u.Input.value, Output: u.Output.value, CacheRead: u.CacheRead.value, CacheWrite: u.CacheWrite.value}
}

// transcriptModel keeps a model the record allow-list would accept and drops anything else, so a
// transcript cannot smuggle free text into the model field `tsp telemetry record` refuses.
func transcriptModel(model string) string {
	if model == "" || !modelRe.MatchString(model) {
		return ""
	}
	return model
}

// parseTranscriptBlocks decodes the array-shaped content Claude writes. A string content (a user
// prompt) and any shape this package does not know decode to no blocks.
func parseTranscriptBlocks(raw json.RawMessage) []transcriptBlock {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []transcriptBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	return blocks
}

// parseTranscriptTime reads the timestamp a transcript line carries; a malformed one leaves the line
// without a wall-time contribution instead of failing the parse.
func parseTranscriptTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// HostEvent turns a transcript into the host_run event one Stop notes. The session stays raw here
// because Flush is the only place the salt exists.
func (t Transcript) HostEvent(session string) Event {
	return Event{Source: SourceClaude, Kind: KindHostRun, Session: session, Host: t.host()}
}

// host aggregates the transcript's turns into one host body; every count it writes was measured by
// the parse, and a count the transcript never carried stays absent.
func (t Transcript) host() *Host {
	if len(t.Turns) == 0 {
		return nil
	}
	turns := int64(len(t.Turns))
	host := &Host{Model: t.Model, Turns: &turns, ToolCalls: new(int64), ToolErrors: new(int64)}
	tokens := Tokens{}
	measured := false
	tools := map[string]int64{}
	for i := range t.Turns {
		turn := &t.Turns[i]
		if turn.Model != "" && host.Model == "" {
			host.Model = turn.Model
		}
		if turn.Tools != nil {
			*host.ToolCalls += sumToolCounts(turn.Tools)
			for name, count := range turn.Tools {
				if _, known := tools[name]; !known && len(tools) >= maxTools {
					continue
				}
				tools[name] += count
			}
		}
		*host.ToolErrors += turn.ToolErrors
		if turn.Tokens.Input != nil || turn.Tokens.Output != nil || turn.Tokens.CacheRead != nil || turn.Tokens.CacheWrite != nil {
			addTokens(&tokens, turn.Tokens)
			measured = true
			if len(host.PerTurn) < maxPerTurn {
				host.PerTurn = append(host.PerTurn, turn.Tokens)
			}
		}
	}
	if measured {
		host.Tokens = &tokens
	}
	if len(tools) > 0 {
		host.Tools = tools
	}
	if !t.First.IsZero() && !t.Last.IsZero() {
		wall := t.Last.Sub(t.First).Milliseconds()
		if wall < 0 {
			wall = 0
		}
		host.WallMS = &wall
	}
	return host
}

// sumToolCounts adds one turn's tool call counts.
func sumToolCounts(tools map[string]int64) int64 {
	var total int64
	for _, count := range tools {
		total += count
	}
	return total
}

// addTokens adds one turn's counts into the run's, preserving an absent count as absent.
func addTokens(dst *Tokens, src Tokens) {
	addCount(&dst.Input, src.Input)
	addCount(&dst.Output, src.Output)
	addCount(&dst.CacheRead, src.CacheRead)
	addCount(&dst.CacheWrite, src.CacheWrite)
}

// addCount adds src into dst, allocating dst only when the source was measured.
func addCount(dst **int64, src *int64) {
	if src == nil {
		return
	}
	if *dst == nil {
		*dst = new(int64)
	}
	**dst += *src
}

// cursor is the per-session high-water mark that keeps repeated Stops from counting the same turns
// twice: the number of turns already counted and a hash of the last one's message id, so an
// append-only transcript can be aligned even if its earlier lines were rewritten. Only the hash is
// stored: the cursor body is a position, never a copy of what the host wrote.
type cursor struct {
	Turns    int64  `json:"turns"`
	LastHash string `json:"last_hash,omitempty"`
}

// hashMessageID answers the cursor's position for one message id. A hash prefix is enough to name a
// line and carries no content.
func hashMessageID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:16])
}

// CaptureClaude notes one host_run for the transcript turns this session has not been counted for
// yet and advances its cursor. It writes nothing when the telemetry feature is off, when there is no
// session to key the cursor with, or when the transcript carries no new turn. The caller ignores the
// error: telemetry never changes what the hook decides or prints.
func CaptureClaude(session string, transcript io.Reader) (bool, error) {
	session = strings.TrimSpace(session)
	if session == "" {
		return false, nil
	}
	enabled, err := feature.Enabled(feature.TelemetryID)
	if err != nil || !enabled {
		return false, nil
	}
	parsed, err := ParseTranscript(transcript)
	if err != nil {
		return false, err
	}
	if len(parsed.Turns) == 0 {
		return false, nil
	}
	dir, err := Dir()
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, fmt.Errorf("create telemetry directory: %w", err)
	}
	path := cursorPath(dir, session)
	// The whole read-modify-write is one critical section: a Stop that read the cursor before its
	// sibling advanced it would count the same turns again.
	unlock, err := lockCursor(path)
	if err != nil {
		return false, err
	}
	defer unlock()
	fresh := parsed.after(readCursor(path))
	if len(fresh.Turns) == 0 {
		return false, nil
	}
	// The cursor moves before the event is noted, and the lock is still held: a ledger line lost to a
	// failed flush is cheaper than counting the same turn again on the next Stop.
	if err := writeCursor(path, cursor{Turns: int64(len(parsed.Turns)), LastHash: lastMessageHash(parsed)}); err != nil {
		return false, err
	}
	Note(fresh.HostEvent(session))
	return true, nil
}

// lastMessageHash answers the cursor position for the transcript's last turn, and "" when that turn
// carries no message id: a hash of emptiness would name every id-less turn.
func lastMessageHash(t Transcript) string {
	if len(t.Turns) == 0 || t.Turns[len(t.Turns)-1].ID == "" {
		return ""
	}
	return hashMessageID(t.Turns[len(t.Turns)-1].ID)
}

// cursorPath hashes the raw session id: the telemetry directory never carries the host's own
// identity value in a file name.
func cursorPath(dir, session string) string {
	sum := sha256.Sum256([]byte(session))
	return filepath.Join(dir, "cursor-"+hex.EncodeToString(sum[:8])+".json")
}

// cursorLockPath is the file beside the cursor that serializes the read-modify-write of two Stops of
// the same session. It is hidden so a cursor-* listing still counts one file per session.
func cursorLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".lock")
}

// lockCursor takes the exclusive lock a Stop holds from the cursor read to the event note. A
// platform with no cross-process lock proceeds unlocked: the atomic replace still keeps the cursor
// whole, and losing the measurement is worse than an unserialized one.
func lockCursor(path string) (func(), error) {
	file, err := os.OpenFile(cursorLockPath(path), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open telemetry cursor lock: %w", err)
	}
	if err := filelock.Lock(file); err != nil {
		file.Close()
		if lockUnavailable(err) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("lock telemetry cursor: %w", err)
	}
	return func() { filelock.Unlock(file); file.Close() }, nil
}

// readCursor answers the stored high-water mark, and an absent or unreadable one as the zero
// cursor: the first Stop of a session counts from the start.
func readCursor(path string) cursor {
	body, err := os.ReadFile(path)
	if err != nil {
		return cursor{}
	}
	var stored cursor
	if json.Unmarshal(body, &stored) != nil {
		return cursor{}
	}
	return stored
}

// writeCursor stores the high-water mark for the next Stop. The body goes to a temp file in the same
// directory and is renamed onto the cursor, so a concurrent reader sees either the old cursor or the
// new one, never a torn body.
func writeCursor(path string, stored cursor) error {
	body, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode telemetry cursor: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create telemetry cursor: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("protect telemetry cursor: %w", err)
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return fmt.Errorf("write telemetry cursor: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write telemetry cursor: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace telemetry cursor: %w", err)
	}
	return nil
}

// after answers the turns this cursor has not counted. A cursor whose last id hash the transcript no
// longer carries cannot be aligned, so it claims nothing new rather than risk double counting.
func (t Transcript) after(stored cursor) Transcript {
	if len(t.Turns) == 0 {
		return Transcript{}
	}
	start := 0
	switch {
	case stored.LastHash != "":
		found := -1
		for i := range t.Turns {
			if t.Turns[i].ID != "" && hashMessageID(t.Turns[i].ID) == stored.LastHash {
				found = i
				break
			}
		}
		if found < 0 {
			return Transcript{}
		}
		start = found + 1
	case stored.Turns > 0:
		if stored.Turns >= int64(len(t.Turns)) {
			return Transcript{}
		}
		start = int(stored.Turns)
	}
	if start >= len(t.Turns) {
		return Transcript{}
	}
	sliced := Transcript{Model: t.Model, Turns: t.Turns[start:]}
	if start == 0 {
		// The first Stop measures the whole transcript, prompt included, so its wall time starts
		// where the session did rather than where the first assistant message did.
		sliced.First, sliced.Last = t.First, t.Last
	}
	for i := range sliced.Turns {
		turn := sliced.Turns[i]
		if sliced.Model == "" && turn.Model != "" {
			sliced.Model = turn.Model
		}
		if !turn.Start.IsZero() && (sliced.First.IsZero() || turn.Start.Before(sliced.First)) {
			sliced.First = turn.Start
		}
		if !turn.End.IsZero() && (sliced.Last.IsZero() || turn.End.After(sliced.Last)) {
			sliced.Last = turn.End
		}
	}
	return sliced
}
