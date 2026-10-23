package assets

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/buildinfo"
)

var expected = []string{
	"agent-eval", "appsec-adversarial-auditor", "breakcheck", "clean-architecture-audit", "contract-compat-testing", "crash-and-process-testing", "database-persistence-testing",
	"dependency-legitimacy", "docker-test-containers", "exploit-testing", "go-testing", "iac-safe-auditor", "implementation-theater",
	"llm-eval-design", "llm-redteam", "messaging-eventdriven-testing", "no-excess-tests", "python-testing-patterns", "rag-audit-evaluator", "real-run-validation", "resilience-fault-injection", "runtime-reliability-testing", "silent-degradation",
	"test-strategy", "test-strategy-verifier",
}

func TestEverySkillIsEmbeddedWithAMatchingName(t *testing.T) {
	names := SkillNames()
	if len(names) != len(expected) {
		t.Fatalf("embedded %d skills, want %d: %v", len(names), len(expected), names)
	}
	nameLine := regexp.MustCompile(`(?m)^name:\s*"?([a-z0-9-]+)"?\s*$`)
	for i, name := range expected {
		t.Run(name, func(t *testing.T) {
			if names[i] != name {
				t.Fatalf("position %d is %q, want %q", i, names[i], name)
			}
			data, err := fs.ReadFile(Skills(), name+"/SKILL.md")
			if err != nil {
				t.Fatalf("SKILL.md missing: %v", err)
			}
			m := nameLine.FindSubmatch(data)
			if m == nil || string(m[1]) != name {
				t.Fatalf("frontmatter name does not match directory: %q", m)
			}
		})
	}
}

func TestTestStrategyDescriptionPreservesDiscoveryTriggers(t *testing.T) {
	data, err := fs.ReadFile(Skills(), "test-strategy/SKILL.md")
	if err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
	description := regexp.MustCompile(`(?m)^description:\s*"?([^"\n]+?)"?\s*$`).FindSubmatch(data)
	if description == nil {
		t.Fatal("test-strategy frontmatter has no single-line description")
	}
	const want = "Trigger: haz el TSP, haz TSP, TSP, haz test stragety, haz test strategy, test-strategy, haz el testing, testea esto, prueba esto, test this, test the app, test my change, run the testing, calibra el testing, calibrate the testing, test strategy, what to test, where to start testing, test planning, testing priorities, test altitude, low coverage, legacy code testing, test plan for the app, testing roadmap, plan de testing, roadmap de pruebas. Find the defects a green suite hides and pin each one with a failing test."
	if got := string(description[1]); got != want {
		t.Fatalf("test-strategy description = %q, want %q", got, want)
	}
}

// The legacy bridge requirement remains pinned independently of the TSP build version.
func TestSkillNamesTheTppVersionItRequires(t *testing.T) {
	data, err := fs.ReadFile(Skills(), "test-strategy/SKILL.md")
	if err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
	field := regexp.MustCompile(`(?m)^\s*requires_tpp:\s*"?([0-9]+\.[0-9]+\.[0-9]+)"?\s*$`).FindSubmatch(data)
	if field == nil {
		t.Fatal("test-strategy frontmatter has no requires_tpp")
	}
	if got := string(field[1]); got != "0.5.1" {
		t.Fatalf("test-strategy requires tpp %s, want legacy bridge version 0.5.1", got)
	}
	// The lean skill never runs tsp itself, so it names no install path: the requirement lives in the
	// frontmatter, where doctor and sync read it.
}

// The canonical requirement is the TSP release this skill ships with, installed from the new module.
func TestSkillRequiresTheTspReleaseItShipsWith(t *testing.T) {
	data, err := fs.ReadFile(Skills(), "test-strategy/SKILL.md")
	if err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
	field := regexp.MustCompile(`(?m)^\s*requires_tsp:\s*"?([0-9]+\.[0-9]+\.[0-9]+)"?\s*$`).FindSubmatch(data)
	if field == nil {
		t.Fatal("test-strategy frontmatter has no requires_tsp")
	}
	if got := string(field[1]); got != "0.6.9" || got != buildinfo.Version {
		t.Fatalf("test-strategy requires tsp %s, want the shipped build %s and release 0.6.9", got, buildinfo.Version)
	}
	if got := string(requiredVersion(data)[1]); got != "0.6.9" {
		t.Fatalf("required version = %s, want requires_tsp to win over legacy keys", got)
	}
}

// requiredVersion reads the binary version a skill's frontmatter requires: requires_tpp, or requires_tpp
// in a skill written before the rename.
func requiredVersion(skill []byte) [][]byte {
	for _, key := range []string{"requires_tsp", "requires_tpp", "requires_rdd_plus"} {
		if m := regexp.MustCompile(`(?m)^\s*` + key + `:\s*"?([0-9]+\.[0-9]+\.[0-9]+)"?\s*$`).FindSubmatch(skill); m != nil {
			return m
		}
	}
	return nil
}

func TestNoRunArtifactsAreEmbedded(t *testing.T) {
	banned := regexp.MustCompile(`(^|/)(__pycache__|\.runs|results)(/|$)`)
	err := fs.WalkDir(Skills(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if banned.MatchString(p) {
			t.Errorf("run artifact embedded: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The test gap in the gpt-6-luna readings was runs that reported defects and wrote no test file at all. The
// lean skill has no plan to close on: its deliverable is the test itself, one per defect, red on today's code
// and green once fixed, in the repository's own test files. This pins that deliverable.
func TestTestStrategySkillRequiresATestFileForEveryReportedFinding(t *testing.T) {
	data, err := fs.ReadFile(Skills(), "test-strategy/SKILL.md")
	if err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
	text := string(data)
	for _, want := range []struct{ phrase, contract string }{
		{"leave one test per defect in the", "the deliverable: a test in the suite for every defect"},
		{"RED on today's code", "the test has to fail on the defective code"},
		{"GREEN once the defect is fixed", "and pass once it is fixed, so it pins the defect and not today's output"},
		{"Pin every defect** in the repository's own test files", "where the tests go: the suite, not a scratch file"},
	} {
		if !strings.Contains(text, want.phrase) {
			t.Errorf("test-strategy no longer names %q: %s", want.phrase, want.contract)
		}
	}
	if strings.Contains(text, "the tree is the arbiter") {
		t.Error("test-strategy still carries the pinning-claim gate written for a gap the measurement did not show")
	}
}

// test-strategy routes a surface it cannot judge alone to the sibling that owns it: a repository with a RAG
// pipeline, an LLM, infrastructure or a database gets that sibling's probes. Every sibling it names is embedded,
// so a route never points at a skill sync does not install.
const routeStep = "**Route special surfaces.**"

func TestTestStrategyRoutesSpecialSurfacesToEmbeddedSiblings(t *testing.T) {
	data, err := fs.ReadFile(Skills(), "test-strategy/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	owners := []string{"rag-audit-evaluator", "llm-eval-design", "llm-redteam", "agent-eval", "iac-safe-auditor",
		"database-persistence-testing", "messaging-eventdriven-testing", "contract-compat-testing",
		"crash-and-process-testing", "resilience-fault-injection", "appsec-adversarial-auditor"}
	for _, o := range owners {
		if !strings.Contains(body, "`"+o+"`") {
			t.Errorf("test-strategy does not route to `%s`", o)
		}
	}
	embedded := map[string]bool{}
	for _, n := range SkillNames() {
		embedded[n] = true
	}
	step, _, ok := strings.Cut(body[strings.Index(body, routeStep)+1:], "\n\n")
	if !strings.Contains(body, routeStep) || !ok {
		t.Fatalf("test-strategy has no %q step", routeStep)
	}
	for _, m := range regexp.MustCompile("`([a-z]+(?:-[a-z]+)+)`").FindAllStringSubmatch(step, -1) {
		if !embedded[m[1]] {
			t.Errorf("the route step names `%s`, which is not an embedded skill", m[1])
		}
	}
}
