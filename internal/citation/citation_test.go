package citation

import (
	"reflect"
	"testing"
)

func TestExtractReadsPathsLinesAndRanges(t *testing.T) {
	got := Extract("see src/a.go, then `middleware/decompress.go:85` and pkg/x.go:10-14.")
	want := []Citation{
		{Path: "src/a.go"},
		{Path: "middleware/decompress.go", First: 85, Last: 85},
		{Path: "pkg/x.go", First: 10, Last: 14},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Extract = %+v, want %+v", got, want)
	}
	if got := Extract("no file here"); len(got) != 0 {
		t.Fatalf("Extract on prose = %+v, want none", got)
	}
}

func TestSamePathMatchesBySuffixOnWholeSegments(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"search_traces.go", "cmd/jaeger/internal/handlers/search_traces.go", true},
		{"./src/render.js", "src/render.js", true},
		{"fixture/src/render.js", "src/render.js", true},
		{"traces.go", "search_traces.go", false},
		{"src/a.go", "src/b.go", false},
	} {
		if got := SamePath(tc.a, tc.b); got != tc.want {
			t.Errorf("SamePath(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
