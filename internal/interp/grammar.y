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

// The awk grammar, a transcription of the original's awkgram.y for goyacc: the same tokens, precedences and
// rules, so the parser makes the same decisions (and detects syntax errors at the same tokens) as the yacc
// parser of the original. Build the parser with
//
//	goyacc -l -o grammar.go -v /dev/null grammar.y
%{
package interp
%}

%union {
	p  *Node
	cp *Cell
	i  int
	s  string
}

%token	<i>	FIRSTTOKEN	/* must be first */
%token	<p>	PROGRAM PASTAT PASTAT2 XBEGIN XEND
%token	<i>	NL ',' '{' '(' '|' ';' '/' ')' '}' '[' ']'
%token	<i>	ARRAY
%token	<i>	MATCH NOTMATCH MATCHOP
%token	<i>	FINAL DOT ALL CCL NCCL CHAR OR STAR QUEST PLUS EMPTYRE ZERO
%token	<i>	AND BOR APPEND EQ GE GT LE LT NE IN
%token	<i>	ARG BLTIN BREAK CLOSE CONTINUE DELETE DO EXIT FOR FUNC
%token	<i>	SUB GSUB IF INDEX LSUBSTR MATCHFCN NEXT NEXTFILE
%token	<i>	ADD MINUS MULT DIVIDE MOD
%token	<i>	ASSIGN ASGNOP ADDEQ SUBEQ MULTEQ DIVEQ MODEQ POWEQ
%token	<i>	PRINT PRINTF SPRINTF
%token	<p>	ELSE INTEST CONDEXPR
%token	<i>	POSTINCR PREINCR POSTDECR PREDECR
%token	<cp>	VAR IVAR VARNF CALL NUMBER STRING
%token	<s>	REGEXPR

%type	<p>	pas pattern ppattern plist pplist patlist prarg term re
%type	<p>	pa_pat pa_stat pa_stats
%type	<s>	reg_expr
%type	<p>	simple_stmt opt_simple_stmt stmt stmtlist
%type	<p>	var varname varlist
%type	<cp>	funcname
%type	<p>	for if else while
%type	<i>	do st
%type	<i>	pst opt_pst lbrace rbrace rparen comma nl opt_nl and bor
%type	<i>	subop print
%type	<cp>	string

%right	ASGNOP
%right	'?'
%right	':'
%left	BOR
%left	AND
%left	GETLINE
%nonassoc APPEND EQ GE GT LE LT NE MATCHOP IN '|'
%left	ARG BLTIN BREAK CALL CLOSE CONTINUE DELETE DO EXIT FOR FUNC
%left	GSUB IF INDEX LSUBSTR MATCHFCN NEXT NUMBER
%left	PRINT PRINTF RETURN SPLIT SPRINTF STRING SUB SUBSTR
%left	REGEXPR VAR VARNF IVAR WHILE '('
%left	CAT
%left	'+' '-'
%left	'*' '/' '%'
%left	NOT UMINUS UPLUS
%right	POWER
%right	DECR INCR
%left	INDIRECT
%token	LASTTOKEN	/* must be last */

%%

program:
	  pas	{ if pp(yylex).errorflag() == 0 {
			pp(yylex).setWinner($1) } }
	| error	{ yyrcvr.char = -1; pp(yylex).in.bracecheck(); pp(yylex).in.syntax("bailing out") }
	;

and:
	  AND | and NL
	;

bor:
	  BOR | bor NL
	;

comma:
	  ',' | comma NL
	;

do:
	  DO | do NL
	;

else:
	  ELSE | else NL
	;

for:
	  FOR '(' opt_simple_stmt ';' opt_nl pattern ';' opt_nl opt_simple_stmt rparen {pp(yylex).inloop++} stmt
		{ pp(yylex).inloop--; $$ = pp(yylex).stat4(FOR, $3, pp(yylex).notnull($6), $9, $12) }
	| FOR '(' opt_simple_stmt ';'  ';' opt_nl opt_simple_stmt rparen {pp(yylex).inloop++} stmt
		{ pp(yylex).inloop--; $$ = pp(yylex).stat4(FOR, $3, nil, $7, $10) }
	| FOR '(' varname IN varname rparen {pp(yylex).inloop++} stmt
		{ pp(yylex).inloop--; $$ = pp(yylex).stat3(IN, $3, pp(yylex).makearr($5), $8) }
	;

funcname:
	  VAR	{ pp(yylex).setfname($1) }
	| CALL	{ pp(yylex).setfname($1) }
	;

if:
	  IF '(' pattern rparen		{ $$ = pp(yylex).notnull($3) }
	;

lbrace:
	  '{' | lbrace NL
	;

nl:
	  NL | nl NL
	;

opt_nl:
	  /* empty */	{ $$ = 0 }
	| nl
	;

opt_pst:
	  /* empty */	{ $$ = 0 }
	| pst
	;


opt_simple_stmt:
	  /* empty */			{ $$ = nil }
	| simple_stmt
	;

pas:
	  opt_pst			{ $$ = nil }
	| opt_pst pa_stats opt_pst	{ $$ = $2 }
	;

pa_pat:
	  pattern	{ $$ = pp(yylex).notnull($1) }
	;

pa_stat:
	  pa_pat			{ $$ = pp(yylex).stat2(PASTAT, $1, pp(yylex).stat2(PRINT, pp(yylex).rectonode(), nil)) }
	| pa_pat lbrace stmtlist '}'	{ $$ = pp(yylex).stat2(PASTAT, $1, $3) }
	| pa_pat ',' opt_nl pa_pat		{ $$ = pp(yylex).pa2stat($1, $4, pp(yylex).stat2(PRINT, pp(yylex).rectonode(), nil)) }
	| pa_pat ',' opt_nl pa_pat lbrace stmtlist '}'	{ $$ = pp(yylex).pa2stat($1, $4, $6) }
	| lbrace stmtlist '}'		{ $$ = pp(yylex).stat2(PASTAT, nil, $2) }
	| XBEGIN lbrace stmtlist '}'
		{ pp(yylex).beginloc = pp(yylex).linkum(pp(yylex).beginloc, $3); $$ = nil }
	| XEND lbrace stmtlist '}'
		{ pp(yylex).endloc = pp(yylex).linkum(pp(yylex).endloc, $3); $$ = nil }
	| FUNC funcname '(' varlist rparen {pp(yylex).infunc = true} lbrace stmtlist '}'
		{ pp(yylex).infunc = false; pp(yylex).in.curfname = ""; pp(yylex).defn($2, $4, $8); $$ = nil }
	;

pa_stats:
	  pa_stat
	| pa_stats opt_pst pa_stat	{ $$ = pp(yylex).linkum($1, $3) }
	;

patlist:
	  pattern
	| patlist comma pattern		{ $$ = pp(yylex).linkum($1, $3) }
	;

ppattern:
	  var ASGNOP ppattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| ppattern '?' ppattern ':' ppattern %prec '?'
		{ $$ = pp(yylex).op3(CONDEXPR, pp(yylex).notnull($1), $3, $5) }
	| ppattern bor ppattern %prec BOR
		{ $$ = pp(yylex).op2(BOR, pp(yylex).notnull($1), pp(yylex).notnull($3)) }
	| ppattern and ppattern %prec AND
		{ $$ = pp(yylex).op2(AND, pp(yylex).notnull($1), pp(yylex).notnull($3)) }
	| ppattern MATCHOP reg_expr	{ $$ = pp(yylex).matchop($2, $1, $3) }
	| ppattern MATCHOP ppattern	{ $$ = pp(yylex).matchopExpr($2, $1, $3) }
	| ppattern IN varname		{ $$ = pp(yylex).op2(INTEST, $1, pp(yylex).makearr($3)) }
	| '(' plist ')' IN varname	{ $$ = pp(yylex).op2(INTEST, $2, pp(yylex).makearr($5)) }
	| ppattern term %prec CAT	{ $$ = pp(yylex).op2(CAT, $1, $2) }
	| re
	| term
	;

pattern:
	  var ASGNOP pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern '?' pattern ':' pattern %prec '?'
		{ $$ = pp(yylex).op3(CONDEXPR, pp(yylex).notnull($1), $3, $5) }
	| pattern bor pattern %prec BOR
		{ $$ = pp(yylex).op2(BOR, pp(yylex).notnull($1), pp(yylex).notnull($3)) }
	| pattern and pattern %prec AND
		{ $$ = pp(yylex).op2(AND, pp(yylex).notnull($1), pp(yylex).notnull($3)) }
	| pattern EQ pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern GE pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern GT pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern LE pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern LT pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern NE pattern		{ $$ = pp(yylex).op2($2, $1, $3) }
	| pattern MATCHOP reg_expr	{ $$ = pp(yylex).matchop($2, $1, $3) }
	| pattern MATCHOP pattern	{ $$ = pp(yylex).matchopExpr($2, $1, $3) }
	| pattern IN varname		{ $$ = pp(yylex).op2(INTEST, $1, pp(yylex).makearr($3)) }
	| '(' plist ')' IN varname	{ $$ = pp(yylex).op2(INTEST, $2, pp(yylex).makearr($5)) }
	| pattern '|' GETLINE var	{ $$ = pp(yylex).getline($4, '|', $1) }
	| pattern '|' GETLINE		{ $$ = pp(yylex).getline(nil, '|', $1) }
	| pattern term %prec CAT	{ $$ = pp(yylex).op2(CAT, $1, $2) }
	| re
	| term
	;

plist:
	  pattern comma pattern		{ $$ = pp(yylex).linkum($1, $3) }
	| plist comma pattern		{ $$ = pp(yylex).linkum($1, $3) }
	;

pplist:
	  ppattern
	| pplist comma ppattern		{ $$ = pp(yylex).linkum($1, $3) }
	;

prarg:
	  /* empty */			{ $$ = pp(yylex).rectonode() }
	| pplist
	| '(' plist ')'			{ $$ = $2 }
	;

print:
	  PRINT | PRINTF
	;

pst:
	  NL | ';' | pst NL | pst ';'
	;

rbrace:
	  '}' | rbrace NL
	;

re:
	   reg_expr
		{ $$ = pp(yylex).matchop(MATCH, pp(yylex).rectonode(), $1) }
	| NOT re	{ $$ = pp(yylex).op1(NOT, pp(yylex).notnull($2)) }
	;

reg_expr:
	  '/' {pp(yylex).startreg()} REGEXPR '/'		{ $$ = $3 }
	;

rparen:
	  ')' | rparen NL
	;

simple_stmt:
	  print prarg '|' term		{ $$ = pp(yylex).stat3print($1, $2, '|', $4) }
	| print prarg APPEND term	{ $$ = pp(yylex).stat3print($1, $2, APPEND, $4) }
	| print prarg GT term		{ $$ = pp(yylex).stat3print($1, $2, GT, $4) }
	| print prarg			{ $$ = pp(yylex).stat3print($1, $2, 0, nil) }
	| DELETE varname '[' patlist ']' { $$ = pp(yylex).stat2(DELETE, pp(yylex).makearr($2), $4) }
	| DELETE varname		 { $$ = pp(yylex).stat2(DELETE, pp(yylex).makearr($2), nil) }
	| pattern			{ $$ = exptostat($1) }
	| error				{ yyrcvr.char = -1; pp(yylex).in.syntax("illegal statement") }
	;

st:
	  nl
	| ';' opt_nl
	;

stmt:
	  BREAK st		{ if pp(yylex).inloop == 0 { pp(yylex).in.syntax("break illegal outside of loops") }
				  $$ = pp(yylex).stat1(BREAK, nil) }
	| CONTINUE st		{ if pp(yylex).inloop == 0 { pp(yylex).in.syntax("continue illegal outside of loops") }
				  $$ = pp(yylex).stat1(CONTINUE, nil) }
	| do {pp(yylex).inloop++} stmt {pp(yylex).inloop--} WHILE '(' pattern ')' st
		{ $$ = pp(yylex).stat2(DO, $3, pp(yylex).notnull($7)) }
	| EXIT pattern st	{ $$ = pp(yylex).stat1(EXIT, $2) }
	| EXIT st		{ $$ = pp(yylex).stat1(EXIT, nil) }
	| for
	| if stmt else stmt	{ $$ = pp(yylex).stat3(IF, $1, $2, $4) }
	| if stmt		{ $$ = pp(yylex).stat3(IF, $1, $2, nil) }
	| lbrace stmtlist rbrace { $$ = $2 }
	| NEXT st	{ if pp(yylex).infunc { pp(yylex).in.syntax("next is illegal inside a function") }
			  $$ = pp(yylex).stat1(NEXT, nil) }
	| NEXTFILE st	{ if pp(yylex).infunc { pp(yylex).in.syntax("nextfile is illegal inside a function") }
			  $$ = pp(yylex).stat1(NEXTFILE, nil) }
	| RETURN pattern st	{ $$ = pp(yylex).stat1(RETURN, $2) }
	| RETURN st		{ $$ = pp(yylex).stat1(RETURN, nil) }
	| simple_stmt st
	| while {pp(yylex).inloop++} stmt	{ pp(yylex).inloop--; $$ = pp(yylex).stat2(WHILE, $1, $3) }
	| ';' opt_nl		{ $$ = nil }
	;

stmtlist:
	  stmt
	| stmtlist stmt		{ $$ = pp(yylex).linkum($1, $2) }
	;

subop:
	  SUB | GSUB
	;

string:
	  STRING
	| string STRING		{ $$ = pp(yylex).in.catstr($1, $2) }
	;

term:
	  term '/' ASGNOP term		{ $$ = pp(yylex).op2(DIVEQ, $1, $4) }
	| term '+' term			{ $$ = pp(yylex).op2(ADD, $1, $3) }
	| term '-' term			{ $$ = pp(yylex).op2(MINUS, $1, $3) }
	| term '*' term			{ $$ = pp(yylex).op2(MULT, $1, $3) }
	| term '/' term			{ $$ = pp(yylex).op2(DIVIDE, $1, $3) }
	| term '%' term			{ $$ = pp(yylex).op2(MOD, $1, $3) }
	| term POWER term		{ $$ = pp(yylex).op2(POWER, $1, $3) }
	| '-' term %prec UMINUS		{ $$ = pp(yylex).op1(UMINUS, $2) }
	| '+' term %prec UMINUS		{ $$ = pp(yylex).op1(UPLUS, $2) }
	| NOT term %prec UMINUS		{ $$ = pp(yylex).op1(NOT, pp(yylex).notnull($2)) }
	| BLTIN '(' ')'			{ $$ = pp(yylex).bltin($1, pp(yylex).rectonode()) }
	| BLTIN '(' patlist ')'		{ $$ = pp(yylex).bltin($1, $3) }
	| BLTIN				{ $$ = pp(yylex).bltin($1, pp(yylex).rectonode()) }
	| CALL '(' ')'			{ $$ = pp(yylex).op2(CALL, pp(yylex).celltonode($1, cVAR), nil) }
	| CALL '(' patlist ')'		{ $$ = pp(yylex).op2(CALL, pp(yylex).celltonode($1, cVAR), $3) }
	| CLOSE term			{ $$ = pp(yylex).op1(CLOSE, $2) }
	| DECR var			{ $$ = pp(yylex).op1(PREDECR, $2) }
	| INCR var			{ $$ = pp(yylex).op1(PREINCR, $2) }
	| var DECR			{ $$ = pp(yylex).op1(POSTDECR, $1) }
	| var INCR			{ $$ = pp(yylex).op1(POSTINCR, $1) }
	| GETLINE var LT term		{ $$ = pp(yylex).getline($2, LT, $4) }
	| GETLINE LT term		{ $$ = pp(yylex).getline(nil, LT, $3) }
	| GETLINE var			{ $$ = pp(yylex).getline($2, 0, nil) }
	| GETLINE			{ $$ = pp(yylex).getline(nil, 0, nil) }
	| INDEX '(' pattern comma pattern ')'
		{ $$ = pp(yylex).op2(INDEX, $3, $5) }
	| INDEX '(' pattern comma reg_expr ')'
		{ pp(yylex).in.syntax("index() doesn't permit regular expressions")
		  $$ = pp(yylex).op2(INDEX, $3, nil) }
	| '(' pattern ')'		{ $$ = $2 }
	| MATCHFCN '(' pattern comma reg_expr ')'
		{ $$ = pp(yylex).matchfcn($3, $5) }
	| MATCHFCN '(' pattern comma pattern ')'
		{ $$ = pp(yylex).matchfcnExpr($3, $5) }
	| NUMBER			{ $$ = pp(yylex).celltonode($1, cCON) }
	| SPLIT '(' pattern comma varname comma pattern ')'     /* string */
		{ $$ = pp(yylex).split($3, pp(yylex).makearr($5), $7, "") }
	| SPLIT '(' pattern comma varname comma reg_expr ')'    /* const /regexp/ */
		{ $$ = pp(yylex).splitRE($3, pp(yylex).makearr($5), $7) }
	| SPLIT '(' pattern comma varname ')'
		{ $$ = pp(yylex).split($3, pp(yylex).makearr($5), nil, "") }  /* default */
	| SPRINTF '(' patlist ')'	{ $$ = pp(yylex).op1($1, $3) }
	| string	 		{ $$ = pp(yylex).celltonode($1, cCON) }
	| subop '(' reg_expr comma pattern ')'
		{ $$ = pp(yylex).subRE($1, $3, $5, pp(yylex).rectonode()) }
	| subop '(' pattern comma pattern ')'
		{ $$ = pp(yylex).subExpr($1, $3, $5, pp(yylex).rectonode()) }
	| subop '(' reg_expr comma pattern comma var ')'
		{ $$ = pp(yylex).subRE($1, $3, $5, $7) }
	| subop '(' pattern comma pattern comma var ')'
		{ $$ = pp(yylex).subExpr($1, $3, $5, $7) }
	| SUBSTR '(' pattern comma pattern comma pattern ')'
		{ $$ = pp(yylex).op3(SUBSTR, $3, $5, $7) }
	| SUBSTR '(' pattern comma pattern ')'
		{ $$ = pp(yylex).op3(SUBSTR, $3, $5, nil) }
	| var
	;

var:
	  varname
	| varname '[' patlist ']'	{ $$ = pp(yylex).op2(ARRAY, pp(yylex).makearr($1), $3) }
	| IVAR				{ $$ = pp(yylex).op1(INDIRECT, pp(yylex).celltonode($1, cVAR)) }
	| INDIRECT term	 		{ $$ = pp(yylex).op1(INDIRECT, $2) }
	;

varlist:
	  /* nothing */		{ $$ = nil; pp(yylex).arglist = nil }
	| VAR			{ $$ = pp(yylex).celltonode($1, cVAR); pp(yylex).arglist = $$ }
	| varlist comma VAR	{
			pp(yylex).checkdup($1, $3)
			$$ = pp(yylex).linkum($1, pp(yylex).celltonode($3, cVAR)); pp(yylex).arglist = $$ }
	;

varname:
	  VAR			{ $$ = pp(yylex).celltonode($1, cVAR) }
	| ARG 			{ $$ = pp(yylex).arg($1) }
	| VARNF			{ $$ = pp(yylex).varnf($1) }
	;


while:
	  WHILE '(' pattern rparen	{ $$ = pp(yylex).notnull($3) }
	;

%%
