// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package tagexpr compiles JUnit5-style boolean tag expressions into a
// predicate over a scenario's tag set. Operators are ! (not), & (and),
// | (or); parentheses group. Operands are tag identifiers matching
// [A-Za-z0-9_./-]+. Precedence: ! > & > |. It is a dependency-free leaf.
package tagexpr

import "fmt"

// Expr is a compiled tag expression.
type Expr struct {
	fn func(map[string]bool) bool
}

// Eval reports whether the given tags satisfy the expression.
func (e Expr) Eval(tags []string) bool {
	set := make(map[string]bool, len(tags))
	for _, t := range tags {
		set[t] = true
	}
	return e.fn(set)
}

// Compile parses s into an Expr, or returns an error describing the first
// syntax problem.
func Compile(s string) (Expr, error) {
	p := &parser{tokens: tokenize(s)}
	fn, err := p.parseOr()
	if err != nil {
		return Expr{}, err
	}
	if p.pos != len(p.tokens) {
		return Expr{}, fmt.Errorf("unexpected %q in tag expression", p.tokens[p.pos].val)
	}
	return Expr{fn: fn}, nil
}

type tokKind int

const (
	tokIdent tokKind = iota
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
	tokErr
)

type token struct {
	kind tokKind
	val  string
}

func isIdentChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '_' || c == '.' || c == '/' || c == '-':
		return true
	}
	return false
}

func tokenize(s string) []token {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '&':
			toks = append(toks, token{tokAnd, "&"})
			i++
		case c == '|':
			toks = append(toks, token{tokOr, "|"})
			i++
		case c == '!':
			toks = append(toks, token{tokNot, "!"})
			i++
		case c == '(':
			toks = append(toks, token{tokLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tokRParen, ")"})
			i++
		case isIdentChar(c):
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			toks = append(toks, token{tokIdent, s[i:j]})
			i = j
		default:
			toks = append(toks, token{tokErr, string(c)})
			i++
		}
	}
	return toks
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() (token, bool) {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos], true
	}
	return token{}, false
}

func (p *parser) parseOr() (func(map[string]bool) bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tokOr {
			return left, nil
		}
		p.pos++
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(set map[string]bool) bool { return l(set) || r(set) }
	}
}

func (p *parser) parseAnd() (func(map[string]bool) bool, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tokAnd {
			return left, nil
		}
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(set map[string]bool) bool { return l(set) && r(set) }
	}
}

func (p *parser) parseUnary() (func(map[string]bool) bool, error) {
	if t, ok := p.peek(); ok && t.kind == tokNot {
		p.pos++
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return func(set map[string]bool) bool { return !inner(set) }, nil
	}
	return p.parseAtom()
}

func (p *parser) parseAtom() (func(map[string]bool) bool, error) {
	t, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected end of tag expression")
	}
	switch t.kind {
	case tokIdent:
		p.pos++
		name := t.val
		return func(set map[string]bool) bool { return set[name] }, nil
	case tokLParen:
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		c, ok := p.peek()
		if !ok || c.kind != tokRParen {
			return nil, fmt.Errorf("missing ) in tag expression")
		}
		p.pos++
		return inner, nil
	default:
		return nil, fmt.Errorf("unexpected %q in tag expression", t.val)
	}
}
