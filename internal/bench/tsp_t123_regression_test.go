package bench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOracleQualifiedJSNames(t *testing.T) {
	data := []byte(`describe('outer', () => {
		describe.only("inner", function () {
			test('fails', () => {});
			it.skip('failsAB', () => {});
		});
		test('fails', () => {});
	});
	// test('not a test', () => {});
	const message = "test('also not a test')";`)
	c := CatchResult{CaughtBy: map[string][]string{"D1": {"outer inner fails", "unrelated fails"}}}
	names := oracleNames("agent.test.js", data, c)
	if strings.Join(names, "|") != "outer fails|outer inner fails|outer inner failsAB" {
		t.Fatalf("qualified names: %v", names)
	}
}

func TestOracleRefusesOutputSymlinksBeforeJudging(t *testing.T) {
	for _, name := range []string{"oracle", "oracle/summary.md", "oracle/nested/link"} {
		t.Run(name, func(t *testing.T) {
			run, opts := oracleReading(t)
			target := filepath.Join(run, "aggregate.json")
			before, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(run, name)
			if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			opts.Agent = func(context.Context, string, Key, Options) (AgentResult, error) {
				t.Error("judge invoked before refusing output symlink")
				return AgentResult{}, nil
			}
			if _, err := VerifyOracle(run, opts); err == nil {
				t.Fatal("accepted output symlink")
			}
			after, err := os.ReadFile(target)
			if err != nil || string(after) != string(before) {
				t.Fatalf("reading changed: %v", err)
			}
			if name != "oracle" {
				if _, err := os.Stat(filepath.Join(run, "oracle/verdicts.json")); !os.IsNotExist(err) {
					t.Fatalf("output written before refusal: %v", err)
				}
			}
		})
	}
}

func TestSourceCacheLayoutLeavesLegacyCacheUntouched(t *testing.T) {
	repo, commit := upstreamRepo(t)
	source := sourceCase(t, repo, commit)
	cache := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(source, SourceFile))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append(append(raw, 0), []byte(sourcePatch)...))
	old := filepath.Join(cache, filepath.Base(source)+"-"+hex.EncodeToString(sum[:])[:12])
	writeFile(t, old, "fix/all/sentinel", "legacy cache")
	materialized, err := MaterializeCase(source, cache)
	if err != nil {
		t.Fatal(err)
	}
	if materialized == old {
		t.Fatal("reused legacy cache without deletion metadata")
	}
	if data, err := os.ReadFile(filepath.Join(old, "fix/all/sentinel")); err != nil || string(data) != "legacy cache" {
		t.Fatalf("legacy cache changed: %s %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(materialized, sourceDeletionsFile)); err != nil || strings.TrimSpace(string(data)) != "[]" {
		t.Fatalf("missing explicit empty deletion manifest: %s %v", data, err)
	}
}

func TestSessionPartialOverlayDoesNotDeleteOmittedFiles(t *testing.T) {
	caseDir, ws := t.TempDir(), t.TempDir()
	writeFile(t, caseDir, "fixture/retained.go", "package demo\n")
	writeFile(t, caseDir, "fix/all/changed.go", "package demo\n")
	writeFile(t, ws, "retained.go", "package demo\n")
	if err := applySessionFix(caseDir, ws, nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(ws, "retained.go")); err != nil || string(data) != "package demo\n" {
		t.Fatalf("partial overlay deleted omitted source: %s %v", data, err)
	}
}
