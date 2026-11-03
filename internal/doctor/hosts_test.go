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

// S1: Pi without its extension is a state doctor reports, not a host it hides: the extension is
// opt-in, and a reader has to know whether this machine opted in.
func TestHostInventoryReportsAPiExtensionThatIsNotInstalled(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent", "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}

	pi := piOf(t, home)
	if pi.ExtensionState != ExtensionAbsent {
		t.Fatalf("extension state = %q, want %q", pi.ExtensionState, ExtensionAbsent)
	}
	if pi.Transport != TransportAbsent {
		t.Fatalf("transport = %q, want %q without an extension", pi.Transport, TransportAbsent)
	}
	if !strings.Contains(pi.TransportNote, "tsp sync --hosts pi") {
		t.Fatalf("note = %q, want the opt-in command", pi.TransportNote)
	}
}

// S1/S4: an extension that matches what this build ships is the only state that reads as
// installed, and it is decided by content, not by the file's presence.
func TestHostInventoryReportsAPiExtensionThatMatchesTheBuild(t *testing.T) {
	home := t.TempDir()
	installExtension(t, home, shippedExtension(t))

	pi := piOf(t, home)
	if pi.ExtensionState != ExtensionCurrent {
		t.Fatalf("extension state = %q, want %q", pi.ExtensionState, ExtensionCurrent)
	}
	if pi.Transport != TransportExtension {
		t.Fatalf("transport = %q, want %q", pi.Transport, TransportExtension)
	}
}

// S1: an older extension on disk is stale, and the report names the command that restores it.
func TestHostInventoryReportsAPiExtensionThatIsStale(t *testing.T) {
	home := t.TempDir()
	installExtension(t, home, append(shippedExtension(t), []byte("\n// an older build\n")...))

	pi := piOf(t, home)
	if pi.ExtensionState != ExtensionStale {
		t.Fatalf("extension state = %q, want %q", pi.ExtensionState, ExtensionStale)
	}
	if !strings.Contains(pi.TransportNote, "tsp sync --hosts pi") {
		t.Fatalf("note = %q, want the command that restores it", pi.TransportNote)
	}
}

// S1: a file that is there but is not the extension is broken, not stale: a truncated write has
// to be told apart from an older version, because only one of them is a plain re-sync.
func TestHostInventoryReportsAPiExtensionThatIsBroken(t *testing.T) {
	home := t.TempDir()
	installExtension(t, home, []byte("export const half = \n"))

	pi := piOf(t, home)
	if pi.ExtensionState != ExtensionBroken {
		t.Fatalf("extension state = %q, want %q", pi.ExtensionState, ExtensionBroken)
	}
}

// S1: the rendered report says the same thing as the JSON, and it stops counting Pi's skills as
// if the extension were a skills directory.
func TestReportRendersThePiExtensionState(t *testing.T) {
	home := t.TempDir()
	installExtension(t, home, append(shippedExtension(t), []byte("\n// an older build\n")...))

	text := Report{Hosts: HostInventory(home, filepath.Join(home, ".claude"))}.String()
	if !strings.Contains(text, "extension stale") {
		t.Fatalf("hosts block does not report the stale extension:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pi ") && strings.Contains(line, "/25 skills") {
			t.Fatalf("hosts block counts skills for an extension host:\n%s", text)
		}
	}
}

// installExtension writes the extension where a sync puts it.
func installExtension(t *testing.T, home string, content []byte) {
	t.Helper()
	path := filepath.Join(home, ".pi", "agent", "extensions", "tpp", "index.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func shippedExtension(t *testing.T) []byte {
	t.Helper()
	data, err := fs.ReadFile(assets.Hosts(), "pi/tsp.ts")
	if err != nil {
		t.Fatalf("read the shipped extension: %v", err)
	}
	return data
}

func piOf(t *testing.T, home string) HostStatus {
	t.Helper()
	for _, h := range HostInventory(home, filepath.Join(home, ".claude")) {
		if h.Name == "pi" {
			return h
		}
	}
	t.Fatalf("inventory has no pi host")
	return HostStatus{}
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
