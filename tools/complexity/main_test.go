package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// scoreSource parses one example function and returns its E63 score.
func scoreSource(t *testing.T, src string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			return scoreFunc(fn)
		}
	}
	t.Fatal("example declares no function")
	return 0
}

// TestScoreFixesTheE63Rule pins the measurement rule of E63 (issue #212):
// base 1; +1 for each if/for/range/switch/select plus the nesting depth
// where it opens; +1 for each case/default/else; +1 for every && and ||.
func TestScoreFixesTheE63Rule(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"empty body is the base path", `func f() {}`, 1},
		{"one top-level if", `func f() { if a {} }`, 2},
		{"if inside for pays its depth", `func f() { for { if a {} } }`, 4},
		{"three-deep ifs pay 1+2+3", `func f() { if a { if b { if c {} } } }`, 7},
		{"range counts as a control", `func f() { for range x {} }`, 2},
		{"switch plus one case per clause", `func f() { switch { case a: case b: } }`, 4},
		{"default is a clause too", `func f() { switch { default: } }`, 3},
		{"case inside if pays nesting", `func f() { if a { switch { case b: } } }`, 5},
		{"select counts comm clauses as cases", `func f() { select { case <-c: case <-d: } }`, 4},
		{"each && and || adds one", `func f() { if a && b || c {} }`, 4},
		{"operators in any expression count", `func f() { _ = a && b && c }`, 3},
		{"else adds one", `func f() { if a {} else {} }`, 3},
		{"else-if chain: each else and each if", `func f() { if a {} else if b {} else {} }`, 6},
		{"else-if nests inside its else", `func f() { for { if a {} else if b {} } }`, 8},
		{"func literals do not reset depth", `func f() { if a { g(func() { if b {} }) } }`, 5},
		{"method receivers score like functions", `func (r R) m() { if a {} }`, 2},
		{"type switches score like switches", `func f() { switch v := x.(type) { case int: _ = v } }`, 3},
		{"loops and branches mix at depth", `func f() { for i := range x { if a && b { switch { case c: } } } }`, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreSource(t, tc.src); got != tc.want {
				t.Errorf("score = %d, want %d for %s", got, tc.want, tc.src)
			}
		})
	}
}

// TestOutputSortsDeterministically pins the report order: score descending,
// then file, line and name ascending — the same tree always prints the same
// list (issue #212's determinism criterion).
func TestOutputSortsDeterministically(t *testing.T) {
	fns := []*ast.FuncDecl{}
	rows := sortRows([]row{
		{score: 3, name: "b", file: "z.go", line: 2},
		{score: 3, name: "a", file: "z.go", line: 1},
		{score: 9, name: "big", file: "a.go", line: 4},
		{score: 3, name: "c", file: "a.go", line: 7},
	})
	want := []row{
		{score: 9, name: "big", file: "a.go", line: 4},
		{score: 3, name: "c", file: "a.go", line: 7},
		{score: 3, name: "a", file: "z.go", line: 1},
		{score: 3, name: "b", file: "z.go", line: 2},
	}
	if len(fns) != 0 {
		t.Fatal("unreachable")
	}
	for i, w := range want {
		if rows[i] != w {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
}
