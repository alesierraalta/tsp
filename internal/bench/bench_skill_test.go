package bench

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/alesierraalta/tsp/internal/assets"
)

// A reading measures the skill under test, so its throwaway config carries that skill and nothing else: the
// other embedded skills would ride every turn as catalog entries and change what the measurement costs, which
// is what the lean skill was measured without.
func TestBenchConfigCarriesOnlyTheSkillUnderTest(t *testing.T) {
	if len(assets.SkillNames()) < 2 {
		t.Skip("the embedded tree ships one skill: nothing to leave out")
	}
	dir := t.TempDir()
	if err := writeEmbeddedSkills(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != BenchSkill {
		t.Fatalf("bench config skills = %v, want only %s", names, BenchSkill)
	}
	err = fs.WalkDir(assets.Skills(), BenchSkill, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		want, _ := fs.ReadFile(assets.Skills(), p)
		got, rerr := os.ReadFile(filepath.Join(dir, "skills", filepath.FromSlash(p)))
		if rerr != nil || !bytes.Equal(got, want) {
			t.Errorf("%s differs from the embedded bytes (read error %v)", p, rerr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBenchSkills(dir); err != nil {
		t.Fatalf("VerifyBenchSkills on the bench config: %v", err)
	}
}
