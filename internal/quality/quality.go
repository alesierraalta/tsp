// Package quality probes the lines a change touches for the code hygiene a reviewer would have to point
// out by hand: functions over the complexity bar, and blocks copied inside the change.
//
// Two rules make the probe usable twice. It measures only the lines the change touches, because a
// five-year-old repository returns thousands of findings nobody acts on; and it names the languages it
// could not measure instead of passing them over, because silence there reads as clean.
package quality

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alesierraalta/tsp/internal/gitdiff"
)

// ErrUnusable marks a probe that measured no language the change touched.
var ErrUnusable = errors.New("quality probe unusable")

// The rules this package reports.
const (
	RuleComplexity  = "complexity"
	RuleDuplication = "duplication"
)

// Finding is one hygiene problem, located where a person can open it.
type Finding struct {
	Rule    string `json:"rule"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Report is what one probe measured.
type Report struct {
	Measured   []string  `json:"measured,omitempty"`
	Skipped    []string  `json:"skipped,omitempty"`
	NotChecked []string  `json:"not_checked,omitempty"`
	Findings   []Finding `json:"findings,omitempty"`
}

// Blocking reports whether the probe found anything worth fixing.
func (r Report) Blocking() bool { return len(r.Findings) > 0 }

// Deps are the process boundaries, injected so the decisions are testable.
type Deps struct {
	Git      gitdiff.Runner
	ReadFile func(string) ([]byte, error)
}

// Options are one probe.
type Options struct {
	Cwd           string
	Base          string // commit-ish the change is measured from; empty reads the working tree
	MaxComplexity int    // functions scoring over this are reported; 0 means the default of 15
	DupWindow     int    // identical normalized lines that make a block; 0 means the default of 6
	Deps          Deps
}

// Run probes the change and reports what it found, and what it could not look at.
func Run(ctx context.Context, opts Options) (Report, error) {
	d, limits := withDefaults(opts)
	report := newReport()
	root, err := gitdiff.Root(opts.Cwd, d.Git)
	if err != nil {
		return report, fmt.Errorf("%w: not a git repository: nothing to probe", ErrUnusable)
	}
	ranges, err := changedLines(root, opts.Base, d)
	if err != nil {
		return report, err
	}
	if len(ranges) == 0 {
		return report, nil
	}
	changed := changedPaths(ranges)
	source := classify(&report, changed)
	goFiles, err := measurable(&report, source)
	if err != nil {
		return report, err
	}
	report.Findings = append(report.Findings, complexityFindings(root, goFiles, ranges, limits.MaxComplexity, d)...)
	dupes, err := duplicationFindings(root, changed, ranges, limits.DupWindow, d)
	if err != nil {
		return report, err
	}
	report.Findings = append(report.Findings, dupes...)
	sortFindings(report.Findings)
	return report, nil
}

// withDefaults fills the collaborators and the limits the probe runs with.
func withDefaults(opts Options) (Deps, Options) {
	d := opts.Deps
	if d.Git == nil {
		d.Git = gitdiff.Default
	}
	if d.ReadFile == nil {
		d.ReadFile = os.ReadFile
	}
	if opts.MaxComplexity <= 0 {
		opts.MaxComplexity = 15
	}
	if opts.DupWindow <= 0 {
		opts.DupWindow = 6
	}
	return d, opts
}

// changedPaths lists the files the change touched, in a stable order.
func changedPaths(ranges map[string][]lineRange) []string {
	paths := make([]string, 0, len(ranges))
	for rel := range ranges {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

// newReport is the honest floor every run reports: the rules this probe does not implement.
func newReport() Report {
	return Report{NotChecked: []string{
		"dead code: counting references over the whole program is not implemented here",
		"test files: the bar governs shipped code, so *_test.go helpers are not measured",
	}}
}

// classify groups the changed paths by language and records what is not source at all.
func classify(report *Report, changed []string) map[string][]string {
	source := map[string][]string{}
	for _, p := range changed {
		language, isSource := languageOf(p)
		if !isSource {
			report.Skipped = appendUnique(report.Skipped, "not source: "+filepath.ToSlash(p))
			continue
		}
		source[language] = append(source[language], p)
	}
	return source
}

// measurable returns the Go files to measure and refuses to report clean when no language had an adapter.
func measurable(report *Report, source map[string][]string) ([]string, error) {
	var goFiles []string
	for _, language := range sortedKeys(source) {
		if language != "go" {
			report.Skipped = appendUnique(report.Skipped, "no adapter for "+language+" ("+fmt.Sprint(len(source[language]))+" file(s))")
			continue
		}
		report.Measured = append(report.Measured, language)
		goFiles = source[language]
	}
	if len(report.Measured) == 0 {
		return nil, fmt.Errorf("%w: the change touched no language this probe measures (%s)", ErrUnusable, strings.Join(report.Skipped, "; "))
	}
	return goFiles, nil
}

// sortFindings orders the report so two runs on the same change print the same lines.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
}

// complexityFindings scores the Go functions the change touches and reports the ones over the bar.
func complexityFindings(root string, files []string, ranges map[string][]lineRange, bar int, d Deps) []Finding {
	var findings []Finding
	for _, rel := range files {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		findings = append(findings, fileComplexity(root, rel, ranges[filepath.ToSlash(rel)], bar, d)...)
	}
	return findings
}

// fileComplexity scores one parsed file, keeping the functions whose lines meet the change.
func fileComplexity(root, rel string, touched []lineRange, bar int, d Deps) []Finding {
	fset := token.NewFileSet()
	body, err := d.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil
	}
	file, err := parser.ParseFile(fset, filepath.Join(root, rel), body, 0)
	if err != nil {
		return nil
	}
	whole := wholeFile(touched)
	var findings []Finding
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		if !whole && !intersects(touched, start, end) {
			continue
		}
		if score := Score(fn); score > bar {
			findings = append(findings, Finding{
				Rule:    RuleComplexity,
				File:    filepath.ToSlash(rel),
				Line:    start,
				Message: fmt.Sprintf("%s scores %d over the bar of %d: split it or name why it stays", fn.Name.Name, score, bar),
			})
		}
	}
	return findings
}

// duplicationFindings compares normalized windows inside the change: a block copied twice in changed
// code is a review comment the author can still act on. Overlapping windows of one copy are not a
// duplicate, so a pair only counts when the two occurrences do not overlap, and one copy is reported
// once even as its windows slide.
func duplicationFindings(root string, files []string, ranges map[string][]lineRange, window int, d Deps) ([]Finding, error) {
	blocks := collectBlocks(root, files, ranges, window, d)
	candidates := duplicatePairs(blocks, window)
	return dropSlidingCopies(candidates, window), nil
}

// occurrence is one place a block of lines appears, and whether the change touches it.
type occurrence struct {
	file    string
	line    int
	touched bool
}

// collectBlocks indexes every window of every changed file by its normalized content. Occurrences
// outside the change are kept too, so a copy inside the change is still caught.
func collectBlocks(root string, files []string, ranges map[string][]lineRange, window int, d Deps) map[string][]occurrence {
	blocks := map[string][]occurrence{}
	for _, rel := range files {
		body, err := d.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		lines := strings.Split(string(body), "\n")
		normalized := make([]string, len(lines))
		for i, line := range lines {
			normalized[i] = normalizeLine(line)
		}
		whole := wholeFile(ranges[filepath.ToSlash(rel)])
		touched := ranges[filepath.ToSlash(rel)]
		for i := 0; i+window <= len(lines); i++ {
			key, ok := windowSignature(normalized, i, window)
			if !ok {
				continue
			}
			blocks[key] = append(blocks[key], occurrence{
				file:    filepath.ToSlash(rel),
				line:    i + 1,
				touched: whole || intersects(touched, i+1, i+window),
			})
		}
	}
	return blocks
}

// duplicatePairs pairs two non-overlapping occurrences of the same block in one file: a window pair that
// overlaps is the same copy seen through a sliding window, not a second copy.
func duplicatePairs(blocks map[string][]occurrence, window int) []Finding {
	var findings []Finding
	for _, occurrences := range blocks {
		if len(occurrences) < 2 {
			continue
		}
		sort.Slice(occurrences, func(i, j int) bool {
			if occurrences[i].file != occurrences[j].file {
				return occurrences[i].file < occurrences[j].file
			}
			return occurrences[i].line < occurrences[j].line
		})
		findings = append(findings, firstPairPerCopy(occurrences, window)...)
	}
	return findings
}

// firstPairPerCopy reports each copy once, pointing at the earliest earlier occurrence in the same file.
// The copy that receives the finding has to sit inside the change: the author can only act on lines they
// are touching, and a pre-existing pair is not this change's problem.
func firstPairPerCopy(occurrences []occurrence, window int) []Finding {
	var findings []Finding
	for i := 1; i < len(occurrences); i++ {
		if !occurrences[i].touched {
			continue
		}
		for j := 0; j < i; j++ {
			if occurrences[i].file != occurrences[j].file {
				continue
			}
			if occurrences[i].line-occurrences[j].line < window {
				continue
			}
			findings = append(findings, Finding{
				Rule:    RuleDuplication,
				File:    occurrences[i].file,
				Line:    occurrences[i].line,
				Message: fmt.Sprintf("this %d-line block also appears at %s:%d: keep one of them", window, occurrences[j].file, occurrences[j].line),
			})
			break
		}
	}
	return findings
}

// dropSlidingCopies keeps one finding per copied block: a block of N lines produces N overlapping
// candidates, and the author only has one decision to make.
func dropSlidingCopies(findings []Finding, window int) []Finding {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	kept := findings[:0]
	lastByFile := map[string]int{}
	for _, f := range findings {
		last, seen := lastByFile[f.File]
		if seen && f.Line-last < window {
			continue
		}
		lastByFile[f.File] = f.Line
		kept = append(kept, f)
	}
	return kept
}

// windowSignature is the canonical form of a window of lines, or false when the window has too little
// substance to count: blank lines, braces and comments repeat for honest reasons.
func windowSignature(normalized []string, start, window int) (string, bool) {
	parts := make([]string, 0, window)
	content := 0
	for _, line := range normalized[start : start+window] {
		if line != "" && !structural(line) {
			content++
		}
		parts = append(parts, line)
	}
	// Braces, blanks and comments repeat for honest reasons: a window needs real statements in it before
	// an identical window elsewhere counts as a copied block.
	if content < 3 {
		return "", false
	}
	return strings.Join(parts, "\x1f"), true
}

// structural reports a line that carries punctuation or commentary but no statement of its own.
func structural(line string) bool {
	switch line {
	case "", "{", "}", "};", ")", "else {", "return", "break", "continue":
		return true
	}
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "*")
}

// normalizeLine removes the whitespace that formatting decides, so indentation does not hide a copy.
func normalizeLine(line string) string { return strings.Join(strings.Fields(line), " ") }

// skipLine reports lines that repeat for honest reasons and would otherwise match everywhere.
func skipLine(line string) bool {
	switch line {
	case "", "{", "}", "};", ")", "else {":
		return true
	}
	if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "*") {
		return true
	}
	return false
}

// lineRange is one run of changed lines in the new version of a file.
type lineRange struct{ first, last int }

// wholeFile reports whether the ranges mean "the entire file", which is how a new file is treated.
func wholeFile(ranges []lineRange) bool {
	return len(ranges) == 1 && ranges[0].first <= 1 && ranges[0].last >= 1<<30
}

// intersects reports whether [first,last] meets any changed range.
func intersects(ranges []lineRange, first, last int) bool {
	for _, r := range ranges {
		if first <= r.last && r.first <= last {
			return true
		}
	}
	return false
}

// changedLines reads the new-side line ranges of the change: the hunks of the diff, plus every line of
// each untracked file, because the whole file is new.
func changedLines(root, base string, d Deps) (map[string][]lineRange, error) {
	ranges := map[string][]lineRange{}
	args := []string{"diff", "-U0", "--no-renames"}
	if base != "" {
		// The ref is compared with the working tree, so a probe run before committing sees the change
		// it is about to review.
		args = append(args, base)
	}
	out, err := d.Git(root, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: git diff failed while reading the changed lines", ErrUnusable)
	}
	parseHunks(out, ranges)
	untracked, err := d.Git(root, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("%w: git status failed while reading the changed lines", ErrUnusable)
	}
	for _, entry := range strings.Split(untracked, "\x00") {
		if !strings.HasPrefix(entry, "?? ") {
			continue
		}
		key := filepath.ToSlash(strings.TrimPrefix(entry, "?? "))
		// An untracked file is entirely new, so the whole of it is the change.
		ranges[key] = []lineRange{{first: 1, last: 1 << 30}}
	}
	return ranges, nil
}

// parseHunks reads the new-side ranges out of a unified diff: the file changes with "+++ b/<path>" and the
// range is the second number of the "@@" header.
func parseHunks(diff string, ranges map[string][]lineRange) {
	current := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = diffPath(line)
		case strings.HasPrefix(line, "@@ ") && current != "":
			if span, ok := hunkRange(line); ok {
				ranges[current] = append(ranges[current], span)
			}
		}
	}
}

// diffPath is the new-side path of a "+++ " header, or empty for a deleted file.
func diffPath(header string) string {
	path := strings.TrimPrefix(strings.TrimPrefix(header, "+++ "), "b/")
	if path == "/dev/null" {
		return ""
	}
	return path
}

// hunkRange reads one "@@ -a,b +c,d @@" header as the range of lines the change occupies on the new side.
func hunkRange(header string) (lineRange, bool) {
	for _, field := range strings.Fields(header) {
		if !strings.HasPrefix(field, "+") {
			continue
		}
		first, count, ok := plusSpec(field)
		if !ok || count == 0 {
			return lineRange{}, false
		}
		return lineRange{first: first, last: first + count - 1}, true
	}
	return lineRange{}, false
}

// plusSpec parses the "+c,d" half of a hunk header; a missing count means one line.
func plusSpec(field string) (first, count int, ok bool) {
	parts := strings.SplitN(strings.TrimPrefix(field, "+"), ",", 2)
	first, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	count = 1
	if len(parts) == 2 {
		if parsed, err := strconv.Atoi(parts[1]); err == nil {
			count = parsed
		}
	}
	return first, count, true
}

// languageOf names the language of a path and whether it is source at all.
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
	case "":
		return "", false
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".json", ".yml", ".yaml", ".toml", ".txt", ".lock", ".mod", ".sum":
		return "", false
	}
	return "unclassified", true
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
