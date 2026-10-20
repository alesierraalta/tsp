package findings_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

func TestD3NodeOwnBodyFingerprint(t *testing.T) {
	if _, e := exec.LookPath("node"); e != nil {
		t.Skip("node unavailable")
	}
	d := repo(t)
	put(t, d, "package.json", "{\"type\":\"module\"}\n")
	command(t, d, "git", "add", "package.json")
	command(t, d, "git", "commit", "-qm", "node base")
	source := "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\n// Keeps delimiters safe.\ntest('promise', () => { /* ) */ assert.equal(')', '('); });\n"
	put(t, d, "promise.test.mjs", source)
	out, c := findings.Record(d, "node --test promise.test.mjs", wait)
	if c != 0 {
		t.Fatalf("record: %d %s", c, out)
	}
	put(t, d, "promise.test.mjs", source+"// Unrelated text must not alter the fingerprint.\n")
	out, c = findings.Check(d, wait)
	want := "review 1 on topic: 0 fixed, 0 open, 1 new, 0 regressed\nF1 open promise.test.mjs promise — Keeps delimiters safe.\n"
	if c != 1 || out != want {
		t.Fatalf("node report: %d %q want %q", c, out, want)
	}
	put(t, d, "promise.test.mjs", strings.ReplaceAll(source, "assert.equal(')', '(')", "assert.equal(')', ')')"))
	out, c = findings.Check(d, wait)
	want = "review 2 on topic: 0 fixed, 0 open, 0 new, 0 regressed, 1 need review\nF1 test-changed promise.test.mjs promise — Keeps delimiters safe.\n"
	if c != 1 || out != want {
		t.Fatalf("node changed: %d %q want %q", c, out, want)
	}
}
