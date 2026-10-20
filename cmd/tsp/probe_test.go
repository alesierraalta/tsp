package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func probeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	d := t.TempDir()
	for p, s := range files {
		p = filepath.Join(d, p)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}
func probeRun(t *testing.T, bin string, args ...string) (int, string, string) {
	t.Helper()
	c := exec.Command(bin, append([]string{"probe", "env"}, args...)...)
	var out, stderr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &stderr
	err := c.Run()
	code := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return code, out.String(), stderr.String()
}
func TestProbeRecognizesReads(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{"a.py": "os.getenv(\"APP_A\")\nos.environ[\"APP_B\"]\nos.environ.get(\"APP_C\")", "b.js": "process.env.APP_D\nprocess.env[\"APP_E\"]", "c.go": "os.Getenv(\"APP_F\")", "deploy/env": "APP_OLD"})
	code, out, _ := probeRun(t, bin, d)
	if code != 1 {
		t.Fatalf("reads: exit %d, want 1", code)
	}
	for _, s := range []string{"APP_A read at a.py:1", "APP_B read at a.py:2", "APP_C read at a.py:3", "APP_D read at b.js:1", "APP_E read at b.js:2", "APP_F read at c.go:1"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %s: %s", s, out)
		}
	}
}
func TestProbeSiblingScope(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{"a.py": "os.getenv('APP_NEW')\nos.getenv('APP_OLD')\nos.getenv('OTHER_NEW')", "deploy/env": "APP_OLD"})
	code, out, _ := probeRun(t, bin, d)
	if code != 1 || out != "APP_NEW read at a.py:1; siblings delivered by deploy/env\n" {
		t.Fatalf("sibling scope: %d %q", code, out)
	}
}
func TestProbeArtifactsAndExclusions(t *testing.T) {
	bin := buildCLI(t)
	artifacts := []string{"deploy/e", "infra/e", "k8s/e", "helm/e", ".github/workflows/e", "nested/Dockerfile.prod", "nested/docker-compose.yml", "nested/e.tf", "nested/Procfile", "nested/.env.example", "nested/.env.sample", "nested/.env.template"}
	for _, p := range artifacts {
		d := probeFixture(t, map[string]string{"a.py": "os.getenv('APP_NEW')", p: "APP_OLD"})
		code, out, _ := probeRun(t, bin, d)
		if code != 1 || !strings.Contains(out, "APP_NEW") {
			t.Errorf("artifact %s: %d %q", p, code, out)
		}
	}
	files := map[string]string{"deploy/e": "APP_OLD", "notes.txt": "OTHER_OLD", "a.py": "os.getenv('OTHER_NEW')", ".env": "APP_NEW"}
	for _, p := range []string{"tests/a.py", "test_a.py", "a_test.go", "a.test.js", ".git/a.py", "node_modules/a.js", "vendor/a.go", ".venv/a.py", "venv/a.py", "custom/a.py"} {
		files[p] = "os.getenv('APP_NEW')"
	}
	files["custom/pyvenv.cfg"] = ""
	d := probeFixture(t, files)
	code, out, _ := probeRun(t, bin, d)
	if code != 0 || out != "" {
		t.Fatalf("exclusions: %d %q", code, out)
	}
	// A directory named .env must be skipped too, without opening its contents.
	d = probeFixture(t, map[string]string{".env/hidden.py": "os.getenv('APP_NEW')", "deploy/e": "APP_OLD"})
	code, out, _ = probeRun(t, bin, d)
	if code != 0 || out != "" {
		t.Fatalf(".env opened: %d %q", code, out)
	}
}
func TestProbeOutputAndUsage(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{"a.py": "os.getenv('APP_Z')\nos.getenv('APP_A')", "deploy/b": "APP_OLD", "infra/a": "APP_TWO"})
	code, out, stderr := probeRun(t, bin, d, "--json")
	var got []struct {
		Name        string   `json:"name"`
		ReadAt      []string `json:"read_at"`
		Siblings    []string `json:"siblings"`
		DeliveredBy []string `json:"delivered_by"`
	}
	err := json.Unmarshal([]byte(out), &got)
	if code != 1 || stderr != "" || err != nil || len(got) != 2 {
		t.Fatalf("json: %d %q %q %v", code, out, stderr, err)
	}
	if got[0].Name != "APP_A" || got[1].Name != "APP_Z" || !reflect.DeepEqual(got[0].ReadAt, []string{"a.py:2"}) || !reflect.DeepEqual(got[0].Siblings, []string{"APP_OLD", "APP_TWO"}) || !reflect.DeepEqual(got[0].DeliveredBy, []string{"deploy/b", "infra/a"}) {
		t.Fatalf("json fields: %+v", got)
	}
	clean := t.TempDir()
	code, out, _ = probeRun(t, bin, clean)
	if code != 0 || out != "" {
		t.Fatalf("clean: %d %q", code, out)
	}
	for _, args := range [][]string{{"--bad"}, {d, clean}, {filepath.Join(d, "missing")}, {d, "--bad"}} {
		code, out, stderr = probeRun(t, bin, args...)
		if code != 2 || out != "" || stderr == "" {
			t.Errorf("usage %v: %d %q %q", args, code, out, stderr)
		}
	}
}
func TestProbeSettingsModuleHelperReads(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{
		"settings.js":  "function settings(env = process.env) {\nreturn choice(env, 'APP_MODE', 'all');\n}",
		"settings.py":  "env = os.environ\nvalue = _env(\"APP_X\")",
		"constants.js": "const label = 'APP_NOT_A_READ';",
		"deploy/env":   "APP_OLD",
	})
	code, out, stderr := probeRun(t, bin, d)
	want := "APP_MODE read at settings.js:2; siblings delivered by deploy/env\nAPP_X read at settings.py:2; siblings delivered by deploy/env\n"
	if code != 1 || out != want || stderr != "" {
		t.Fatalf("helper reads: exit %d, stdout %q, stderr %q; want %q", code, out, stderr, want)
	}
}

func TestProbeDoesNotMutateTree(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{"a.py": "os.getenv('APP_NEW')", "deploy/e": "APP_OLD"})
	snapshot := func() map[string]string {
		m := map[string]string{}
		err := filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := e.Info()
			if err != nil {
				return err
			}
			m[p] = info.Mode().String() + info.ModTime().String()
			if !e.IsDir() {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				m[p] += string(b)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	before := snapshot()
	code, _, _ := probeRun(t, bin, d)
	if code != 1 {
		t.Fatalf("probe did not run: exit %d", code)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("scanned tree changed")
	}
}

// `from os import getenv` and `from os import environ` are the same reads as os.getenv and os.environ.
func TestProbeRecognizesImportedPythonReads(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{
		"a.py":       "from os import getenv\nB = getenv(\"APP_B\")\n",
		"b.py":       "from os import environ\nC = environ[\"APP_C\"]\n",
		"deploy/env": "APP_A",
	})
	code, out, _ := probeRun(t, bin, d)
	if code != 1 || !strings.Contains(out, "APP_B read at a.py:2") || !strings.Contains(out, "APP_C read at b.py:2") {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

// os.LookupEnv is Go's other way to read a setting.
func TestProbeRecognizesGoLookupEnv(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{
		"main.go":    "package main\n\nimport \"os\"\n\nfunc main() { v, _ := os.LookupEnv(\"APP_B\"); _ = v }\n",
		"deploy/env": "APP_A",
	})
	code, out, _ := probeRun(t, bin, d)
	if code != 1 || !strings.Contains(out, "APP_B read at main.go:5") {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

// A sibling is a deployed setting. A Dockerfile instruction or a word in a comment is not one.
func TestProbeSiblingIsADeployedSettingNotAnyWord(t *testing.T) {
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{
		"app/config.py":            "import os\nMODE = os.getenv(\"RUN_MODE\")\nKEY = os.getenv(\"API_KEY\")\n",
		"Dockerfile":               "FROM python:3.12\nENV APP_A=1\nRUN pip install x\n",
		".github/workflows/ci.yml": "name: ci\n# call the API before deploy\njobs: {}\n",
	})
	code, out, _ := probeRun(t, bin, d)
	if strings.Contains(out, "RUN_MODE") || strings.Contains(out, "API_KEY") || code != 0 {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

// A directory the probe cannot read (a root-owned data volume) must not hide the findings of the rest.
func TestProbeUnreadableDirectoryDoesNotHideFindings(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	bin := buildCLI(t)
	d := probeFixture(t, map[string]string{
		"app/config.py": "import os\nB = os.getenv(\"APP_B\")\n",
		"deploy/env.sh": "export APP_A=1\n",
	})
	locked := filepath.Join(d, "pgdata")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	code, out, _ := probeRun(t, bin, d)
	if code != 1 || !strings.Contains(out, "APP_B read at app/config.py:2") {
		t.Fatalf("exit %d, output %q", code, out)
	}
}
