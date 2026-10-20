package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tspOracleFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, filepath.Join(dir, FixtureDir), name, content)
	}
	return dir
}

// DEFECT oracle.go go-doc walk: a directory named "fix" is skipped at every depth, not only the case's own
// fix/ tree (which is a sibling of fixture/, never inside it). A real upstream package called fix (cmd/fix,
// internal/fix) drops out of contract.md without any diagnostic.
func TestTSPOracleContractKeepsAPackageNamedFix(t *testing.T) {
	dir := tspOracleFixture(t, map[string]string{
		"go.mod":            "module example.com/contract\n\ngo 1.22\n",
		"internal/fix/f.go": "// Package fix repairs inputs.\npackage fix\n\n// Apply repairs one input.\nfunc Apply() {}\n",
	})
	contract, err := oracleContract(dir, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contract, "func Apply()") {
		t.Fatalf("package internal/fix is part of the module but missing from the contract: %q", contract)
	}
}

// DEFECT oracle.go go-doc walk: every directory with a go.mod below the fixture root is treated as "nested",
// even when the fixture root has no go.mod. A fixture whose only module lives in a subdirectory is then
// skipped whole and the contract silently loses all of its Go documentation.
func TestTSPOracleContractDocumentsAModuleWhenTheFixtureRootHasNoGoMod(t *testing.T) {
	dir := tspOracleFixture(t, map[string]string{
		"README.md":  "readme body\n",
		"sub/go.mod": "module example.com/sub\n\ngo 1.22\n",
		"sub/s.go":   "// Package s documents the API.\npackage s\n\n// Do does the work.\nfunc Do() {}\n",
	})
	contract, err := oracleContract(dir, 30*time.Second)
	if err != nil {
		return
	}
	if !strings.Contains(contract, "func Do()") {
		t.Fatalf("module in a subdirectory vanished from the contract without an error: %q", contract)
	}
}

// DEFECT oracle.go go-doc skip: a package excluded by build constraints used to fail loudly; now it is skipped,
// so a fixture whose Go files are all excluded on the verifier's platform yields an empty contract.md with a
// nil error, and every test in the reading is judged against nothing.
func TestTSPOracleContractIsNotSilentlyEmptyWhenEveryPackageIsExcluded(t *testing.T) {
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	dir := tspOracleFixture(t, map[string]string{
		"go.mod": "module example.com/contract\n\ngo 1.22\n",
		"w.go":   "//go:build windows\n\npackage w\n\n// W is the whole API.\nfunc W() {}\n",
	})
	contract, err := oracleContract(dir, 30*time.Second)
	if err == nil && strings.TrimSpace(contract) == "" {
		t.Fatal("contract is empty and no error was reported")
	}
}

// DEFECT oracle.go VerifyOracle: the "case changed since the reading" guard only runs for the fallback
// (resolvedSource). A reading that recorded the bare name of a case which has since become a source-referenced
// case resolves directly, is rematerialized under <case>-<12hex>, and is judged against that fixture with no
// error, though the recorded fixture is not the one being judged.
func TestTSPVerifyOracleRejectsABareRecordedNameThatNowMaterializesElsewhere(t *testing.T) {
	repo, commit := upstreamRepo(t)
	source := sourceCase(t, repo, commit)
	name := filepath.Base(source)
	run, opts := oracleReading(t)
	caseDir := filepath.Join(opts.BenchDir, "cases", name)
	for _, f := range []string{KeyFile, SourceFile, "fix.patch"} {
		data, err := os.ReadFile(filepath.Join(source, f))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, caseDir, f, string(data))
	}
	ws := filepath.Join(run, name, "1", "ws")
	writeFile(t, ws, "agent_test.go", "package calc\nimport \"testing\"\nfunc TestAdd(t *testing.T) {}\n")
	data, err := json.Marshal(Aggregate{Cases: []Result{{Case: name, Run: 1, Workspace: ws, Catch: CatchResult{TestFiles: []string{"agent_test.go"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, run, "aggregate.json", string(data))
	opts.CaseCache = t.TempDir()
	opts.DryRun = true
	if _, err := VerifyOracle(run, opts); err == nil {
		t.Fatalf("judged %q against a fixture materialized under a different name than the reading recorded", name)
	}
}
