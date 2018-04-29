package semantics

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/jhnl/dingo/common"
	"github.com/jhnl/dingo/ir"
	"github.com/jhnl/dingo/token"
)

// TODO: Evaluate constant boolean expressions

func (v *typeChecker) VisitBinaryExpr(expr *ir.BinaryExpr) ir.Expr {
	expr.Left = ir.VisitExpr(v, expr.Left)
	expr.Right = ir.VisitExpr(v, expr.Right)

	leftType := expr.Left.Type()
	rightType := expr.Right.Type()

	binType := ir.TUntyped
	boolOp := expr.Op.OneOf(token.Eq, token.Neq, token.Gt, token.GtEq, token.Lt, token.LtEq)
	arithOp := expr.Op.OneOf(token.Add, token.Sub, token.Mul, token.Div, token.Mod)
	typeNotSupported := ir.TBuiltinUntyped

	if expr.Op.OneOf(token.Land, token.Lor) {
		if leftType.ID() != ir.TBool || rightType.ID() != ir.TBool {
			v.c.errorExpr(expr, "type mismatch: expression has types %s and %s (expected %s)", leftType, rightType, ir.TBool)
		} else {
			binType = ir.TBool
		}
	} else if boolOp || arithOp {
		leftLit, _ := expr.Left.(*ir.BasicLit)
		rightLit, _ := expr.Right.(*ir.BasicLit)

		if ir.IsNumericType(leftType) && ir.IsNumericType(rightType) {
			var leftBigInt *big.Int
			var leftBigFloat *big.Float
			var rightBigInt *big.Int
			var rightBigFloat *big.Float

			if leftLit != nil {
				leftBigInt, _ = leftLit.Raw.(*big.Int)
				leftBigFloat, _ = leftLit.Raw.(*big.Float)
			}

			if rightLit != nil {
				rightBigInt, _ = rightLit.Raw.(*big.Int)
				rightBigFloat, _ = rightLit.Raw.(*big.Float)
			}

			// Check division by zero

			if expr.Op.ID == token.Div || expr.Op.ID == token.Mod {
				if (rightBigInt != nil && rightBigInt.Cmp(ir.BigIntZero) == 0) ||
					(rightBigFloat != nil && rightBigFloat.Cmp(ir.BigFloatZero) == 0) {
					v.c.errorExpr(rightLit, "division by zero")
					expr.T = ir.NewBasicType(ir.TUntyped)
					return expr
				}
			}

			// Convert integer literals to floats

			if leftBigInt != nil && rightBigFloat != nil {
				leftBigFloat = big.NewFloat(0)
				leftBigFloat.SetInt(leftBigInt)
				leftLit.Raw = leftBigFloat
				leftLit.T = ir.NewBasicType(ir.TBigFloat)
				leftType = leftLit.T
				leftBigInt = nil
			}

			if rightBigInt != nil && leftBigFloat != nil {
				rightBigFloat = big.NewFloat(0)
				rightBigFloat.SetInt(rightBigInt)
				rightLit.Raw = rightBigFloat
				rightLit.T = ir.NewBasicType(ir.TBigFloat)
				rightType = rightLit.T
				rightBigInt = nil
			}

			bigIntOperands := (leftBigInt != nil && rightBigInt != nil)
			bigFloatOperands := (leftBigFloat != nil && rightBigFloat != nil)

			if bigIntOperands || bigFloatOperands {
				cmpRes := 0
				if bigIntOperands {
					cmpRes = leftBigInt.Cmp(rightBigInt)
				} else {
					cmpRes = leftBigFloat.Cmp(rightBigFloat)
				}

				boolRes := false
				switch expr.Op.ID {
				case token.Eq:
					boolRes = (cmpRes == 0)
				case token.Neq:
					boolRes = (cmpRes != 0)
				case token.Gt:
					boolRes = (cmpRes > 0)
				case token.GtEq:
					boolRes = (cmpRes >= 0)
				case token.Lt:
					boolRes = (cmpRes < 0)
				case token.LtEq:
					boolRes = (cmpRes <= 0)
				default:
					if bigIntOperands {
						switch expr.Op.ID {
						case token.Add:
							leftBigInt.Add(leftBigInt, rightBigInt)
						case token.Sub:
							leftBigInt.Sub(leftBigInt, rightBigInt)
						case token.Mul:
							leftBigInt.Mul(leftBigInt, rightBigInt)
						case token.Div:
							leftBigInt.Div(leftBigInt, rightBigInt)
						case token.Mod:
							leftBigInt.Mod(leftBigInt, rightBigInt)
						default:
							panic(fmt.Sprintf("Unhandled binop %s", expr.Op.ID))
						}
					} else {
						switch expr.Op.ID {
						case token.Add:
							leftBigFloat.Add(leftBigFloat, rightBigFloat)
						case token.Sub:
							leftBigFloat.Sub(leftBigFloat, rightBigFloat)
						case token.Mul:
							leftBigFloat.Mul(leftBigFloat, rightBigFloat)
						case token.Div:
							leftBigFloat.Quo(leftBigFloat, rightBigFloat)
						case token.Mod:
							typeNotSupported = leftType
						default:
							panic(fmt.Sprintf("Unhandled binop %s", expr.Op.ID))
						}
					}
				}

				if ir.IsTypeID(typeNotSupported, ir.TUntyped) {
					if boolOp {
						if boolRes {
							leftLit.Tok.ID = token.True
						} else {
							leftLit.Tok.ID = token.False
						}
						leftLit.T = ir.NewBasicType(ir.TBool)
						leftLit.Raw = nil
					}

					leftLit.Value = "(" + leftLit.Value + " " + expr.Op.String() + " " + rightLit.Value + ")"
					leftLit.Rewrite++
					return leftLit
				}
			} else if leftBigInt != nil && rightBigInt == nil {
				typeCastNumericLit(leftLit, rightType)
				leftType = leftLit.T
			} else if leftBigInt == nil && rightBigInt != nil {
				typeCastNumericLit(rightLit, leftType)
				rightType = rightLit.T
			} else if leftBigFloat != nil && rightBigFloat == nil {
				typeCastNumericLit(leftLit, rightType)
				leftType = leftLit.T
			} else if leftBigFloat == nil && rightBigFloat != nil {
				typeCastNumericLit(rightLit, leftType)
				rightType = rightLit.T
			}
		} else if leftType.ID() == ir.TPointer && rightType.ID() == ir.TPointer {
			if arithOp {
				typeNotSupported = leftType
			}

			leftPtr := leftType.(*ir.PointerType)
			rightPtr := rightType.(*ir.PointerType)
			leftNull := leftLit != nil && leftLit.Tok.Is(token.Null)
			rightNull := rightLit != nil && rightLit.Tok.Is(token.Null)

			if leftNull && rightNull {
				leftLit.T = ir.NewPointerType(ir.NewBasicType(ir.TUInt8), false)
				leftType = leftLit.T
				rightLit.T = ir.NewPointerType(ir.NewBasicType(ir.TUInt8), false)
				rightType = rightLit.T
			} else if leftNull {
				leftLit.T = ir.NewPointerType(rightPtr.Underlying, false)
				leftType = leftLit.T
			} else if rightNull {
				rightLit.T = ir.NewPointerType(leftPtr.Underlying, false)
				rightType = rightLit.T
			}
		} else if leftType.ID() == ir.TBool && rightType.ID() == ir.TBool {
			if arithOp || expr.Op.OneOf(token.Gt, token.GtEq, token.Lt, token.LtEq) {
				typeNotSupported = leftType
			}
		} else if leftType.Equals(rightType) {
			typeNotSupported = leftType
		}

		if !checkTypes(v.c, leftType, rightType) {
			v.c.errorExpr(expr, "type mismatch %s and %s", leftType, rightType)
		} else if !ir.IsUntyped(leftType) && !ir.IsUntyped(rightType) {
			if !ir.IsTypeID(typeNotSupported, ir.TUntyped) {
				v.c.errorExpr(expr, "operation '%s' cannot be performed on type %s", expr.Op.ID.String(), typeNotSupported)
			} else {
				if boolOp {
					binType = ir.TBool
				} else {
					binType = leftType.ID()
				}
			}
		}
	} else {
		panic(fmt.Sprintf("Unhandled binop %s", expr.Op.ID))
	}

	expr.T = ir.NewBasicType(binType)
	return expr
}

func (v *typeChecker) VisitUnaryExpr(expr *ir.UnaryExpr) ir.Expr {
	expr.X = ir.VisitExpr(v, expr.X)
	expr.T = expr.X.Type()
	switch expr.Op.ID {
	case token.Sub:
		if !ir.IsNumericType(expr.T) {
			v.c.error(expr.Op.Pos, "type mismatch: expression has type %s (expected integer or float)", expr.T)
		} else if lit, ok := expr.X.(*ir.BasicLit); ok {
			var raw interface{}

			switch n := lit.Raw.(type) {
			case *big.Int:
				raw = n.Neg(n)
			case *big.Float:
				raw = n.Neg(n)
			default:
				panic(fmt.Sprintf("Unhandled raw type %T", n))
			}

			lit.Tok.Pos = expr.Op.Pos
			if lit.Rewrite > 0 {
				lit.Value = "(" + lit.Value + ")"
			}
			lit.Value = expr.Op.String() + lit.Value
			lit.Rewrite++
			lit.Raw = raw
			return lit
		}
	case token.Lnot:
		if expr.T.ID() != ir.TBool {
			v.c.error(expr.Op.Pos, "type mismatch: expression has type %s (expected %s)", expr.T, ir.TBuiltinBool)
		}
	case token.Mul:
		lvalue := false

		if deref, ok := expr.X.(*ir.UnaryExpr); ok {
			if deref.Op.ID == token.And {
				// Inverse
				lvalue = deref.X.Lvalue()
			}
		} else {
			lvalue = expr.X.Lvalue()
		}

		if !lvalue {
			v.c.error(expr.X.Pos(), "expression cannot be dereferenced (not an lvalue)")
		} else {
			switch t := expr.T.(type) {
			case *ir.PointerType:
				expr.T = t.Underlying
			default:
				v.c.error(expr.X.Pos(), "expression cannot be dereferenced (has type %s)", expr.T)
			}
		}

		if expr.T == nil {
			expr.T = ir.TBuiltinUntyped
		}
	default:
		panic(fmt.Sprintf("Unhandled unary op %s", expr.Op.ID))
	}
	return expr
}

// TODO: Move to lexer and add better support for escape sequences.
func (v *typeChecker) unescapeStringLiteral(lit *ir.BasicLit) (string, bool) {
	escaped := []rune(lit.Value)
	var unescaped []rune

	start := 0
	n := len(escaped)

	// Remove quotes
	if n >= 2 {
		start++
		n--
	}

	for i := start; i < n; i++ {
		ch1 := escaped[i]
		if ch1 == '\\' && (i+1) < len(escaped) {
			i++
			ch2 := escaped[i]

			if ch2 == 'a' {
				ch1 = 0x07
			} else if ch2 == 'b' {
				ch1 = 0x08
			} else if ch2 == 'f' {
				ch1 = 0x0c
			} else if ch2 == 'n' {
				ch1 = 0x0a
			} else if ch2 == 'r' {
				ch1 = 0x0d
			} else if ch2 == 't' {
				ch1 = 0x09
			} else if ch2 == 'v' {
				ch1 = 0x0b
			} else {
				v.c.error(lit.Tok.Pos, "invalid escape sequence '\\%c'", ch2)
				return "", false
			}
		}
		unescaped = append(unescaped, ch1)
	}

	return string(unescaped), true
}

func removeUnderscores(lit string) string {
	res := strings.Replace(lit, "_", "", -1)
	return res
}

func (v *typeChecker) VisitBasicLit(expr *ir.BasicLit) ir.Expr {
	if expr.Tok.ID == token.False || expr.Tok.ID == token.True {
		expr.T = ir.TBuiltinBool
	} else if expr.Tok.ID == token.Char {
		if expr.Raw == nil {
			if raw, ok := v.unescapeStringLiteral(expr); ok {
				common.Assert(len(raw) == 1, "Unexpected length on char literal")

				val := big.NewInt(0)
				val.SetUint64(uint64(raw[0]))
				expr.Raw = val
				expr.T = ir.NewBasicType(ir.TBigInt)
			} else {
				expr.T = ir.TBuiltinUntyped
			}
		}
	} else if expr.Tok.ID == token.String {
		if expr.Raw == nil {
			if raw, ok := v.unescapeStringLiteral(expr); ok {
				if expr.Prefix == nil {
					expr.T = ir.NewSliceType(ir.TBuiltinInt8, true, true)
					expr.Raw = raw
				} else if expr.Prefix.Literal == "c" {
					expr.T = ir.NewPointerType(ir.TBuiltinInt8, true)
					expr.Raw = raw
				} else {
					v.c.error(expr.Prefix.Pos(), "invalid string prefix '%s'", expr.Prefix.Literal)
					expr.T = ir.TBuiltinUntyped
				}
			} else {
				expr.T = ir.TBuiltinUntyped
			}
		}
	} else if expr.Tok.ID == token.Integer {
		if expr.Raw == nil {
			base := ir.TBigInt
			target := ir.TBigInt

			if expr.Suffix != nil {
				switch expr.Suffix.Literal {
				case ir.TFloat64.String():
					base = ir.TBigFloat
					target = ir.TFloat64
				case ir.TFloat32.String():
					base = ir.TBigFloat
					target = ir.TFloat32
				case ir.TUInt64.String():
					target = ir.TUInt64
				case ir.TUInt32.String():
					target = ir.TUInt32
				case ir.TUInt16.String():
					target = ir.TUInt16
				case ir.TUInt8.String():
					target = ir.TUInt8
				case ir.TInt64.String():
					target = ir.TInt64
				case ir.TInt32.String():
					target = ir.TInt32
				case ir.TInt16.String():
					target = ir.TInt16
				case ir.TInt8.String():
					target = ir.TInt8
				default:
					v.c.error(expr.Suffix.Pos(), "invalid int suffix '%s'", expr.Suffix.Literal)
					base = ir.TUntyped
				}
			}

			if base != ir.TUntyped {
				normalized := removeUnderscores(expr.Value)

				if base == ir.TBigInt {
					val := big.NewInt(0)
					_, ok := val.SetString(normalized, 0)
					if ok {
						expr.Raw = val
					}
				} else if base == ir.TBigFloat {
					val := big.NewFloat(0)
					_, ok := val.SetString(normalized)
					if ok {
						expr.Raw = val
					}
				}

				if expr.Raw != nil {
					expr.T = ir.NewBasicType(base)
					if target != ir.TBigInt && target != ir.TBigFloat {
						v.tryMakeTypedLit(expr, ir.NewBasicType(target))
					}
				} else {
					v.c.error(expr.Tok.Pos, "unable to interpret int literal '%s'", normalized)
				}
			}

			if expr.T == nil {
				expr.T = ir.TBuiltinUntyped
			}
		}
	} else if expr.Tok.ID == token.Float {
		if expr.Raw == nil {
			base := ir.TBigFloat
			target := ir.TBigFloat

			if expr.Suffix != nil {
				switch expr.Suffix.Literal {
				case ir.TFloat64.String():
					target = ir.TFloat64
				case ir.TFloat32.String():
					target = ir.TFloat32
				default:
					v.c.error(expr.Suffix.Pos(), "invalid float suffix '%s'", expr.Suffix.Literal)
					base = ir.TUntyped
				}
			}

			if base != ir.TUntyped {
				val := big.NewFloat(0)
				normalized := removeUnderscores(expr.Value)
				_, ok := val.SetString(normalized)
				if ok {
					expr.T = ir.NewBasicType(base)
					expr.Raw = val

					if target != ir.TBigFloat {
						v.tryMakeTypedLit(expr, ir.NewBasicType(target))
					}
				} else {
					v.c.error(expr.Tok.Pos, "unable to interpret float literal '%s'", normalized)
				}
			}

			if expr.T == nil {
				expr.T = ir.TBuiltinUntyped
			}
		}
	} else if expr.Tok.ID == token.Null {
		expr.T = ir.NewPointerType(ir.TBuiltinUntyped, false)
	} else {
		panic(fmt.Sprintf("Unhandled literal %s", expr.Tok.ID))
	}

	return expr
}

func (v *typeChecker) VisitStructLit(expr *ir.StructLit) ir.Expr {
	expr.Name = v.visitType(expr.Name)
	t := expr.Name.Type()

	if ir.IsUntyped(t) {
		expr.T = ir.TBuiltinUntyped
		return expr
	} else if typeSym := ir.ExprSymbol(expr.Name); typeSym != nil {
		if typeSym.ID != ir.TypeSymbol && typeSym.T.ID() != ir.TStruct {
			v.c.error(expr.Name.Pos(), "'%s' is not a struct", typeSym.Name)
			expr.T = ir.TBuiltinUntyped
			return expr
		}
	}

	err := false
	inits := make(map[string]ir.Expr)
	structt, _ := t.(*ir.StructType)

	for _, kv := range expr.Initializers {
		if existing, ok := inits[kv.Key.Literal]; ok {
			if existing != nil {
				v.c.error(kv.Key.Pos(), "duplicate field key '%s'", kv.Key.Literal)
			}
			inits[kv.Key.Literal] = nil
			err = true
			continue
		}

		fieldSym := structt.Scope.Lookup(kv.Key.Literal)
		if fieldSym == nil {
			v.c.error(kv.Key.Pos(), "'%s' undefined struct field", kv.Key.Literal)
			inits[kv.Key.Literal] = nil
			err = true
			continue
		}

		kv.Value = v.makeTypedExpr(kv.Value, fieldSym.T)

		if ir.IsUntyped(fieldSym.T) || ir.IsUntyped(kv.Value.Type()) {
			inits[kv.Key.Literal] = nil
			err = true
			continue
		}

		if !checkTypes(v.c, fieldSym.T, kv.Value.Type()) {
			v.c.error(kv.Key.Pos(), "type mismatch: field '%s' expects type %s but got %s",
				kv.Key.Literal, fieldSym.T, kv.Value.Type())
			inits[kv.Key.Literal] = nil
			err = true
			continue
		}
		inits[kv.Key.Literal] = kv.Value
	}

	if err {
		expr.T = ir.TBuiltinUntyped
		return expr
	}

	expr.T = structt
	return createStructLit(structt, expr)
}

func (v *typeChecker) VisitArrayLit(expr *ir.ArrayLit) ir.Expr {
	t := ir.TBuiltinUntyped
	backup := ir.TBuiltinUntyped

	for i, init := range expr.Initializers {
		init = ir.VisitExpr(v, init)
		expr.Initializers[i] = init

		if t == ir.TBuiltinUntyped && ir.IsActualType(init.Type()) {
			t = init.Type()
		}
		if backup == ir.TBuiltinUntyped && !ir.IsUntyped(init.Type()) {
			backup = init.Type()
		}
	}

	if t != ir.TBuiltinUntyped {
		for _, init := range expr.Initializers {
			if !v.tryMakeTypedLit(init, t) {
				break
			}
		}
	} else {
		t = backup
	}

	if t != ir.TBuiltinUntyped {
		for _, init := range expr.Initializers {
			if ir.IsUntyped(init.Type()) {
				t = ir.TBuiltinUntyped
			} else if !checkTypes(v.c, t, init.Type()) {
				v.c.error(init.Pos(), "type mismatch: array elements must be of the same type (expected %s, got %s)", t, init.Type())
				t = ir.TBuiltinUntyped
				break
			}
		}
	}

	if len(expr.Initializers) == 0 {
		v.c.error(expr.Lbrack.Pos, "array literal cannot have 0 elements")
		t = ir.TBuiltinUntyped
	}

	if t == ir.TBuiltinUntyped {
		expr.T = t
	} else {
		expr.T = ir.NewArrayType(t, len(expr.Initializers))
	}
	return expr
}

func createDefaultLit(t ir.Type) ir.Expr {
	if t.ID() == ir.TStruct {
		tstruct := t.(*ir.StructType)
		lit := &ir.StructLit{}
		lit.T = t
		return createStructLit(tstruct, lit)
	} else if t.ID() == ir.TArray {
		tarray := t.(*ir.ArrayType)
		lit := &ir.ArrayLit{}
		lit.T = tarray
		for i := 0; i < tarray.Size; i++ {
			init := createDefaultLit(tarray.Elem)
			lit.Initializers = append(lit.Initializers, init)
		}
		return lit
	}
	return createDefaultBasicLit(t)
}

func createDefaultBasicLit(t ir.Type) *ir.BasicLit {
	var lit *ir.BasicLit
	if ir.IsTypeID(t, ir.TBool) {
		lit = &ir.BasicLit{Tok: token.Synthetic(token.False), Value: token.False.String()}
		lit.T = ir.NewBasicType(ir.TBool)
	} else if ir.IsTypeID(t, ir.TUInt64, ir.TInt64, ir.TUInt32, ir.TInt32, ir.TUInt16, ir.TInt16, ir.TUInt8, ir.TInt8) {
		lit = &ir.BasicLit{Tok: token.Synthetic(token.Integer), Value: "0"}
		lit.Raw = ir.BigIntZero
		lit.T = ir.NewBasicType(t.ID())
	} else if ir.IsTypeID(t, ir.TFloat64, ir.TFloat32) {
		lit = &ir.BasicLit{Tok: token.Synthetic(token.Float), Value: "0"}
		lit.Raw = ir.BigFloatZero
		lit.T = ir.NewBasicType(t.ID())
	} else if ir.IsTypeID(t, ir.TSlice) {
		lit = &ir.BasicLit{Tok: token.Synthetic(token.Null), Value: token.Null.String()}
		slice := t.(*ir.SliceType)
		lit.T = ir.NewSliceType(slice.Elem, true, true)
	} else if ir.IsTypeID(t, ir.TPointer) {
		lit = &ir.BasicLit{Tok: token.Synthetic(token.Null), Value: token.Null.String()}
		ptr := t.(*ir.PointerType)
		lit.T = ir.NewPointerType(ptr.Underlying, true)
	} else if ir.IsTypeID(t, ir.TFunc) {
		lit = &ir.BasicLit{Tok: token.Synthetic(token.Null), Value: token.Null.String()}
		fun := t.(*ir.FuncType)
		lit.T = ir.NewFuncType(fun.Params, fun.Return, fun.C)
	} else if !ir.IsTypeID(t, ir.TUntyped) {
		panic(fmt.Sprintf("Unhandled init value for type %s", t.ID()))
	}
	return lit
}

func createStructLit(structt *ir.StructType, lit *ir.StructLit) *ir.StructLit {
	var initializers []*ir.KeyValue
	for _, f := range structt.Fields {
		name := f.Name()
		found := false
		for _, init := range lit.Initializers {
			if init.Key.Literal == name {
				initializers = append(initializers, init)
				found = true
				break
			}
		}
		if found {
			continue
		}
		kv := &ir.KeyValue{}
		kv.Key = ir.NewIdent2(token.Synthetic(token.Ident), name)

		kv.Value = createDefaultLit(f.T)
		initializers = append(initializers, kv)
	}
	lit.Initializers = initializers
	return lit
}
