package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/mode"
)

// writeCodexSource builds a stand-in operator CODEX_HOME holding the login and the operator's own
// instructions and skills, which a bare run must never carry.
func writeCodexSource(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "codex")
	if err := os.MkdirAll(filepath.Join(src, "skills", "operator-skill"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		CodexAuthFile:                    `{"tokens":{"access_token":"operator-token"}}`,
		"AGENTS.md":                      "operator instructions",
		"config.toml":                    `model = "operator-model"`,
		"skills/operator-skill/SKILL.md": "---\nname: operator-skill\n---\n",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func dirNames(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// A bare run carries the login and nothing that could instruct the agent: no skill, no AGENTS.md, no
// user config. Its own empty HOME hides ~/.agents/skills, ~/.codex and ~/.claude from the agent.
func TestNoneConfigHoldsCredentialsAndNoSkills(t *testing.T) {
	pi := filepath.Join(t.TempDir(), "pi")
	if err := WriteNoneConfig(pi, RunnerPi, writePiSource(t)); err != nil {
		t.Fatal(err)
	}
	if got, want := dirNames(t, pi), PiAuthFile+", "+IsolatedHomeDir+", "+PiModelsStoreFile+", settings.json"; got != want {
		t.Fatalf("pi bare config holds %q, want %q", got, want)
	}
	data, err := os.ReadFile(filepath.Join(pi, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if skills, ok := settings["skills"].([]any); !ok || len(skills) != 0 {
		t.Fatalf("a bare pi config must configure no skills: %v", settings["skills"])
	}
	if packages, ok := settings["packages"].([]any); !ok || len(packages) != 0 {
		t.Fatalf("a bare pi config must load no packages: %v", settings["packages"])
	}
	if got := dirNames(t, filepath.Join(pi, IsolatedHomeDir)); got != "" {
		t.Fatalf("the isolated home must start empty, holds %q", got)
	}

	codex := filepath.Join(t.TempDir(), "codex")
	if err := WriteNoneConfig(codex, RunnerCodex, writeCodexSource(t)); err != nil {
		t.Fatal(err)
	}
	if got, want := dirNames(t, codex), CodexAuthFile+", "+IsolatedHomeDir; got != want {
		t.Fatalf("codex bare config holds %q, want %q", got, want)
	}
	if err := WriteNoneConfig(filepath.Join(t.TempDir(), "x"), RunnerClaude, t.TempDir()); err == nil {
		t.Fatal("the claude runner has no bare mode; asking for one must be refused")
	}
}

// The tsp arm gets the same isolation: Pi discovers ~/.agents/skills whatever its settings say, so the
// embedded skills are only the skills a run sees when HOME is the config's own.
func TestPiBenchConfigCarriesItsOwnHome(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "pi")
	if err := WriteBenchPiConfig(cfg, writePiSource(t)); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(cfg, IsolatedHomeDir)); err != nil || !st.IsDir() {
		t.Fatalf("the pi bench config must carry an isolated home: %v", err)
	}
}

func envValues(env []string, key string) []string {
	var got []string
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			got = append(got, strings.TrimPrefix(e, key+"="))
		}
	}
	return got
}

// The agent's HOME is the config's own home when the config carries one; a custom config dir without
// one keeps the operator's HOME. The build cache stays the operator's: it holds compiled packages,
// never instructions, and a cold cache would only cost every Go run its standard library build.
func TestIsolatedEnvPointsHomeAtTheConfig(t *testing.T) {
	cfg := t.TempDir()
	home := filepath.Join(cfg, IsolatedHomeDir)
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	base := []string{"PATH=/bin", "HOME=/operator", "CODEX_HOME=/operator/.codex",
		"XDG_CONFIG_HOME=/operator/.config", "XDG_DATA_HOME=/operator/.local/share",
		"XDG_STATE_HOME=/operator/.local/state", "XDG_CACHE_HOME=/operator/.cache"}

	pi := piEnv(base, cfg, "/build")
	if got := envValues(pi, "HOME"); !reflect.DeepEqual(got, []string{home}) {
		t.Fatalf("pi HOME = %v, want [%s]", got, home)
	}
	// The XDG home-scoped directories are operator config too: left in place they point at the
	// operator's own config, data, state and cache whatever HOME says.
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		if got := envValues(pi, key); len(got) != 0 {
			t.Fatalf("pi %s = %v, want the variable dropped", key, got)
		}
	}
	if got := envValues(pi, "GOCACHE"); !reflect.DeepEqual(got, []string{filepath.Join("/operator", ".cache", "go-build")}) {
		t.Fatalf("pi GOCACHE = %v", got)
	}

	codex := codexEnv(base, cfg, "/build")
	if got := envValues(codex, "CODEX_HOME"); !reflect.DeepEqual(got, []string{cfg}) {
		t.Fatalf("CODEX_HOME = %v, want [%s]", got, cfg)
	}
	if got := envValues(codex, "HOME"); !reflect.DeepEqual(got, []string{home}) {
		t.Fatalf("codex HOME = %v, want [%s]", got, home)
	}
	if got := envValues(codex, "PATH"); len(got) != 1 || !strings.HasPrefix(got[0], "/build") {
		t.Fatalf("codex PATH = %v, want the build under test first", got)
	}

	custom := t.TempDir()
	if got := envValues(piEnv(base, custom, ""), "HOME"); !reflect.DeepEqual(got, []string{"/operator"}) {
		t.Fatalf("a config without its own home keeps HOME, got %v", got)
	}
	if got := envValues(piEnv(base, custom, ""), "XDG_CONFIG_HOME"); !reflect.DeepEqual(got, []string{"/operator/.config"}) {
		t.Fatalf("a config without its own home keeps the operator's XDG vars, got %v", got)
	}
}

// Every isolated pi run ignores AGENTS.md/CLAUDE.md discovery; a bare run also disables every skill.
func TestPiArgsIsolateContextFilesAndSkills(t *testing.T) {
	key := Key{ID: "c1"}
	has := func(args []string, flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
		}
		return false
	}
	bench := piArgs(key, Options{ConfigDir: "/cfg", ConfigMode: ConfigBench})
	if !has(bench, "--no-context-files") || !has(bench, "--no-extensions") || has(bench, "--no-skills") {
		t.Fatalf("bench args = %v", bench)
	}
	bare := piArgs(key, Options{ConfigDir: "/cfg", ConfigMode: ConfigNone})
	if !has(bare, "--no-context-files") || !has(bare, "--no-extensions") || !has(bare, "--no-skills") {
		t.Fatalf("bare args = %v", bare)
	}
	inherited := piArgs(key, Options{})
	if has(inherited, "--no-context-files") || has(inherited, "--no-extensions") || has(inherited, "--no-skills") {
		t.Fatalf("an inherited run keeps the operator's pi as it is: %v", inherited)
	}
}

func TestPiArgsLoadTheProductExtension(t *testing.T) {
	key := Key{ID: "c1"}
	argValue := func(args []string, flag string) (string, bool) {
		for i, a := range args {
			if a == flag && i+1 < len(args) {
				return args[i+1], true
			}
		}
		return "", false
	}
	has := func(args []string, flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
		}
		return false
	}
	product := piArgs(key, Options{ConfigDir: "/cfg", ConfigMode: ConfigProduct})
	// One extension, named explicitly: discovery stays off, so nothing of the operator's can ride along.
	loaded, ok := argValue(product, "-e")
	if !ok || loaded != PiExtensionFile("/cfg") {
		t.Fatalf("product args = %v, want -e %s", product, PiExtensionFile("/cfg"))
	}
	if !has(product, "--no-extensions") || !has(product, "--no-context-files") {
		t.Fatalf("product args = %v, want discovery still off", product)
	}
	if has(product, "--no-skills") {
		t.Fatalf("product args = %v, the skill under test has to load", product)
	}
	// The other modes load no extension at all.
	for _, opts := range []Options{{ConfigDir: "/cfg", ConfigMode: ConfigBench}, {ConfigDir: "/cfg", ConfigMode: ConfigNone}, {ConfigDir: "/cfg", ConfigMode: ConfigBenchAll}} {
		if args := piArgs(key, opts); has(args, "-e") || has(args, "--extension") {
			t.Fatalf("%s args = %v, want no extension", opts.ConfigMode, args)
		}
	}
}

// A print-mode session has no UI, so nobody can answer the tsp extension's offer. The product arm answers it the
// deterministic way an accepting user does — the gate the extension's non-interactive path acts on — and it drops
// the Gentle variables a parent session exports, because GENTLE_PI_AGENTS_CHILD=1 would silently disable it.
func TestPiProductEnvAnswersTheOfferAndDropsTheParentSession(t *testing.T) {
	cfg := t.TempDir()
	if err := os.Mkdir(filepath.Join(cfg, IsolatedHomeDir), 0o700); err != nil {
		t.Fatal(err)
	}
	base := []string{"PATH=/bin", "HOME=/operator", GentleChildEnv + "=1", "TSP_GENTLE_OBSERVATION=whatever", LegacyGentleObservationEnv + "=pi-session-gentle-active"}
	product := piProductEnv(base, cfg, "/build")
	if got := envValues(product, "TSP_GENTLE_OBSERVATION"); !reflect.DeepEqual(got, []string{mode.GentleObservationValue}) {
		t.Fatalf("TSP_GENTLE_OBSERVATION = %v, want exactly the arm's own observation", got)
	}
	for _, key := range []string{GentleChildEnv, LegacyGentleObservationEnv} {
		if got := envValues(product, key); len(got) != 0 {
			t.Fatalf("%s = %v, want the parent session's dropped", key, got)
		}
	}
	if got := envValues(product, "HOME"); !reflect.DeepEqual(got, []string{filepath.Join(cfg, IsolatedHomeDir)}) {
		t.Fatalf("HOME = %v, want the config's own home", got)
	}
	// The extension's own `tsp` children resolve their state root from these (internal/state/state.go Root),
	// so an inherited one would make a product run read and write the operator's findings registry (#471).
	withState := append(append([]string{}, base...), "TSP_HOME=/operator/tsp", "TPP_HOME=/operator/tpp", "RDD_PLUS_HOME=/operator/rdd")
	product = piProductEnv(withState, cfg, "/build")
	if got := envValues(product, "TSP_HOME"); !reflect.DeepEqual(got, []string{filepath.Join(cfg, IsolatedHomeDir, ".config", "tsp")}) {
		t.Fatalf("TSP_HOME = %v, want the run's own state root", got)
	}
	for _, key := range []string{"TPP_HOME", "RDD_PLUS_HOME"} {
		if got := envValues(product, key); len(got) != 0 {
			t.Fatalf("%s = %v, want the operator's dropped", key, got)
		}
	}
	// No config dir means no throwaway home to pin: whatever the process inherited stays as it was.
	if got := envValues(piProductEnv(withState, "", ""), "TSP_HOME"); !reflect.DeepEqual(got, []string{"/operator/tsp"}) {
		t.Fatalf("TSP_HOME = %v without a config dir, want the inherited root untouched", got)
	}
	// A bench or bare run declares nothing: the extension does not run there, and an inherited observation would
	// change what the agent's own `tsp check` answers about the case.
	for _, mode := range []string{ConfigBench, ConfigBenchAll, ConfigNone} {
		env := piEnv(base, cfg, "/build")
		if got := envValues(env, "TSP_GENTLE_OBSERVATION"); len(got) != 0 {
			t.Fatalf("%s: TSP_GENTLE_OBSERVATION = %v, want none", mode, got)
		}
		if got := envValues(env, GentleChildEnv); len(got) != 0 {
			t.Fatalf("%s: %s = %v, want the parent session's dropped", mode, GentleChildEnv, got)
		}
	}
}

func TestCodexIsAKnownRunnerWithItsOwnAgent(t *testing.T) {
	if !KnownRunner(RunnerCodex) {
		t.Fatal("codex must be a runner the bench can spawn")
	}
	if got := reflect.ValueOf(runnerAgent(RunnerCodex)).Pointer(); got != reflect.ValueOf(codexAgent).Pointer() {
		t.Fatal("RunnerCodex must select codexAgent")
	}
	if DefaultModel(RunnerCodex) != "gpt-6-luna" {
		t.Fatalf("codex default model = %q", DefaultModel(RunnerCodex))
	}
}

// A bare codex run reads no user config, rules, or session history, and works only in the workspace.
func TestCodexArgsRunBareInTheWorkspace(t *testing.T) {
	key := Key{ID: "c1"}
	args := codexArgs(key, Options{Model: "gpt-6-luna"}, "/ws")
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config",
		"--ignore-rules", "--dangerously-bypass-approvals-and-sandbox", "-C /ws", "-m gpt-6-luna"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("codex args %q lack %q", joined, want)
		}
	}
	if args[len(args)-1] != agentPrompt(key, Options{}) {
		t.Fatalf("the prompt must be the last argument: %q", args[len(args)-1])
	}

	// The bench hands workspaces over as paths relative to its own directory, and codex runs inside the
	// workspace: a relative -C would resolve a second time from there and name nothing.
	rel := codexArgs(key, Options{}, filepath.Join("bench", "results", "run", "ws"))
	for i, a := range rel {
		if a == "-C" {
			if dir := rel[i+1]; !filepath.IsAbs(dir) || !strings.HasSuffix(dir, filepath.Join("bench", "results", "run", "ws")) {
				t.Fatalf("-C must name the workspace absolutely, got %q", dir)
			}
		}
	}
}

func TestParseCodexStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"i1","type":"reasoning","text":"think"}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"command_execution","command":"node --test"}}`,
		`{"type":"item.completed","item":{"id":"i3","type":"agent_message","text":"first"}}`,
		`{"type":"item.completed","item":{"id":"i4","type":"file_change"}}`,
		`not json`,
		`{"type":"item.completed","item":{"id":"i5","type":"agent_message","text":"final answer"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`,
	}, "\n")
	got := ParseCodexStream(strings.NewReader(stream))
	if got.SessionID != "thread-1" || got.Result != "final answer" || got.Turns != 2 || got.IsError {
		t.Fatalf("parsed = %+v, want session thread-1, last agent message, two tool items, no error", got)
	}

	failed := ParseCodexStream(strings.NewReader(`{"type":"thread.started","thread_id":"t"}` + "\n" +
		`{"type":"turn.failed","error":{"message":"rate limited"}}`))
	if !failed.IsError || failed.ErrorText != "rate limited" {
		t.Fatalf("a failed turn must be an error with its message: %+v", failed)
	}
	errored := ParseCodexStream(strings.NewReader(`{"type":"error","message":"boom"}`))
	if !errored.IsError || errored.ErrorText != "boom" {
		t.Fatalf("an error event must be an error with its message: %+v", errored)
	}
	// A stream that stops before a turn ends is not a run that finished: without the terminal
	// event a clean exit and a truncated stream are the same zero-value result, which scored as a
	// valid run that found nothing.
	for name, stream := range map[string]string{
		"empty":          "",
		"header only":    `{"type":"thread.started","thread_id":"t"}`,
		"items, no turn": `{"type":"item.completed","item":{"type":"command_execution"}}`,
	} {
		got := ParseCodexStream(strings.NewReader(stream))
		if !got.IsError || got.ErrorText == "" {
			t.Fatalf("%s: a stream without a terminal event must be an error, got %+v", name, got)
		}
	}
}

// A prepared config is a template: each run works on its own copy, so two runs side by side cannot
// see or overwrite each other's login, caches, or home, and no run writes through the template.
func TestEachRunWorksOnItsOwnConfigCopy(t *testing.T) {
	requireNodeAndGit(t)
	caseDir := fakeCaseNamed(t, t.TempDir(), "case-a")
	template := t.TempDir()
	if err := os.WriteFile(filepath.Join(template, PiAuthFile), []byte(`{"token":"t"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	dirs := map[string]bool{}
	agent := func(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
		mu.Lock()
		dirs[opts.ConfigDir] = true
		mu.Unlock()
		if opts.ConfigDir == template {
			t.Errorf("the agent was handed the shared template %s", template)
		}
		// The agent runs inside its workspace: a relative config path would resolve from there and
		// point at nothing, failing every run with "path does not exist" or "no API key".
		if !filepath.IsAbs(opts.ConfigDir) {
			t.Errorf("the run's config path %q is relative to the bench's cwd, not the agent's", opts.ConfigDir)
		}
		if _, err := os.Stat(filepath.Join(opts.ConfigDir, PiAuthFile)); err != nil {
			t.Errorf("the run's copy must carry the prepared login: %v", err)
		}
		// A write into the run's config is how a login refresh behaves: the template must not see it.
		if err := os.WriteFile(filepath.Join(opts.ConfigDir, "run-write"), []byte(ws), 0o600); err != nil {
			t.Errorf("writing the run's own config: %v", err)
		}
		return AgentResult{Result: "nothing found", Turns: 2}, nil
	}
	// --out is usually given relative to the repository, as in `--out bench/results/<run>`.
	t.Chdir(t.TempDir())
	agg, code := Run(Options{CasesGlob: caseDir, Runs: 2, Workers: 2, Timeout: time.Minute, SuiteTimeout: time.Minute, Out: filepath.Join("bench", "results", "run"), BenchDir: t.TempDir(), ConfigDir: template, Agent: agent})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if len(agg.Cases) != 2 {
		t.Fatalf("%d cases in the aggregate, want 2", len(agg.Cases))
	}
	if len(dirs) != 2 {
		t.Fatalf("two runs used %d config dirs, want one copy each: %v", len(dirs), dirs)
	}
	if _, err := os.Stat(filepath.Join(template, "run-write")); err == nil {
		t.Fatal("a run wrote into the template instead of its own copy")
	}
}
