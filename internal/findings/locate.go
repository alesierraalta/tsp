package findings

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func digest(body string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(body))) }

func locate(root, name string, files []string) (string, string, string) {
	for _, file := range files {
		if file == "" || filepath.IsAbs(file) || file == ".." || strings.HasPrefix(filepath.Clean(file), ".."+string(filepath.Separator)) {
			continue
		}
		data, e := os.ReadFile(filepath.Join(root, file))
		if e != nil {
			continue
		}
		source := string(data)
		var body, promise string
		switch {
		case strings.HasSuffix(file, "_test.go"):
			i := strings.LastIndex(name, ".Test")
			if i < 0 {
				continue
			}
			test := strings.Split(name[i+1:], "/")[0]
			pkg := name[:i]
			dir := filepath.ToSlash(filepath.Dir(file))
			if dir != "." && !strings.HasSuffix(pkg, "/"+dir) {
				continue
			}
			fs := token.NewFileSet()
			tree, e := parser.ParseFile(fs, file, data, parser.ParseComments)
			if e != nil {
				continue
			}
			for _, decl := range tree.Decls {
				f, ok := decl.(*ast.FuncDecl)
				if !ok || f.Name.Name != test || f.Body == nil {
					continue
				}
				body = source[fs.Position(f.Body.Pos()).Offset:fs.Position(f.Body.End()).Offset]
				if f.Doc != nil {
					promise = first(f.Doc.Text())
				}
				break
			}
		case strings.HasSuffix(file, ".py"):
			parts := strings.Split(name, "::")
			if len(parts) < 2 || filepath.ToSlash(file) != parts[0] {
				continue
			}
			test := strings.Split(parts[len(parts)-1], "[")[0]
			body, promise = pythonBody(source, test, parts[1:len(parts)-1])
		case strings.HasSuffix(file, ".js") || strings.HasSuffix(file, ".mjs") || strings.HasSuffix(file, ".cjs"):
			body, promise = nodeBody(source, name)
		}
		if body != "" {
			return file, digest(body), promise
		}
	}
	return "", "", ""
}
func first(s string) string    { return strings.TrimSpace(strings.Split(strings.TrimSpace(s), "\n")[0]) }
func indentation(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }

var pyDefinition = regexp.MustCompile(`^(?:async\s+)?def\s+([A-Za-z_][A-Za-z_0-9]*)\s*\(`)
var pyClass = regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z_0-9]*)`)

func pythonBody(source, test string, parents []string) (string, string) {
	lines := strings.SplitAfter(source, "\n")
	class := ""
	classIndent := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := indentation(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && indent <= classIndent {
			class = ""
			classIndent = -1
		}
		if m := pyClass.FindStringSubmatch(trimmed); m != nil {
			class = m[1]
			classIndent = indent
			continue
		}
		m := pyDefinition.FindStringSubmatch(trimmed)
		if m == nil || m[1] != test {
			continue
		}
		if len(parents) > 0 && class != parents[len(parents)-1] {
			continue
		}
		if len(parents) == 0 && class != "" {
			continue
		}
		end := i + 1
		for end < len(lines) {
			text := strings.TrimSpace(lines[end])
			if text != "" && indentation(lines[end]) <= indent {
				break
			}
			end++
		}
		start := pythonDecoratorStart(lines, i)
		if start < 0 {
			return "", ""
		}
		promise := commentPromise(lines[:start], "#")
		for j := i + 1; j < end; j++ {
			s := strings.TrimSpace(lines[j])
			if s == "" {
				continue
			}
			for _, quote := range []string{"\"\"\"", "'''"} {
				if strings.HasPrefix(s, quote) {
					doc := strings.TrimSpace(strings.Join(lines[j:end], ""))
					doc = strings.TrimPrefix(doc, quote)
					if close := strings.Index(doc, quote); close >= 0 {
						doc = doc[:close]
					}
					promise = first(doc)
				}
			}
			break
		}
		return strings.TrimRight(strings.Join(lines[start:end], ""), " \t\r\n"), promise
	}
	return "", ""
}

func pythonDecoratorStart(lines []string, def int) int {
	start := def
	stack := ""
	for i, line := range lines[:def] {
		text := strings.TrimSpace(line)
		if stack == "" {
			if text == "" || strings.HasPrefix(text, "#") {
				continue
			}
			if !strings.HasPrefix(text, "@") || indentation(line) != indentation(lines[def]) {
				start = def
				continue
			}
			if start == def {
				start = i
			}
		}
		for j := 0; j < len(line); j++ {
			c := line[j]
			if c == '#' {
				break
			}
			if c == '\'' || c == '"' {
				if strings.HasPrefix(line[j:], strings.Repeat(string(c), 3)) {
					return -1
				}
				end := quotedEnd(line, j)
				if end <= j+1 || line[end-1] != c {
					return -1
				}
				j = end - 1
				continue
			}
			if strings.ContainsRune("([{", rune(c)) {
				stack += string(c)
			} else if close := strings.IndexByte(")]}", c); close >= 0 {
				if len(stack) == 0 || stack[len(stack)-1] != "([{"[close] {
					return -1
				}
				stack = stack[:len(stack)-1]
			} else if c == '\\' {
				return -1
			}
		}
	}
	if stack != "" {
		return -1
	}
	return start
}

func commentPromise(lines []string, marker string) string {
	promise := ""
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, marker) {
			break
		}
		promise = strings.TrimSpace(strings.TrimPrefix(line, marker))
	}
	return promise
}

// quotedEnd also skips template literals, so delimiters in strings never balance a call.
func quotedEnd(s string, start int) int {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == quote {
			return i + 1
		}
	}
	return len(s)
}
func commentEnd(s string, i int) int {
	if strings.HasPrefix(s[i:], "//") {
		if n := strings.IndexByte(s[i:], '\n'); n >= 0 {
			return i + n
		}
		return len(s)
	}
	if strings.HasPrefix(s[i:], "/*") {
		if n := strings.Index(s[i+2:], "*/"); n >= 0 {
			return i + 2 + n + 2
		}
		return len(s)
	}
	return i
}
func nodeBody(source, name string) (string, string) {
	for i := 0; i < len(source); {
		if source[i] == '\'' || source[i] == '"' || source[i] == '`' {
			i = quotedEnd(source, i)
			continue
		}
		if end := commentEnd(source, i); end != i {
			i = end
			continue
		}
		start := i
		if !((source[i] >= 'a' && source[i] <= 'z') || (source[i] >= 'A' && source[i] <= 'Z') || source[i] == '_' || source[i] == '$') {
			i++
			continue
		}
		i++
		for i < len(source) && ((source[i] >= 'a' && source[i] <= 'z') || (source[i] >= 'A' && source[i] <= 'Z') || (source[i] >= '0' && source[i] <= '9') || source[i] == '_' || source[i] == '$') {
			i++
		}
		word := source[start:i]
		if word != "test" && word != "it" {
			continue
		}
		j := i
		for j < len(source) && strings.ContainsRune(" \t\r\n", rune(source[j])) {
			j++
		}
		if j >= len(source) || source[j] != '(' {
			continue
		}
		call := j
		j++
		for j < len(source) && strings.ContainsRune(" \t\r\n", rune(source[j])) {
			j++
		}
		if j >= len(source) || (source[j] != '\'' && source[j] != '"' && source[j] != '`') {
			continue
		}
		end := quotedEnd(source, j)
		literal := source[j+1 : end-1]
		var decoded strings.Builder
		for remaining := literal; remaining != ""; {
			value, _, tail, err := strconv.UnquoteChar(remaining, source[j])
			if err != nil {
				break
			}
			decoded.WriteRune(value)
			remaining = tail
		}
		if decoded.Len() > 0 {
			literal = decoded.String()
		}
		if literal != name {
			continue
		}
		depth := 0
		for k := call; k < len(source); k++ {
			if source[k] == '\'' || source[k] == '"' || source[k] == '`' {
				k = quotedEnd(source, k) - 1
				continue
			}
			if e := commentEnd(source, k); e != k {
				k = e - 1
				continue
			}
			if source[k] == '(' {
				depth++
			}
			if source[k] == ')' {
				depth--
				if depth == 0 {
					prefix := strings.TrimRight(source[:start], " \t")
					lines := strings.Split(prefix, "\n")
					promise := commentPromise(lines[:len(lines)-1], "//")
					return source[call : k+1], promise
				}
			}
		}
	}
	return "", ""
}
