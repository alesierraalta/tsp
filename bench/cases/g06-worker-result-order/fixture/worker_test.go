package worker

import (
	"os"
	"reflect"
	"testing"
)

func TestSingleInputPreservesOrder(t *testing.T) {
	got := Process([]string{"alpha"})
	if !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("Process(single input) = %v", got)
	}
}

func TestDuplicateInputsAreDeduped(t *testing.T) {
	got := Process([]string{"alpha", "alpha"})
	if !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("Process(duplicate inputs) = %v", got)
	}
}

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
