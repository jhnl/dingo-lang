package semantics

import (
	"fmt"

	"github.com/cjo5/dingo/internal/ir"
	"github.com/cjo5/dingo/internal/token"
)

func (c *checker) checkTypes() {
	c.step = 0
	for _, objList := range c.objectMatrix {
		c.objectList = objList
		for _, obj := range objList.objects {
			c.checkObject(obj)
		}
	}
	c.step++
	keys := c.sortedIncompleteKeys()
	for _, key := range keys {
		decl := c.incomplete[key]
		c.checkIncompleteObject(decl)
	}
}

func (c *checker) checkIncompleteObject(obj *object) {
	if obj.color != whiteColor {
		return
	}
	obj.color = grayColor
	for _, dep := range obj.deps {
		if dep.obj.incomplete {
			c.checkIncompleteObject(dep.obj)
		}
	}
	c.objectList = c.objectMatrix[obj.CUID()]
	obj.checked = false
	obj.incomplete = false
	c.checkObject(obj)
	obj.color = blackColor
}

func (c *checker) checkObject(obj *object) {
	c.object = obj
	defer c.setScope(c.setScope(obj.parentScope))
	c.checkDecl(obj.d)
	c.object.checked = true
	switch decl := obj.d.(type) {
	case *ir.ValDecl:
		if !isUntyped(decl.Sym.T) {
			init := decl.Initializer
			if !decl.Sym.IsConst() {
				c.error(init.Pos(), "top-level initializer must be a compile-time constant")
				decl.Sym.T = ir.TBuiltinInvalid
			}
		}
	}
}

func (c *checker) checkLocalDecl(decl ir.Decl) {
	if c.step == 0 {
		switch decl := decl.(type) {
		case *ir.UseDecl:
			decl.Sym = c.newUseSymbol(decl, c.object.CUID(), c.object.modFQN(), false, false)
		case *ir.TypeDecl:
			decl.Sym = c.newLocalTypeDeclSymbol(decl, c.object.CUID(), c.object.modFQN())
		case *ir.ValDecl:
			decl.Sym = c.newLocalValDeclSymbol(decl, c.object.CUID(), c.object.modFQN())
		default:
			panic(fmt.Sprintf("Unhandled decl %T", decl))
		}
	}
	c.checkDecl(decl)
	if c.step == 0 {
		sym := decl.Symbol()
		if sym != nil {
			sym = c.insertSymbol(c.scope, sym.Name, sym)
			decl.SetSymbol(sym)
		}
	}
}

func (c *checker) checkDecl(decl ir.Decl) {
	sym := decl.Symbol()
	if sym == nil {
		return
	}
	switch decl := decl.(type) {
	case *ir.ImportDecl:
		// Nothing to do
	case *ir.UseDecl:
		if isUnknownType(sym.T) {
			c.checkUseDecl(decl)
		}
	case *ir.TypeDecl:
		if isUnknownType(sym.T) {
			c.checkTypeDecl(decl)
		}
	case *ir.ValDecl:
		if isUnknownType(sym.T) {
			c.checkValDecl(decl)
		}
	case *ir.FuncDecl:
		c.checkFuncDecl(decl)
	case *ir.StructDecl:
		c.checkStructDecl(decl)
	default:
		panic(fmt.Sprintf("Unhandled decl %T", decl))
	}
}

func (c *checker) checkUseDecl(decl *ir.UseDecl) {
	c.resolveScopeLookup(decl.Name)
	tuse := decl.Name.T
	if !isUntyped(decl.Name.T) {
		last := decl.Name.Last()
		decl.Sym.Kind = last.Sym.Kind
		decl.Sym.Key = last.Sym.Key
		decl.Sym.ABI = last.Sym.ABI
		decl.Sym.Flags |= (last.Sym.Flags & (ir.SymFlagReadOnly | ir.SymFlagConst))
		if decl.Sym.Public && !last.Sym.Public {
			c.nodeError(decl.Name, "private '%s' cannot be re-exported as public", last.Sym.Name)
			tuse = ir.TBuiltinInvalid
		} else if last.Sym.IsBuiltin() {
			c.nodeError(decl.Name, "builtin '%s' cannot be brought into scope", last.Sym.Name)
			tuse = ir.TBuiltinInvalid
		}
	}
	decl.Alias.T = tuse
	decl.Sym.T = tuse
}

func (c *checker) checkTypeDecl(decl *ir.TypeDecl) {
	decl.Type = c.checkRootTypeExpr(decl.Type, false)
	tbase := decl.Type.Type()
	if isUntyped(tbase) {
		decl.Sym.T = tbase
	} else {
		decl.Sym.T = ir.NewAliasType(decl.Name.Literal, decl.Type.Type())
	}
}

func (c *checker) checkValDecl(decl *ir.ValDecl) {
	if decl.Type != nil {
		decl.Type = c.checkRootTypeExpr(decl.Type, true)
	}
	if decl.Initializer != nil {
		decl.Initializer = c.checkExpr(decl.Initializer)
	}
	if tuntyped := checkUntypedExprs(decl.Type, decl.Initializer); tuntyped != nil {
		decl.Sym.T = tuntyped
		return
	}
	tval := ir.TBuiltinInvalid
	if decl.Initializer != nil {
		var tdecl ir.Type
		if decl.Type != nil {
			tdecl = decl.Type.Type()
			decl.Initializer = c.finalizeExpr(decl.Initializer, tdecl)
		} else {
			decl.Initializer = c.finalizeExpr(decl.Initializer, nil)
			tdecl = decl.Initializer.Type()
		}
		tinit := decl.Initializer.Type()
		if tdecl.Equals(tinit) {
			tval = tdecl
		} else {
			c.nodeError(decl, "type mismatch '%s' and '%s'", tdecl, tinit)
		}
	} else {
		tval = decl.Type.Type()
	}
	if !isUntyped(tval) {
		if decl.Initializer == nil {
			decl.Initializer = ir.NewDefaultInit(tval)
		}
		if decl.Decl.Is(token.Val) {
			decl.Sym.Flags |= ir.SymFlagReadOnly
		}
		if checkCompileTimeConstant(decl.Initializer) {
			decl.Sym.Flags |= ir.SymFlagConst
		}
	}
	decl.Sym.T = tval
}

func checkCompileTimeConstant(expr ir.Expr) bool {
	constant := true
	switch t := expr.(type) {
	case *ir.BasicLit:
	case *ir.ConstExpr:
	case *ir.DefaultInit:
	case *ir.AppExpr:
		if t.IsStruct {
			for _, arg := range t.Args {
				if !checkCompileTimeConstant(arg.Value) {
					return false
				}
			}
		} else {
			constant = false
		}
	case *ir.ArrayLit:
		for _, elem := range t.Initializers {
			if !checkCompileTimeConstant(elem) {
				return false
			}
		}
	case *ir.Ident:
		if t.Sym == nil || t.Sym.Kind != ir.FuncSymbol {
			return false
		}
	default:
		constant = false
	}
	return constant
}

func (c *checker) checkFuncDecl(decl *ir.FuncDecl) {
	defer c.setScope(c.setScope(decl.Scope))
	if isUnknownType(decl.Sym.T) {
		var tuntyped ir.Type
		for _, param := range decl.Params {
			c.checkLocalDecl(param)
			if param.Sym != nil {
				tuntyped = checkUntyped(param.Sym.T, tuntyped)
			} else {
				tuntyped = ir.TBuiltinInvalid
			}
		}
		if c.step == 0 {
			sym := decl.Sym
			c.insertLocalValDeclSymbol(decl.Return, sym.CUID, sym.ModFQN)
		}
		decl.Return.Type = c.checkRootTypeExpr(decl.Return.Type, false)
		tret := decl.Return.Type.Type()
		tuntyped = checkUntyped(tuntyped, tret)
		if tuntyped != nil {
			decl.Sym.T = tuntyped
		} else {
			var params []ir.Field
			for _, param := range decl.Params {
				params = append(params, ir.Field{Name: param.Sym.Name, T: param.Type.Type()})
			}
			cabi := (decl.Sym.ABI == ir.CABI)
			tfun := ir.NewFuncType(params, tret, cabi)
			if isTypeMismatch(decl.Sym.T, tfun) {
				c.error(decl.Name.Pos(), "redeclaration of '%s' (different declaration is at %s)", decl.Name.Literal, decl.Sym.Pos)
				decl.Sym.T = ir.TBuiltinInvalid
			} else {
				decl.Sym.T = tfun
			}
		}
		c.object.checked = true
	}
	if !decl.SignatureOnly() {
		stmtList(decl.Body.Stmts, c.checkStmt)
	}
}

func (c *checker) checkStructDecl(decl *ir.StructDecl) {
	if decl.Opaque && decl.Sym.IsDefined() {
		return
	}
	var tstruct *ir.StructType
	switch tbase := ir.ToBaseType(decl.Sym.T).(type) {
	case *ir.StructType:
		tstruct = tbase
	default:
		if tbase.Kind() != ir.TInvalid {
			panic(fmt.Sprintf("unknown type %T", tbase))
		}
		return
	}
	if tstruct.TypedBody {
		return
	}
	var tuntyped ir.Type
	for _, field := range decl.Fields {
		if field.Sym != nil {
			// The struct has a dependency on its own fields
			c.trySetDep(field.Sym, false)
			tuntyped = checkUntyped(field.Sym.T, tuntyped)
		} else {
			tuntyped = ir.TBuiltinInvalid
		}
	}
	typedBody := true
	if tuntyped != nil {
		if isInvalidType(tuntyped) {
			decl.Sym.T = tuntyped
			return
		}
		typedBody = false
	}
	var fields []ir.Field
	for _, field := range decl.Fields {
		if field.Sym != nil {
			fields = append(fields, ir.Field{Name: field.Sym.Name, T: field.Type.Type()})
		}
	}
	tstruct.SetBody(fields, typedBody)
}

func (c *checker) checkStmt(stmt ir.Stmt) {
	switch stmt := stmt.(type) {
	case *ir.BlockStmt:
		if c.step == 0 {
			c.openScope("block")
			stmt.Scope = c.scope
			c.closeScope()
		}
		prevScope := c.setScope(stmt.Scope)
		stmtList(stmt.Stmts, c.checkStmt)
		c.setScope(prevScope)
	case *ir.DeclStmt:
		c.checkLocalDecl(stmt.D)
	case *ir.IfStmt:
		if isUnknownExprType(stmt.Cond) {
			stmt.Cond = c.checkExpr(stmt.Cond)
			if isTypeMismatch(stmt.Cond.Type(), ir.TBuiltinBool) {
				c.error(stmt.Cond.Pos(), "condition expects type %s (got %s)", ir.TBool, stmt.Cond.Type())
				stmt.Cond.SetType(ir.TBuiltinInvalid)
			}
		}
		c.checkStmt(stmt.Body)
		if stmt.Else != nil {
			c.checkStmt(stmt.Else)
		}
	case *ir.ForStmt:
		if c.step == 0 {
			c.openScope("for")
			stmt.Body.Scope = c.scope
			c.closeScope()
		}
		prevScope := c.setScope(stmt.Body.Scope)
		if stmt.Init != nil {
			c.checkStmt(stmt.Init)
		}
		if stmt.Cond != nil {
			if isUnknownExprType(stmt.Cond) {
				stmt.Cond = c.checkExpr(stmt.Cond)
				if isTypeMismatch(stmt.Cond.Type(), ir.TBuiltinBool) {
					c.error(stmt.Cond.Pos(), "condition expects type %s (got %s)", ir.TBool, stmt.Cond.Type())
					stmt.Cond.SetType(ir.TBuiltinInvalid)
				}
			}
		}
		if stmt.Inc != nil {
			c.checkStmt(stmt.Inc)
		}
		c.loop++
		stmtList(stmt.Body.Stmts, c.checkStmt)
		c.loop--
		c.setScope(prevScope)
	case *ir.ReturnStmt:
		if stmt.X != nil && isUnknownExprType(stmt.X) {
			stmt.X = c.checkExpr(stmt.X)
			if isUntypedExpr(stmt.X) {
				return
			}
		}
		fun := c.object.d.(*ir.FuncDecl)
		tret := fun.Return.Type.Type()
		if isUntyped(tret) {
			return
		}
		if stmt.X == nil {
			stmt.X = ir.NewIdent1(token.Placeholder)
			stmt.X.SetType(ir.TBuiltinVoid)
		} else {
			stmt.X = c.finalizeExpr(stmt.X, tret)
		}
		texpr := stmt.X.Type()
		if isTypeMismatch(texpr, tret) {
			c.error(stmt.Pos(), "function expects return type %s (got %s)", tret, texpr)
			stmt.X.SetType(ir.TBuiltinInvalid)
		}
	case *ir.DeferStmt:
		c.scope.Defer = true
		c.checkStmt(stmt.S)
	case *ir.BranchStmt:
		if c.step == 0 && c.loop == 0 {
			c.error(stmt.Pos(), "'%s' can only be used in a loop", stmt.Tok)
		}
	case *ir.AssignStmt:
		if isUnknownExprType(stmt.Left) || isUnknownExprType(stmt.Right) {
			stmt.Left = c.checkExpr(stmt.Left)
			stmt.Right = c.checkExpr(stmt.Right)
			if checkUntypedExprs(stmt.Left, stmt.Right) != nil {
				return
			}
			left := stmt.Left
			err := false
			if stmt.Assign != token.Assign {
				if !ir.IsNumericType(left.Type()) {
					err = true
					c.nodeError(left, "type %s is not numeric", left.Type())
				}
			}
			if !err {
				stmt.Right = c.finalizeExpr(stmt.Right, left.Type())
				right := stmt.Right
				if isTypeMismatch(left.Type(), right.Type()) {
					err = true
					c.nodeError(stmt, "type mismatch %s and %s", left.Type(), right.Type())
				}
			}
			if !err {
				if !left.Lvalue() {
					err = true
					c.error(left.Pos(), "expression is not an lvalue")
				} else if left.ReadOnly() {
					err = true
					c.error(left.Pos(), "expression is read-only")
				}
			}
		}
	case *ir.ExprStmt:
		if isUnknownExprType(stmt.X) {
			stmt.X = c.checkExpr(stmt.X)
			stmt.X = c.finalizeExpr(stmt.X, nil)
			tx := stmt.X.Type()
			if tx.Kind() == ir.TModule {
				c.nodeError(stmt.X, "invalid expression (type '%s')", tx)
			}
		}
	default:
		panic(fmt.Sprintf("Unhandled stmt %T", stmt))
	}
}

func (c *checker) checkExpr2(expr ir.Expr, mode int) ir.Expr {
	prevMode := c.setMode(mode)
	expr = c.checkExpr(expr)
	c.mode = prevMode
	return expr
}

func (c *checker) checkExpr(expr ir.Expr) ir.Expr {
	if !isUnknownExprType(expr) {
		return expr
	}
	switch expr := expr.(type) {
	case *ir.Typeof:
		if isUnknownExprType(expr.X) {
			expr.X = c.checkExpr2(expr.X, modeExpr)
			expr.X = c.finalizeExpr(expr.X, nil)
			tx := expr.X.Type()
			if tx.Kind() == ir.TModule {
				c.nodeError(expr.X, "invalid expression (type '%s')", tx)
				expr.T = ir.TBuiltinInvalid
			} else {
				expr.T = tx
			}
		}
		return expr
	case *ir.PointerTypeExpr:
		return c.checkPointerTypeExpr(expr)
	case *ir.SliceTypeExpr:
		return c.checkSliceTypeExpr(expr)
	case *ir.ArrayTypeExpr:
		return c.checkArrayTypeExpr(expr)
	case *ir.FuncTypeExpr:
		return c.checkFuncTypeExpr(expr)
	case *ir.Ident:
		return c.checkIdent(expr)
	case *ir.ScopeLookup:
		return c.checkScopeLookup(expr)
	case *ir.DotExpr:
		return c.checkDotExpr(expr)
	case *ir.BasicLit:
		return c.checkBasicLit(expr)
	case *ir.ArrayLit:
		return c.checkArrayLit(expr)
	case *ir.BinaryExpr:
		return c.checkBinaryExpr(expr)
	case *ir.UnaryExpr:
		return c.checkUnaryExpr(expr)
	case *ir.AddrExpr:
		return c.checkAddrExpr(expr)
	case *ir.DerefExpr:
		return c.checkDerefExpr(expr)
	case *ir.IndexExpr:
		return c.checkIndexExpr(expr)
	case *ir.SliceExpr:
		return c.checkSliceExpr(expr)
	case *ir.AppExpr:
		return c.checkAppExpr(expr)
	case *ir.CastExpr:
		return c.checkCastExpr(expr)
	case *ir.LenExpr:
		return c.checkLenExpr(expr)
	case *ir.SizeofExpr:
		return c.checkSizeofExpr(expr)
	case *ir.ConstExpr:
		return expr
	default:
		panic(fmt.Sprintf("Unhandled expr %T at %s", expr, expr.Pos()))
	}
}

func (c *checker) finalizeExpr(expr ir.Expr, target ir.Type) ir.Expr {
	if isUntypedExpr(expr) || (target != nil && isUntyped(target)) {
		return expr
	}

	checkIncomplete := false
	texpr := expr.Type()

	switch expr.(type) {
	case *ir.AddrExpr:
		checkIncomplete = true
	case *ir.DerefExpr:
		checkIncomplete = true
	case *ir.SliceExpr:
		checkIncomplete = true
	}

	if checkIncomplete && isIncompleteType(texpr, nil) {
		c.nodeError(expr, "expression has incomplete type '%s'", texpr)
		expr.SetType(ir.TBuiltinInvalid)
		return expr
	}

	return ensureCompatibleType(expr, target)
}

func (c *checker) checkRootTypeExpr(expr ir.Expr, checkVoid bool) ir.Expr {
	expr = c.checkExpr2(expr, modeType)
	texpr := expr.Type()
	if !isUntyped(texpr) {
		if texpr.Kind() != ir.TVoid || checkVoid {
			if isIncompleteType(texpr, nil) {
				c.nodeError(expr, "incomplete type '%s'", texpr)
				expr.SetType(ir.TBuiltinInvalid)
				texpr = expr.Type()
			}
		}
	}
	return expr
}

func (c *checker) checkPointerTypeExpr(expr *ir.PointerTypeExpr) ir.Expr {
	expr.X = c.checkExpr2(expr.X, modeIndirectType)
	if tuntyped := checkUntypedExprs(expr.X); tuntyped != nil {
		expr.T = tuntyped
		return expr
	}
	ro := expr.Decl.Is(token.Val)
	tx := expr.X.Type()
	if tslice, ok := tx.(*ir.SliceType); ok {
		if !tslice.Ptr {
			tslice.Ptr = true
			tslice.ReadOnly = ro
			expr.T = tslice
		} else {
			expr.T = ir.NewPointerType(tx, ro)
		}
	} else {
		expr.T = ir.NewPointerType(tx, ro)
	}
	return expr
}

func (c *checker) checkSliceTypeExpr(expr *ir.SliceTypeExpr) ir.Expr {
	expr.X = c.checkExpr2(expr.X, modeIndirectType)
	tx := expr.X.Type()
	if isUntyped(tx) {
		expr.T = tx
	} else {
		expr.T = ir.NewSliceType(tx, true, false)
	}
	return expr
}

func (c *checker) checkArrayTypeExpr(expr *ir.ArrayTypeExpr) ir.Expr {
	if isUnknownExprType(expr.Size) {
		expr.Size = c.checkExpr2(expr.Size, modeExpr)
		expr.Size = c.finalizeExpr(expr.Size, nil)
	}
	expr.X = c.checkExpr(expr.X)
	if tuntyped := checkUntypedExprs(expr.Size, expr.X); tuntyped != nil {
		expr.T = tuntyped
		return expr
	}
	size := 0
	if !ir.IsIntegerType(expr.Size.Type()) {
		c.error(expr.Size.Pos(), "array size expects an integer type (got %s)", expr.Size.Type())
	} else if lit, ok := expr.Size.(*ir.BasicLit); !ok {
		c.error(expr.Size.Pos(), "array size is not a constant expression")
	} else if lit.NegatigeInteger() {
		c.error(expr.Size.Pos(), "array size cannot be negative")
	} else if lit.Zero() {
		c.error(expr.Size.Pos(), "array size cannot be zero")
	} else {
		size = int(lit.AsU64())
	}
	if size == 0 {
		expr.T = ir.TBuiltinInvalid
		return expr
	}
	tx := expr.X.Type()
	expr.T = ir.NewArrayType(tx, size)
	return expr
}

func (c *checker) checkFuncTypeExpr(expr *ir.FuncTypeExpr) ir.Expr {
	var params []ir.Field
	var tuntyped ir.Type
	for i, param := range expr.Params {
		expr.Params[i].Type = c.checkRootTypeExpr(param.Type, true)
		tparam := expr.Params[i].Type.Type()
		params = append(params, ir.Field{Name: param.Name.Literal, T: tparam})
		tuntyped = checkUntyped(tparam, tuntyped)
	}
	expr.Return.Type = c.checkRootTypeExpr(expr.Return.Type, false)
	if tuntyped = checkUntyped(expr.Return.Type.Type(), tuntyped); tuntyped != nil {
		expr.T = tuntyped
		return expr
	}
	cabi := false
	if expr.ABI != nil {
		if expr.ABI.Literal == ir.CABI {
			cabi = true
		} else if !ir.IsValidABI(expr.ABI.Literal) {
			c.error(expr.ABI.Pos(), "unknown abi '%s'", expr.ABI.Literal)
			expr.T = ir.TBuiltinInvalid
			return expr
		}
	}
	expr.T = ir.NewFuncType(params, expr.Return.Type.Type(), cabi)
	return expr
}

func (c *checker) resolveIdent(expr *ir.Ident) {
	if !isUnknownExprType(expr) {
		return
	}
	if expr.Sym == nil {
		expr.Sym = c.lookup(expr.Literal)
		if expr.Sym == nil {
			expr.T = ir.TBuiltinInvalid
			c.nodeError(expr, "unknown identifier '%s'", expr.Literal)
			return
		}
	}
	c.trySetDep(expr.Sym, true)
	if isUntyped(expr.Sym.T) {
		expr.T = expr.Sym.T
		return
	}
	sym := expr.Sym
	valid := true
	if c.mode != modeBoth {
		if c.isTypeMode() {
			if sym.Kind != ir.TypeSymbol {
				valid = false
				c.nodeError(expr, "'%s' is not a type", sym.Name)
			}
		} else {
			if sym.Kind == ir.TypeSymbol {
				valid = false
				c.nodeError(expr, "type '%s' cannot be used in expression", sym.T)
			}
		}
	}
	if valid {
		if !sym.Public && sym.CUID != c.object.CUID() {
			valid = false
			c.nodeError(expr, "'%s' is private and cannot be accessed from a different compilation unit", expr.Literal)
		}
	}
	expr.T = ir.TBuiltinInvalid
	if valid {
		expr.T = expr.Sym.T
	}
}

func (c *checker) resolveScopeLookup(expr *ir.ScopeLookup) {
	defer c.setScope(c.scope)
	if expr.Mode == ir.AbsLookup {
		c.scope = c.objectList.rootScope
	}
	defer c.setMode(c.mode)
	prevMode := c.mode
	for i, part := range expr.Parts {
		c.mode = modeBoth
		if (i + 1) >= len(expr.Parts) {
			c.mode = prevMode
		}
		c.resolveIdent(part)
		if isUntyped(part.T) {
			expr.T = part.T
			break
		}
		valid := true
		sym := part.Sym
		if (i + 1) < len(expr.Parts) {
			if tsym, ok := ir.ToBaseType(sym.T).(ir.TypeScope); ok {
				if sym.Kind == ir.TypeSymbol || sym.Kind == ir.ModuleSymbol {
					c.scope = tsym.Scope()
				} else {
					c.error(part.Pos(), "scope operator can only be used on modules and types")
				}
			} else {
				valid = false
				c.error(part.Pos(), "scope operator cannot be used on type '%s'", sym.T)
			}
		} else {
			if sym.IsField() {
				valid = false
				c.error(part.Pos(), "scope operator cannot access field")
			} else {
				expr.T = sym.T
			}
		}
		if !valid {
			expr.T = ir.TBuiltinInvalid
			break
		}
	}
	if isUntyped(expr.T) {
		for _, part := range expr.Parts {
			if part.T == nil {
				part.T = expr.T
			}
		}
	}
}

func (c *checker) maybeConstExpr(ident *ir.Ident) ir.Expr {
	sym := ident.Sym
	if sym.IsBuiltin() && sym.IsConst() {
		res := &ir.ConstExpr{
			X: c.constMap[sym.Key],
		}
		res.T = sym.T
		res.SetRange(ident.Pos(), ident.EndPos())
		return res
	}
	return ident
}

func (c *checker) checkIdent(expr *ir.Ident) ir.Expr {
	c.resolveIdent(expr)
	if !isUntyped(expr.T) {
		return c.maybeConstExpr(expr)
	}
	return expr
}

func (c *checker) checkScopeLookup(expr *ir.ScopeLookup) ir.Expr {
	c.resolveScopeLookup(expr)
	if !isUntyped(expr.T) {
		last := expr.Last()
		last.SetRange(expr.Pos(), expr.EndPos())
		return c.maybeConstExpr(last)
	}
	return expr
}
