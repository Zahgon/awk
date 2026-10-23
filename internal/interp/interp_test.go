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
	"testing"
)

func TestIsValidNumber(t *testing.T) {
	tests := []struct {
		s          string
		trailingOK bool
		val        float64
		noTrailing bool
		ok         bool
	}{
		{"10", false, 10, true, true},
		{" 12 ", false, 12, true, true},
		{"1e3", false, 1000, true, true},
		{"0x1A", false, 0, false, false},
		{"+0x1A", false, 26, true, true},
		{"12abc", false, 12, false, false},
		{"12abc", true, 12, false, true},
		{"+inf", false, math.Inf(1), true, true},
		{"inf", false, 0, false, false},
		{"+infinity", false, 0, false, false},
		{"1e400", true, 0, false, false},
		{".", false, 0, false, false},
		{"", false, 0, false, false},
	}
	for _, tc := range tests {
		v, nt, ok := isValidNumber(tc.s, tc.trailingOK)
		if ok != tc.ok || (ok && (v != tc.val || nt != tc.noTrailing)) {
			t.Errorf("isValidNumber(%q, %v) = %v, %v, %v; want %v, %v, %v", tc.s, tc.trailingOK, v, nt, ok,
				tc.val, tc.noTrailing, tc.ok)
		}
	}
}

func TestCfmt(t *testing.T) {
	tests := []struct {
		format string
		args   []any
		want   string
	}{
		{"non-terminated string %.10s...", []any{"héllo wörld"}, "non-terminated string héllo wö..."},
		{"function %s called with %d args, uses only %d", []any{"f", 3, 1}, "function f called with 3 args, uses only 1"},
		{"division by zero in %%=", nil, "division by zero in %="},
		{"bad type variable %d", []any{16}, "bad type variable 16"},
	}
	for _, tc := range tests {
		if got := cfmt(tc.format, tc.args...); got != tc.want {
			t.Errorf("cfmt(%q) = %q, want %q", tc.format, got, tc.want)
		}
	}
}

// The visiting order of for (k in a) is the original's hash table order: chains at the head of buckets,
// regrown by four once the table is twice full.
func TestArrayOrder(t *testing.T) {
	a := makeSymtab(nSymTab)
	for _, k := range []string{"1", "2", "3", "4", "5", "6"} {
		a.set(k, "", 0, tSTR)
	}
	var got []string
	for _, c := range a.tab {
		for ; c != nil; c = c.cnext {
			got = append(got, c.nval)
		}
	}
	want := []string{"2", "3", "4", "5", "6", "1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	for i := 0; i < 200; i++ {
		a.set(string(rune('a'+i%26))+string(rune('a'+i/26)), "", 0, tSTR)
	}
	if a.nelem != 206 || len(a.tab) != 200 {
		t.Errorf("after growing: %d elements in %d buckets, want 206 in 200", a.nelem, len(a.tab))
	}
}
