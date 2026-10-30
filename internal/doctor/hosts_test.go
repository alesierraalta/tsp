package doctor

import (
	"github.com/alesierraalta/tsp/internal/assets"
	"github.com/alesierraalta/tsp/internal/sync"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S3: a skill edited on disk after install is reported, so doctor never calls a drifting host
// healthy while sync would silently skip the edit.
func TestHostInventoryReportsSkillsModifiedSinceInstall(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := assets.SkillNames()[0]
	installSkill(t, skillsDir, name)

	if got := driftedOf(t, home); got.SkillsDrifted != 0 {
		t.Fatalf("a fresh install drifted = %d, want 0", got.SkillsDrifted)
	}

	edit := filepath.Join(skillsDir, name, "SKILL.md")
	shipped, err := os.ReadFile(edit)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(edit, append(shipped, []byte("\n## Local edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	got := driftedOf(t, home)
	if got.SkillsDrifted != 1 {
		t.Fatalf("claude drifted = %d, want the one edited skill", got.SkillsDrifted)
	}
	if len(got.DriftedPaths) != 1 || got.DriftedPaths[0] != name {
		t.Fatalf("drifted paths = %v, want [%s]", got.DriftedPaths, name)
	}
}

// installSkill copies one shipped skill to disk, which is what a sync leaves behind.
func installSkill(t *testing.T, skillsDir, name string) {
	t.Helper()
	err := fs.WalkDir(assets.Skills(), name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, name)
		dest := filepath.Join(skillsDir, name, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := fs.ReadFile(assets.Skills(), path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// driftedOf reports the Claude host of a fixture home.
func driftedOf(t *testing.T, home string) HostStatus {
	t.Helper()
	for _, h := range HostInventory(home, filepath.Join(home, ".claude")) {
		if h.Name == "claude" {
			return h
		}
	}
	t.Fatalf("inventory has no claude host")
	return HostStatus{}
}

// S2: doctor reports each host's real state, and no host claims a transport it does not have.
func TestHostInventoryReportsEachTransportAsItIs(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{
		filepath.Join(".claude", "skills"),
		filepath.Join(".claude"),
		filepath.Join(".config", "opencode", "skills"),
		filepath.Join(".gemini", "antigravity", "skills"),
		filepath.Join(".pi", "agent", "extensions", "tpp"),
	} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Claude is the one host whose transport TSP wires, and only when the Stop hook is there.
	settings := `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"tsp gate"}]}]}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pi is opt-in and its extension is what carries the payload.
	if err := os.WriteFile(filepath.Join(home, ".pi", "agent", "extensions", "tpp", "index.ts"), []byte("// extension\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// One skill installed in Claude, none elsewhere: the count has to be per host, not global.
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills", "test-strategy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "skills", "test-strategy", "SKILL.md"), []byte("# skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hosts := HostInventory(home, filepath.Join(home, ".claude"))
	byName := map[string]HostStatus{}
	for _, h := range hosts {
		byName[h.Name] = h
	}

	wantClaude, ok := byName["claude"]
	if !ok {
		t.Fatalf("inventory has no claude host: %+v", hosts)
	}
	if wantClaude.Transport != TransportWired {
		t.Errorf("claude transport = %q, want %q", wantClaude.Transport, TransportWired)
	}
	if wantClaude.SkillsPresent != 1 {
		t.Errorf("claude skills present = %d, want the one installed", wantClaude.SkillsPresent)
	}

	if got := byName["opencode"].Transport; got != TransportInert {
		t.Errorf("opencode transport = %q, want %q: its adapter loads and stays quiet on purpose", got, TransportInert)
	}
	if got := byName["antigravity"].Transport; got != TransportAbsent {
		t.Errorf("antigravity transport = %q, want %q", got, TransportAbsent)
	}
	if note := byName["antigravity"].TransportNote; !strings.Contains(note, "antigravity-cli/hooks") {
		t.Errorf("antigravity note = %q, want it to name the hooks directory agy reads", note)
	}
	if got := byName["pi"].Transport; got != TransportExtension {
		t.Errorf("pi transport = %q, want %q when its extension is installed", got, TransportExtension)
	}
	for name, h := range byName {
		if h.Transport == TransportWired && name != "claude" {
			t.Errorf("%s claims a wired transport; only claude has one", name)
		}
	}
}

// A host that is present but whose hook file is gone has no transport, and the inventory says so
// instead of reusing the answer it gave for a different machine.
func TestHostInventoryWithoutTheStopHookReportsNoTransport(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	hosts := HostInventory(home, filepath.Join(home, ".claude"))
	if len(hosts) != 1 || hosts[0].Name != "claude" {
		t.Fatalf("hosts = %+v, want only claude", hosts)
	}
	if hosts[0].Transport != TransportAbsent {
		t.Fatalf("claude transport = %q, want %q without a Stop hook", hosts[0].Transport, TransportAbsent)
	}
}

// The rendered report carries the same sentences the JSON does, so a reader and a program cannot
// disagree about a host that has skills and no transport.
func TestReportRendersTheHostsBlock(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	report := Report{Hosts: HostInventory(home, filepath.Join(home, ".claude"))}
	text := report.String()
	if !strings.Contains(text, "\nhosts\n") {
		t.Fatalf("report has no hosts block:\n%s", text)
	}
	if !strings.Contains(text, "antigravity") || !strings.Contains(text, "absent") {
		t.Fatalf("hosts block does not name antigravity's absent transport:\n%s", text)
	}
}

// An AfterAgent gate is a wired gate: the event name follows the host, not the gate.
func TestGeminiWithAnAfterAgentHookIsWired(t *testing.T) {
	dir := t.TempDir()
	settings := `{"hooks":{"AfterAgent":[{"matcher":"","hooks":[{"type":"command","command":"tsp gate"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	kind, note := transportFor(sync.Host{Name: "gemini", ConfigDir: dir})
	if kind != TransportWired || !strings.Contains(note, "AfterAgent") {
		t.Fatalf("kind = %q note = %q, want wired on AfterAgent", kind, note)
	}
}
