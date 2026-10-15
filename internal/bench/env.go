package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesierraalta/tsp/internal/assets"
)

// CredentialsFile is the only piece of the operator's configuration a benchmark run needs: the
// agent cannot log in without it.
const CredentialsFile = ".credentials.json"

// PiAuthFile and PiModelsStoreFile are the Pi equivalent: without them an agent dir built from
// scratch starts with no provider credentials at all, and every case fails before the model runs.
const (
	PiAuthFile        = "auth.json"
	PiModelsStoreFile = "models-store.json"
)

// WriteBenchConfig builds a Claude configuration directory holding the skill under test and
// nothing else. A benchmark case must measure the testing skills, not the operator's global
// instructions, memory protocol, or MCP servers, which cost turns and vary between machines.
func WriteBenchConfig(dir, credentialsFrom string) error {
	if err := writeEmbeddedSkills(dir); err != nil {
		return err
	}
	settings, err := json.MarshalIndent(map[string]any{"includeCoAuthoredBy": false}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), append(settings, '\n'), 0o644); err != nil {
		return err
	}
	// Copied, never linked: a refresh writes into this throwaway copy, so a failed refresh cannot
	// empty the operator's login the way it did when the two paths were the same file. F25.
	src := filepath.Join(credentialsFrom, CredentialsFile)
	if err := copyFile(src, filepath.Join(dir, CredentialsFile)); err != nil {
		return err
	}
	return VerifyBenchSkills(dir)
}

// WriteBenchPiConfig builds the Pi equivalent of WriteBenchConfig in the same directory: the
// skill under test and a settings.json that loads nothing else. Pi derives its package root, and
// therefore the operator's extensions, memory protocol, and MCP servers, from the agent dir the
// run owns, and PI_CODING_AGENT_DIR is what points it there.
func WriteBenchPiConfig(dir, configFrom string) error {
	if err := writeEmbeddedSkills(dir); err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	// Absolute: the path must resolve to this directory whatever base the agent dir implies.
	if err := writePiConfig(dir, configFrom, []string{filepath.Join(abs, "skills")}); err != nil {
		return err
	}
	return VerifyBenchSkills(dir)
}

// writePiConfig writes the settings an isolated Pi run loads, copies the operator's login, and gives the
// run its own empty home: Pi discovers ~/.agents/skills whatever its settings say, so the operator's own
// skills are only out of the measurement when HOME is not the operator's.
func writePiConfig(dir, configFrom string, skills []string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	settings, err := json.MarshalIndent(map[string]any{
		"skills": skills,
		// Empty: the operator's packages bring their own skills, extensions, and MCP servers, which
		// would cost turns and differ between machines.
		"packages": []string{},
		// Pinned to its current default. A batch run never types /skill:name, but pinning the value
		// means a future default flip cannot quietly change what a case loads.
		"enableSkillCommands": true,
		// Non-interactive runs never see the trust prompt and fall back to this setting; asking is
		// impossible in a batch run, and trusting would load whatever the fixture happens to carry.
		"defaultProjectTrust": "never",
		// No version ping or install telemetry: the run must not depend on the network being up.
		"enableInstallTelemetry": false,
		// The bench owns the retry policy (--retries, --retry-delay); an agent-level retry would add
		// turns and cost that no case asked for.
		"retry": map[string]any{"enabled": false},
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), append(settings, '\n'), 0o644); err != nil {
		return err
	}
	// Copied, never linked. A symlink makes the operator's token the working file: a refresh that
	// fails writes the empty token state straight through the link. That is finding F25 in
	// docs/testing/test-plan.md, where a failed refresh logged the operator out of Claude Code
	// everywhere. A copy means a failed refresh can only empty the copy this run throws away.
	for _, name := range []string{PiAuthFile, PiModelsStoreFile} {
		if err := copyFile(filepath.Join(configFrom, name), filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return writeIsolatedHome(dir)
}

// WriteNoneConfig builds the bare-agent configuration for a runner: the operator's login and an empty
// home, and nothing that could instruct the agent — no skill, no AGENTS.md, no user config or rules. It
// is the baseline a skill is measured against.
func WriteNoneConfig(dir, runner, configFrom string) error {
	switch runner {
	case RunnerPi:
		return writePiConfig(dir, configFrom, []string{})
	case RunnerCodex:
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(configFrom, CodexAuthFile), filepath.Join(dir, CodexAuthFile)); err != nil {
			return err
		}
		return writeIsolatedHome(dir)
	default:
		return fmt.Errorf("runner %q has no bare configuration; use %s or %s", runner, RunnerPi, RunnerCodex)
	}
}

// writeIsolatedHome replaces dir's home with an empty one, so a rerun into the same config starts clean.
func writeIsolatedHome(dir string) error {
	home := filepath.Join(dir, IsolatedHomeDir)
	if err := os.RemoveAll(home); err != nil {
		return err
	}
	return os.MkdirAll(home, 0o700)
}

// copyFile copies src to dst with owner-only permissions and does nothing when src is absent: a
// source without credentials is not a failure to build the config, it is a run that cannot log in.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	_ = os.Remove(dst)
	return os.WriteFile(dst, data, 0o600)
}

// BenchSkill is the skill a reading measures. A throwaway config carries it alone: the other embedded skills
// would ride every turn as catalog entries and change what the measurement costs.
const BenchSkill = "test-strategy"

// writeEmbeddedSkills materializes the embedded skill under test (BenchSkill) under dir/skills. Both runners write the
// same bytes, so a case measures the same skill content whichever agent runs it.
func writeEmbeddedSkills(dir string) error {
	skills := filepath.Join(dir, "skills")
	if err := os.RemoveAll(skills); err != nil {
		return err
	}
	return fs.WalkDir(assets.Skills(), BenchSkill, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(skills, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(assets.Skills(), p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// ClaudeConfigEnv and PiConfigEnv are the variables each runner reads to find its config
// directory: the Claude CLI config dir, and the Pi agent dir.
const (
	ClaudeConfigEnv = "CLAUDE_CONFIG_DIR"
	PiConfigEnv     = "PI_CODING_AGENT_DIR"
	CodexConfigEnv  = "CODEX_HOME"
)

// CodexAuthFile is the login a bare codex run copies; IsolatedHomeDir is the empty HOME an isolated
// config gives its agent; agentConfigDir is the per-run copy a run keeps beside its workspace.
const (
	CodexAuthFile   = "auth.json"
	IsolatedHomeDir = "home"
	agentConfigDir  = "agent-config"
)

// agentEnv returns the environment for the agent process: CLAUDE_CONFIG_DIR set to cfgDir,
// replacing any inherited value, and binDir first on PATH so the flow finds the binary under
// test. Empty cfgDir and binDir mean "inherit unchanged".
func agentEnv(base []string, cfgDir, binDir string) []string {
	return agentEnvWith(base, ClaudeConfigEnv, cfgDir, binDir)
}

// piEnv is the same contract for Pi: PI_CODING_AGENT_DIR is the agent directory it loads settings,
// credentials, and packages from (docs/environment-variables.md), so a throwaway config dir is
// what keeps the operator's own configuration out of the measurement.
func piEnv(base []string, cfgDir, binDir string) []string {
	return isolateHome(agentEnvWith(base, PiConfigEnv, cfgDir, binDir), cfgDir)
}

// codexEnv is the same contract for Codex: CODEX_HOME is where it reads its login, config, AGENTS.md
// and skills, so a throwaway CODEX_HOME holding only the login is what makes a codex run bare.
func codexEnv(base []string, cfgDir, binDir string) []string {
	return isolateHome(agentEnvWith(base, CodexConfigEnv, cfgDir, binDir), cfgDir)
}

// isolateHome points HOME at cfgDir's own home when the config carries one, so nothing the operator keeps
// under HOME (~/.agents/skills, ~/.codex, ~/.claude, AGENTS.md) reaches the agent. The Go build cache stays
// the operator's: it holds compiled packages, never instructions.
func isolateHome(env []string, cfgDir string) []string {
	if env == nil || cfgDir == "" {
		return env
	}
	home := filepath.Join(cfgDir, IsolatedHomeDir)
	if st, err := os.Stat(home); err != nil || !st.IsDir() {
		return env
	}
	out := make([]string, 0, len(env)+2)
	operatorHome, goCache := "", false
	for _, e := range env {
		switch {
		case strings.HasPrefix(e, "HOME="):
			operatorHome = strings.TrimPrefix(e, "HOME=")
			continue
		// XDG's home-scoped directories are operator configuration too: left alone they point at the
		// operator's own config, data, state, and cache whatever HOME says.
		case strings.HasPrefix(e, "XDG_CONFIG_HOME="), strings.HasPrefix(e, "XDG_DATA_HOME="),
			strings.HasPrefix(e, "XDG_STATE_HOME="), strings.HasPrefix(e, "XDG_CACHE_HOME="):
			continue
		case strings.HasPrefix(e, "GOCACHE="):
			goCache = true
		}
		out = append(out, e)
	}
	out = append(out, "HOME="+home)
	if operatorHome != "" && !goCache {
		out = append(out, "GOCACHE="+filepath.Join(operatorHome, ".cache", "go-build"))
	}
	return out
}

func agentEnvWith(base []string, configKey, cfgDir, binDir string) []string {
	if cfgDir == "" && binDir == "" {
		return nil
	}
	out := make([]string, 0, len(base)+1)
	prefix := configKey + "="
	for _, e := range base {
		switch {
		case strings.HasPrefix(e, prefix):
			continue
		case binDir != "" && strings.HasPrefix(e, "PATH="):
			out = append(out, "PATH="+binDir+string(os.PathListSeparator)+strings.TrimPrefix(e, "PATH="))
		default:
			out = append(out, e)
		}
	}
	if cfgDir != "" {
		out = append(out, prefix+cfgDir)
	}
	return out
}

// DefaultCodexConfigDir is where Codex keeps its login when CODEX_HOME is unset.
func DefaultCodexConfigDir() string {
	if dir := os.Getenv(CodexConfigEnv); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// DefaultPiConfigDir is where Pi keeps its agent directory when PI_CODING_AGENT_DIR is unset; the
// benchmark reads the operator's credentials from there to copy them into a throwaway config.
func DefaultPiConfigDir() string {
	if dir := os.Getenv(PiConfigEnv); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent")
}
