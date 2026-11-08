// Package citation reads the file references a plan row or a report makes: a path with an extension,
// optionally followed by :line or :first-last.
package citation

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var re = regexp.MustCompile(`([A-Za-z0-9_./\\-]+\.[A-Za-z][A-Za-z0-9]*)(?::(\d+)(?:-(\d+))?)?`)

// Citation is one file reference. First and Last are 0 when the reference names no line, and equal for a single
// line.
type Citation struct {
	Path  string
	First int
	Last  int
}

// Extract returns every file reference in text, in order.
func Extract(text string) []Citation {
	var out []Citation
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		c := Citation{Path: m[1]}
		if m[2] != "" {
			c.First, _ = strconv.Atoi(m[2])
			c.Last = c.First
			if m[3] != "" {
				c.Last, _ = strconv.Atoi(m[3])
			}
		}
		out = append(out, c)
	}
	return out
}

// SamePath matches two references by suffix on whole path segments: "render.js" and "fixture/src/render.js" both
// name "src/render.js", while "traces.go" does not name "search_traces.go".
func SamePath(a, b string) bool {
	x := strings.TrimPrefix(filepath.ToSlash(a), "./")
	y := strings.TrimPrefix(filepath.ToSlash(b), "./")
	if x == y {
		return true
	}
	return strings.HasSuffix(x, "/"+y) || strings.HasSuffix(y, "/"+x)
}
