package gate

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/alesierraalta/tsp/internal/feature"
	"github.com/alesierraalta/tsp/internal/telemetry"
)

// recordHostTelemetry notes one claude host_run for the transcript this Stop carries, when that
// transcript evidences a TSP run — the same check Decide uses, so the two never disagree about what
// counts as a run. It returns nothing and writes nothing itself: the process's one Flush appends
// the event. Telemetry must never change the hook's decision, stdout or exit code, so every failure
// here is silence.
func recordHostTelemetry(in Input) {
	path := strings.TrimSpace(in.TranscriptPath)
	if path == "" || strings.TrimSpace(in.SessionID) == "" {
		return
	}
	enabled, err := feature.Enabled(feature.TelemetryID)
	if err != nil || !enabled {
		return
	}
	raw, ok := readTranscript(path)
	if !ok {
		return
	}
	if !isAdversarial(SkillsLoaded(bytes.NewReader(raw))) {
		return
	}
	_, _ = telemetry.CaptureClaude(in.SessionID, bytes.NewReader(raw))
}

// readTranscript reads the whole transcript once, because the evidence check and the usage parse
// both read it; a transcript the host cannot open or read is no measurement at all.
func readTranscript(path string) ([]byte, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, false
	}
	return raw, true
}
