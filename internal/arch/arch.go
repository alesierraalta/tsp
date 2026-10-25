// Package arch answers one question about a change: does it add an import edge the repository
// declared it never wants? The declaration is data in the repository, so the rule is reviewable and
// the answer is the same for every session and every machine.
//
// Cycles are deliberately absent from the Go path: the toolchain already refuses an import cycle, so
// a cycle a build accepts cannot exist and reporting one would be an opinion, not a measurement.
package arch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alesierraalta/tsp/internal/gitdiff"
)

// ErrUnusable marks a probe that measured nothing: no declaration, no changed Go file, no repository.
var ErrUnusable = errors.New("architecture probe unusable")

// RuleForbiddenEdge is the only rule this probe enforces today.
const RuleForbiddenEdge = "forbidden-edge"

// RulesName is the declaration the probe reads when the caller names none.
const RulesName = ".tsp-arch.json"

// Layer is one group of packages, matched by repository path prefix or by import prefix.
type Layer struct {
	Name    string   `json:"name"`
	Paths   []string `json:"paths,omitempty"`
	Imports []string `json:"imports,omitempty"`
}

// Forbid names an edge the repository refuses: a package in From must never import a package in To.
type Forbid struct {
	From string `json:"from"`
	To   string `json:"to"`
	Why  string `json:"why,omitempty"`
}

// Declaration is the whole rule set a repository publishes.
type Declaration struct {
	Layers []Layer  `json:"layers"`
	Forbid []Forbid `json:"forbid,omitempty"`
}

// Finding is one edge the change added.
type Finding struct {
	Rule    string `json:"rule"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Report is what the probe measured and what it could not.
type Report struct {
	Layers     []string  `json:"layers,omitempty"`
	Measured   []string  `json:"measured,omitempty"`
	Findings   []Finding `json:"findings,omitempty"`
	NotChecked []string  `json:"not_checked,omitempty"`
}

// Blocking reports whether the change broke a declared rule.
func (r Report) Blocking() bool { return len(r.Findings) > 0 }

// Deps are the process boundaries, injected so the decisions are testable.
type Deps struct {
	Git      gitdiff.Runner
	ReadFile func(string) ([]byte, error)
}

// Options are one probe.
type Options struct {
	Cwd       string
	Base      string
	RulesPath string
	Deps      Deps
}

// DefaultNotChecked names what this probe does not measure, so a clean report cannot read as a
// stronger claim than it is.
var DefaultNotChecked = []string{
	"cycles: the Go toolchain refuses an import cycle, so a cycle a build accepts cannot exist; a JavaScript or TypeScript cycle is not reported here",
	"layers: a package that matches no declared layer is not judged",
}

// Load reads and validates a declaration. A rule that names a layer nobody declared is a typo, and a
// typo must not silently disable a rule.
func Load(path string, read func(string) ([]byte, error)) (Declaration, error) {
	if read == nil {
		read = os.ReadFile
	}
	body, err := read(path)
	if err != nil {
		return Declaration{}, fmt.Errorf("%w: no architecture rules at %s: write one, or pass --rules", ErrUnusable, path)
	}
	declared, err := decodeDeclaration(path, body)
	if err != nil {
		return Declaration{}, err
	}
	if err := validateDeclaration(path, declared); err != nil {
		return Declaration{}, err
	}
	return declared, nil
}

// decodeDeclaration reads the file into a declaration and refuses one that enforces nothing.
func decodeDeclaration(path string, body []byte) (Declaration, error) {
	var declared Declaration
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declared); err != nil {
		return Declaration{}, fmt.Errorf("%w: %s is not a valid declaration: %v", ErrUnusable, path, err)
	}
	if len(declared.Layers) == 0 {
		return Declaration{}, fmt.Errorf("%w: %s declares no layer, so no rule can be enforced", ErrUnusable, path)
	}
	return declared, nil
}

// validateDeclaration refuses a rule naming a layer that does not exist: a typo there would
// silently enforce nothing.
func validateDeclaration(path string, declared Declaration) error {
	known, err := declaredLayerNames(path, declared.Layers)
	if err != nil {
		return err
	}
	for _, forbid := range declared.Forbid {
		for _, side := range []string{forbid.From, forbid.To} {
			if !known[side] {
				return fmt.Errorf("%w: %s forbids an edge involving %q, which no layer declares", ErrUnusable, path, side)
			}
		}
	}
	return nil
}

// declaredLayerNames collects the declared layer names, refusing an unnamed or repeated one.
func declaredLayerNames(path string, layers []Layer) (map[string]bool, error) {
	known := make(map[string]bool, len(layers))
	for _, layer := range layers {
		if strings.TrimSpace(layer.Name) == "" {
			return nil, fmt.Errorf("%w: %s has a layer with no name", ErrUnusable, path)
		}
		if known[layer.Name] {
			return nil, fmt.Errorf("%w: %s declares the layer %q twice", ErrUnusable, path, layer.Name)
		}
		known[layer.Name] = true
	}
	return known, nil
}

// Run reports the declared edges the change added.
func Run(ctx context.Context, opts Options) (Report, error) {
	d := opts.Deps
	if d.Git == nil {
		d.Git = gitdiff.Default
	}
	if d.ReadFile == nil {
		d.ReadFile = os.ReadFile
	}
	report := Report{NotChecked: append([]string(nil), DefaultNotChecked...)}
	root, err := gitdiff.Root(opts.Cwd, d.Git)
	if err != nil {
		return report, fmt.Errorf("%w: not a git repository, so there is no change to probe", ErrUnusable)
	}
	rulesPath := opts.RulesPath
	if rulesPath == "" {
		rulesPath = filepath.Join(root, RulesName)
	}
	declared, err := Load(rulesPath, d.ReadFile)
	if err != nil {
		return report, err
	}
	report.Layers = layerNames(declared)
	ranges, err := gitdiff.ChangedRanges(root, opts.Base, d.Git)
	if err != nil {
		return report, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	if len(ranges) == 0 {
		return report, nil
	}
	files := changedGoFiles(ranges)
	if len(files) == 0 {
		return report, fmt.Errorf("%w: the change touched no Go file, so no import edge was judged", ErrUnusable)
	}
	module := modulePath(root, d)
	findings, err := edges(root, files, ranges, declared, module, d)
	if err != nil {
		return report, err
	}
	report.Findings = findings
	report.Measured = []string{"go"}
	return report, nil
}

// edges reads each changed file's imports and reports the ones a declared rule refuses.
func edges(root string, files []string, ranges map[string][]gitdiff.Range, declared Declaration, module string, d Deps) ([]Finding, error) {
	var findings []Finding
	for _, rel := range files {
		findings = append(findings, fileEdges(root, rel, ranges, declared, module, d)...)
	}
	sortFindings(findings)
	return findings, nil
}

// fileEdges reports the refused import edges of one changed file.
func fileEdges(root, rel string, ranges map[string][]gitdiff.Range, declared Declaration, module string, d Deps) []Finding {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	body, err := d.ReadFile(abs)
	if err != nil {
		return nil
	}
	from := layerOfPath(declared, rel)
	if from == "" {
		return nil
	}
	if jsExtensions[strings.ToLower(path.Ext(rel))] {
		return jsEdges(rel, body, ranges[rel], declared, from)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, abs, body, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	var findings []Finding
	for _, spec := range file.Imports {
		if finding, found := refusedEdge(rel, spec, fset, ranges[rel], declared, module, from); found {
			findings = append(findings, finding)
		}
	}
	return findings
}

// refusedEdge judges one import spec: owned by this module, on a changed line, between two
// declared layers, and refused by a rule.
func refusedEdge(rel string, spec *ast.ImportSpec, fset *token.FileSet, ranges []gitdiff.Range, declared Declaration, module, from string) (Finding, bool) {
	importPath := strings.Trim(spec.Path.Value, `"`)
	if module != "" && !ownedBy(importPath, module) {
		return Finding{}, false
	}
	line := fset.Position(spec.Pos()).Line
	if !inRange(ranges, line) {
		return Finding{}, false
	}
	to := layerOfImport(declared, importPath)
	if to == "" {
		return Finding{}, false
	}
	rule, refused := forbidden(declared, from, to)
	if !refused {
		return Finding{}, false
	}
	return Finding{Rule: RuleForbiddenEdge, File: rel, Line: line, Message: refusal(from, to, importPath, rule)}, true
}

// jsEdges judges the imports of one JavaScript or TypeScript file the same way a Go file is judged:
// a specifier on a changed line that lands in a refused layer is a finding.
func jsEdges(rel string, body []byte, ranges []gitdiff.Range, declared Declaration, from string) []Finding {
	var findings []Finding
	for _, spec := range jsImports(body) {
		if !inRange(ranges, spec.Line) {
			continue
		}
		importPath, to := jsLayer(declared, rel, spec.Path)
		if to == "" {
			continue
		}
		rule, refused := forbidden(declared, from, to)
		if !refused {
			continue
		}
		findings = append(findings, Finding{Rule: RuleForbiddenEdge, File: rel, Line: spec.Line, Message: refusal(from, to, importPath, rule)})
	}
	return findings
}

// sortFindings orders findings by file then line so a report reads the same twice.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}

// refusal states the edge and the repository's own reason for refusing it.
func refusal(from, to, importPath string, rule Forbid) string {
	message := fmt.Sprintf("%s must not import %s (%s)", from, to, importPath)
	if why := strings.TrimSpace(rule.Why); why != "" {
		message += ": " + why
	}
	return message
}

// ownedBy reports whether an import path belongs to this module.
func ownedBy(importPath, module string) bool {
	return importPath == module || strings.HasPrefix(importPath, module+"/")
}

// inRange reports whether a line falls inside one of the changed hunks, so an import that was already
// there before the change is not blamed on it.
func inRange(ranges []gitdiff.Range, line int) bool {
	for _, r := range ranges {
		if line >= r.First && line <= r.Last {
			return true
		}
	}
	return false
}

// layerNames lists the declared layers in declaration order.
func layerNames(declared Declaration) []string {
	names := make([]string, 0, len(declared.Layers))
	for _, layer := range declared.Layers {
		names = append(names, layer.Name)
	}
	return names
}

// changedGoFiles keeps the changed Go production files, sorted.
func changedGoFiles(ranges map[string][]gitdiff.Range) []string {
	files := make([]string, 0, len(ranges))
	for changed := range ranges {
		slash := filepath.ToSlash(changed)
		if strings.HasSuffix(slash, "_test.go") || strings.HasSuffix(slash, ".test.ts") || strings.HasSuffix(slash, ".test.tsx") {
			continue
		}
		if strings.HasSuffix(slash, ".go") || jsExtensions[strings.ToLower(path.Ext(slash))] {
			files = append(files, slash)
		}
	}
	sort.Strings(files)
	return files
}

// modulePath reads the module name from go.mod; without one, own imports cannot be told from outside.
func modulePath(root string, d Deps) string {
	body, err := d.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

// layerOfPath names the layer a repository path belongs to, or empty when none matches.
func layerOfPath(declared Declaration, path string) string {
	path = filepath.ToSlash(path)
	for _, layer := range declared.Layers {
		for _, prefix := range layer.Paths {
			if strings.HasPrefix(path, filepath.ToSlash(prefix)) {
				return layer.Name
			}
		}
	}
	return ""
}

// layerOfImport names the layer an import path belongs to, or empty when none matches.
func layerOfImport(declared Declaration, importPath string) string {
	for _, layer := range declared.Layers {
		for _, prefix := range layer.Imports {
			if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
				return layer.Name
			}
		}
	}
	return ""
}

// forbidden reports the rule that refuses this edge, if the repository declared one.
func forbidden(declared Declaration, from, to string) (Forbid, bool) {
	for _, rule := range declared.Forbid {
		if rule.From == from && rule.To == to {
			return rule, true
		}
	}
	return Forbid{}, false
}
