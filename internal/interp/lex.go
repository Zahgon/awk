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
	"os"
	"sort"

	"github.com/onetrueawk/awk/internal/libc"
	"github.com/onetrueawk/awk/internal/u8"
)

// A keyword of the language with its token and token value.
type keyword struct {
	word string
	sub  int // the token value: a builtin number, or the token itself
	typ  int // the token
}

// keywords is kept sorted: it is binary searched.
var keywords = []keyword{
	{"BEGIN", XBEGIN, XBEGIN},
	{"END", XEND, XEND},
	{"NF", VARNF, VARNF},
	{"atan2", fATAN, BLTIN},
	{"break", BREAK, BREAK},
	{"close", CLOSE, CLOSE},
	{"continue", CONTINUE, CONTINUE},
	{"cos", fCOS, BLTIN},
	{"delete", DELETE, DELETE},
	{"do", DO, DO},
	{"else", ELSE, ELSE},
	{"exit", EXIT, EXIT},
	{"exp", fEXP, BLTIN},
	{"fflush", fFLUSH, BLTIN},
	{"for", FOR, FOR},
	{"func", FUNC, FUNC},
	{"function", FUNC, FUNC},
	{"getline", GETLINE, GETLINE},
	{"gsub", GSUB, GSUB},
	{"if", IF, IF},
	{"in", IN, IN},
	{"index", INDEX, INDEX},
	{"int", fINT, BLTIN},
	{"length", fLENGTH, BLTIN},
	{"log", fLOG, BLTIN},
	{"match", MATCHFCN, MATCHFCN},
	{"next", NEXT, NEXT},
	{"nextfile", NEXTFILE, NEXTFILE},
	{"print", PRINT, PRINT},
	{"printf", PRINTF, PRINTF},
	{"rand", fRAND, BLTIN},
	{"return", RETURN, RETURN},
	{"sin", fSIN, BLTIN},
	{"split", SPLIT, SPLIT},
	{"sprintf", SPRINTF, SPRINTF},
	{"sqrt", fSQRT, BLTIN},
	{"srand", fSRAND, BLTIN},
	{"sub", SUB, SUB},
	{"substr", SUBSTR, SUBSTR},
	{"system", fSYSTEM, BLTIN},
	{"tolower", fTOLOWER, BLTIN},
	{"toupper", fTOUPPER, BLTIN},
	{"while", WHILE, WHILE},
}

// parser holds the state of the lexer and of the grammar actions. The lexer reads the program text, from the
// command line or the -f files, one character at a time. Like the original it keeps a pushback stack and a ring
// buffer of the last characters read, which error messages print as the context of a syntax error.
type parser struct {
	in *Interp

	prog    string // the program argument, when there is no -f
	useProg bool
	progPos int

	pfiles   []string  // program files from -f
	curpfile int       // current program file
	yyin     *inStream // open program file, or nil
	pfile    *os.File  // the os file behind yyin, unless it is standard input
	pushback []byte    // pushed-back characters
	ebuf     [300]byte // ring buffer of the last characters read
	ep       int       // next slot of ebuf

	lineno   int
	bracecnt int
	brackcnt int
	parencnt int

	sc  bool // return a } right now
	reg bool // return a REGEXPR now

	// grammar state
	beginloc *Node // BEGIN actions
	endloc   *Node // END actions
	infunc   bool  // in the parameter list or body of a function
	inloop   int   // depth of while, for and do loops
	arglist  *Node // parameters of the current function
	paircnt  int   // pat1, pat2 patterns so far
}

func (lx *parser) pgetc() byte {
	for {
		if lx.yyin == nil {
			if lx.curpfile >= len(lx.pfiles) {
				return 0
			}
			name := lx.pfiles[lx.curpfile]
			if name == "-" {
				lx.yyin = lx.in.stdin
			} else {
				f, err := os.Open(name)
				if err != nil {
					lx.in.fatal("can't open file %s", name)
				}
				lx.pfile = f
				lx.yyin = newInStream(f, f)
			}
			lx.lineno = 1
		}
		if c := lx.yyin.Getc(); c != eof {
			return byte(c)
		}
		if lx.pfile != nil {
			lx.pfile.Close()
			lx.pfile = nil
		}
		lx.yyin = nil
		lx.curpfile++
	}
}

// cursource is the name of the program file being read, if the program comes from files.
func (lx *parser) cursource() (string, bool) {
	if len(lx.pfiles) == 0 {
		return "", false
	}
	if lx.curpfile < len(lx.pfiles) {
		return lx.pfiles[lx.curpfile], true
	}
	return lx.pfiles[lx.curpfile-1], true
}

// input returns the next character of the program, 0 at the end.
func (lx *parser) input() byte {
	var c byte
	switch {
	case len(lx.pushback) > 0:
		c = lx.pushback[len(lx.pushback)-1]
		lx.pushback = lx.pushback[:len(lx.pushback)-1]
	case lx.useProg:
		if lx.progPos < len(lx.prog) {
			c = lx.prog[lx.progPos]
			if c != 0 {
				lx.progPos++
			}
		}
	default:
		c = lx.pgetc()
	}
	if lx.ep >= len(lx.ebuf) {
		lx.ep = 0
	}
	lx.ebuf[lx.ep] = c
	if c != 0 {
		lx.ep++
	}
	return c
}

// unput puts a character back on the input.
func (lx *parser) unput(c byte) {
	if len(lx.pushback) >= 100 {
		lx.in.fatal("pushed back too much: %.20s...", string(lx.pushback))
	}
	lx.pushback = append(lx.pushback, c)
	lx.ep--
	if lx.ep < 0 {
		lx.ep = len(lx.ebuf) - 1
	}
}

// unputstr puts a string back on the input.
func (lx *parser) unputstr(s string) {
	for i := len(s) - 1; i >= 0; i-- {
		lx.unput(s[i])
	}
}

func (lx *parser) peek() byte {
	c := lx.input()
	lx.unput(c)
	return c
}

// bclass counts the braces, brackets and parentheses that bracecheck reports.
func (lx *parser) bclass(c byte) {
	switch c {
	case '{':
		lx.bracecnt++
	case '}':
		lx.bracecnt--
	case '[':
		lx.brackcnt++
	case ']':
		lx.brackcnt--
	case '(':
		lx.parencnt++
	case ')':
		lx.parencnt--
	}
}

// Token classes gettok returns besides single characters.
const (
	tokName   = 'a'
	tokNumber = '0'
)

// gettok reads the next raw token: a name, a number, or a single character (0 at the end).
func (lx *parser) gettok() (string, int) {
	c := lx.input()
	if c == 0 {
		return "", 0
	}
	if !isAlnum(c) && c != '.' && c != '_' {
		return string(c), int(c)
	}
	buf := []byte{c}
	if isAlpha(c) || c == '_' { // a name
		for {
			c = lx.input()
			if c == 0 {
				break
			}
			if isAlnum(c) || c == '_' {
				buf = append(buf, c)
			} else {
				lx.unput(c)
				break
			}
		}
		return string(buf), tokName
	}
	// maybe a number, but it could be a lone '.': read what could be part of one
	for {
		c = lx.input()
		if c == 0 {
			break
		}
		if isDigit(c) || c == 'e' || c == 'E' || c == '.' || c == '+' || c == '-' {
			buf = append(buf, c)
		} else {
			lx.unput(c)
			break
		}
	}
	_, n, _ := libc.Strtod(string(buf))
	if n == 0 {
		// Not a number at all: return the first character. The original cuts its buffer to that character
		// before putting "the rest" back, so the rest is dropped.
		return string(buf[:1]), int(buf[0])
	}
	lx.unputstr(string(buf[n:])) // put the rest back
	return string(buf[:n]), tokNumber
}

// Lex returns the next token for the parser.
func (lx *parser) lex(lval *yySymType) int {
	if lx.sc {
		lx.sc = false
		return '}'
	}
	if lx.reg {
		lx.reg = false
		return lx.regexpr(lval)
	}
	for {
		buf, c := lx.gettok()
		if c == 0 {
			return 0
		}
		if isAlpha(byte(c)) || c == '_' {
			return lx.word(buf, lval)
		}
		if c == tokNumber {
			if r, ok := isNumber(buf); ok {
				lval.cp = lx.in.symtab.set(buf, buf, r, tCON|tNUM)
			} else {
				lval.cp = lx.in.symtab.set(buf, buf, 0, tSTR)
			}
			return NUMBER
		}
		lval.i = c
		switch c {
		case '\n':
			lx.lineno++
			return NL
		case '\r', ' ', '\t': // \r assumes a \n is coming
		case '#': // comment to the end of the line
			var c byte
			for c = lx.input(); c != '\n' && c != 0; c = lx.input() {
			}
			lx.unput(c)
		case ';':
			return ';'
		case '\\':
			// every alternative peeks again, as in the original: each peek moves the error context
			if lx.peek() == '\n' {
				lx.input()
				lx.lineno++
			} else if lx.peek() == '\r' {
				lx.input()
				lx.input() // the \n
				lx.lineno++
			} else {
				return c
			}
		case '&':
			if lx.peek() == '&' {
				lx.input()
				return AND
			}
			return '&'
		case '|':
			if lx.peek() == '|' {
				lx.input()
				return BOR
			}
			return '|'
		case '!':
			if lx.peek() == '=' {
				lx.input()
				lval.i = NE
				return NE
			} else if lx.peek() == '~' {
				lx.input()
				lval.i = NOTMATCH
				return MATCHOP
			}
			return NOT
		case '~':
			lval.i = MATCH
			return MATCHOP
		case '<':
			if lx.peek() == '=' {
				lx.input()
				lval.i = LE
				return LE
			}
			lval.i = LT
			return LT
		case '=':
			if lx.peek() == '=' {
				lx.input()
				lval.i = EQ
				return EQ
			}
			lval.i = ASSIGN
			return ASGNOP
		case '>':
			if lx.peek() == '=' {
				lx.input()
				lval.i = GE
				return GE
			} else if lx.peek() == '>' {
				lx.input()
				lval.i = APPEND
				return APPEND
			}
			lval.i = GT
			return GT
		case '+':
			if lx.peek() == '+' {
				lx.input()
				lval.i = INCR
				return INCR
			} else if lx.peek() == '=' {
				lx.input()
				lval.i = ADDEQ
				return ASGNOP
			}
			return '+'
		case '-':
			if lx.peek() == '-' {
				lx.input()
				lval.i = DECR
				return DECR
			} else if lx.peek() == '=' {
				lx.input()
				lval.i = SUBEQ
				return ASGNOP
			}
			return '-'
		case '*':
			if lx.peek() == '=' { // *=
				lx.input()
				lval.i = MULTEQ
				return ASGNOP
			} else if lx.peek() == '*' { // ** or **=
				lx.input()
				if lx.peek() == '=' {
					lx.input()
					lval.i = POWEQ
					return ASGNOP
				}
				return POWER
			}
			return '*'
		case '/':
			return '/'
		case '%':
			if lx.peek() == '=' {
				lx.input()
				lval.i = MODEQ
				return ASGNOP
			}
			return '%'
		case '^':
			if lx.peek() == '=' {
				lx.input()
				lval.i = POWEQ
				return ASGNOP
			}
			return POWER
		case '$':
			buf, c := lx.gettok()
			if c > 0 && c < 256 && isAlpha(byte(c)) {
				if buf == "NF" { // very special
					lx.unputstr("(NF)")
					return INDIRECT
				}
				c := lx.peek()
				if c == '(' || c == '[' || (lx.infunc && lx.isarg(buf) >= 0) {
					lx.unputstr(buf)
					return INDIRECT
				}
				lval.cp = lx.in.symtab.set(buf, "", 0, tSTR|tNUM)
				return IVAR
			} else if c == 0 {
				lx.in.syntax("unexpected end of input after $")
				return ';'
			}
			lx.unputstr(buf)
			return INDIRECT
		case '}':
			lx.bracecnt--
			if lx.bracecnt < 0 {
				lx.in.syntax("extra }")
			}
			lx.sc = true
			return ';'
		case ']':
			lx.brackcnt--
			if lx.brackcnt < 0 {
				lx.in.syntax("extra ]")
			}
			return ']'
		case ')':
			lx.parencnt--
			if lx.parencnt < 0 {
				lx.in.syntax("extra )")
			}
			return ')'
		case '{':
			lx.bracecnt++
			return '{'
		case '[':
			lx.brackcnt++
			return '['
		case '(':
			lx.parencnt++
			return '('
		case '"':
			return lx.string(lval)
		default:
			return c
		}
	}
}

// string reads a string constant after its opening quote.
func (lx *parser) string(lval *yySymType) int {
	var buf []byte
	for c := lx.input(); c != '"'; c = lx.input() {
		switch c {
		case '\n', '\r', 0:
			lx.in.syntax("non-terminated string %.10s...", cString(string(buf)))
			if c == 0 { // hopeless
				lx.in.fatal("giving up")
			}
			lx.lineno++
		case '\\':
			c = lx.input()
			switch c {
			case '\n':
			case '"':
				buf = append(buf, '"')
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'f':
				buf = append(buf, '\f')
			case 'r':
				buf = append(buf, '\r')
			case 'b':
				buf = append(buf, '\b')
			case 'v':
				buf = append(buf, '\v')
			case 'a':
				buf = append(buf, '\a')
			case '\\':
				buf = append(buf, '\\')
			case '0', '1', '2', '3', '4', '5', '6', '7': // octal: \d \dd \ddd
				n := int(c - '0')
				if c = lx.peek(); c >= '0' && c < '8' {
					n = 8*n + int(lx.input()-'0')
					if c = lx.peek(); c >= '0' && c < '8' {
						n = 8*n + int(lx.input()-'0')
					}
				}
				buf = append(buf, byte(n))
			case 'x': // hexadecimal: \x and up to two digits
				if !isXDigit(lx.peek()) {
					lx.unput(c)
					break
				}
				n, i := 0, 0
				for ; i < 2; i++ {
					c = lx.input()
					if c == 0 {
						break
					}
					if !isXDigit(c) {
						lx.unput(c)
						break
					}
					n = 16*n + hexValue(c)
				}
				if i > 0 {
					buf = append(buf, byte(n))
				}
			case 'u': // a code point: \u and up to eight hexadecimal digits
				var n int32
				for i := 0; i < 8; i++ {
					c = lx.input()
					if !isXDigit(c) || c == 0 {
						break
					}
					n = 16*n + int32(hexValue(c))
				}
				lx.unput(c)
				buf = u8.AppendRune(buf, n)
			default:
				buf = append(buf, c)
			}
		default:
			buf = append(buf, c)
		}
	}
	s := cString(string(buf))
	// the symbol table name of a string constant is its text followed by a blank
	lval.cp = lx.in.symtab.set(cString(string(buf)+" "), s, 0, tCON|tSTR|tDONTFREE)
	return STRING
}

func hexValue(c byte) int {
	switch {
	case isDigit(c):
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

// word classifies a name: a keyword, a function argument, a function call or a variable.
func (lx *parser) word(w string, lval *yySymType) int {
	n := sort.Search(len(keywords), func(i int) bool { return keywords[i].word >= w })
	if n < len(keywords) && keywords[n].word == w {
		kp := keywords[n]
		lval.i = kp.sub
		switch kp.typ {
		case BLTIN:
			if kp.sub == fSYSTEM && lx.in.safe {
				lx.in.syntax("system is unsafe")
			}
		case FUNC:
			if lx.infunc {
				lx.in.syntax("illegal nested function")
			}
		case RETURN:
			if !lx.infunc {
				lx.in.syntax("return not in function")
			}
		case VARNF:
			lval.cp = lx.in.symtab.set("NF", "", 0, tNUM)
		}
		return kp.typ
	}
	c := lx.peek() // look for '('
	if c != '(' && lx.infunc {
		if n := lx.isarg(w); n >= 0 {
			lval.i = n
			return ARG
		}
	}
	lval.cp = lx.in.symtab.set(w, "", 0, tSTR|tNUM|tDONTFREE)
	if c == '(' {
		return CALL
	}
	return VAR
}

// startreg makes the next token a regular expression.
func (lx *parser) startreg() {
	lx.reg = true
}

// regexpr reads the text of a regular expression up to its closing slash, which is left for the parser.
func (lx *parser) regexpr(lval *yySymType) int {
	var buf []byte
	var c byte
	for c = lx.input(); c != '/' && c != 0; c = lx.input() {
		if c == '\n' {
			lx.in.syntax("newline in regular expression %.10s...", cString(string(buf)))
			lx.unput('\n')
			break
		} else if c == '\\' {
			buf = append(buf, '\\', lx.input())
		} else {
			buf = append(buf, c)
		}
	}
	if c == 0 {
		lx.in.syntax("non-terminated regular expression %.10s...", cString(string(buf)))
	}
	lval.s = cString(string(buf))
	lx.unput('/')
	return REGEXPR
}
