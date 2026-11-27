// Package calibrate seeds sealed mutants into a throwaway worktree so a test-strategy calibration
// run can measure what a skill found rather than what it claims: candidate sites are read from the
// token stream, the pick is deterministic in its seed, and the answer key is sealed outside the tree
// it changes.
package calibrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Site is one candidate mutation. The JSON field names are the ones assets/seed-mutants.py wrote, so
// a key that script sealed replays through Replay unchanged.
type Site struct {
	File     string `json:"file"`     // path relative to the worktree root
	Line     int    `json:"line"`     // 1-based
	Operator string `json:"operator"` // the operator name the key records
	Original string `json:"original"` // the whole line before the mutation
	Mutated  string `json:"mutated"`  // the whole line after it
}

// allowedBranchPrefixes names the throwaway worktrees calibration may mutate.
var allowedBranchPrefixes = []string{"calib-", "skilltest-"}

// List answers the candidate sites per file, in the given order, with the total the files hold.
func List(root string, files []string) (map[string]int, int, error) {
	if err := guardWorktree(root); err != nil {
		return nil, 0, err
	}
	perFile := make(map[string]int, len(files))
	total := 0
	for _, rel := range files {
		sites, err := candidates(root, rel)
		if err != nil {
			return nil, 0, err
		}
		perFile[rel] = len(sites)
		total += len(sites)
	}
	return perFile, total, nil
}

// guardWorktree refuses any tree whose branch is not named for a throwaway worktree.
func guardWorktree(root string) error {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("REFUSED: %s is not a git worktree: %w", root, err)
	}
	branch := strings.TrimSpace(string(out))
	for _, prefix := range allowedBranchPrefixes {
		if strings.HasPrefix(branch, prefix) {
			return nil
		}
	}
	return fmt.Errorf("REFUSED: branch %q at %s is not a throwaway worktree; calibration only runs on a branch named calib-* or skilltest-*",
		branch, root)
}

// candidates answers every site one file holds. Only Go source is scanned: a site lives in the token
// stream, never in a comment or a string literal, and test files and generated files are left alone.
func candidates(root, rel string) ([]Site, error) {
	if filepath.Ext(rel) != ".go" || strings.HasSuffix(rel, "_test.go") {
		return nil, nil
	}
	src, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	if isGenerated(src) {
		return nil, nil
	}
	tokens, err := scanTokens(rel, src)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", rel, err)
	}
	lines, starts := splitLines(src)
	var sites []Site
	for _, group := range byLine(tokens) {
		line := lines[group.line-1]
		for _, mutation := range lineMutations(group, line, starts[group.line-1]) {
			if mutated := mutation.apply(line); mutated != line {
				sites = append(sites, Site{
					File:     rel,
					Line:     group.line,
					Operator: mutation.operator,
					Original: line,
					Mutated:  mutated,
				})
			}
		}
	}
	return sites, nil
}

// isGenerated reports the marker the Go toolchain itself reads: a line beginning "// Code generated"
// and ending "DO NOT EDIT.".
func isGenerated(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "// Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true
		}
	}
	return false
}

// sourceToken is one token with the byte offset and line the site construction needs.
type sourceToken struct {
	tok    token.Token
	lit    string
	offset int
	line   int
}

// scanTokens reads a file through go/scanner. An operator inside a comment or a literal is never a
// token of its own, which is what keeps such text from hosting a site.
func scanTokens(name string, src []byte) ([]sourceToken, error) {
	fset := token.NewFileSet()
	file := fset.AddFile(name, fset.Base(), len(src))
	var s scanner.Scanner
	// Mode 0: go/scanner answers the literal of every token and skips comments, which are text, not sites.
	s.Init(file, src, nil, 0)
	var tokens []sourceToken
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		tokens = append(tokens, sourceToken{
			tok:    tok,
			lit:    lit,
			offset: file.Offset(pos),
			line:   file.Position(pos).Line,
		})
	}
	return tokens, nil
}

// splitLines answers the file's lines and the byte offset each of them starts at. A trailing newline
// leaves an empty last line, the shape strings.Split gives, so a rewrite preserves every byte.
func splitLines(src []byte) ([]string, []int) {
	lines := strings.Split(string(src), "\n")
	starts := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		starts[i] = offset
		offset += len(line) + 1
	}
	return lines, starts
}

// lineGroup holds the tokens one line carries, in source order.
type lineGroup struct {
	line   int
	tokens []sourceToken
}

func byLine(tokens []sourceToken) []lineGroup {
	var groups []lineGroup
	for _, tok := range tokens {
		if len(groups) == 0 || groups[len(groups)-1].line != tok.line {
			groups = append(groups, lineGroup{line: tok.line})
		}
		groups[len(groups)-1].tokens = append(groups[len(groups)-1].tokens, tok)
	}
	return groups
}

// mutation is one replacement inside a line, already carrying the name the key records.
type mutation struct {
	operator   string
	start, end int
	text       string
}

func (m mutation) apply(line string) string {
	return line[:m.start] + m.text + line[m.end:]
}

// lineMutations answers every site one line offers: at most one per operator, in the order the
// contract names them.
func lineMutations(group lineGroup, line string, lineStart int) []mutation {
	var mutations []mutation
	for _, operator := range lineOperators {
		if m, ok := operator(group, line, lineStart); ok {
			mutations = append(mutations, m)
		}
	}
	return mutations
}

// lineOperators are the five operators the sealed seeder knows, in contract order.
var lineOperators = []func(lineGroup, string, int) (mutation, bool){
	flipComparison,
	swapLogical,
	negateIf,
	flipBoolReturn,
	dropErrorCheck,
}

// text answers the source text of a token: go/scanner answers the literal for identifiers and
// literals, and the token's own spelling for everything else.
func (t sourceToken) text() string {
	if t.lit != "" {
		return t.lit
	}
	return t.tok.String()
}

// span answers the offset of a token within its line, and where it ends.
func span(tok sourceToken, lineStart int) (int, int) {
	start := tok.offset - lineStart
	return start, start + len(tok.text())
}

// firstToken answers the first token of the kind the line carries.
func firstToken(group lineGroup, kinds ...token.Token) int {
	for i, tok := range group.tokens {
		for _, kind := range kinds {
			if tok.tok == kind {
				return i
			}
		}
	}
	return -1
}

// comparisonPairs is ordered so ">=" is read as a whole rather than as ">" followed by "=".
var comparisonPairs = []struct {
	from, to token.Token
	operator string
}{
	{token.GEQ, token.GTR, "flip-comparison >=->>"},
	{token.LEQ, token.LSS, "flip-comparison <=-><"},
	{token.EQL, token.NEQ, "flip-comparison ==->!="},
	{token.NEQ, token.EQL, "flip-comparison !=->=="},
	{token.GTR, token.GEQ, "flip-comparison >->>="},
	{token.LSS, token.LEQ, "flip-comparison <-><="},
}

// flipComparison flips the first comparison of the line through the pair the contract lists for it.
func flipComparison(group lineGroup, _ string, lineStart int) (mutation, bool) {
	for _, pair := range comparisonPairs {
		if index := firstToken(group, pair.from); index >= 0 {
			start, end := span(group.tokens[index], lineStart)
			return mutation{operator: pair.operator, start: start, end: end, text: pair.to.String()}, true
		}
	}
	return mutation{}, false
}

// swapLogical turns the first logical operator of the line into the other one.
func swapLogical(group lineGroup, _ string, lineStart int) (mutation, bool) {
	index := firstToken(group, token.LAND, token.LOR)
	if index < 0 {
		return mutation{}, false
	}
	start, end := span(group.tokens[index], lineStart)
	if group.tokens[index].tok == token.LAND {
		return mutation{operator: "swap-logical &&->||", start: start, end: end, text: "||"}, true
	}
	return mutation{operator: "swap-logical ||->&&", start: start, end: end, text: "&&"}, true
}

// negateIf wraps a condition that opens and closes on this line. A condition the source spreads over
// several lines is left alone: the key records one whole line, and no single line can carry it.
func negateIf(group lineGroup, line string, lineStart int) (mutation, bool) {
	index := firstToken(group, token.IF)
	if index < 0 {
		return mutation{}, false
	}
	brace := lastBrace(group.tokens[index:])
	if brace < 0 {
		return mutation{}, false
	}
	_, ifEnd := span(group.tokens[index], lineStart)
	braceStart, _ := span(group.tokens[index+brace], lineStart)
	condition := strings.TrimSpace(line[ifEnd:braceStart])
	if condition == "" || strings.HasPrefix(condition, "!(") {
		return mutation{}, false
	}
	return mutation{operator: "negate-if", start: ifEnd, end: braceStart, text: " !(" + condition + ") "}, true
}

// lastBrace answers the index of the last "{" among tokens: the body a statement opens rather than a
// composite literal inside its condition. A line that carries a second block is caught by the parse
// check, which reverts the mutant it cannot parse.
func lastBrace(tokens []sourceToken) int {
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].tok == token.LBRACE {
			return i
		}
	}
	return -1
}

// flipBoolReturn turns a returned boolean literal into the other one.
func flipBoolReturn(group lineGroup, _ string, lineStart int) (mutation, bool) {
	index := firstToken(group, token.RETURN)
	if index < 0 || index+1 >= len(group.tokens) {
		return mutation{}, false
	}
	value := group.tokens[index+1]
	if value.tok != token.IDENT {
		return mutation{}, false
	}
	start, end := span(value, lineStart)
	switch value.lit {
	case "true":
		return mutation{operator: "flip-bool-return true->false", start: start, end: end, text: "false"}, true
	case "false":
		return mutation{operator: "flip-bool-return false->true", start: start, end: end, text: "true"}, true
	}
	return mutation{}, false
}

// dropErrorCheck turns the one check every Go function writes into a check that never fires.
func dropErrorCheck(group lineGroup, _ string, lineStart int) (mutation, bool) {
	index := firstToken(group, token.IF)
	if index < 0 || index+3 >= len(group.tokens) {
		return mutation{}, false
	}
	subject, operator, value := group.tokens[index+1], group.tokens[index+2], group.tokens[index+3]
	if subject.lit != "err" || operator.tok != token.NEQ || value.lit != "nil" {
		return mutation{}, false
	}
	brace := lastBrace(group.tokens[index:])
	if brace < 0 {
		return mutation{}, false
	}
	_, ifEnd := span(group.tokens[index], lineStart)
	braceStart, _ := span(group.tokens[index+brace], lineStart)
	return mutation{operator: "drop-error-check", start: ifEnd, end: braceStart, text: " false "}, true
}

// KeyResult is what one seeding or one replay did: the mutants it applied, the files it touched, the
// key it wrote or read, and the digest of that key as it stands on disk.
type KeyResult struct {
	Applied int
	Files   []string
	KeyPath string
	SHA256  string
}

// Seed applies count mutants chosen deterministically by seed and seals them in a key outside the
// tree. A candidate whose mutation does not parse is reverted; when too few parse-valid seeds remain,
// every applied seed is reverted and Seed refuses, so a refusal always leaves the tree byte-identical.
func Seed(root string, files []string, count int, seed int64, keyOut string) (KeyResult, error) {
	if err := guardWorktree(root); err != nil {
		return KeyResult{}, err
	}
	keyPath, err := keyOutsideRoot(root, keyOut)
	if err != nil {
		return KeyResult{}, err
	}
	var all []Site
	for _, rel := range files {
		sites, err := candidates(root, rel)
		if err != nil {
			return KeyResult{}, err
		}
		all = append(all, sites...)
	}
	if len(all) < count {
		return KeyResult{}, fmt.Errorf("REFUSED: only %d candidate sites for %d mutations", len(all), count)
	}

	tree := &worktree{root: root, files: map[string]*treeFile{}}
	chosen, err := tree.pick(all, count, seed)
	if err != nil {
		return KeyResult{}, tree.refuse(err)
	}

	payload, err := encodeKey(chosen)
	if err != nil {
		return KeyResult{}, tree.refuse(err)
	}
	if err := writeKey(keyPath, payload); err != nil {
		return KeyResult{}, tree.refuse(err)
	}
	return KeyResult{
		Applied: len(chosen),
		Files:   touchedFiles(chosen),
		KeyPath: keyPath,
		SHA256:  sha256Hex(payload),
	}, nil
}

// pick applies mutants in the order the seed answers them, one per line, keeping a candidate only
// when the file still loads with it; a rejected candidate is reverted and the pick moves on.
func (w *worktree) pick(all []Site, count int, seed int64) ([]Site, error) {
	var chosen []Site
	used := make(map[siteLine]bool, len(all))
	rejected := 0
	for _, index := range rand.New(rand.NewSource(seed)).Perm(len(all)) {
		if len(chosen) >= count {
			break
		}
		site := all[index]
		line := siteLine{file: site.File, line: site.Line}
		if used[line] {
			continue
		}
		used[line] = true
		if err := w.apply(site); err != nil {
			return nil, err
		}
		if !w.parses(site.File) {
			if err := w.revert(site); err != nil {
				return nil, err
			}
			rejected++
			continue
		}
		chosen = append(chosen, site)
	}
	if len(chosen) < count {
		return nil, fmt.Errorf("REFUSED: only %d parse-valid seeds found for %d mutations (%d rejected)",
			len(chosen), count, rejected)
	}
	return chosen, nil
}

// Replay re-applies a key that already exists. Every site is checked against the line the key
// recorded before a single byte is written, so a line that moved is a refusal, not a wrong mutant.
func Replay(root, keyPath string) (KeyResult, error) {
	if err := guardWorktree(root); err != nil {
		return KeyResult{}, err
	}
	path, err := filepath.Abs(keyPath)
	if err != nil {
		return KeyResult{}, fmt.Errorf("resolve key %s: %w", keyPath, err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return KeyResult{}, fmt.Errorf("read key: %w", err)
	}
	var sites []Site
	if err := json.Unmarshal(payload, &sites); err != nil {
		return KeyResult{}, fmt.Errorf("read key %s: %w", path, err)
	}

	tree := &worktree{root: root, files: map[string]*treeFile{}}
	for _, site := range sites {
		if err := tree.verify(site); err != nil {
			return KeyResult{}, tree.refuse(err)
		}
	}
	for _, site := range sites {
		if err := tree.apply(site); err != nil {
			return KeyResult{}, tree.refuse(err)
		}
	}
	return KeyResult{
		Applied: len(sites),
		Files:   touchedFiles(sites),
		KeyPath: path,
		SHA256:  sha256Hex(payload),
	}, nil
}

// keyOutsideRoot answers the key path an absolute one, refusing a key the calibrated run could read.
// The outside check resolves symlinks — /tmp and /private/tmp are one directory — while the path that
// comes back is the caller's own, made absolute, because that is the path the caller prints.
func keyOutsideRoot(root, keyOut string) (string, error) {
	if strings.TrimSpace(keyOut) == "" {
		return "", fmt.Errorf("REFUSED: a key path outside the worktree is required")
	}
	absolute, err := filepath.Abs(keyOut)
	if err != nil {
		return "", fmt.Errorf("resolve key %s: %w", keyOut, err)
	}
	resolvedRoot := realPath(root)
	resolvedKey := realPath(absolute)
	if inside, err := filepath.Rel(resolvedRoot, resolvedKey); err == nil &&
		inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator)) && !filepath.IsAbs(inside) {
		return "", fmt.Errorf("REFUSED: key %s must be outside the worktree %s", absolute, resolvedRoot)
	}
	return absolute, nil
}

// realPath resolves the symlinks of a path that may not exist yet by resolving its directory.
func realPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(absolute)); err == nil {
		return filepath.Join(resolved, filepath.Base(absolute))
	}
	return absolute
}

// siteLine names one line of one file: two mutants on the same line would overwrite each other.
type siteLine struct {
	file string
	line int
}

// treeFile is one file the seeding holds: the bytes it had, and the lines it has now.
type treeFile struct {
	rel      string
	path     string
	mode     os.FileMode
	pristine []byte
	lines    []string
}

// worktree is the set of files a seeding or a replay has touched, so every refusal can put them back
// exactly as they were found.
type worktree struct {
	root  string
	files map[string]*treeFile
}

func (w *worktree) load(rel string) (*treeFile, error) {
	if file, ok := w.files[rel]; ok {
		return file, nil
	}
	path := filepath.Join(w.root, rel)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	pristine, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	file := &treeFile{
		rel:      rel,
		path:     path,
		mode:     info.Mode().Perm(),
		pristine: pristine,
		lines:    strings.Split(string(pristine), "\n"),
	}
	w.files[rel] = file
	return file, nil
}

// verify refuses a site whose line is not the line the key recorded.
func (w *worktree) verify(site Site) error {
	file, err := w.load(site.File)
	if err != nil {
		return err
	}
	if site.Line < 1 || site.Line > len(file.lines) {
		return fmt.Errorf("REFUSED: %s has no line %d", site.File, site.Line)
	}
	if file.lines[site.Line-1] != site.Original {
		return fmt.Errorf("REFUSED: %s:%d is not the line the key recorded", site.File, site.Line)
	}
	return nil
}

func (w *worktree) apply(site Site) error {
	if err := w.verify(site); err != nil {
		return err
	}
	file := w.files[site.File]
	file.lines[site.Line-1] = site.Mutated
	return w.write(file)
}

func (w *worktree) revert(site Site) error {
	file, err := w.load(site.File)
	if err != nil {
		return err
	}
	file.lines[site.Line-1] = site.Original
	return w.write(file)
}

// parses answers whether the file, with the mutants it holds now, still loads.
func (w *worktree) parses(rel string) bool {
	file, ok := w.files[rel]
	if !ok {
		return false
	}
	_, err := parser.ParseFile(token.NewFileSet(), rel, strings.Join(file.lines, "\n"), parser.SkipObjectResolution)
	return err == nil
}

func (w *worktree) write(file *treeFile) error {
	if err := os.WriteFile(file.path, []byte(strings.Join(file.lines, "\n")), file.mode); err != nil {
		return fmt.Errorf("write %s: %w", file.rel, err)
	}
	return nil
}

// refuse restores every file it touched and answers the refusal, so the caller's tree is byte-identical.
// A file still holding the bytes it was found with is left alone rather than rewritten.
func (w *worktree) refuse(reason error) error {
	for _, file := range w.files {
		if strings.Join(file.lines, "\n") == string(file.pristine) {
			continue
		}
		if err := os.WriteFile(file.path, file.pristine, file.mode); err != nil {
			return fmt.Errorf("%w (and %s could not be restored: %v)", reason, file.rel, err)
		}
	}
	return reason
}

// encodeKey answers the key in the shape the Python seeder wrote: an indented JSON array of whole
// lines, its operators readable rather than escaped.
func encodeKey(sites []Site) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sites); err != nil {
		return nil, fmt.Errorf("encode key: %w", err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func writeKey(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return nil
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// touchedFiles answers the files the sites name, each once, in a stable order.
func touchedFiles(sites []Site) []string {
	seen := make(map[string]bool, len(sites))
	files := make([]string, 0, len(sites))
	for _, site := range sites {
		if !seen[site.File] {
			seen[site.File] = true
			files = append(files, site.File)
		}
	}
	sort.Strings(files)
	return files
}
