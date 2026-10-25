package arch

import (
	"path"
	"strings"
)

// jsExtensions are the files whose imports this package reads without a parser. A TypeScript program
// is not a Go program: the specifier is a path, so the layer comes from where the path lands.
var jsExtensions = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true,
}

// jsImport is one specifier and the line it was written on.
type jsImport struct {
	Path string
	Line int
}

// jsImports reads the specifiers a JavaScript or TypeScript file pulls in: the `from` of an import or
// export, a `require(...)` and a dynamic `import(...)`. Comments are not skipped, which is the honest
// limit of reading code without a parser: a commented-out import reports at worst one extra line.
func jsImports(body []byte) []jsImport {
	var out []jsImport
	for i, line := range strings.Split(string(body), "\n") {
		number := i + 1
		for _, spec := range []string{"from", "require(", "import("} {
			rest, found := afterSpecifier(line, spec)
			if !found {
				continue
			}
			out = append(out, jsImport{Path: rest, Line: number})
		}
	}
	return out
}

// afterSpecifier returns the quoted path that follows the given keyword on one line.
func afterSpecifier(line, keyword string) (string, bool) {
	index := strings.Index(line, keyword)
	if index < 0 {
		return "", false
	}
	rest := line[index+len(keyword):]
	for _, quote := range []byte{'\'', '"', '`'} {
		start := strings.IndexByte(rest, quote)
		if start < 0 {
			continue
		}
		end := strings.IndexByte(rest[start+1:], quote)
		if end < 0 {
			continue
		}
		candidate := strings.TrimSpace(rest[start+1 : start+1+end])
		if candidate == "" || strings.ContainsAny(candidate, " \t(") {
			continue
		}
		return candidate, true
	}
	return "", false
}

// jsLayer resolves one specifier from a file to the layer it lands in, or "" when the import leaves the
// repository: a package from node_modules is not an edge between layers of this code.
func jsLayer(declared Declaration, file, specifier string) (string, string) {
	if !strings.HasPrefix(specifier, ".") && !strings.HasPrefix(specifier, "/") {
		if layer := layerOfPath(declared, specifier); layer != "" {
			return path.Clean(specifier), layer
		}
		return "", ""
	}
	if strings.HasPrefix(specifier, "/") {
		return "", ""
	}
	resolved := path.Join(path.Dir(file), specifier)
	return resolved, layerOfPath(declared, resolved)
}
