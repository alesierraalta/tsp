package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestKeyPythonLanguage(t *testing.T) {
	for _, language := range []string{"python", "node", "go", "ruby"} {
		k := Key{ID: "languages", Language: language, Suite: "pytest", Control: ControlClean}
		err := k.Validate()
		if language == "ruby" {
			if err == nil || !strings.Contains(err.Error(), "node, go, or python") {
				t.Fatalf("unsupported language: %v", err)
			}
		} else if err != nil {
			t.Errorf("%s: %v", language, err)
		}
	}
}

func TestPerTestIncludesPythonAgentFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("real Python runner")
	}
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	t.Setenv("TSP_HOME", t.TempDir())
	dir := t.TempDir()
	for _, f := range []string{"backend/tests/test_a.py", "backend/tests/test_b.py"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte("def test_ok():\n    assert True\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	suites := []string{"python3 -m pytest backend/tests/test_a.py", "python3 -m pytest backend/tests"}
	if _, err := exec.LookPath("pytest"); err == nil {
		suites = append(suites, "pytest backend/tests/test_a.py", "pytest backend/tests")
	}
	for _, scope := range suites {
		outcomes, code, out, err := perTest(dir, scope, 30*time.Second, "backend/tests/test_a.py", "backend/tests/test_b.py", "backend/tests/test_b.py", "missing_test.go")
		if err != nil || code != 0 || len(outcomes) != 2 || !outcomes["backend/tests/test_b.py::test_ok"] || strings.Count(out, "PASSED backend/tests/test_b.py::test_ok") != 1 {
			t.Fatalf("scope %s: outcomes=%v code=%d err=%v output=%s", scope, outcomes, code, err, out)
		}
	}
}

// A pytest suite that names no path collects what its own configuration says (testpaths); adding agent files must
// not replace that with the whole tree, which `tsp findings check` and the red check would then run.
func TestPerTestKeepsPytestDefaultCollectionWithoutPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("real Python runner")
	}
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	t.Setenv("TSP_HOME", t.TempDir())
	dir := t.TempDir()
	files := map[string]string{
		"pyproject.toml":        "[tool.pytest.ini_options]\ntestpaths = [\"tests\"]\n",
		"tests/test_kept.py":    "def test_kept():\n    assert True\n",
		"scratch/test_stray.py": "def test_stray():\n    assert False\n",
	}
	for f, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outcomes, code, out, err := PerTest(dir, "python3 -m pytest", 30*time.Second)
	if err != nil || code != 0 || !reflect.DeepEqual(outcomes, map[string]bool{"tests/test_kept.py::test_kept": true}) {
		t.Fatalf("outcomes=%v code=%d err=%v output=%s", outcomes, code, err, out)
	}
}

// A flag's value is not a path: `-W error` names no test path, so the suite still collects its testpaths, and an
// agent file those testpaths cover must not narrow the run to that one file.
func TestPerTestKeepsPytestCollectionWhenAFlagTakesAValue(t *testing.T) {
	if testing.Short() {
		t.Skip("real Python runner")
	}
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	t.Setenv("TSP_HOME", t.TempDir())
	dir := t.TempDir()
	for f, body := range map[string]string{
		"pyproject.toml":  "[tool.pytest.ini_options]\ntestpaths = [\"tests\"]\n",
		"tests/test_a.py": "def test_a():\n    assert True\n",
		"tests/test_b.py": "def test_b():\n    assert True\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outcomes, code, out, err := perTest(dir, "python3 -m pytest -W error", 30*time.Second, "tests/test_b.py")
	if err != nil || code != 0 || !outcomes["tests/test_a.py::test_a"] || !outcomes["tests/test_b.py::test_b"] {
		t.Fatalf("the suite's own tests must still run: outcomes=%v code=%d err=%v output=%s", outcomes, code, err, out)
	}
}

// A virtual environment is marked by its pyvenv.cfg, whatever its directory is called (`python -m venv env`).
func TestAgentTestFilesSkipsAVirtualEnvironmentUnderAnyName(t *testing.T) {
	fixture, ws := t.TempDir(), t.TempDir()
	for f, body := range map[string]string{
		"env/pyvenv.cfg": "home = /usr/bin\n",
		"env/lib/python3.12/site-packages/pkg/tests/test_vendored.py": "def test_v(): pass\n",
		"app/tests/test_sample.py":                                    "def test_ok(): pass\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(ws, f)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ws, f), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := agentTestFiles(fixture, ws)
	if err != nil || !reflect.DeepEqual(got, []string{"app/tests/test_sample.py"}) {
		t.Fatalf("agent tests = %v, err=%v", got, err)
	}
}

func TestAgentTestFilesIgnoresPythonEnvironments(t *testing.T) {
	fixture, ws := t.TempDir(), t.TempDir()
	for _, dir := range []string{".venv", "venv", "__pycache__", ".pytest_cache", ".git", "node_modules", "vendor", ".atl", ".claude", "docs", "testdata", "app"} {
		f := filepath.Join(ws, dir, "tests", "test_sample.py")
		if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("def test_ok(): pass\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := agentTestFiles(fixture, ws)
	if err != nil || !reflect.DeepEqual(got, []string{"app/tests/test_sample.py"}) {
		t.Fatalf("agent tests = %v, err=%v", got, err)
	}
}
