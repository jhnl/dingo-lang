package token

import (
	"fmt"
	"strconv"
)

// ID of token.
type ID int

// List of tokens.
//
const (
	Invalid ID = iota
	EOF
	Comment
	MultiComment

	Ident
	Integer
	Float
	String

	Lparen
	Rparen
	Lbrace
	Rbrace
	Lbrack
	Rbrack
	Dot
	Comma
	Semicolon
	Colon
	Underscore
	Directive

	And
	Lnot

	binopBeg
	Add
	Sub
	Mul
	Div
	Mod

	Land
	Lor

	Eq
	Neq
	Gt
	GtEq
	Lt
	LtEq
	binopEnd

	assignBeg
	Assign
	AddAssign
	SubAssign
	MulAssign
	DivAssign
	ModAssign
	assignEnd

	Inc
	Dec

	keywordBeg
	If
	Else
	Elif
	For
	While
	Return
	Continue
	Break
	Cast
	Lenof
	Module
	Require
	Import
	Var
	Val
	Func
	Struct
	Public
	Private

	True
	False
	Null
	keywordEnd
)

var tokens = [...]string{
	Invalid: "invalid",
	EOF:     "eof",
	Comment: "comment",

	Ident:   "ident",
	Integer: "integer",
	Float:   "float",
	String:  "string",

	Lparen:     "(",
	Rparen:     ")",
	Lbrace:     "{",
	Rbrace:     "}",
	Lbrack:     "[",
	Rbrack:     "]",
	Dot:        ".",
	Comma:      ",",
	Semicolon:  ";",
	Colon:      ":",
	Underscore: "_",
	Directive:  "@",

	And:  "&",
	Lnot: "!",

	Add: "+",
	Sub: "-",
	Mul: "*",
	Div: "/",
	Mod: "%",

	Land: "&&",
	Lor:  "||",

	Eq:   "==",
	Neq:  "!=",
	Gt:   ">",
	GtEq: ">=",
	Lt:   "<",
	LtEq: "<=",

	Assign:    "=",
	AddAssign: "+=",
	SubAssign: "-=",
	MulAssign: "*=",
	DivAssign: "/=",
	ModAssign: "%=",

	Inc: "++",
	Dec: "--",

	If:       "if",
	Else:     "else",
	Elif:     "elif",
	For:      "for",
	While:    "while",
	Return:   "return",
	Continue: "continue",
	Break:    "break",
	Cast:     "cast",
	Lenof:    "lenof",
	Module:   "module",
	Require:  "require",
	Import:   "import",
	Var:      "var",
	Val:      "val",
	Func:     "fun",
	Struct:   "struct",
	Public:   "pub",
	Private:  "priv",

	True:  "true",
	False: "false",
	Null:  "null",
}

var keywords map[string]ID

func init() {
	keywords = make(map[string]ID)
	for i := keywordBeg + 1; i < keywordEnd; i++ {
		keywords[tokens[i]] = i
	}
}

// Lookup returns the identifier's token ID.
//
func Lookup(ident string) ID {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return Ident
}

func (tok ID) String() string {
	s := ""
	if 0 <= tok && tok < ID(len(tokens)) {
		s = tokens[tok]
	}
	if s == "" {
		s = "token(" + strconv.Itoa(int(tok)) + ")"
	}
	return s
}

// Position of token in a file.
type Position struct {
	Offset int
	Line   int
	Column int
}

func NewPosition(line, column int) Position {
	return Position{Line: line, Column: column}
}

// NoPosition means it wasn't part of a file.
var NoPosition = Position{-1, -1, 0}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// IsValid returns true if it's a valid file position.
func (p Position) IsValid() bool {
	return p.Line > -1
}

// Token struct.
type Token struct {
	ID      ID
	Literal string
	Pos     Position
}

// Synthetic creates a token that does not have a representation in the source code.
func Synthetic(id ID, literal string) Token {
	return Token{ID: id, Literal: literal, Pos: Position{Line: -1, Column: -1}}
}

func (t Token) String() string {
	if t.ID == Ident || t.ID == String || t.ID == Integer || t.ID == Float {
		return fmt.Sprintf("%s: %s", t.Pos, t.Literal)
	}
	return fmt.Sprintf("%s: %v", t.Pos, t.ID)
}

func (t Token) Quote() string {
	s := strconv.Quote(t.Literal)
	i := len(s) - 1
	return s[1:i]
}

func (t Token) IsValid() bool {
	return t.Pos.IsValid()
}

// IsAssignOp returns true if the token represents an assignment operator:
// ('=', '+=', '-=', '*=', '/=', '%=')
func (t Token) IsAssignOp() bool {
	return assignBeg < t.ID && t.ID < assignEnd
}

// IsBinaryOp return true if the token represents a binary operator:
// ('+', '-', '*', '/', '%', '||', '&&', '!=', '==', '>', '>=', '<', '<=')
func (t Token) IsBinaryOp() bool {
	return binopBeg < t.ID && t.ID < binopEnd
}

// OneOf returns true if token one of the IDs match.
func (t Token) OneOf(ids ...ID) bool {
	for _, id := range ids {
		if t.ID == id {
			return true
		}
	}
	return false
}

// Is returns true if ID matches.
func (t Token) Is(id ID) bool {
	return t.ID == id
}
