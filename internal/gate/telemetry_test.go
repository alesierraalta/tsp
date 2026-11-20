package gate

import (
	"os"
	"testing"
)

// TestMain points the installation state at a throwaway directory, so a machine with the telemetry
// feature on cannot receive usage events from a gate test that runs the real Stop hook.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "tsp-gate-state-")
	if err != nil {
		panic(err)
	}
	os.Setenv("TSP_HOME", root)
	os.Setenv("TPP_HOME", "")
	os.Setenv("RDD_PLUS_HOME", "")
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
