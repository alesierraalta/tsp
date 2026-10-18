package bench

import (
	"os"
	"path/filepath"
)

// useBenchGoCache points GOCACHE at a build cache under root that only the bench uses, unless the operator set
// one. Fixture suites, fix variants and agent runs all inherit it. The operator's default cache is cleaned by
// other tools (go clean -cache, disk reclaim); an entry deleted mid-reading fails a build with "package ... is
// not in std" or "open .../go-build/...", which reads as an invalid case or a broken agent test.
func useBenchGoCache(root string) {
	if os.Getenv("GOCACHE") != "" {
		return
	}
	_ = os.Setenv("GOCACHE", filepath.Join(root, "go-build"))
}
