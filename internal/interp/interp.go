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
	"os/signal"
	"syscall"

	"github.com/onetrueawk/awk/internal/libc"
	"github.com/onetrueawk/awk/internal/regex"
	"github.com/onetrueawk/awk/internal/u8"
)

// Version is what --version prints.
const Version = "version 20260426"

// Compile states, which decide what error messages include.
const (
	running = iota
	compiling
	errorPrinting
)

// fFLUSHMode is the openfile mode that only finds an open file, for fflush.
const fFLUSHMode = fFLUSH

// Interp is one run of an awk program.
type Interp struct {
	cmdname string
	lex     *parser
	symtab  *Array

	literal0, nullloc *Cell
	nullnode          *Node

	fsloc, rsloc, ofsloc, orsloc, ofmtloc, convfmtloc, filenameloc *Cell
	nfloc, nrloc, fnrloc, subseploc, rstartloc, rlengthloc         *Cell
	symtabloc, argcloc, argvcell                                   *Cell

	// the current record and its fields
	fldtab    []*Cell
	nfields   int     // last allocated field
	lastfld   int     // last used field
	donefld   bool    // the record has been split into fields
	donerec   bool    // the record is valid (no field has changed)
	inputFS   *string // FS when the record was read
	recBuf    string  // the record buffer: the last record read or rebuilt
	recIsBuf  bool    // $0 is the record buffer
	infile    *inStream
	innew     bool // infile has not been read yet
	argno     int  // current ARGV index
	firsttime bool

	stdin                                   *inStream
	stdinStream, stdoutStream, stderrStream *stream
	files                                   []*fileEntry

	compileTime  int
	errorflag    int
	syntaxCount  int
	bracechecked bool
	eprinted     bool
	curfname     string
	winner       *Node
	curnode      *Node
	csv          bool
	safe         bool
	dbg          bool

	frames    []*frame
	pairstack [pa2Num]int
	random    libc.Random
	srandSeed float64

	fatab        map[faKey]*regex.FA
	posixChecked bool
	posixSub     bool
}

type faKey struct {
	re     string
	anchor bool
}

// maxFA bounds the cache of regular expressions compiled at run time.
const maxFA = 128

// makedfa compiles a regular expression; at run time compiled expressions are cached. A bad expression is
// fatal.
func (in *Interp) makedfa(s string, anchor bool) *regex.FA {
	key := faKey{s, anchor}
	if in.compileTime == running {
		if f, ok := in.fatab[key]; ok {
			return f
		}
	}
	f, err := regex.Compile(s, anchor)
	if err != nil {
		in.fatal("%s", err.Error())
	}
	if in.compileTime == running {
		if len(in.fatab) >= maxFA {
			for k := range in.fatab {
				delete(in.fatab, k)
				break
			}
		}
		in.fatab[key] = f
	}
	return f
}

// setLocale follows setlocale(LC_CTYPE, ""): characters are UTF-8 only when the locale named by LC_ALL,
// LC_CTYPE or LANG (the first one set) is a UTF-8 locale this system has, which is C.UTF-8.
func setLocale() {
	name := ""
	for _, v := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if s := os.Getenv(v); s != "" {
			name = s
			break
		}
	}
	utf8 := name == "C.UTF-8" || name == "C.utf8"
	u8.MultiByte = utf8
	if utf8 {
		u8.MaxLen = 6
	} else {
		u8.MaxLen = 1
	}
}

// Main runs awk with the command line args (args[0] is the command name used in messages) and returns the
// exit status.
func Main(args []string) (status int) {
	// A write to a closed pipe kills the program unless SIGPIPE was ignored when it started; then the write
	// fails and the program reports it.
	if signal.Ignored(syscall.SIGPIPE) {
		signal.Ignore(syscall.SIGPIPE)
	}
	in := &Interp{
		cmdname:     args[0],
		compileTime: errorPrinting,
		srandSeed:   1,
		firsttime:   true,
		argno:       1,
		fatab:       map[faKey]*regex.FA{},
	}
	in.stdin = newInStream(os.Stdin, nil)
	in.stdinStream = &stream{in: in.stdin, file: os.Stdin, std: 0}
	in.stdoutStream = &stream{out: &outStream{w: os.Stdout}, file: os.Stdout, std: 1}
	in.stderrStream = &stream{out: &outStream{w: os.Stderr, unbuffered: true}, file: os.Stderr, std: 2}
	in.lex = &parser{in: in, lineno: 1}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalError); !ok {
				panic(r)
			}
			// exit(2): the C library flushes every stream
			in.flushStdout()
			in.flushAll()
			status = 2
		}
	}()
	return in.main(args)
}

func (in *Interp) main(argv []string) int {
	setLocale()
	if len(argv) == 1 {
		in.errout("usage: " + in.cmdname + " [-F fs | --csv] [-v var=value] [-f progfile | 'prog'] [file ...]\n")
		return 1
	}
	in.symtab = makeSymtab(1)
	var fs *string
	args := argv[1:]
	// getarg is the value of an option: the rest of the argument, or the next argument
	getarg := func(msg string) string {
		if len(args[0]) > 2 {
			return args[0][2:]
		}
		args = args[1:]
		if len(args) == 0 {
			in.fatal("%s", msg)
		}
		return args[0]
	}
options:
	for len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' {
		switch arg := args[0]; {
		case arg == "-version" || arg == "--version":
			in.stdoutStream.out.writeString("awk " + Version + "\n")
			in.flushStdout()
			return 0
		case arg == "--": // explicit end of options
			args = args[1:]
			break options
		case arg == "--csv":
			in.csv = true
			args = args[1:]
			continue
		}
		switch args[0][1] {
		case 's':
			if args[0] == "-safe" {
				in.safe = true
			}
		case 'f': // the next argument is a program file
			in.lex.pfiles = append(in.lex.pfiles, getarg("no program filename"))
		case 'F': // field separator; "t" is a tab
			v := getarg("no field separator")
			if v == "t" {
				v = "\t"
			}
			fs = &v
		case 'd': // debugging: only its version line is reproduced
			in.dbg = true
			in.stdoutStream.out.writeString("awk " + Version + "\n")
		case 'v': // -v a=1, done now
			vn := getarg("no variable name")
			if isclvar(vn) {
				in.setclvar(vn)
			} else {
				in.fatal("invalid -v option argument: %s", vn)
			}
		default:
			in.warning("unknown option %s ignored", args[0])
		}
		args = args[1:]
	}
	if in.csv && (fs != nil || in.symtab.lookup("FS") != nil) {
		in.warning("danger: don't set FS when --csv is in effect")
	}
	if len(in.lex.pfiles) == 0 { // no -f: the first argument is the program
		if len(args) == 0 {
			if in.dbg {
				in.flushStdout()
				return 0
			}
			in.fatal("no program given")
		}
		in.lex.prog = args[0]
		in.lex.useProg = true
		args = args[1:]
	}
	in.recinit()
	in.syminit()
	in.compileTime = compiling
	in.arginit(append([]string{in.cmdname}, args...))
	if !in.safe {
		in.envinit(os.Environ())
	}
	yyParse(in.lex)
	if fs != nil {
		in.fsloc.setSval(in.qstring(*fs, 0))
	}
	if in.errorflag == 0 {
		in.compileTime = running
		in.run(in.winner)
	} else {
		in.bracecheck()
	}
	in.flushStdout()
	in.flushAll()
	return in.errorflag
}
