// Package surface inventories, for the functions a change touches, every input they read and the
// sensitive sinks each input reaches, with the guard that stands between the two.
//
// The probe answers a question a reviewer has to answer anyway — which of these inputs, made hostile,
// does something dangerous? — deterministically, so a small model can start from a list instead of an
// intuition. It also says out loud how shallow that list is: the flow is traced inside one function and a
// guard is a check that mentions the input, not proof the check is correct.
package surface

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alesierraalta/tsp/internal/gitdiff"
)

// ErrUnusable marks a probe that measured no file it has an extractor for.
var ErrUnusable = errors.New("surface probe unusable")

// Heuristic is the sentence every report carries, in text and in JSON: the reader has to know how little
// the flow analysis claims before acting on a row.
const Heuristic = "data flow is intra-function; a guard is a check that mentions the input, not proof it is correct"

// The origins an input can have. A parameter is public or internal by the function's own name, because
// that is what decides who can call it; the rest name the door the value came through.
const (
	OriginPublic   = "public"
	OriginInternal = "internal"
	OriginHTTP     = "http"
	OriginCLI      = "cli"
	OriginEnv      = "env"
	OriginFile     = "file"
	OriginDecode   = "decode"
	OriginReader   = "reader"
)

// The fidelities a row can claim: how much the extractor saw of the function that produced it. A parser
// knows the shape of a call; a lexer only knows the tokens around it.
const (
	FidelityAST       = "ast"
	FidelityHeuristic = "heuristic"
)

// Row is one input reaching one sink, with an empty verdict for the reader to fill.
type Row struct {
	File         string `json:"file"`
	Function     string `json:"function"`
	FunctionLine int    `json:"function_line"`
	Input        string `json:"input"`
	Origin       string `json:"origin"`
	Sink         string `json:"sink"`
	SinkCall     string `json:"sink_call"`
	SinkLine     int    `json:"sink_line"`
	Guard        string `json:"guard"`
	Guarded      bool   `json:"guarded"`
	// GuardBody is the decisive expression of the guard's own body when the guard calls a function this
	// package or file defines, so a reader sees what the check does not inspect. It is one call deep.
	GuardBody string `json:"guard_body,omitempty"`
	Fidelity  string `json:"fidelity"`
}

// Report is what one probe measured.
type Report struct {
	Measured   []string `json:"measured,omitempty"`
	Skipped    []string `json:"skipped,omitempty"`
	NotChecked []string `json:"not_checked,omitempty"`
	Rows       []Row    `json:"rows,omitempty"`
	Heuristic  string   `json:"heuristic"`
	// Fidelity says, per measured language, how deeply the rows were read: a parser or a lexer. A reader who
	// knows the language's rows are heuristic spends a probe differently.
	Fidelity map[string]string `json:"fidelity,omitempty"`
}

// Blocking reports whether an unguarded input reaches a sink.
func (r Report) Blocking() bool { return r.Unguarded() > 0 }

// Unguarded counts the rows no guard stands in front of.
func (r Report) Unguarded() int {
	count := 0
	for _, row := range r.Rows {
		if !row.Guarded {
			count++
		}
	}
	return count
}

// Functions counts the functions the rows belong to, so the header can say how much of the change this is.
func (r Report) Functions() int {
	seen := map[string]bool{}
	for _, row := range r.Rows {
		seen[fmt.Sprintf("%s:%d:%s", row.File, row.FunctionLine, row.Function)] = true
	}
	return len(seen)
}

// Extractor reads one language's source into the functions the probe can see. A language that lands later
// — JavaScript, Python — is a new file that implements this interface and registers itself in init, so the
// scope, the report, the rows and the exit codes stay exactly as they are.
type Extractor interface {
	Language() string
	// Fidelity says how the language is read, so a row carries how much it can be trusted.
	Fidelity() string
	Extract(src Source, want Want) ([]Function, error)
}

// Source is one file the probe reads. The path is repository-relative, so a row points where a person opens
// the file.
type Source struct {
	Path    string
	Content []byte
	// Siblings are the other files of the same package, which is where a Go guard's body one call deep is
	// written. A language whose units are files instead of packages leaves it empty.
	Siblings []Source
}

// Want says which lines of the file the change occupies; the zero value wants every line, which is how a new
// file or an explicit --paths target is read.
type Want struct{ Ranges []Range }

// Range is one run of changed lines in the new version of a file.
type Range struct{ First, Last int }

// Any reports whether [first,last] meets the change.
func (w Want) Any(first, last int) bool {
	if len(w.Ranges) == 0 {
		return true
	}
	for _, r := range w.Ranges {
		if first <= r.Last && r.First <= last {
			return true
		}
	}
	return false
}

// Function is one function the probe inventoried.
type Function struct {
	File string
	Name string
	Line int
	Rows []Row
}

// extractors is the per-language registry. An implementation registers itself from init, so adding a
// language touches one new file and no shared one.
var extractors = map[string]Extractor{}

// register adds one language's extractor.
func register(extractor Extractor) { registerAs(extractor, extractor.Language()) }

// registerAs adds one extractor under a second language name: JavaScript and TypeScript are the same file
// with different extensions, and both are already named separately by languageOf.
func registerAs(extractor Extractor, language string) { extractors[language] = extractor }

// extractorFor returns the extractor for a language, and whether the probe can measure it at all.
func extractorFor(language string) (Extractor, bool) {
	extractor, ok := extractors[language]
	return extractor, ok
}

// Deps are the process boundaries, injected so the decisions are testable.
type Deps struct {
	Git      gitdiff.Runner
	ReadFile func(string) ([]byte, error)
}

// Options are one probe.
type Options struct {
	Cwd   string
	Base  string   // commit-ish the change is measured from; empty reads the working tree
	Paths []string // files or directories to inventory instead of the change
	Deps  Deps
}

// Run inventories the inputs and sinks of the functions the change touches, or of the named paths. The
// context is accepted to match the other analyzers' shape; nothing here blocks on anything but files.
func Run(ctx context.Context, opts Options) (Report, error) {
	d := withDefaults(opts)
	report := newReport()
	root, ranges, err := scope(opts, d)
	if err != nil {
		return report, err
	}
	if len(ranges) == 0 {
		return report, nil
	}
	rows, err := inventoryFiles(&report, root, ranges, d)
	if err != nil {
		return report, err
	}
	report.Rows = dedupe(rows)
	return report, nil
}

// withDefaults fills the collaborators the probe runs with.
func withDefaults(opts Options) Deps {
	d := opts.Deps
	if d.Git == nil {
		d.Git = gitdiff.Default
	}
	if d.ReadFile == nil {
		d.ReadFile = os.ReadFile
	}
	return d
}

// newReport is the honest floor every run reports: what this probe does not follow.
func newReport() Report {
	return Report{
		Heuristic: Heuristic,
		Fidelity:  map[string]string{},
		NotChecked: []string{
			"test files: the probe inventories shipped code, so *_test.go, *.test.js/*.spec.js and *_test.py/test_*.py functions are not measured",
			"interprocedural flow: an input passing through a helper, a struct field, a global or a reflection call is not followed, and two names holding one value are not aliased",
		},
	}
}

// scope answers which files the probe reads, and which lines of each: the hunks of the change plus every
// line of an untracked file, or every line of the paths the caller named.
func scope(opts Options, d Deps) (string, map[string][]Range, error) {
	if len(opts.Paths) > 0 {
		return pathScope(opts, d)
	}
	root, err := gitdiff.Root(opts.Cwd, d.Git)
	if err != nil {
		return "", nil, fmt.Errorf("%w: not a git repository, so there is no change to probe", ErrUnusable)
	}
	found, err := gitdiff.ChangedRanges(root, opts.Base, d.Git)
	if err != nil {
		return root, nil, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	ranges := make(map[string][]Range, len(found))
	for path, spans := range found {
		for _, span := range spans {
			ranges[filepath.ToSlash(path)] = append(ranges[filepath.ToSlash(path)], Range{First: span.First, Last: span.Last})
		}
	}
	return root, ranges, nil
}

// pathScope expands the paths the caller named: a file is read whole, a directory contributes every source
// file under it.
func pathScope(opts Options, d Deps) (string, map[string][]Range, error) {
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	root, err := gitdiff.Root(cwd, d.Git)
	if err != nil {
		root, err = filepath.Abs(cwd)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %v", ErrUnusable, err)
		}
	}
	ranges := map[string][]Range{}
	for _, named := range opts.Paths {
		full := named
		if !filepath.IsAbs(full) {
			full = filepath.Join(cwd, named)
		}
		// The report names files relative to the repository, so a path the caller wrote relative to their
		// shell has to be made absolute before it can be compared with the root.
		if absolute, err := filepath.Abs(full); err == nil {
			full = absolute
		}
		info, err := os.Stat(full)
		if err != nil {
			return "", nil, fmt.Errorf("%w: cannot read %s: %v", ErrUnusable, named, err)
		}
		if !info.IsDir() {
			if !isSource(named) {
				return "", nil, fmt.Errorf("%w: %s is not source the probe can read", ErrUnusable, named)
			}
			ranges[relative(root, full)] = nil
			continue
		}
		if err := collectDir(ranges, root, full); err != nil {
			return "", nil, fmt.Errorf("%w: %v", ErrUnusable, err)
		}
	}
	if len(ranges) == 0 {
		return "", nil, fmt.Errorf("%w: the paths named no source file", ErrUnusable)
	}
	return root, ranges, nil
}

// collectDir walks a directory the caller named, skipping the folders no reviewer means by "the source".
func collectDir(ranges map[string][]Range, root, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skippedDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSource(path) {
			return nil
		}
		ranges[relative(root, path)] = nil
		return nil
	})
}

// skippedDir names the folders a probe never reads.
func skippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".venv", "venv", ".idea", ".vscode":
		return true
	}
	return false
}

// isTestSource names the files the probe does not measure: the shipped code is what a caller can reach, and
// a fixture nobody ships cannot be made hostile from outside.
func isTestSource(path string) bool {
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	for _, suffix := range []string{".test.js", ".spec.js", ".test.jsx", ".spec.jsx", ".test.mjs", ".spec.mjs", ".test.cjs", ".spec.cjs", ".test.ts", ".spec.ts", ".test.tsx", ".spec.tsx", "_test.py"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")
}

// relative names a file the way the report prints it: repository-relative, slash-separated.
func relative(root, full string) string {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return filepath.ToSlash(full)
	}
	return filepath.ToSlash(rel)
}

// inventoryFiles reads every measurable file and returns the rows of the functions the change touches.
func inventoryFiles(report *Report, root string, ranges map[string][]Range, d Deps) ([]Row, error) {
	grouped := map[string][]string{}
	for _, rel := range sortedPaths(ranges) {
		language, ok := languageOf(rel)
		if !ok {
			report.Skipped = appendUnique(report.Skipped, "not source: "+rel)
			continue
		}
		grouped[language] = append(grouped[language], rel)
	}
	var rows []Row
	read := 0
	// A Go guard's body can live in a sibling file of the same package, so the package is read once per
	// directory instead of once per file.
	packages := map[string][]Source{}
	for _, language := range sortedKeys(grouped) {
		extractor, ok := extractorFor(language)
		if !ok {
			report.Skipped = appendUnique(report.Skipped, fmt.Sprintf("no extractor for %s (%d file(s))", language, len(grouped[language])))
			continue
		}
		report.Measured = append(report.Measured, language)
		report.Fidelity[language] = extractor.Fidelity()
		for _, rel := range grouped[language] {
			if isTestSource(rel) {
				report.Skipped = appendUnique(report.Skipped, "test file, not measured: "+rel)
				continue
			}
			body, err := d.ReadFile(filepath.Join(root, rel))
			if err != nil {
				report.Skipped = appendUnique(report.Skipped, "unreadable: "+rel)
				continue
			}
			source := Source{Path: rel, Content: body}
			if language == "go" {
				source.Siblings = packageFiles(packages, root, rel, d)
			}
			functions, err := extractor.Extract(source, Want{Ranges: ranges[rel]})
			if err != nil {
				report.Skipped = appendUnique(report.Skipped, "cannot parse: "+rel)
				continue
			}
			read++
			for _, fn := range functions {
				for _, row := range fn.Rows {
					row.Fidelity = extractor.Fidelity()
					rows = append(rows, row)
				}
			}
		}
	}
	if read == 0 {
		return nil, fmt.Errorf("%w: nothing in this change was measured (%s)", ErrUnusable, strings.Join(report.Skipped, "; "))
	}
	return rows, nil
}

// packageFiles answers the other Go files of rel's package, read once per directory. A file the probe cannot
// read is left out instead of failing the report: the rows of the file it was asked about are still worth
// having, and the guard then shows what it is called without its body.
func packageFiles(cache map[string][]Source, root, rel string, d Deps) []Source {
	dir := filepath.Dir(rel)
	if sources, seen := cache[dir]; seen {
		return sources
	}
	var sources []Source
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || isTestSource(name) {
				continue
			}
			path := filepath.ToSlash(filepath.Join(dir, name))
			if path == rel {
				continue
			}
			body, err := d.ReadFile(filepath.Join(root, path))
			if err != nil {
				continue
			}
			sources = append(sources, Source{Path: path, Content: body})
		}
	}
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	cache[dir] = sources
	return sources
}

// dedupe drops the rows one call produced twice and orders the rest so two runs print the same report,
// unguarded first: the reader fixes what is broken before reading what is promised.
func dedupe(rows []Row) []Row {
	seen := map[string]bool{}
	kept := make([]Row, 0, len(rows))
	for _, row := range rows {
		key := fmt.Sprintf("%s|%d|%s|%s|%s|%d|%s", row.File, row.FunctionLine, row.Input, row.Origin, row.Sink, row.SinkLine, row.Guard)
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, row)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.Guarded != b.Guarded {
			return !a.Guarded
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.FunctionLine != b.FunctionLine {
			return a.FunctionLine < b.FunctionLine
		}
		if a.SinkLine != b.SinkLine {
			return a.SinkLine < b.SinkLine
		}
		if a.Sink != b.Sink {
			return a.Sink < b.Sink
		}
		return a.Input < b.Input
	})
	return kept
}

// sortedPaths lists the scoped files in a stable order.
func sortedPaths(ranges map[string][]Range) []string {
	paths := make([]string, 0, len(ranges))
	for rel := range ranges {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

// sortedKeys lists a grouped map's keys in a stable order.
func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

// isSource reports whether a path is source at all.
func isSource(p string) bool {
	_, ok := languageOf(p)
	return ok
}

// languageOf names the language of a path and whether it is source at all. A language with no extractor is
// still source: naming it as unmeasured is the point.
func languageOf(p string) (string, bool) {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".go":
		return "go", true
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript", true
	case ".ts", ".tsx":
		return "typescript", true
	case ".py":
		return "python", true
	case ".rb":
		return "ruby", true
	case ".rs":
		return "rust", true
	case ".java":
		return "java", true
	case ".c", ".h", ".cc", ".cpp", ".hpp":
		return "c/c++", true
	case ".sh", ".bash":
		return "shell", true
	case ".md", ".json", ".yml", ".yaml", ".toml", ".txt", ".lock", ".mod", ".sum", "":
		return "", false
	}
	return "unclassified", true
}
