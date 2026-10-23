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

// Command yaccfix adjusts the parser goyacc generates so that its error recovery is the one of yacc and
// bison, which the original awk's parser uses. When a syntax error is found again before three tokens have
// been shifted since the last one, goyacc discards the lookahead token and tries again in the same state;
// yacc and bison discard it and then recover once more, popping to a state that accepts the error token and
// shifting it. The difference shows in the grammar's error actions (an "illegal statement" message per
// recovery), so the generated recovery is rewritten to do what bison does.
//
//	go run ./internal/interp/yaccfix grammar.go
package main

import (
	"fmt"
	"os"
	"strings"
)

const goyaccRetry = `			yyrcvr.char = -1
			yytoken = -1
			goto yynewstate /* try again in the same state */`

const bisonRetry = `			yyrcvr.char = -1
			yytoken = -1
			/* as yacc and bison do: recover again, shifting "error" */
			for yyp >= 0 {
				yyn = int(yyPact[yyS[yyp].yys]) + yyErrCode
				if yyn >= 0 && yyn < yyLast {
					yystate = int(yyAct[yyn])
					if int(yyChk[yystate]) == yyErrCode {
						goto yystack
					}
				}
				yyp--
			}
			if yyp < 0 { /* no state on the stack takes "error": abort */
				goto ret1
			}
			goto yynewstate`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: yaccfix grammar.go")
		os.Exit(2)
	}
	name := os.Args[1]
	src, err := os.ReadFile(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	s := string(src)
	if strings.Count(s, goyaccRetry) != 1 {
		fmt.Fprintln(os.Stderr, "yaccfix: goyacc's error recovery code not found in", name)
		os.Exit(1)
	}
	s = strings.Replace(s, goyaccRetry, bisonRetry, 1)
	if err := os.WriteFile(name, []byte(s), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
