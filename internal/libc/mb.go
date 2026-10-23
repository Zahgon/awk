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

// Package libc reproduces the parts of the GNU C library that the original awk leans on and whose exact
// behaviour shows in awk's output: strtod's number syntax and range errors, printf's conversions (including
// what it prints for a malformed conversion), the random()/srandom() generator, and the C.UTF-8 locale's
// multibyte conversions and case mapping. Everything here is written from the documented and observed
// behaviour of glibc, not from its source.
package libc

import "unicode"

// ToUpper maps r as towupper does in the C.UTF-8 locale. Its table is the Unicode simple case mapping, the
// same one Go's unicode package carries; code points outside Unicode map to themselves.
func ToUpper(r rune) rune {
	return unicode.ToUpper(r)
}

// ToLower maps r as towlower does in the C.UTF-8 locale.
func ToLower(r rune) rune {
	return unicode.ToLower(r)
}

// DecodeMB decodes the multibyte character at the start of s like mbtowc in the C.UTF-8 locale. n is the
// number of bytes used, 0 for an empty string or a leading NUL, and -1 for a byte sequence that is not a
// character: a stray continuation byte, an overlong form, a surrogate, or a sequence cut short (mbtowc,
// unlike mbrtowc, has no separate "incomplete" result). The old 5- and 6-byte forms up to 0x7FFFFFFF are
// accepted, as glibc accepts them.
func DecodeMB(s string) (r rune, n int) {
	if len(s) == 0 || s[0] == 0 {
		return 0, 0
	}
	c := s[0]
	var size int
	var min uint32
	var v uint32
	switch {
	case c < 0x80:
		return rune(c), 1
	case c < 0xC2:
		return 0, -1 // continuation byte, or the start of an overlong 2-byte form
	case c < 0xE0:
		size, min, v = 2, 0x80, uint32(c&0x1F)
	case c < 0xF0:
		size, min, v = 3, 0x800, uint32(c&0x0F)
	case c < 0xF8:
		size, min, v = 4, 0x10000, uint32(c&0x07)
	case c < 0xFC:
		size, min, v = 5, 0x200000, uint32(c&0x03)
	case c < 0xFE:
		size, min, v = 6, 0x4000000, uint32(c&0x01)
	default:
		return 0, -1
	}
	if len(s) < size {
		return 0, -1
	}
	for i := 1; i < size; i++ {
		if s[i]&0xC0 != 0x80 {
			return 0, -1
		}
		v = v<<6 | uint32(s[i]&0x3F)
	}
	if v < min || (v >= 0xD800 && v <= 0xDFFF) {
		return 0, -1
	}
	return rune(v), size
}

// EncodeMB encodes r like wctomb in the C.UTF-8 locale: surrogates and negative values are rejected, and
// values above U+10FFFF still get the old 4- to 6-byte forms.
func EncodeMB(r rune) (string, bool) {
	v := uint32(r)
	switch {
	case r < 0 || (v >= 0xD800 && v <= 0xDFFF):
		return "", false
	case v < 0x80:
		return string([]byte{byte(v)}), true
	}
	var size int
	var lead byte
	switch {
	case v < 0x800:
		size, lead = 2, 0xC0
	case v < 0x10000:
		size, lead = 3, 0xE0
	case v < 0x200000:
		size, lead = 4, 0xF0
	case v < 0x4000000:
		size, lead = 5, 0xF8
	default:
		size, lead = 6, 0xFC
	}
	b := make([]byte, size)
	for i := size - 1; i > 0; i-- {
		b[i] = byte(0x80 | v&0x3F)
		v >>= 6
	}
	b[0] = lead | byte(v)
	return string(b), true
}
