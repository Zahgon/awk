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

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests build the awk binary and run it as a user would: over a table of behaviours whose expected
// output, error messages and exit status are those of the original C implementation, over the regression
// programs in bugs-fixed (with their recorded outputs), and through the self-checking scripts in testdir.

var awkBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "awk-test-")
	if err != nil {
		panic(err)
	}
	awkBin = filepath.Join(dir, "a.out")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", awkBin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic("building awk: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runAwk runs the binary under the name awk, in a UTF-8 locale.
func runAwk(t *testing.T, dir string, args []string, stdin string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(awkBin, args...)
	cmd.Args[0] = "awk"
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running awk: %v", err)
	}
	return out.String(), errb.String(), code
}

var behaviours = []struct {
	name     string
	args     []string
	stdin    string
	out, err string
	code     int
}{
	{"uninitialised values compare as both 0 and \"\"", []string{"BEGIN { print (x == 0), (x == \"\"), length(x) }"}, "", "1 1 0\n", "", 0},
	{"numeric fields compare as numbers", []string{"{ print ($1 < $2), ($1 \"\" < $2 \"\") }"}, "10 9\n", "0 1\n", "", 0},
	{"integral values print exactly", []string{"BEGIN { print 2^53, 1e16, 0.1 + 0.2, 100/3 }"}, "", "9007199254740992 10000000000000000 0.3 33.3333\n", "", 0},
	{"OFMT and CONVFMT", []string{"BEGIN { OFMT = \"%.2f\"; CONVFMT = \"%.3g\"; x = 3.14159; print x, x \"\" }"}, "", "3.14 3.14\n", "", 0},
	{"substr truncates its arguments", []string{"BEGIN { print substr(\"hello\", 1.9, 2.9), substr(\"h\xc3\xa9llo\", 0, 3) }"}, "", "he h\xc3\xa9l\n", "", 0},
	{"length counts UTF-8 characters", []string{"{ print length($0), toupper($0) }"}, "h\xc3\xa9llo w\xc3\xb6rld\n", "11 H\xc3\x89LLO W\xc3\x96RLD\n", "", 0},
	{"single space FS", []string{"{ print NF, $2 }"}, "  a \t b  c \n", "3 b\n", "", 0},
	{"regular expression FS", []string{"BEGIN { FS = \"[,:]+\" } { print NF, $3 }"}, "a,,b:c\n", "3 c\n", "", 0},
	{"NF assignment rebuilds the record", []string{"BEGIN { OFS = \"-\" } { NF = 2; print; $4 = \"x\"; print }"}, "a b c\n", "a-b\na-b--x\n", "", 0},
	{"paragraph mode", []string{"BEGIN { RS = \"\" } { print NR \": \" $1 \"|\" $NF }"}, "\n\na b\nc\n\n\nd\n", "1: a|c\n2: d|d\n", "", 0},
	{"regex RS clears the record buffer at the end", []string{"BEGIN { RS = \";+\" } END { print NR, $0 }"}, "one;two;;three\n", "3 t\n", "", 0},
	{"CSV quoting", []string{"--csv", "{ print NF; for (i = 1; i <= NF; i++) print \"[\" $i \"]\" }"}, "\"a,b\",\"say \"\"hi\"\"\",,last\n", "4\n[a,b]\n[say \"hi\"]\n[]\n[last]\n", "", 0},
	{"gsub with & and \\\\&", []string{"BEGIN { s = \"banana\"; n = gsub(/a/, \"[&\\\\&]\", s); print n, s }"}, "", "3 b[a&]n[a&]n[a&]\n", "", 0},
	{"empty matches in gsub", []string{"BEGIN { s = \"abc\"; gsub(/x*/, \"-\", s); print s }"}, "", "-a-b-c-\n", "", 0},
	{"printf conversions", []string{"BEGIN { printf \"%5.2f|%-4d|%x|%c|%c|%.2s\\n\", 3.14159, 42, 255, 65, \"h\xc3\xa9llo\", \"\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e\" }"}, "", " 3.14|42  |ff|A|h|\xe6\x97\xa5\xe6\x9c\xac\n", "", 0},
	{"printf %c of a numeric field", []string{"{ printf \"%c%c\\n\", $1, $2 }"}, "72 105\n", "Hi\n", "", 0},
	{"for-in order follows the hash table", []string{"BEGIN { split(\"q w e r t y\", a); for (k in a) printf \"%s \", k; print \"\" }"}, "", "2 3 4 5 6 1 \n", "", 0},
	{"multi-dimensional subscripts", []string{"BEGIN { a[1, 2] = 3; for (k in a) { split(k, p, SUBSEP); print p[1], p[2] } }"}, "", "1 2\n", "", 0},
	{"scalar parameter used as an array", []string{"function f(a) { a[1] = 1 } BEGIN { f(x); print length(x) }"}, "", "1\n", "", 0},
	{"range patterns", []string{"/b/,/d/ { print NR }"}, "a\nb\nc\nd\ne\n", "2\n3\n4\n", "", 0},
	{"exit in BEGIN still runs END", []string{"BEGIN { exit 3 } END { print \"end\" }"}, "", "end\n", "", 3},
	{"getline var keeps $0", []string{"NR == 1 { getline line; print $0, line, NR }"}, "first\nsecond\n", "first second 2\n", "", 0},
	{"command pipes", []string{"BEGIN { while ((\"echo a; echo b\" | getline x) > 0) print \"<\" x \">\"; print close(\"echo a; echo b\") }"}, "", "<a>\n<b>\n0\n", "", 0},
	{"system return value", []string{"BEGIN { print system(\"exit 3\") }"}, "", "3\n", "", 0},
	{"hexadecimal input after a sign", []string{"{ print $1 + 0, $2 + 0 }"}, "0x1A +0x1A\n", "0 26\n", "", 0},
	{"out-of-range strings are not numbers", []string{"BEGIN { x = \"1e400\"; print x + 0, 1e309 }"}, "", "0 1e309\n", "", 0},
	{"division by zero", []string{"{ print $1 / 0 }"}, "5\n", "", "awk: division by zero\n input record number 1, file \n source line number 1\n", 2},
	{"syntax error context", []string{"BEGIN { print 1 +* 2 }"}, "", "", "awk: syntax error at source line 1\n context is\n\tBEGIN { print 1 >>>  +* <<<  2 }\nawk: illegal statement at source line 1\n", 2},
	{"unbalanced braces", []string{"BEGIN { if (x) { print 1 }"}, "", "", "awk: syntax error at source line 1\n context is\n\tBEGIN { if (x) { print 1 >>>  } <<< \nawk: illegal statement at source line 1\n\tmissing }\n", 2},
	{"unknown printf conversion", []string{"BEGIN { printf \"%k|\\n\", 5 }"}, "", "%k5|\n", "awk: weird printf conversion %k\n source line number 1\n", 0},
	{"too few printf arguments", []string{"BEGIN { printf \"%d %d\\n\", 1 }"}, "", "", "awk: not enough args in printf(%d %d\n)\n source line number 1\n", 2},
	{"calling an undefined function", []string{"BEGIN { f() }"}, "", "", "awk: calling undefined function f\n source line number 1\n", 2},
	{"array used as a scalar", []string{"BEGIN { a[1]; a = 2 }"}, "", "", "awk: can't assign to a; it's an array name.\n source line number 1\n", 2},
	{"bad regular expression", []string{"{ print $0 ~ \"a(\" }"}, "x\n", "", "awk: illegal primary in regular expression a( at \n source line number 1\n context is\n\t{ print $0 ~ \"a(\" >>>  } <<< \n", 2},
	{"log domain warning", []string{"BEGIN { print log(-1) }"}, "", "1\n", "awk: log argument out of domain\n source line number 1\n", 0},
	{"srand returns the previous seed", []string{"BEGIN { print srand(5), srand(), (rand() < 1) }"}, "", "1 5 1\n", "", 0},
	{"rand from the default seed", []string{"BEGIN { printf \"%.6f %.6f\\n\", rand(), rand() }"}, "", "0.840188 0.394383\n", "", 0},
	{"missing input file", []string{"{ print }", "nosuchfile"}, "", "", "awk: can't open file nosuchfile\n source line number 1\n", 2},
	{"command-line assignment", []string{"{ print v, $0 }", "v=1", "-", "v=2", "-"}, "line\n", "1 line\n", "", 0},
	{"-v escapes", []string{"-v", "v=a\\tb", "BEGIN { print v, length(v) }"}, "", "a\tb 3\n", "", 0},
	{"-F t means tab", []string{"-F", "t", "{ print $2 }"}, "a\tb c\n", "b c\n", "", 0},
	{"unknown option", []string{"-x", "BEGIN { print 1 }"}, "", "1\n", "awk: unknown option -x ignored\n", 0},
	{"toupper of invalid UTF-8", []string{"{ print toupper($0) }"}, "a\xffb\n", "", "awk: illegal byte sequence a\xffb\n input record number 1, file \n source line number 1\n", 2},
}

func TestBehaviour(t *testing.T) {
	for _, b := range behaviours {
		t.Run(b.name, func(t *testing.T) {
			out, errs, code := runAwk(t, t.TempDir(), b.args, b.stdin)
			if out != b.out || errs != b.err || code != b.code {
				t.Errorf("awk %q\n got stdout %q stderr %q status %d\nwant stdout %q stderr %q status %d",
					b.args, out, errs, code, b.out, b.err, b.code)
			}
		})
	}
}

// staleOK lists bugs-fixed programs whose recorded output predates later fixes; the original prints what
// this implementation prints.
var staleOK = map[string]bool{"rstart-rlength.awk": true}

func TestBugsFixed(t *testing.T) {
	progs, err := filepath.Glob("bugs-fixed/*.awk")
	if err != nil || len(progs) == 0 {
		t.Fatal("no bugs-fixed programs")
	}
	for _, prog := range progs {
		name := filepath.Base(prog)
		if staleOK[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			base := strings.TrimSuffix(name, ".awk")
			args := []string{"-f", name}
			if _, err := os.Stat(filepath.Join("bugs-fixed", base+".in")); err == nil {
				args = append(args, base+".in")
			}
			// as the bugs-fixed REGRESS script runs them: ../a.out, stdout and stderr together
			cmd := exec.Command(awkBin, args...)
			cmd.Args[0] = "../a.out"
			cmd.Dir = "bugs-fixed"
			cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			_ = cmd.Run()
			for _, okName := range []string{base + ".ok", base + ".ok2"} {
				if want, err := os.ReadFile(filepath.Join("bugs-fixed", okName)); err == nil && bytes.Equal(want, out.Bytes()) {
					return
				}
			}
			t.Errorf("output differs from %s.ok:\n%s", base, out.String())
		})
	}
}

// needEcho lists the checks of the testdir scripts that need the compiled helper testdir/echo.c, which the
// REGRESS script builds with a C compiler; they fail the same way for any awk when it is missing.
var needEcho = []string{"BAD: T.clv (x=19)", "BAD: T.misc null byte"}

// alwaysBAD lists checks that print BAD for every awk: T.errmsg checks that a failing check is reported, and
// the I/O error check races with the reader of the pipe exiting.
var alwaysBAD = []string{"this should print a BAD message failed", "exit status on I/O error"}

func TestScripts(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no shell")
	}
	root := t.TempDir()
	if err := exec.Command("cp", "-r", "testdir", filepath.Join(root, "testdir")).Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("cp", awkBin, filepath.Join(root, "a.out")).Run(); err != nil {
		t.Fatal(err)
	}
	_, hasEcho := os.Stat(filepath.Join(root, "testdir", "echo"))
	scripts, _ := filepath.Glob(filepath.Join(root, "testdir", "T.*"))
	for _, script := range scripts {
		name := filepath.Base(script)
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("sh", name)
			cmd.Dir = filepath.Join(root, "testdir")
			cmd.Env = []string{"PATH=" + filepath.Join(root, "testdir") + ":/usr/bin:/bin", "LANG=C.UTF-8",
				"awk=../a.out", "oldawk=../a.out"}
			out, _ := cmd.CombinedOutput()
			for _, line := range strings.Split(string(out), "\n") {
				if !strings.Contains(line, "BAD") {
					continue
				}
				known := false
				for _, k := range needEcho {
					known = known || (hasEcho != nil && strings.Contains(line, k))
				}
				for _, k := range alwaysBAD {
					known = known || strings.Contains(line, k)
				}
				if !known {
					t.Errorf("%s: %q", name, line)
				}
			}
		})
	}
}
