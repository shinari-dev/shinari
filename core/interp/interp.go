// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package interp implements ${...} interpolation: each ${...} is a jq
// expression evaluated over the scope (vars overlaid by captures). jq is the
// single expression language, shared with read:/capture:.
package interp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shinari-dev/shinari/core/jqx"
	"github.com/shinari-dev/shinari/utils/conv"
)

// refRe matches a ${ jq } reference. The jq body cannot contain a literal `}`
// (so jq object construction like ${ {a: .x} } is unsupported in interpolation);
// reach for those shapes in a read:/capture: step instead.
var refRe = regexp.MustCompile(`\$\{([^}]*)\}`)

// Scope resolves references. The jq input is vars overlaid by captures, so a
// captured name shadows a var of the same name.
type Scope struct {
	Vars     map[string]any
	Captures map[string]any
}

// root builds the jq input document from the scope.
func (sc Scope) root() map[string]any {
	root := make(map[string]any, len(sc.Vars)+len(sc.Captures))
	for k, v := range sc.Vars {
		root[k] = v
	}
	for k, v := range sc.Captures {
		root[k] = v
	}
	return root
}

// Refs returns every ${...} expression in s, in order.
func Refs(s string) []string {
	var out []string
	for _, m := range refRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// String interpolates every ${...} in s, stringifying each jq result. A jq
// parse/eval error is reported, naming the expression; a jq result of null
// renders as empty.
func (sc Scope) String(s string) (string, error) {
	var firstErr error
	root := sc.root()
	out := refRe.ReplaceAllStringFunc(s, func(m string) string {
		expr := m[2 : len(m)-1]
		v, err := jqx.Eval(expr, root)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("interpolating ${%s}: %w", expr, err)
			}
			return m
		}
		return Stringify(v)
	})
	return out, firstErr
}

// Value interpolates s, preserving the jq result's type when the whole string
// is exactly one ${...} (`with: ${.job}`); otherwise it behaves like String.
func (sc Scope) Value(s string) (any, error) {
	trimmed := strings.TrimSpace(s)
	if m := refRe.FindStringSubmatch(trimmed); m != nil && m[0] == trimmed {
		return jqx.Eval(m[1], sc.root())
	}
	return sc.String(s)
}

// Any walks an already-decoded YAML value and interpolates every string in it.
func (sc Scope) Any(v any) (any, error) {
	switch t := v.(type) {
	case string:
		return sc.Value(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			r, err := sc.Any(e)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			r, err := sc.Any(e)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

// Stringify renders a value the way interpolation embeds it.
func Stringify(v any) string { return conv.ToString(v) }
