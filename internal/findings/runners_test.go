package findings_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/findings"
)

func TestCargoInlineWitnessIsReported(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo unavailable")
	}
	d := repo(t)
	put(t, d, "Cargo.toml", "[package]\nname = \"sample\"\nversion = \"0.1.0\"\nedition = \"2021\"\n[lib]\npath = \"lib.rs\"\n")
	put(t, d, "lib.rs", "pub fn value() -> i32 { 0 }\n")
	command(t, d, "git", "add", "Cargo.toml", "lib.rs")
	command(t, d, "git", "commit", "-qm", "Rust base")
	put(t, d, "lib.rs", "pub fn value() -> i32 { 0 }\n#[test]\nfn promise() { assert_eq!(value(), 1); }\n")
	rep, err := bench.RedCheck(d, "cargo test", wait)
	if err == nil || !strings.Contains(err.Error(), "promise: inline Rust unit tests are not supported; move the test to tests/") || len(rep.Red) != 0 {
		t.Fatalf("redcheck: %+v %v", rep, err)
	}
	out, code := findings.Record(d, "cargo test", wait)
	if code != 2 || !strings.Contains(out, "inline Rust unit tests are not supported") {
		t.Fatalf("record: %d %s", code, out)
	}
}

func TestNewRunnerFindingsLifecycle(t *testing.T) {
	for _, runner := range []string{"jest", "vitest", "cargo"} {
		t.Run(runner, func(t *testing.T) {
			d := repo(t)
			// Keep staged suites alongside their disposable npm binaries, visible to the runner.
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			var suite, testFile, tests, codeFile, broken, fixed, failing, passing string
			if runner == "cargo" {
				if _, err := exec.LookPath("cargo"); err != nil {
					t.Skip("cargo unavailable")
				}
				put(t, d, "Cargo.toml", "[package]\nname = \"sample\"\nversion = \"0.1.0\"\nedition = \"2021\"\n[lib]\npath = \"lib.rs\"\n")
				suite, testFile, codeFile = "cargo test", "tests/promise.rs", "lib.rs"
				broken, fixed = "pub fn value() -> i32 { 0 }\n", "pub fn value() -> i32 { 1 }\n"
				tests = "// Keeps the promise.\n#[test]\nfn promise() { assert_eq!(sample::value(), 1); }\n#[test]\nfn passes() { assert_eq!(1, 1); }\n"
				failing, passing = "promise", "passes"
				if err := os.MkdirAll(filepath.Join(d, "tests"), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := exec.LookPath("npm"); err != nil {
					t.Skip("npm unavailable")
				}
				if _, err := exec.LookPath("node"); err != nil {
					t.Skip("node unavailable")
				}
				version := "29.7.0"
				if runner == "vitest" {
					version = "2.1.9"
				}
				// Install only exact, canonical runner packages into this disposable project.
				cache := t.TempDir()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				probe := exec.CommandContext(ctx, "npm", "view", runner+"@"+version, "version", "--fetch-retries=0", "--fetch-timeout=10000", "--cache", cache)
				probe.Dir = d
				if out, err := probe.CombinedOutput(); err != nil {
					t.Skipf("npm registry unavailable for %s: %v %s", runner, err, out)
				}
				install := exec.CommandContext(ctx, "npm", "install", "--save-exact", "--ignore-scripts", "--no-fund", "--no-audit", "--cache", cache, runner+"@"+version)
				install.Dir = d
				if out, err := install.CombinedOutput(); err != nil {
					for _, networkError := range []string{"ENOTFOUND", "EAI_AGAIN", "ETIMEDOUT", "ECONNRESET", "ECONNREFUSED"} {
						if strings.Contains(string(out), networkError) {
							t.Skipf("npm registry unreachable installing %s: %s", runner, out)
						}
					}
					t.Fatalf("install %s: %v %s", runner, err, out)
				}
				t.Setenv("PATH", filepath.Join(d, "node_modules", ".bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
				put(t, d, ".gitignore", "node_modules/\n")
				suite = "jest --runInBand"
				if runner == "vitest" {
					suite = "vitest run --globals --maxWorkers=1 --minWorkers=1"
				}
				testFile, codeFile = "promise.test.js", "value.js"
				broken, fixed = "exports.value = () => 0;\n", "exports.value = () => 1;\n"
				tests = "const { value } = require('./value');\ndescribe('outer', () => {\n// Keeps the promise.\nit('promise', () => { expect(value()).toBe(1); });\nit('passes', () => { expect(1).toBe(1); });\n});\n"
				failing, passing = "outer promise", "outer passes"
			}
			put(t, d, codeFile, broken)
			command(t, d, "git", "add", ".")
			command(t, d, "git", "commit", "-qm", "runner base")
			put(t, d, testFile, tests)
			outcomes, code, out, err := bench.PerTest(d, suite, wait)
			if err != nil || code == 0 || len(outcomes) != 2 || outcomes[failing] || !outcomes[passing] {
				t.Fatalf("outcomes: %v %d %v %s", outcomes, code, err, out)
			}
			out, code = findings.Record(d, suite, wait)
			if code != 0 || !strings.Contains(out, "F1 recorded "+testFile+" "+failing) {
				t.Fatalf("record: %d %s", code, out)
			}
			out, code = findings.Check(d, wait)
			if code != 1 || !strings.Contains(out, "F1 open "+testFile+" "+failing) {
				t.Fatalf("open: %d %s", code, out)
			}
			put(t, d, codeFile, fixed)
			out, code = findings.Check(d, wait)
			if code != 0 || !strings.Contains(out, "F1 fixed "+testFile+" "+failing) {
				t.Fatalf("fixed: %d %s", code, out)
			}
		})
	}
}
