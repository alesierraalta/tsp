package manifest

import (
	"strings"
	"testing"

	"example.com/archive/internal/config"
)

func limits() Limits {
	return config.Config{MaxUpload: 1 << 20, MaxEntries: 10, MaxPages: 40, MaxExportRows: 100}.Limits()
}

func body() string {
	return `{
	  "tenant": "acme",
	  "operator": "amara",
	  "created": "2024-04-30T09:00:00Z",
	  "entries": [
	    {"name": "scan-01", "kind": "scan", "title": "Letter", "pages": 2, "bytes": 120,
	     "digest": "0000000000000000000000000000000000000000000000000000000000000000",
	     "source": "https://scanner.example/2024/04/30/scan-01.pdf"},
	    {"name": "listing.csv", "kind": "index", "note": "12 rows"}
	  ]
	}`
}

func TestParseAcceptsADelivery(t *testing.T) {
	m, err := Parse(strings.NewReader(body()), limits())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Tenant != "acme" || len(m.Entries) != 2 {
		t.Fatalf("m = %+v", m)
	}
	if m.Created != "2024-04-30T09:00:00Z" {
		t.Fatalf("created = %q", m.Created)
	}
	if m.Entries[0].KindOf() != KindScan || m.Entries[1].KindOf() != KindIndex {
		t.Fatalf("kinds = %q %q", m.Entries[0].KindOf(), m.Entries[1].KindOf())
	}
	if m.Pages() != 2 || m.TotalBytes() != 120 {
		t.Fatalf("pages=%d bytes=%d", m.Pages(), m.TotalBytes())
	}
}

func TestParseRefusesWhatTheRulesName(t *testing.T) {
	cases := []struct {
		name string
		edit func(string) string
		want string
	}{
		{"name with a separator", func(s string) string { return strings.Replace(s, "scan-01", "scans/scan-01", 1) }, "separator"},
		{"name that walks up", func(s string) string { return strings.Replace(s, "scan-01", "..", 1) }, "dot segment"},
		{"unknown kind", func(s string) string { return strings.Replace(s, `"kind": "scan"`, `"kind": "video"`, 1) }, "kind"},
		{"scan without pages", func(s string) string { return strings.Replace(s, `"pages": 2`, `"pages": 0`, 1) }, "no pages"},
		{"uppercase digest", func(s string) string {
			return strings.Replace(s, "0000000000000000000000000000000000000000000000000000000000000000",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", 1)
		}, "uppercase"},
		{"unknown source scheme", func(s string) string { return strings.Replace(s, "https://", "gopher://", 1) }, "scheme"},
		{"empty tenant", func(s string) string { return strings.Replace(s, `"tenant": "acme"`, `"tenant": ""`, 1) }, "tenant"},
		{"unknown field", func(s string) string {
			return strings.Replace(s, `"operator": "amara",`, `"operator": "amara", "extra": 1,`, 1)
		}, "unknown field"},
		{"repeat name", func(s string) string { return strings.Replace(s, `"listing.csv"`, `"scan-01"`, 1) }, "repeated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(c.edit(body())), limits()); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestParseRefusesABodyOverTheCap(t *testing.T) {
	lim := limits()
	lim.MaxBodyBytes = 32
	if _, err := Parse(strings.NewReader(body()), lim); err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Fatalf("err = %v", err)
	}
}

func TestEncodeRoundTrips(t *testing.T) {
	m, err := Parse(strings.NewReader(body()), limits())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	raw, err := Encode(m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	again, err := Decode(raw, limits())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(again.Entries) != len(m.Entries) || again.Created != m.Created {
		t.Fatalf("round trip changed the manifest: %+v", again)
	}
}

func TestSourceRules(t *testing.T) {
	ok := []string{"https://scanner.example/a.pdf", "file:///srv/scans/a.pdf", "vendor://crate-7"}
	for _, raw := range ok {
		if err := ValidateSource(raw); err != nil {
			t.Errorf("ValidateSource(%q) = %v", raw, err)
		}
	}
	bad := []string{"", "scanner.example/a.pdf", "gopher://x/a", "https://x/a#frag", "https://"}
	for _, raw := range bad {
		if err := ValidateSource(raw); err == nil {
			t.Errorf("ValidateSource(%q) was accepted", raw)
		}
	}
	if SourceClass("https://scanner.example/a") != "delivery" {
		t.Fatal("https is not a delivery")
	}
	if SourceLabel("https://scanner.example/open/scan-01.pdf") != "scanner.example/open/scan-01.pdf" {
		t.Fatalf("label = %q", SourceLabel("https://scanner.example/open/scan-01.pdf"))
	}
}

func TestKindsAndDirs(t *testing.T) {
	scan := Entry{Name: "scan-01.pdf", Pages: 1}
	if scan.KindOf() != KindScan || scan.Dir() != "scans/scan-01.pdf" {
		t.Fatalf("scan kind=%q dir=%q", scan.KindOf(), scan.Dir())
	}
	side := Entry{Name: "scan-01.txt"}
	if side.KindOf() != KindSidecar || side.Dir() != "sidecars/scan-01.txt" {
		t.Fatalf("sidecar kind=%q dir=%q", side.KindOf(), side.Dir())
	}
	if got := (Entry{Name: "scan-02"}).ContainerFile(); got != "scan-02.pdf" {
		t.Fatalf("container = %q", got)
	}
	if got := (Entry{Name: "scan-02"}).PageFile(7); got != "page-007.pgm" {
		t.Fatalf("page = %q", got)
	}
}

func TestKindRulesTable(t *testing.T) {
	if len(Rules()) < 10 {
		t.Fatalf("rules = %d", len(Rules()))
	}
	if err := RequiredPages(KindScan, 0); err == nil {
		t.Fatal("a scan with no pages was accepted")
	}
	if err := RequiredPages(KindScan, 2); err != nil {
		t.Fatalf("RequiredPages(scan, 2) = %v", err)
	}
	if err := RequiredPages(KindSidecar, 1); err == nil {
		t.Fatal("a sidecar with pages was accepted")
	}
	if NormalizeKind("SCANS") != KindScan || NormalizeKind("indices") != KindIndex || NormalizeKind("video") != "" {
		t.Fatal("NormalizeKind folds the scanner's spellings")
	}
}
