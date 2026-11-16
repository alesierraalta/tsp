package bench

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// upstreamRepo is a git repository standing in for a real project: one commit whose tree is the buggy code.
func upstreamRepo(t *testing.T) (dir, commit string) {
	t.Helper()
	dir = t.TempDir()
	writeFile(t, dir, "calc.go", "package calc\n\nfunc Add(a, b int) int { return a - b }\n")
	writeFile(t, dir, "calc_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestZero(t *testing.T) {\n\tif Add(0, 0) != 0 {\n\t\tt.Fatal()\n\t}\n}\n")
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"add", "-A"}, {"commit", "-qm", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return dir, strings.TrimSpace(string(out))
}

const sourceKey = `{"id":"r99-calc-add","language":"go","suite":"go test .","defects":[{"id":"d1","file":"calc.go","line":3,"keywords":["add"]}]}`

const sourcePatch = "--- a/calc.go\n+++ b/calc.go\n@@ -1,3 +1,3 @@\n package calc\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n"

// sourceCase is a case that ships only its key, where its code comes from, and the fix as a patch.
func sourceCase(t *testing.T, repo, commit string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "r99-calc-add")
	writeFile(t, dir, KeyFile, sourceKey)
	writeFile(t, dir, SourceFile, `{"repo":"`+repo+`","commit":"`+commit+`","patch":"fix.patch"}`)
	writeFile(t, dir, "fix.patch", sourcePatch)
	return dir
}

// A case shipped by reference materializes into the layout every other case has: the key, the fixture as
// the upstream commit's tree without its history, and fix/all as that tree with the fix applied. A second
// materialization reuses the cache instead of fetching again.
func TestSourceCaseMaterializesFixtureAndFix(t *testing.T) {
	repo, commit := upstreamRepo(t)
	caseDir := sourceCase(t, repo, commit)
	cache := t.TempDir()

	dir, err := MaterializeCase(caseDir, cache)
	if err != nil {
		t.Fatal(err)
	}
	if dir == caseDir || !strings.HasPrefix(dir, cache) {
		t.Fatalf("materialized into %s, want a directory under the cache %s", dir, cache)
	}
	if key, err := os.ReadFile(filepath.Join(dir, KeyFile)); err != nil || string(key) != sourceKey {
		t.Fatalf("key = %q, %v", key, err)
	}
	fixture, _ := os.ReadFile(filepath.Join(dir, FixtureDir, "calc.go"))
	fixed, _ := os.ReadFile(filepath.Join(dir, FixDir, "all", "calc.go"))
	if !strings.Contains(string(fixture), "a - b") || !strings.Contains(string(fixed), "a + b") {
		t.Fatalf("fixture %q, fix/all %q", fixture, fixed)
	}
	if _, err := os.Stat(filepath.Join(dir, FixtureDir, "calc_test.go")); err != nil {
		t.Fatalf("the fixture must carry the whole upstream tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FixtureDir, ".git")); err == nil {
		t.Fatal("the fixture must not carry the upstream history")
	}

	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	again, err := MaterializeCase(caseDir, cache)
	if err != nil || again != dir {
		t.Fatalf("second materialization = %s, %v; want the cached %s", again, err, dir)
	}
}

// sourceWithExclusions is a source case whose upstream tree also carries the answers of other cases.
func sourceWithExclusions(t *testing.T, repo, commit, exclude string) string {
	t.Helper()
	dir := sourceCase(t, repo, commit)
	writeFile(t, dir, SourceFile, `{"repo":"`+repo+`","commit":"`+commit+`","patch":"fix.patch","exclude":`+exclude+`}`)
	return dir
}

// An exclusion is declared in SOURCE.json and applied to both sides of the case before the fixture
// reaches the agent. The paths a case needs are kept; what was dropped is recorded beside the fixture.
func TestSourceCaseExcludesDeclaredPaths(t *testing.T) {
	repo, commit := upstreamRepo(t)
	// Commit the leaked answers after upstreamRepo's base commit.
	writeFile(t, repo, "bench/cases/o01/KEY.json", `{"id":"other"}`)
	writeFile(t, repo, "docs/testing/test-plan.md", "| F45 | findings row |\n")
	writeFile(t, repo, "docs/testing/keep.md", "needed by the suite\n")
	if out, err := exec.Command("git", "-C", repo, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "answers").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit = strings.TrimSpace(string(out))

	caseDir := sourceWithExclusions(t, repo, commit, `["bench/cases","docs/testing/test-plan.md"]`)
	dir, err := MaterializeCase(caseDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tree := range []string{filepath.Join(dir, FixtureDir), filepath.Join(dir, FixDir, "all")} {
		if _, err := os.Stat(filepath.Join(tree, "bench", "cases")); !os.IsNotExist(err) {
			t.Errorf("%s still carries bench/cases: %v", tree, err)
		}
		if _, err := os.Stat(filepath.Join(tree, "docs", "testing", "test-plan.md")); !os.IsNotExist(err) {
			t.Errorf("%s still carries the test plan: %v", tree, err)
		}
		if _, err := os.Stat(filepath.Join(tree, "docs", "testing", "keep.md")); err != nil {
			t.Errorf("%s lost the file the suite needs: %v", tree, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "source-exclusions.json"))
	if err != nil {
		t.Fatalf("exclusions were not recorded in the case: %v", err)
	}
	var dropped []string
	if err := json.Unmarshal(manifest, &dropped); err != nil {
		t.Fatal(err)
	}
	want := []string{"bench/cases", "docs/testing/test-plan.md"}
	if !reflect.DeepEqual(dropped, want) {
		t.Fatalf("recorded exclusions = %q, want %q", dropped, want)
	}
}

// The exclusion is part of the case's identity: editing it must move the materialized case (and so its
// digest) instead of silently reusing a cache built with the answers in place.
func TestSourceDigestRecordsExclusions(t *testing.T) {
	repo, commit := upstreamRepo(t)
	plain, err := MaterializeCase(sourceCase(t, repo, commit), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	excluded, err := MaterializeCase(sourceWithExclusions(t, repo, commit, `["calc_test.go"]`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(plain) == filepath.Base(excluded) {
		t.Fatalf("both layouts hashed to %s: an exclusion did not change the case digest", filepath.Base(plain))
	}
}

// A path that escapes the tree cannot be used to delete something outside the cache.
func TestSourceRefusesAnExclusionOutsideTheTree(t *testing.T) {
	repo, commit := upstreamRepo(t)
	if _, err := MaterializeCase(sourceWithExclusions(t, repo, commit, `["../outside"]`), t.TempDir()); err == nil || !strings.Contains(err.Error(), "exclude") {
		t.Fatalf("err = %v, want one naming the bad exclusion", err)
	}
}

// A vendored case already has its directories and is used as it is.
func TestVendoredCaseIsUsedInPlace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, KeyFile, sourceKey)
	got, err := MaterializeCase(dir, t.TempDir())
	if err != nil || got != dir {
		t.Fatalf("got %s, %v; want %s", got, err, dir)
	}
}

// A patch that no longer applies is an invalid case, reported, never a fixture silently left unfixed.
func TestSourceCaseRefusesAPatchThatDoesNotApply(t *testing.T) {
	repo, commit := upstreamRepo(t)
	caseDir := sourceCase(t, repo, commit)
	writeFile(t, caseDir, "fix.patch", strings.ReplaceAll(sourcePatch, "a - b", "a * b"))
	if _, err := MaterializeCase(caseDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "fix.patch") {
		t.Fatalf("err = %v, want one naming fix.patch", err)
	}
}

// bench run resolves a source-referenced case to its materialized directory.
func TestResolveCaseDirsMaterializesSourceCases(t *testing.T) {
	repo, commit := upstreamRepo(t)
	caseDir := sourceCase(t, repo, commit)
	cache := t.TempDir()
	dirs, ok := resolveCaseDirs(Options{CasesGlob: caseDir, CaseCache: cache})
	if !ok || len(dirs) != 1 || !strings.HasPrefix(dirs[0], cache) {
		t.Fatalf("dirs = %v, ok = %v", dirs, ok)
	}
}
