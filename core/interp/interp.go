// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package interp implements ${...} interpolation: pure string substitution
// over vars and captures — deliberately not an expression language.
package interp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shinari-dev/shinari/utils/conv"
)

var refRe = regexp.MustCompile(`\$\{([^}]*)\}`)

// Scope resolves references. Lookup order: explicit "vars." prefix reads
// Vars; bare names read Captures first, then Vars (a captured name shadows
// a var, last-write-wins).
type Scope struct {
	Vars     map[string]any
	Captures map[string]any
}

// Refs returns every ${...} reference name in s, in order.
func Refs(s string) []string {
	var out []string
	for _, m := range refRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func (sc Scope) lookup(ref string) (any, bool) {
	ref = strings.TrimSpace(ref)
	if name, ok := strings.CutPrefix(ref, "vars."); ok {
		v, found := sc.Vars[name]
		return v, found
	}
	if v, found := sc.Captures[ref]; found {
		return v, true
	}
	v, found := sc.Vars[ref]
	return v, found
}

// String interpolates every ${...} in s. A reference that does not resolve
// is an error naming the reference — including anything that looks like
// arithmetic (`${a - b}`), which is simply an unknown name (there is no
// expression language).
func (sc Scope) String(s string) (string, error) {
	var firstErr error
	out := refRe.ReplaceAllStringFunc(s, func(m string) string {
		ref := m[2 : len(m)-1]
		v, ok := sc.lookup(ref)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("unresolved reference ${%s} (no var or capture by that name; note: Shinari has no expression language)", ref)
			}
			return m
		}
		return Stringify(v)
	})
	return out, firstErr
}

// Value interpolates s, preserving the referenced value's type when the
// whole string is exactly one reference (`with: ${job}`); otherwise it
// behaves like String.
func (sc Scope) Value(s string) (any, error) {
	trimmed := strings.TrimSpace(s)
	if m := refRe.FindStringSubmatch(trimmed); m != nil && m[0] == trimmed {
		v, ok := sc.lookup(m[1])
		if !ok {
			return nil, fmt.Errorf("unresolved reference ${%s} (no var or capture by that name)", m[1])
		}
		return v, nil
	}
	return sc.String(s)
}

// Any walks an already-decoded YAML value (maps, lists, scalars) and
// interpolates every string in it.
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

// Stringify renders a value the way interpolation embeds it: numbers
// canonically, everything else via fmt.
func Stringify(v any) string { return conv.ToString(v) }
