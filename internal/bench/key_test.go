package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKey(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, KeyFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadKey(t *testing.T) {
	valid := `{"id":"c1","language":"node","suite":"node --test","surface":"lib","defects":[{"id":"d1","file":"src/a.js","line":3,"class":"boundary","keywords":["limit"],"description":"x","trigger":{"input":"i","expected":"e","actual":"a"},"why_missed":"w"}]}`
	cases := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"valid key parses", valid, ""},
		{"malformed json", "{not json", "parse key"},
		{"language outside node/go", strings.Replace(valid, `"node"`, `"rust"`, 1), "not node or go"},
		{"defect without keywords", strings.Replace(valid, `["limit"]`, `[]`, 1), "no keywords"},
		{"defect without a positive line", strings.Replace(valid, `"line":3`, `"line":0`, 1), "positive line"},
		{"no defects", `{"id":"c1","language":"go","suite":"go test ./...","defects":[]}`, "no defects"},
		{"repeated defect id", strings.Replace(valid, `"defects":[`, `"defects":[{"id":"d1","file":"f.go","line":1,"keywords":["k"]},`, 1), "repeated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, err := LoadKey(writeKey(t, tc.body))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if k.ID != "c1" || len(k.Defects) != 1 || k.Defects[0].Trigger.Expected != "e" {
					t.Fatalf("parsed key wrong: %+v", k)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// Traps are documented behaviors planted next to a case's real defects: a flow that reports one
// instead of probing against the fixture README drags precision below 1.00. They live in their own
// top-level array — never inside defects — so every defect consumer (totals, keep-directory
// requirements, mechanical matching, caught attribution, clean-control counting) excludes them by
// construction, and adjudication can never confirm a trap id as a defect.
func TestLoadKeyTraps(t *testing.T) {
	valid := `{"id":"c1","language":"node","suite":"node --test","surface":"lib","defects":[{"id":"d1","file":"src/a.js","line":3,"class":"boundary","keywords":["limit"],"description":"x","trigger":{"input":"i","expected":"e","actual":"a"},"why_missed":"w"}],"traps":[{"id":"t1","file":"src/a.js","line":9,"class":"documented-behavior","keywords":["jitter"],"description":"documented jitter","documented":"README.md:12"}]}`
	cases := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"a trap parses with its documented line", valid, ""},
		{"trap without id", strings.Replace(valid, `"id":"t1",`, ``, 1), "no id"},
		{"trap without keywords", strings.Replace(valid, `["jitter"]`, `[]`, 1), "no keywords"},
		{"trap without a positive line", strings.Replace(valid, `"line":9`, `"line":0`, 1), "positive line"},
		{"trap without its documented line", strings.Replace(valid, `,"documented":"README.md:12"`, ``, 1), "documented"},
		{"trap id repeating a defect id", strings.Replace(valid, `"id":"t1"`, `"id":"d1"`, 1), "repeated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, err := LoadKey(writeKey(t, tc.body))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(k.Traps) != 1 || k.Traps[0].ID != "t1" || k.Traps[0].Documented != "README.md:12" {
					t.Fatalf("parsed traps wrong: %+v", k.Traps)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadKeyMissingFile(t *testing.T) {
	if _, err := LoadKey(t.TempDir()); err == nil || !strings.Contains(err.Error(), "read key") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadKeyCleanControl(t *testing.T) {
	clean := `{"id":"c1","language":"go","suite":"go test ./...","surface":"lib","control":"clean","defects":[]}`
	key, err := LoadKey(writeKey(t, clean))
	if err != nil {
		t.Fatalf("clean control refused: %v", err)
	}
	if !key.IsCleanControl() {
		t.Fatalf("IsCleanControl = false for %+v", key)
	}

	withDefect := `{"id":"c1","language":"go","suite":"go test ./...","surface":"lib","control":"clean","defects":[{"id":"d1","file":"src/a.go","line":3,"keywords":["limit"]}]}`
	if _, err := LoadKey(writeKey(t, withDefect)); err == nil || !strings.Contains(err.Error(), "1 defect") {
		t.Fatalf("clean control with one defect error = %v, want the count", err)
	}

	unknown := strings.Replace(clean, `"clean"`, `"mystery"`, 1)
	if _, err := LoadKey(writeKey(t, unknown)); err == nil || !strings.Contains(err.Error(), `mystery`) || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("unknown control error = %v, want value and accepted control", err)
	}
}

// A case's bounded request is the unit of work the run is asked to test. A key without one is the
// generic case the bench has always run; a key that supplies a blank one is a mistake in the key, so
// it is refused instead of being read back as "no request".
func TestLoadKeyValidatesTheCaseRequest(t *testing.T) {
	valid := `{"id":"c1","language":"node","suite":"node --test","surface":"lib","defects":[{"id":"d1","file":"src/a.js","line":3,"class":"boundary","keywords":["limit"],"description":"x","trigger":{"input":"i","expected":"e","actual":"a"},"why_missed":"w"}]}`
	withRequest := func(value string) string {
		return strings.Replace(valid, `"surface":"lib"`, `"surface":"lib","request":`+value, 1)
	}
	cases := []struct {
		name    string
		body    string
		want    string
		wantErr string
	}{
		{"no request is the generic case", valid, "", ""},
		{"a request names the bounded unit of work", withRequest(`"src/slug.js"`), "src/slug.js", ""},
		{"a request is trimmed", withRequest(`"  src/slug.js  "`), "src/slug.js", ""},
		{"a blank request is refused", withRequest(`"   "`), "", "request"},
		{"a null request is the generic case", withRequest("null"), "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, err := LoadKey(writeKey(t, tc.body))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := k.RequestText(); got != tc.want {
				t.Fatalf("RequestText = %q, want %q", got, tc.want)
			}
		})
	}
}
