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

// Package u8 holds the UTF-8 helpers of awk's run.c: awk strings are byte strings that may hold UTF-8, random
// 8-bit bytes (Latin-1, say) or both, and the functions that count characters (length, index, substr, match,
// printf widths) step over them one "character" at a time, where a character is a complete UTF-8 sequence of
// 2 to 4 bytes or else a single byte. The checks are deliberately as loose as the original's: overlong forms,
// surrogates and values above U+10FFFF all count as valid sequences.
package u8

// MultiByte is false when the character type locale is single-byte (the C locale); every byte is then one
// character, as when the original's MB_CUR_MAX is 1.
var MultiByte = true

// MaxLen mirrors MB_CUR_MAX: how many bytes the record reader looks ahead when it feeds the regular expression
// matcher one character at a time.
var MaxLen = 6

// IsUTF reports the length (2 to 4) of the UTF-8 sequence at the start of s, 1 for an ASCII byte or the end of
// the string, and 0 for a byte with the high bit set that does not start a sequence (u8_isutf).
func IsUTF(s string) int {
	if len(s) == 0 || s[0] < 128 || !MultiByte {
		return 1
	}
	c := s[0]
	n := len(s)
	switch {
	case n >= 2 && (c>>5)&0x7 == 0x6 && s[1]&0xC0 == 0x80:
		return 2
	case n >= 3 && (c>>4)&0xF == 0xE && s[1]&0xC0 == 0x80 && s[2]&0xC0 == 0x80:
		return 3
	case n >= 4 && (c>>3)&0x1F == 0x1E && s[1]&0xC0 == 0x80 && s[2]&0xC0 == 0x80 && s[3]&0xC0 == 0x80:
		return 4
	}
	return 0
}

// Rune decodes the character at the start of s: its value and its length in bytes. A byte that does not start
// a sequence is returned as itself with length 1, and so is the end of the string (value 0), like u8_rune.
func Rune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 1
	}
	c := s[0]
	if c < 128 || !MultiByte {
		return rune(c), 1
	}
	n := len(s)
	switch {
	case n >= 2 && (c>>5)&0x7 == 0x6 && s[1]&0xC0 == 0x80:
		return rune(c&0x1F)<<6 | rune(s[1]&0x3F), 2
	case n >= 3 && (c>>4)&0xF == 0xE && s[1]&0xC0 == 0x80 && s[2]&0xC0 == 0x80:
		return rune(c&0xF)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	case n >= 4 && (c>>3)&0x1F == 0x1E && s[1]&0xC0 == 0x80 && s[2]&0xC0 == 0x80 && s[3]&0xC0 == 0x80:
		return rune(c&0x7)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
	return rune(c), 1
}

// NextLen is the length of the next character of s: 2 to 4 for a UTF-8 sequence, otherwise 1.
func NextLen(s string) int {
	if n := IsUTF(s); n != 0 {
		return n
	}
	return 1
}

// Strlen counts the characters of s.
func Strlen(s string) int {
	total := 0
	for i := 0; i < len(s); i += NextLen(s[i:]) {
		total++
	}
	return total
}

// Char2Byte converts a character count from the start of s into a byte offset. Past the end of s every
// further character counts as one byte, so callers clamp their counts first, as the original's do.
func Char2Byte(s string, charnum int) int {
	b := 0
	for ; charnum > 0; charnum-- {
		if b < len(s) {
			b += NextLen(s[b:])
		} else {
			b++
		}
	}
	return b
}

// Byte2Char returns the 1-based number of the character that contains byte offset bytenum of s (u8_byte2char);
// bytenum may equal len(s), the terminating position. It is -1 past that.
func Byte2Char(s string, bytenum int) int {
	if bytenum > len(s) {
		return -1
	}
	charnum := 0
	for i := 0; i <= bytenum; {
		if i < len(s) {
			i += NextLen(s[i:])
		} else {
			i++
		}
		charnum++
	}
	return charnum
}

// HasMulti reports whether s contains a character of two or more bytes (has_utf8).
func HasMulti(s string) bool {
	for i := 0; i < len(s); {
		n := NextLen(s[i:])
		if n > 1 {
			return true
		}
		i += n
	}
	return false
}

// AppendRune encodes c the way the original's runetochar (from Plan 9) does for \u escapes in program text:
// values above U+10FFFF become the error value 128, encoded in three bytes, and a negative value keeps only its
// low byte.
func AppendRune(b []byte, c int32) []byte {
	const (
		runeError = 128
		runeMax   = 0x10FFFF
	)
	switch {
	case c <= 0x7F:
		return append(b, byte(c))
	case c <= 0x7FF:
		return append(b, byte(0xC0|c>>6), byte(0x80|c&0x3F))
	}
	if c > runeMax {
		c = runeError
	}
	if c <= 0xFFFF {
		return append(b, byte(0xE0|c>>12), byte(0x80|(c>>6)&0x3F), byte(0x80|c&0x3F))
	}
	return append(b, byte(0xF0|c>>18), byte(0x80|(c>>12)&0x3F), byte(0x80|(c>>6)&0x3F), byte(0x80|c&0x3F))
}

// Encode is the UTF-8 encoding printf's %c uses for a code point given as a number; ok is false outside
// 0..U+10FFFF (surrogates are encoded like any other value).
func Encode(r int) (s string, ok bool) {
	if r < 0 || r > 0x10FFFF {
		return "", false
	}
	switch {
	case r <= 0x7F:
		return string([]byte{byte(r)}), true
	case r <= 0x7FF:
		return string([]byte{byte(0xC0 | r>>6), byte(0x80 | r&0x3F)}), true
	case r <= 0xFFFF:
		return string([]byte{byte(0xE0 | r>>12), byte(0x80 | (r>>6)&0x3F), byte(0x80 | r&0x3F)}), true
	}
	return string([]byte{byte(0xF0 | r>>18), byte(0x80 | (r>>12)&0x3F), byte(0x80 | (r>>6)&0x3F), byte(0x80 | r&0x3F)}), true
}
