package doctor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/assets"
	"github.com/alesierraalta/tsp/internal/sync"
)

// Transport says what happens when an agent finishes a turn, and it is the honest answer for
// this machine, not the intended one. A host that receives skills and no transport says so.
const (
	// TransportWired means a Stop hook runs the gate: the host's own settings file holds it.
	TransportWired = "wired"
	// TransportInert means an adapter loads and stays quiet on purpose, because an automatic
	// check with no session binding behind it would fail closed on every turn.
	TransportInert = "inert"
	// TransportExtension means a host extension carries the payload; it is installed and this
	// report does not claim it has been observed running.
	TransportExtension = "extension"
	// TransportAbsent means skills only: the host has no transport TSP wires today.
	TransportAbsent = "absent"
)

// Extension states answer what the file an extension host loads actually is. A host whose
// transport is an extension reports one of these instead of a skills count: its skills come from
// the Claude directory Pi reads too, so counting them here said nothing about the transport.
const (
	// ExtensionCurrent means the file matches the copy this build ships.
	ExtensionCurrent = "up-to-date"
	// ExtensionStale means an older or edited copy is on disk, which a sync replaces.
	ExtensionStale = "stale"
	// ExtensionBroken means the file is there and is not the extension, as a truncated write is.
	ExtensionBroken = "broken"
	// ExtensionUnreadable means the file exists and cannot be read.
	ExtensionUnreadable = "unreadable"
	// ExtensionAbsent means the opt-in host is present and nothing was installed for it.
	ExtensionAbsent = "absent"
)

// TestedPiVersion is the Pi version this build's extension was exercised against. A different
// version is reported, not refused: the extension negotiates nothing, and the events it needs
// belong to the host, so a mismatch is information a reader can act on.
const TestedPiVersion = "1.5.0"

// HostStatus is one agent host on this machine: where it loads TSP skills from, how many are
// there now, and what carries the gate's answer, if anything.
type HostStatus struct {
	Name          string `json:"name"`
	ConfigDir     string `json:"config_dir"`
	SkillsDir     string `json:"skills_dir"`
	SkillsPresent int    `json:"skills_present"`
	// SkillsDrifted counts skills whose installed files no longer match what this build ships:
	// the copy a sync would refuse to overwrite, and the one a --force would silently replace.
	SkillsDrifted int      `json:"skills_drifted"`
	DriftedPaths  []string `json:"drifted_paths,omitempty"`
	Transport     string   `json:"transport"`
	TransportNote string   `json:"transport_note,omitempty"`
	// ExtensionPath, ExtensionState and ExtensionNote describe the file an extension host loads.
	// They stay empty for hosts whose transport is a hook or nothing at all.
	ExtensionPath  string `json:"extension_path,omitempty"`
	ExtensionState string `json:"extension_state,omitempty"`
	ExtensionNote  string `json:"extension_note,omitempty"`
	// PiVersion is the host's own version, read when it is on PATH, so a reader can compare it
	// with TestedPiVersion instead of guessing.
	PiVersion string `json:"pi_version,omitempty"`
	// PiFound records whether pi was on PATH at all, so an empty PiVersion is not rendered as
	// "not on PATH" when the command was found but did not answer.
	PiFound bool `json:"pi_found,omitempty"`
}

// hostTransports is what TSP knows how to put in front of each host, and the sentence a reader
// needs when the answer is "skills only". Claude is absent here because its transport is the
// Stop hook in the config dir this report already inspected: it is read, not assumed.
var hostTransports = map[string]struct{ kind, note string }{
	"opencode": {
		TransportInert,
		"the plugin loads and stays quiet: session.idle is disabled until a session binding exists",
	},
	"gemini": {
		TransportAbsent,
		"skills only: TSP wires no Stop hook for this host",
	},
	"codex": {
		TransportAbsent,
		"skills only: TSP wires no Stop hook for this host",
	},
	"antigravity": {
		TransportAbsent,
		"skills only: agy reads hooks from ~/.gemini/antigravity-cli/hooks and TSP does not wire them yet",
	},
}

// HostInventory lists every host found under home with the transport it really has. Pi is the
// opt-in host and is never discovered: it appears whenever the host itself is on this machine,
// because an installed Pi with no extension is a state a reader has to be able to see.
func HostInventory(home, claudeConfigDir string) []HostStatus {
	discovered := sync.DiscoverHosts(home, claudeConfigDir).Hosts
	statuses := make([]HostStatus, 0, len(discovered)+1)
	for _, host := range discovered {
		status := HostStatus{
			Name:          host.Name,
			ConfigDir:     host.ConfigDir,
			SkillsDir:     host.SkillsDir,
			SkillsPresent: skillsPresent(host.SkillsDir),
		}
		status.Transport, status.TransportNote = transportFor(host)
		status.SkillsDrifted, status.DriftedPaths = skillsDrifted(host.SkillsDir)
		statuses = append(statuses, status)
	}
	if home != "" {
		if pi, ok := piHostStatus(home); ok {
			statuses = append(statuses, pi)
		}
	}
	return statuses
}

// transportFor answers one host's transport: Claude's is read from the settings file that would
// hold it, Pi's from whether its extension landed, and the rest from the table above.
func transportFor(host sync.Host) (string, string) {
	switch host.Name {
	case "claude":
		if kind, _ := hookWired(filepath.Join(host.ConfigDir, "settings.json")); kind != HookNone {
			return TransportWired, "Stop hook in settings.json"
		}
		return TransportAbsent, "skills only: no Stop hook in settings.json (run: tsp sync)"
	case "codex":
		path := filepath.Join(host.ConfigDir, "hooks.json")
		if kind, _ := hookWired(path); kind != HookNone {
			return TransportWired, "Stop hook in hooks.json; the Codex CLI asks for hook trust before it runs one (tsp writes no trusted_hash)"
		}
		return TransportAbsent, "skills only: no Stop hook in hooks.json (run: tsp sync)"
	case "gemini":
		if kind, _ := hookWired(filepath.Join(host.ConfigDir, "settings.json")); kind != HookNone {
			return TransportWired, "AfterAgent hook in settings.json"
		}
		return TransportAbsent, "skills only: no AfterAgent hook in settings.json (run: tsp sync)"
	}
	if known, ok := hostTransports[host.Name]; ok {
		return known.kind, known.note
	}
	return TransportAbsent, "skills only: no transport is known for this host"
}

// piHostStatus reports Pi whenever the host is on this machine, with the state of the file its
// extension would load. The extension is opt-in, so absence is reported, never assumed away.
func piHostStatus(home string) (HostStatus, bool) {
	configDir := filepath.Join(home, ".pi")
	if info, err := os.Stat(configDir); err != nil || !info.IsDir() {
		return HostStatus{}, false
	}
	skillsDir := filepath.Join(configDir, "agent", "extensions")
	extension := filepath.Join(skillsDir, "tpp", "index.ts")
	state, note := extensionStatus(extension)
	version, found := piVersion()
	status := HostStatus{
		Name:           "pi",
		ConfigDir:      configDir,
		SkillsDir:      skillsDir,
		ExtensionPath:  extension,
		ExtensionState: state,
		ExtensionNote:  note,
		PiVersion:      version,
		PiFound:        found,
		Transport:      TransportExtension,
		TransportNote:  note,
	}
	if state == ExtensionAbsent {
		status.Transport = TransportAbsent
	}
	return status, true
}

// extensionStatus answers what the file a host loads is, by content: a file that merely exists
// never reads as installed, and an older copy is told apart from a truncated one.
func extensionStatus(path string) (string, string) {
	installed, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ExtensionAbsent, "not installed: opt-in, install with: tsp sync --hosts pi"
	}
	if err != nil {
		return ExtensionUnreadable, fmt.Sprintf("cannot read %s: %v", path, err)
	}
	if !bytes.Contains(installed, []byte("export default")) {
		return ExtensionBroken, "at " + path + ", does not look like the extension: no default export (run: tsp sync --hosts pi --force)"
	}
	shipped, err := fs.ReadFile(assets.Hosts(), "pi/tsp.ts")
	if err != nil {
		return ExtensionUnreadable, "this build cannot read its own extension: " + err.Error()
	}
	if sha256.Sum256(installed) == sha256.Sum256(shipped) {
		return ExtensionCurrent, "at " + path + ", matches the extension this build ships"
	}
	return ExtensionStale, "at " + path + ", differs from the extension this build ships (run: tsp sync --hosts pi)"
}

// HostProblems names the host states a reader has to act on. An extension that is there and wrong
// is not healthy: a verdict that called it healthy while the host line said stale would be the
// silent success this classification exists to remove.
func HostProblems(hosts []HostStatus) []string {
	problems := make([]string, 0, 1)
	for _, host := range hosts {
		switch host.ExtensionState {
		case "", ExtensionCurrent, ExtensionAbsent:
			continue
		}
		problems = append(problems, "pi extension "+host.ExtensionState+": "+host.ExtensionNote)
	}
	return problems
}

// piVersion reads the host's version when it is on PATH. An unreadable or slow answer is empty:
// the report says the version is unknown rather than failing a doctor run.
func piVersion() (string, bool) {
	path, err := exec.LookPath("pi")
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	// Output waits for every holder of the pipe. A wrapper that leaves a child running would
	// otherwise outlive the timeout, so close the pipes shortly after the kill.
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), true
}

// skillsPresent counts this build's skills that are actually readable in a host's directory, so
// a host with none is visible as a host with none instead of as a host that merely exists.
// skillsDrifted names the skills whose files on disk differ from the manifest this build embeds.
// A file missing on disk is not drift: skillsPresent already reports that, and a sync installs it.
func skillsDrifted(skillsDir string) (int, []string) {
	count := 0
	paths := make([]string, 0, 2)
	for _, name := range assets.SkillNames() {
		drifted := false
		err := fs.WalkDir(assets.Skills(), name, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			shipped, err := fs.ReadFile(assets.Skills(), path)
			if err != nil {
				return nil
			}
			installed, err := os.ReadFile(filepath.Join(skillsDir, filepath.FromSlash(path)))
			if err != nil {
				return nil
			}
			if sha256.Sum256(installed) != sha256.Sum256(shipped) {
				drifted = true
				return fs.SkipAll
			}
			return nil
		})
		if err == nil && drifted {
			count++
			paths = append(paths, name)
		}
	}
	return count, paths
}

func skillsPresent(skillsDir string) int {
	present := 0
	for _, name := range assets.SkillNames() {
		if info, err := os.Stat(filepath.Join(skillsDir, name, "SKILL.md")); err == nil && !info.IsDir() {
			present++
		}
	}
	return present
}

// writeHosts is the block a reader with more than one agent needs: which hosts this machine has,
// how much of TSP each one holds, and what carries the answer when the agent stops.
func (r Report) writeHosts(b *strings.Builder) {
	if len(r.Hosts) == 0 {
		return
	}
	fmt.Fprintf(b, "\nhosts\n")
	total := len(assets.SkillNames())
	for _, h := range r.Hosts {
		if h.ExtensionState != "" {
			fmt.Fprintf(b, "  %-12s extension %s: %s%s\n", h.Name, h.ExtensionState, h.ExtensionNote, piVersionSuffix(h.PiVersion, h.PiFound))
			continue
		}
		fmt.Fprintf(b, "  %-12s %d/%d skills · %s", h.Name, h.SkillsPresent, total, h.Transport)
		if h.TransportNote != "" {
			fmt.Fprintf(b, ": %s", h.TransportNote)
		}
		if h.SkillsDrifted > 0 {
			fmt.Fprintf(b, " · %d modified since install: run tsp sync --dry-run to list them", h.SkillsDrifted)
		}
		b.WriteString("\n")
	}
}

// piVersionSuffix reports the host version against the one this build was tested with, so a
// mismatch is visible without being treated as a failure.
func piVersionSuffix(version string, found bool) string {
	switch {
	case !found:
		return " · pi not on PATH"
	case version == "":
		return " · pi did not answer"
	case version == TestedPiVersion:
		return " · pi " + version
	default:
		return " · pi " + version + " (this build was tested with " + TestedPiVersion + ")"
	}
}
