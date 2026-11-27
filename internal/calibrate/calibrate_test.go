package calibrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// repo writes files into a fresh git worktree checked out on branch and answers its root.
func repo(t *testing.T, branch string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", branch)
	git(t, root, "-c", "user.email=calibrate@example.com", "-c", "user.name=calibrate",
		"-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "base")
	for rel, body := range files {
		write(t, root, rel, body)
	}
	return root
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

// sitesOf scans one source file in a directory of its own and answers the candidate sites it holds.
func sitesOf(t *testing.T, src string) []Site {
	t.Helper()
	root := t.TempDir()
	write(t, root, "x.go", src)
	sites, err := candidates(root, "x.go")
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	return sites
}

// Each operator answers the one line it claims, with the name and the text the sealed key records.
func TestEachOperatorFiresOnItsOwnConstruct(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		operator string
		line     int
		mutated  string
	}{
		{
			name:     "flip-comparison >= to >",
			src:      "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
			operator: "flip-comparison >=->>",
			line:     4,
			mutated:  "\treturn a > b",
		},
		{
			name:     "flip-comparison <= to <",
			src:      "package x\n\nfunc f(a, b int) bool {\n\treturn a <= b\n}\n",
			operator: "flip-comparison <=-><",
			line:     4,
			mutated:  "\treturn a < b",
		},
		{
			name:     "flip-comparison == to !=",
			src:      "package x\n\nfunc f(a, b int) bool {\n\treturn a == b\n}\n",
			operator: "flip-comparison ==->!=",
			line:     4,
			mutated:  "\treturn a != b",
		},
		{
			name:     "flip-comparison != to ==",
			src:      "package x\n\nfunc f(a, b int) bool {\n\treturn a != b\n}\n",
			operator: "flip-comparison !=->==",
			line:     4,
			mutated:  "\treturn a == b",
		},
		{
			name:     "flip-comparison > to >=",
			src:      "package x\n\nfunc f(a, b int) bool {\n\treturn a > b\n}\n",
			operator: "flip-comparison >->>=",
			line:     4,
			mutated:  "\treturn a >= b",
		},
		{
			name:     "flip-comparison < to <=",
			src:      "package x\n\nfunc f(a, b int) bool {\n\treturn a < b\n}\n",
			operator: "flip-comparison <-><=",
			line:     4,
			mutated:  "\treturn a <= b",
		},
		{
			name:     "swap-logical && to ||",
			src:      "package x\n\nfunc f(a, b bool) bool {\n\treturn a && b\n}\n",
			operator: "swap-logical &&->||",
			line:     4,
			mutated:  "\treturn a || b",
		},
		{
			name:     "swap-logical || to &&",
			src:      "package x\n\nfunc f(a, b bool) bool {\n\treturn a || b\n}\n",
			operator: "swap-logical ||->&&",
			line:     4,
			mutated:  "\treturn a && b",
		},
		{
			name:     "negate-if on a single-line condition",
			src:      "package x\n\nfunc f(a bool) {\n\tif a {\n\t\tprintln(\"x\")\n\t}\n}\n",
			operator: "negate-if",
			line:     4,
			mutated:  "\tif !(a) {",
		},
		{
			name:     "flip-bool-return true to false",
			src:      "package x\n\nfunc f() bool {\n\treturn true\n}\n",
			operator: "flip-bool-return true->false",
			line:     4,
			mutated:  "\treturn false",
		},
		{
			name:     "flip-bool-return false to true",
			src:      "package x\n\nfunc f() bool {\n\treturn false\n}\n",
			operator: "flip-bool-return false->true",
			line:     4,
			mutated:  "\treturn true",
		},
		{
			name:     "drop-error-check",
			src:      "package x\n\nfunc f(err error) {\n\tif err != nil {\n\t\tprintln(\"x\")\n\t}\n}\n",
			operator: "drop-error-check",
			line:     4,
			mutated:  "\tif false {",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var found []Site
			for _, site := range sitesOf(t, tc.src) {
				if site.Operator == tc.operator {
					found = append(found, site)
				}
			}
			if len(found) != 1 {
				t.Fatalf("%s sites = %v, want exactly one", tc.operator, found)
			}
			got := found[0]
			if got.Line != tc.line || got.Mutated != tc.mutated || got.File != "x.go" {
				t.Fatalf("site = %+v, want line %d mutated %q in x.go", got, tc.line, tc.mutated)
			}
			if lines := strings.Split(tc.src, "\n"); got.Original != lines[tc.line-1] {
				t.Fatalf("original = %q, want the line the file holds: %q", got.Original, lines[tc.line-1])
			}
		})
	}
}

// The scan reads tokens, so an operator written in a comment or a string literal is not a site.
func TestCommentsAndStringLiteralsHostNoSite(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "line comments",
			src:  "package x\n\n// if err != nil {\n// return true\nfunc f(a, b int) bool {\n\treturn a > b // a >= b && b < a\n}\n",
			want: []string{"flip-comparison >->>="},
		},
		{
			name: "block comment",
			src:  "package x\n\n/*\nif a >= b {\n\treturn true\n}\n*/\nfunc f() {}\n",
		},
		{
			name: "string literal",
			src:  "package x\n\nfunc f() string {\n\treturn \"if a >= b { return true } a && b\"\n}\n",
		},
		{
			name: "raw string",
			src:  "package x\n\nfunc f() string {\n\treturn `if err != nil { a <= b }`\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, site := range sitesOf(t, tc.src) {
				got = append(got, site.Operator)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("operators = %v, want %v", got, tc.want)
			}
		})
	}
}

// A test file and a generated file are never mutated.
func TestTestFilesAndGeneratedFilesAreSkipped(t *testing.T) {
	root := repo(t, "calib-skip", map[string]string{
		"a_test.go": "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
		"gen.go":    "// Code generated by tool. DO NOT EDIT.\n\npackage x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
		"sites.go":  "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
	})
	perFile, total, err := List(root, []string{"a_test.go", "gen.go", "sites.go"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := map[string]int{"a_test.go": 0, "gen.go": 0, "sites.go": 1}
	if !reflect.DeepEqual(perFile, want) {
		t.Fatalf("per-file = %v, want %v", perFile, want)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
}

// List counts the given files in the order it was handed them and sums what it counted.
func TestListCountsEveryGivenFile(t *testing.T) {
	root := repo(t, "calib-list", map[string]string{
		"first.go":  "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
		"second.go": "package x\n\nfunc f() bool {\n\treturn true\n}\n\nfunc g() bool {\n\treturn false\n}\n",
		"empty.go":  "package x\n\nfunc f() {}\n",
	})
	perFile, total, err := List(root, []string{"first.go", "second.go", "empty.go"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := map[string]int{"first.go": 1, "second.go": 2, "empty.go": 0}
	if !reflect.DeepEqual(perFile, want) {
		t.Fatalf("per-file = %v, want %v", perFile, want)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
}

// Calibration never runs on a branch somebody keeps: the guard answers before anything is read.
func TestListRefusesANonThrowawayBranch(t *testing.T) {
	root := repo(t, "main", map[string]string{
		"x.go": "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
	})
	if _, _, err := List(root, []string{"x.go"}); err == nil {
		t.Fatal("List on branch main = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "calib-") {
		t.Fatalf("error = %q, want the branch prefixes it accepts", err)
	}
}

// The two throwaway prefixes the Python seeder accepted stay the ones the guard accepts.
func TestListAcceptsBothThrowawayPrefixes(t *testing.T) {
	for _, branch := range []string{"calib-519", "skilltest-v18"} {
		t.Run(branch, func(t *testing.T) {
			root := repo(t, branch, map[string]string{
				"x.go": "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
			})
			if _, total, err := List(root, []string{"x.go"}); err != nil || total != 1 {
				t.Fatalf("List = %d sites, %v; want 1, nil", total, err)
			}
		})
	}
}

// manySites is one file with room for a deterministic pick to choose differently between seeds.
const manySites = `package x

func a(v int) bool {
	return v > 1
}

func b(p, q bool) bool {
	return p && q
}

func c() bool {
	return true
}

func d() bool {
	return false
}

func e(err error) {
	if err != nil {
		println("x")
	}
}
`

// invalidAfterNegating is the one construct a negate-if cannot survive: an init statement moved
// inside the parentheses no longer parses, so the seeder must reject it and answer a valid mutant.
const invalidAfterNegating = `package x

func f() int { return 0 }

func g() {
	if n := f(); n > 0 {
		println(n)
	}
}
`

// pythonKey is the shape assets/seed-mutants.py sealed: a JSON array of whole lines, indent two,
// written with ensure_ascii=false.
const pythonKey = `[
  {
    "file": "x.go",
    "line": 4,
    "operator": "flip-comparison >=->>",
    "original": "\treturn a >= b",
    "mutated": "\treturn a > b"
  }
]`

func keySites(t *testing.T, path string) []Site {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	var sites []Site
	if err := json.Unmarshal(body, &sites); err != nil {
		t.Fatalf("key is not the seeder's JSON: %v\n%s", err, body)
	}
	return sites
}

func digestOf(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// The key is the sealed answer: it names the lines it changed, it is hashed as written, and the file
// it keys parses. Replaying it elsewhere must arrive at the same bytes.
func TestSeedWritesASealedKeyThatReplaysUnchanged(t *testing.T) {
	files := map[string]string{"x.go": manySites, "y.go": "package x\n\nfunc y(v int) bool {\n\treturn v < 2\n}\n"}
	seededIn := repo(t, "calib-seed", files)
	keyPath := filepath.Join(t.TempDir(), "key.json")

	seeded, err := Seed(seededIn, []string{"x.go", "y.go"}, 3, 0, keyPath)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if seeded.Applied != 3 {
		t.Fatalf("applied = %d, want 3", seeded.Applied)
	}
	if seeded.KeyPath != keyPath {
		t.Fatalf("key path = %q, want %q", seeded.KeyPath, keyPath)
	}

	body, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if seeded.SHA256 != digestOf(body) {
		t.Fatalf("sha256 = %s, want the digest of the key as written %s", seeded.SHA256, digestOf(body))
	}
	text := string(body)
	for _, field := range []string{`"file":`, `"line":`, `"operator":`, `"original":`, `"mutated":`} {
		if !strings.Contains(text, field) {
			t.Fatalf("key does not use the seeder's field names %s:\n%s", field, text)
		}
	}
	if strings.Contains(text, `\u003`) {
		t.Fatalf("key escapes the operators it records:\n%s", text)
	}
	if !(strings.Index(text, `"file":`) < strings.Index(text, `"line":`) &&
		strings.Index(text, `"line":`) < strings.Index(text, `"operator":`)) {
		t.Fatalf("key does not keep the field order of the seeder:\n%s", text)
	}

	sites := keySites(t, keyPath)
	if len(sites) != 3 {
		t.Fatalf("key holds %d sites, want 3:\n%s", len(sites), text)
	}
	touched := map[string]bool{}
	for _, site := range sites {
		touched[site.File] = true
		lines := strings.Split(read(t, seededIn, site.File), "\n")
		if lines[site.Line-1] != site.Mutated {
			t.Fatalf("%s:%d = %q, want the keyed mutant %q", site.File, site.Line, lines[site.Line-1], site.Mutated)
		}
	}
	var wantFiles []string
	for rel := range touched {
		wantFiles = append(wantFiles, rel)
	}
	sort.Strings(wantFiles)
	if !reflect.DeepEqual(seeded.Files, wantFiles) {
		t.Fatalf("files = %v, want the files the key names %v", seeded.Files, wantFiles)
	}
	for rel := range files {
		if _, err := parser.ParseFile(token.NewFileSet(), rel, read(t, seededIn, rel), 0); err != nil {
			t.Fatalf("%s does not parse after seeding: %v", rel, err)
		}
	}

	replayedIn := repo(t, "calib-seed", files)
	replayed, err := Replay(replayedIn, keyPath)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if replayed.Applied != 3 || !reflect.DeepEqual(replayed.Files, seeded.Files) {
		t.Fatalf("replay = %+v, want 3 sites in %v", replayed, seeded.Files)
	}
	if replayed.SHA256 != seeded.SHA256 {
		t.Fatalf("replay sha256 = %s, want the key's own %s", replayed.SHA256, seeded.SHA256)
	}
	for rel := range files {
		if got, want := read(t, replayedIn, rel), read(t, seededIn, rel); got != want {
			t.Fatalf("%s after replay = %q, want the seeded %q", rel, got, want)
		}
	}
}

// A key the Python seeder wrote replays unchanged: same field names, same whole lines.
func TestReplayAcceptsAKeyThePythonScriptWrote(t *testing.T) {
	root := repo(t, "calib-python", map[string]string{
		"x.go": "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n",
	})
	keyPath := filepath.Join(t.TempDir(), "python-key.json")
	if err := os.WriteFile(keyPath, []byte(pythonKey), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	replayed, err := Replay(root, keyPath)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if replayed.Applied != 1 || !reflect.DeepEqual(replayed.Files, []string{"x.go"}) {
		t.Fatalf("replay = %+v, want one site in x.go", replayed)
	}
	want := "package x\n\nfunc f(a, b int) bool {\n\treturn a > b\n}\n"
	if got := read(t, root, "x.go"); got != want {
		t.Fatalf("replayed file =\n%s\nwant\n%s", got, want)
	}
}

// A seed is a reproducible pick: the same seed answers the same key, another seed another key.
func TestSeedIsDeterministicInItsSeedAndPicksDifferentlyForAnother(t *testing.T) {
	files := map[string]string{"x.go": manySites}
	seedWith := func(seed int64) []byte {
		root := repo(t, "calib-deterministic", files)
		keyPath := filepath.Join(t.TempDir(), "key.json")
		if _, err := Seed(root, []string{"x.go"}, 4, seed, keyPath); err != nil {
			t.Fatalf("Seed(seed %d): %v", seed, err)
		}
		body, err := os.ReadFile(keyPath)
		if err != nil {
			t.Fatalf("read key: %v", err)
		}
		return body
	}
	first, sameFirst := seedWith(0), seedWith(0)
	if !bytes.Equal(first, sameFirst) {
		t.Fatalf("seed 0 answered two keys:\n%s\n%s", first, sameFirst)
	}
	if other := seedWith(7); bytes.Equal(first, other) {
		t.Fatalf("seed 7 answered the key of seed 0:\n%s", first)
	}
}

// Fewer sites than the count asks for is a refusal before anything is written.
func TestSeedRefusesWhenTheCountExceedsTheSites(t *testing.T) {
	files := map[string]string{"x.go": "package x\n\nfunc f(a, b int) bool {\n\treturn a >= b\n}\n"}
	root := repo(t, "calib-count", files)
	keyPath := filepath.Join(t.TempDir(), "key.json")

	if _, err := Seed(root, []string{"x.go"}, 5, 0, keyPath); err == nil {
		t.Fatal("Seed with an unreachable count = nil error, want a refusal")
	}
	if got, want := read(t, root, "x.go"), files["x.go"]; got != want {
		t.Fatalf("file after refusal = %q, want it byte-identical to %q", got, want)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key after refusal: %v, want it absent", err)
	}
}

// The key is the answer: a copy inside the tree would be readable by the run being calibrated.
func TestSeedRefusesAKeyPathInsideTheRoot(t *testing.T) {
	files := map[string]string{"x.go": manySites}
	root := repo(t, "calib-inside", files)
	keyPath := filepath.Join(root, "key.json")

	if _, err := Seed(root, []string{"x.go"}, 1, 0, keyPath); err == nil {
		t.Fatal("Seed with a key inside the tree = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "outside") {
		t.Fatalf("error = %q, want it to say the key belongs outside the tree", err)
	}
	if got, want := read(t, root, "x.go"), files["x.go"]; got != want {
		t.Fatalf("file after refusal = %q, want it byte-identical to %q", got, want)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key after refusal: %v, want it absent", err)
	}
}

// Candidates whose mutation does not parse are rejected; when too few valid ones remain, every seed
// that was applied is reverted and the tree is byte-identical again.
func TestSeedRevertsEverySeedWhenTooFewParse(t *testing.T) {
	files := map[string]string{"x.go": invalidAfterNegating}
	root := repo(t, "calib-parse", files)
	keyPath := filepath.Join(t.TempDir(), "key.json")

	if _, err := Seed(root, []string{"x.go"}, 2, 0, keyPath); err == nil {
		t.Fatal("Seed with one parse-valid seed for two mutants = nil error, want a refusal")
	}
	if got, want := read(t, root, "x.go"), files["x.go"]; got != want {
		t.Fatalf("file after refusal =\n%s\nwant it byte-identical to\n%s", got, want)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key after refusal: %v, want it absent", err)
	}
}

// Replay re-applies a sealed key, and refuses rather than touching a line that moved.
func TestReplayRefusesWhenTheLineNoLongerMatchesItsOriginal(t *testing.T) {
	files := map[string]string{"x.go": manySites}
	seededIn := repo(t, "calib-replay", files)
	keyPath := filepath.Join(t.TempDir(), "key.json")
	if _, err := Seed(seededIn, []string{"x.go"}, 2, 0, keyPath); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	sites := keySites(t, keyPath)

	root := repo(t, "calib-replay", files)
	edited := strings.Split(files["x.go"], "\n")
	edited[sites[0].Line-1] = "\treturn 42"
	write(t, root, "x.go", strings.Join(edited, "\n"))

	if _, err := Replay(root, keyPath); err == nil {
		t.Fatal("Replay over a moved line = nil error, want a refusal")
	}
	if got, want := read(t, root, "x.go"), strings.Join(edited, "\n"); got != want {
		t.Fatalf("file after refusal = %q, want it byte-identical to %q", got, want)
	}
}

// Both mutating entry points refuse a tree that is not a throwaway worktree.
func TestSeedAndReplayRefuseANonThrowawayBranch(t *testing.T) {
	root := repo(t, "main", map[string]string{"x.go": manySites})
	keyPath := filepath.Join(t.TempDir(), "key.json")

	if _, err := Seed(root, []string{"x.go"}, 1, 0, keyPath); err == nil {
		t.Fatal("Seed on branch main = nil error, want a refusal")
	}
	if err := os.WriteFile(keyPath, []byte(pythonKey), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if _, err := Replay(root, keyPath); err == nil {
		t.Fatal("Replay on branch main = nil error, want a refusal")
	}
	if got, want := read(t, root, "x.go"), manySites; got != want {
		t.Fatalf("file after refusals = %q, want it byte-identical to %q", got, want)
	}
}

// One line can host three candidate operators and only one of them may be applied: the key records a
// whole line, so a second mutant of the same line could not match what the first one left behind.
func TestSeedNeverTakesTwoMutantsOfOneLine(t *testing.T) {
	files := map[string]string{"x.go": "package x\n\nfunc f(err error) {\n\tif err != nil {\n\t\tprintln(\"x\")\n\t}\n}\n"}
	root := repo(t, "calib-oneline", files)
	keyPath := filepath.Join(t.TempDir(), "key.json")

	if _, err := Seed(root, []string{"x.go"}, 2, 0, keyPath); err == nil {
		t.Fatal("Seed with two mutants for one line = nil error, want a refusal")
	}
	if got, want := read(t, root, "x.go"), files["x.go"]; got != want {
		t.Fatalf("file after refusal = %q, want it byte-identical to %q", got, want)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key after refusal: %v, want it absent", err)
	}
}
