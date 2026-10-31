package doctor

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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

// HostInventory lists every host found under home with the transport it really has. Pi is not
// discovered — it is the opt-in host — so it appears only when an installed extension is there
// to report on.
func HostInventory(home, claudeConfigDir string) []HostStatus {
	discovered := sync.DiscoverHosts(home, claudeConfigDir).Hosts
	if home != "" {
		if pi, ok := piHostStatus(home); ok {
			discovered = append(discovered, sync.Host{Name: "pi", ConfigDir: filepath.Join(home, ".pi"), SkillsDir: pi.SkillsDir})
		}
	}
	statuses := make([]HostStatus, 0, len(discovered))
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
	case "pi":
		return TransportExtension, "extension at " + filepath.Join(host.SkillsDir, "tpp", "index.ts") + "; opt-in, installed with: tsp sync --hosts pi"
	}
	if known, ok := hostTransports[host.Name]; ok {
		return known.kind, known.note
	}
	return TransportAbsent, "skills only: no transport is known for this host"
}

// piHostStatus reports Pi only when its extension is installed, which is the only state in which
// this machine has a Pi transport to describe.
func piHostStatus(home string) (HostStatus, bool) {
	extension := filepath.Join(home, ".pi", "agent", "extensions", "tpp", "index.ts")
	info, err := os.Stat(extension)
	if err != nil || info.IsDir() {
		return HostStatus{}, false
	}
	return HostStatus{SkillsDir: filepath.Join(home, ".pi", "agent", "extensions")}, true
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
