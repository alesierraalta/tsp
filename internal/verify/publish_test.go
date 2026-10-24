package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePackage(t *testing.T, verdict any) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal(verdict)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, VerdictFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A host that already runs its own blind review (RDD freezes the candidate and admits a verdict) must
// not need a second model: it writes the same verdict document and TSP publishes it.
func TestPublishAcceptsAHostVerdict(t *testing.T) {
	dir := writePackage(t, map[string]any{
		"verdict":  VerdictUnpinned,
		"findings": []map[string]any{{"file": "src/app.go", "line": 12, "claim": "the new test pins nothing", "evidence": "passes on base"}},
	})
	res, err := Publish(dir, "abc123", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !res.Blocking() {
		t.Fatalf("verdict %q must block delivery", res.Verdict.Verdict)
	}
	if res.ArtifactPath != filepath.Join(dir, ArtifactName) {
		t.Fatalf("artifact = %q, want it beside the package", res.ArtifactPath)
	}
	if _, err := os.Stat(res.ArtifactPath); err != nil {
		t.Fatalf("the artifact was not written: %v", err)
	}
}

func TestPublishRefusesAVerdictWithoutEvidence(t *testing.T) {
	dir := writePackage(t, map[string]any{"verdict": VerdictUnpinned})
	if _, err := Publish(dir, "abc123", time.Unix(0, 0)); err == nil {
		t.Fatal("want a rejection without evidence refused")
	}
}

func TestPublishRefusesAMissingPackage(t *testing.T) {
	if _, err := Publish(t.TempDir(), "abc123", time.Unix(0, 0)); err == nil {
		t.Fatal("want a package with no verdict refused")
	}
}
