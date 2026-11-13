package execx

import (
	"fmt"
	"strings"
	"unicode"
)

// Tools is the allowlist of binaries a stage may run. A tool outside it is refused, so a stage cannot
// be turned into a runner for whatever the sender typed.
var Tools = map[string]bool{
	"tar":       true,
	"gzip":      true,
	"gunzip":    true,
	"sha256sum": true,
	"unzip":     true,
}

// CheckTool refuses a tool that is not on the allowlist or that carries a path to somewhere else.
func CheckTool(tool string) error {
	if tool == "" {
		return fmt.Errorf("%w: the tool name is empty", ErrTool)
	}
	if strings.ContainsAny(tool, "/\\") {
		return fmt.Errorf("%w: %q carries a path", ErrTool, tool)
	}
	if !Tools[tool] {
		return fmt.Errorf("%w: %q", ErrTool, tool)
	}
	return nil
}

// CheckArg refuses an argument a process may not carry. A NUL ends a C string, a newline and a carriage
// return are how a second line of a file becomes a second instruction, and an argument above the bound
// is a name no filesystem accepts.
func CheckArg(arg string) error {
	if strings.ContainsRune(arg, 0) {
		return fmt.Errorf("%w: it carries a NUL byte", ErrArg)
	}
	if strings.ContainsAny(arg, "\n\r") {
		return fmt.Errorf("%w: %q carries a line break", ErrArg, arg)
	}
	if len(arg) > 4096 {
		return fmt.Errorf("%w: it is %d bytes", ErrArg, len(arg))
	}
	return nil
}

// CheckName refuses a name a stage may write. It is stricter than CheckArg because a name becomes a
// path: an absolute name, one that walks upwards and one carrying a separator the layout does not
// expect are all refusals, and control characters are refused so a name stays one line of a manifest.
func CheckName(name string) error {
	if err := CheckArg(name); err != nil {
		return err
	}
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("%w: %q is not a name", ErrArg, name)
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return fmt.Errorf("%w: %q is absolute", ErrArg, name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q walks upwards", ErrArg, name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: %q carries a control character", ErrArg, name)
		}
	}
	return nil
}

// Quote renders an argv the way a log line carries it, with every element quoted so an operator can see
// the boundaries a process receives. It is a rendering, never a command line that is executed.
func Quote(tool string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, tool)
	for _, arg := range args {
		parts = append(parts, fmt.Sprintf("%q", arg))
	}
	return strings.Join(parts, " ")
}
