package execx

import (
	"strings"
	"testing"
)

func TestCheckToolRefusesAnythingNotAllowed(t *testing.T) {
	if err := CheckTool("tar"); err != nil {
		t.Fatalf("tar: %v", err)
	}
	for _, tool := range []string{"sh", "bash", "/usr/bin/tar", "", "curl"} {
		if err := CheckTool(tool); err == nil {
			t.Errorf("%q was accepted as a tool", tool)
		}
	}
}

func TestCheckArgRefusesAnArgumentThatIsNotOne(t *testing.T) {
	if err := CheckArg("report.csv"); err != nil {
		t.Fatalf("a plain name: %v", err)
	}
	if err := CheckArg("report.csv;printf pwned"); err != nil {
		t.Fatalf("a name carrying punctuation is still one argument: %v", err)
	}
	for _, arg := range []string{"a\x00b", "a\nb", "a\rb", strings.Repeat("x", 5000)} {
		if err := CheckArg(arg); err == nil {
			t.Errorf("%q was accepted as an argument", arg)
		}
	}
}

func TestCheckNameRefusesAPath(t *testing.T) {
	if err := CheckName("report.csv"); err != nil {
		t.Fatalf("a plain name: %v", err)
	}
	for _, name := range []string{"", ".", "..", "/etc/passwd", "..%2fescape", "a/../b"} {
		if err := CheckName(name); err == nil {
			t.Errorf("%q was accepted as a name", name)
		}
	}
}

func TestQuoteIsARenderingNotACommand(t *testing.T) {
	got := Quote("tar", "-czf", "report.csv;printf pwned")
	if !strings.Contains(got, `"report.csv;printf pwned"`) {
		t.Fatalf("quote = %q", got)
	}
}
