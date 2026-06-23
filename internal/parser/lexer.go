package parser

import "strings"

// This file implements a small, dependency-free SQL lexer good enough to parse
// PostgreSQL DDL. It is deliberately permissive: it tokenizes the structure
// (identifiers, strings, numbers, punctuation) without validating semantics, so
// the parser above it can recognize the statements it cares about and skip the
// rest. The tricky parts it must get right are the ones that affect statement
// boundaries: comments, quoted identifiers, string literals, and — crucially —
// dollar-quoted strings ($$ ... $$ / $tag$ ... $tag$) used by DO blocks and
// function bodies.

type tokenKind int

const (
	tEOF    tokenKind = iota
	tWord             // identifier or keyword (unquoted -> lowercased; quoted -> verbatim)
	tString           // 'string', E'string', or $$dollar$$ literal
	tNumber           // numeric literal
	tPunct            // punctuation or operator run
)

type token struct {
	kind   tokenKind
	val    string // normalized text (see tokenKind)
	quoted bool   // true for a double-quoted identifier (never matches a keyword)
	start  int    // byte offset of the token start in the source
	end    int    // byte offset just past the token in the source
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '$'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isOperatorChar reports whether c is one of PostgreSQL's operator characters.
func isOperatorChar(c byte) bool {
	switch c {
	case '+', '-', '*', '/', '<', '>', '=', '~', '!', '@', '#', '%', '^', '&', '|', '`', '?', ':':
		return true
	}
	return false
}

// lex tokenizes the whole input. Comments are dropped. The final token is tEOF.
func lex(src string) []token {
	var toks []token
	i := 0
	n := len(src)

	for i < n {
		c := src[i]

		// Whitespace.
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v' {
			i++
			continue
		}

		// Line comment.
		if c == '-' && i+1 < n && src[i+1] == '-' {
			i += 2
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment (nested).
		if c == '/' && i+1 < n && src[i+1] == '*' {
			depth := 1
			i += 2
			for i < n && depth > 0 {
				switch {
				case src[i] == '/' && i+1 < n && src[i+1] == '*':
					depth++
					i += 2
				case src[i] == '*' && i+1 < n && src[i+1] == '/':
					depth--
					i += 2
				default:
					i++
				}
			}
			continue
		}

		// Dollar-quoted string ($$...$$ or $tag$...$tag$).
		if c == '$' {
			if end, ok := scanDollar(src, i); ok {
				toks = append(toks, token{kind: tString, val: src[i:end], start: i, end: end})
				i = end
				continue
			}
			// Not a dollar quote (e.g. $1) — emit '$' as punctuation.
			toks = append(toks, token{kind: tPunct, val: "$", start: i, end: i + 1})
			i++
			continue
		}

		// Escape string E'...'.
		if (c == 'E' || c == 'e') && i+1 < n && src[i+1] == '\'' {
			end := scanString(src, i+1, true)
			toks = append(toks, token{kind: tString, val: src[i:end], start: i, end: end})
			i = end
			continue
		}

		// Ordinary string literal '...'.
		if c == '\'' {
			end := scanString(src, i, false)
			toks = append(toks, token{kind: tString, val: src[i:end], start: i, end: end})
			i = end
			continue
		}

		// Quoted identifier "...".
		if c == '"' {
			j := i + 1
			var b strings.Builder
			for j < n {
				if src[j] == '"' {
					if j+1 < n && src[j+1] == '"' { // "" -> literal quote
						b.WriteByte('"')
						j += 2
						continue
					}
					j++ // closing quote
					break
				}
				b.WriteByte(src[j])
				j++
			}
			toks = append(toks, token{kind: tWord, val: b.String(), quoted: true, start: i, end: j})
			i = j
			continue
		}

		// Identifier / keyword.
		if isIdentStart(c) {
			j := i + 1
			for j < n && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, token{kind: tWord, val: strings.ToLower(src[i:j]), start: i, end: j})
			i = j
			continue
		}

		// Number.
		if isDigit(c) || (c == '.' && i+1 < n && isDigit(src[i+1])) {
			j := i
			for j < n && (isDigit(src[j]) || src[j] == '.' || src[j] == '_') {
				j++
			}
			// optional exponent
			if j < n && (src[j] == 'e' || src[j] == 'E') {
				j++
				if j < n && (src[j] == '+' || src[j] == '-') {
					j++
				}
				for j < n && isDigit(src[j]) {
					j++
				}
			}
			toks = append(toks, token{kind: tNumber, val: src[i:j], start: i, end: j})
			i = j
			continue
		}

		// Single-character structural punctuation.
		if c == '(' || c == ')' || c == ',' || c == ';' || c == '[' || c == ']' || c == '.' {
			toks = append(toks, token{kind: tPunct, val: string(c), start: i, end: i + 1})
			i++
			continue
		}

		// Operator run.
		if isOperatorChar(c) {
			j := i + 1
			for j < n && isOperatorChar(src[j]) {
				j++
			}
			toks = append(toks, token{kind: tPunct, val: src[i:j], start: i, end: j})
			i = j
			continue
		}

		// Anything else: emit as a single-char punct so position tracking stays sane.
		toks = append(toks, token{kind: tPunct, val: string(c), start: i, end: i + 1})
		i++
	}

	toks = append(toks, token{kind: tEOF, start: n, end: n})
	return toks
}

// scanString returns the offset just past a single-quoted string starting at
// src[start] == '\”. With escape=true ('E' strings) a backslash escapes the
// next character; doubled quotes (”) always escape a quote.
func scanString(src string, start int, escape bool) int {
	n := len(src)
	j := start + 1
	for j < n {
		if escape && src[j] == '\\' && j+1 < n {
			j += 2
			continue
		}
		if src[j] == '\'' {
			if j+1 < n && src[j+1] == '\'' {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return n
}

// scanDollar tests whether src[i] begins a dollar-quoted string and, if so,
// returns the offset just past its closing tag. A bare "$" not forming a valid
// open tag (e.g. the "$1" of a positional parameter) returns ok=false.
func scanDollar(src string, i int) (int, bool) {
	n := len(src)
	j := i + 1
	for j < n && src[j] != '$' && isIdentPart(src[j]) {
		j++
	}
	if j >= n || src[j] != '$' {
		return 0, false
	}
	tag := src[i : j+1] // includes both '$' delimiters
	rest := src[j+1:]
	k := strings.Index(rest, tag)
	if k < 0 {
		return n, true // unterminated: consume to EOF
	}
	return j + 1 + k + len(tag), true
}
