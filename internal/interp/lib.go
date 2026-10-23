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
	"fmt"
	"strconv"
	"strings"

	"github.com/onetrueawk/awk/internal/libc"
	"github.com/onetrueawk/awk/internal/u8"
)

const maxFld = 2 // fields allocated up front; more are made on demand

// recinit creates $0 and the first fields.
func (in *Interp) recinit() {
	in.fldtab = []*Cell{{ctype: oCELL, csub: cFLD, nval: "0", tval: tREC | tSTR | tDONTFREE}}
	in.nfields = 0
	in.makefields(1, maxFld)
	in.nfields = maxFld
}

// makefields creates the cells $n1..$n2.
func (in *Interp) makefields(n1, n2 int) {
	for i := n1; i <= n2; i++ {
		in.fldtab = append(in.fldtab, &Cell{ctype: oCELL, csub: cFLD, nval: strconv.Itoa(i), tval: tFLD | tSTR | tDONTFREE})
	}
}

// initgetrec runs the command-line assignments before the first file name and finds the first input.
func (in *Interp) initgetrec() {
	for i := 1; float64(i) < in.argcloc.fval; i++ {
		p, ok := in.getargv(i) // find the first real file name
		if !ok || p == "" {    // deleted or zapped
			in.argno++
			continue
		}
		if !isclvar(p) {
			in.setsval(in.symtab.lookup("FILENAME"), p)
			return
		}
		in.setclvar(p) // a command-line assignment before the file name
		in.argno++
	}
	in.infile = in.stdin // no file names, so read standard input
	in.innew = true
}

// savefs remembers FS as it is when a record is read or $0 assigned: the record is split with that value.
func (in *Interp) savefs() {
	s := in.getsval(in.fsloc)
	in.inputFS = &s
}

// getrec reads the next input record: into $0 when isrecord, otherwise only returned. It moves on through
// the ARGV files and assignments, and reports false at the true end of input.
//
// $0 lives in a record buffer, as in the original. At the end of input the buffer keeps the last record, except
// that reading with a regular expression RS clears it first: only its first byte is put back, so $0 in END is
// then just that byte.
func (in *Interp) getrec(isrecord bool) (string, bool) {
	if in.firsttime {
		in.firsttime = false
		in.initgetrec()
	}
	var save0 byte
	if isrecord && in.recBuf != "" {
		save0 = in.recBuf[0]
	}
	cleared := false
	for float64(in.argno) < in.argcloc.fval || in.infile == in.stdin {
		if in.infile == nil { // open the next file
			file, ok := in.getargv(in.argno)
			if !ok || file == "" { // deleted or zapped
				in.argno++
				continue
			}
			if isclvar(file) { // a var=value argument
				in.setclvar(file)
				in.argno++
				continue
			}
			in.filenameloc.setSval(file)
			if file == "-" {
				in.infile = in.stdin
			} else if f, err := openInFile(file); err == nil {
				in.infile = f
			} else {
				in.fatal("can't open file %s", file)
			}
			in.innew = true
			in.setfval(in.fnrloc, 0)
		}
		buf, isrec := in.readrec(in.infile, in.innew)
		in.innew = false
		if isrecord && !in.csv && len(in.rsloc.sval) > 1 {
			cleared = true
		}
		if isrec || buf != "" { // a normal record
			if isrecord {
				rec := in.fldtab[0]
				in.recBuf, in.recIsBuf = buf, true
				rec.setSval(buf)
				rec.tval = tREC | tSTR | tDONTFREE
				if r, ok := isNumber(buf); ok {
					rec.fval = r
					rec.tval |= tNUM
				}
				in.donefld = false
				in.donerec = true
				in.savefs()
			}
			in.setfval(in.nrloc, in.nrloc.fval+1)
			in.setfval(in.fnrloc, in.fnrloc.fval+1)
			return buf, true
		}
		// end of this file; go on to the next
		if in.infile != in.stdin {
			in.infile.close()
		}
		in.infile = nil
		in.argno++
	}
	if isrecord && cleared {
		in.recBuf = ""
		if save0 != 0 {
			in.recBuf = string([]byte{save0})
		}
		if in.recIsBuf {
			in.fldtab[0].setSval(in.recBuf)
		}
	}
	return "", false
}

func (in *Interp) nextfile() {
	if in.infile != nil && in.infile != in.stdin {
		in.infile.close()
	}
	in.infile = nil
	in.argno++
}

// readrec reads one record from inf with the current RS (or as CSV). The bool reports whether a record was
// read even if it is empty.
func (in *Interp) readrec(inf *inStream, newflag bool) (string, bool) {
	rs := in.getsval(in.rsloc)
	switch {
	case in.csv:
		// a record ending at the end of the input does not count as read, though it is returned
		buf, c := readcsvrec(inf)
		return buf, c != eof
	case len(rs) > 1:
		pfa := in.makedfa(rs, true)
		buf, start, _, found := pfa.FNEMatch(inf, newflag)
		if found {
			buf = buf[:start]
		}
		rec := cString(string(buf))
		return rec, !(!found && rec == "")
	}
	var sep int
	var c int
	if rs == "" {
		sep = '\n'
		for c = inf.Getc(); c == '\n'; c = inf.Getc() { // skip leading newlines
		}
		if c != eof {
			inf.Ungetc(byte(c))
		}
	} else {
		sep = int(rs[0])
	}
	var b []byte
	for {
		for c = inf.Getc(); c != sep && c != eof; c = inf.Getc() {
			b = append(b, byte(c))
		}
		if rs != "" || c == eof {
			break
		}
		if c = inf.Getc(); c == '\n' || c == eof { // two newlines in a row end the record
			break
		}
		b = append(b, '\n', byte(c))
	}
	return cString(string(b)), !(c == eof && len(b) == 0)
}

// readcsvrec reads a CSV record, which may span lines inside double quotes; a \r before a line break is
// dropped. It returns the record and the last character read.
func readcsvrec(inf *inStream) (string, int) {
	var b []byte
	inQuote := false
	c := inf.Getc()
	for ; c != eof; c = inf.Getc() {
		if c == '\n' {
			if !inQuote {
				break
			}
			if len(b) > 0 && b[len(b)-1] == '\r' { // \r\n inside quotes keeps only the \n
				b = b[:len(b)-1]
			}
		}
		b = append(b, byte(c))
		if c == '"' {
			inQuote = !inQuote
		}
	}
	if c == '\n' && len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return cString(string(b)), c
}

// getargv returns ARGV[n], and false when there is no such element.
func (in *Interp) getargv(n int) (string, bool) {
	ap := in.argvcell.arr
	key := strconv.Itoa(n)
	if ap == nil || ap.lookup(key) == nil {
		return "", false
	}
	x := ap.set(key, "", 0, tSTR)
	return in.getsval(x), true
}

// setclvar performs a command-line assignment var=value.
func (in *Interp) setclvar(s string) {
	i := strings.IndexByte(s, '=')
	name := s[:i]
	p := in.qstring(s[i+1:], 0)
	q := in.symtab.set(name, p, 0, tSTR)
	in.setsval(q, p)
	if r, ok := isNumber(q.sval); ok {
		q.fval = r
		q.tval |= tNUM
	}
}

// fldbld splits $0 into fields with the FS in effect when the record was read.
func (in *Interp) fldbld() {
	if in.donefld {
		return
	}
	if !in.fldtab[0].isStr() {
		in.getsval(in.fldtab[0])
	}
	r := in.fldtab[0].sval
	if in.inputFS == nil {
		in.savefs()
	}
	fs := *in.inputFS
	i := 0 // number of fields
	setField := func(s string, t int) {
		i++
		if i > in.nfields {
			in.growfldtab(i)
		}
		f := in.fldtab[i]
		f.setSval(s)
		f.tval = t
	}
	switch {
	case !in.csv && len(fs) > 1: // a regular expression
		i = in.refldbld(r, fs)
	case !in.csv && fs == " ": // default: runs of blanks, tabs and newlines
		for {
			for len(r) > 0 && (r[0] == ' ' || r[0] == '\t' || r[0] == '\n') {
				r = r[1:]
			}
			if r == "" {
				break
			}
			j := 1
			for j < len(r) && r[j] != ' ' && r[j] != '\t' && r[j] != '\n' {
				j++
			}
			setField(r[:j], tFLD|tSTR|tDONTFREE)
			r = r[j:]
		}
	case in.csv:
		if r != "" {
			for {
				var f []byte
				k := 0
				if r != "" && r[0] == '"' { // start of "..."
					for k = 1; k < len(r); {
						if r[k] == '"' && k+1 < len(r) && r[k+1] == '"' {
							k += 2 // doubled quote
							f = append(f, '"')
						} else if r[k] == '"' && (k+1 == len(r) || r[k+1] == ',') {
							k++ // skip over the closing quote
							break
						} else {
							f = append(f, r[k])
							k++
						}
					}
				} else { // unquoted field
					for k < len(r) && r[k] != ',' {
						k++
					}
					f = []byte(r[:k])
				}
				setField(string(f), tFLD|tSTR|tDONTFREE)
				if k >= len(r) {
					break
				}
				r = r[k+1:]
			}
		}
	case fs == "": // one character per field
		for r != "" {
			n := u8.NextLen(r)
			setField(r[:n], tFLD|tSTR)
			r = r[n:]
		}
	case r != "": // a single character; an empty record has no fields
		sep := fs[0]
		// a newline also separates fields when RS is "" (paragraph mode)
		nlSep := in.rsloc.sval == ""
		for {
			j := 0
			for j < len(r) && r[j] != sep && !(nlSep && r[j] == '\n') {
				j++
			}
			setField(r[:j], tFLD|tSTR|tDONTFREE)
			if j >= len(r) {
				break
			}
			r = r[j+1:]
		}
	}
	in.cleanfld(i+1, in.lastfld) // clean out junk from the previous record
	in.lastfld = i
	in.donefld = true
	for j := 1; j <= in.lastfld; j++ {
		p := in.fldtab[j]
		if r, ok := isNumber(p.sval); ok {
			p.fval = r
			p.tval |= tNUM
		}
	}
	in.setfval(in.nfloc, float64(in.lastfld))
	in.donerec = true // restore
}

// cleanfld empties the fields n1..n2.
func (in *Interp) cleanfld(n1, n2 int) {
	for i := n1; i <= n2; i++ {
		p := in.fldtab[i]
		p.setSval("")
		p.tval = tFLD | tSTR | tDONTFREE
	}
}

// newfld adds field n after the end of the existing fields.
func (in *Interp) newfld(n int) {
	if n > in.nfields {
		in.growfldtab(n)
	}
	in.cleanfld(in.lastfld+1, n)
	in.lastfld = n
	in.setfval(in.nfloc, float64(n))
}

// setlastfld changes the number of fields, emptying the ones that come or go.
func (in *Interp) setlastfld(n int) {
	if n < 0 {
		in.fatal("cannot set NF to a negative value")
	}
	if n > in.nfields {
		in.growfldtab(n)
	}
	if in.lastfld < n {
		in.cleanfld(in.lastfld+1, n)
	} else {
		in.cleanfld(n+1, in.lastfld)
	}
	in.lastfld = n
}

// fieldadr returns the cell of $n; fields past NF are empty but do not change NF.
func (in *Interp) fieldadr(n int) *Cell {
	if n < 0 {
		in.fatal("trying to access out of range field %d", n)
	}
	if n > in.nfields {
		in.growfldtab(n)
	}
	return in.fldtab[n]
}

// growfldtab makes fields up to at least $n.
func (in *Interp) growfldtab(n int) {
	nf := 2 * in.nfields
	if n > nf {
		nf = n
	}
	in.makefields(in.nfields+1, nf)
	in.nfields = nf
}

// refldbld splits rec at the non-empty matches of the regular expression fs.
func (in *Interp) refldbld(rec, fs string) int {
	if rec == "" {
		return 0
	}
	pfa := in.makedfa(fs, true)
	bol := true
	for i := 1; ; i++ {
		if i > in.nfields {
			in.growfldtab(i)
		}
		f := in.fldtab[i]
		f.tval = tFLD | tSTR | tDONTFREE
		start, length, ok := pfa.NEMatch(rec, bol)
		if !ok {
			f.setSval(rec)
			return i
		}
		bol = false
		f.setSval(rec[:start])
		rec = rec[start+length:]
	}
}

// recbld rebuilds $0 from $1..$NF joined with OFS.
func (in *Interp) recbld() {
	if in.donerec {
		return
	}
	sep := in.getsval(in.ofsloc)
	var b strings.Builder
	for i := 1; float64(i) <= in.nfloc.fval; i++ {
		b.WriteString(in.getsval(in.fldtab[i]))
		if float64(i) < in.nfloc.fval {
			b.WriteString(sep)
		}
	}
	rec := in.fldtab[0]
	rec.tval = tREC | tSTR | tDONTFREE
	rec.setSval(b.String())
	in.recBuf, in.recIsBuf = rec.sval, true
	in.donerec = true
}

// syntax reports a syntax error; only the first three are printed. It makes the exit status 2.
func (in *Interp) syntax(format string, args ...any) {
	in.syntaxCount++
	if in.syntaxCount > 3 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: ", in.cmdname)
	b.WriteString(cfmt(format, args...))
	fmt.Fprintf(&b, " at source line %d", in.lex.lineno)
	if in.curfname != "" {
		fmt.Fprintf(&b, " in function %s", in.curfname)
	}
	if in.compileTime == compiling {
		if src, ok := in.lex.cursource(); ok {
			fmt.Fprintf(&b, " source file %s", src)
		}
	}
	b.WriteByte('\n')
	in.errout(b.String())
	in.errorflag = 2
	in.eprint()
}

// bracecheck reports the unbalanced braces, brackets and parentheses after a syntax error.
func (in *Interp) bracecheck() {
	if in.bracechecked {
		return
	}
	in.bracechecked = true
	for c := in.lex.input(); c != 0; c = in.lex.input() {
		in.lex.bclass(c)
	}
	in.bcheck2(in.lex.bracecnt, '{', '}')
	in.bcheck2(in.lex.brackcnt, '[', ']')
	in.bcheck2(in.lex.parencnt, '(', ')')
}

func (in *Interp) bcheck2(n int, c1, c2 byte) {
	switch {
	case n == 1:
		in.errout(fmt.Sprintf("\tmissing %c\n", c2))
	case n > 1:
		in.errout(fmt.Sprintf("\t%d missing %c's\n", n, c2))
	case n == -1:
		in.errout(fmt.Sprintf("\textra %c\n", c2))
	case n < -1:
		in.errout(fmt.Sprintf("\t%d extra %c's\n", -n, c2))
	}
}

// fatalError unwinds the interpreter after a fatal error has been reported; the exit status is 2.
type fatalError struct{}

// fatal reports a fatal error with its context and stops the program.
func (in *Interp) fatal(format string, args ...any) {
	in.flushStdout()
	in.errout(fmt.Sprintf("%s: %s", in.cmdname, cfmt(format, args...)))
	in.errorContext()
	panic(fatalError{})
}

// warning reports a problem with its context and carries on.
func (in *Interp) warning(format string, args ...any) {
	in.flushStdout()
	in.errout(fmt.Sprintf("%s: %s", in.cmdname, cfmt(format, args...)))
	in.errorContext()
}

// errorContext ends an error message with the input record, source line and program text where it happened.
func (in *Interp) errorContext() {
	var b strings.Builder
	b.WriteByte('\n')
	if in.compileTime != errorPrinting {
		if in.nrloc != nil && in.nrloc.fval > 0 {
			fmt.Fprintf(&b, " input record number %d", int(in.fnrloc.fval))
			if fn := in.filenameloc.sval; fn != "-" {
				fmt.Fprintf(&b, ", file %s", fn)
			}
			b.WriteByte('\n')
		}
		if in.curnode != nil {
			fmt.Fprintf(&b, " source line number %d", in.curnode.lineno)
		} else if in.lex.lineno != 0 {
			fmt.Fprintf(&b, " source line number %d", in.lex.lineno)
		}
		if in.compileTime == compiling {
			if src, ok := in.lex.cursource(); ok {
				fmt.Fprintf(&b, " source file %s", src)
			}
		}
		b.WriteByte('\n')
	}
	in.errout(b.String())
	if in.compileTime != errorPrinting {
		in.eprint()
	}
}

// eprint prints the program text around a compile-time error, marking the point the lexer had reached
// with >>> and <<<.
func (in *Interp) eprint() {
	lx := in.lex
	if in.compileTime != compiling || in.eprinted {
		return
	}
	in.eprinted = true
	if lx.ep == 0 {
		return
	}
	eb := lx.ebuf[:]
	p := lx.ep - 1
	if p > 0 && eb[p] == '\n' {
		p--
	}
	for ; p > 0 && eb[p] != '\n' && eb[p] != 0; p-- {
	}
	for eb[p] == '\n' {
		p++
	}
	var b []byte
	b = append(b, " context is\n\t"...)
	q := lx.ep - 1
	for ; q >= p && eb[q] != ' ' && eb[q] != '\t' && eb[q] != '\n'; q-- {
	}
	for ; p < q; p++ {
		if eb[p] != 0 {
			b = append(b, eb[p])
		}
	}
	b = append(b, " >>> "...)
	for ; p < lx.ep; p++ {
		if eb[p] != 0 {
			b = append(b, eb[p])
		}
	}
	b = append(b, " <<< "...)
	// After the last slot the original reads the byte past its buffer, which is 0: nothing more is printed.
	if lx.ep < len(eb) && eb[lx.ep] != 0 {
		for c := lx.input(); c != '\n' && c != 0; c = lx.input() {
			b = append(b, c)
			lx.bclass(c)
		}
	}
	b = append(b, '\n')
	in.errout(string(b))
	lx.ep = 0
}

// errcheck turns a domain or range error of a math function into a warning and the value 1.
func (in *Interp) errcheck(x float64, edom, erange bool, s string) float64 {
	switch {
	case edom:
		in.warning("%s argument out of domain", s)
		x = 1
	case erange:
		in.warning("%s result out of range", s)
		x = 1
	}
	return x
}

// isclvar reports whether s has the form var=something.
func isclvar(s string) bool {
	if s == "" || !(isAlpha(s[0]) || s[0] == '_') {
		return false
	}
	i := 1
	for i < len(s) && (isAlnum(s[i]) || s[i] == '_') {
		i++
	}
	return i < len(s) && s[i] == '='
}

// isValidNumber reports whether s looks like a number: optional blanks, a decimal or (after a sign)
// hexadecimal number as strtod reads it, or +nan, -nan, +inf, -inf; no other letters, and no hexadecimal
// without a sign. noTrailing tells whether only blanks follow; with trailingOK the number may be followed by
// anything.
func isValidNumber(s string, trailingOK bool) (r float64, noTrailing bool, ok bool) {
	for len(s) > 0 && isSpace(s[0]) {
		s = s[1:]
	}
	at := func(i int) byte {
		if i < len(s) {
			return s[i]
		}
		return 0
	}
	// no hexadecimal floating point
	if at(0) == '0' && toLowerASCII(at(1)) == 'x' && isXDigit(at(2)) {
		return 0, false, false
	}
	if at(0) == '+' || at(0) == '-' {
		isNaN := len(s) >= 4 && strings.EqualFold(s[1:4], "nan")
		isInf := len(s) >= 4 && strings.EqualFold(s[1:4], "inf")
		if (isNaN || isInf) && (isSpace(at(4)) || at(4) == 0) {
			// +nan, -nan, +inf, -inf
		} else if !isDigit(at(1)) && at(1) != '.' {
			return 0, false, false
		}
	} else if !isDigit(at(0)) && at(0) != '.' {
		return 0, false, false
	}
	v, n, erange := libc.Strtod(s)
	if n == 0 || erange {
		return 0, false, false
	}
	if v != v && s[0] == '-' && !signbit(v) {
		v = -v
	}
	rest := s[n:]
	for len(rest) > 0 && isSpace(rest[0]) {
		rest = rest[1:]
	}
	noTrailing = rest == ""
	return v, noTrailing, noTrailing || trailingOK
}

// isNumber reports whether all of s is a number, and its value.
func isNumber(s string) (float64, bool) {
	r, _, ok := isValidNumber(s, false)
	return r, ok
}
