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

package interp

// The parse tree builders the grammar actions call (parse.c and the helpers of awkgram.y). Every node records
// the program line the lexer has reached when it is made, which is the line runtime errors report.

func pp(l yyLexer) *parser { return l.(*parser) }

// Lex is the parser's lexer.
func (lx *parser) Lex(lval *yySymType) int { return lx.lex(lval) }

// Error reports a syntax error found by the parser.
func (lx *parser) Error(s string) { lx.in.syntax("%s", s) }

func (lx *parser) errorflag() int { return lx.in.errorflag }

func (lx *parser) setWinner(p *Node) {
	lx.in.winner = lx.stat3(PROGRAM, lx.beginloc, p, lx.endloc)
}

func (lx *parser) nodealloc(nobj int, ntype int, args ...*Node) *Node {
	x := &Node{nobj: nobj, ntype: ntype, lineno: lx.lineno}
	copy(x.args[:], args)
	return x
}

func (lx *parser) stat1(a int, b *Node) *Node          { return lx.nodealloc(a, nSTAT, b) }
func (lx *parser) stat2(a int, b, c *Node) *Node       { return lx.nodealloc(a, nSTAT, b, c) }
func (lx *parser) stat3(a int, b, c, d *Node) *Node    { return lx.nodealloc(a, nSTAT, b, c, d) }
func (lx *parser) stat4(a int, b, c, d, e *Node) *Node { return lx.nodealloc(a, nSTAT, b, c, d, e) }
func (lx *parser) op1(a int, b *Node) *Node            { return lx.nodealloc(a, nEXPR, b) }
func (lx *parser) op2(a int, b, c *Node) *Node         { return lx.nodealloc(a, nEXPR, b, c) }
func (lx *parser) op3(a int, b, c, d *Node) *Node      { return lx.nodealloc(a, nEXPR, b, c, d) }

// exptostat makes an expression into a statement.
func exptostat(a *Node) *Node {
	a.ntype = nSTAT
	return a
}

// newValueNode makes a leaf node for a cell, setting the cell's kind.
func newValueNode(c *Cell, csub uint8, lineno int) *Node {
	c.ctype = oCELL
	c.csub = csub
	return &Node{ntype: nVALUE, cell: c, lineno: lineno}
}

func (lx *parser) celltonode(c *Cell, csub uint8) *Node { return newValueNode(c, csub, lx.lineno) }

// rectonode is $0, as the expression $(0).
func (lx *parser) rectonode() *Node {
	return lx.op1(INDIRECT, lx.celltonode(lx.in.literal0, cUNK))
}

// makearr makes the variable in a value node into an array, as soon as the program uses it as one.
func (lx *parser) makearr(p *Node) *Node {
	if p.ntype == nVALUE {
		cp := p.cell
		if cp.isFcn() {
			lx.in.syntax("%s is a function, not an array", cp.nval)
		} else if !cp.isArr() {
			cp.arr = makeSymtab(nSymTab)
			cp.tval = tARR
		}
	}
	return p
}

// pa2stat makes a pat1, pat2 { action } statement; each gets a slot recording whether its range is active.
func (lx *parser) pa2stat(a, b, c *Node) *Node {
	x := lx.nodealloc(PASTAT2, nSTAT, a, b, c)
	x.iv = lx.paircnt
	if lx.paircnt >= pa2Num {
		lx.in.syntax("limited to %d pat,pat statements", pa2Num)
	}
	lx.paircnt++
	return x
}

// linkum chains b after a; nothing is linked once there has been an error.
func (lx *parser) linkum(a, b *Node) *Node {
	if lx.in.errorflag != 0 {
		return a
	}
	if a == nil {
		return b
	} else if b == nil {
		return a
	}
	c := a
	for c.next != nil {
		c = c.next
	}
	c.next = b
	return a
}

// defn makes a function: it turns on the function flag of v and records its body and parameter count.
func (lx *parser) defn(v *Cell, vl, st *Node) {
	if v.isArr() {
		lx.in.syntax("`%s' is an array name and a function name", v.nval)
		return
	}
	if lx.isarg(v.nval) != -1 {
		lx.in.syntax("`%s' is both function name and argument name", v.nval)
		return
	}
	v.tval = tFCN
	v.body = st
	n := 0
	for p := vl; p != nil; p = p.next {
		n++
	}
	v.fval = float64(n)
}

// isarg returns the index of s among the current function's parameters, or -1.
func (lx *parser) isarg(s string) int {
	n := 0
	for p := lx.arglist; p != nil; p = p.next {
		if p.cell.nval == s {
			return n
		}
		n++
	}
	return -1
}

func (lx *parser) setfname(p *Cell) {
	if p.isArr() {
		lx.in.syntax("%s is an array, not a function", p.nval)
	} else if p.isFcn() {
		lx.in.syntax("you can't define function %s more than once", p.nval)
	}
	lx.in.curfname = p.nval
}

func constnode(p *Node) bool { return p.ntype == nVALUE && p.cell.csub == cCON }

// strnode is the text of a constant: for a numeric constant, as written in the program.
func strnode(p *Node) string { return p.cell.sval }

// notnull makes an expression a condition: anything but a comparison or a logical operator is compared with
// the empty, zero value.
func (lx *parser) notnull(n *Node) *Node {
	switch n.nobj {
	case LE, LT, EQ, NE, GT, GE, BOR, AND, NOT:
		return n
	}
	return lx.op2(NE, n, lx.in.nullnode)
}

func (lx *parser) checkdup(vl *Node, cp *Cell) {
	for ; vl != nil; vl = vl.next {
		if cp.nval == vl.cell.nval {
			lx.in.syntax("duplicate argument %s", cp.nval)
			break
		}
	}
}

// matchop is target ~ /re/ (or !~), with the regular expression compiled now.
func (lx *parser) matchop(op int, target *Node, re string) *Node {
	x := lx.op3(op, nil, target, nil)
	x.re = lx.in.makedfa(re, false)
	return x
}

// matchopExpr is target ~ expr: a constant expression is compiled now, anything else at each match.
func (lx *parser) matchopExpr(op int, target, e *Node) *Node {
	if constnode(e) {
		return lx.matchop(op, target, strnode(e))
	}
	x := lx.op3(op, nil, target, e)
	x.dyn = true
	return x
}

func (lx *parser) matchfcn(target *Node, re string) *Node {
	x := lx.op3(MATCHFCN, nil, target, nil)
	x.re = lx.in.makedfa(re, true)
	return x
}

func (lx *parser) matchfcnExpr(target, e *Node) *Node {
	if constnode(e) {
		return lx.matchfcn(target, strnode(e))
	}
	x := lx.op3(MATCHFCN, nil, target, e)
	x.dyn = true
	return x
}

// getline is a getline expression: mode is 0 (from the current input), LT (< file) or '|' (cmd |).
func (lx *parser) getline(v *Node, mode int, src *Node) *Node {
	if mode == '|' && lx.in.safe {
		lx.in.syntax("cmd | getline is unsafe")
	}
	x := lx.op3(GETLINE, v, nil, src)
	x.iv = mode
	return x
}

// stat3print is a print or printf statement, with redir 0 or one of '|', APPEND and GT.
func (lx *parser) stat3print(op int, args *Node, redir int, dest *Node) *Node {
	if lx.in.safe {
		switch redir {
		case '|':
			lx.in.syntax("print | is unsafe")
		case APPEND:
			lx.in.syntax("print >> is unsafe")
		case GT:
			lx.in.syntax("print > is unsafe")
		}
	}
	x := lx.stat3(op, args, nil, dest)
	x.iv = redir
	return x
}

func (lx *parser) bltin(fn int, args *Node) *Node {
	x := lx.op2(BLTIN, nil, args)
	x.iv = fn
	return x
}

// split is split(s, a) or split(s, a, sep) with sep an expression.
func (lx *parser) split(s, a, sep *Node, _ string) *Node {
	x := lx.nodealloc(SPLIT, nEXPR, s, a, sep)
	x.iv = STRING
	return x
}

// splitRE is split(s, a, /re/).
func (lx *parser) splitRE(s, a *Node, re string) *Node {
	x := lx.nodealloc(SPLIT, nEXPR, s, a, nil)
	x.re = lx.in.makedfa(re, true)
	x.iv = REGEXPR
	return x
}

// subRE is sub or gsub with a /re/; target is $0 when not given.
func (lx *parser) subRE(op int, re string, repl, target *Node) *Node {
	x := lx.nodealloc(op, nEXPR, nil, nil, repl, target)
	x.re = lx.in.makedfa(re, true)
	return x
}

// subExpr is sub or gsub with an expression as the regular expression.
func (lx *parser) subExpr(op int, e, repl, target *Node) *Node {
	if constnode(e) {
		return lx.subRE(op, strnode(e), repl, target)
	}
	x := lx.nodealloc(op, nEXPR, nil, e, repl, target)
	x.dyn = true
	return x
}

// arg is the n-th parameter of the function being defined.
func (lx *parser) arg(n int) *Node {
	x := lx.op1(ARG, nil)
	x.iv = n
	return x
}

func (lx *parser) varnf(cp *Cell) *Node {
	x := lx.op1(VARNF, nil)
	x.cell = cp
	return x
}
