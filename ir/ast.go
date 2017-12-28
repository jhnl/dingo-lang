package ir

import (
	"fmt"
	"math/big"

	"github.com/jhnl/dingo/token"
)

// NodeColor is used to color nodes during dfs when sorting dependencies.
type NodeColor int

// The node colors.
//
// White: node not visited
// Gray: node visit in progress
// Black: node visit finished
const (
	NodeColorWhite NodeColor = iota
	NodeColorGray
	NodeColorBlack
)

// AST flags.
const (
	AstFlagNoInit = 1 << 0
)

// Node interface.
type Node interface {
	FirstPos() token.Position
	node()
}

// Decl is the main interface for declaration nodes.
type Decl interface {
	Node
	declNode()
}

// TopDecl represents a top-level declaration.
type TopDecl interface {
	Decl
	Symbol() *Symbol

	SetContext(*FileContext)
	Context() *FileContext

	AddDependency(TopDecl)
	Dependencies() []TopDecl
	SetNodeColor(NodeColor)
	NodeColor() NodeColor
}

// Stmt is the main interface for statement nodes.
type Stmt interface {
	Node
	stmtNode()
}

// Expr is the main interface for expression nodes.
type Expr interface {
	Node
	Type() Type
	Lvalue() bool
	ReadOnly() bool
	exprNode()
}

type baseNode struct{}

func (n *baseNode) node() {}

// Declaration nodes.

type baseDecl struct {
	baseNode
}

func (d *baseDecl) declNode() {}

type baseTopDecl struct {
	baseDecl
	Sym *Symbol
	Ctx *FileContext

	Deps  []TopDecl // Dependencies don't cross module boundaries
	Color NodeColor
}

func (d *baseTopDecl) Symbol() *Symbol {
	return d.Sym
}

func (d *baseTopDecl) SetContext(ctx *FileContext) {
	d.Ctx = ctx
}

func (d *baseTopDecl) Context() *FileContext {
	return d.Ctx
}

func (d *baseTopDecl) AddDependency(dep TopDecl) {
	d.Deps = append(d.Deps, dep)
}

func (d *baseTopDecl) Dependencies() []TopDecl {
	return d.Deps
}

func (d *baseTopDecl) SetNodeColor(color NodeColor) {
	d.Color = color
}

func (d *baseTopDecl) NodeColor() NodeColor {
	return d.Color
}

type BadDecl struct {
	baseDecl
	From token.Token
	To   token.Token
}

func (d *BadDecl) FirstPos() token.Position { return d.From.Pos }

// A ModuleSet is a collection of modules that make up the program.
type ModuleSet struct {
	baseNode
	Modules []*Module
}

func (d *ModuleSet) FirstPos() token.Position { return token.NoPosition }

// A Module is a collection of files sharing the same namespace.
type Module struct {
	baseDecl
	Path  string
	Name  token.Token
	Scope *Scope
	Files []*File
	Decls []TopDecl
}

func (m *Module) FirstPos() token.Position { return token.NoPosition }

type File struct {
	Ctx      *FileContext
	Includes []*Include
}

// Path returns the file path.
func (f *File) Path() string {
	return f.Ctx.Path
}

type FileContext struct {
	Path   string
	Decl   token.Token
	Module token.Token
}

type Include struct {
	baseDecl
	Include token.Token
	Literal token.Token
	File    *File
}

func (i *Include) FirstPos() token.Position { return i.Include.Pos }

type ValDeclSpec struct {
	Decl        token.Token
	Name        token.Token
	Type        Expr
	Assign      token.Token
	Initializer Expr
}

type ValTopDecl struct {
	baseTopDecl
	ValDeclSpec
	Visibility token.Token
}

func (d *ValTopDecl) FirstPos() token.Position {
	if d.Visibility.Pos.IsValid() {
		return d.Visibility.Pos
	}
	return d.Decl.Pos
}

type ValDecl struct {
	baseDecl
	ValDeclSpec
	Sym   *Symbol
	Flags int
}

func (d *ValDecl) FirstPos() token.Position {
	return d.Decl.Pos
}

func (d *ValDecl) Init() bool {
	return (d.Flags & AstFlagNoInit) == 0
}

// FuncDecl represents a function (with body) or a function signature.
type FuncDecl struct {
	baseTopDecl
	Visibility token.Token
	Decl       token.Token
	Name       token.Token
	Lparen     token.Token
	Params     []*ValDecl
	Rparen     token.Token
	TReturn    Expr
	Body       *BlockStmt
	Scope      *Scope
}

func (d *FuncDecl) FirstPos() token.Position { return d.Decl.Pos }

func (d *FuncDecl) SignatureOnly() bool { return d.Body == nil }

// StructDecl represents a struct declaration.
type StructDecl struct {
	baseTopDecl
	Visibility token.Token
	Decl       token.Token
	Name       token.Token
	Lbrace     token.Token
	Fields     []*ValDecl
	Rbrace     token.Token
	Scope      *Scope
}

func (d *StructDecl) FirstPos() token.Position { return d.Decl.Pos }

// Statement nodes.

type baseStmt struct {
	baseNode
}

func (s *baseStmt) stmtNode() {}

// BadStmt is a placeholder node for a statement that failed parsing.
type BadStmt struct {
	baseStmt
	From token.Token
	To   token.Token
}

func (s *BadStmt) FirstPos() token.Position { return s.From.Pos }

type BlockStmt struct {
	baseStmt
	Lbrace token.Token
	Scope  *Scope
	Stmts  []Stmt
	Rbrace token.Token
}

func (s *BlockStmt) FirstPos() token.Position { return s.Lbrace.Pos }

type DeclStmt struct {
	baseStmt
	D Decl
}

func (s *DeclStmt) FirstPos() token.Position { return s.D.FirstPos() }

// IfStmt represents a chain of if/elif/else statements.
type IfStmt struct {
	baseStmt
	If   token.Token
	Cond Expr
	Body *BlockStmt
	Else Stmt // Optional
}

func (s *IfStmt) FirstPos() token.Position { return s.If.Pos }

type ForStmt struct {
	baseStmt
	For   token.Token
	Scope *Scope
	Init  *ValDecl
	Inc   Stmt
	Cond  Expr
	Body  *BlockStmt
}

func (s *ForStmt) FirstPos() token.Position { return s.For.Pos }

type ReturnStmt struct {
	baseStmt
	Return token.Token
	X      Expr
}

func (s *ReturnStmt) FirstPos() token.Position { return s.Return.Pos }

type BranchStmt struct {
	baseStmt
	Tok token.Token
}

func (s *BranchStmt) FirstPos() token.Position { return s.Tok.Pos }

type AssignStmt struct {
	baseStmt
	Left   Expr
	Assign token.Token
	Right  Expr
}

func (s *AssignStmt) FirstPos() token.Position { return s.Left.FirstPos() }

type ExprStmt struct {
	baseStmt
	X Expr
}

func (s *ExprStmt) FirstPos() token.Position { return s.X.FirstPos() }

// Expression nodes.

type baseExpr struct {
	baseNode
	T Type
}

func (x *baseExpr) exprNode() {}

func (x *baseExpr) Type() Type {
	if x.T == nil {
		return TBuiltinUntyped
	}
	return x.T
}

func (x *baseExpr) Lvalue() bool {
	return false
}

func (x *baseExpr) ReadOnly() bool {
	return false
}

type BadExpr struct {
	baseExpr
	From token.Token
	To   token.Token
}

func (x *BadExpr) FirstPos() token.Position { return x.From.Pos }

type PointerTypeExpr struct {
	baseExpr
	Pointer token.Token
	Decl    token.Token
	X       Expr
}

func (x *PointerTypeExpr) FirstPos() token.Position { return x.Pointer.Pos }

type ArrayTypeExpr struct {
	baseExpr
	Lbrack token.Token
	Size   Expr
	Colon  token.Token
	X      Expr
	Rbrack token.Token
}

func (x *ArrayTypeExpr) FirstPos() token.Position { return x.Lbrack.Pos }

type BinaryExpr struct {
	baseExpr
	Left  Expr
	Op    token.Token
	Right Expr
}

func (x *BinaryExpr) FirstPos() token.Position { return x.Left.FirstPos() }

type UnaryExpr struct {
	baseExpr
	Op token.Token
	X  Expr
}

func (x *UnaryExpr) FirstPos() token.Position { return x.Op.Pos }

func (x *UnaryExpr) Lvalue() bool {
	switch x.Op.ID {
	case token.Mul:
		return x.X.Lvalue()
	}
	return false
}

func (x *UnaryExpr) ReadOnly() bool {
	switch x.Op.ID {
	case token.Mul:
		t := x.X.Type()
		if t != nil {
			if tptr, ok := t.(*PointerType); ok {
				return tptr.ReadOnly
			}
		}
	}
	return false
}

type AddressExpr struct {
	baseExpr
	And  token.Token
	Decl token.Token
	X    Expr
}

func (x *AddressExpr) FirstPos() token.Position { return x.And.Pos }

type IndexExpr struct {
	baseExpr
	X      Expr
	Lbrack token.Token
	Index  Expr
	Rbrack token.Token
}

func (x *IndexExpr) FirstPos() token.Position { return x.X.FirstPos() }

func (x *IndexExpr) Lvalue() bool {
	return x.X.Lvalue()
}

func (x *IndexExpr) ReadOnly() bool {
	t := x.X.Type()
	if t != nil {
		if tslice, ok := t.(*SliceType); ok {
			return tslice.ReadOnly
		}
	}
	return x.X.ReadOnly()
}

type SliceExpr struct {
	baseExpr
	X      Expr
	Lbrack token.Token
	Start  Expr
	Colon  token.Token
	End    Expr
	Rbrack token.Token
}

func (x *SliceExpr) FirstPos() token.Position { return x.X.FirstPos() }

func (x *SliceExpr) Lvalue() bool {
	return x.X.Lvalue()
}

func (x *SliceExpr) ReadOnly() bool {
	t := x.X.Type()
	if t != nil {
		if tslice, ok := t.(*SliceType); ok {
			return tslice.ReadOnly
		}
	}
	return x.X.ReadOnly()
}

type BasicLit struct {
	baseExpr
	Value   token.Token
	Raw     interface{}
	Rewrite int
}

func (x *BasicLit) FirstPos() token.Position { return x.Value.Pos }

func (x *BasicLit) AsString() string {
	return x.Raw.(string)
}

func (x *BasicLit) AsU64() uint64 {
	bigInt := x.Raw.(*big.Int)
	return bigInt.Uint64()
}

func (x *BasicLit) NegatigeInteger() bool {
	bigInt := x.Raw.(*big.Int)
	return bigInt.Sign() < 0
}

func (x *BasicLit) Zero() bool {
	switch t := x.Raw.(type) {
	case *big.Int:
		return t.Uint64() == 0
	case *big.Float:
		val, _ := t.Float64()
		return val == 0
	}
	return false
}

func (x *BasicLit) AsF64() float64 {
	bigFloat := x.Raw.(*big.Float)
	val, _ := bigFloat.Float64()
	return val
}

type KeyValue struct {
	baseNode
	Key   token.Token
	Equal token.Token
	Value Expr
}

func (k *KeyValue) FirstPos() token.Position { return k.Key.Pos }

type StructLit struct {
	baseExpr
	Name         Expr // Ident or DotIdent
	Lbrace       token.Token
	Initializers []*KeyValue
	Rbrace       token.Token
}

func (x *StructLit) FirstPos() token.Position { return x.Name.FirstPos() }

type ArrayLit struct {
	baseExpr
	Lbrack       token.Token
	Initializers []Expr
	Rbrack       token.Token
}

func (x *ArrayLit) FirstPos() token.Position { return x.Lbrack.Pos }

type Ident struct {
	baseExpr
	Name token.Token
	Sym  *Symbol
}

func (x *Ident) FirstPos() token.Position { return x.Name.Pos }

func (x *Ident) Lvalue() bool {
	if x.Sym != nil && x.Sym.ID == ValSymbol {
		return true
	}
	return false
}

func (x *Ident) ReadOnly() bool {
	if x.Sym != nil {
		return x.Sym.ReadOnly()
	}
	return false
}

func (x *Ident) Literal() string {
	return x.Name.Literal
}
func (x *Ident) Pos() token.Position {
	return x.Name.Pos
}

func (x *Ident) SetSymbol(sym *Symbol) {
	x.Sym = sym
	if sym != nil {
		x.T = sym.T
	}
}

type DotExpr struct {
	baseExpr
	X    Expr
	Dot  token.Token
	Name *Ident
}

func (x *DotExpr) FirstPos() token.Position { return x.X.FirstPos() }

func (x *DotExpr) Lvalue() bool {
	return x.X.Lvalue()
}

func (x *DotExpr) ReadOnly() bool {
	return x.Name.ReadOnly() || x.X.ReadOnly()
}

type CastExpr struct {
	baseExpr
	Cast   token.Token
	Lparen token.Token
	ToTyp  Expr
	X      Expr
	Rparen token.Token
}

func (x *CastExpr) FirstPos() token.Position { return x.Cast.Pos }

type FuncCall struct {
	baseExpr
	X      Expr
	Lparen token.Token
	Args   []Expr
	Rparen token.Token
}

func (x *FuncCall) FirstPos() token.Position { return x.X.FirstPos() }

func TypeExprToIdent(expr Expr) *Ident {
	switch t := expr.(type) {
	case *Ident:
		return t
	case *PointerTypeExpr:
		return TypeExprToIdent(t.X)
	case *ArrayTypeExpr:
		return TypeExprToIdent(t.X)
	}
	return nil
}

func ExprSymbol(expr Expr) *Symbol {
	switch t := expr.(type) {
	case *Ident:
		return t.Sym
	case *DotExpr:
		return t.Name.Sym
	}
	return nil
}

func CopyExpr(expr Expr, includePositions bool) Expr {
	switch src := expr.(type) {
	case *Ident:
		dst := &Ident{}
		dst.Name = src.Name
		if !includePositions {
			dst.Name.Pos = token.NoPosition
		}
		dst.SetSymbol(src.Sym)
		return dst
	case *DotExpr:
		dst := &DotExpr{}
		dst.X = CopyExpr(src.X, includePositions)
		dst.Name = CopyExpr(src.Name, includePositions).(*Ident)
		dst.T = src.T
		return dst
	default:
		panic(fmt.Sprintf("Unhandled CopyExpr src %T", src))
	}
}
