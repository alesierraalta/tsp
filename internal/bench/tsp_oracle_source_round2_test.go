package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// DEFECT oracle.go:409 (go doc walk): the skip list still drops every directory named node_modules, which is not
// one of Go's own package rules. Go only excludes testdata, _- and .-prefixed directories, vendor and a module
// nested below the fixture's own module; `go list ./...` in this fixture lists example.com/contract/node_modules/lib
// and `go doc -all .` documents it. So a Go package that is part of the fixture's module disappears from
// contract.md with no diagnostic and no error, which is the same silent loss a144137e fixed for a directory named
// fix (a sibling package keeps the documented==0 rule quiet, so the loss is the skip itself).
func TestTSPOracleContractKeepsAPackageUnderNodeModules(t *testing.T) {
	dir := tspOracleFixture(t, map[string]string{
		"go.mod":                     "module example.com/contract\n\ngo 1.22\n",
		"ok/ok.go":                   "// Package ok documents the API.\npackage ok\n\n// Portable is available everywhere.\nfunc Portable() {}\n",
		"node_modules/lib/lib.go":    "// Package lib is a Go package installed under node_modules.\npackage lib\n\n// Vendored documents itself.\nfunc Vendored() {}\n",
		"node_modules/lib/other.txt": "not go\n",
	})
	contract, err := oracleContract(dir, 30*time.Second)
	if err != nil {
		t.Fatalf("contract build failed: %v", err)
	}
	if !strings.Contains(contract, "func Vendored()") {
		t.Fatalf("package node_modules/lib is part of the module Go builds but missing from the contract: %q", contract)
	}
}

// DEFECT oracle.go:142 (VerifyOracle case resolution): the recorded case name is joined onto <bench>/cases without
// checking that it is a single path element, so a name carrying ".." silently resolves to a directory outside the
// case corpus and is judged as if it were that case. Nothing about the recorded case is compared, so the reading is
// scored against a fixture that is not in bench/cases; every other name the oracle accepts is a directory name.
func TestTSPVerifyOracleRefusesARecordedCaseNameOutsideTheCorpus(t *testing.T) {
	run, opts := oracleReading(t)
	root := filepath.Dir(run)
	// A complete, judgeable case that is not in the corpus: fixture with a contract, a key, and a kept workspace.
	planted := filepath.Join(root, "planted")
	writeFile(t, planted, FixtureDir+"/README.md", "# Contract\nAnswer returns forty-two.\n")
	writeFile(t, planted, KeyFile, `{"request":"Test Answer"}`)
	ws := filepath.Join(root, "ws")
	writeFile(t, ws, "agent_test.go", "package demo\n\nimport \"testing\"\n\nfunc TestContract(t *testing.T) {}\n")
	data, err := json.Marshal(Aggregate{Cases: []Result{{
		Case:      "../../planted",
		Run:       1,
		Workspace: ws,
		Catch:     CatchResult{TestFiles: []string{"agent_test.go"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, run, "aggregate.json", string(data))
	opts.CaseCache = t.TempDir()
	opts.DryRun = true
	if _, err := VerifyOracle(run, opts); err == nil {
		if _, statErr := os.Stat(filepath.Join(planted, FixtureDir)); statErr != nil {
			t.Fatalf("planted fixture vanished: %v", statErr)
		}
		t.Fatalf("judged ../../planted, a directory outside %s, as the recorded case",
			filepath.Join(opts.BenchDir, "cases"))
	}
}
