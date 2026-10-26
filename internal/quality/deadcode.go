package quality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// deadCodeNote is the honest scope: this rule judges one narrow shape, because the wide shape needs a
// whole-program reference graph and would report every symbol that is exported for callers we cannot see.
const deadCodeNote = "dead code: unexported top-level functions, types, variables and constants only; a method, an exported symbol or a reference from outside this repository is not judged"

// deadCodeFindings reports a declaration the change added that nothing in the repository mentions.
// It reads the changed declarations and every file's references, so a helper used from another package
// is live and a helper used only by itself is not.
func deadCodeFindings(root string, goFiles []string, d Deps) []Finding {
	if len(goFiles) == 0 {
		return nil
	}
	changed := map[string]bool{}
	for _, rel := range goFiles {
		changed[filepath.ToSlash(rel)] = true
	}
	declared, used := collectDeclarations(root, changed, d)
	var findings []Finding
	for name, decl := range declared {
		if used[name] > decl.self {
			continue
		}
		findings = append(findings, Finding{
			Rule:    "dead code",
			File:    decl.file,
			Line:    decl.line,
			Message: name + " is declared here and referenced nowhere in this repository: remove it or use it",
		})
	}
	sortFindings(findings)
	return findings
}

// declaration is one candidate: where it lives and how many of its references are its own name.
type declaration struct {
	file string
	line int
	self int
}

// recordDeclarations keeps the unexported top-level declarations of the changed files. Methods are left
// out on purpose: they satisfy interfaces, so "nobody calls it" says nothing about them.
func recordDeclarations(fset *token.FileSet, file *ast.File, rel string, isTest bool, changed map[string]bool, decls map[string]*declaration) {
	if isTest || !changed[rel] {
		return
	}
	for _, node := range file.Decls {
		switch node := node.(type) {
		case *ast.FuncDecl:
			recordFunc(fset, node, rel, decls)
		case *ast.GenDecl:
			recordGenDecl(fset, node, rel, decls)
		}
	}
}

// skipDeclaration names what must never be reported: entry points the runtime calls and the hooks the
// go tool calls.
func skipDeclaration(name string) bool {
	switch name {
	case "main", "init":
		return true
	}
	for _, prefix := range []string{"Test", "Benchmark", "Example", "Fuzz"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// countReferences counts every identifier the file mentions, which is what makes a declaration live.
func countReferences(fset *token.FileSet, file *ast.File, used map[string]int) {
	self := declarationPositions(file)
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok || self[ident.Pos()] {
			return true
		}
		used[ident.Name]++
		return true
	})
	_ = fset
}

// declarationPositions marks the identifiers that name a declaration, so their own occurrence is not
// mistaken for a reference.
func declarationPositions(file *ast.File) map[token.Pos]bool {
	self := map[token.Pos]bool{}
	for _, node := range file.Decls {
		switch node := node.(type) {
		case *ast.FuncDecl:
			self[node.Name.Pos()] = true
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				specPositions(spec, self)
			}
		}
	}
	return self
}

// specPositions marks the names one type or value specification declares.
func specPositions(spec ast.Spec, self map[token.Pos]bool) {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		self[spec.Name.Pos()] = true
	case *ast.ValueSpec:
		for _, name := range spec.Names {
			self[name.Pos()] = true
		}
	}
}

// collectDeclarations walks the repository once: what the changed files declare, and what every file
// mentions. One pass keeps the rule cheap enough to run on every change.
func collectDeclarations(root string, changed map[string]bool, d Deps) (map[string]*declaration, map[string]int) {
	fset := token.NewFileSet()
	declared := map[string]*declaration{}
	used := map[string]int{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if entry != nil && entry.IsDir() && skippedDir(entry.Name()) {
			return fs.SkipDir
		}
		rel, file, ok := parseRepoFile(root, path, entry, err, fset, d)
		if !ok {
			return nil
		}
		recordDeclarations(fset, file, rel, strings.HasSuffix(rel, "_test.go"), changed, declared)
		countReferences(fset, file, used)
		return nil
	})
	return declared, used
}

// recordFunc keeps one top-level function the change added, unless the runtime or the go tool calls it.
func recordFunc(fset *token.FileSet, node *ast.FuncDecl, rel string, decls map[string]*declaration) {
	if node.Recv != nil {
		return
	}
	addDeclaration(decls, node.Name.Name, rel, fset.Position(node.Name.Pos()).Line)
}

// recordGenDecl keeps the types, variables and constants a declaration adds.
func recordGenDecl(fset *token.FileSet, node *ast.GenDecl, rel string, decls map[string]*declaration) {
	for _, spec := range node.Specs {
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			addDeclaration(decls, spec.Name.Name, rel, fset.Position(spec.Name.Pos()).Line)
		case *ast.ValueSpec:
			for _, name := range spec.Names {
				addDeclaration(decls, name.Name, rel, fset.Position(name.Pos()).Line)
			}
		}
	}
}

// addDeclaration records one candidate, refusing what the rule cannot judge: exported symbols and the
// names the runtime calls.
func addDeclaration(decls map[string]*declaration, name, rel string, line int) {
	if name == "" || name == "_" || ast.IsExported(name) || skipDeclaration(name) {
		return
	}
	if _, seen := decls[name]; seen {
		return
	}
	decls[name] = &declaration{file: rel, line: line}
}

// parseRepoFile reads one path if it is a Go file the rule looks at, and answers with its repository
// relative name. A directory the rule skips answers false, which the walker turns into a skip.
func parseRepoFile(root, path string, entry fs.DirEntry, walkErr error, fset *token.FileSet, d Deps) (string, *ast.File, bool) {
	if walkErr != nil {
		return "", nil, false
	}
	if entry.IsDir() {
		return "", nil, false
	}
	if !strings.HasSuffix(path, ".go") {
		return "", nil, false
	}
	body, readErr := d.ReadFile(path)
	if readErr != nil {
		return "", nil, false
	}
	file, parseErr := parser.ParseFile(fset, path, body, parser.SkipObjectResolution)
	if parseErr != nil {
		return "", nil, false
	}
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		return "", nil, false
	}
	return filepath.ToSlash(rel), file, true
}

// skippedDir names the trees a reference count must not read: installed packages, vendored code and
// fixtures are not the repository's own references.
func skippedDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "testdata":
		return true
	}
	return false
}
