package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alesierraalta/tsp/internal/manifest"
)

func TestDiscoverySkipsAHostWithNoConfigDir(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{".claude", ".gemini"} {
		if err := os.Mkdir(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	discovery := DiscoverHosts(home, "")
	if got := hostNames(discovery.Hosts); len(got) != 2 || got[0] != "claude" || got[1] != "gemini" {
		t.Fatalf("discovered hosts = %v, want claude and gemini", got)
	}
}

func TestDiscoveryFindsTheFourKnownHosts(t *testing.T) {
	home := homeWith(t, ".claude", filepath.Join(".config", "opencode"), ".gemini", ".codex")

	discovery := DiscoverHosts(home, "")
	if got := hostNames(discovery.Hosts); len(got) != 4 || got[0] != "claude" || got[1] != "opencode" || got[2] != "gemini" || got[3] != "codex" {
		t.Fatalf("discovered hosts = %v, want all four known hosts", got)
	}
	wantSkills := map[string]string{
		"claude":   filepath.Join(home, ".claude", "skills"),
		"opencode": filepath.Join(home, ".config", "opencode", "skills"),
		"gemini":   filepath.Join(home, ".gemini", "skills"),
		"codex":    filepath.Join(home, ".codex", "skills"),
	}
	for _, host := range discovery.Hosts {
		if host.SkillsDir != wantSkills[host.Name] {
			t.Errorf("%s skills dir = %s, want %s", host.Name, host.SkillsDir, wantSkills[host.Name])
		}
	}
}

func hostNames(hosts []Host) []string {
	names := make([]string, 0, len(hosts))
	for _, host := range hosts {
		names = append(names, host.Name)
	}
	return names
}

// Pi is opt-in: discovery never offers it, and the constructor names the extension directory
// Pi actually loads (<config>/agent/extensions/<component>/index.ts).
func TestDiscoveryNeverOffersPiAndPiHostFollowsTheExtensionConvention(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent", "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, host := range DiscoverHosts(home, "").Hosts {
		if host.Name == "pi" {
			t.Fatal("discovery must not offer the opt-in pi host")
		}
	}

	host := PiHost(home)
	if host.Name != "pi" || host.ConfigDir != filepath.Join(home, ".pi") || host.SkillsDir != filepath.Join(home, ".pi", "agent", "extensions") {
		t.Fatalf("PiHost(home) = %+v, want the .pi config dir with agent/extensions as the managed root", host)
	}
}

// Anti-Gravity keeps its skills under ~/.gemini/antigravity/skills, beside the Gemini CLI's own
// ~/.gemini/skills. They are two hosts that read two different directories: installing into one
// must not be reported as installing into the other.
func TestDiscoveryFindsAntigravityBesideTheGeminiCli(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{filepath.Join(".gemini", "skills"), filepath.Join(".gemini", "antigravity", "skills")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// The Gemini CLI writes settings.json the first time it runs; TSP never does.
	if err := os.WriteFile(filepath.Join(home, ".gemini", "settings.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	discovery := DiscoverHosts(home, "")
	want := map[string]string{
		"gemini":      filepath.Join(home, ".gemini", "skills"),
		"antigravity": filepath.Join(home, ".gemini", "antigravity", "skills"),
	}
	seen := map[string]string{}
	for _, host := range discovery.Hosts {
		seen[host.Name] = host.SkillsDir
	}
	for name, skillsDir := range want {
		if seen[name] != skillsDir {
			t.Errorf("%s skills dir = %q, want %q", name, seen[name], skillsDir)
		}
	}
	if len(discovery.Hosts) != 2 {
		t.Fatalf("discovered hosts = %v, want gemini and antigravity", hostNames(discovery.Hosts))
	}
}

// An Anti-Gravity-only machine has ~/.gemini because agy keeps its tree there. That is not the
// Gemini CLI, and installing a second copy of every skill for a host that is not there is the
// misreport S2 forbids.
func TestDiscoveryDoesNotMistakeAntigravityForTheGeminiCli(t *testing.T) {
	home := homeWith(t, filepath.Join(".gemini", "antigravity", "skills"), filepath.Join(".gemini", "antigravity-cli"))

	discovery := DiscoverHosts(home, "")
	if got := hostNames(discovery.Hosts); len(got) != 1 || got[0] != "antigravity" {
		t.Fatalf("discovered hosts = %v, want only antigravity", got)
	}
}

// A host discovery offers must also be a host the skill tree claims. The two lists are one
// decision kept in two places: when they drift, doctor reports the host and sync never writes
// to it, which is exactly what happened to Anti-Gravity.
func TestEveryDiscoveredHostReceivesSkills(t *testing.T) {
	for _, name := range KnownHosts() {
		if name == "pi" {
			continue // opt-in extension host: its payload is the adapter, never the skill tree
		}
		claimed := false
		for _, component := range manifest.Components() {
			if component.Kind == manifest.KindSkill && manifest.AppliesTo(name, component) {
				claimed = true
				break
			}
		}
		if !claimed {
			t.Fatalf("%s is discovered but no skill component claims it: add it to manifest.skillHosts", name)
		}
	}
}

// homeWith builds a throwaway home holding exactly these directories, so a discovery test states
// what exists instead of writing it out each time.
func homeWith(t *testing.T, dirs ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}
