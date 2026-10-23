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
	"strings"

	"github.com/onetrueawk/awk/internal/libc"
	"github.com/onetrueawk/awk/internal/u8"
)

// format expands a printf format with the argument list a (format() of run.c). Each conversion is handed to
// the C library's formatting with the size modifiers removed and '*' replaced by the argument's value;
// integer conversions use the intmax_t forms. Conversions the original does not know are printed as written,
// followed by the argument. Unused arguments are still evaluated.
func (in *Interp) format(s string, a *Node) []byte {
	os := s
	var buf []byte
	for i := 0; i < len(s); {
		if s[i] != '%' {
			buf = append(buf, s[i])
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			buf = append(buf, '%')
			i += 2
			continue
		}
		fmtwd := atoi(s[i+1:])
		if fmtwd < 0 {
			fmtwd = -fmtwd
		}
		// collect the conversion: up to and including the first letter, dropping size modifiers
		var spec []byte
		conv := byte(0)
		for ; i < len(s); i++ {
			c := s[i]
			if strings.IndexByte("hjLlqtz", c) >= 0 {
				continue
			}
			spec = append(spec, c)
			if isAlpha(c) {
				conv = c
				break
			}
			if c == '$' {
				in.fatal("'$' not permitted in awk formats")
			}
			if c == '*' {
				if a == nil {
					in.fatal("not enough args in printf(%s)", os)
				}
				x := in.execute(a)
				a = a.next
				fmtwd = f2i32(in.getfval(x))
				spec = append(spec[:len(spec)-1], itoa(fmtwd)...)
				if fmtwd < 0 {
					fmtwd = -fmtwd
				}
			}
		}
		var flag byte
		switch conv {
		case 'a', 'A':
			flag = conv
		case 'f', 'e', 'g', 'E', 'G':
			flag = 'f'
		case 'd', 'i', 'o', 'x', 'X', 'u':
			if conv == 'd' || conv == 'i' {
				flag = 'd'
			} else {
				flag = 'u'
			}
			spec = append(spec[:len(spec)-1], 'j', conv)
		case 's':
			flag = 's'
		case 'c':
			flag = 'c'
		default:
			in.warning("weird printf conversion %s", string(spec))
			flag = '?'
		}
		if a == nil {
			in.fatal("not enough args in printf(%s)", os)
		}
		x := in.execute(a)
		a = a.next
		f := string(spec)
		var out string
		switch flag {
		case '?': // unknown, so dump it too
			out = f + in.getsval(x)
		case 'a', 'A', 'f':
			out = libc.SprintfFloat(f, in.getfval(x))
		case 'd':
			out = libc.SprintfInt(f, int64(in.getfval(x)))
		case 'u':
			out = libc.SprintfUint(f, f2u64(in.getfval(x)))
		case 's':
			t := in.getsval(x)
			if !u8.HasMulti(t) || f == "%s" {
				out = libc.SprintfString(f, t)
			} else {
				out = padUTF8(f, t, u8.Strlen(t))
			}
		case 'c':
			if x.isNum() {
				charval := f2i32(in.getfval(x))
				switch {
				case charval == 0: // an explicit NUL byte; the width is ignored
					buf = append(buf, 0)
					i++
					continue
				case charval < 128 || !u8.MultiByte:
					out = libc.SprintfChar(f, charval)
				default: // a code point: its UTF-8 bytes, or U+FFFD when it is not one
					t, ok := u8.Encode(charval)
					if !ok {
						t = "�"
					}
					out = padUTF8(f, t, 1)
				}
				break
			}
			t := in.getsval(x)
			if u8.NextLen(t) < 2 { // not UTF-8
				var c int
				if t != "" {
					c = charValue(t[0])
				}
				out = libc.SprintfChar(f, c)
			} else {
				out = padUTF8(f, t[:u8.NextLen(t)], 1)
			}
		}
		buf = append(buf, cString(out)...)
		i++
	}
	for ; a != nil; a = a.next { // evaluate any remaining arguments
		in.execute(a)
	}
	return buf
}

// padUTF8 formats a string with UTF-8 characters for "%-w.ps", counting characters: only the - flag, a width
// and a precision are understood (0 and + are skipped), and at most limit characters are printed.
func padUTF8(f, t string, limit int) string {
	p := f[1:]
	ljust := false
	if strings.HasPrefix(p, "-") {
		ljust = true
		p = p[1:]
	}
	if strings.HasPrefix(p, "0") {
		p = p[1:]
		p = strings.TrimPrefix(p, "+")
	}
	if strings.HasPrefix(p, "+") {
		p = p[1:]
		p = strings.TrimPrefix(p, "0")
	}
	wid := 0
	prec := len(t)
	if p != "" && isDigit(p[0]) {
		wid, p = strtol(p)
	}
	if strings.HasPrefix(p, ".") {
		prec, _ = strtol(p[1:])
	}
	if prec > limit {
		prec = limit
	}
	pad := 0
	if wid > prec {
		pad = wid - prec
	}
	body := t[:u8.Char2Byte(t, prec)]
	if limit == 1 {
		body = t // %c: the whole character
	}
	if ljust {
		return body + strings.Repeat(" ", pad)
	}
	return strings.Repeat(" ", pad) + body
}

// strtol reads a decimal number with an optional sign at the start of s, as strtol(s, &end, 10) does.
func strtol(s string) (int, string) {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i
	n := 0
	for i < len(s) && isDigit(s[i]) {
		n = 10*n + int(s[i]-'0')
		i++
	}
	if i == start {
		return 0, s
	}
	if neg {
		n = -n
	}
	return n, s[i:]
}

func itoa(n int) string { return libc.SprintfInt("%jd", int64(n)) }

// sprintfNumber formats a number with OFMT or CONVFMT, a printf format for one double.
func sprintfNumber(format string, f float64) string {
	var b strings.Builder
	used := false
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		j := i + 1
		for j < len(format) && strings.IndexByte("-+ #'I0123456789.*", format[j]) >= 0 {
			j++
		}
		for j < len(format) && strings.IndexByte("hlLqjzt", format[j]) >= 0 {
			j++
		}
		if j >= len(format) {
			b.WriteString(format[i:])
			break
		}
		spec := format[i : j+1]
		if format[j] == '%' && j == i+1 {
			b.WriteByte('%')
		} else if !used {
			b.WriteString(libc.SprintfFloat(spec, f))
			used = true
		} else {
			b.WriteString(libc.SprintfFloat(spec, 0))
		}
		i = j
	}
	return b.String()
}
