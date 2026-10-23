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

package regex

import "github.com/onetrueawk/awk/internal/u8"

// rrow is one position (leaf) of the expression: its type, its character or class, and its follow set.
type rrow struct {
	ltype   int
	val     int
	ccl     []int
	lfollow []int // count, then positions in decreasing order
}

// FA is a compiled expression: its positions and the deterministic automaton built from them on demand.
// State 1 is the dead state (no positions), state 2 the start state before ^; posns lists each state's
// positions as a count followed by the positions in decreasing order. An FA is not safe for concurrent use.
type FA struct {
	re       []rrow
	accept   int           // the FINAL position
	gototab  []map[int]int // cached transitions: state, character -> state
	out      []bool        // accepting states
	posns    [][]int       // positions of each state
	curstat  int           // highest state number in use
	initstat int           // the start state (after ^)
	anchor   bool          // matches only at the start
	restr    string        // the expression's text
	setvec   []int         // scratch position set
	tmpset   []int         // scratch state
	setcnt   int
}

func newFA(npos int) *FA {
	return &FA{
		re:     make([]rrow, npos),
		accept: npos - 1,
		setvec: make([]int, npos),
		tmpset: make([]int, npos+1),
	}
}

// Source is the text of the expression.
func (f *FA) Source() string { return f.restr }

// resizeState makes room for state number state.
func (f *FA) resizeState(state int) {
	for len(f.out) <= state {
		f.gototab = append(f.gototab, nil)
		f.out = append(f.out, false)
		f.posns = append(f.posns, nil)
	}
}

func (f *FA) makeinit(anchor bool) int {
	f.curstat = 2
	f.resizeState(2)
	f.out[2] = false
	k := f.re[0].lfollow[0]
	f.posns[2] = append([]int(nil), f.re[0].lfollow[:k+1]...)
	if f.posns[2][1] == f.accept {
		f.out[2] = true
	}
	f.gototab[2] = nil
	f.curstat = f.cgoto(2, hat)
	if anchor {
		f.posns[2][0] = k - 1 // leave out position 0, the leading ALL
		for i := 0; i < k; i++ {
			f.posns[0][i] = f.posns[2][i]
		}
		f.out[0] = f.out[2]
		if f.curstat != 2 {
			f.posns[f.curstat][0]-- // and from the state after ^
		}
	}
	return f.curstat
}

// cfoll enters the follow set of each leaf of v into its position's lfollow.
func (f *FA) cfoll(v *node) {
	switch v.typ {
	case tEmptyRE, tCCL, tNCCL, tChar, tDot, tFinal, tAll:
		r := &f.re[v.index]
		r.ltype = v.typ
		r.val = v.val
		r.ccl = v.ccl
		for i := 0; i <= f.accept; i++ {
			f.setvec[i] = 0
		}
		f.setcnt = 0
		f.follow(v) // computes setvec and setcnt
		p := make([]int, f.setcnt+1)
		p[0] = f.setcnt
		j := 1
		for i := f.accept; i >= 0; i-- {
			if f.setvec[i] == 1 {
				p[j] = i
				j++
			}
		}
		r.lfollow = p
	case tStar, tPlus, tQuest:
		f.cfoll(v.left)
	case tCat, tOr:
		f.cfoll(v.left)
		f.cfoll(v.right)
	}
}

// first collects the initially active leaves of p into setvec; it returns 0 if p matches the empty string.
func (f *FA) first(p *node) int {
	switch p.typ {
	case tEmptyRE, tCCL, tNCCL, tChar, tDot, tFinal, tAll:
		lp := p.index
		if p.typ == tEmptyRE {
			f.setvec[lp] = 0
			return 0
		}
		if f.setvec[lp] != 1 {
			f.setvec[lp] = 1
			f.setcnt++
		}
		if p.typ == tCCL && p.ccl[0] == 0 {
			return 0 // empty CCL
		}
		return 1
	case tPlus:
		if f.first(p.left) == 0 {
			return 0
		}
		return 1
	case tStar, tQuest:
		f.first(p.left)
		return 0
	case tCat:
		if f.first(p.left) == 0 && f.first(p.right) == 0 {
			return 0
		}
		return 1
	case tOr:
		b := f.first(p.right)
		if f.first(p.left) == 0 || b == 0 {
			return 0
		}
		return 1
	}
	return 0 // ZERO
}

// follow collects the leaves that can follow v into setvec.
func (f *FA) follow(v *node) {
	if v.typ == tFinal {
		return
	}
	p := v.parent
	switch p.typ {
	case tStar, tPlus:
		f.first(v)
		f.follow(p)
	case tOr, tQuest:
		f.follow(p)
	case tCat:
		if v == p.left { // v is left child of p
			if f.first(p.right) == 0 {
				f.follow(p)
			}
		} else { // v is right child
			f.follow(p)
		}
	}
}

// member reports whether c is in the 0-terminated class s.
func member(c int, s []int) bool {
	for _, m := range s {
		if m == 0 {
			break
		}
		if c == m {
			return true
		}
	}
	return false
}

// next is the transition from state s on character c, computed and cached on first use.
func (f *FA) next(s, c int) int {
	if ns := f.gototab[s][c]; ns != 0 {
		return ns
	}
	return f.cgoto(s, c)
}

func (f *FA) setGototab(s, c, val int) {
	if f.gototab[s] == nil {
		f.gototab[s] = make(map[int]int)
	}
	f.gototab[s][c] = val
}

// cgoto computes the state reached from s on character c, adding it if it is new. Transitions on ^ are not
// cached.
func (f *FA) cgoto(s, c int) int {
	for i := 0; i <= f.accept; i++ {
		f.setvec[i] = 0
	}
	f.setcnt = 0
	f.resizeState(s)
	// compute positions of gototab[s,c] into setvec
	p := f.posns[s]
	for i := 1; i <= p[0]; i++ {
		r := &f.re[p[i]]
		k := r.ltype
		if k == tFinal {
			continue
		}
		if (k == tChar && c == r.val) ||
			(k == tDot && c != 0 && c != hat) ||
			(k == tAll && c != 0) ||
			(k == tEmptyRE && c != 0) ||
			(k == tCCL && member(c, r.ccl)) ||
			(k == tNCCL && !member(c, r.ccl) && c != 0 && c != hat) {
			q := r.lfollow
			for j := 1; j <= q[0]; j++ {
				if f.setvec[q[j]] == 0 {
					f.setcnt++
					f.setvec[q[j]] = 1
				}
			}
		}
	}
	// determine if setvec is a previous state
	f.tmpset[0] = f.setcnt
	j := 1
	for i := f.accept; i >= 0; i-- {
		if f.setvec[i] != 0 {
			f.tmpset[j] = i
			j++
		}
	}
	if f.curstat > s {
		f.resizeState(f.curstat)
	}
states:
	for i := 1; i <= f.curstat; i++ {
		p := f.posns[i]
		k := f.tmpset[0]
		if k != p[0] {
			continue
		}
		for j := 1; j <= k; j++ {
			if f.tmpset[j] != p[j] {
				continue states
			}
		}
		// setvec is state i
		if c != hat {
			f.setGototab(s, c, i)
		}
		return i
	}
	// add tmpset to current set of states
	f.curstat++
	f.resizeState(f.curstat)
	f.gototab[f.curstat] = nil
	f.posns[f.curstat] = append([]int(nil), f.tmpset[:f.setcnt+1]...)
	if c != hat {
		f.setGototab(s, c, f.curstat)
	}
	f.out[f.curstat] = f.setvec[f.accept] != 0
	return f.curstat
}

// Match reports whether s contains a match (the original's match(); it stops at the first accepting state).
func (f *FA) Match(s string) bool {
	st := f.initstat
	if f.out[st] {
		return true
	}
	for p := 0; ; {
		r, n := u8.Rune(s[p:])
		st = f.next(st, int(r))
		if f.out[st] {
			return true
		}
		if p >= len(s) {
			break
		}
		p += n
	}
	return false
}

// startState is the initial state, or state 2 when the text does not start at the beginning of a line (the
// callers' "initstat = 2" for matches after the first, so that ^ cannot match there).
func (f *FA) startState(bol bool) int {
	if bol {
		return f.initstat
	}
	return 2
}

// PMatch finds the leftmost longest match in s (pmatch, used by sub, gsub and match()). It returns its byte
// offset and length; the length is -1 (and the offset 0) when there is none.
func (f *FA) PMatch(s string, bol bool) (start, length int) {
	st := f.startState(bol)
	p := 0
	patlen := -1
	for {
		q := p
		dead := false
		for {
			if f.out[st] { // final state
				patlen = q - p
			}
			r, n := u8.Rune(s[q:])
			st = f.next(st, int(r))
			if st == 1 { // no transition
				if patlen >= 0 {
					return p, patlen
				}
				dead = true // no match here
				break
			}
			if q >= len(s) {
				break
			}
			q += n
		}
		if !dead {
			q++
			if f.out[st] {
				patlen = q - p - 1 // don't count $
			}
			if patlen >= 0 {
				return p, patlen
			}
		}
		st = 2
		if p >= len(s) {
			break
		}
		_, n := u8.Rune(s[p:])
		p += n
	}
	return 0, -1
}

// NEMatch finds the leftmost longest non-empty match in s (nematch, used for field and split separators).
// Unlike PMatch it moves its start one byte at a time.
func (f *FA) NEMatch(s string, bol bool) (start, length int, ok bool) {
	st := f.startState(bol)
	patlen := -1
	for p := 0; p < len(s); p++ {
		q := p
		dead := false
		for {
			if f.out[st] { // final state
				patlen = q - p
			}
			r, n := u8.Rune(s[q:])
			st = f.next(st, int(r))
			if st == 1 { // no transition
				if patlen > 0 {
					return p, patlen, true
				}
				dead = true // no nonempty match
				break
			}
			if q >= len(s) {
				break
			}
			q += n
		}
		if !dead {
			q++
			if f.out[st] {
				patlen = q - p - 1 // don't count $
			}
			if patlen > 0 {
				return p, patlen, true
			}
		}
		st = 2
	}
	return 0, -1, false
}

// Input is a byte stream the record reader matches separators on: Getc returns the next byte or -1 at end of
// input, and Ungetc pushes a byte back so that the next Getc returns it (several may be pushed back). The
// original pushes back with C's ungetc of a plain char: where char is signed (CharSigned), the byte 0xFF
// becomes EOF and the push-back fails ("unable to ungetc"), so an Input should fail on it there.
type Input interface {
	Getc() int
	Ungetc(c byte)
}

// runeAt decodes the character at buf[j], seeing the buffer as a C string: a NUL byte (the end of input is
// stored as one) ends it.
func runeAt(buf []byte, j int) (int, int) {
	end := j + 4
	if end > len(buf) {
		end = len(buf)
	}
	b := buf[j:end]
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	r, n := u8.Rune(string(b))
	return int(r), n
}

// FNEMatch is NEMatch on a stream (fnematch, used for regular expression record separators). It reads
// characters into buf until the leftmost longest non-empty match is complete, keeping at least u8.MaxLen
// bytes of look-ahead. On a match, the bytes read after it are pushed back, buf ends with the match, and
// start and length locate the match in buf. Without one, buf holds the rest of the input (up to a NUL byte).
func (f *FA) FNEMatch(in Input, bol bool) (buf []byte, start, length int, found bool) {
	st := f.startState(bol)
	patlen, patbeg := 0, 0
	// i: origin of the active substring; j: current character; len(buf): where the next byte goes.
	i, j := 0, 0
	for {
		if len(buf)-j < u8.MaxLen {
			for n := u8.MaxLen; n > 0; n-- {
				c := in.Getc()
				if c < 0 {
					buf = append(buf, 0)
					break
				}
				buf = append(buf, byte(c))
			}
		}
		c, n := runeAt(buf, j)
		j += n
		st = f.next(st, c)
		if f.out[st] { // final state
			patbeg = i
			patlen = j - i
			if c == 0 { // don't count $
				patlen--
			}
		}
		if c != 0 && st != 1 {
			continue // origin i still viable, next j
		}
		if patlen != 0 {
			break // best match found
		}
		// no match at origin i, next i and start over
		c, n = runeAt(buf, i)
		i += n
		if c == 0 {
			break // no match
		}
		j = i
		st = 2
	}
	if patlen != 0 {
		// The last character fed to the automaton is never part of the match: push back everything after
		// the match except the end of input's NUL.
		k := len(buf)
		for {
			k--
			if buf[k] != 0 {
				in.Ungetc(buf[k])
			}
			if k <= patbeg+patlen {
				break
			}
		}
		return buf[:k], patbeg, patlen, true
	}
	for i, b := range buf {
		if b == 0 {
			return buf[:i], 0, 0, false
		}
	}
	return buf, 0, 0, false
}
