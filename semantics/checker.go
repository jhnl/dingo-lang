package semantics

import (
	"fmt"
	"math/big"

	"github.com/jhnl/dingo/common"
	"github.com/jhnl/dingo/ir"
	"github.com/jhnl/dingo/token"
)

const (
	exprModeNone = 0
	exprModeType = 1
	exprModeFunc = 2
)

var builtinScope = ir.NewScope(ir.RootScope, nil)

func addBuiltinType(t ir.Type) {
	sym := &ir.Symbol{}
	sym.ID = ir.TypeSymbol
	sym.T = t
	sym.Name = t.ID().String()
	sym.Pos = token.NoPosition
	builtinScope.Insert(sym)
}

func init() {
	addBuiltinType(ir.TBuiltinVoid)
	addBuiltinType(ir.TBuiltinBool)
	addBuiltinType(ir.TBuiltinUInt64)
	addBuiltinType(ir.TBuiltinInt64)
	addBuiltinType(ir.TBuiltinUInt32)
	addBuiltinType(ir.TBuiltinInt32)
	addBuiltinType(ir.TBuiltinUInt16)
	addBuiltinType(ir.TBuiltinInt16)
	addBuiltinType(ir.TBuiltinUInt8)
	addBuiltinType(ir.TBuiltinInt8)
	addBuiltinType(ir.TBuiltinFloat64)
	addBuiltinType(ir.TBuiltinFloat32)
}

type checker struct {
	set    *ir.ModuleSet
	errors *common.ErrorList

	// State that changes when visiting nodes
	scope   *ir.Scope
	mod     *ir.Module
	fileCtx *ir.FileContext
	topDecl ir.TopDecl
}

func newChecker(set *ir.ModuleSet) *checker {
	c := &checker{set: set, scope: builtinScope}
	c.errors = &common.ErrorList{}
	return c
}

func (c *checker) resetWalkState() {
	c.mod = nil
	c.fileCtx = nil
	c.topDecl = nil
}

func (c *checker) openScope(id ir.ScopeID) {
	c.scope = ir.NewScope(id, c.scope)
}

func (c *checker) closeScope() {
	c.scope = c.scope.Outer
}

func setScope(c *checker, scope *ir.Scope) (*checker, *ir.Scope) {
	curr := c.scope
	c.scope = scope
	return c, curr
}

func (c *checker) visibilityScope(tok token.Token) *ir.Scope {
	return c.mod.Scope
}

func (c *checker) setTopDecl(decl ir.TopDecl) {
	c.topDecl = decl
	c.fileCtx = decl.Context()
}

func (c *checker) error(pos token.Position, format string, args ...interface{}) {
	filename := ""
	if c.fileCtx != nil {
		filename = c.fileCtx.Path
	}
	c.errors.Add(filename, pos, common.GenericError, format, args...)
}

func (c *checker) insert(scope *ir.Scope, id ir.SymbolID, name string, pos token.Position, src ir.Decl) *ir.Symbol {
	sym := ir.NewSymbol(id, scope.ID, name, pos, src)
	if existing := scope.Insert(sym); existing != nil {
		msg := fmt.Sprintf("redeclaration of '%s', previously declared at %s", name, existing.Pos)
		c.error(pos, msg)
		return nil
	}
	return sym
}

func (c *checker) lookup(name string) *ir.Symbol {
	if existing := c.scope.Lookup(name); existing != nil {
		return existing
	}
	return nil
}

func (c *checker) sortDecls() {
	for _, mod := range c.set.Modules {
		for _, decl := range mod.Decls {
			decl.SetNodeColor(ir.NodeColorWhite)
		}
	}

	for _, mod := range c.set.Modules {
		var sortedDecls []ir.TopDecl
		for _, decl := range mod.Decls {
			sym := decl.Symbol()
			if sym == nil {
				continue
			}

			var cycleTrace []ir.TopDecl
			if !sortDeclDependencies(decl, &cycleTrace, &sortedDecls) {
				// Report most specific cycle
				i, j := 0, len(cycleTrace)-1
				for ; i < len(cycleTrace) && j >= 0; i, j = i+1, j-1 {
					if cycleTrace[i] == cycleTrace[j] {
						break
					}
				}

				if i < j {
					decl = cycleTrace[j]
					cycleTrace = cycleTrace[i:j]
				}

				sym.Flags |= ir.SymFlagDepCycle

				trace := common.NewTrace(fmt.Sprintf("%s uses:", sym.Name), nil)
				for i := len(cycleTrace) - 1; i >= 0; i-- {
					s := cycleTrace[i].Symbol()
					s.Flags |= ir.SymFlagDepCycle
					line := cycleTrace[i].Context().Path + ":" + s.Name
					trace.Lines = append(trace.Lines, line)
				}

				errorMsg := "initializer cycle detected"
				if sym.ID == ir.TypeSymbol {
					errorMsg = "type cycle detected"
				}

				c.errors.AddTrace(decl.Context().Path, sym.Pos, common.GenericError, trace, errorMsg)
			}
		}
		mod.Decls = sortedDecls
	}
}

// Returns false if cycle
func sortDeclDependencies(decl ir.TopDecl, trace *[]ir.TopDecl, sortedDecls *[]ir.TopDecl) bool {
	color := decl.NodeColor()
	if color == ir.NodeColorBlack {
		return true
	} else if color == ir.NodeColorGray {
		return false
	}

	sortOK := true
	decl.SetNodeColor(ir.NodeColorGray)
	for _, dep := range decl.Dependencies() {
		if !sortDeclDependencies(dep, trace, sortedDecls) {
			*trace = append(*trace, dep)
			sortOK = false
			break
		}
	}
	decl.SetNodeColor(ir.NodeColorBlack)
	*sortedDecls = append(*sortedDecls, decl)
	return sortOK
}

func (c *checker) checkCompleteType(t1 ir.Type) bool {
	complete := true
	switch t2 := t1.(type) {
	case *ir.SliceType:
		if !t2.Ptr {
			complete = false
		}
	}
	return complete
}

// Returns false if an error should be reported
func (c *checker) checkTypes(t1 ir.Type, t2 ir.Type) bool {
	if ir.IsUntyped(t1) || ir.IsUntyped(t2) {
		// TODO: Improve assert to check that an actual type error was reported for t1 and/or t2
		common.Assert(c.errors.Count() > 0, "t1 or t2 are untyped and no error was reported")
		return true
	}
	return t1.Equals(t2)
}

type numericCastResult int

const (
	numericCastOK numericCastResult = iota
	numericCastFails
	numericCastOverflows
	numericCastTruncated
)

func toBigFloat(val *big.Int) *big.Float {
	res := big.NewFloat(0)
	res.SetInt(val)
	return res
}

func toBigInt(val *big.Float) *big.Int {
	if !val.IsInt() {
		return nil
	}
	res := big.NewInt(0)
	val.Int(res)
	return res
}

func integerOverflows(val *big.Int, t ir.TypeID) bool {
	fits := true

	switch t {
	case ir.TBigInt:
		// OK
	case ir.TUInt64:
		fits = 0 <= val.Cmp(ir.BigIntZero) && val.Cmp(ir.MaxU64) <= 0
	case ir.TUInt32:
		fits = 0 <= val.Cmp(ir.BigIntZero) && val.Cmp(ir.MaxU32) <= 0
	case ir.TUInt16:
		fits = 0 <= val.Cmp(ir.BigIntZero) && val.Cmp(ir.MaxU16) <= 0
	case ir.TUInt8:
		fits = 0 <= val.Cmp(ir.BigIntZero) && val.Cmp(ir.MaxU8) <= 0
	case ir.TInt64:
		fits = 0 <= val.Cmp(ir.MinI64) && val.Cmp(ir.MaxI64) <= 0
	case ir.TInt32:
		fits = 0 <= val.Cmp(ir.MinI32) && val.Cmp(ir.MaxI32) <= 0
	case ir.TInt16:
		fits = 0 <= val.Cmp(ir.MinI16) && val.Cmp(ir.MaxI16) <= 0
	case ir.TInt8:
		fits = 0 <= val.Cmp(ir.MinI8) && val.Cmp(ir.MaxI8) <= 0
	}

	return !fits
}

func floatOverflows(val *big.Float, t ir.TypeID) bool {
	fits := true

	switch t {
	case ir.TBigFloat:
		// OK
	case ir.TFloat64:
		fits = 0 <= val.Cmp(ir.MinF64) && val.Cmp(ir.MaxF64) <= 0
	case ir.TFloat32:
		fits = 0 <= val.Cmp(ir.MinF32) && val.Cmp(ir.MaxF32) <= 0
	}

	return !fits
}

func typeCastNumericLit(lit *ir.BasicLit, target ir.Type) numericCastResult {
	res := numericCastOK
	id := target.ID()

	switch t := lit.Raw.(type) {
	case *big.Int:
		switch id {
		case ir.TBigInt, ir.TUInt64, ir.TUInt32, ir.TUInt16, ir.TUInt8, ir.TInt64, ir.TInt32, ir.TInt16, ir.TInt8:
			if integerOverflows(t, id) {
				res = numericCastOverflows
			}
		case ir.TBigFloat, ir.TFloat64, ir.TFloat32:
			fval := toBigFloat(t)
			if floatOverflows(fval, id) {
				res = numericCastOverflows
			} else {
				lit.Raw = fval
			}
		default:
			return numericCastFails
		}
	case *big.Float:
		switch id {
		case ir.TBigInt, ir.TUInt64, ir.TUInt32, ir.TUInt16, ir.TUInt8, ir.TInt64, ir.TInt32, ir.TInt16, ir.TInt8:
			if ival := toBigInt(t); ival != nil {
				if integerOverflows(ival, id) {
					res = numericCastOverflows
				} else {
					lit.Raw = ival
				}
			} else {
				res = numericCastTruncated
			}
		case ir.TBigFloat, ir.TFloat64, ir.TFloat32:
			if floatOverflows(t, id) {
				res = numericCastOverflows
			}
		default:
			return numericCastFails
		}
	default:
		return numericCastFails
	}

	if res == numericCastOK {
		lit.T = ir.NewBasicType(id)
	}

	return res
}
