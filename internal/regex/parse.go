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

// Package regex is awk's regular expression machinery from b.c: a parser for POSIX-style extended regular
// expressions with the original's own extensions and quirks, and a lazily built DFA (states are created the
// first time a transition is needed) with the shortest-match, leftmost-longest, non-empty and stream matchers
// the interpreter uses for ~, match(), sub/gsub, split and FS/RS separators.
//
// Characters are UTF-8 sequences decoded with package u8; the end of a string is fed to the automaton as the
// character 0, which is how $ matches. ^ is the pseudo-character hat, fed once before the first character.
package regex

import (
	"fmt"
	"runtime"

	"github.com/onetrueawk/awk/internal/u8"
)

const (
	nchars = 1256 + 3   // the original's NCHARS
	hat    = nchars + 2 // matches ^ in a regular expression (U+04ED collides with it, as in the original)
)

// CharSigned reports whether C's plain char is signed on this platform, as it is on x86 and not on ARM: the
// original stores some bytes in a char, so bytes above 127 behave differently between the two.
var CharSigned = runtime.GOARCH == "amd64" || runtime.GOARCH == "386"

// cchar is byte b stored in a C char and read back as an int.
func cchar(b byte) int {
	if CharSigned {
		return int(int8(b))
	}
	return int(b)
}

// Token and node types. Single characters ('^', '$', '(', ')') and 0 for the end of the expression are their
// own tokens.
const (
	tChar = 256 + iota
	tDot
	tAll
	tCCL
	tNCCL
	tOr
	tStar
	tPlus
	tQuest
	tEmptyRE
	tZero
	tFinal
	tCat
)

// node is a parse tree node. Leaves (CHAR, DOT, ALL, CCL, NCCL, EMPTYRE, FINAL) carry their position index and
// value; STAR, PLUS, QUEST and ZERO have one child (left); CAT and OR have two.
type node struct {
	typ         int
	index       int
	left, right *node
	parent      *node
	val         int   // CHAR: the character
	ccl         []int // CCL, NCCL: the class members, terminated by 0
}

func op2(typ int, left, right *node) *node {
	return &node{typ: typ, left: left, right: right}
}

// Error is a regular expression the original rejects; its text is what follows "awk: " in the original's
// fatal error message.
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

// compiler holds the state of one compilation (the original's file-level statics). The expression being
// parsed may be rewritten while it is parsed, to expand repetitions such as a{2,3} into aaa?, so positions
// are byte offsets into base, the current text.
type compiler struct {
	base     string // current text: the expression, or its rewritten form
	pos      int    // next byte to scan (prestr)
	lastre   string // the expression as given, for error messages
	starttok int    // start of the token just scanned
	lastatom int    // start of the last atom, for repetitions; -1 when there is none
	rtok     int    // next token
	rlxval   int    // its character value
	rlxstr   string // its class text (CCL, NCCL)
	parens   int    // unmatched '(' so far; a ')' with none open is an ordinary character (POSIX)
	poscnt   int
}

// at returns the byte at offset i of the current text, or 0 at and past its end, like reading the C string.
func (c *compiler) at(i int) byte {
	if i < len(c.base) {
		return c.base[i]
	}
	return 0
}

// cstr is the C string at offset i: the rest of the text.
func (c *compiler) cstr(i int) string {
	if i < len(c.base) {
		return c.base[i:]
	}
	return ""
}

func (c *compiler) fatal(format string, args ...any) {
	panic(&Error{fmt.Sprintf(format, args...)})
}

// trunc20 is printf's %.20s.
func trunc20(s string) string {
	if len(s) > 20 {
		return s[:20]
	}
	return s
}

// Compile parses re and builds its automaton (mkdfa). An anchored automaton matches only at the start of the
// text it is given: the callers that look for leftmost matches move the start themselves.
func Compile(re string, anchor bool) (f *FA, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			f, err = nil, e
		}
	}()
	c := &compiler{base: re, lastre: re, lastatom: -1}
	p := c.reparse()
	// Put ALL STAR in front of the expression and FINAL after it.
	p1 := op2(tCat, op2(tStar, op2(tAll, nil, nil), nil), p)
	p1 = op2(tCat, p1, op2(tFinal, nil, nil))
	c.penter(p1)
	f = newFA(c.poscnt)
	f.cfoll(p1)
	f.resizeState(1)
	f.posns[0] = make([]int, f.re[0].lfollow[0])
	f.posns[1] = []int{0}
	f.initstat = f.makeinit(anchor)
	f.anchor = anchor
	f.restr = re
	return f, nil
}

// penter sets up parent pointers and leaf position indices.
func (c *compiler) penter(p *node) {
	switch p.typ {
	case tEmptyRE, tCCL, tNCCL, tChar, tDot, tFinal, tAll:
		p.index = c.poscnt
		c.poscnt++
	case tStar, tPlus, tQuest:
		c.penter(p.left)
		p.left.parent = p
	case tCat, tOr:
		c.penter(p.left)
		c.penter(p.right)
		p.left.parent = p
		p.right.parent = p
	case tZero:
		// The repeated atom of x{0} matches nothing and takes no positions.
	default:
		c.fatal("can't happen: unknown type %d in penter", p.typ)
	}
}

// hexstr evaluates up to max hex digits at *pp, advancing past them; no digits give 0. Like the original's
// int arithmetic, eight digits wrap around.
func (c *compiler) hexstr(s string, pp *int, max int) int {
	var n int32
	p := *pp
	for i := 0; i < max && p < len(s) && isxdigit(s[p]); i, p = i+1, p+1 {
		switch b := s[p]; {
		case b >= '0' && b <= '9':
			n = 16*n + int32(b-'0')
		case b >= 'a' && b <= 'f':
			n = 16*n + int32(b-'a'+10)
		default:
			n = 16*n + int32(b-'A'+10)
		}
	}
	*pp = p
	return int(n)
}

func isoctdigit(b byte) bool { return b >= '0' && b <= '7' }

// byteAt is s[i], or 0 past the end of s.
func byteAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// quoted picks up the escaped character after a backslash at *pp in s and advances past it. Only one byte is
// taken, even when it starts a UTF-8 sequence (the original's known bug).
func (c *compiler) quoted(s string, pp *int) int {
	p := *pp
	ch := int(byteAt(s, p))
	p++
	switch ch {
	case 't':
		ch = '\t'
	case 'n':
		ch = '\n'
	case 'f':
		ch = '\f'
	case 'r':
		ch = '\r'
	case 'b':
		ch = '\b'
	case 'v':
		ch = '\v'
	case 'a':
		ch = '\a'
	case '\\':
		ch = '\\'
	case 'x': // 2 hex digits follow; none gives a NUL
		ch = c.hexstr(s, &p, 2)
	case 'u': // a Unicode code point, up to 8 hex digits
		ch = c.hexstr(s, &p, 8)
	default:
		if isoctdigit(byte(ch)) { // \d \dd \ddd
			n := ch - '0'
			if isoctdigit(byteAt(s, p)) {
				n = 8*n + int(s[p]-'0')
				p++
				if isoctdigit(byteAt(s, p)) {
					n = 8*n + int(s[p]-'0')
					p++
				}
			}
			ch = n
		}
	}
	*pp = p
	return ch
}

// cclenter expands a character class body into its list of members, terminated by 0: escapes are decoded and
// ranges a-z (also over UTF-8 characters) are spelled out; a range whose end is below its start is dropped
// together with its start.
func (c *compiler) cclenter(s string) []int {
	var buf []int
	for p := 0; p < len(s); {
		ch, n := u8.Rune(s[p:])
		p += n
		cv := int(ch)
		if cv == '\\' {
			cv = c.quoted(s, &p)
		} else if cv == '-' && len(buf) > 0 && buf[len(buf)-1] != 0 {
			if p < len(s) {
				cv = buf[len(buf)-1]
				r2, n2 := u8.Rune(s[p:])
				p += n2
				c2 := int(r2)
				if c2 == '\\' {
					c2 = c.quoted(s, &p)
				}
				if cv > c2 { // empty; ignore
					buf = buf[:len(buf)-1]
					continue
				}
				for cv < c2 {
					cv++
					buf = append(buf, cv)
				}
				continue
			}
		}
		buf = append(buf, cv)
	}
	return append(buf, 0)
}

// reparse parses the whole expression; the empty expression matches anything (GNU compatibility).
func (c *compiler) reparse() *node {
	c.pos = 0
	c.rtok = c.relex()
	if c.rtok == 0 {
		return op2(tEmptyRE, nil, nil)
	}
	np := c.regexp()
	if c.rtok != 0 {
		c.fatal("syntax error in regular expression %s at %s", c.lastre, c.cstr(c.pos))
	}
	return np
}

func (c *compiler) regexp() *node {
	return c.alt(c.concat(c.primary()))
}

func (c *compiler) primary() *node {
	switch c.rtok {
	case tChar:
		c.lastatom = c.starttok
		np := &node{typ: tChar, val: c.rlxval}
		c.rtok = c.relex()
		return c.unary(np)
	case tAll:
		c.rtok = c.relex()
		return c.unary(op2(tAll, nil, nil))
	case tEmptyRE:
		c.rtok = c.relex()
		return c.unary(op2(tEmptyRE, nil, nil))
	case tDot:
		c.lastatom = c.starttok
		c.rtok = c.relex()
		return c.unary(op2(tDot, nil, nil))
	case tCCL, tNCCL:
		np := &node{typ: c.rtok, ccl: c.cclenter(c.rlxstr)}
		c.lastatom = c.starttok
		c.rtok = c.relex()
		return c.unary(np)
	case '^':
		c.rtok = c.relex()
		return c.unary(&node{typ: tChar, val: hat})
	case '$':
		c.rtok = c.relex()
		return c.unary(&node{typ: tChar, val: 0})
	case '(':
		c.lastatom = c.starttok
		savelastatom := c.starttok // an offset, so it survives rewrites inside the group
		c.rtok = c.relex()
		if c.rtok == ')' { // special pleading for (): an empty class
			c.rtok = c.relex()
			return c.unary(&node{typ: tCCL, ccl: c.cclenter("")})
		}
		np := c.regexp()
		if c.rtok == ')' {
			c.lastatom = savelastatom
			c.rtok = c.relex()
			return c.unary(np)
		}
		c.fatal("syntax error in regular expression %s at %s", c.lastre, c.cstr(c.pos))
	}
	c.fatal("illegal primary in regular expression %s at %s", c.lastre, c.cstr(c.pos))
	return nil
}

func (c *compiler) concat(np *node) *node {
	switch c.rtok {
	case tChar, tDot, tAll, tCCL, tNCCL, '$', '(':
		return c.concat(op2(tCat, np, c.primary()))
	case tEmptyRE:
		c.rtok = c.relex()
		return c.concat(op2(tCat, &node{typ: tCCL, ccl: c.cclenter("")}, c.primary()))
	}
	return np
}

func (c *compiler) alt(np *node) *node {
	if c.rtok == tOr {
		c.rtok = c.relex()
		return c.alt(op2(tOr, np, c.concat(c.primary())))
	}
	return np
}

func (c *compiler) unary(np *node) *node {
	switch c.rtok {
	case tStar, tPlus, tQuest, tZero:
		t := c.rtok
		c.rtok = c.relex()
		return c.unary(op2(t, np, nil))
	}
	return np
}

// Character classes of the POSIX locale over ASCII; bytes above 127 are in none of them.
var charclasses = []struct {
	name string
	fn   func(byte) bool
}{
	{"alnum", func(b byte) bool { return isalpha(b) || isdigit(b) }},
	{"alpha", isalpha},
	{"blank", func(b byte) bool { return b == ' ' || b == '\t' }},
	{"cntrl", func(b byte) bool { return b < 32 || b == 127 }},
	{"digit", isdigit},
	{"graph", func(b byte) bool { return b > 32 && b < 127 }},
	{"lower", func(b byte) bool { return b >= 'a' && b <= 'z' }},
	{"print", func(b byte) bool { return b >= 32 && b < 127 }},
	{"punct", func(b byte) bool { return b > 32 && b < 127 && !isalpha(b) && !isdigit(b) }},
	{"space", func(b byte) bool { return b == ' ' || (b >= '\t' && b <= '\r') }},
	{"upper", func(b byte) bool { return b >= 'A' && b <= 'Z' }},
	{"xdigit", isxdigit},
}

func isalpha(b byte) bool  { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func isdigit(b byte) bool  { return b >= '0' && b <= '9' }
func isxdigit(b byte) bool { return isdigit(b) || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F' }

// Repetition rewrites (replaceRepeat's special cases).
const (
	repeatSimple = iota
	repeatPlusAppended
	repeatWithQ
	repeatZero
)

// replaceRepeat rewrites the text so that the repetition token at reptok (reptoklen bytes) applied to the atom
// atom..atom+atomlen becomes plain ERE: the atom repeated, followed by ? or + as needed. Scanning resumes
// after the kept prefix, so the copies are scanned again.
func (c *compiler) replaceRepeat(reptok, reptoklen, atom, atomlen, firstnum, secondnum, special int) int {
	ret := 1
	initQ := 0
	if firstnum == 0 { // first added char will be ?
		initQ = 1
	}
	nQReps := secondnum - firstnum // m>n, so reduce until {1,m-n} left
	prefixLength := reptok         // prefix includes first rep
	atomText := c.base[atom : atom+atomlen]
	buf := []byte(c.base[:prefixLength])
	j := prefixLength
	if special == repeatZero {
		j -= atomlen
		buf = append(buf[:j], '(', ')')
		j += 2
	}
	for i := 1; i < firstnum; i++ { // copy x reps
		buf = append(buf, atomText...)
	}
	if special == repeatPlusAppended {
		buf = append(buf, '+')
	} else if special == repeatWithQ {
		if initQ != 0 {
			buf = append(buf, '?')
		}
		for i := initQ; i < nQReps; i++ { // copy x? reps
			buf = append(buf, atomText...)
			buf = append(buf, '?')
		}
	}
	buf = append(buf, c.cstr(reptok+reptoklen)...)
	c.base = string(buf)
	c.pos = prefixLength
	if special == repeatZero {
		c.pos -= atomlen
		ret++
	}
	return ret
}

// repeat replaces a bound {n}, {n,} or {n,m} after an atom by equivalent ERE text. Without an atom it
// returns 0 and the caller treats the bound like +.
func (c *compiler) repeat(reptok, reptoklen, atom, atomlen, firstnum, secondnum int) int {
	if atom < 0 {
		return 0
	}
	switch {
	case secondnum < 0: // {n,}: repeat n-1 times followed by +
		if firstnum < 2 {
			c.fatal("internal error") // 0 or 1 are handled before we get here
		}
		return c.replaceRepeat(reptok, reptoklen, atom, atomlen, firstnum, secondnum, repeatPlusAppended)
	case firstnum == secondnum: // {n} or {n,n}: simply repeat n-1 times
		if firstnum == 0 { // {0} or {0,0}: the text may shrink
			return c.replaceRepeat(reptok, reptoklen, atom, atomlen, firstnum, secondnum, repeatZero)
		}
		return c.replaceRepeat(reptok, reptoklen, atom, atomlen, firstnum, secondnum, repeatSimple)
	case firstnum < secondnum: // x{n,m} => xx...x{1,m-n+1} => xx...x?x?x?..x?
		return c.replaceRepeat(reptok, reptoklen, atom, atomlen, firstnum, secondnum, repeatWithQ)
	}
	c.fatal("internal error")
	return 0
}

// relex is the lexical analyzer for reparse.
func (c *compiler) relex() int {
rescan:
	c.starttok = c.pos
	if r, n := u8.Rune(c.cstr(c.pos)); n > 1 {
		c.rlxval = int(r)
		c.pos += n
		c.starttok = c.pos // after the character: such an atom is empty for a repetition
		return tChar
	}
	ch := c.at(c.pos)
	c.pos++
	switch ch {
	case '|':
		return tOr
	case '*':
		return tStar
	case '+':
		return tPlus
	case '?':
		return tQuest
	case '.':
		return tDot
	case 0:
		c.pos--
		return 0
	case '^', '$':
		return int(ch)
	case '(':
		c.parens++
		return int(ch)
	case ')':
		if c.parens > 0 {
			c.parens--
			return int(ch)
		}
		// unmatched close parenthesis; per POSIX, treat as literal
		c.rlxval = int(ch)
		return tChar
	case '\\':
		c.rlxval = c.quoted(c.base, &c.pos)
		return tChar
	default:
		c.rlxval = int(ch)
		return tChar
	case '[':
		return c.relexClass()
	case '{':
		if !isdigit(c.at(c.pos)) { // just a { char, not a repetition
			c.rlxval = int(ch)
			return tChar
		}
		num, n, m := 0, -1, -1
		commafound, digitfound := false, false
		startreptok := c.pos - 1
		for {
			ch = c.at(c.pos)
			c.pos++
			switch {
			case ch == '}':
				if commafound {
					if digitfound { // {n,m}
						m = num
						if m < n {
							c.fatal("illegal repetition expression: class %s", trunc20(c.lastre))
						}
						if n == 0 && m == 1 {
							return tQuest
						}
					} else { // {n,}
						if n == 0 {
							return tStar
						} else if n == 1 {
							return tPlus
						}
					}
				} else {
					if !digitfound { // {}
						c.fatal("illegal repetition expression: class %s", trunc20(c.lastre))
					}
					n, m = num, num // {n} same as {n,n}
				}
				atomlen := 0
				if c.lastatom >= 0 {
					atomlen = startreptok - c.lastatom
				}
				if c.repeat(c.starttok, c.pos-c.starttok, c.lastatom, atomlen, n, m) > 0 {
					if n == 0 && m == 0 {
						return tZero
					}
					goto rescan // must rescan input for next token
				}
				// Failed to replace: eat up {...} characters and treat like just PLUS
				return tPlus
			case ch == 0:
				c.fatal("nonterminated character class %s", trunc20(c.lastre))
			case isdigit(ch):
				num = 10*num + int(ch-'0')
				if num > 255 {
					c.fatal("repetition count %s too large", trunc20(c.lastre))
				}
				digitfound = true
			case ch == ',':
				if commafound {
					c.fatal("illegal repetition expression: class %s", trunc20(c.lastre))
				}
				// looking for {n,} or {n,m}
				commafound = true
				n = num
				digitfound = false // reset
				num = 0
			default:
				c.fatal("illegal repetition expression: class %s", trunc20(c.lastre))
			}
		}
	}
}

// relexClass scans a bracket expression after its '[': [:class:] names are expanded to their members here,
// escapes are kept for cclenter, and [.x.] or [=x=] followed by ']' stand for the single character x.
func (c *compiler) relexClass() int {
	var buf []byte
	cflag := false
	if c.at(c.pos) == '^' {
		cflag = true
		c.pos++
	}
	for {
		if r, n := u8.Rune(c.cstr(c.pos)); n > 1 {
			c.rlxval = int(r)
			buf = append(buf, c.base[c.pos:c.pos+n]...)
			c.pos += n
			continue
		}
		ch := c.at(c.pos)
		c.pos++
		switch {
		case ch == '\\':
			buf = append(buf, '\\')
			ch = c.at(c.pos)
			c.pos++
			if ch == 0 {
				c.fatal("nonterminated character class %s...", trunc20(c.lastre))
			}
			buf = append(buf, ch)
		case ch == '[' && c.at(c.pos) == ':':
			// POSIX char class names, Dag-Erling Smorgrav, des@ofug.org
			found := -1
			for i, cc := range charclasses {
				if len(c.cstr(c.pos+1)) >= len(cc.name) && c.base[c.pos+1:c.pos+1+len(cc.name)] == cc.name {
					found = i
					break
				}
			}
			if found >= 0 && c.at(c.pos+1+len(charclasses[found].name)) == ':' &&
				c.at(c.pos+2+len(charclasses[found].name)) == ']' {
				c.pos += len(charclasses[found].name) + 3
				// Start at 1, not 0: the class string is NUL-terminated, so NUL can't be a member.
				for i := 1; i <= 255; i++ {
					if charclasses[found].fn(byte(i)) {
						if i == '\\' { // escape backslash
							buf = append(buf, '\\')
						}
						buf = append(buf, byte(i))
					}
				}
			} else {
				buf = append(buf, ch)
			}
		case ch == '[' && (c.at(c.pos) == '.' || c.at(c.pos) == '='):
			// Collating symbol [.x.] or equivalence class [=x=]: with no locale support, just the character x,
			// and only when the bracket expression ends right after it. The character is a C char (see
			// CharSigned).
			delim := c.at(c.pos)
			c.pos++
			single := cchar(c.at(c.pos))
			c.pos++
			if c.at(c.pos) == delim && c.at(c.pos+1) == ']' {
				c.pos += 2
				if c.at(c.pos) == ']' {
					c.pos++
					c.rlxval = single
					return tChar
				}
			}
		case ch == 0:
			c.fatal("nonterminated character class %s", trunc20(c.lastre))
		case len(buf) == 0: // 1st char is special
			buf = append(buf, ch)
		case ch == ']':
			c.rlxstr = string(buf)
			if cflag {
				return tNCCL
			}
			return tCCL
		default:
			buf = append(buf, ch)
		}
	}
}
