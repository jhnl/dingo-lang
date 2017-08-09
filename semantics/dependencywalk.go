package semantics

import "github.com/jhnl/interpreter/ir"

type dependencyVisitor struct {
	BaseVisitor
	c *checker
}

func dependencyWalk(c *checker) {
	v := &dependencyVisitor{c: c}
	c.resetWalkState()
	VisitModuleSet(v, c.set)
}

func (v *dependencyVisitor) Module(mod *ir.Module) {
	v.c.mod = mod
	for _, decl := range mod.Decls {
		v.c.setTopDecl(decl)
		VisitDecl(v, decl)
	}
}

func (v *dependencyVisitor) VisitValTopDecl(decl *ir.ValTopDecl) {
	if decl.Type != nil {
		VisitExpr(v, decl.Type)
	}
	if decl.Initializer != nil {
		VisitExpr(v, decl.Initializer)
	}
}

func (v *dependencyVisitor) VisitValDecl(decl *ir.ValDecl) {
	if decl.Type != nil {
		VisitExpr(v, decl.Type)
	}
	if decl.Initializer != nil {
		VisitExpr(v, decl.Initializer)
	}
}

func (v *dependencyVisitor) VisitFuncDecl(decl *ir.FuncDecl) {
	defer setScope(setScope(v.c, decl.Scope))
	for _, param := range decl.Params {
		VisitExpr(v, param.Type)
	}
	VisitExpr(v, decl.TReturn)
	VisitStmtList(v, decl.Body.Stmts)
}

func (v *dependencyVisitor) VisitStructDecl(decl *ir.StructDecl) {
	for _, f := range decl.Fields {
		v.VisitValDecl(f)
	}
}

func (v *dependencyVisitor) VisitBlockStmt(stmt *ir.BlockStmt) {
	defer setScope(setScope(v.c, stmt.Scope))
	VisitStmtList(v, stmt.Stmts)
}

func (v *dependencyVisitor) VisitDeclStmt(stmt *ir.DeclStmt) {
	VisitDecl(v, stmt.D)
}

func (v *dependencyVisitor) VisitPrintStmt(stmt *ir.PrintStmt) {
	for _, x := range stmt.Xs {
		VisitExpr(v, x)
	}
}

func (v *dependencyVisitor) VisitIfStmt(stmt *ir.IfStmt) {
	v.VisitBlockStmt(stmt.Body)
	if stmt.Else != nil {
		VisitStmt(v, stmt.Else)
	}
}

func (v *dependencyVisitor) VisitWhileStmt(stmt *ir.WhileStmt) {
	v.VisitBlockStmt(stmt.Body)
}

func (v *dependencyVisitor) VisitReturnStmt(stmt *ir.ReturnStmt) {
	if stmt.X != nil {
		VisitExpr(v, stmt.X)
	}
}

func (v *dependencyVisitor) VisitAssignStmt(stmt *ir.AssignStmt) {
	VisitExpr(v, stmt.Left)
	VisitExpr(v, stmt.Right)
}

func (v *dependencyVisitor) VisitExprStmt(stmt *ir.ExprStmt) {
	VisitExpr(v, stmt.X)
}

func (v *dependencyVisitor) VisitBinaryExpr(expr *ir.BinaryExpr) ir.Expr {
	VisitExpr(v, expr.Left)
	VisitExpr(v, expr.Right)
	return expr
}

func (v *dependencyVisitor) VisitUnaryExpr(expr *ir.UnaryExpr) ir.Expr {
	VisitExpr(v, expr.X)
	return expr
}

func (v *dependencyVisitor) VisitStructLit(expr *ir.StructLit) ir.Expr {
	VisitExpr(v, expr.Name)
	for _, kv := range expr.Initializers {
		VisitExpr(v, kv.Value)
	}
	return expr
}

func (v *dependencyVisitor) VisitIdent(expr *ir.Ident) ir.Expr {
	sym := v.c.lookup(expr.Literal())
	if sym != nil {
		if decl, ok := sym.Src.(ir.TopDecl); ok {
			_, isFunc1 := v.c.topDecl.(*ir.FuncDecl)
			_, isFunc2 := decl.(*ir.FuncDecl)

			// Cycle between functions is ok
			if !isFunc1 || !isFunc2 {
				v.c.topDecl.AddDependency(decl)
			}
		}
	}
	return expr
}

func (v *dependencyVisitor) VisitDotExpr(expr *ir.DotExpr) ir.Expr {
	VisitExpr(v, expr.X)
	v.VisitIdent(expr.Name)
	return expr
}

func (v *dependencyVisitor) VisitFuncCall(expr *ir.FuncCall) ir.Expr {
	VisitExpr(v, expr.X)
	VisitExprList(v, expr.Args)
	return expr
}
