package assets

import (
	"errors"
	"testing"
)

func testResolver(t *testing.T) *Resolver {
	t.Helper()
	r, err := New("/srv/assets/", map[string]string{
		"app":    "bundles/app.min.js",
		"styles": "bundles/app.min.css",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestNewCleansTheRoot(t *testing.T) {
	r := testResolver(t)
	if r.Root() != "/srv/assets" {
		t.Fatalf("Root() = %q, want %q", r.Root(), "/srv/assets")
	}
}

func TestNewRefusesARelativeRoot(t *testing.T) {
	if _, err := New("srv/assets", nil); err == nil {
		t.Fatal("New accepted a relative root")
	}
}

func TestNewRefusesTheFilesystemRoot(t *testing.T) {
	if _, err := New("/", nil); err == nil {
		t.Fatal("New accepted the filesystem root")
	}
}

func TestResolveJoinsARelativePath(t *testing.T) {
	r := testResolver(t)
	got, err := r.Resolve("img/logo.png")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/srv/assets/img/logo.png" {
		t.Fatalf("Resolve = %q", got)
	}
}

func TestResolveDecodesPercentEncoding(t *testing.T) {
	r := testResolver(t)
	got, err := r.Resolve("img/logo%20wide.png")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/srv/assets/img/logo wide.png" {
		t.Fatalf("Resolve = %q", got)
	}
}

func TestResolveRefusesAParentSegment(t *testing.T) {
	r := testResolver(t)
	if _, err := r.Resolve("../secrets/key.pem"); !errors.Is(err, ErrOutside) {
		t.Fatalf("Resolve of a parent segment = %v, want ErrOutside", err)
	}
}

func TestResolveRefusesAnAbsolutePath(t *testing.T) {
	r := testResolver(t)
	if _, err := r.Resolve("/etc/passwd"); !errors.Is(err, ErrOutside) {
		t.Fatalf("Resolve of an absolute path = %v, want ErrOutside", err)
	}
}

func TestResolveRefusesAnEmptyPath(t *testing.T) {
	r := testResolver(t)
	if _, err := r.Resolve(""); !errors.Is(err, ErrEmpty) {
		t.Fatalf("Resolve of an empty path = %v, want ErrEmpty", err)
	}
}

func TestDecodeReportsMalformedEncoding(t *testing.T) {
	if _, err := Decode("img/%zz.png"); err == nil {
		t.Fatal("Decode accepted malformed percent-encoding")
	}
}

func TestBundleReturnsTheManifestPath(t *testing.T) {
	r := testResolver(t)
	got, err := r.Bundle("app")
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	if got != "/srv/assets/bundles/app.min.js" {
		t.Fatalf("Bundle = %q", got)
	}
}

func TestBundleRefusesAnUnknownName(t *testing.T) {
	r := testResolver(t)
	if _, err := r.Bundle("missing"); !errors.Is(err, ErrNoBundle) {
		t.Fatalf("Bundle of an unknown name = %v, want ErrNoBundle", err)
	}
}

func TestKeyIsThePathRelativeToTheRoot(t *testing.T) {
	r := testResolver(t)
	if got := r.Key("bundles/app.min.js"); got != "bundles/app.min.js" {
		t.Fatalf("Key = %q", got)
	}
	if got := r.Key("../secrets/key.pem"); got != "" {
		t.Fatalf("Key of a refused path = %q, want empty", got)
	}
}

func TestManifestIsSorted(t *testing.T) {
	r := testResolver(t)
	names := r.Manifest()
	if len(names) != 2 || names[0] != "app" || names[1] != "styles" {
		t.Fatalf("Manifest = %v", names)
	}
}

func TestReloadReplacesTheManifest(t *testing.T) {
	r := testResolver(t)
	if err := r.Reload(map[string]string{"app": "bundles/app.v2.js"}); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	got, err := r.Bundle("app")
	if err != nil {
		t.Fatalf("Bundle after Reload: %v", err)
	}
	if got != "/srv/assets/bundles/app.v2.js" {
		t.Fatalf("Bundle after Reload = %q", got)
	}
	if len(r.Manifest()) != 1 {
		t.Fatalf("Manifest after Reload = %v", r.Manifest())
	}
}
