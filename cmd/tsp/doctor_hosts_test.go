package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S4: a stale Pi extension is not healthy, even when the Claude side is fine. The verdict is what
// a script reads, so it must not say healthy while the host line says stale.
func TestDoctorFailsWhenThePiExtensionIsStale(t *testing.T) {
	home := syncTestHome(t)
	entry := filepath.Join(home, ".pi", "agent", "extensions", "tpp", "index.ts")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("// an older build\nexport default function () {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLIWithHome(t, home, buildCLI(t), "doctor", "--config-dir", filepath.Join(home, ".claude"), "--json")
	if code == 0 {
		t.Fatalf("doctor exits 0 with a stale extension:\n%s", out)
	}
	if !strings.Contains(out, "pi extension stale") {
		t.Fatalf("the verdict does not name the stale extension:\n%s", out)
	}
}

// S4, the setup side: setup shares doctorReport but is Claude-centered and refuses --hosts pi, so
// a stale opt-in extension must not turn a successful setup into a failure. The host stays visible
// in the report; only the doctor command reads it into the verdict.
func TestDoctorReportLeavesTheVerdictToTheDoctorCommand(t *testing.T) {
	home := syncTestHome(t)
	entry := filepath.Join(home, ".pi", "agent", "extensions", "tpp", "index.ts")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("// an older build\nexport default function () {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := doctorReport(filepath.Join(home, ".claude"), "standalone")
	stale := false
	for _, host := range report.Hosts {
		if host.Name == "pi" && host.ExtensionState == "stale" {
			stale = true
		}
	}
	if !stale {
		t.Fatalf("the report does not name the stale host: %+v", report.Hosts)
	}
	for _, problem := range report.Problems {
		if strings.Contains(problem, "pi extension") {
			t.Fatalf("the setup report took the doctor's verdict: %q", problem)
		}
	}
}

// S1/S4: the opt-in host with no extension is a state, not a problem: doctor reports it and still
// lets a healthy installation read as healthy.
func TestDoctorDoesNotFailOnAnAbsentOptInExtension(t *testing.T) {
	home := syncTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent", "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, _ := runCLIWithHome(t, home, buildCLI(t), "doctor", "--config-dir", filepath.Join(home, ".claude"), "--json")
	if !strings.Contains(out, `"extension_state": "absent"`) {
		t.Fatalf("the report does not name the absent extension:\n%s", out)
	}
	if strings.Contains(out, "pi extension absent") {
		t.Fatalf("an absent opt-in extension became a problem:\n%s", out)
	}
}
