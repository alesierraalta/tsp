package quality

import (
	"go/ast"
	"go/token"
)

// Score applies the E63 complexity rule to one function or method: it starts at 1, adds 1 plus the
// nesting depth for every if/for/range/switch/select, 1 for every case, default and else clause, and 1
// for every && and ||. A function literal does not reset the depth: it scores inside its enclosing
// function. The rule lives here so the standalone tool and the probe cannot drift apart.
func Score(fn *ast.FuncDecl) int {
	if fn == nil || fn.Body == nil {
		return 0
	}
	s := 1
	walkStmts(&s, fn.Body.List, 0)
	return s
}

func walkStmts(s *int, stmts []ast.Stmt, depth int) {
	for _, st := range stmts {
		walkStmt(s, st, depth)
	}
}

// walkStmt scores one statement; controls add 1+depth, case/default/else clauses add 1, and their bodies
// nest one level deeper.
func walkStmt(s *int, st ast.Stmt, depth int) {
	if st == nil {
		return
	}
	if !walkControl(s, st, depth) {
		walkNode(s, st, depth)
	}
}

// walkControl scores the statements that own a body of their own. Each arm delegates so the switch stays
// one table of statement kinds instead of nine inlined bodies.
func walkControl(s *int, st ast.Stmt, depth int) bool {
	switch st := st.(type) {
	case *ast.IfStmt:
		scoreIf(s, st, depth)
	case *ast.ForStmt:
		scoreFor(s, st, depth)
	case *ast.RangeStmt:
		scoreRange(s, st, depth)
	case *ast.SwitchStmt:
		scoreSwitch(s, st, depth)
	case *ast.TypeSwitchStmt:
		scoreTypeSwitch(s, st, depth)
	case *ast.SelectStmt:
		scoreSelect(s, st, depth)
	case *ast.CaseClause:
		scoreCase(s, st, depth)
	case *ast.CommClause:
		scoreComm(s, st, depth)
	case *ast.BlockStmt:
		walkStmts(s, st.List, depth)
	case *ast.LabeledStmt:
		walkStmt(s, st.Stmt, depth)
	default:
		return false
	}
	return true
}

// scoreIf scores a branch and its else arm.
func scoreIf(s *int, st *ast.IfStmt, depth int) {
	*s += 1 + depth
	walkStmt(s, st.Init, depth)
	walkNode(s, st.Cond, depth)
	walkStmts(s, st.Body.List, depth+1)
	if st.Else != nil {
		*s += 1
		walkStmt(s, st.Else, depth+1)
	}
}

// scoreFor scores a for loop and its three clauses.
func scoreFor(s *int, st *ast.ForStmt, depth int) {
	*s += 1 + depth
	walkStmt(s, st.Init, depth)
	walkNode(s, st.Cond, depth)
	walkStmt(s, st.Post, depth)
	walkStmts(s, st.Body.List, depth+1)
}

// scoreRange scores a range loop and the expressions it ranges over.
func scoreRange(s *int, st *ast.RangeStmt, depth int) {
	*s += 1 + depth
	walkNode(s, st.Key, depth)
	walkNode(s, st.Value, depth)
	walkNode(s, st.X, depth)
	walkStmts(s, st.Body.List, depth+1)
}

// scoreSwitch scores a value switch.
func scoreSwitch(s *int, st *ast.SwitchStmt, depth int) {
	*s += 1 + depth
	walkStmt(s, st.Init, depth)
	walkNode(s, st.Tag, depth)
	walkStmts(s, st.Body.List, depth+1)
}

// scoreTypeSwitch scores a type switch, which has no tag expression.
func scoreTypeSwitch(s *int, st *ast.TypeSwitchStmt, depth int) {
	*s += 1 + depth
	walkStmt(s, st.Init, depth)
	walkStmts(s, st.Body.List, depth+1)
}

// scoreSelect scores a select and its communication clauses.
func scoreSelect(s *int, st *ast.SelectStmt, depth int) {
	*s += 1 + depth
	if st.Body != nil {
		walkStmts(s, st.Body.List, depth+1)
	}
}

// scoreCase scores one case clause.
func scoreCase(s *int, st *ast.CaseClause, depth int) {
	*s += 1
	for _, e := range st.List {
		walkNode(s, e, depth)
	}
	walkStmts(s, st.Body, depth)
}

// scoreComm scores one select clause.
func scoreComm(s *int, st *ast.CommClause, depth int) {
	*s += 1
	walkStmt(s, st.Comm, depth)
	walkStmts(s, st.Body, depth)
}

// walkNode scores expressions: every && and || adds one, and a function literal's body is walked one
// level deeper - it adds a nesting level but does not reset the enclosing depth.
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
