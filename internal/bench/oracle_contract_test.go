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
