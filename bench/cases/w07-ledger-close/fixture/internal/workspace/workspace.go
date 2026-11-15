// Package workspace is the directory the service keeps its statements in, and the only code that
// turns a name into a filesystem path. A name is a slash-separated relative name such as
// "tenants/acme/statements/2024-05/team-api/statement.pdf". It is resolved here once, against the
// configured root, and the resolved path is what the callers use: no other package joins a path of
// its own, which is what keeps a name from deciding where the service reads and writes.
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"example.com/ledgerclose/internal/calendar"
)

// Errors the path rule reports. Callers compare with errors.Is.
var (
	ErrOutside    = errors.New("workspace: path leaves the root")
	ErrBadName    = errors.New("workspace: invalid name")
	ErrNoObject   = errors.New("workspace: no such object")
	ErrNotRegular = errors.New("workspace: object is not a regular file")
)

// sep is the separator the containment comparison is built from, so the rule and filepath.Join
// agree about what a separator is.
const sep = string(filepath.Separator)

// segmentRE is the form of the variable parts of a statement name: a tenant id and an account id.
// The workspace checks them itself instead of trusting the caller, because they are joined onto the
// root.
var segmentRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// ValidSegment reports whether a value may be one part of a name.
func ValidSegment(value string) bool { return segmentRE.MatchString(value) }

// Workspace is one directory under its absolute root.
type Workspace struct {
	root string
}

// Open prepares the workspace at root. The root is made absolute and cleaned first; a root that is
// the filesystem root is refused, because every containment check would then be vacuous.
func Open(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: empty root", ErrBadName)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: root %q: %w", root, err)
	}
	absolute = filepath.Clean(absolute)
	if absolute == sep {
		return nil, fmt.Errorf("%w: the filesystem root is not a workspace", ErrBadName)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("workspace: root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: root %q is not a directory", ErrBadName, root)
	}
	return &Workspace{root: absolute}, nil
}

// Root is the absolute root. It is a value, not a handle: a caller that needs a path inside the
// workspace asks Resolve, never Root plus its own join.
func (w *Workspace) Root() string { return w.root }

// Resolve returns the absolute path of a name. The name is checked first: empty, a control
// character, an absolute name, a name that looks like a Windows drive, and a name whose cleaned
// form walks upwards are all refused. The joined path is then checked as well, so a name that is
// harmless on its own but lands outside after cleaning is refused too.
func (w *Workspace) Resolve(name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	clean := path.Clean(filepath.ToSlash(name))
	joined := filepath.Join(w.root, filepath.FromSlash(clean))
	if !contained(w.root, joined) {
		return "", fmt.Errorf("%w: %q", ErrOutside, name)
	}
	return joined, nil
}

// checkName is the name rule. It runs before any join, so a refused name never reaches the
// filesystem and no directory is created for it.
func checkName(name string) error {
	if name == "" || len(name) > 512 {
		return fmt.Errorf("%w: %d bytes", ErrBadName, len(name))
	}
	for i := 0; i < len(name); i++ {
		if name[i] < 0x20 || name[i] == 0x7f {
			return fmt.Errorf("%w: %q carries a control character", ErrBadName, name)
		}
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return fmt.Errorf("%w: %q is absolute", ErrBadName, name)
	}
	// "c:" is a drive to filepath on Windows and a plain name to filepath here; the rule refuses both
	// readings, so a statement written on one host is readable on the other.
	if len(name) >= 2 && name[1] == ':' {
		return fmt.Errorf("%w: %q looks like a drive path", ErrBadName, name)
	}
	cleaned := path.Clean(filepath.ToSlash(name))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: %q walks upwards", ErrOutside, name)
	}
	if cleaned == "." {
		return fmt.Errorf("%w: %q is the root itself", ErrBadName, name)
	}
	return nil
}

// contained reports whether p is the root or sits under it. It is a prefix test that includes the
// separator, which is the point: "/srv/close-copy" does not start with "/srv/close/", so a sibling
// directory that shares the root's name is not inside the tree. The test is made on the joined path
// after filepath.Join, which has already cleaned it, so no ".." survives into it.
func contained(root, p string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+sep)
}

// Write stores an object, creating its directory, and returns the path it wrote. The bytes go to a
// temporary file in the object's own directory and are renamed, so a reader never sees half a statement.
func (w *Workspace) Write(name string, data []byte, mode fs.FileMode) (string, error) {
	target, err := w.Resolve(name)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("workspace: prepare %q: %w", name, err)
	}
	temp, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return "", fmt.Errorf("workspace: stage %q: %w", name, err)
	}
	staged := temp.Name()
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		os.Remove(staged)
		return "", fmt.Errorf("workspace: write %q: %w", name, err)
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		os.Remove(staged)
		return "", fmt.Errorf("workspace: mode %q: %w", name, err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(staged)
		return "", fmt.Errorf("workspace: close %q: %w", name, err)
	}
	if err := os.Rename(staged, target); err != nil {
		os.Remove(staged)
		return "", fmt.Errorf("workspace: publish %q: %w", name, err)
	}
	return target, nil
}

// Read returns the bytes of an object. A name that resolves to a symbolic link or to something that
// is not a regular file is refused: the containment rule is lexical, so the one way a name could
// still point outside the root is a link some other process planted, and the service does not read
// one.
func (w *Workspace) Read(name string) ([]byte, error) {
	target, err := w.Resolve(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q", ErrNoObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: stat %q: %w", name, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", ErrNotRegular, name)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("workspace: read %q: %w", name, err)
	}
	return data, nil
}

// StatementName is the name of the statement of one account and period:
// tenants/<tenant>/statements/<period>/<account>/statement.<ext> Every variable part is validated
// here, so the only variable bytes of the name are lowercase letters, digits, a dash, a dot and the
// digits of the month.
func StatementName(tenant string, period calendar.Period, account, extension string) (string, error) {
	if !ValidSegment(tenant) {
		return "", fmt.Errorf("%w: tenant %q", ErrBadName, tenant)
	}
	if !period.Valid() {
		return "", fmt.Errorf("%w: period %q", ErrBadName, period)
	}
	if !ValidSegment(account) {
		return "", fmt.Errorf("%w: account %q", ErrBadName, account)
	}
	switch extension {
	case ".html", ".pdf":
	default:
		return "", fmt.Errorf("%w: statement extension %q", ErrBadName, extension)
	}
	return "tenants/" + tenant + "/statements/" + period.String() + "/" + account + "/statement" + extension, nil
}

// StatementPath resolves the statement of one account and period.
func (w *Workspace) StatementPath(tenant string, period calendar.Period, account, extension string) (string, error) {
	name, err := StatementName(tenant, period, account, extension)
	if err != nil {
		return "", err
	}
	return w.Resolve(name)
}
