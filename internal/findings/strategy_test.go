package findings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/findings"
)

func TestStrategyEscapedLongBranch(t *testing.T) {
	d := repo(t)
	// Git accepts this 180-byte ref component, but filename escaping expands it.
	branch := "review/" + strings.Repeat("界", 60)
	command(t, d, "git", "switch", "-c", branch)
	put(t, d, "value_test.go", redTest)
	if out, code := findings.Record(d, "", wait); code != 0 {
		t.Fatalf("record on valid branch exit=%d: %s", code, out)
	}
	if out, code := findings.Check(d, wait); code != 1 || !strings.Contains(out, "F1 open") {
		t.Fatalf("review on valid branch exit=%d: %s", code, out)
	}
}

func TestStrategyMovedWitnessIsNotDeleted(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	put(t, d, "value_test.go", "package sample\n")
	put(t, d, "moved_test.go", redTest)
	put(t, d, "value.go", "package sample\nfunc Value() int { return 1 }\n")
	out, code := findings.Check(d, wait)
	if code != 0 || !strings.Contains(out, "F1 fixed") {
		t.Fatalf("unchanged witness still runs after move; exit=%d: %s", code, out)
	}
}

func TestStrategyConcurrentRecordAndCheckKeepWitnesses(t *testing.T) {
	d := repo(t)
	source := `package sample
import ("testing"; "os"; "path/filepath"; "time")
func TestPromise(t *testing.T) {
 if gate := os.Getenv("FINDINGS_STRATEGY_GATE"); gate != "" {
  if err := os.WriteFile(filepath.Join(gate, "entered"), nil, 0600); err != nil { t.Fatal(err) }
  deadline := time.Now().Add(10*time.Second)
  for { if _, err := os.Stat(filepath.Join(gate, "release")); err == nil { break }; if time.Now().After(deadline) { t.Fatal("gate timeout") }; time.Sleep(10*time.Millisecond) }
 }
 if Value() != 1 { t.Fatal("broken") }
}
func TestSecond(t *testing.T) { if Value() != 2 { t.Fatal("second defect") } }
`
	put(t, d, "value_test.go", source)
	if out, code := findings.Record(d, "go test -count=1 -run '^TestPromise$' .", wait); code != 0 {
		t.Fatalf("initial record: %d %s", code, out)
	}
	gate := t.TempDir()
	t.Setenv("FINDINGS_STRATEGY_GATE", gate)
	type result struct {
		out  string
		code int
	}
	checked := make(chan result, 1)
	go func() {
		out, code := findings.Check(d, wait)
		checked <- result{out, code}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(gate, "entered")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("check did not reach runner gate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	recorded := make(chan result, 1)
	go func() {
		out, code := findings.Record(d, "go test -count=1 -run '^TestSecond$' .", wait)
		recorded <- result{out, code}
	}()
	var recording result
	finished := false
	select {
	case recording = <-recorded:
		finished = true
	case <-time.After(5 * time.Second):
		// A serialized implementation can wait for the check to release its lock.
	}
	put(t, gate, "release", "")
	checking := <-checked
	if !finished {
		recording = <-recorded
	}
	if checking.code != 1 || recording.code != 0 {
		t.Fatalf("check=%d %s; record=%d %s", checking.code, checking.out, recording.code, recording.out)
	}
	files, _ := filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "*.json"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var registry struct{ Findings []findings.Finding }
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Findings) != 2 {
		t.Fatalf("successful concurrent record lost its witness: got %d findings, want 2", len(registry.Findings))
	}
}
