package parser

import "testing"

// kinds extracts the token kinds (excluding the trailing EOF) for assertions.
func lexVals(src string) []token {
	toks := lex(src)
	return toks[:len(toks)-1] // drop EOF
}

func TestLexDollarQuoteIsOneToken(t *testing.T) {
	src := `DO $$ BEGIN; PERFORM 1; END $$;`
	toks := lexVals(src)
	// Expect: DO, <dollar string>, ;
	if len(toks) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(toks), toks)
	}
	if toks[1].kind != tString {
		t.Errorf("dollar body should be one tString, got %v", toks[1])
	}
	if toks[2].kind != tPunct || toks[2].val != ";" {
		t.Errorf("third token should be ';', got %v", toks[2])
	}
}

func TestLexTaggedDollarQuote(t *testing.T) {
	src := `$body$ a ; b $body$`
	toks := lexVals(src)
	if len(toks) != 1 || toks[0].kind != tString {
		t.Fatalf("tagged dollar quote should be one string token, got %+v", toks)
	}
}

func TestLexComments(t *testing.T) {
	src := "-- line comment\nSELECT /* block /* nested */ still */ 1;"
	toks := lexVals(src)
	// SELECT, 1, ;
	if len(toks) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(toks), toks)
	}
	if toks[0].kind != tWord || toks[0].val != "select" {
		t.Errorf("first token should be 'select', got %v", toks[0])
	}
}

func TestLexQuotedIdentifierPreservesCase(t *testing.T) {
	toks := lexVals(`"MyTable" foo`)
	if toks[0].kind != tWord || !toks[0].quoted || toks[0].val != "MyTable" {
		t.Errorf("quoted ident wrong: %+v", toks[0])
	}
	if toks[1].kind != tWord || toks[1].quoted || toks[1].val != "foo" {
		t.Errorf("unquoted ident should be lowercased/unquoted: %+v", toks[1])
	}
}

func TestLexStringEscapes(t *testing.T) {
	// Doubled-quote escaping keeps the literal as one token.
	toks := lexVals(`'a''b' x`)
	if toks[0].kind != tString || toks[0].val != `'a''b'` {
		t.Errorf("string with '' escape wrong: %+v", toks[0])
	}
	// E-string backslash escape.
	toks = lexVals(`E'a\'b' x`)
	if toks[0].kind != tString || toks[0].val != `E'a\'b'` {
		t.Errorf("E-string wrong: %+v", toks[0])
	}
}

func TestLexPositionalParamNotDollarQuote(t *testing.T) {
	toks := lexVals(`$1`)
	if toks[0].kind != tPunct || toks[0].val != "$" {
		t.Errorf("$1 should lex '$' as punct, got %+v", toks[0])
	}
}
