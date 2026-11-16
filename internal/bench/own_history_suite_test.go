package bench

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The cases invalidated in the first o* reading (#481), plus the two whose suite is internal/bench and
// therefore runs the corpus checks, must have green fixture suites on a machine whose temp root sits under
// a git repository. This runs the bench path end to end — SOURCE.json to materialized case to scaffold to
// suite — without ever starting a model.
func TestCorpusOwnHistoryFixtureSuitesUnderAnEnclosingRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("materializes own-history cases and runs their suites")
	}
	root := enclosingRepo(t)
	tmp := filepath.Join(root, "bench-runs", "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	runs := filepath.Join(root, "runs")
	for _, id := range []string{
		"o08-bench-score-blank-line-rows",
		"o10-bench-timeout-process-tree",
		"o17-findings-ignored-witnesses",
		"o21-plan-write-gitignored-path",
	} {
		t.Run(id, func(t *testing.T) {
			dir := filepath.Join(corpusDir(t), id)
			built, err := MaterializeCase(dir, DefaultCaseCache())
			if err != nil {
				t.Fatal(err)
			}
			key, err := LoadKey(built)
			if err != nil {
				t.Fatal(err)
			}
			res, err := Scaffold(built, filepath.Join(runs, id), key, 10*time.Minute)
			if err != nil || !res.Green {
				t.Fatalf("fixture suite not green: err=%v\n%s", err, tail(res.Output, 2000))
			}
		})
	}
}
