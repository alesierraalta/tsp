package bench

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOracleContractBuildConstraints(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, FixtureDir)
	if err := os.MkdirAll(filepath.Join(fixture, "platform"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":           "module example.com/contract\n\ngo 1.22\n",
		"normal.go":        "// Package normal documents the portable API.\npackage normal\n\n// Portable is available everywhere.\nfunc Portable() {}\n",
		"platform/wasm.go": "//go:build js && wasm\n\npackage platform\n\nfunc Browser() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(fixture, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	contract, err := oracleContract(dir, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contract, "func Portable()") {
		t.Fatalf("portable documentation missing: %s", contract)
	}
	if strings.Contains(contract, "func Browser()") {
		t.Fatalf("excluded documentation included: %s", contract)
	}
	if err := os.WriteFile(filepath.Join(fixture, "platform", "broken.go"), []byte("package platform\nfunc Broken(\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := oracleContract(dir, 30*time.Second); err == nil {
		t.Fatal("invalid buildable source must remain a hard error")
	}
}

// The contract documents the packages Go itself would build: testdata, _-prefixed directories and nested
// modules are not part of the module, and a symlinked document is skipped rather than followed or fatal.
func TestOracleContractFollowsGoPackageRules(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	dir := t.TempDir()
	fixture := filepath.Join(dir, FixtureDir)
	files := map[string]string{
		"go.mod":                     "module example.com/contract\n\ngo 1.22\n",
		"README.md":                  "readme body\n",
		"docs/guide.md":              "guide body\n",
		"normal.go":                  "// Package normal documents the API.\npackage normal\n\n// Portable is available everywhere.\nfunc Portable() {}\n",
		"testdata/repo/api/a.go":     "package api\n",
		"testdata/repo/api/b.go":     "package main\n",
		"_examples/x.go":             "package main\nfunc Broken(\n",
		"nested/go.mod":              "module example.com/nested\n\ngo 1.22\n\nrequire example.com/absent v1.0.0\n",
		"nested/n.go":                "package nested\n\nimport _ \"example.com/absent\"\n",
		"outside/CODE_OF_CONDUCT.md": "linked body\n",
	}
	for name, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(fixture, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixture, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("../outside/CODE_OF_CONDUCT.md", filepath.Join(fixture, "docs", "CODE_OF_CONDUCT.md")); err != nil {
		t.Fatal(err)
	}
	contract, err := oracleContract(dir, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"readme body", "guide body", "func Portable()"} {
		if !strings.Contains(contract, want) {
			t.Fatalf("contract lost %q: %s", want, contract)
		}
	}
	if strings.Contains(contract, "linked body") {
		t.Fatalf("symlinked document was followed: %s", contract)
	}
}

func TestOracleContractGoDocEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go uses a POSIX shell")
	}
	dir := t.TempDir()
	fixture := filepath.Join(dir, FixtureDir)
	bin := t.TempDir()
	if err := os.MkdirAll(fixture, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "normal.go"), []byte("package normal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$GOTOOLCHAIN\" = auto ] && [ \"$GOPROXY\" = off ] && [ \"$GOWORK\" = off ] || exit 1\nprintf 'toolchain policy inherited\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOTOOLCHAIN", "auto")
	contract, err := oracleContract(dir, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contract, "toolchain policy inherited") {
		t.Fatalf("missing go doc output: %s", contract)
	}
}
