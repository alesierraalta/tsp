package policy

import "testing"

func TestOverlay(t *testing.T) {
	base := Config{Retries: 3, Enabled: true}
	if got := Merge(base, Patch{}); got != base {
		t.Fatalf("inherit: %+v", got)
	}
	retries, enabled := 5, true
	if got := Merge(Config{}, Patch{Retries: &retries, Enabled: &enabled}); got != (Config{5, true}) {
		t.Fatalf("overlay: %+v", got)
	}
	if base != (Config{3, true}) {
		t.Fatal("input changed")
	}
}
