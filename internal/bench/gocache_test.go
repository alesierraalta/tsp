package bench

import (
	"os"
	"path/filepath"
	"testing"
)

// A reading builds Go in a cache of its own: anything that cleans the operator's cache mid-reading (go clean
// -cache, a disk reclaim) would otherwise fail fixture suites and agent builds with "package ... is not in std".
func TestBenchGoCacheIsTheBenchsOwnUnlessTheOperatorChoseOne(t *testing.T) {
	t.Setenv("GOCACHE", "")
	os.Unsetenv("GOCACHE")
	cache := t.TempDir()
	useBenchGoCache(cache)
	if got, want := os.Getenv("GOCACHE"), filepath.Join(cache, "go-build"); got != want {
		t.Fatalf("GOCACHE = %q, want %q", got, want)
	}

	t.Setenv("GOCACHE", "/operator/choice")
	useBenchGoCache(cache)
	if got := os.Getenv("GOCACHE"); got != "/operator/choice" {
		t.Fatalf("an operator's GOCACHE must win, got %q", got)
	}
}
