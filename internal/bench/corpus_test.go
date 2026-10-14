package bench

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// corpusDir is the shipped benchmark; tests here check the corpus itself, not the runner.
func corpusDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "bench", "cases")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no shipped corpus beside the module")
	}
	return dir
}

// hiddenTriggerRe is a bench-internal hook a fixture must not carry: a TPP_ variable that switches a test on,
// or a test named after the trigger that proves a planted defect.
var hiddenTriggerRe = regexp.MustCompile(`TPP_[A-Z_]+|func Test\w*Trigger\b|test\(["'][^"']*trigger`)

// The fixture is what the agent sees. A trigger test shipped in it, even one skipped by default, names the
// defect and hands over its proof: the agent can run it instead of finding the defect, and the reading
// stops measuring discovery.
func TestCorpusFixturesCarryNoHiddenTriggers(t *testing.T) {
	cases, _ := filepath.Glob(filepath.Join(corpusDir(t), "*"))
	for _, c := range cases {
		_ = filepath.Walk(filepath.Join(c, FixtureDir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if m := hiddenTriggerRe.Find(data); m != nil {
				t.Errorf("%s: the fixture carries a hidden trigger %q", path, m)
			}
			return nil
		})
	}
}

// Every case ships the fixed versions the discriminating check needs.
func TestCorpusShipsFixedVersions(t *testing.T) {
	cases, _ := filepath.Glob(filepath.Join(corpusDir(t), "*"))
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil {
			t.Errorf("%s: %v", c, err)
			continue
		}
		if key.IsCleanControl() {
			if len(key.Defects) != 0 {
				t.Errorf("%s: clean control declares %d defects", filepath.Base(c), len(key.Defects))
			}
			continue
		}
		if st, err := os.Stat(filepath.Join(c, FixDir, "all")); err != nil || !st.IsDir() {
			t.Errorf("%s: no fix/all", filepath.Base(c))
		}
		if len(key.Defects) < 2 {
			continue
		}
		for _, d := range key.Defects {
			if st, err := os.Stat(filepath.Join(c, FixDir, "keep-"+d.ID)); err != nil || !st.IsDir() {
				t.Errorf("%s: no fix/keep-%s", filepath.Base(c), d.ID)
			}
		}
	}
}

func TestCorpusShipsCleanControlsWithoutFixVersions(t *testing.T) {
	cases, _ := filepath.Glob(filepath.Join(corpusDir(t), "*"))
	controls := 0
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil || !key.IsCleanControl() {
			continue
		}
		controls++
		if len(key.Defects) != 0 {
			t.Errorf("%s: clean control declares %d defects", filepath.Base(c), len(key.Defects))
		}
		if _, err := os.Stat(filepath.Join(c, FixDir)); !os.IsNotExist(err) {
			t.Errorf("%s: clean control ships a fix directory", filepath.Base(c))
		}
	}
	if controls == 0 {
		t.Fatal("no clean controls in shipped corpus")
	}
}

// The happy suite is green on the fixture and on every fixed variant: a variant that breaks the
// suite would make the check read "red" for the wrong reason.
func TestCorpusSuitesAreGreenOnEveryVariant(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the corpus suites")
	}
	cases, _ := filepath.Glob(filepath.Join(corpusDir(t), "*"))
	for _, c := range cases {
		key, err := LoadKey(c)
		if err != nil {
			continue
		}
		variants := []string{""}
		if entries, err := os.ReadDir(filepath.Join(c, FixDir)); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					variants = append(variants, filepath.Join(c, FixDir, e.Name()))
				}
			}
		}
		for _, v := range variants {
			green, out, err := suiteOn(filepath.Join(c, FixtureDir), v, c, nil, key.Suite, 5*time.Minute)
			if err != nil || !green {
				t.Errorf("%s variant %q: green=%v err=%v\n%s", filepath.Base(c), filepath.Base(v), green, err, tail(out, 600))
			}
		}
	}
}
