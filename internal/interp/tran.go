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
)

const (
	nSymTab = 50 // initial size of a symbol table
	fullTab = 2  // rehash when a table gets this many times full
	growTab = 4  // grow a table by this factor
)

func makeSymtab(n int) *Array {
	return &Array{tab: make([]*Cell, n)}
}

// symHash is the original's string hash. The bytes of a C char are signed on some machines and unsigned on
// others, so bytes above 0x7F hash differently from one architecture to the next, exactly as there.
func symHash(s string, n int) int {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = uint32(charValue(s[i])) + 31*h
	}
	return int(h % uint32(n))
}

func (tp *Array) rehash() {
	nsz := growTab * len(tp.tab)
	np := make([]*Cell, nsz)
	for _, cp := range tp.tab {
		for cp != nil {
			op := cp.cnext
			nh := symHash(cp.nval, nsz)
			cp.cnext = np[nh]
			np[nh] = cp
			cp = op
		}
	}
	tp.tab = np
}

// lookup finds the element named s, or nil.
func (tp *Array) lookup(s string) *Cell {
	for p := tp.tab[symHash(s, len(tp.tab))]; p != nil; p = p.cnext {
		if p.nval == s {
			return p
		}
	}
	return nil
}

// set returns the element named n, creating it with the string s, number f and flags t if it does not exist.
func (tp *Array) set(n, s string, f float64, t int) *Cell {
	if p := tp.lookup(n); p != nil {
		return p
	}
	p := &Cell{nval: n, sval: s, fval: f, tval: t, csub: cUNK, ctype: oCELL, sid: nextSID()}
	tp.nelem++
	if tp.nelem > fullTab*len(tp.tab) {
		tp.rehash()
	}
	h := symHash(n, len(tp.tab))
	p.cnext = tp.tab[h]
	tp.tab[h] = p
	return p
}

// remove deletes the element named s, if present.
func (tp *Array) remove(s string) {
	h := symHash(s, len(tp.tab))
	var prev *Cell
	for p := tp.tab[h]; p != nil; prev, p = p, p.cnext {
		if p.nval == s {
			if prev == nil {
				tp.tab[h] = p.cnext
			} else {
				prev.cnext = p.cnext
			}
			tp.nelem--
			return
		}
	}
}

// sidCounter numbers string values: a conversion done with CONVFMT or OFMT remembers which string of the format
// variable it used, and is redone when the format variable has since been given a new string, as the original
// compares the format's string pointer.
var sidCounter uint64

func nextSID() uint64 {
	sidCounter++
	return sidCounter
}

// setSval replaces the stored string of a cell without any of the assignment side effects.
func (c *Cell) setSval(s string) {
	c.sval = s
	c.sid = nextSID()
}

// syminit creates the builtin variables.
func (in *Interp) syminit() {
	st := in.symtab
	in.literal0 = st.set("0", "0", 0, tNUM|tSTR|tCON|tDONTFREE)
	// $zero&null is used for if (x) ... tests.
	in.nullloc = st.set("$zero&null", "", 0, tNUM|tSTR|tCON|tDONTFREE)
	in.nullnode = newValueNode(in.nullloc, cCON, 1)

	in.fsloc = st.set("FS", " ", 0, tSTR|tDONTFREE)
	in.rsloc = st.set("RS", "\n", 0, tSTR|tDONTFREE)
	in.ofsloc = st.set("OFS", " ", 0, tSTR|tDONTFREE)
	in.orsloc = st.set("ORS", "\n", 0, tSTR|tDONTFREE)
	in.ofmtloc = st.set("OFMT", "%.6g", 0, tSTR|tDONTFREE)
	in.convfmtloc = st.set("CONVFMT", "%.6g", 0, tSTR|tDONTFREE)
	in.filenameloc = st.set("FILENAME", "", 0, tSTR|tDONTFREE)
	in.nfloc = st.set("NF", "", 0, tNUM)
	in.nrloc = st.set("NR", "", 0, tNUM)
	in.fnrloc = st.set("FNR", "", 0, tNUM)
	in.subseploc = st.set("SUBSEP", "\034", 0, tSTR|tDONTFREE)
	in.rstartloc = st.set("RSTART", "", 0, tNUM)
	in.rlengthloc = st.set("RLENGTH", "", 0, tNUM)
	in.symtabloc = st.set("SYMTAB", "", 0, tARR)
	in.symtabloc.arr = st
}

// arginit sets up ARGV and ARGC.
func (in *Interp) arginit(av []string) {
	in.argcloc = in.symtab.set("ARGC", "", float64(len(av)), tNUM)
	cp := in.symtab.set("ARGV", "", 0, tARR)
	ap := makeSymtab(nSymTab)
	cp.arr = ap
	for i, a := range av {
		if r, ok := isNumber(a); ok {
			ap.set(strconv.Itoa(i), a, r, tSTR|tNUM)
		} else {
			ap.set(strconv.Itoa(i), a, 0, tSTR)
		}
	}
	in.argvcell = cp
}

// envinit sets up ENVIRON from the process environment.
func (in *Interp) envinit(env []string) {
	cp := in.symtab.set("ENVIRON", "", 0, tARR)
	ap := makeSymtab(nSymTab)
	cp.arr = ap
	for _, e := range env {
		i := strings.IndexByte(e, '=')
		if i <= 0 { // no '=', or no name before it
			continue
		}
		name, val := e[:i], e[i+1:]
		if r, ok := isNumber(val); ok {
			ap.set(name, val, r, tSTR|tNUM)
		} else {
			ap.set(name, val, 0, tSTR)
		}
	}
}

// freesymtab empties the array held by ap.
func freesymtab(ap *Cell) {
	if !ap.isArr() || ap.arr == nil {
		return
	}
	ap.arr = nil
}

func (in *Interp) funnyvar(vp *Cell, rw string) {
	if vp.isArr() {
		in.fatal("can't %s %s; it's an array name.", rw, vp.nval)
	}
	if vp.tval&tFCN != 0 {
		in.fatal("can't %s %s; it's a function.", rw, vp.nval)
	}
	in.warning("funny variable %p: n=%s s=\"%s\" f=%g t=%o", vp, vp.nval, vp.sval, vp.fval, vp.tval)
}

// setfval assigns a number to a cell, with the side effects of assigning to a field, NF, $0 or OFS.
func (in *Interp) setfval(vp *Cell, f float64) float64 {
	f += 0.0 // normalise negative zero to positive zero
	if vp.tval&(tNUM|tSTR) == 0 {
		in.funnyvar(vp, "assign to")
	}
	switch {
	case vp.isFld():
		in.donerec = false // $0 is now invalid
		fldno := atoi(vp.nval)
		if float64(fldno) > in.nfloc.fval {
			in.newfld(fldno)
		}
	case vp == in.nfloc:
		in.donerec = false
		in.setlastfld(int(f))
	case vp.isRec():
		in.donefld = false // $1... are now invalid
		in.donerec = true
		in.recIsBuf = false
		in.savefs()
	case vp == in.ofsloc:
		if !in.donerec {
			in.recbld()
		}
	}
	vp.tval &^= tSTR | tCONVC | tCONVO
	vp.fmtID = 0
	vp.tval |= tNUM
	if f == 0 {
		f = 0
	}
	vp.fval = f
	return f
}

// setsval assigns a string to a cell, with the same side effects as setfval.
func (in *Interp) setsval(vp *Cell, s string) string {
	if vp.tval&(tNUM|tSTR) == 0 {
		in.funnyvar(vp, "assign to")
	}
	if in.csv && vp == in.rsloc {
		in.warning("danger: don't set RS when --csv is in effect")
	}
	if in.csv && vp == in.fsloc {
		in.warning("danger: don't set FS when --csv is in effect")
	}
	switch {
	case vp.isFld():
		in.donerec = false
		fldno := atoi(vp.nval)
		if float64(fldno) > in.nfloc.fval {
			in.newfld(fldno)
		}
	case vp.isRec():
		in.donefld = false
		in.donerec = true
		in.recIsBuf = false
		in.savefs()
	case vp == in.ofsloc:
		if !in.donerec {
			in.recbld()
		}
	}
	vp.tval &^= tNUM | tDONTFREE | tCONVC | tCONVO
	vp.tval |= tSTR
	vp.fmtID = 0
	vp.setSval(s)
	if vp == in.nfloc {
		in.donerec = false
		f := in.getfval(vp)
		in.setlastfld(int(f))
	}
	return vp.sval
}

// getfval returns the number value of a cell, converting its string if the number is not current.
func (in *Interp) getfval(vp *Cell) float64 {
	if vp.tval&(tNUM|tSTR) == 0 {
		in.funnyvar(vp, "read value of")
	}
	if vp.isFld() && !in.donefld {
		in.fldbld()
	} else if vp.isRec() && !in.donerec {
		in.recbld()
	}
	if !vp.isNum() {
		if f, noTrailing, ok := isValidNumber(vp.sval, true); ok {
			vp.fval = f
			if noTrailing && vp.tval&tCON == 0 {
				vp.tval |= tNUM // make NUM only sparingly
			}
		} else {
			vp.fval = 0
		}
	}
	return vp.fval
}

// infNaN spells infinities and NaNs the way the original prints them.
func infNaN(d float64) (string, bool) {
	switch {
	case math.IsInf(d, 0):
		if d < 0 {
			return "-inf", true
		}
		return "+inf", true
	case math.IsNaN(d):
		if math.Signbit(d) {
			return "-nan", true
		}
		return "+nan", true
	}
	return "", false
}

// numToStr converts a number for output or conversion: integral values exactly ("%.30g"), others with the
// format, whose expansion the original truncates to 255 bytes.
func numToStr(f float64, format string) string {
	if s, ok := infNaN(f); ok {
		return s
	}
	var s string
	if _, frac := math.Modf(f); frac == 0 {
		s = libc.SprintfFloat("%.30g", f)
	} else {
		s = sprintfNumber(format, f)
	}
	if len(s) > 255 {
		s = s[:255]
	}
	return s
}

// getStrVal returns the string value of a cell, converting its number with the format in fmtCell (CONVFMT or
// OFMT) when the string is not current or was made with the other format or an older string of this one.
func (in *Interp) getStrVal(vp *Cell, fmtCell *Cell) string {
	if vp.tval&(tNUM|tSTR) == 0 {
		in.funnyvar(vp, "read value of")
	}
	if vp.isFld() && !in.donefld {
		in.fldbld()
	} else if vp.isRec() && !in.donerec {
		in.recbld()
	}
	ofmt := fmtCell == in.ofmtloc
	update := func() {
		vp.setSval(numToStr(vp.fval, fmtCell.sval))
		vp.tval &^= tDONTFREE
		vp.tval |= tSTR
		if ofmt {
			vp.tval &^= tCONVC
			vp.tval |= tCONVO
		} else {
			vp.tval &^= tCONVO
			vp.tval |= tCONVC
		}
		vp.fmtID = fmtCell.sid
	}
	switch {
	case !vp.isStr():
		update()
	case vp.tval&tDONTFREE != 0 || !vp.isNum() || vp.isFld():
		// strings of constants, fields and pure strings are never reconverted
	case ofmt:
		if vp.tval&tCONVC != 0 || (vp.tval&tCONVO != 0 && vp.fmtID != fmtCell.sid) {
			update()
		}
	default:
		if vp.tval&tCONVO != 0 || (vp.tval&tCONVC != 0 && vp.fmtID != fmtCell.sid) {
			update()
		}
	}
	return vp.sval
}

// getsval is the string value of a cell for computation (CONVFMT).
func (in *Interp) getsval(vp *Cell) string { return in.getStrVal(vp, in.convfmtloc) }

// getpssval is the string value of a cell for print (OFMT).
func (in *Interp) getpssval(vp *Cell) string { return in.getStrVal(vp, in.ofmtloc) }

// catstr joins two adjacent string constants of the program into one constant.
func (in *Interp) catstr(a, b *Cell) *Cell {
	p := in.getsval(a) + in.getsval(b)
	// A string constant "xx" is entered in the symbol table under the name "xx ".
	return in.symtab.set(p+" ", p, 0, tCON|tSTR|tDONTFREE)
}

// qstring collects the escapes of a -v or command-line assignment value, or of -F's argument, up to delim.
func (in *Interp) qstring(is string, delim byte) string {
	var b []byte
	s := is
	for i := 0; i < len(s) && s[i] != delim; i++ {
		c := s[i]
		if c == '\n' {
			in.syntax("newline in string %.20s...", is)
			continue
		}
		if c != '\\' {
			b = append(b, c)
			continue
		}
		i++
		if i >= len(s) { // \ at the end
			b = append(b, '\\')
			break
		}
		switch c = s[i]; c {
		case '\\':
			b = append(b, '\\')
		case 'n':
			b = append(b, '\n')
		case 't':
			b = append(b, '\t')
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		case 'r':
			b = append(b, '\r')
		case 'v':
			b = append(b, '\v')
		case 'a':
			b = append(b, '\a')
		default:
			if !isDigit(c) {
				b = append(b, c)
				break
			}
			// up to three decimal digits, read as octal as in the original
			n := int(c - '0')
			if i+1 < len(s) && isDigit(s[i+1]) {
				i++
				n = 8*n + int(s[i]-'0')
				if i+1 < len(s) && isDigit(s[i+1]) {
					i++
					n = 8*n + int(s[i]-'0')
				}
			}
			b = append(b, byte(n))
		}
	}
	return cString(string(b))
}
