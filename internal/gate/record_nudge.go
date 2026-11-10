package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/citation"
	"github.com/alesierraalta/tsp/internal/plan"
)

// SkippedNudgeAccepted records the settle that follows a recording nudge: the host asked once, and a
// later settle without the row is accepted rather than asked again.
const SkippedNudgeAccepted = "nudge_accepted"

// SkippedNudgeUnmarkable records a stop that owed a recording but had nowhere to remember asking:
// no marker directory means no once-per-session promise, so the hook stays silent instead of nagging.
const SkippedNudgeUnmarkable = "nudge_unmarkable"

// editingTools are the calls that write a file. A session's own calls are the only proof this gate
// accepts that it authored a change: the worktree shows what is dirty, never who dirtied it.
var editingTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// toolBlock is one content block of a transcript message.
type toolBlock struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Input struct {
		FilePath string `json:"file_path"`
	} `json:"input"`
}

// transcriptLine is one JSONL row: Claude Code nests the blocks under `message`, and a host that
// writes them at the top level is read the same way instead of dropped.
type transcriptLine struct {
	Content []toolBlock `json:"content"`
	Message struct {
		Content []toolBlock `json:"content"`
	} `json:"message"`
}

// EditedFiles returns every path this session edited through a tool call, once each and in
// transcript order. A line that is not JSON is skipped: a transcript the host is still writing, or
// one carrying a shape this hook does not know, must never break the stop.
func EditedFiles(r io.Reader) []string {
	if r == nil {
		return []string{}
	}
	found := []string{}
	seen := map[string]bool{}
	add := func(blocks []toolBlock) {
		for _, b := range blocks {
			if b.Type != "tool_use" || !editingTools[b.Name] || b.Input.FilePath == "" {
				continue
			}
			if !seen[b.Input.FilePath] {
				seen[b.Input.FilePath] = true
				found = append(found, b.Input.FilePath)
			}
		}
	}
	eachLine(r, func(line string) bool {
		var row transcriptLine
		if json.Unmarshal([]byte(line), &row) != nil {
			return true
		}
		add(row.Content)
		add(row.Message.Content)
		return true
	})
	return found
}

// recordingOwed answers the nudge's question: the first test file this session edited that no
// Findings row records. A plan it cannot read owes nothing — the audit reports an unreadable plan,
// and a hook that refused a stop on evidence it could not read would be guessing.
func recordingOwed(d Deps, root, planRel string, edited []string) (string, bool) {
	body, err := readPlan(d, filepath.Join(root, planRel))
	if err != nil {
		return "", false
	}
	_, rows := plan.Table(body, "Findings")
	for _, path := range edited {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !bench.IsTestFile(rel) || recordsTest(rows, rel) {
			continue
		}
		return "tsp: this session edited " + rel + " and no Findings row records it, so the defect that test pins is lost to every reader of " +
			planRel + ". Record it with `tsp plan add-finding`, naming that test in its pinning-test column, then run `tsp plan check --pins`. This is asked once per session.", true
	}
	return "", false
}

// recordsTest reports whether a Findings row names the test: the pinning-test column carries it, and
// a row that cites it anywhere else records it just as well.
func recordsTest(rows [][]string, rel string) bool {
	for _, row := range rows {
		for _, c := range citation.Extract(strings.Join(row, " ")) {
			if citation.SamePath(c.Path, rel) {
				return true
			}
		}
	}
	return false
}

// nudgeMarkerPath names the file that remembers this session was already asked in this repository.
// It is keyed by root and session, exactly like the binding it belongs to: two worktrees under one
// session are two plans, and one ask must not silence the other's debt. It sits beside the bindings,
// under the same hashed-name discipline, so no filename carries a raw session ID. No directory means
// no marker, and the caller refuses nothing rather than nagging a session it cannot remember.
func nudgeMarkerPath(dir, root, sessionID string) string {
	if dir == "" || root == "" || sessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("nudge\x00" + root + "\x00" + sessionID))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".nudged")
}

func nudged(dir, root, sessionID string) bool {
	p := nudgeMarkerPath(dir, root, sessionID)
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// markNudged never fails the turn: a marker that cannot be written costs one repeated question, and
// a stop is not worth breaking over it.
func markNudged(dir, root, sessionID string) {
	p := nudgeMarkerPath(dir, root, sessionID)
	if p == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(p, []byte("nudged\n"), 0o600)
}

// nudge asks the model once, in this session, to record an edited test the plan does not carry. It
// runs only for a bound session that invoked the discipline: a session with no plan has nothing to
// record, and one that never ran the discipline has nothing to report.
func nudge(d Deps, root, planRel string, in Input, entry *Entry) (Result, bool) {
	rc := openOrNil(d, in.TranscriptPath)
	if rc == nil {
		return Result{}, false
	}
	edited := EditedFiles(rc)
	rc.Close()
	reason, owed := recordingOwed(d, root, planRel, edited)
	if !owed {
		return Result{}, false
	}
	if nudgeMarkerPath(d.BindingDir, root, in.SessionID) == "" {
		// No marker directory is no memory of the ask, and a hook that refused every stop would nag a
		// session it cannot remember until the host overrides it: silence, with the reason logged.
		entry.Skipped = SkippedNudgeUnmarkable
		return Result{Entry: entry}, true
	}
	if nudged(d.BindingDir, root, in.SessionID) {
		entry.Skipped = SkippedNudgeAccepted
		entry.NudgeAccepted = true
		return Result{Entry: entry}, true
	}
	markNudged(d.BindingDir, root, in.SessionID)
	entry.Nudged = true
	return Result{Block: true, BlockReason: reason, Entry: entry}, true
}
