package telemetry

import (
	"encoding/json"
	"testing"
)

// The domain bodies are what a later reader decodes by exact JSON name; a renamed or dropped field
// would make a stats reader silently lose a measurement.
func TestDomainEventJSONNames(t *testing.T) {
	event := Event{
		Kind: KindCheck,
		Check: &Check{
			Decision:    "ask",
			Exit:        1,
			Reason:      "no_witness",
			Suite:       "go test",
			Runners:     []string{"go test", "node --test"},
			Languages:   []string{"go", "js"},
			Files:       Count(2),
			Lines:       Count(9),
			TestFiles:   Count(0),
			Red:         Count(0),
			PinsNothing: Count(0),
			NewCode:     Count(0),
		},
		Finding: &Finding{Action: "record", Class: "boundary", Verdict: "rejected", Count: Count(3)},
		Plan:    &Plan{Action: "check", Findings: Count(4), Evidence: Count(5), Problems: Count(0)},
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Check   map[string]any `json:"check"`
		Finding map[string]any `json:"finding"`
		Plan    map[string]any `json:"plan"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	check := decoded.Check
	for _, key := range []string{"decision", "exit", "reason", "suite", "runners", "languages", "files", "lines", "test_files", "red", "pins_nothing", "new_code"} {
		if _, ok := check[key]; !ok {
			t.Errorf("check body lacks %s: %v", key, check)
		}
	}
	for _, key := range []string{"waiver", "opt_out"} {
		if _, ok := check[key]; ok {
			t.Errorf("a false %s must stay absent, not read as a measurement: %v", key, check)
		}
	}
	finding := decoded.Finding
	for _, key := range []string{"action", "class", "verdict", "count"} {
		if _, ok := finding[key]; !ok {
			t.Errorf("finding body lacks %s: %v", key, finding)
		}
	}
	plan := decoded.Plan
	for _, key := range []string{"action", "findings", "evidence", "problems"} {
		if _, ok := plan[key]; !ok {
			t.Errorf("plan body lacks %s: %v", key, plan)
		}
	}
}

// A count nobody measured stays absent; zero is a measurement and must survive.
func TestDomainCountsDistinguishAbsentFromZero(t *testing.T) {
	raw, err := json.Marshal(Event{Kind: KindPlan, Plan: &Plan{Action: "check", Findings: Count(0)}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Plan map[string]any `json:"plan"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Plan["findings"] != float64(0) {
		t.Errorf("measured zero = %v, want 0", decoded.Plan["findings"])
	}
	for _, absent := range []string{"evidence", "problems"} {
		if _, ok := decoded.Plan[absent]; ok {
			t.Errorf("unmeasured %s must stay absent: %v", absent, decoded.Plan)
		}
	}
}
