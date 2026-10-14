package bench

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// AgentResult is what one agent run produced, as read from Claude Code's stream-json output.
type AgentResult struct {
	Result     string  `json:"result"`
	CostUSD    float64 `json:"cost_usd"`
	Turns      int     `json:"turns"`
	DurationMS int64   `json:"duration_ms"`
	SessionID  string  `json:"session_id"`
	ExitCode   int     `json:"exit_code"`
	TimedOut   bool    `json:"timed_out"`
	IsError    bool    `json:"is_error"`
	ErrorText  string  `json:"error_text,omitempty"`
	Raw        string  `json:"-"` // tail of the raw stream, written to agent.log for diagnosis
	// Transcript is the whole stream, kept compressed beside the result: the tail in agent.log loses the
	// run's exploration, which is where a reading has to look to say what its turns were spent on.
	Transcript []byte `json:"-"`
}

// ParseStream keeps the last "result" event of a stream-json transcript; every other line is
// ignored, including lines that are not JSON, so a partial stream still yields what it has.
func ParseStream(r io.Reader) AgentResult {
	var out AgentResult
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev struct {
			Type       string  `json:"type"`
			Result     string  `json:"result"`
			CostUSD    float64 `json:"total_cost_usd"`
			Turns      int     `json:"num_turns"`
			DurationMS int64   `json:"duration_ms"`
			SessionID  string  `json:"session_id"`
			IsError    bool    `json:"is_error"`
			Subtype    string  `json:"subtype"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.SessionID != "" && out.SessionID == "" {
			out.SessionID = ev.SessionID
		}
		if ev.Type == "result" {
			out.Result, out.CostUSD, out.Turns, out.DurationMS = ev.Result, ev.CostUSD, ev.Turns, ev.DurationMS
			// A result event can carry an error instead of an answer; the exit code alone does not say so.
			if ev.IsError || strings.HasPrefix(ev.Subtype, "error") {
				out.IsError = true
				out.ErrorText = strings.TrimSpace(ev.Subtype + " " + ev.Result)
			}
			if ev.SessionID != "" {
				out.SessionID = ev.SessionID
			}
		}
	}
	return out
}
