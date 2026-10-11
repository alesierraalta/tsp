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

// scoreFunc applies the E63 rule to one function or method.
func scoreFunc(fn *ast.FuncDecl) int {
	s := 1
	walkStmts(&s, fn.Body.List, 0)
	return s
}

// walkStmts scores a statement list at the given nesting depth.
func walkStmts(s *int, stmts []ast.Stmt, depth int) {
	for _, st := range stmts {
		walkStmt(s, st, depth)
	}
}

// walkStmt scores one statement; controls add 1+depth, case/default/else
// clauses add 1, and their bodies nest one level deeper.
func walkStmt(s *int, st ast.Stmt, depth int) {
	switch st := st.(type) {
	case *ast.IfStmt:
		*s += 1 + depth
		walkStmt(s, st.Init, depth)
		walkNode(s, st.Cond, depth)
		walkStmts(s, st.Body.List, depth+1)
		if st.Else != nil {
			*s += 1
			walkStmt(s, st.Else, depth+1)
		}
	case *ast.ForStmt:
		*s += 1 + depth
		walkStmt(s, st.Init, depth)
		walkNode(s, st.Cond, depth)
		walkStmt(s, st.Post, depth)
		walkStmts(s, st.Body.List, depth+1)
	case *ast.RangeStmt:
		*s += 1 + depth
		walkNode(s, st.Key, depth)
		walkNode(s, st.Value, depth)
		walkNode(s, st.X, depth)
		walkStmts(s, st.Body.List, depth+1)
	case *ast.SwitchStmt:
		*s += 1 + depth
		walkStmt(s, st.Init, depth)
		walkNode(s, st.Tag, depth)
		walkStmts(s, st.Body.List, depth+1)
	case *ast.TypeSwitchStmt:
		*s += 1 + depth
		walkStmt(s, st.Init, depth)
		walkStmts(s, st.Body.List, depth+1)
	case *ast.SelectStmt:
		*s += 1 + depth
		if st.Body != nil {
			walkStmts(s, st.Body.List, depth+1)
		}
	case *ast.CaseClause:
		*s += 1
		for _, e := range st.List {
			walkNode(s, e, depth)
		}
		walkStmts(s, st.Body, depth)
	case *ast.CommClause:
		*s += 1
		walkStmt(s, st.Comm, depth)
		walkStmts(s, st.Body, depth)
	case *ast.BlockStmt:
		walkStmts(s, st.List, depth)
	case *ast.LabeledStmt:
		walkStmt(s, st.Stmt, depth)
	case nil:
	default:
		walkNode(s, st, depth)
	}
}

// walkNode scores expressions: every && and || adds one, and a function
// literal's body is walked one level deeper — it adds a nesting level but
// does not reset the enclosing depth.
func walkNode(s *int, n ast.Node, depth int) {
	if n == nil {
		return
	}
	ast.Inspect(n, func(m ast.Node) bool {
		switch m := m.(type) {
		case *ast.BinaryExpr:
			if m.Op == token.LAND || m.Op == token.LOR {
				*s++
			}
		case *ast.FuncLit:
			if m.Body != nil {
				walkStmts(s, m.Body.List, depth+1)
			}
			return false
		}
		return true
	})
}
