package findings

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/state"
)

// ClassUse is what one class of defect has done to one repository: how many witnesses it produced and where the
// newest one lived. It is knowledge about the repository, never evidence about a branch, so it is not a finding and
// never reaches the registry.
type ClassUse struct {
	Name  string    `json:"name"`
	Count int       `json:"count"`
	Last  string    `json:"last_test,omitempty"`
	File  string    `json:"last_file,omitempty"`
	At    time.Time `json:"last_at"`
}

// classMemory is the whole file for one repository. Version exists so a later shape is refused instead of guessed.
type classMemory struct {
	Version    int        `json:"version"`
	Repository string     `json:"repository"`
	Classes    []ClassUse `json:"classes"`
}

// Classes answers the defect classes the tool recognizes. They are the bench families: a second list would drift.
func Classes() []string { return append([]string(nil), bench.Families...) }

// classList writes the classes as next names them: the class, then how many witnesses it has produced here.
func classList(classes []ClassUse) string {
	parts := make([]string, 0, len(classes))
	for _, c := range classes {
		parts = append(parts, fmt.Sprintf("%s %d", c.Name, c.Count))
	}
	return strings.Join(parts, ", ")
}

func knownClass(name string) bool {
	for _, family := range bench.Families {
		if family == name {
			return true
		}
	}
	return false
}

// memoryPath resolves the memory file of the repository dir belongs to. The identity is the git common directory,
// the same one the registry uses, so every worktree of one clone shares its memory and another clone does not.
func memoryPath(dir string) (string, error) {
	common, e := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return "", e
	}
	root, e := state.Root()
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256([]byte(filepath.Clean(common)))
	return filepath.Join(root, "memory", fmt.Sprintf("%x", sum[:8])+".json"), nil
}

// ClassMemory answers the classes already recorded for the repository dir belongs to, most frequent first and by
// name when two classes tie. A repository with no memory answers an empty list.
func ClassMemory(dir string) ([]ClassUse, error) {
	top, e := git(dir, "rev-parse", "--show-toplevel")
	if e != nil {
		return nil, e
	}
	path, e := memoryPath(top)
	if e != nil {
		return nil, e
	}
	m, e := readMemory(path)
	if e != nil {
		return nil, e
	}
	uses := m.Classes
	sort.SliceStable(uses, func(i, j int) bool {
		if uses[i].Count != uses[j].Count {
			return uses[i].Count > uses[j].Count
		}
		return uses[i].Name < uses[j].Name
	})
	return uses, nil
}

func readMemory(path string) (*classMemory, error) {
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return &classMemory{Version: 1, Classes: []ClassUse{}}, nil
	}
	if e != nil {
		return nil, e
	}
	var m *classMemory
	if e = json.Unmarshal(b, &m); e != nil {
		return nil, fmt.Errorf("unreadable defect memory %s: %w", path, e)
	}
	if m == nil || m.Version != 1 {
		return nil, fmt.Errorf("unreadable defect memory %s: invalid version", path)
	}
	return m, nil
}

// classWitness is one witness a round pinned, as the memory remembers it.
type classWitness struct {
	test string
	file string
}

// rememberClass adds the witnesses this round pinned to the class the caller declared, under the memory's own lock.
// It never guesses: a round with no witness, or one that declared nothing, writes nothing.
func rememberClass(dir, class string, witnesses []classWitness) error {
	if len(witnesses) == 0 {
		return nil
	}
	top, e := git(dir, "rev-parse", "--show-toplevel")
	if e != nil {
		return e
	}
	path, e := memoryPath(top)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	unlock, e := lockRegistry(path)
	if e != nil {
		return e
	}
	defer unlock()
	m, e := readMemory(path)
	if e != nil {
		return e
	}
	index := -1
	for i := range m.Classes {
		if m.Classes[i].Name == class {
			index = i
			break
		}
	}
	if index < 0 {
		m.Classes = append(m.Classes, ClassUse{Name: class})
		index = len(m.Classes) - 1
	}
	use := &m.Classes[index]
	use.Count += len(witnesses)
	newest := witnesses[len(witnesses)-1]
	use.Last, use.File, use.At = newest.test, newest.file, time.Now().UTC()
	m.Version, m.Repository = 1, top
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	return state.WriteAtomic(path, append(b, '\n'))
}

// unknownClass is the refusal a record gets when it declares a class the tool does not recognize.
func unknownClass(class string) error {
	return fmt.Errorf("unknown defect class %q: use one of %s", class, strings.Join(Classes(), ", "))
}
