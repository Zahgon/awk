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
	"os"
	"time"
)

// The C library's mathematical functions report domain and range errors through errno, which the original
// turns into warnings. These wrappers compute the result with Go's math package and the errno the C library
// sets.

// libmLog is log(x): a negative argument is a domain error, zero a range (pole) error.
func libmLog(x float64) (r float64, edom, erange bool) {
	switch {
	case x < 0:
		return math.NaN(), true, false
	case x == 0:
		return math.Inf(-1), false, true
	}
	return math.Log(x), false, false
}

// libmExp is exp(x): overflow to infinity and underflow to zero from a finite argument are range errors.
func libmExp(x float64) (float64, bool) {
	r := math.Exp(x)
	finite := !math.IsInf(x, 0) && !math.IsNaN(x)
	return r, finite && (math.IsInf(r, 0) || r == 0)
}

// libmPow is pow(x, y) for the exponents the interpreter does not compute itself (negative or fractional):
// a NaN from numeric arguments is a domain error, an infinite or zero result from finite non-zero ones a
// range error.
func libmPow(x, y float64) (r float64, edom, erange bool) {
	r = math.Pow(x, y)
	if math.IsNaN(x) || math.IsNaN(y) {
		return r, false, false
	}
	finite := !math.IsInf(x, 0) && !math.IsInf(y, 0)
	switch {
	case math.IsNaN(r):
		return r, true, false
	case finite && math.IsInf(r, 0):
		return r, false, true
	case finite && r == 0 && x != 0:
		return r, false, true
	}
	return r, false, false
}

func timeNow() int64 { return time.Now().Unix() }

func lookupEnv(name string) (string, bool) { return os.LookupEnv(name) }
