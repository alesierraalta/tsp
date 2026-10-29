package findings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/findings"
)

// A declared class is the one thing the memory knows: which kinds of defect already bit this repository.
func TestRecordClassRemembersTheClassForThisRepository(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitPending(t, d)

	out, code := findings.RecordClass(d, "", "", "contract", wait)
	want := "F1 recorded value_test.go example.invalid/sample.TestPromise\n"
	if code != 0 || out != want {
		t.Fatalf("record exit=%d: %q want %q", code, out, want)
	}
	uses, err := findings.ClassMemory(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 1 {
		t.Fatalf("memory holds %d classes, want 1: %+v", len(uses), uses)
	}
	got := uses[0]
	if got.Name != "contract" || got.Count != 1 || got.File != "value_test.go" {
		t.Fatalf("remembered %+v, want contract count 1 in value_test.go", got)
	}
	if !strings.Contains(got.Last, "TestPromise") {
		t.Fatalf("remembered last test %q, want the witness name", got.Last)
	}
}

// A record without --class declares nothing, and the memory stays empty.
func TestRecordWithoutClassWritesNoMemory(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitPending(t, d)

	if out, code := findings.Record(d, "", "", wait); code != 0 {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	uses, err := findings.ClassMemory(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 0 {
		t.Fatalf("a record without --class wrote %+v", uses)
	}
}

// A class nobody declares is refused before anything is written, so the registry stays as it was.
func TestRecordClassRefusesAClassNobodyDeclares(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitPending(t, d)

	out, code := findings.RecordClass(d, "", "", "vibes", wait)
	if code != 2 {
		t.Fatalf("record exit=%d, want 2: %q", code, out)
	}
	if !strings.Contains(out, "vibes") || !strings.Contains(out, "boundary") {
		t.Fatalf("refusal %q must name the class and the ones that exist", out)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("TSP_HOME"), "findings")); err == nil {
		t.Fatalf("a refused record left a findings directory behind")
	}
	if uses, err := findings.ClassMemory(d); err != nil || len(uses) != 0 {
		t.Fatalf("a refused record wrote memory: %+v (%v)", uses, err)
	}
}

// The memory counts each class separately and belongs to one repository, not to the machine.
func TestClassMemoryCountsPerClassAndPerRepository(t *testing.T) {
	first := repo(t)
	// Both repositories answer from one config directory, so only the repository identity can tell them apart.
	home := os.Getenv("TSP_HOME")
	put(t, first, "value_test.go", redTest)
	commitPending(t, first)
	if _, code := findings.RecordClass(first, "", "", "contract", wait); code != 0 {
		t.Fatalf("first record exit=%d", code)
	}

	put(t, first, "more_test.go", "package sample\nimport \"testing\"\n// Keeps its promise.\nfunc TestMore(t *testing.T) { if Value() != 2 { t.Fatal(\"broken\") } }\n")
	commitPending(t, first)
	if out, code := findings.RecordClass(first, "", "", "concurrency", wait); code != 0 {
		t.Fatalf("second record exit=%d: %q", code, out)
	}

	second := repo(t)
	t.Setenv("TSP_HOME", home)
	put(t, second, "value_test.go", redTest)
	commitPending(t, second)
	if _, code := findings.RecordClass(second, "", "", "time", wait); code != 0 {
		t.Fatalf("other repository record exit=%d", code)
	}

	uses, err := findings.ClassMemory(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 2 {
		t.Fatalf("first repository holds %+v, want exactly contract and concurrency", uses)
	}
	seen := map[string]int{}
	for _, use := range uses {
		seen[use.Name] = use.Count
	}
	if seen["contract"] != 1 || seen["concurrency"] != 1 || seen["time"] != 0 {
		t.Fatalf("first repository counts %v", seen)
	}
	other, err := findings.ClassMemory(second)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].Name != "time" {
		t.Fatalf("second repository holds %+v, want only time", other)
	}
}

// The classes are the bench families, not a second list that can drift away from them.
func TestClassesAreTheBenchFamilies(t *testing.T) {
	got := findings.Classes()
	if len(got) != len(bench.Families) {
		t.Fatalf("Classes() holds %d, want %d", len(got), len(bench.Families))
	}
	for i := range got {
		if got[i] != bench.Families[i] {
			t.Fatalf("Classes()[%d]=%q, want %q", i, got[i], bench.Families[i])
		}
	}
	got[0] = "mutated"
	if findings.Classes()[0] != bench.Families[0] {
		t.Fatal("Classes() handed out the caller a slice the package still uses")
	}
}
