package worker

import (
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
