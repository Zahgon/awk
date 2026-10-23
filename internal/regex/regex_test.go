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

package regex

import "testing"

// Expected values recorded from the original b.c (built with MB_CUR_MAX 6, as in a UTF-8 locale).
var matchTests = []struct {
	re           string
	anchor       bool
	text         string
	match        bool
	start, len   int // PMatch at the start of a line
	start0, len0 int // PMatch elsewhere (^ cannot match)
	ne           bool
	nestart      int
	nelen        int
}{
	{"abc", true, "", false, 0, -1, 0, -1, false, 0, -1},
	{"abc", true, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"abc", true, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"abc", true, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"abc", true, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"a|b", false, "abc", true, 0, 2, 0, 2, true, 0, 2},
	{"a|b", false, "ab", true, 0, 2, 0, 2, true, 0, 2},
	{"a|b", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"a|b", false, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"a|b", false, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"ab+c*", false, "xaabbcc", true, 0, 7, 0, 7, true, 0, 7},
	{"ab+c*", false, "a.b", false, 0, -1, 0, -1, false, 0, -1},
	{"ab+c*", false, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"ab+c*", false, "\tz", false, 0, -1, 0, -1, false, 0, -1},
	{"^a", true, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"^a", true, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"^a", true, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"^a", true, "aab", true, 0, 1, 0, -1, true, 0, 1},
	{"a$", false, "aaa", true, 0, 3, 0, 3, true, 0, 3},
	{"a$", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a$", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"a$", false, "ba", true, 0, 2, 0, 2, true, 0, 2},
	{"^$", false, "", true, 0, 0, 0, -1, false, 0, -1},
	{"^$", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"^$", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"^$", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"^$", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"", true, "abc", true, 0, 0, 0, 0, false, 0, -1},
	{"", true, "ab", true, 0, 0, 0, 0, false, 0, -1},
	{"", true, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", true, 0, 0, 0, 0, false, 0, -1},
	{"", true, "c", true, 0, 0, 0, 0, false, 0, -1},
	{"", true, "\xc3x", true, 0, 0, 0, 0, false, 0, -1},
	{"x*", false, "xaabbcc", true, 0, 7, 0, 7, true, 0, 7},
	{"x*", false, "a.b", true, 0, 3, 0, 3, true, 0, 3},
	{"x*", false, "A1 b2", true, 0, 5, 0, 5, true, 0, 5},
	{"x*", false, "\tz", true, 0, 2, 0, 2, true, 0, 2},
	{"(ab)+", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab)+", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab)+", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab)+", false, "aab", true, 0, 3, 0, 3, true, 0, 3},
	{"()", true, "aaa", true, 0, 0, 0, 0, false, 0, -1},
	{"()", true, "x\xc3\xa9\xc3\xbcy", true, 0, 0, 0, 0, false, 0, -1},
	{"()", true, "(a)", true, 0, 0, 0, 0, false, 0, -1},
	{"()", true, "ba", true, 0, 0, 0, 0, false, 0, -1},
	{"a()b", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"a()b", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"a()b", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a()b", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"a()b", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"[abc]", false, "abc", true, 0, 3, 0, 3, true, 0, 3},
	{"[abc]", false, "ab", true, 0, 2, 0, 2, true, 0, 2},
	{"[abc]", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"[abc]", false, "c", true, 0, 1, 0, 1, true, 0, 1},
	{"[abc]", false, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"[^abc]", true, "xaabbcc", true, 0, 1, 0, 1, true, 0, 1},
	{"[^abc]", true, "a.b", false, 1, 1, 1, 1, true, 1, 1},
	{"[^abc]", true, "A1 b2", true, 0, 1, 0, 1, true, 0, 1},
	{"[^abc]", true, "\tz", true, 0, 1, 0, 1, true, 0, 1},
	{"[a-c]+", false, "b", true, 0, 1, 0, 1, true, 0, 1},
	{"[a-c]+", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"[a-c]+", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"[a-c]+", false, "aab", true, 0, 3, 0, 3, true, 0, 3},
	{"[c-a]x", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"[c-a]x", false, "x\xc3\xa9\xc3\xbcy", true, 0, 1, 0, 1, true, 0, 1},
	{"[c-a]x", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"[c-a]x", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"[]a]", true, "", false, 0, -1, 0, -1, false, 0, -1},
	{"[]a]", true, "aaa", true, 0, 1, 0, 1, true, 0, 1},
	{"[]a]", true, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"[]a]", true, "(a)", false, 1, 1, 1, 1, true, 1, 1},
	{"[]a]", true, "ba", false, 1, 1, 1, 1, true, 1, 1},
	{"[^]a]", false, "abc", true, 0, 3, 0, 3, true, 0, 3},
	{"[^]a]", false, "ab", true, 0, 2, 0, 2, true, 0, 2},
	{"[^]a]", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", true, 0, 9, 0, 9, true, 0, 9},
	{"[^]a]", false, "c", true, 0, 1, 0, 1, true, 0, 1},
	{"[^]a]", false, "\xc3x", true, 0, 2, 0, 2, true, 0, 2},
	{"[a-]", false, "xaabbcc", true, 0, 3, 0, 3, true, 0, 3},
	{"[a-]", false, "a.b", true, 0, 1, 0, 1, true, 0, 1},
	{"[a-]", false, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"[a-]", false, "\tz", false, 0, -1, 0, -1, false, 0, -1},
	{"[-a]", true, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"[-a]", true, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"[-a]", true, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"[-a]", true, "aab", true, 0, 1, 0, 1, true, 0, 1},
	{"[[:alpha:]]+", false, "aaa", true, 0, 3, 0, 3, true, 0, 3},
	{"[[:alpha:]]+", false, "x\xc3\xa9\xc3\xbcy", true, 0, 6, 0, 6, true, 0, 6},
	{"[[:alpha:]]+", false, "(a)", true, 0, 2, 0, 2, true, 0, 2},
	{"[[:alpha:]]+", false, "ba", true, 0, 2, 0, 2, true, 0, 2},
	{"[[:digit:][:upper:]]+", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:digit:][:upper:]]+", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:digit:][:upper:]]+", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:digit:][:upper:]]+", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:digit:][:upper:]]+", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:punct:]]", true, "abc", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:punct:]]", true, "ab", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:punct:]]", true, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:punct:]]", true, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:punct:]]", true, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:space:]]", false, "xaabbcc", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:space:]]", false, "a.b", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:space:]]", false, "A1 b2", true, 0, 3, 0, 3, true, 0, 3},
	{"[[:space:]]", false, "\tz", true, 0, 1, 0, 1, true, 0, 1},
	{"[[:bogus:]]", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:bogus:]]", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:bogus:]]", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:bogus:]]", false, "aab", false, 0, -1, 0, -1, false, 0, -1},
	{"[[.a.]]", true, "aaa", true, 0, 1, 0, 1, true, 0, 1},
	{"[[.a.]]", true, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"[[.a.]]", true, "(a)", false, 1, 1, 1, 1, true, 1, 1},
	{"[[.a.]]", true, "ba", false, 1, 1, 1, 1, true, 1, 1},
	{"[[=e=]]", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"[[=e=]]", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"[[=e=]]", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"[[=e=]]", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"[[=e=]]", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"[\xc3\xa9-\xc3\xbc]+", false, "abc", false, 0, -1, 0, -1, false, 0, -1},
	{"[\xc3\xa9-\xc3\xbc]+", false, "ab", false, 0, -1, 0, -1, false, 0, -1},
	{"[\xc3\xa9-\xc3\xbc]+", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"[\xc3\xa9-\xc3\xbc]+", false, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"[\xc3\xa9-\xc3\xbc]+", false, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"[^\xc3\xa9]", true, "xaabbcc", true, 0, 1, 0, 1, true, 0, 1},
	{"[^\xc3\xa9]", true, "a.b", true, 0, 1, 0, 1, true, 0, 1},
	{"[^\xc3\xa9]", true, "A1 b2", true, 0, 1, 0, 1, true, 0, 1},
	{"[^\xc3\xa9]", true, "\tz", true, 0, 1, 0, 1, true, 0, 1},
	{"\xc3\xa9+", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"\xc3\xa9+", false, "\xc3\xa9\xc3\xa9", true, 0, 4, 0, 4, true, 0, 4},
	{"\xc3\xa9+", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"\xc3\xa9+", false, "aab", false, 0, -1, 0, -1, false, 0, -1},
	{"\xe6\x97\xa5\xe6\x9c\xac", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"\xe6\x97\xa5\xe6\x9c\xac", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"\xe6\x97\xa5\xe6\x9c\xac", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"\xe6\x97\xa5\xe6\x9c\xac", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{".", true, "", false, 0, -1, 0, -1, false, 0, -1},
	{".", true, "aaa", true, 0, 1, 0, 1, true, 0, 1},
	{".", true, "x\xc3\xa9\xc3\xbcy", true, 0, 1, 0, 1, true, 0, 1},
	{".", true, "(a)", true, 0, 1, 0, 1, true, 0, 1},
	{".", true, "ba", true, 0, 1, 0, 1, true, 0, 1},
	{"..", false, "abc", true, 0, 3, 0, 3, true, 0, 3},
	{"..", false, "ab", true, 0, 2, 0, 2, true, 0, 2},
	{"..", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", true, 0, 9, 0, 9, true, 0, 9},
	{"..", false, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"..", false, "\xc3x", true, 0, 2, 0, 2, true, 0, 2},
	{"a{2}", false, "xaabbcc", true, 0, 3, 0, 3, true, 0, 3},
	{"a{2}", false, "a.b", false, 0, -1, 0, -1, false, 0, -1},
	{"a{2}", false, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"a{2}", false, "\tz", false, 0, -1, 0, -1, false, 0, -1},
	{"a{2,}", true, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"a{2,}", true, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"a{2,}", true, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"a{2,}", true, "aab", true, 0, 2, 0, 2, true, 0, 2},
	{"a{1,3}", false, "aaa", true, 0, 3, 0, 3, true, 0, 3},
	{"a{1,3}", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a{1,3}", false, "(a)", true, 0, 2, 0, 2, true, 0, 2},
	{"a{1,3}", false, "ba", true, 0, 2, 0, 2, true, 0, 2},
	{"a{0}b", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"a{0}b", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"a{0}b", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a{0}b", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"a{0}b", false, "ba", true, 0, 1, 0, 1, true, 0, 1},
	{"a{0,1}b", true, "abc", true, 0, 2, 0, 2, true, 0, 2},
	{"a{0,1}b", true, "ab", true, 0, 2, 0, 2, true, 0, 2},
	{"a{0,1}b", true, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"a{0,1}b", true, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"a{0,1}b", true, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab){2}", false, "xaabbcc", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab){2}", false, "a.b", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab){2}", false, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"(ab){2}", false, "\tz", false, 0, -1, 0, -1, false, 0, -1},
	{"x{,2}", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"x{,2}", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"x{,2}", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"x{,2}", false, "aab", false, 0, -1, 0, -1, false, 0, -1},
	{"a{", true, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"a{", true, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a{", true, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"a{", true, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"a)", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"a)", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"a)", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a)", false, "(a)", true, 0, 3, 0, 3, true, 0, 3},
	{"a)", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"\\(a\\)", false, "abc", false, 0, -1, 0, -1, false, 0, -1},
	{"\\(a\\)", false, "ab", false, 0, -1, 0, -1, false, 0, -1},
	{"\\(a\\)", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"\\(a\\)", false, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"\\(a\\)", false, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"a\\.b", true, "xaabbcc", false, 0, -1, 0, -1, false, 0, -1},
	{"a\\.b", true, "a.b", true, 0, 3, 0, 3, true, 0, 3},
	{"a\\.b", true, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"a\\.b", true, "\tz", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x41", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x41", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x41", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x41", false, "aab", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x4g", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x4g", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x4g", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"\\x4g", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"\\101", true, "", false, 0, -1, 0, -1, false, 0, -1},
	{"\\101", true, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"\\101", true, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"\\101", true, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"\\101", true, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"\\u00e9", false, "abc", false, 0, -1, 0, -1, false, 0, -1},
	{"\\u00e9", false, "ab", false, 0, -1, 0, -1, false, 0, -1},
	{"\\u00e9", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"\\u00e9", false, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"\\u00e9", false, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"\\t", false, "xaabbcc", false, 0, -1, 0, -1, false, 0, -1},
	{"\\t", false, "a.b", false, 0, -1, 0, -1, false, 0, -1},
	{"\\t", false, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"\\t", false, "\tz", true, 0, 1, 0, 1, true, 0, 1},
	{"a|^b", true, "b", true, 0, 1, 0, -1, true, 0, 1},
	{"a|^b", true, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"a|^b", true, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"a|^b", true, "aab", true, 0, 1, 0, 1, true, 0, 1},
	{"(^a|b)c", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"(^a|b)c", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"(^a|b)c", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"(^a|b)c", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"x$|y", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"x$|y", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"x$|y", false, "x\xc3\xa9\xc3\xbcy", true, 0, 6, 0, 6, true, 0, 6},
	{"x$|y", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"x$|y", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"$a", true, "abc", false, 0, -1, 0, -1, false, 0, -1},
	{"$a", true, "ab", false, 0, -1, 0, -1, false, 0, -1},
	{"$a", true, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"$a", true, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"$a", true, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"ab|a", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"ab|a", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"ab|a", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"ab|a", false, "aab", true, 0, 3, 0, 3, true, 0, 3},
	{"a|ab", true, "aaa", true, 0, 1, 0, 1, true, 0, 1},
	{"a|ab", true, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"a|ab", true, "(a)", false, 1, 1, 1, 1, true, 1, 1},
	{"a|ab", true, "ba", false, 1, 1, 1, 1, true, 1, 1},
	{"[[:alnum:]_]+@", false, "", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:alnum:]_]+@", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:alnum:]_]+@", false, "x\xc3\xa9\xc3\xbcy", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:alnum:]_]+@", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"[[:alnum:]_]+@", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"\\/", false, "abc", false, 0, -1, 0, -1, false, 0, -1},
	{"\\/", false, "ab", false, 0, -1, 0, -1, false, 0, -1},
	{"\\/", false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", false, 0, -1, 0, -1, false, 0, -1},
	{"\\/", false, "c", false, 0, -1, 0, -1, false, 0, -1},
	{"\\/", false, "\xc3x", false, 0, -1, 0, -1, false, 0, -1},
	{"a.c", true, "xaabbcc", false, 0, -1, 0, -1, false, 0, -1},
	{"a.c", true, "a.b", false, 0, -1, 0, -1, false, 0, -1},
	{"a.c", true, "A1 b2", false, 0, -1, 0, -1, false, 0, -1},
	{"a.c", true, "\tz", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\]]", false, "b", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\]]", false, "\xc3\xa9\xc3\xa9", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\]]", false, "x)y", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\]]", false, "aab", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\^x]", false, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\^x]", false, "x\xc3\xa9\xc3\xbcy", true, 0, 1, 0, 1, true, 0, 1},
	{"[\\^x]", false, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"[\\^x]", false, "ba", false, 0, -1, 0, -1, false, 0, -1},
	{"\xc3\xa9{2}", true, "", false, 0, -1, 0, -1, false, 0, -1},
	{"\xc3\xa9{2}", true, "aaa", false, 0, -1, 0, -1, false, 0, -1},
	{"\xc3\xa9{2}", true, "x\xc3\xa9\xc3\xbcy", false, 1, 2, 1, 2, true, 1, 2},
	{"\xc3\xa9{2}", true, "(a)", false, 0, -1, 0, -1, false, 0, -1},
	{"\xc3\xa9{2}", true, "ba", false, 0, -1, 0, -1, false, 0, -1},
}

func TestMatchers(t *testing.T) {
	for _, tc := range matchTests {
		f, err := Compile(tc.re, tc.anchor)
		if err != nil {
			t.Errorf("Compile(%q): %v", tc.re, err)
			continue
		}
		if got := f.Match(tc.text); got != tc.match {
			t.Errorf("%q Match(%q) = %v, want %v", tc.re, tc.text, got, tc.match)
		}
		if s, l := f.PMatch(tc.text, true); s != tc.start || l != tc.len {
			t.Errorf("%q PMatch(%q, true) = %d %d, want %d %d", tc.re, tc.text, s, l, tc.start, tc.len)
		}
		if s, l := f.PMatch(tc.text, false); s != tc.start0 || l != tc.len0 {
			t.Errorf("%q PMatch(%q, false) = %d %d, want %d %d", tc.re, tc.text, s, l, tc.start0, tc.len0)
		}
		if s, l, ok := f.NEMatch(tc.text, true); ok != tc.ne || s != tc.nestart || l != tc.nelen {
			t.Errorf("%q NEMatch(%q) = %d %d %v, want %d %d %v", tc.re, tc.text, s, l, ok, tc.nestart, tc.nelen, tc.ne)
		}
	}
}

var errorTests = []struct{ re, msg string }{
	{"(a|)b", "illegal primary in regular expression (a|)b at b"},
	{"a(b", "syntax error in regular expression a(b at "},
	{"a^b", "syntax error in regular expression a^b at b"},
	{"a^b", "syntax error in regular expression a^b at b"},
	{"(", "illegal primary in regular expression ( at "},
	{"a{3,1}", "illegal repetition expression: class a{3,1}"},
	{"a{300}", "repetition count a{300} too large"},
	{"a{2,x}", "illegal repetition expression: class a{2,x}"},
	{"[abc", "nonterminated character class [abc"},
	{"[a\\", "nonterminated character class [a\\..."},
	{"*a", "illegal primary in regular expression *a at a"},
	{"+", "illegal primary in regular expression + at "},
	{"a|*", "illegal primary in regular expression a|* at "},
	{"a{1,2", "nonterminated character class a{1,2"},
	{"(|a)", "illegal primary in regular expression (|a) at a)"},
	{"[[:alpha:", "nonterminated character class [[:alpha:"},
	{"abcdefghijklmnopqrstuvwxyz(", "illegal primary in regular expression abcdefghijklmnopqrstuvwxyz( at "},
}

func TestErrors(t *testing.T) {
	for _, tc := range errorTests {
		if _, err := Compile(tc.re, false); err == nil || err.Error() != tc.msg {
			t.Errorf("Compile(%q) error = %v, want %q", tc.re, err, tc.msg)
		}
	}
}

// bytesInput is an Input over a byte slice.
type bytesInput struct {
	data []byte
	back []byte
}

func (in *bytesInput) Getc() int {
	if n := len(in.back); n > 0 {
		c := in.back[n-1]
		in.back = in.back[:n-1]
		return int(c)
	}
	if len(in.data) == 0 {
		return -1
	}
	c := in.data[0]
	in.data = in.data[1:]
	return int(c)
}

func (in *bytesInput) Ungetc(c byte) { in.back = append(in.back, c) }

// TestFNEMatch splits a stream into records the way a regular expression RS does.
func TestFNEMatch(t *testing.T) {
	tests := []struct {
		re, input string
		records   []string
	}{
		{"\\n\\n+", "a\nb\n\n\nc\n", []string{"a\nb", "c\n"}},
		{"[,;]", "x,y;;z", []string{"x", "y", "", "z"}},
		{"^a", "aaa", []string{"", "aa"}},
		{"日|€", "1日2€3", []string{"1", "2", "3"}},
	}
	for _, tc := range tests {
		f, err := Compile(tc.re, true)
		if err != nil {
			t.Fatal(err)
		}
		in := &bytesInput{data: []byte(tc.input)}
		var got []string
		for bol := true; ; bol = false {
			buf, start, _, found := f.FNEMatch(in, bol)
			if found {
				buf = buf[:start]
			} else if len(buf) == 0 {
				break
			}
			got = append(got, string(buf))
		}
		if len(got) != len(tc.records) {
			t.Errorf("%q on %q: records %q, want %q", tc.re, tc.input, got, tc.records)
			continue
		}
		for i := range got {
			if got[i] != tc.records[i] {
				t.Errorf("%q on %q: records %q, want %q", tc.re, tc.input, got, tc.records)
				break
			}
		}
	}
}
