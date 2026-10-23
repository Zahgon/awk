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
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// eof is what getc returns at the end of input.
const eof = -1

// An inStream reads bytes like a stdio input stream: buffered, with characters pushed back by ungetc read
// again first, and with a sticky end-of-file.
type inStream struct {
	r      io.Reader
	closer io.Closer
	buf    []byte
	pos    int
	push   []byte
	atEOF  bool
}

func newInStream(r io.Reader, c io.Closer) *inStream {
	return &inStream{r: r, closer: c}
}

func openInFile(name string) (*inStream, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return newInStream(f, f), nil
}

// Getc returns the next byte, or eof.
func (s *inStream) Getc() int {
	if n := len(s.push); n > 0 {
		c := s.push[n-1]
		s.push = s.push[:n-1]
		return int(c)
	}
	if s.pos < len(s.buf) {
		c := s.buf[s.pos]
		s.pos++
		return int(c)
	}
	if s.atEOF {
		return eof
	}
	if s.buf == nil {
		s.buf = make([]byte, 0, 4096)
	}
	for {
		n, err := s.r.Read(s.buf[:cap(s.buf)])
		if n > 0 {
			s.buf = s.buf[:n]
			s.pos = 1
			return int(s.buf[0])
		}
		if err != nil {
			s.buf = s.buf[:0]
			s.pos = 0
			s.atEOF = true
			return eof
		}
	}
}

// Ungetc pushes a byte back.
func (s *inStream) Ungetc(c byte) {
	s.push = append(s.push, c)
}

func (s *inStream) close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

// stdioBufSize is the buffer size of a stdio stream on a pipe or file.
const stdioBufSize = 4096

// An outStream writes like a stdio output stream: unbuffered (standard error) or fully buffered, with the
// buffer handed to write(2) the way the C library does it, so that output interleaves with child processes
// writing to the same file exactly as the original's does.
type outStream struct {
	w          io.Writer
	buf        []byte
	unbuffered bool
	err        bool // the error indicator, as ferror reports it
}

func (o *outStream) writeRaw(p []byte) {
	if len(p) == 0 {
		return
	}
	if _, err := o.w.Write(p); err != nil {
		o.err = true
	}
}

// write appends p: what fits goes into the buffer; when the buffer is full it is written, whole blocks of
// the rest go straight out and the remainder is buffered.
func (o *outStream) write(p []byte) {
	if o.unbuffered {
		o.writeRaw(p)
		return
	}
	if o.buf == nil {
		o.buf = make([]byte, 0, stdioBufSize)
	}
	room := cap(o.buf) - len(o.buf)
	if len(p) <= room {
		o.buf = append(o.buf, p...)
		return
	}
	o.buf = append(o.buf, p[:room]...)
	p = p[room:]
	o.flush()
	direct := len(p) - len(p)%stdioBufSize
	o.writeRaw(p[:direct])
	o.buf = append(o.buf, p[direct:]...)
}

func (o *outStream) writeString(s string) { o.write([]byte(s)) }

func (o *outStream) flush() {
	if len(o.buf) > 0 {
		o.writeRaw(o.buf)
		o.buf = o.buf[:0]
	}
}

// A stream is an entry's open file: a stdio stream on a file, a pipe to or from a command, or one of the
// standard streams.
type stream struct {
	in   *inStream
	out  *outStream
	file *os.File  // the file or our end of the pipe
	cmd  *exec.Cmd // the command of a pipe
	std  int       // 0, 1 or 2 for the standard streams, -1 otherwise
}

// A fileEntry is a slot of the table of open files, named as the program names them.
type fileEntry struct {
	fp    *stream
	fname string
	mode  int // LT, GT ('w' and 'a'), '|' or LE (input pipe)
}

// fopenMax is the number of slots the file table starts with and grows by.
const fopenMax = 16

// stdinit sets up the file table with the standard streams.
func (in *Interp) stdinit() {
	in.files = make([]*fileEntry, fopenMax)
	in.files[0] = &fileEntry{fp: in.stdinStream, fname: "/dev/stdin", mode: LT}
	in.files[1] = &fileEntry{fp: in.stdoutStream, fname: "/dev/stdout", mode: GT}
	in.files[2] = &fileEntry{fp: in.stderrStream, fname: "/dev/stderr", mode: GT}
}

// openfile finds or opens the stream named us for mode a (LT, GT, APPEND, '|' or LE; FFLUSH only finds).
// newflag tells whether it was opened now. It returns nil when the file cannot be opened.
func (in *Interp) openfile(a int, us string) (fp *stream, newflag bool) {
	if us == "" {
		in.fatal("null file name in print or getline")
	}
	for _, f := range in.files {
		if f != nil && f.fname == us && (a == f.mode || (a == APPEND && f.mode == GT) || a == fFLUSHMode) {
			return f.fp, false
		}
	}
	if a == fFLUSHMode { // not open, so don't create it
		return nil, false
	}
	i := 0
	for i < len(in.files) && in.files[i] != nil {
		i++
	}
	if i >= len(in.files) {
		in.files = append(in.files, make([]*fileEntry, fopenMax)...)
	}
	in.flushStdout() // force a semblance of order

	// don't try to read or write a directory
	if a == LT || a == GT || a == APPEND {
		if st, err := os.Stat(us); err == nil && st.IsDir() {
			return nil, false
		}
	}
	m := a
	switch a {
	case GT:
		fp = openOut(us, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	case APPEND:
		fp = openOut(us, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
		m = GT // so > and >> can be mixed
	case '|': // output pipe
		fp = in.popen(us, false)
	case LE: // input pipe
		fp = in.popen(us, true)
	case LT: // getline <file; "-" is standard input
		if us == "-" {
			fp = in.stdinStream
		} else if f, err := os.Open(us); err == nil {
			fp = &stream{in: newInStream(f, f), file: f, std: -1}
		}
	default:
		in.fatal("illegal redirection %d", a)
	}
	if fp == nil {
		return nil, false
	}
	in.files[i] = &fileEntry{fp: fp, fname: us, mode: m}
	return fp, true
}

func openOut(name string, flag int) *stream {
	f, err := os.OpenFile(name, flag, 0o666)
	if err != nil {
		return nil
	}
	return &stream{out: &outStream{w: f}, file: f, std: -1}
}

// popen runs cmd with the shell, with a pipe to its standard input or from its standard output; the other
// standard streams are the interpreter's own.
func (in *Interp) popen(cmd string, read bool) *stream {
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	c := shellCommand(cmd)
	c.Stderr = os.Stderr
	if read {
		c.Stdin = os.Stdin
		c.Stdout = w
	} else {
		c.Stdin = r
		c.Stdout = os.Stdout
	}
	if err := c.Start(); err != nil {
		r.Close()
		w.Close()
		return nil
	}
	if read {
		w.Close()
		return &stream{in: newInStream(r, nil), file: r, cmd: c, std: -1}
	}
	r.Close()
	return &stream{out: &outStream{w: w}, file: w, cmd: c, std: -1}
}

// shellCommand runs cmd as the C library's popen and system do: /bin/sh -c cmd, with "sh" as the shell's name
// (which the shell uses in its own error messages).
func shellCommand(cmd string) *exec.Cmd {
	c := exec.Command("/bin/sh", "-c", cmd)
	c.Args[0] = "sh"
	return c
}

// pclose closes our end of the pipe and waits for the command; like the C library it fails only when the
// command cannot be waited for.
func pclose(fp *stream) bool {
	fp.file.Close()
	if err := fp.cmd.Wait(); err != nil {
		var ee *exec.ExitError
		return errors.As(err, &ee)
	}
	return true
}

// closeStream closes a file or pipe; it reports success.
func (in *Interp) closeStream(fp *stream, pipe bool) bool {
	if fp.out != nil {
		fp.out.flush()
	}
	if pipe {
		return pclose(fp)
	}
	return fp.file.Close() == nil
}

// reopenNull points a standard stream at /dev/null, as freopen does when the program closes it: the
// descriptor itself is replaced, so commands started later see /dev/null too.
func (in *Interp) reopenNull(fp *stream) bool {
	if fp.out != nil {
		fp.out.flush()
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer null.Close()
	if err := syscall.Dup3(int(null.Fd()), fp.std, 0); err != nil {
		return false
	}
	switch fp.std {
	case 0:
		fp.in.buf, fp.in.pos, fp.in.push, fp.in.atEOF = nil, 0, nil, false
	default:
		fp.out.err = false
	}
	return true
}

// filename names a stream for error messages.
func (in *Interp) filename(fp *stream) string {
	for _, f := range in.files {
		if f != nil && f.fp == fp {
			return f.fname
		}
	}
	return "???"
}

// closefile is close(name): it closes the first open file or pipe of that name.
func (in *Interp) closefile(name string) float64 {
	failed := true
	for i, f := range in.files {
		if f == nil || f.fname != name {
			continue
		}
		if (f.mode == GT || f.mode == '|') && f.fp.out != nil {
			f.fp.out.flush()
		}
		if f.fp.out != nil && f.fp.out.err {
			if (f.mode == GT && f.fp != in.stderrStream) || f.mode == '|' {
				in.fatal("write error on %s", f.fname)
			} else {
				in.warning("i/o error occurred on %s", f.fname)
			}
		}
		if f.fp.std >= 0 {
			failed = !in.reopenNull(f.fp)
		} else {
			failed = !in.closeStream(f.fp, f.mode == '|' || f.mode == LE)
		}
		if failed {
			in.warning("i/o error occurred closing %s", f.fname)
		}
		in.files[i] = nil
		break
	}
	if failed {
		return -1
	}
	return 0
}

// closeall flushes and closes everything at the end of the program, waiting for the commands of pipes.
func (in *Interp) closeall() {
	for _, f := range in.files {
		if f == nil {
			continue
		}
		if (f.mode == GT || f.mode == '|') && f.fp.out != nil {
			f.fp.out.flush()
		}
		if f.fp.out != nil && f.fp.out.err {
			if (f.mode == GT && f.fp != in.stderrStream) || f.mode == '|' {
				in.fatal("write error on %s", f.fname)
			} else {
				in.warning("i/o error occurred on %s", f.fname)
			}
		}
		if f.fp.std >= 0 {
			continue
		}
		if !in.closeStream(f.fp, f.mode == '|' || f.mode == LE) {
			in.warning("i/o error occurred while closing %s", f.fname)
		}
	}
}

// flushAll flushes every open output stream (fflush() and fflush("")).
func (in *Interp) flushAll() {
	for _, f := range in.files {
		if f != nil && f.fp.out != nil {
			f.fp.out.flush()
		}
	}
}

// flushStdout flushes standard output, as the original does before anything that might interleave with it.
func (in *Interp) flushStdout() {
	in.stdoutStream.out.flush()
}

// errout writes an error message to standard error.
func (in *Interp) errout(s string) {
	in.stderrStream.out.writeString(s)
}

// system runs cmd with the shell and returns its exit status, or 256 plus the signal that killed it (plus 256
// more with a core dump), or -1 when it could not be run.
func (in *Interp) system(cmd string) float64 {
	in.flushStdout()
	c := shellCommand(cmd)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := c.Run()
	if c.ProcessState == nil {
		if err != nil {
			return -1
		}
		return 0
	}
	ws, ok := c.ProcessState.Sys().(syscall.WaitStatus)
	switch {
	case !ok:
		return 0
	case ws.Exited():
		return float64(ws.ExitStatus())
	case ws.Signaled():
		st := int(ws.Signal()) + 256
		if ws.CoreDump() {
			st += 256
		}
		return float64(st)
	}
	return 0
}
