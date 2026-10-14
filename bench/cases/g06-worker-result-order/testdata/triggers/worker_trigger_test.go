package worker

// Sealed trigger probes: never copied into a workspace. Copy this file into the fixture (or a variant)
// to reproduce a planted defect; see KEY.json.

import (
	"os"
	"reflect"
	"testing"
)

func TestWorkerOrderTrigger(t *testing.T) {
	if os.Getenv("TPP_WORKER_TRIGGER") != "1" {
		t.Skip("trigger probe")
	}

	inputs := []string{"a", "b", "c", "d", "e", "f"}
	got := Process(inputs)
	matches := reflect.DeepEqual(got, inputs)
	t.Logf("actual: output order matches input: %t (output count=%d)", matches, len(got))
	if !matches {
		t.Fail()
	}
}

func TestEmptyInputTrigger(t *testing.T) {
	if os.Getenv("TPP_WORKER_EMPTY_TRIGGER") != "1" {
		t.Skip("trigger probe")
	}

	inputs := []string{"alpha", ""}
	got := Process(inputs)
	includesEmpty := false
	for _, output := range got {
		if output == "" {
			includesEmpty = true
		}
	}
	t.Logf("actual: output count=%d (input count=%d), includes empty: %t", len(got), len(inputs), includesEmpty)
	if len(got) != len(inputs) || !includesEmpty {
		t.Fail()
	}
}
