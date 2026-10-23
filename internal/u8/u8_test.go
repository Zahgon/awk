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

package u8

import "testing"

func TestCounting(t *testing.T) {
	tests := []struct {
		s     string
		chars int
	}{
		{"", 0}, {"abc", 3}, {"héllo", 5}, {"日本語", 3}, {"a\xffb", 3}, {"\xc3", 1}, {"😀x", 2},
	}
	for _, tc := range tests {
		if got := Strlen(tc.s); got != tc.chars {
			t.Errorf("Strlen(%q) = %d, want %d", tc.s, got, tc.chars)
		}
	}
	if got := Char2Byte("héllo", 2); got != 3 {
		t.Errorf("Char2Byte = %d, want 3", got)
	}
	if got := Byte2Char("héllo", 3); got != 3 {
		t.Errorf("Byte2Char = %d, want 3", got)
	}
	if got := Byte2Char("abc", 4); got != -1 {
		t.Errorf("Byte2Char past the end = %d, want -1", got)
	}
}

func TestAppendRune(t *testing.T) {
	tests := []struct {
		c    int32
		want string
	}{
		{0x41, "A"}, {0xe9, "é"}, {0x65e5, "日"}, {0x1F600, "😀"}, {0x110000, "\xe0\x82\x80"},
	}
	for _, tc := range tests {
		if got := string(AppendRune(nil, tc.c)); got != tc.want {
			t.Errorf("AppendRune(%#x) = %q, want %q", tc.c, got, tc.want)
		}
	}
}
