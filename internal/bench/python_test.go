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

func TestInferredPytestUsesVenv(t *testing.T) {
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	dir := redCheckRepo(t)
	venv := filepath.Join(dir, "backend", ".venv")
	if out, err := exec.Command("python3", "-m", "venv", "--system-site-packages", venv).CombinedOutput(); err != nil {
		t.Fatalf("venv: %v: %s", err, out)
	}
	python := filepath.Join(venv, "bin", "python")
	if out, err := exec.Command(python, "-c", "import sysconfig,pathlib; pathlib.Path(sysconfig.get_path('purelib'), 'tsp_venv_only.py').write_text('value = 42\\n')").CombinedOutput(); err != nil {
		t.Fatalf("package: %v: %s", err, out)
	}
	writeFile(t, dir, "backend/test_venv.py", "import tsp_venv_only\ndef test_venv():\n    assert tsp_venv_only.value == 0\n")
	rep, err := RedCheck(dir, "", 30*time.Second)
	if err != nil || !strings.Contains(rep.Suite, python) || !reflect.DeepEqual(rep.Red, []string{"backend/test_venv.py::test_venv"}) {
		t.Fatalf("venv inference: report=%+v err=%v", rep, err)
	}
}

func TestPytestJudgesWhatCollects(t *testing.T) {
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "test_good.py", "def test_red():\n    assert False\n")
	writeFile(t, dir, "test_bad.py", "import tsp_missing_dependency_123456\n")
	rep, err := RedCheck(dir, "", 30*time.Second)
	if err != nil || !reflect.DeepEqual(rep.Red, []string{"test_good.py::test_red"}) {
		t.Fatalf("partial collection: report=%+v err=%v", rep, err)
	}
	want := []Uncollected{{File: "test_bad.py", Reason: "ModuleNotFoundError: No module named 'tsp_missing_dependency_123456'"}}
	if !reflect.DeepEqual(rep.Uncollected, want) {
		t.Fatalf("uncollected=%+v", rep.Uncollected)
	}
	// Changed tests are added to explicitly scoped suites, so use a fresh repository
	// containing only the uncollectable file for the empty-collection diagnostic.
	empty := redCheckRepo(t)
	writeFile(t, empty, "test_bad.py", "import tsp_missing_dependency_123456\n")
	_, err = RedCheck(empty, "", 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "could not collect: test_bad.py (ModuleNotFoundError:") || !strings.Contains(err.Error(), `--suite "<venv>/bin/python -m pytest ..."`) {
		t.Fatalf("empty collection: %v", err)
	}
}

func TestPytestEnvironmentChoice(t *testing.T) {
	dir := t.TempDir()
	for _, env := range []string{"aaa", "backend/a-env", "backend/z-env", "backend/deep/env", "node_modules/env", ".git/env", "too/deep/to/find/env"} {
		writeFile(t, dir, env+"/pyvenv.cfg", "home = python\n")
		writeFile(t, dir, env+"/bin/python", "")
	}
	got, err := inferRedCheckSuite(dir, []string{"backend/test_sample.py"})
	if err != nil || !strings.Contains(got, filepath.Join(dir, "backend/a-env/bin/python")) {
		t.Fatalf("nearest then lexical: %q %v", got, err)
	}
	fallback, err := inferRedCheckSuite(t.TempDir(), []string{"test_sample.py"})
	if err != nil || fallback != "python3 -m pytest test_sample.py" {
		t.Fatalf("fallback: %q %v", fallback, err)
	}
}

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
