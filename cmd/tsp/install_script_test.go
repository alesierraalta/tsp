package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/buildinfo"
)

func TestInstallScript(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux installer")
	}
	for _, tool := range []string{"sh", "sha256sum", "tar", "curl", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	version := "v" + buildinfo.Version
	release := filepath.Join(base, version)
	if err := os.MkdirAll(release, 0755); err != nil {
		t.Fatal(err)
	}
	payload := t.TempDir()
	cmd := exec.Command("go", "build", "-o", filepath.Join(payload, "tsp"), "./cmd/tsp")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	arch := runtime.GOARCH
	archive := "tsp_" + buildinfo.Version + "_linux_" + arch + ".tar.gz"
	cmd = exec.Command("tar", "-czf", filepath.Join(release, archive), "-C", payload, "tsp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
	cmd = exec.Command("sha256sum", archive)
	cmd.Dir = release
	sums, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []string{"success", "tampered", "missing", "darwin", "unpublished"} {
		t.Run(tc, func(t *testing.T) {
			checksum := sums
			if tc == "tampered" {
				checksum = []byte(strings.Repeat("0", 64) + "  " + archive + "\n")
			}
			if tc == "missing" {
				checksum = []byte(strings.Repeat("0", 64) + "  other.tar.gz\n")
			}
			if err := os.WriteFile(filepath.Join(release, "checksums.txt"), checksum, 0644); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "bin")
			cmd := exec.Command("sh", filepath.Join(root, "install.sh"))
			cmd.Env = append(os.Environ(), "TSP_RELEASE_BASE_URL=file://"+base, "TSP_VERSION="+version, "TSP_INSTALL_DIR="+dest, "TSP_NO_SETUP=1")
			if tc == "unpublished" {
				cmd.Env = append(cmd.Env, "TSP_VERSION=v9.9.9")
			}
			if tc == "darwin" {
				fake := t.TempDir()
				if err := os.WriteFile(filepath.Join(fake, "uname"), []byte("#!/bin/sh\necho Darwin\n"), 0755); err != nil {
					t.Fatal(err)
				}
				cmd.Env = append(cmd.Env, "PATH="+fake+":"+os.Getenv("PATH"))
			}
			out, err := cmd.CombinedOutput()
			if tc != "success" {
				if err == nil {
					t.Fatalf("expected refusal: %s", out)
				}
				if _, err := os.Stat(filepath.Join(dest, "tsp")); !os.IsNotExist(err) {
					t.Fatalf("installed despite refusal: %v", err)
				}
				if tc == "darwin" && !strings.Contains(string(out), "go install github.com/alesierraalta/tsp/cmd/tsp@latest") {
					t.Fatalf("missing alternative: %s", out)
				}
				// A release that is not published must say which download failed, never exit without a word.
				if tc == "unpublished" && !strings.Contains(string(out), "tsp install: download failed: file://"+base+"/v9.9.9/tsp_9.9.9_linux_") {
					t.Fatalf("missing download failure message: %s", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("install: %v %s", err, out)
			}
			got, err := exec.Command(filepath.Join(dest, "tsp"), "version").CombinedOutput()
			if err != nil || !strings.Contains(string(got), buildinfo.Version) {
				t.Fatalf("version: %v %s", err, got)
			}
		})
	}
}
