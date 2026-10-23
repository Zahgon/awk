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

package libc

import (
	"math"
	"strconv"
	"strings"
)

// spec is one printf conversion specification, parsed the way glibc parses it.
type spec struct {
	left, plus, space, alt, zero, group bool
	width                               int
	prec                                int // -1 when absent
	conv                                byte
	tail                                string // the text after the conversion character, printed as is
	bad                                 bool   // a width or precision too large for an int: printf fails
}

// parseSpec parses s, which starts with '%': flags, a decimal width, a precision ("." and decimal digits; a
// "." with no digits is precision 0), length modifiers (ignored here), then the conversion character.
func parseSpec(s string) spec {
	p := spec{prec: -1}
	i := 1
flags:
	for ; i < len(s); i++ {
		switch s[i] {
		case '-':
			p.left = true
		case '+':
			p.plus = true
		case ' ':
			p.space = true
		case '#':
			p.alt = true
		case '0':
			p.zero = true
		case '\'':
			p.group = true
		case 'I':
		default:
			break flags
		}
	}
	readInt := func() int {
		n := 0
		for ; i < len(s) && isDigit(s[i]); i++ {
			n = n*10 + int(s[i]-'0')
			if n > math.MaxInt32 {
				p.bad = true
				n = math.MaxInt32
			}
		}
		return n
	}
	p.width = readInt()
	if i < len(s) && s[i] == '.' {
		i++
		p.prec = readInt()
	}
	for i < len(s) && strings.IndexByte("hlLqjzZt", s[i]) >= 0 {
		i++
	}
	if i < len(s) {
		p.conv = s[i]
		p.tail = s[i+1:]
	}
	return p
}

// unknown is what glibc prints for a conversion character it does not know: the specification rebuilt from
// what it parsed (flags in a fixed order, length modifiers dropped).
func (p spec) unknown() string {
	var b strings.Builder
	b.WriteByte('%')
	if p.alt {
		b.WriteByte('#')
	}
	if p.group {
		b.WriteByte('\'')
	}
	if p.plus {
		b.WriteByte('+')
	} else if p.space {
		b.WriteByte(' ')
	}
	if p.left {
		b.WriteByte('-')
	} else if p.zero {
		b.WriteByte('0')
	}
	if p.width != 0 {
		b.WriteString(strconv.Itoa(p.width))
	}
	if p.prec >= 0 {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(p.prec))
	}
	b.WriteByte(p.conv)
	return b.String()
}

// format formats s, which starts with a conversion specification, for one argument: conv formats the argument
// for a known conversion character. A "%%" or an unknown conversion character does not use the argument, and the
// text after it is formatted again, so that a later conversion still gets it, as glibc's printf does. A width
// or precision too large for an int makes printf fail and print nothing.
func format(s, known string, conv func(spec) string) string {
	var b strings.Builder
	for used := false; s != ""; {
		i := strings.IndexByte(s, '%')
		if i < 0 || used {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		p := parseSpec(s[i:])
		switch {
		case p.bad:
			return ""
		case p.conv == 0:
			return b.String()
		case p.conv == '%':
			b.WriteByte('%')
		case strings.IndexByte(known, p.conv) < 0:
			b.WriteString(p.unknown())
		default:
			b.WriteString(conv(p))
			used = true
		}
		s = p.tail
	}
	return b.String()
}

// pad justifies body (made of sign or prefix, then digits) to the width: spaces on the left or right, or, for
// zero padding, zeros between the prefix and the digits.
func (p spec) pad(prefix, digits string, zeroOK bool) string {
	n := len(prefix) + len(digits)
	if n >= p.width {
		return prefix + digits
	}
	fill := p.width - n
	switch {
	case p.left:
		return prefix + digits + strings.Repeat(" ", fill)
	case p.zero && zeroOK:
		return prefix + strings.Repeat("0", fill) + digits
	}
	return strings.Repeat(" ", fill) + prefix + digits
}

// SprintfInt formats v as snprintf does for a signed conversion (d or i, given an intmax_t).
func SprintfInt(f string, v int64) string {
	return format(f, "di", func(p spec) string {
		mag := uint64(v)
		sign := ""
		switch {
		case v < 0:
			mag = -mag
			sign = "-"
		case p.plus:
			sign = "+"
		case p.space:
			sign = " "
		}
		return p.pad(sign, precDigits(strconv.FormatUint(mag, 10), mag, p.prec), p.prec < 0)
	})
}

// SprintfUint formats v as snprintf does for an unsigned conversion (o, x, X or u, given a uintmax_t).
func SprintfUint(f string, v uint64) string {
	return format(f, "ouxX", func(p spec) string {
		var digits, prefix string
		switch p.conv {
		case 'o':
			digits = precDigits(strconv.FormatUint(v, 8), v, p.prec)
			if p.alt && (digits == "" || digits[0] != '0') {
				digits = "0" + digits
			}
		case 'x', 'X':
			digits = precDigits(strconv.FormatUint(v, 16), v, p.prec)
			if p.alt && v != 0 {
				prefix = "0x"
			}
			if p.conv == 'X' {
				digits = strings.ToUpper(digits)
				prefix = strings.ToUpper(prefix)
			}
		default:
			digits = precDigits(strconv.FormatUint(v, 10), v, p.prec)
		}
		return p.pad(prefix, digits, p.prec < 0)
	})
}

// precDigits applies an integer precision: the minimum number of digits, and no digits at all for zero
// with precision 0.
func precDigits(digits string, v uint64, prec int) string {
	if prec == 0 && v == 0 {
		return ""
	}
	if len(digits) < prec {
		return strings.Repeat("0", prec-len(digits)) + digits
	}
	return digits
}

// SprintfChar formats c, converted to an unsigned char, as snprintf does for %c.
func SprintfChar(f string, c int) string {
	return format(f, "c", func(p spec) string {
		return p.pad("", string([]byte{byte(c)}), false)
	})
}

// SprintfString formats s as snprintf does for %s: the precision limits the number of bytes.
func SprintfString(f string, s string) string {
	return format(f, "s", func(p spec) string {
		if p.prec >= 0 && p.prec < len(s) {
			s = s[:p.prec]
		}
		return p.pad("", s, false)
	})
}

// SprintfFloat formats v as snprintf does for a floating-point conversion (a, A, e, E, f, F, g or G).
func SprintfFloat(f string, v float64) string {
	return format(f, "aAeEfFgG", func(p spec) string {
		sign := ""
		switch {
		case math.Signbit(v):
			sign = "-"
		case p.plus:
			sign = "+"
		case p.space:
			sign = " "
		}
		upper := p.conv >= 'A' && p.conv <= 'Z'
		if math.IsInf(v, 0) || math.IsNaN(v) {
			body := "inf"
			if math.IsNaN(v) {
				body = "nan"
			}
			if upper {
				body = strings.ToUpper(body)
			}
			return p.pad(sign, body, false)
		}
		a := math.Abs(v)
		var body string
		prefix := sign
		switch p.conv {
		case 'f', 'F':
			body = fixed(a, p.precOr(6), p.alt)
		case 'e', 'E':
			body = expo(a, p.precOr(6), p.alt)
		case 'g', 'G':
			body = general(a, p.precOr(6), p.alt)
		default:
			prefix += "0x"
			body = hexFloat(a, p.prec, p.alt)
		}
		if upper {
			body = strings.ToUpper(body)
			prefix = strings.ToUpper(prefix)
		}
		return p.pad(prefix, body, true)
	})
}

func (p spec) precOr(def int) int {
	if p.prec < 0 {
		return def
	}
	return p.prec
}

// fixed is %f of a non-negative finite v: exact decimal digits, rounded to nearest with ties to even.
func fixed(v float64, prec int, alt bool) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if alt && prec == 0 {
		s += "."
	}
	return s
}

// expo is %e: d.ddde+XX with at least two exponent digits.
func expo(v float64, prec int, alt bool) string {
	s := strconv.FormatFloat(v, 'e', prec, 64)
	mant, exp, _ := strings.Cut(s, "e")
	if alt && prec == 0 {
		mant += "."
	}
	return mant + "e" + exp
}

// general is %g: %e or %f depending on the exponent, trailing zeros removed unless '#' is given.
func general(v float64, prec int, alt bool) string {
	if prec == 0 {
		prec = 1
	}
	x := 0
	if v != 0 {
		e := strconv.FormatFloat(v, 'e', prec-1, 64)
		x, _ = strconv.Atoi(e[strings.IndexByte(e, 'e')+1:])
	}
	var s string
	if x < -4 || x >= prec {
		s = expo(v, prec-1, alt)
		mant, exp, _ := strings.Cut(s, "e")
		if !alt {
			mant = trimZeros(mant)
		}
		return mant + "e" + exp
	}
	s = fixed(v, prec-1-x, alt)
	if alt {
		if !strings.Contains(s, ".") {
			s += "."
		}
		return s
	}
	return trimZeros(s)
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// hexFloat is %a of a non-negative finite v without the "0x": a leading 1 (0 for zero and subnormals), the
// fraction in hex and a binary exponent. With a precision the fraction is rounded to nearest, ties to even,
// and a carry out of the leading digit shows as a leading 2.
func hexFloat(v float64, prec int, alt bool) string {
	bits := math.Float64bits(v)
	frac := bits & (1<<52 - 1)
	bexp := int((bits >> 52) & 0x7FF)
	lead := uint64(1)
	exp := bexp - 1023
	switch {
	case v == 0:
		lead, exp = 0, 0
	case bexp == 0:
		lead, exp = 0, -1022
	}
	digits := 13
	if prec < 0 {
		for digits > 0 && frac&0xF == 0 {
			frac >>= 4
			digits--
		}
	} else if prec < digits {
		drop := uint(4 * (digits - prec))
		rem := frac & (1<<drop - 1)
		frac >>= drop
		half := uint64(1) << (drop - 1)
		if rem > half || (rem == half && (frac&1 == 1 || (prec == 0 && lead&1 == 1))) {
			frac++
			if prec == 0 || frac>>uint(4*prec) != 0 {
				frac &= 1<<uint(4*prec) - 1
				lead++
			}
		}
		digits = prec
	}
	var b strings.Builder
	b.WriteString(strconv.FormatUint(lead, 16))
	fs := ""
	if digits > 0 {
		fs = strconv.FormatUint(frac, 16)
		fs = strings.Repeat("0", digits-len(fs)) + fs
	}
	if prec > digits {
		fs += strings.Repeat("0", prec-digits)
	}
	if fs != "" || alt {
		b.WriteByte('.')
		b.WriteString(fs)
	}
	b.WriteByte('p')
	if exp >= 0 {
		b.WriteByte('+')
	}
	b.WriteString(strconv.Itoa(exp))
	return b.String()
}
