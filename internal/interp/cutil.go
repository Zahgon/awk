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
	"github.com/onetrueawk/awk/internal/regex"
)

// The original is C: its strings end at the first NUL byte, its character classification is that of the C
// locale for single bytes, and its messages are printf formats. These helpers give the port the same
// behaviour.

// cString cuts s at its first NUL byte, where the original's string would end.
func cString(s string) string {
	if i := strings.IndexByte(s, 0); i >= 0 {
		return s[:i]
	}
	return s
}

// charValue is the value of a byte read through a plain C char, which is signed on some machines and unsigned
// on others; it decides how bytes above 0x7F hash.
func charValue(b byte) int {
	if regex.CharSigned {
		return int(int8(b))
	}
	return int(b)
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isUpper(c byte) bool  { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool  { return c >= 'a' && c <= 'z' }
func isAlpha(c byte) bool  { return isUpper(c) || isLower(c) }
func isAlnum(c byte) bool  { return isAlpha(c) || isDigit(c) }
func isXDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isSpace(c byte) bool  { return c == ' ' || (c >= '\t' && c <= '\r') }

func toLowerASCII(c byte) byte {
	if isUpper(c) {
		return c + 'a' - 'A'
	}
	return c
}

func signbit(f float64) bool { return math.Signbit(f) }

// atoi is C's atoi: leading blanks, an optional sign and the digits that follow.
func atoi(s string) int {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	n := int32(0)
	for ; i < len(s) && isDigit(s[i]); i++ {
		n = 10*n + int32(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return int(n)
}

// cfmt formats a message the way the original's printf-style error functions do; it knows the conversions
// those messages use (%s with an optional byte precision, %d, %c, %g, %o, %p and %%).
func cfmt(format string, args ...any) string {
	var b strings.Builder
	ai := 0
	next := func() any {
		if ai < len(args) {
			ai++
			return args[ai-1]
		}
		return nil
	}
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		j := i + 1
		for j < len(format) && strings.IndexByte("-+ #0123456789.", format[j]) >= 0 {
			j++
		}
		spec := format[i:j]
		for j < len(format) && (format[j] == 'z' || format[j] == 'l') {
			j++
		}
		if j >= len(format) {
			b.WriteString(format[i:])
			break
		}
		switch conv := format[j]; conv {
		case '%':
			b.WriteByte('%')
		case 's':
			s, _ := next().(string)
			b.WriteString(libc.SprintfString(spec+"s", s))
		case 'd', 'u':
			b.WriteString(libc.SprintfInt(spec+"jd", toInt64(next())))
		case 'o':
			b.WriteString(libc.SprintfUint(spec+"jo", uint64(toInt64(next()))))
		case 'c':
			b.WriteString(libc.SprintfChar(spec+"c", int(toInt64(next()))))
		case 'g', 'f', 'e':
			f, _ := next().(float64)
			b.WriteString(libc.SprintfFloat(spec+string(conv), f))
		case 'p':
			b.WriteString("0x" + strconv.FormatUint(uint64(ai), 16))
			next()
		default:
			b.WriteString(format[i : j+1])
		}
		i = j
	}
	return b.String()
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case byte:
		return int64(x)
	case rune:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}
