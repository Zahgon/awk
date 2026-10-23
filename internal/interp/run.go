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

import (
	"math"
	"strconv"
	"strings"

	"github.com/onetrueawk/awk/internal/libc"
	"github.com/onetrueawk/awk/internal/u8"
)

// The tree walker (run.c). execute runs a node and returns a cell; statements return the True cell, or a jump
// cell for break, continue, next, nextfile and return, which the enclosing loops and calls act on. exit
// unwinds with a panic to the program's BEGIN, main loop or END, like the original's longjmp.

var (
	trueCell      = &Cell{ctype: oBOOL, csub: bTRUE, fval: 1, tval: tNUM}
	falseCell     = &Cell{ctype: oBOOL, csub: bFALSE, tval: tNUM}
	breakCell     = &Cell{ctype: oJUMP, csub: jBREAK, tval: tNUM}
	contCell      = &Cell{ctype: oJUMP, csub: jCONT, tval: tNUM}
	nextCell      = &Cell{ctype: oJUMP, csub: jNEXT, tval: tNUM}
	nextfileCell  = &Cell{ctype: oJUMP, csub: jNEXTFILE, tval: tNUM}
	retCell       = &Cell{ctype: oJUMP, csub: jRET, tval: tNUM}
	exitCellValue = &Cell{ctype: oJUMP, csub: jEXIT, tval: tNUM}
)

// exitJump unwinds the program for exit.
type exitJump struct{}

// nullName is what the original prints for the missing name of a temporary cell.
const nullName = "(null)"

// gettemp returns a fresh temporary cell, empty and zero.
func gettemp() *Cell {
	return &Cell{ctype: oCELL, csub: cTEMP, nval: nullName, tval: tNUM | tSTR | tDONTFREE, sid: nextSID()}
}

func boolCell(b bool) *Cell {
	if b {
		return trueCell
	}
	return falseCell
}

// run executes the program and closes everything.
func (in *Interp) run(a *Node) {
	in.stdinit()
	in.execute(a)
	in.closeall()
}

// execute runs the node list u, returning the value of an expression, or of the last statement or the first
// jump of a statement list.
func (in *Interp) execute(u *Node) *Cell {
	if u == nil {
		return trueCell
	}
	for a := u; ; a = a.next {
		in.curnode = a
		var x *Cell
		if a.ntype == nVALUE {
			x = a.cell
		} else {
			x = in.proc(a)
		}
		if x.isFld() && !in.donefld {
			in.fldbld()
		} else if x.isRec() && !in.donerec {
			in.recbld()
		}
		if a.ntype == nVALUE || a.ntype == nEXPR || x.isJump() || a.next == nil {
			return x
		}
	}
}

// proc dispatches a node to the routine for its operator.
func (in *Interp) proc(a *Node) *Cell {
	switch a.nobj {
	case PROGRAM:
		return in.program(a)
	case BOR, AND, NOT:
		return in.boolop(a)
	case NE, EQ, LE, LT, GE, GT:
		return in.relop(a)
	case ARRAY:
		return in.array(a)
	case INDIRECT:
		return in.indirect(a)
	case SUBSTR:
		return in.substr(a)
	case SUB, GSUB:
		return in.dosub(a)
	case INDEX:
		return in.sindex(a)
	case SPRINTF:
		return in.awksprintf(a)
	case ADD, MINUS, MULT, DIVIDE, MOD, UMINUS, UPLUS, POWER:
		return in.arith(a)
	case PREINCR, POSTINCR, PREDECR, POSTDECR:
		return in.incrdecr(a)
	case CAT:
		return in.cat(a)
	case PASTAT:
		return in.pastat(a)
	case PASTAT2:
		return in.dopa2(a)
	case MATCH, NOTMATCH, MATCHFCN:
		return in.matchop(a)
	case INTEST:
		return in.intest(a)
	case PRINTF:
		return in.awkprintf(a)
	case PRINT:
		return in.printstat(a)
	case CLOSE:
		return in.closeop(a)
	case DELETE:
		return in.awkdelete(a)
	case SPLIT:
		return in.split(a)
	case ASSIGN, ADDEQ, SUBEQ, MULTEQ, DIVEQ, MODEQ, POWEQ:
		return in.assign(a)
	case CONDEXPR:
		return in.condexpr(a)
	case IF:
		return in.ifstat(a)
	case WHILE:
		return in.whilestat(a)
	case FOR:
		return in.forstat(a)
	case DO:
		return in.dostat(a)
	case IN:
		return in.instat(a)
	case NEXT, NEXTFILE, EXIT, BREAK, CONTINUE, RETURN:
		return in.jump(a)
	case BLTIN:
		return in.bltin(a)
	case CALL:
		return in.call(a)
	case ARG:
		return in.arg(a)
	case VARNF:
		return in.getnf(a)
	case GETLINE:
		return in.awkgetline(a)
	}
	in.fatal("illegal statement")
	return nil
}

// program runs BEGIN, the main loop over the input records, and END; exit leaves BEGIN or the loop for END,
// and END itself.
func (in *Interp) program(a *Node) *Cell {
	if !in.runUntilExit(func() {
		if a.args[0] != nil { // BEGIN
			x := in.execute(a.args[0])
			if x.isExit() {
				return
			}
			if x.isJump() {
				in.fatal("illegal break, continue, next or nextfile from BEGIN")
			}
		}
		if a.args[1] != nil || a.args[2] != nil {
			for {
				if _, ok := in.getrec(true); !ok {
					break
				}
				if x := in.execute(a.args[1]); x.isExit() {
					break
				}
			}
		}
	}) {
		// exit: go on with END
	}
	in.runUntilExit(func() {
		if a.args[2] != nil { // END
			x := in.execute(a.args[2])
			if x.isBreak() || x.isNext() || x.isCont() {
				in.fatal("illegal break, continue, next or nextfile from END")
			}
		}
	})
	return trueCell
}

// runUntilExit runs f, stopping it at an exit statement; it reports whether f ran to its end.
func (in *Interp) runUntilExit(f func()) (completed bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(exitJump); !ok {
				panic(r)
			}
			completed = false
		}
	}()
	f()
	return true
}

// A frame is the stack frame of a function call.
type frame struct {
	fcncell *Cell
	args    []*Cell
	nargs   int // number of parameters (those past the arguments given are locals)
	retval  *Cell
}

const nArgs = 50 // limit on arguments plus parameters of a call

// call runs a user function. Scalars are passed by value, arrays by reference; a parameter the function
// turned into an array is given back to the caller's variable.
func (in *Interp) call(a *Node) *Cell {
	fcn := in.execute(a.args[0])
	s := fcn.nval
	if !fcn.isFcn() {
		in.fatal("calling undefined function %s", s)
	}
	ncall := 0
	for x := a.args[1]; x != nil; x = x.next {
		ncall++
	}
	ndef := int(fcn.fval)
	if ncall > ndef {
		in.warning("function %s called with %d args, uses only %d", s, ncall, ndef)
	}
	if ncall+ndef > nArgs {
		in.fatal("function %s has %d arguments, limit %d", s, ncall+ndef, nArgs)
	}
	args := make([]*Cell, 0, ndef+ncall)
	oargs := make([]*Cell, 0, ncall)
	for x := a.args[1]; x != nil; x = x.next {
		y := in.execute(x)
		oargs = append(oargs, y)
		if y.isFcn() {
			in.fatal("can't use function %s as argument in %s", y.nval, s)
		}
		if y.isArr() {
			args = append(args, y) // arrays by reference
		} else {
			args = append(args, copycell(y))
		}
	}
	for len(args) < ndef { // missing arguments are empty locals
		args = append(args, &Cell{ctype: oCELL, csub: cCOPY, nval: nullName, tval: tNUM | tSTR | tDONTFREE, sid: nextSID()})
	}
	fr := &frame{fcncell: fcn, args: args, nargs: ndef, retval: gettemp()}
	in.frames = append(in.frames, fr)
	y := in.execute(fcn.body)
	for i := 0; i < ndef; i++ {
		t := fr.args[i]
		if t.isArr() && t.csub == cCOPY && i < ncall {
			// the parameter became an array: so does the caller's variable
			o := oargs[i]
			o.tval = t.tval &^ (tSTR | tNUM | tDONTFREE)
			o.arr = t.arr
		}
	}
	if y.isExit() || y.isNext() {
		return y
	}
	in.frames = in.frames[:len(in.frames)-1]
	return fr.retval
}

// copycell copies a scalar argument for a call.
func copycell(x *Cell) *Cell {
	y := gettemp()
	y.tval = x.tval &^ (tCON | tFLD | tREC)
	y.csub = cCOPY // not freed until the call is over
	y.nval = x.nval
	if x.isStr() {
		y.setSval(x.sval)
		y.tval &^= tDONTFREE
	} else {
		y.tval |= tDONTFREE
	}
	y.fval = x.fval
	return y
}

func (in *Interp) frame() *frame { return in.frames[len(in.frames)-1] }

// arg returns the n-th parameter of the function being run.
func (in *Interp) arg(a *Node) *Cell {
	n := a.iv
	fr := in.frame()
	if n+1 > fr.nargs {
		in.fatal("argument #%d of function %s was not supplied", n+1, fr.fcncell.nval)
	}
	return fr.args[n]
}

// jump runs break, continue, next, nextfile, return and exit.
func (in *Interp) jump(a *Node) *Cell {
	switch a.nobj {
	case EXIT:
		if a.args[0] != nil {
			y := in.execute(a.args[0])
			in.errorflag = int(f2i32(in.getfval(y)))
		}
		panic(exitJump{})
	case RETURN:
		if a.args[0] != nil {
			y := in.execute(a.args[0])
			rv := in.frame().retval
			switch {
			case y.tval&(tSTR|tNUM) == tSTR|tNUM:
				in.setsval(rv, in.getsval(y))
				rv.fval = in.getfval(y)
				rv.tval |= tNUM
			case y.tval&tSTR != 0:
				in.setsval(rv, in.getsval(y))
			case y.tval&tNUM != 0:
				in.setfval(rv, in.getfval(y))
			default: // can't happen
				in.fatal("bad type variable %d", y.tval)
			}
		}
		return retCell
	case NEXT:
		return nextCell
	case NEXTFILE:
		in.nextfile()
		return nextfileCell
	case BREAK:
		return breakCell
	case CONTINUE:
		return contCell
	}
	in.fatal("illegal jump type %d", a.nobj)
	return nil
}

// awkgetline runs the forms of getline: from the current input, from a file, or from a command, into $0 or
// a variable. It returns 1, 0 at the end, or -1 when the file or command cannot be opened.
func (in *Interp) awkgetline(a *Node) *Cell {
	in.flushStdout() // in case someone is waiting for a prompt
	r := gettemp()
	var n int
	if a.iv != 0 { // getline <file, cmd | getline
		x := in.execute(a.args[2])
		mode := a.iv
		if mode == '|' {
			mode = LE
		}
		fp, newflag := in.openfile(mode, in.getsval(x))
		var buf string
		if fp == nil {
			n = -1
		} else {
			var ok bool
			if buf, ok = in.readrec(fp.in, newflag); ok {
				n = 1
			}
		}
		if n > 0 {
			if a.args[0] != nil { // getline var <file
				x := in.execute(a.args[0])
				in.setsval(x, buf)
				if r, ok := isNumber(x.sval); ok {
					x.fval = r
					x.tval |= tNUM
				}
			} else { // getline <file
				rec := in.fldtab[0]
				in.setsval(rec, buf)
				if r, ok := isNumber(rec.sval); ok {
					rec.fval = r
					rec.tval |= tNUM
				}
			}
		}
	} else if a.args[0] == nil { // getline
		if _, ok := in.getrec(true); ok {
			n = 1
		}
	} else { // getline var
		if buf, ok := in.getrec(false); ok {
			n = 1
			x := in.execute(a.args[0])
			in.setsval(x, buf)
			if r, ok := isNumber(x.sval); ok {
				x.fval = r
				x.tval |= tNUM
			}
		}
	}
	in.setfval(r, float64(n))
	return r
}

func (in *Interp) getnf(a *Node) *Cell {
	if !in.donefld {
		in.fldbld()
	}
	return a.cell
}

// makearraystring joins the subscripts of an array reference with SUBSEP.
func (in *Interp) makearraystring(p *Node) string {
	var b strings.Builder
	for ; p != nil; p = p.next {
		x := in.execute(p)
		s := in.getsval(x)
		sep := in.getsval(in.subseploc)
		b.WriteString(s)
		if p.next != nil {
			b.WriteString(sep)
		}
	}
	return b.String()
}

// toArray makes x into an empty array if it is not one.
func toArray(x *Cell) {
	if !x.isArr() {
		x.tval &^= tSTR | tNUM | tDONTFREE
		x.tval |= tARR
		x.arr = makeSymtab(nSymTab)
	}
}

// array is a[subscripts]; the element is created if needed.
func (in *Interp) array(a *Node) *Cell {
	x := in.execute(a.args[0])
	buf := in.makearraystring(a.args[1])
	toArray(x)
	z := x.arr.set(buf, "", 0, tSTR|tNUM)
	z.ctype = oCELL
	z.csub = cVAR
	return z
}

func (in *Interp) awkdelete(a *Node) *Cell {
	x := in.execute(a.args[0])
	if x == in.symtabloc {
		in.fatal("cannot delete SYMTAB or its elements")
	}
	if !x.isArr() {
		return trueCell
	}
	if a.args[1] == nil { // delete the elements, not the array
		freesymtab(x)
		x.tval &^= tSTR
		x.tval |= tARR
		x.arr = makeSymtab(nSymTab)
	} else {
		x.arr.remove(in.makearraystring(a.args[1]))
	}
	return trueCell
}

// intest is (subscripts) in a.
func (in *Interp) intest(a *Node) *Cell {
	ap := in.execute(a.args[1])
	toArray(ap)
	buf := in.makearraystring(a.args[0])
	return boolCell(ap.arr.lookup(buf) != nil)
}

// matchop is ~, !~ and match().
func (in *Interp) matchop(a *Node) *Cell {
	n := a.nobj
	x := in.execute(a.args[1])
	s := in.getsval(x)
	pfa := a.re
	if a.dyn {
		y := in.execute(a.args[2])
		pfa = in.makedfa(in.getsval(y), n == MATCHFCN)
	}
	if n != MATCHFCN {
		i := pfa.Match(s)
		return boolCell(i == (n == MATCH))
	}
	pstart, patlen := pfa.PMatch(s, true)
	start := 0
	if patlen >= 0 {
		start = u8.Byte2Char(s, pstart)
		chars := 0
		for i := 0; i < patlen; i += u8.NextLen(s[pstart+i:]) {
			chars++
		}
		patlen = chars
	}
	in.setfval(in.rstartloc, float64(start))
	in.setfval(in.rlengthloc, float64(patlen))
	x = gettemp()
	x.tval = tNUM
	x.fval = float64(start)
	return x
}

func (in *Interp) boolop(a *Node) *Cell {
	i := in.execute(a.args[0]).isTrue()
	switch a.nobj {
	case BOR:
		if i {
			return trueCell
		}
		return boolCell(in.execute(a.args[1]).isTrue())
	case AND:
		if !i {
			return falseCell
		}
		return boolCell(in.execute(a.args[1]).isTrue())
	}
	return boolCell(!i) // NOT
}

// relop compares two values: as numbers when both are numbers (strnums included), otherwise as strings.
func (in *Interp) relop(a *Node) *Cell {
	n := a.nobj
	x := in.execute(a.args[0])
	y := in.execute(a.args[1])
	xNaN := math.IsNaN(x.fval)
	yNaN := math.IsNaN(y.fval)
	var i int
	if x.tval&tNUM != 0 && y.tval&tNUM != 0 {
		if (xNaN || yNaN) && n != NE {
			return falseCell
		}
		switch j := x.fval - y.fval; {
		case j < 0:
			i = -1
		case j > 0:
			i = 1
		}
	} else {
		i = strings.Compare(in.getsval(x), in.getsval(y))
	}
	switch n {
	case LT:
		return boolCell(i < 0)
	case LE:
		return boolCell(i <= 0)
	case NE:
		return boolCell((xNaN && yNaN) || i != 0)
	case EQ:
		return boolCell(i == 0)
	case GE:
		return boolCell(i >= 0)
	}
	return boolCell(i > 0) // GT
}

// f2i32 converts like a C cast of a double to int, saturating where the machine's conversion would.
func f2i32(f float64) int {
	switch {
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	case f != f:
		return math.MinInt32
	}
	return int(int32(f))
}

// indirect is $expr.
func (in *Interp) indirect(a *Node) *Cell {
	x := in.execute(a.args[0])
	val := in.getfval(x)
	if math.MaxInt32 < val {
		in.fatal("trying to access out of range field %s", x.nval)
	}
	x = in.fieldadr(f2i32(val))
	x.ctype = oCELL
	x.csub = cFLD
	return x
}

// substr counts characters; its position and length are truncated toward zero and clamped to the string.
func (in *Interp) substr(a *Node) *Cell {
	x := in.execute(a.args[0])
	y := in.execute(a.args[1])
	var z *Cell
	if a.args[2] != nil {
		z = in.execute(a.args[2])
	}
	s := in.getsval(x)
	k := u8.Strlen(s) + 1
	if k <= 1 {
		r := gettemp()
		in.setsval(r, "")
		return r
	}
	m := f2i32(in.getfval(y))
	if m <= 0 {
		m = 1
	} else if m > k {
		m = k
	}
	n := k - 1
	if z != nil {
		n = f2i32(in.getfval(z))
	}
	if n < 0 {
		n = 0
	} else if n > k-m {
		n = k - m
	}
	mb := u8.Char2Byte(s, m-1)
	nb := u8.Char2Byte(s, m-1+n)
	r := gettemp()
	in.setsval(r, s[mb:nb])
	return r
}

// sindex is index(s1, s2), in characters.
func (in *Interp) sindex(a *Node) *Cell {
	s1 := in.getsval(in.execute(a.args[0]))
	s2 := in.getsval(in.execute(a.args[1]))
	z := gettemp()
	v := 0.0
	for p1 := 0; p1 < len(s1); p1++ {
		if strings.HasPrefix(s1[p1:], s2) {
			for i := 0; i < p1+1; i += u8.NextLen(s1[i:]) {
				v++
			}
			break
		}
	}
	in.setfval(z, v)
	return z
}

func (in *Interp) awksprintf(a *Node) *Cell {
	y := a.args[0].next
	x := in.execute(a.args[0])
	buf := in.format(in.getsval(x), y)
	x = gettemp()
	x.setSval(cString(string(buf)))
	x.tval = tSTR
	return x
}

func (in *Interp) awkprintf(a *Node) *Cell {
	y := a.args[0].next
	x := in.execute(a.args[0])
	buf := in.format(in.getsval(x), y)
	if a.iv == 0 {
		out := in.stdoutStream.out
		out.write(buf)
		if out.err {
			in.fatal("write error on stdout")
		}
	} else {
		fp := in.redirect(a.iv, a.args[2])
		fp.out.write(buf)
		fp.out.flush()
		if fp.out.err {
			in.fatal("write error on %s", in.filename(fp))
		}
	}
	return trueCell
}

// arith is the arithmetic operators.
func (in *Interp) arith(a *Node) *Cell {
	n := a.nobj
	i := in.getfval(in.execute(a.args[0]))
	var j float64
	if n != UMINUS && n != UPLUS {
		j = in.getfval(in.execute(a.args[1]))
	}
	switch n {
	case ADD:
		i += j
	case MINUS:
		i -= j
	case MULT:
		i *= j
	case DIVIDE:
		if j == 0 {
			in.fatal("division by zero")
		}
		i /= j
	case MOD:
		if j == 0 {
			in.fatal("division by zero in mod")
		}
		v, _ := math.Modf(i / j)
		i = i - j*v
	case UMINUS:
		i = -i
	case UPLUS: // done by getfval
	case POWER:
		i = in.power(i, j)
	}
	z := gettemp()
	in.setfval(z, i)
	return z
}

// power is x ** y: repeated squaring for non-negative integral exponents, the C library's pow otherwise.
func (in *Interp) power(x, y float64) float64 {
	if _, frac := math.Modf(y); y >= 0 && frac == 0 {
		return ipow(x, f2i32(y))
	}
	r, edom, erange := libmPow(x, y)
	return in.errcheck(r, edom, erange, "pow")
}

func ipow(x float64, n int) float64 {
	if n <= 0 {
		return 1
	}
	v := ipow(x, n/2)
	if n%2 == 0 {
		return v * v
	}
	return x * v * v
}

func (in *Interp) incrdecr(a *Node) *Cell {
	n := a.nobj
	x := in.execute(a.args[0])
	xf := in.getfval(x)
	k := 1.0
	if n == PREDECR || n == POSTDECR {
		k = -1
	}
	if n == PREINCR || n == PREDECR {
		in.setfval(x, xf+k)
		return x
	}
	z := gettemp()
	in.setfval(z, xf)
	in.setfval(x, xf+k)
	return z
}

// assign is = and the arithmetic assignments; the right side is evaluated first.
func (in *Interp) assign(a *Node) *Cell {
	n := a.nobj
	y := in.execute(a.args[1])
	x := in.execute(a.args[0])
	if n == ASSIGN {
		switch {
		case x == y && x.tval&(tFLD|tREC) == 0 && x != in.nfloc:
			// self-assignment: leave it alone unless it is a field or NF
		case y.tval&(tSTR|tNUM) == tSTR|tNUM:
			yf := in.getfval(y)
			in.setsval(x, in.getsval(y))
			x.fval = yf
			x.tval |= tNUM
		case y.isStr():
			in.setsval(x, in.getsval(y))
		case y.isNum():
			in.setfval(x, in.getfval(y))
		default:
			in.funnyvar(y, "read value of")
		}
		return x
	}
	xf := in.getfval(x)
	yf := in.getfval(y)
	switch n {
	case ADDEQ:
		xf += yf
	case SUBEQ:
		xf -= yf
	case MULTEQ:
		xf *= yf
	case DIVEQ:
		if x.tval&tCON != 0 {
			in.fatal("non-constant required for left side of /=")
		}
		if yf == 0 {
			in.fatal("division by zero in /=")
		}
		xf /= yf
	case MODEQ:
		if yf == 0 {
			in.fatal("division by zero in %%=")
		}
		v, _ := math.Modf(xf / yf)
		xf = xf - yf*v
	case POWEQ:
		xf = in.power(xf, yf)
	}
	in.setfval(x, xf)
	return x
}

// cat concatenates; the left operand is converted before the right one is evaluated.
func (in *Interp) cat(a *Node) *Cell {
	s1 := in.getsval(in.execute(a.args[0]))
	s2 := in.getsval(in.execute(a.args[1]))
	z := gettemp()
	z.setSval(s1 + s2)
	z.tval = tSTR
	return z
}

func (in *Interp) pastat(a *Node) *Cell {
	if a.args[0] == nil {
		return in.execute(a.args[1])
	}
	x := in.execute(a.args[0])
	if x.isTrue() {
		x = in.execute(a.args[1])
	}
	return x
}

// dopa2 is pat1, pat2 { action }: the range starts at a record matching pat1 and ends at one matching pat2.
func (in *Interp) dopa2(a *Node) *Cell {
	pair := a.iv
	if in.pairstack[pair] == 0 {
		if in.execute(a.args[0]).isTrue() {
			in.pairstack[pair] = 1
		}
	}
	if in.pairstack[pair] == 1 {
		if in.execute(a.args[1]).isTrue() {
			in.pairstack[pair] = 0
		}
		return in.execute(a.args[2])
	}
	return falseCell
}

// split is split(s, a [, sep]). sep is FS by default; a separator longer than one byte (or a /re/) is a
// regular expression, " " splits at runs of blanks, "" into characters, and any other byte at that byte.
func (in *Interp) split(a *Node) *Cell {
	s := in.getsval(in.execute(a.args[0]))
	arg3type := a.iv
	var fs string
	switch {
	case a.args[2] == nil && a.re == nil:
		fs = in.getsval(in.fsloc)
	case arg3type == STRING:
		fs = in.getsval(in.execute(a.args[2]))
	case arg3type == REGEXPR:
		fs = "(regexpr)"
	default:
		in.fatal("illegal type of split")
	}
	ap := in.execute(a.args[1])
	freesymtab(ap)
	ap.tval &^= tSTR
	ap.tval |= tARR
	ap.arr = makeSymtab(nSymTab)
	tab := ap.arr

	elem := func(n int, v string) {
		if r, ok := isNumber(v); ok {
			tab.set(strconv.Itoa(n), v, r, tSTR|tNUM)
		} else {
			tab.set(strconv.Itoa(n), v, 0, tSTR)
		}
	}
	if arg3type == REGEXPR && a.re.Source() == "" {
		// split(s, a, //) splits like an empty separator
		arg3type = 0
		fs = ""
	}
	n := 0
	switch {
	case s != "" && (len(fs) > 1 || arg3type == REGEXPR): // a regular expression
		pfa := a.re
		if arg3type != REGEXPR {
			pfa = in.makedfa(fs, true)
		}
		bol := true
		for {
			start, length, ok := pfa.NEMatch(s, bol)
			if !ok {
				break
			}
			bol = false
			n++
			elem(n, s[:start])
			s = s[start+length:]
			if s == "" { // a separator at the end leaves an empty last element
				n++
				tab.set(strconv.Itoa(n), "", 0, tSTR)
				return numTemp(n)
			}
		}
		n++
		elem(n, s)
	case a.args[2] == nil && a.re == nil && in.csv: // CSV only when no separator is given
		for {
			n++
			var f []byte
			k := 0
			if s != "" && s[0] == '"' {
				for k = 1; k < len(s); {
					if s[k] == '"' && k+1 < len(s) && s[k+1] == '"' {
						k += 2 // doubled quote
						f = append(f, '"')
					} else if s[k] == '"' && (k+1 == len(s) || s[k+1] == ',') {
						k++ // skip over the closing quote
						break
					} else {
						f = append(f, s[k])
						k++
					}
				}
			} else {
				for k < len(s) && s[k] != ',' {
					k++
				}
				f = []byte(s[:k])
			}
			elem(n, string(f))
			if k >= len(s) {
				break
			}
			s = s[k+1:]
		}
	case !in.csv && fs != "" && fs[0] == ' ': // the usual case: runs of blanks
		isWS := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }
		for {
			for s != "" && isWS(s[0]) {
				s = s[1:]
			}
			if s == "" {
				break
			}
			j := 1
			for j < len(s) && !isWS(s[j]) {
				j++
			}
			n++
			elem(n, s[:j])
			s = s[j:]
			if s != "" {
				s = s[1:]
			}
		}
	case fs == "": // one element per character
		for s != "" {
			l := u8.NextLen(s)
			c := s[:l]
			n++
			if isDigit(c[0]) {
				tab.set(strconv.Itoa(n), c, atof(c), tSTR|tNUM)
			} else {
				tab.set(strconv.Itoa(n), c, 0, tSTR)
			}
			s = s[l:]
		}
	case s != "": // some single byte
		sep := fs[0]
		for {
			n++
			j := strings.IndexByte(s, sep)
			if j < 0 {
				elem(n, s)
				break
			}
			elem(n, s[:j])
			s = s[j+1:]
		}
	}
	return numTemp(n)
}

// atof is C's atof of a string that starts with a digit.
func atof(s string) float64 {
	v, _, _ := libc.Strtod(s)
	return v
}

func numTemp(n int) *Cell {
	x := gettemp()
	x.tval = tNUM
	x.fval = float64(n)
	return x
}

func (in *Interp) condexpr(a *Node) *Cell {
	if in.execute(a.args[0]).isTrue() {
		return in.execute(a.args[1])
	}
	return in.execute(a.args[2])
}

func (in *Interp) ifstat(a *Node) *Cell {
	x := in.execute(a.args[0])
	if x.isTrue() {
		x = in.execute(a.args[1])
	} else if a.args[2] != nil {
		x = in.execute(a.args[2])
	}
	return x
}

func (in *Interp) whilestat(a *Node) *Cell {
	for {
		x := in.execute(a.args[0])
		if !x.isTrue() {
			return x
		}
		x = in.execute(a.args[1])
		if x.isBreak() {
			return trueCell
		}
		if x.isNext() || x.isExit() || x.isRet() {
			return x
		}
	}
}

func (in *Interp) dostat(a *Node) *Cell {
	for {
		x := in.execute(a.args[0])
		if x.isBreak() {
			return trueCell
		}
		if x.isNext() || x.isExit() || x.isRet() {
			return x
		}
		x = in.execute(a.args[1])
		if !x.isTrue() {
			return x
		}
	}
}

func (in *Interp) forstat(a *Node) *Cell {
	in.execute(a.args[0])
	for {
		if a.args[1] != nil {
			x := in.execute(a.args[1])
			if !x.isTrue() {
				return x
			}
		}
		x := in.execute(a.args[3])
		if x.isBreak() {
			return trueCell
		}
		if x.isNext() || x.isExit() || x.isRet() {
			return x
		}
		in.execute(a.args[2])
	}
}

// instat is for (k in a): it walks the hash table in bucket and chain order, reading the table afresh at each
// step as the original does, so elements added by the body may or may not be visited.
func (in *Interp) instat(a *Node) *Cell {
	vp := in.execute(a.args[0])
	arrayp := in.execute(a.args[1])
	if !arrayp.isArr() {
		return trueCell
	}
	tp := arrayp.arr
	for i := 0; i < len(tp.tab); i++ {
		var ncp *Cell
		for cp := tp.tab[i]; cp != nil; cp = ncp {
			in.setsval(vp, cp.nval)
			ncp = cp.cnext
			x := in.execute(a.args[2])
			if x.isBreak() {
				return trueCell
			}
			if x.isNext() || x.isExit() || x.isRet() {
				return x
			}
		}
	}
	return trueCell
}

// convertCase is toupper and tolower: by bytes in a single-byte locale, otherwise by wide characters, where
// a byte sequence that is not a character is fatal.
func (in *Interp) convertCase(s string, upper bool) string {
	if !u8.MultiByte {
		b := []byte(s)
		for i, c := range b {
			if upper && isLower(c) {
				b[i] = c - 'a' + 'A'
			} else if !upper && isUpper(c) {
				b[i] = c - 'A' + 'a'
			}
		}
		return string(b)
	}
	var b strings.Builder
	rest := s
	for {
		r, n := libc.DecodeMB(rest)
		if n == 0 {
			break
		}
		if n < 0 {
			in.fatal("illegal byte sequence %s", s)
		}
		rest = rest[n:]
		if upper {
			r = libc.ToUpper(r)
		} else {
			r = libc.ToLower(r)
		}
		enc, ok := libc.EncodeMB(r)
		if !ok {
			in.fatal("illegal wide character %s", s)
		}
		b.WriteString(enc)
	}
	return b.String()
}

// bltin runs the builtin functions.
func (in *Interp) bltin(a *Node) *Cell {
	t := a.iv
	x := in.execute(a.args[1])
	nextarg := a.args[1].next
	var u float64
	switch t {
	case fLENGTH:
		if x.isArr() {
			u = float64(x.arr.nelem)
		} else {
			u = float64(u8.Strlen(in.getsval(x)))
		}
	case fLOG:
		r, edom, erange := libmLog(in.getfval(x))
		u = in.errcheck(r, edom, erange, "log")
	case fINT:
		u, _ = math.Modf(in.getfval(x))
	case fEXP:
		r, erange := libmExp(in.getfval(x))
		u = in.errcheck(r, false, erange, "exp")
	case fSQRT:
		v := in.getfval(x)
		u = in.errcheck(math.Sqrt(v), v < 0, false, "sqrt")
	case fSIN:
		u = math.Sin(in.getfval(x))
	case fCOS:
		u = math.Cos(in.getfval(x))
	case fATAN:
		if nextarg == nil {
			in.warning("atan2 requires two arguments; returning 1.0")
			u = 1
		} else {
			y := in.execute(a.args[1].next)
			u = math.Atan2(in.getfval(x), in.getfval(y))
			nextarg = nextarg.next
		}
	case fSYSTEM:
		u = in.system(in.getsval(x))
	case fRAND:
		// random() returns numbers in [0, 2^31), so this is in [0, 1)
		u = float64(in.random.Next()) / 0x80000000
	case fSRAND:
		if x.isRec() { // no argument
			u = float64(timeNow())
		} else {
			u = in.getfval(x)
		}
		tmp := u
		in.random.Seed(uint32(f2u64(u)))
		u = in.srandSeed
		in.srandSeed = tmp
	case fTOUPPER, fTOLOWER:
		r := gettemp()
		in.setsval(r, in.convertCase(in.getsval(x), t == fTOUPPER))
		return r
	case fFLUSH:
		if x.isRec() || in.getsval(x) == "" {
			in.flushAll() // fflush() or fflush(""): everything
		} else if fp, _ := in.openfile(fFLUSHMode, in.getsval(x)); fp == nil {
			u = -1
		} else if fp.out != nil {
			fp.out.flush()
			if fp.out.err {
				u = -1
			}
		}
	default:
		in.fatal("illegal function type %d", t)
	}
	x = gettemp()
	in.setfval(x, u)
	if nextarg != nil {
		in.warning("warning: function has too many arguments")
		for ; nextarg != nil; nextarg = nextarg.next {
			in.execute(nextarg)
		}
	}
	return x
}

// f2u64 converts like a C cast of a double to an unsigned long.
func f2u64(f float64) uint64 {
	if f < 0 {
		return uint64(int64(f))
	}
	return uint64(f)
}

// printstat is print: the destination is evaluated before the values.
func (in *Interp) printstat(a *Node) *Cell {
	fp := in.stdoutStream
	if a.iv != 0 {
		fp = in.redirect(a.iv, a.args[2])
	}
	for x := a.args[0]; x != nil; x = x.next {
		y := in.execute(x)
		fp.out.writeString(in.getpssval(y))
		if x.next == nil {
			fp.out.writeString(in.getsval(in.orsloc))
		} else {
			fp.out.writeString(in.getsval(in.ofsloc))
		}
	}
	if a.iv != 0 {
		fp.out.flush()
	}
	if fp.out.err {
		in.fatal("write error on %s", in.filename(fp))
	}
	return trueCell
}

// redirect opens the destination of > >> and |.
func (in *Interp) redirect(a int, b *Node) *stream {
	fname := in.getsval(in.execute(b))
	fp, _ := in.openfile(a, fname)
	if fp == nil || fp.out == nil {
		in.fatal("can't open file %s", fname)
	}
	return fp
}

func (in *Interp) closeop(a *Node) *Cell {
	x := in.execute(a.args[0])
	in.getsval(x)
	r := gettemp()
	in.setfval(r, in.closefile(x.sval))
	return r
}

// Match types of sub and gsub.
const (
	mtIgnore  = iota // not selected, or an empty match right after a replacement
	mtInsert         // selected and empty
	mtReplace        // selected and not empty
)

// dosub is sub and gsub: sub replaces the first match, gsub every one; an empty match right after a
// replacement does not count. It returns the number of matches.
func (in *Interp) dosub(a *Node) *Cell {
	pfa := a.re
	if a.dyn {
		pfa = in.makedfa(in.getsval(in.execute(a.args[1])), true)
	}
	repl := in.getsval(in.execute(a.args[2]))
	whichm := 0 // gsub: every match
	if a.nobj == SUB {
		whichm = 1
	}
	x := in.execute(a.args[3]) // the target
	src := in.getsval(x)
	var buf []byte
	matched := false
	start := 0
	noempty := -1 // an empty match is not allowed here
	m := 0
	bol := true
	for {
		mstart, patlen := pfa.PMatch(src[start:], bol)
		if patlen < 0 {
			break
		}
		matched = true
		bol = false
		patbeg := start + mstart
		var mtype int
		if patbeg == noempty && patlen == 0 {
			mtype = mtIgnore
		} else if m++; whichm == m || whichm == 0 {
			if patlen > 0 {
				mtype = mtReplace
			} else {
				mtype = mtInsert
			}
		} else {
			mtype = mtIgnore
		}
		buf = append(buf, src[start:patbeg]...) // leading text
		if mtype != mtIgnore {
			for r := 0; r < len(repl); {
				switch repl[r] {
				case '\\':
					buf, r = in.backsub(buf, repl, r)
				case '&':
					r++
					buf = append(buf, src[patbeg:patbeg+patlen]...)
				default:
					buf = append(buf, repl[r])
					r++
				}
			}
		}
		atEnd := patbeg >= len(src)
		if mtype != mtReplace && !atEnd { // keep the matched text
			if patlen == 0 {
				patlen = u8.NextLen(src[patbeg:])
			}
			buf = append(buf, src[patbeg:patbeg+patlen]...)
		}
		start = patbeg + patlen
		if m == whichm || atEnd {
			break
		}
		if mtype == mtReplace {
			noempty = start
		}
	}
	if matched {
		if start < len(src) {
			buf = append(buf, src[start:]...) // trailing text
		}
		in.setsval(x, cString(string(buf)))
	}
	return numTemp(m)
}

// backsub copies the escape at repl[r] (a backslash) of a replacement: \& is a literal &, \\& a backslash
// and the matched text, \\\& a backslash and &, and other backslashes are kept as they are (POSIXLY_CORRECT
// in the environment makes \\x a single backslash).
func (in *Interp) backsub(buf []byte, repl string, r int) ([]byte, int) {
	if !in.posixChecked {
		in.posixChecked = true
		_, in.posixSub = lookupEnv("POSIXLY_CORRECT")
	}
	at := func(i int) byte {
		if r+i < len(repl) {
			return repl[r+i]
		}
		return 0
	}
	switch {
	case at(1) == '\\':
		switch {
		case at(2) == '\\' && at(3) == '&': // \\\& -> \&
			buf = append(buf, '\\', '&')
			r += 4
		case at(2) == '&': // \\& -> \ and the matched text
			buf = append(buf, '\\')
			r += 2
		case in.posixSub: // \\x -> \x
			r++
			buf = append(buf, repl[r])
			r++
		default: // \\x -> \\x
			buf = append(buf, '\\', '\\')
			r += 2
		}
	case at(1) == '&': // literal &
		buf = append(buf, '&')
		r += 2
	default: // literal backslash
		buf = append(buf, '\\')
		r++
	}
	return buf, r
}
