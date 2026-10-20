package findings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerWitnessBodies(t *testing.T) {
	for _, tc := range []struct{ file, name, source, unrelated, changed string }{
		{"sample.test.ts", "outer inner promise", `describe('other', () => { it('promise', () => { throw 0; }); });
describe('outer', () => { describe('inner', () => {
// Keeps the promise.
it('promise', () => { expect(value()).toBe(1); });
}); });`, "throw 0", "toBe(1)"},
		{"sample.rs", "tests::promise", `#[cfg(test)]
mod tests {
// Keeps the promise.
#[test]
#[should_panic]
fn promise() { assert_eq!(value(), 1); }
fn helper() { panic!("unrelated"); }
}`, "unrelated", "#[should_panic]"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			root := t.TempDir()
			write := func(s string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, tc.file), []byte(s), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(tc.source)
			file, hash, promise := locate(root, tc.name, []string{tc.file})
			if file != tc.file || hash == "" || promise != "Keeps the promise." {
				t.Fatalf("locate: %q %q %q", file, hash, promise)
			}
			write(strings.ReplaceAll(tc.source, tc.unrelated, "irrelevant"))
			_, got, _ := locate(root, tc.name, []string{tc.file})
			if got != hash {
				t.Fatal("unrelated code changed fingerprint")
			}
			write(strings.ReplaceAll(tc.source, tc.changed, ""))
			_, got, _ = locate(root, tc.name, []string{tc.file})
			if got == hash {
				t.Fatal("witness change not detected")
			}
		})
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "helper.rs"), []byte("fn helper() {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, _, _ := locate(root, "helper", []string{"helper.rs"}); file != "" {
		t.Fatal("unmarked Rust function is not a test")
	}
}
