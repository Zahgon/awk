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

// Random is glibc's random() generator with its default state (TYPE_3: an additive feedback generator over 31
// words with taps 3 apart). The zero value behaves like a process that never called srandom, which is the
// same as srandom(1).
type Random struct {
	state  [31]int32
	f, r   int
	seeded bool
}

// Seed restarts the sequence as srandom(seed) does. A seed of 0 is treated as 1.
func (g *Random) Seed(seed uint32) {
	if seed == 0 {
		seed = 1
	}
	g.state[0] = int32(seed)
	word := int64(int32(seed))
	for i := 1; i < len(g.state); i++ {
		// 16807 * word % 2147483647 by Schrage's method, as in the C library, so that the arithmetic
		// stays within 32 bits for the same results.
		hi, lo := word/127773, word%127773
		word = 16807*lo - 2836*hi
		if word < 0 {
			word += 2147483647
		}
		g.state[i] = int32(word)
	}
	g.f, g.r = 3, 0
	g.seeded = true
	for i := 0; i < 310; i++ {
		g.Next()
	}
}

// Next returns the next value in 0..2^31-1.
func (g *Random) Next() int32 {
	if !g.seeded {
		g.Seed(1)
	}
	sum := uint32(g.state[g.f]) + uint32(g.state[g.r])
	g.state[g.f] = int32(sum)
	result := int32(sum>>1) & 0x7fffffff
	g.f++
	if g.f >= len(g.state) {
		g.f = 0
	}
	g.r++
	if g.r >= len(g.state) {
		g.r = 0
	}
	return result
}
