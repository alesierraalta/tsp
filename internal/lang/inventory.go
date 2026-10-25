// Package lang answers one question for every command that shells out: what is this repository made
// of, and which tools for it are actually installed here. Each command that guessed on its own guessed
// differently, and a tool nobody can run is not a capability.
package lang

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Language is one stack found in the repository, with the tools that stack needs.
type Language struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
	Tools []Tool `json:"tools"`
}

// Tool is one executable a language needs, and whether this machine has it.
type Tool struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	Available bool   `json:"available"`
	Install   string `json:"install,omitempty"`
}

// Inventory is what the repository is made of, ordered by language name.
type Inventory struct {
	Languages []Language `json:"languages"`
}

// Deps are the process boundaries, injected so detection is testable without a real machine.
type Deps struct {
	LookPath func(name string) (string, error)
}

// toolchain maps a language to the empty tools it needs, in the order they are reported.
var toolchain = []struct {
	Language string
	Exts     []string
	Tools    []Tool
}{
	{
		Language: "go",
		Exts:     []string{".go"},
		Tools: []Tool{
			{Name: "tests", Command: "go", Install: "install Go from https://go.dev/dl/"},
			{Name: "mutation", Command: "gremlins", Install: `go install github.com/go-gremlins/gremlins/cmd/gremlins@latest`},
		},
	},
	{
		Language: "javascript",
		Exts:     []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx"},
		Tools: []Tool{
			{Name: "tests", Command: "node", Install: "install Node.js from https://nodejs.org/"},
			{Name: "mutation", Command: "npx", Install: "npm install --save-dev @stryker-mutator/core"},
		},
	},
	{
		Language: "python",
		Exts:     []string{".py"},
		Tools: []Tool{
			{Name: "tests", Command: "python3", Install: "install Python 3 from https://www.python.org/downloads/"},
			{Name: "mutation", Command: "mutmut", Install: `pipx install mutmut`},
		},
	},
}

// Detect counts the files of each known language below root, skipping the trees that are never the
// repository's own code: a vendored dependency or a virtualenv documents another project's choices.
func Detect(root string, d Deps, skip []string) Inventory {
	counts := map[string]int{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && ignored(entry.Name(), skip) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		for _, entry := range toolchain {
			for _, candidate := range entry.Exts {
				if ext == candidate {
					counts[entry.Language]++
				}
			}
		}
		return nil
	})

	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	out := Inventory{}
	for _, entry := range toolchain {
		if counts[entry.Language] == 0 {
			continue
		}
		language := Language{Name: entry.Language, Files: counts[entry.Language]}
		for _, tool := range entry.Tools {
			found := tool
			_, err := d.LookPath(tool.Command)
			found.Available = err == nil
			language.Tools = append(language.Tools, found)
		}
		out.Languages = append(out.Languages, language)
	}
	sort.Slice(out.Languages, func(i, j int) bool { return out.Languages[i].Name < out.Languages[j].Name })
	return out
}

// ignored reports whether a directory name is one no command should read as the repository's own code.
func ignored(name string, extra []string) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return name != ".github"
	}
	switch name {
	case "node_modules", "vendor", "testdata", "__pycache__", "site-packages":
		return true
	}
	for _, candidate := range extra {
		if name == candidate {
			return true
		}
	}
	return false
}
