package bench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// DEFECT internal/bench/oracle.go:424 (go doc walk): the set of packages to document is built by a file
// suffix filter (`!strings.HasSuffix(path, "_test.go")`), so a directory whose only Go files are
// _test.go never reaches the `go doc` call. Such a directory is a package Go builds: `go list ./...`
// lists it in the fixture's module, and `go doc -all .` there fails with
// "doc: no source-code package in directory <dir>", a failure the skip list at oracle.go:447 does not
// know. The class is not hypothetical in this corpus: r11-caddy and r21-caddy ship
// caddytest/integration, h13-lo ships benchmark/, r12-memos ships scripts/, h06-ent ships
// examples/version/ and h12-gomicro ships internal/website/docs/guides/, all of them _test.go-only
// packages `go list ./...` reports. When every package of a fixture is test-only, nothing is appended
// to the contract, `documented` stays 0 and `ordered` is empty, so the guard at oracle.go:455 never
// fires: contract.md comes back with no Go documentation and no error, and every test of the reading
// is judged against nothing. That is the same silent loss a144137e and 4dd4620e removed for a package
// named fix, for node_modules and for a module below a fixture without a go.mod: here the walk decides
// by file name instead of asking Go which packages exist.
func TestTSPOracleContractIsNotSilentlyEmptyForATestOnlyPackage(t *testing.T) {
	dir := tspOracleFixture(t, map[string]string{
		"go.mod":      "module example.com/contract\n\ngo 1.22\n",
		"api_test.go": "package api\n\nimport \"testing\"\n\nfunc TestSomething(t *testing.T) {}\n",
	})
	contract, err := oracleContract(dir, 30*time.Second)
	if err == nil && strings.TrimSpace(contract) == "" {
		t.Fatalf("contract is empty and no error was reported, though go list ./... builds %s and "+
			"go doc -all . there refuses it with %q", "example.com/contract", "no source-code package")
	}
}

// DEFECT internal/bench/oracle.go:310-313 (oracleWriteOutput): the two artifacts are replaced one after
// the other, verdicts.json first, so a failure on the second leaves the first one already replaced. A
// run that reports an error therefore still leaves a fresh verdicts.json behind: a later reader, or a
// later VerifyOracle invocation, sees labels from a run that wrote no summary.md, and the pair the
// README describes (<reading>/oracle/verdicts.json records labels, summary.md compares those same
// numbers) no longer belongs to the same run. An occupied target path is the deterministic form of the
// same failure - a full disk or a rename the filesystem refuses leaves exactly this state - and the
// promise a failed command must keep is the weak one pinned here: nothing of the failed run survives.
func TestTSPVerifyOracleLeavesNoVerdictsWhenItsOutputFails(t *testing.T) {
	run, opts := oracleReading(t)
	// The second output path is occupied, so verdicts.json (written first) and summary.md cannot both land.
	if err := os.MkdirAll(filepath.Join(run, "oracle", "summary.md"), 0755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	opts.Agent = func(context.Context, string, Key, Options) (AgentResult, error) {
		calls++
		return AgentResult{Result: `{"tests":[{"name":"TestContract","verdict":"contract","reason":"documented"},` +
			`{"name":"TestInvented","verdict":"invented","reason":"not documented"}]}`}, nil
	}
	report, err := VerifyOracle(run, opts)
	if calls != 1 {
		t.Fatalf("calls=%d, want one judged file", calls)
	}
	if err == nil {
		t.Fatalf("wrote an oracle report over an occupied summary.md path: %+v", report)
	}
	if _, statErr := os.Stat(filepath.Join(run, "oracle", "verdicts.json")); statErr == nil {
		stored, _ := os.ReadFile(filepath.Join(run, "oracle", "verdicts.json"))
		t.Fatalf("verdicts.json from the failed run survived (%d bytes) while the run reported %v: %s",
			len(stored), err, stored)
	}
}
