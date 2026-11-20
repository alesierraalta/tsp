package feature

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/state"
)

func TestAllDeclaresEveryFeatureWithItsDefault(t *testing.T) {
	features := All()
	if len(features) != 4 {
		t.Fatalf("All returned %d features, want 4", len(features))
	}
	feedback := features[0]
	if feedback.ID != "feedback" || feedback.Title != "Feedback" || feedback.Default {
		t.Fatalf("feature = %+v, want feedback with opt-in default", feedback)
	}
	if feedback.Prompt != "¿Quieres activar el feedback para ayudar a mejorar la herramienta?" {
		t.Fatalf("prompt = %q", feedback.Prompt)
	}
	if !strings.Contains(feedback.Help, "anonymized") || !strings.Contains(feedback.Help, "two local files") {
		t.Fatalf("help = %q, want anonymization and local-file safety", feedback.Help)
	}
	// All's declared order: feedback, telemetry, auto-run, headless-loop.
	autoRun := features[2]
	if autoRun.ID != "auto-run" || autoRun.Title != "Automatic TSP run" || !autoRun.Default {
		t.Fatalf("feature = %+v, want auto-run on by default so an existing session keeps behaving as before", autoRun)
	}
	if autoRun.Prompt == "" || autoRun.Help == "" || autoRun.Preview == "" {
		t.Fatalf("auto-run is missing its prompt, help or preview: %+v", autoRun)
	}
	// Telemetry is local and never harvested, but it is still off until a human asks for it: the
	// ledger is a directory on disk, and an install that never asked carries none.
	telemetry := features[1]
	if telemetry.ID != TelemetryID || telemetry.Title != "Telemetry" || telemetry.Default {
		t.Fatalf("feature = %+v, want telemetry off by default", telemetry)
	}
	if telemetry.Prompt == "" || telemetry.Help == "" || telemetry.Preview == "" {
		t.Fatalf("telemetry is missing its prompt, help or preview: %+v", telemetry)
	}
	for _, want := range []string{"local", "no code", "ever sent"} {
		if !strings.Contains(telemetry.Help+telemetry.Preview, want) {
			t.Fatalf("prompt, help and preview must state %q: %+v", want, telemetry)
		}
	}
	// The headless loop costs model turns and the hard23 A/B read flat recall at +91% cost, so it is opt-in.
	headless := features[3]
	if headless.ID != HeadlessLoopID || headless.Title != "Headless TSP loop" || headless.Default {
		t.Fatalf("feature = %+v, want headless-loop off by default", headless)
	}
	if headless.Prompt != "¿Quieres que una sesión sin interfaz arregle y vuelva a verificar sola lo que TSP encuentre?" {
		t.Fatalf("prompt = %q", headless.Prompt)
	}
	for _, want := range []string{"no UI", "fix", "round", "record", "twice", "turns"} {
		if !strings.Contains(headless.Help, want) {
			t.Fatalf("help = %q, want it to name %q", headless.Help, want)
		}
	}
	if !strings.Contains(headless.Preview, "twice") {
		t.Fatalf("preview = %q, want the bound stated", headless.Preview)
	}
}

func TestHeadlessLoopIsOffWithoutAnEntryAndRoundTrips(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())

	enabled, err := Enabled(HeadlessLoopID)
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if enabled {
		t.Fatal("headless-loop is on without a persisted entry, which would spend every headless session's turns by default")
	}

	if _, err := Set(HeadlessLoopID, true); err != nil {
		t.Fatalf("Set true: %v", err)
	}
	if enabled, err = Enabled(HeadlessLoopID); err != nil || !enabled {
		t.Fatalf("Enabled after true = %v, %v; want enabled", enabled, err)
	}
	if _, err := Set(HeadlessLoopID, false); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	if enabled, err = Enabled(HeadlessLoopID); err != nil || enabled {
		t.Fatalf("Enabled after false = %v, %v; want disabled", enabled, err)
	}
}

func TestAutoRunIsOnWithoutAnEntryAndOffAfterSet(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())

	enabled, err := Enabled("auto-run")
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if !enabled {
		t.Fatal("auto-run is off without a persisted entry, which would change the behaviour of every existing session")
	}

	if _, err := Set("auto-run", false); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	enabled, err = Enabled("auto-run")
	if err != nil {
		t.Fatalf("Enabled after false: %v", err)
	}
	if enabled {
		t.Fatal("auto-run did not round-trip as disabled")
	}
}

func TestEnabledAnswersDefaultOnEmptyState(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())

	enabled, err := Enabled("feedback")
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if enabled {
		t.Fatal("feedback is enabled without a persisted entry")
	}
}

func TestSetRoundTripsTheToggle(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())

	updatedAt, err := Set("feedback", true)
	if err != nil {
		t.Fatalf("Set true: %v", err)
	}
	if updatedAt == "" {
		t.Fatal("Set true returned an empty timestamp")
	}
	enabled, err := Enabled("feedback")
	if err != nil {
		t.Fatalf("Enabled after true: %v", err)
	}
	if !enabled {
		t.Fatal("feedback did not round-trip as enabled")
	}

	if _, err := Set("feedback", false); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	enabled, err = Enabled("feedback")
	if err != nil {
		t.Fatalf("Enabled after false: %v", err)
	}
	if enabled {
		t.Fatal("feedback did not round-trip as disabled")
	}
}

func TestEnabledFailsOnCorruptState(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())
	path, err := state.Path()
	if err != nil {
		t.Fatalf("state.Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("state directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("corrupt state: %v", err)
	}

	enabled, err := Enabled("feedback")
	if err == nil || enabled {
		t.Fatalf("Enabled = %v, %v; want a fail-closed error", enabled, err)
	}
}

func TestPreviewDoesNotPersistAnything(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())

	preview, err := Preview("feedback")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !strings.Contains(preview, "never publishable") && !strings.Contains(preview, "ever publishable") {
		t.Fatalf("preview = %q, want local publication safety", preview)
	}
	path, err := state.Path()
	if err != nil {
		t.Fatalf("state.Path: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Preview wrote state %s: %v", path, err)
	}
}

func TestSetSameValueDoesNotChurnStateBytes(t *testing.T) {
	t.Setenv("TPP_HOME", t.TempDir())

	if _, err := Set("feedback", true); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	path, err := state.Path()
	if err != nil {
		t.Fatalf("state.Path: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("state before no-op Set: %v", err)
	}
	if _, err := Set("feedback", true); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("state after no-op Set: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("same-value Set churned state bytes:\n before %s\n after  %s", before, after)
	}
}
