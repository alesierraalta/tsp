// Package probe provides read-only configuration probes.
package probe

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Finding struct {
	Name        string   `json:"name"`
	ReadAt      []string `json:"read_at"`
	Siblings    []string `json:"siblings"`
	DeliveredBy []string `json:"delivered_by"`
}

// The os. prefix is optional so `from os import getenv` / `from os import environ` reads count too.
var reads = regexp.MustCompile(`(?:(?:\bos\s*\.\s*)?\b(?:getenv|Getenv|LookupEnv)\s*\(\s*["']([A-Z][A-Z0-9_]*)["']|(?:\bos\s*\.\s*)?\benviron\s*(?:\[\s*["']([A-Z][A-Z0-9_]*)["']\s*\]|\.\s*get\s*\(\s*["']([A-Z][A-Z0-9_]*)["'])|\bprocess\s*\.\s*env\s*(?:\.\s*([A-Z][A-Z0-9_]*)\b|\[\s*["']([A-Z][A-Z0-9_]*)["']\s*\]))`)
var environmentReference = regexp.MustCompile(`\b(?:process\s*\.\s*env|os\s*\.\s*environ|from\s+os\s+import\s+[^\n]*\b(?:environ|getenv)\b)`)
var settingLiteral = regexp.MustCompile(`"([A-Z][A-Z0-9]*_[A-Z0-9_]+)"|'([A-Z][A-Z0-9]*_[A-Z0-9_]+)'`)
var names = regexp.MustCompile(`\b[A-Z][A-Z0-9_]*\b`)

func ignoredDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".venv", "venv", "tests", ".env":
		return true
	}
	return false
}
func artifact(p string) bool {
	parts := strings.Split(p, "/")
	for i, s := range parts[:len(parts)-1] {
		if s == "deploy" || s == "infra" || s == "k8s" || s == "helm" {
			return true
		}
		if s == ".github" && i+1 < len(parts)-1 && parts[i+1] == "workflows" {
			return true
		}
	}
	b := filepath.Base(p)
	return strings.HasPrefix(b, "Dockerfile") || strings.HasPrefix(b, "docker-compose") || strings.HasSuffix(b, ".tf") || b == "Procfile" || b == ".env.example" || b == ".env.sample" || b == ".env.template"
}
func source(p string) bool {
	b := filepath.Base(p)
	if strings.HasPrefix(b, "test_") && strings.HasSuffix(b, ".py") || strings.HasSuffix(b, "_test.go") || strings.HasSuffix(b, ".test.js") {
		return false
	}
	switch filepath.Ext(b) {
	case ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".go":
		return true
	}
	return false
}
func sorted(m map[string]bool) []string {
	a := make([]string, 0, len(m))
	for s := range m {
		a = append(a, s)
	}
	sort.Strings(a)
	return a
}
func prefix(s string) string { return strings.SplitN(s, "_", 2)[0] }

// Env reports literal reads with deployed siblings but no deployed mention of their own.
// Paths are relative to dir. Symlinks are never followed and .env is never opened.
func Env(dir string) ([]Finding, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}
	locations := map[string]map[string]bool{}
	delivered := map[string]map[string]bool{}
	err = filepath.WalkDir(dir, func(p string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// A directory the probe may not read (a root-owned data volume) holds no settings it could
			// report; skipping it keeps the findings of the rest of the tree.
			if errors.Is(walkErr, fs.ErrPermission) {
				if e != nil && e.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		if e.IsDir() {
			if ignoredDir(e.Name()) {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "pyvenv.cfg")); err == nil || errors.Is(err, fs.ErrPermission) {
				return filepath.SkipDir
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		if e.Type()&os.ModeSymlink != 0 || e.Name() == ".env" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		deploy, src := artifact(rel), source(rel)
		if !deploy && !src {
			return nil
		}
		data, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrPermission) {
			return nil
		}
		if err != nil {
			return err
		}
		if deploy {
			for _, n := range names.FindAllString(string(data), -1) {
				if delivered[n] == nil {
					delivered[n] = map[string]bool{}
				}
				delivered[n][rel] = true
			}
		}
		if src {
			matches := reads.FindAllSubmatchIndex(data, -1)
			if len(matches) > 0 || environmentReference.Match(data) {
				matches = append(matches, settingLiteral.FindAllSubmatchIndex(data, -1)...)
			}
			for _, m := range matches {
				n := ""
				for i := 2; i < len(m); i += 2 {
					if m[i] >= 0 {
						n = string(data[m[i]:m[i+1]])
						break
					}
				}
				if locations[n] == nil {
					locations[n] = map[string]bool{}
				}
				line := 1 + strings.Count(string(data[:m[0]]), "\n")
				locations[n][fmt.Sprintf("%s:%d", rel, line)] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]Finding, 0)
	for n, at := range locations {
		if len(delivered[n]) > 0 {
			continue
		}
		siblings, paths := map[string]bool{}, map[string]bool{}
		for other, ps := range delivered {
			// A sibling is a deployed setting (PREFIX_...), not any word that shares the prefix: a Dockerfile
			// instruction (RUN) or a comment word (API) delivers nothing.
			if other != n && strings.HasPrefix(other, prefix(n)+"_") {
				siblings[other] = true
				for p := range ps {
					paths[p] = true
				}
			}
		}
		if len(siblings) > 0 {
			result = append(result, Finding{n, sorted(at), sorted(siblings), sorted(paths)})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
