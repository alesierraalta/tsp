// Command complexity prints the complete list of functions over the E63
// complexity bar (issue #212).
//
// The rule, pinned by the tests in main_test.go: a function starts at 1;
// +1 for each if/for/range/switch/select plus the nesting depth where it
// opens; +1 for each case/default/else; +1 for every && and every ||.
// A function literal does not reset the depth — it scores inside its
// enclosing function. Test files are not measured: the bar governs shipped
// code, and *_test.go helpers earn their length honestly.
//
// Only functions whose score is strictly over the threshold are printed,
// sorted deterministically (score descending, then file, line and name) so
// two runs on the same commit always produce the same list.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/alesierraalta/tsp/internal/quality"
)

func main() {
	over := flag.Int("over", 15, "report functions scoring over this threshold")
	flag.Parse()

	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "complexity: git ls-files: %v\n", err)
		os.Exit(2)
	}

	fset := token.NewFileSet()
	var rows []row
	for _, path := range strings.Fields(string(out)) {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "complexity: parse %s: %v\n", path, err)
			os.Exit(2)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if score := scoreFunc(fn); score > *over {
				rows = append(rows, row{
					score: score,
					name:  fn.Name.Name,
					file:  path,
					line:  fset.Position(fn.Pos()).Line,
				})
			}
		}
	}

	for _, r := range sortRows(rows) {
		fmt.Printf("%d\t%s\t%s:%d\n", r.score, r.name, r.file, r.line)
	}
}

// row is one reported function.
type row struct {
	score int
	name  string
	file  string
	line  int
}

// sortRows orders the report: score descending, then file, line and name
// ascending, so the same tree always prints the same list.
func sortRows(rows []row) []row {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.file != b.file {
			return a.file < b.file
		}
		if a.line != b.line {
			return a.line < b.line
		}
		return a.name < b.name
	})
	return rows
}

// scoreFunc is the rule itself, kept in internal/quality so the probe and this tool cannot drift apart.
var scoreFunc = quality.Score
