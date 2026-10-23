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
	"math/big"
	"strconv"
	"strings"
)

// Strtod converts the longest prefix of s that strtod accepts in the C locale: optional white space, an
// optional sign, then a decimal number with an optional exponent, a hexadecimal number ("0x", hex digits with
// an optional point, an optional binary exponent "p"), "inf", "infinity", "nan" or "nan(chars)", letters in
// any case. n is the number of bytes used, 0 when there is no number (and v is then 0). erange is true where
// strtod sets ERANGE: the value overflows to an infinity, or it is so small that the result is zero or
// subnormal and not exact.
func Strtod(s string) (v float64, n int, erange bool) {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	rest := s[i:]
	switch {
	case hasPrefixFold(rest, "infinity"):
		return signed(math.Inf(1), neg), i + 8, false
	case hasPrefixFold(rest, "inf"):
		return signed(math.Inf(1), neg), i + 3, false
	case hasPrefixFold(rest, "nan"):
		j := i + 3
		bits := uint64(0x7FF8000000000000)
		if j < len(s) && s[j] == '(' {
			k := j + 1
			for k < len(s) && (isAlnum(s[k]) || s[k] == '_') {
				k++
			}
			if k < len(s) && s[k] == ')' {
				// A payload that is a whole number (decimal, octal or 0x hex) goes into the low
				// mantissa bits of the quiet NaN.
				payload := s[j+1 : k]
				if u, err := strconv.ParseUint(payload, 0, 64); err == nil && cNumber(payload) {
					bits |= u & (1<<51 - 1)
				}
				j = k + 1
			}
		}
		return signed(math.Float64frombits(bits), neg), j, false
	case len(rest) > 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'X') &&
		(isHex(rest[2]) || (rest[2] == '.' && len(rest) > 3 && isHex(rest[3]))):
		v, used, erange := parseHex(rest[2:])
		return signed(v, neg), i + 2 + used, erange
	}
	v, used, erange := parseDecimal(rest)
	if used == 0 {
		return 0, 0, false
	}
	return signed(v, neg), i + used, erange
}

// parseDecimal converts the decimal number at the start of s (after the sign).
func parseDecimal(s string) (float64, int, bool) {
	i := 0
	digits := 0
	nonzero := false
	for i < len(s) && isDigit(s[i]) {
		nonzero = nonzero || s[i] != '0'
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			nonzero = nonzero || s[i] != '0'
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, 0, false
	}
	mant := s[:i]
	exp := ""
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			exp = s[i:j]
			i = j
		}
	}
	// strconv rounds correctly for any number of digits and saturates huge exponents.
	v, err := strconv.ParseFloat(mant+exp, 64)
	if err != nil && !math.IsInf(v, 0) && v != 0 {
		return 0, 0, false
	}
	return v, i, rangeError(v, nonzero, func() bool {
		r, _, _ := big.ParseFloat(mant+exp, 10, 2000, big.ToNearestEven)
		return r != nil && r.Cmp(new(big.Float).SetFloat64(v)) == 0
	})
}

// parseHex converts the hexadecimal mantissa (after "0x") and the binary exponent at the start of s.
func parseHex(s string) (float64, int, bool) {
	i := 0
	var mant strings.Builder
	nonzero := false
	point := -1
	for i < len(s) && (isHex(s[i]) || (s[i] == '.' && point < 0)) {
		if s[i] == '.' {
			point = mant.Len()
		} else {
			nonzero = nonzero || s[i] != '0'
			mant.WriteByte(s[i])
		}
		i++
	}
	exp := int64(0)
	if i < len(s) && (s[i] == 'p' || s[i] == 'P') {
		j := i + 1
		eneg := false
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			eneg = s[j] == '-'
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			for j < len(s) && isDigit(s[j]) {
				if exp < 1<<40 {
					exp = exp*10 + int64(s[j]-'0')
				}
				j++
			}
			if eneg {
				exp = -exp
			}
			i = j
		}
	}
	if !nonzero {
		return 0, i, false
	}
	digits := mant.String()
	if point >= 0 {
		exp -= 4 * int64(len(digits)-point)
	}
	// Leading zeros carry no value; bound the exponent so that big.Float cannot overflow its own range,
	// keeping it far beyond what a float64 can hold either way.
	digits = strings.TrimLeft(digits, "0")
	top := exp + 4*int64(len(digits))
	switch {
	case top > 5000:
		return math.Inf(1), i, true
	case top < -5000:
		return 0, i, true
	}
	m, _ := new(big.Int).SetString(digits, 16)
	f := new(big.Float).SetPrec(uint(4*len(digits) + 8)).SetInt(m)
	f.SetMantExp(f, int(exp))
	v, _ := f.Float64()
	return v, i, rangeError(v, true, func() bool {
		return f.Cmp(new(big.Float).SetFloat64(v)) == 0
	})
}

// rangeError reports whether strtod would set ERANGE for the converted value v of a number whose digits were
// not all zero: overflow, or an underflow to zero or a subnormal that lost precision.
func rangeError(v float64, nonzero bool, exact func() bool) bool {
	if math.IsInf(v, 0) {
		return true
	}
	if !nonzero || math.Abs(v) >= 0x1p-1022 {
		return false
	}
	return v == 0 || !exact()
}

// cNumber rejects the integer spellings strconv accepts but strtoull does not: digit separators and the 0b
// and 0o prefixes.
func cNumber(s string) bool {
	if strings.Contains(s, "_") {
		return false
	}
	return len(s) < 2 || s[0] != '0' || strings.IndexByte("bBoO", s[1]) < 0
}

func signed(v float64, neg bool) float64 {
	if neg {
		return -v
	}
	return v
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func isSpace(c byte) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isAlnum(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
