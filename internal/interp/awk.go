/****************************************************************
Copyright (C) Lucent Technologies 1997
All Rights Reserved

Permission to use, copy, modify, and distribute this software and
its documentation for any purpose and without fee is hereby
granted, provided that the above copyright notice appear in all
copies and that both that the copyright notice and this
permission notice and warranty disclaimer appear in supporting
documentation, and that the name Lucent Technologies or any of
its entities not be used in advertising or publicity pertaining
to distribution of the software without specific, written prior
permission.

LUCENT DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE,
INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS.
IN NO EVENT SHALL LUCENT OR ANY OF ITS ENTITIES BE LIABLE FOR ANY
SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER
IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.
****************************************************************/

// Package interp is the awk interpreter: the lexer and grammar that turn program text into a parse tree, the
// tree walker that runs it over the input records, and the record, field, variable and I/O machinery around
// them. It is a port of the One True Awk (lex.c, awkgram.y, parse.c, tran.c, lib.c, run.c and main.c) and keeps
// the original's data model: every value is a Cell carrying a string and a number, with flags saying which of
// the two is current, and the parse tree is made of Nodes whose operator is a grammar token.
package interp

import "github.com/onetrueawk/awk/internal/regex"

// Cell.tval flags.
const (
	tNUM      = 0o1    // number value is valid
	tSTR      = 0o2    // string value is valid
	tDONTFREE = 0o4    // string is shared (a constant, a field or a lexer-made name): never reconverted
	tCON      = 0o10   // this is a constant
	tARR      = 0o20   // this is an array
	tFCN      = 0o40   // this is a function name
	tFLD      = 0o100  // this is a field $1, $2, ...
	tREC      = 0o200  // this is $0
	tCONVC    = 0o400  // string was converted from the number with CONVFMT
	tCONVO    = 0o1000 // string was converted from the number with OFMT
)

// Cell.ctype values.
const (
	oCELL = 1
	oBOOL = 2
	oJUMP = 3
)

// Cell.csub values: the kind of an ordinary cell, the truth of a boolean, or the kind of a jump.
const (
	cUNK  = 0
	cFLD  = 1
	cVAR  = 2
	cNAME = 3
	cTEMP = 4
	cCON  = 5
	cCOPY = 6

	bTRUE  = 11
	bFALSE = 12

	jEXIT     = 21
	jNEXT     = 22
	jBREAK    = 23
	jCONT     = 24
	jRET      = 25
	jNEXTFILE = 26
)

// Node.ntype values.
const (
	nVALUE = 1
	nSTAT  = 2
	nEXPR  = 3
)

// Builtin function numbers (the BLTIN token's value).
const (
	fLENGTH  = 1
	fSQRT    = 2
	fEXP     = 3
	fLOG     = 4
	fINT     = 5
	fSYSTEM  = 6
	fRAND    = 7
	fSRAND   = 8
	fSIN     = 9
	fCOS     = 10
	fATAN    = 11
	fTOUPPER = 12
	fTOLOWER = 13
	fFLUSH   = 14
)

// A Cell is a variable, constant, field, array element or temporary value. Its string and number are both
// stored; tval says which are valid. An array keeps its elements in arr, a function its body and parameter
// count in body and fval.
type Cell struct {
	ctype uint8
	csub  uint8
	nval  string // name, for variables, array elements and fields
	sval  string // string value
	fval  float64
	tval  int
	arr   *Array // elements, when tval has tARR
	body  *Node  // function body, when tval has tFCN
	sid   uint64 // identity of the current string value, renewed whenever sval is replaced
	fmtID uint64 // sid of the CONVFMT or OFMT string that last produced sval (CONVC/CONVO)
	cnext *Cell  // next cell in a hash chain or on the temporary list
}

func (c *Cell) isNum() bool  { return c.tval&tNUM != 0 }
func (c *Cell) isStr() bool  { return c.tval&tSTR != 0 }
func (c *Cell) isArr() bool  { return c.tval&tARR != 0 }
func (c *Cell) isFcn() bool  { return c.tval&tFCN != 0 }
func (c *Cell) isFld() bool  { return c.tval&tFLD != 0 }
func (c *Cell) isRec() bool  { return c.tval&tREC != 0 }
func (c *Cell) isTrue() bool { return c.csub == bTRUE }
func (c *Cell) isJump() bool { return c.ctype == oJUMP }
func (c *Cell) isExit() bool { return c.csub == jEXIT }
func (c *Cell) isBreak() bool {
	return c.csub == jBREAK
}
func (c *Cell) isCont() bool { return c.csub == jCONT }
func (c *Cell) isNext() bool { return c.csub == jNEXT || c.csub == jNEXTFILE }
func (c *Cell) isRet() bool  { return c.csub == jRET }

// An Array is a symbol table: a chained hash table whose bucket and chain order is the order in which
// "for (k in a)" visits the elements, so it is kept exactly as the original builds and grows it.
type Array struct {
	nelem int
	tab   []*Cell
}

// A Node is a parse tree node. nobj is the operator (a grammar token); args holds the operand subtrees.
// Operands that are not subtrees live in the other fields: the cell of a value node, a compiled regular
// expression, a small integer (a builtin number, a redirection operator, an argument index), the
// "dynamic regular expression" flag of match operators, and the regular expression text of index().
type Node struct {
	ntype  int
	next   *Node
	lineno int
	nobj   int
	args   [4]*Node
	cell   *Cell
	re     *regex.FA
	iv     int
	dyn    bool
	retext string
}

// pair-pattern limit (pat1, pat2) per program.
const pa2Num = 50
